// apple-translate reads one JSON request from stdin and answers with one JSON
// object. It is the only way to reach macOS's on-device Translation framework
// from the Go app: TranslationSession is Swift-only and async, so it lives in
// a helper that is compiled at package time and signed with the bundle.
//
// Request:  {"text":"hello","source":"en","target":"zh-CN"}
// Response: {"text":"你好"} or {"error":"..."}

import Foundation
import Translation
import NaturalLanguage

struct Request: Decodable {
    var text: String
    var source: String?
    var target: String
}

struct Response: Encodable {
    var text: String?
    var error: String?
}

func emit(_ response: Response) {
    let encoder = JSONEncoder()
    if let data = try? encoder.encode(response), let line = String(data: data, encoding: .utf8) {
        print(line)
    }
    exit(response.error == nil ? 0 : 1)
}

func detectLanguage(_ text: String) -> String {
    let recognizer = NLLanguageRecognizer()
    recognizer.processString(text)
    return recognizer.dominantLanguage?.rawValue ?? "en"
}

@main
struct AppleTranslate {
    static func main() async {
        let data = FileHandle.standardInput.readDataToEndOfFile()
        guard let request = try? JSONDecoder().decode(Request.self, from: data) else {
            emit(Response(error: "无法解析翻译请求"))
            return
        }
        let text = request.text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else {
            emit(Response(error: "没有可翻译的文本"))
            return
        }

        // TranslationSession(installedSource:) is the only headless entry
        // point and arrived in macOS 26; older systems keep the clear error.
        guard #available(macOS 26.0, *) else {
            emit(Response(error: "苹果翻译需要 macOS 26 或更高版本"))
            return
        }

        var sourceID = request.source ?? ""
        if sourceID.isEmpty || sourceID == "auto" {
            sourceID = detectLanguage(text)
        }
        let source = Locale.Language(identifier: sourceID)
        let target = Locale.Language(identifier: request.target)

        let availability = LanguageAvailability()
        let status = await availability.status(from: source, to: target)
        if status == .unsupported {
            emit(Response(error: "苹果翻译不支持 \(sourceID) → \(request.target)"))
            return
        }
        if status == .supported {
            // The pair works in principle but the model is not on this Mac;
            // downloading is a System Settings action the user has to take.
            emit(Response(error: "苹果翻译尚未下载 \(sourceID) → \(request.target) 的语言包，请在系统设置中下载"))
            return
        }

        let session = TranslationSession(installedSource: source, target: target)
        if await !session.isReady {
            emit(Response(error: "苹果翻译正在准备语言包，请稍后再试"))
            return
        }

        do {
            let response = try await session.translate(text)
            emit(Response(text: response.targetText))
        } catch {
            emit(Response(error: "苹果翻译失败：\(error.localizedDescription)"))
        }
    }
}
