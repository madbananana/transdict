package gui

import (
	"strings"
	"testing"

	"transdict/internal/dict"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
)

func TestContextOfTreatsMissingResultAsMain(t *testing.T) {
	if got := contextOf(nil); got != settings.ContextMain {
		t.Errorf("contextOf(nil) = %q", got)
	}
	if got := contextOf(&query.Response{Context: settings.ContextClipboard}); got != settings.ContextClipboard {
		t.Errorf("contextOf = %q", got)
	}
}

func TestSameTextIgnoresWhitespace(t *testing.T) {
	if !sameText("  hello   world ", "hello world") {
		t.Error("换行与多余空格不应算作差异")
	}
	if sameText("hello world", "hello there") {
		t.Error("不同译文被判为相同")
	}
}

func TestDropUnchangedKeepsFailures(t *testing.T) {
	items := []translate.Result{
		{Label: "echo", Text: "  hello   world "},
		{Label: "real", Text: "Bonjour le monde"},
		{Label: "broken", Text: "hello world", Error: "超时"},
	}
	got := dropUnchanged(items, "hello world")
	if len(got) != 2 {
		t.Fatalf("保留 %d 条, 期望 2: %+v", len(got), got)
	}
	// A service that failed is not evidence that the text is already in the
	// target language, so it stays and still shows its error.
	if got[0].Label != "real" || got[1].Label != "broken" {
		t.Errorf("保留顺序错误: %v, %v", got[0].Label, got[1].Label)
	}
}

func result(labels ...string) []translate.Result {
	out := make([]translate.Result, 0, len(labels))
	for _, l := range labels {
		if strings.HasPrefix(l, "!") {
			out = append(out, translate.Result{Label: l[1:], Error: "失败"})
			continue
		}
		out = append(out, translate.Result{Label: l, Text: l})
	}
	return out
}

func TestAnalyzeNothingRanIsExplained(t *testing.T) {
	cfg := settings.Config{Translate: []settings.Provider{
		{ID: "ai", Category: settings.CatTranslate, Kind: "openai", Label: "AI 模型", Enabled: true},
	}}
	res := &query.Response{SourceText: "hello"}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain})

	if res.Error != "没有可用的服务" {
		t.Fatalf("没有服务运行时必须给出原因，得到 %q", res.Error)
	}
	// The names travel apart from the sentence: Go words the service labels and
	// the window words the sentence around them, in whatever language it draws.
	if len(res.Skipped) != 1 || res.Skipped[0] != "AI 模型" {
		t.Errorf("Skipped = %v, want [AI 模型]", res.Skipped)
	}
}

// A screenshot with no configured engine is the Windows first-run case, and
// 未识别到文字 would blame the picture for work the app was never able to do.
func TestAnalyzeScreenshotNamesTheMissingEngine(t *testing.T) {
	cfg := settings.Config{OCR: []settings.Provider{
		{ID: "ocr-baidu", Category: settings.CatOCR, Kind: "baidu", Label: "百度通用文字识别", Enabled: true},
	}}
	res := &query.Response{}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextScreenshot, Image: []byte{1}})

	if res.Error != "没有可用的文字识别服务" {
		t.Fatalf("得到 %q", res.Error)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "百度通用文字识别" {
		t.Errorf("Skipped = %v, want [百度通用文字识别]", res.Skipped)
	}
}

// 图片 OCR 后自动查询 off leaves a screenshot answer with text and nothing else.
// That is the setting working, not a service that failed to start.
func TestAnalyzeScreenshotThatStopsAtTextIsQuiet(t *testing.T) {
	cfg := settings.Config{AutoQueryOCR: false}
	res := &query.Response{SourceText: "hello world", ImageURL: "data:image/png;base64,eA==",
		OCR: []ocr.Outcome{{Provider: "ocr", Label: "苹果 OCR", Blocks: []ocr.Block{{Text: "hello world"}}}}}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextScreenshot, Image: []byte{1}})

	if res.Error != "" {
		t.Errorf("关闭自动查询后不该报错，得到 %q", res.Error)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("Skipped = %v, want 空", res.Skipped)
	}
}

func TestAnalyzeSilentWhenOnlyEchoesRemain(t *testing.T) {
	cfg := settings.Config{HideSameLang: true}
	res := &query.Response{SourceText: "hello world", Translate: result("hello world", " hello   world ")}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain})

	if res.Error != "原文已是目标语言" {
		t.Errorf("全部回显时应说明原因，得到 %q", res.Error)
	}
	if len(res.Translate) != 0 {
		t.Errorf("回显应被隐藏，剩下 %+v", res.Translate)
	}
}

