//go:build darwin

package settings

// addAppleServices prepends the three on-device Apple services a darwin build
// registers — Vision OCR, the system dictionary, `say` — and puts each first in
// the contexts that use it, because an answer from the machine itself should be
// asked for before one that needs a key and a network.
func addAppleServices(c *Config) {
	addFirst(&c.OCR, Provider{ID: "ocr-apple", Category: CatOCR, Kind: "apple",
		Label: "苹果 OCR", Enabled: true})
	addFirst(&c.Dict, Provider{ID: "dict-apple", Category: CatDict, Kind: "apple",
		Label: "苹果词典", Enabled: true})
	addFirst(&c.TTS, Provider{ID: "tts-apple", Category: CatTTS, Kind: "apple",
		Label: "苹果朗读", Enabled: true})

	selectFirst(c, ContextMain, "dict-apple")
	selectFirst(c, ContextClipboard, "dict-apple")
	selectFirst(c, ContextScreenshot, "ocr-apple")
	selectFirst(c, ContextSilentOCR, "ocr-apple")
	selectFirst(c, ContextODict, "dict-apple")
}