func TestAnalyzeKeepsDictionaryOnlyQuiet(t *testing.T) {
	cfg := settings.Config{}
	res := &query.Response{SourceText: "word", Dict: []dict.Result{{Label: "苹果词典", Entries: []dict.Entry{{Text: "释义"}}}}}
	(&Service{}).annotate(res, cfg, query.Request{Context: settings.ContextMain, DictionaryOnly: true})
	if res.Error != "" {
		t.Errorf("词典模式有结果时不该报错，得到 %q", res.Error)
	}

	// 词典模式 deliberately runs no translators, so an empty translate list is
	// not a problem to complain about.
	empty := &query.Response{SourceText: "word", Dict: []dict.Result{{Label: "苹果词典"}}}
	(&Service{}).annotate(empty, cfg, query.Request{Context: settings.ContextMain, DictionaryOnly: true})
	if empty.Error != "" {
		t.Errorf("词典模式不该提示没有翻译服务，得到 %q", empty.Error)
	}
}

func TestAnalyzeLeavesErrorsAndEmptyTextAlone(t *testing.T) {
	cfg := settings.Config{}
	withError := &query.Response{SourceText: "x", Error: "已经说过了"}
	(&Service{}).annotate(withError, cfg, query.Request{})
	if withError.Error != "已经说过了" {
		t.Errorf("已有错误不该被覆盖: %q", withError.Error)
	}

	blank := &query.Response{SourceText: ""}
	(&Service{}).annotate(blank, cfg, query.Request{})
	if blank.Error != "" {
		t.Errorf("没有原文时不该补充说明: %q", blank.Error)
	}
}

func TestStageLabelNamesTheWork(t *testing.T) {
	cases := []struct {
		req  query.Request
		want string
	}{
		{query.Request{Text: "hi"}, "查询中…"},
		{query.Request{Image: []byte{1}}, "识别文字中…"},
		{query.Request{Text: "hi", Prompt: "润色"}, "AI 处理中…"},
	}
	for _, c := range cases {
		if got := stageLabel(c.req); got != c.want {
			t.Errorf("stageLabel(%+v) = %q, 期望 %q", c.req, got, c.want)
		}
	}
}

func TestANewQueryReplacesTheOneBeforeIt(t *testing.T) {
	s := &Service{}
	firstCtx, first, _ := s.beginRun(settings.ContextMain)
	if !s.currentRun(settings.ContextMain, first) {
		t.Fatal("刚发起的查询不是当前的")
	}

	_, second, secondCancel := s.beginRun(settings.ContextMain)
	defer secondCancel()
	select {
	case <-firstCtx.Done():
	default:
		t.Error("新查询没有停掉被替代的那一个")
	}
	if s.currentRun(settings.ContextMain, first) {
		t.Error("被替代的查询仍被当作当前查询")
	}
	if !s.currentRun(settings.ContextMain, second) {
		t.Error("新查询不是当前查询")
	}

	// Another context has its own queue: replacing the panel's query must not
	// cancel what a clipboard query is still waiting for.
	_, clip, clipCancel := s.beginRun(settings.ContextClipboard)
	defer clipCancel()
	if !s.currentRun(settings.ContextClipboard, clip) || !s.currentRun(settings.ContextMain, second) {
		t.Error("不同上下文的查询互相影响")
	}
}

// 润色 and 总结 run on the translation stage; marking them as dictionary-only
// removes that stage and every chip answers "没有可用的服务".
func TestPromptRequestIsNotADictionaryQuery(t *testing.T) {
	req := promptRequest("hello", "润色")
	if req.DictionaryOnly {
		t.Error("润色/总结被当成了仅词典查询：翻译阶段会被整段跳过")
	}
	if req.Context != settings.ContextTranslation || req.Mode != "prompt" || req.Prompt != "润色" {
		t.Errorf("promptRequest = %+v", req)
	}
}

func TestCleanQueryTextRemovesCommentSymbols(t *testing.T) {
	cfg := settings.Config{AutoRemoveComments: true}
	cases := map[string]string{
		"// The quick brown\n// fox jumps": "The quick brown fox jumps",
		"# build:\n# make all":             "build: make all",
		"// 待办\n// 明天处理":                   "待办明天处理",
		"/* hello world */":                "hello world",
		"code := 1\n/* note */":            "code := 1\nnote",
		"plain text":                       "plain text",
	}
	for in, want := range cases {
		if got := cleanQueryText(in, cfg); strings.TrimSpace(got) != want {
			t.Errorf("cleanQueryText(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestCleanQueryTextKeepsTextWhenOptionsAreOff(t *testing.T) {
	in := "// The quick\n// brown fox"
	if got := cleanQueryText(in, settings.Config{}); got != in {
		t.Errorf("选项关闭时不该改动原文: %q", got)
	}
}

func TestCleanQueryTextJoinsLinesOnRequest(t *testing.T) {
	cfg := settings.Config{ReplaceNewlineSpace: true}
	got := cleanQueryText("one\n two\r\nthree", cfg)
	if got != "one two three" {
		t.Errorf("换行未替换成空格: %q", got)
	}
}

// 快捷动作 rewrites the text the same way the query-time options do, but on
// demand: the menu and the settings switches must never drift apart.
func TestTransformTextQuickActions(t *testing.T) {
	var s Service
	cases := []struct{ action, in, want string }{
		{"newline", "one\n two\r\nthree", "one two three"},
		{"comments", "// The quick\n// fox", "The quick fox"},
		{"comments", "/* hello */", "hello"},
		{"words", "key_value", "key value"},
		{"words", "hello world", "hello world"},
		{"unknown", "as is", "as is"},
	}
	for _, c := range cases {
		if got := s.TransformText(c.in, c.action); got != c.want {
			t.Errorf("TransformText(%q, %q) = %q, 期望 %q", c.in, c.action, got, c.want)
		}
	}
}

// 查询时自动分词 only touches a single token: a sentence keeps its own spaces.
func TestCleanQueryTextSegmentsASingleIdentifier(t *testing.T) {
	cfg := settings.Config{AutoWordSegment: true}
	cases := map[string]string{
		"key_value":              "key value",
		"anchoredDraggableState": "anchored Draggable State",
		"GetHTTPCode":            "Get HTTP Code",
		"hello world":            "hello world",
		"HTTPServer":             "HTTP Server",
	}
	for in, want := range cases {
		if got := cleanQueryText(in, cfg); got != want {
			t.Errorf("cleanQueryText(%q) = %q, 期望 %q", in, got, want)
		}
	}
	if got := cleanQueryText("key_value", settings.Config{}); got != "key_value" {
		t.Errorf("关闭分词后不该改动原文: %q", got)
	}
}

// The dictionary is what tells an identifier from a word with capitals inside;
// the test stands in for Dictionary Services so it runs on any machine.
func TestSegmentationLeavesKnownWordsAlone(t *testing.T) {
	old := knownWord
	knownWord = func(s string) bool { return s == "iPad" || s == "LaTeX" }
	defer func() { knownWord = old }()

	for _, word := range []string{"iPad", "LaTeX"} {
		if got := segmentWords(word); got != word {
			t.Errorf("%q 被拆成了 %q", word, got)
		}
	}
	if got := segmentWords("goodMorning"); got != "good Morning" {
		t.Errorf("goodMorning = %q, 期望拆开", got)
	}
	if got := segmentWords("'UIKit'"); got != "UIKit" {
		t.Errorf("引号包裹的词 = %q, 期望只去掉引号", got)
	}
	if got := segmentWords("“UIKit”"); got != "UIKit" {
		t.Errorf("中文引号 = %q, 期望只去掉引号", got)
	}
	if got := segmentWords("plain text here"); got != "plain text here" {
		t.Errorf("含空格的文本不该分词: %q", got)
	}
}

// 英文发音 only changes the voice for English; a Chinese sentence keeps the code
// its detection produced, and the default stays on the American voice.
func TestSpeakLangPicksTheEnglishVoice(t *testing.T) {
	cases := []struct{ lang, pref, want string }{
		{"en", "us", "en-US"},
		{"en", "uk", "en-GB"},
		{"en-US", "uk", "en-GB"},
		{"zh-CN", "uk", "zh-CN"},
		{"auto", "uk", "auto"},
		{"en", "", "en-US"},
	}
	for _, c := range cases {
		if got := speakLang(c.lang, c.pref); got != c.want {
			t.Errorf("speakLang(%q, %q) = %q, want %q", c.lang, c.pref, got, c.want)
		}
	}
}
