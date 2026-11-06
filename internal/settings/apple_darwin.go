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
	selectFirst(c, ContextFloating, "dict-apple")
	selectFirst(c, ContextScreenshot, "ocr-apple")
	selectFirst(c, ContextSilentOCR, "ocr-apple")
	selectFirst(c, ContextODict, "dict-apple")
}

func addFirst(list *[]Provider, p Provider) {
	*list = append([]Provider{p}, *list...)
}

// selectFirst keeps the checked list empty where a context has no default, so
// "nothing selected" — which plan reads as "everything enabled" — is not turned
// into "only this one service".
func selectFirst(c *Config, context, id string) {
	ids := c.Selections[context]
	if len(ids) == 0 {
		return
	}
	for _, have := range ids {
		if have == id {
			return
		}
	}
	c.Selections[context] = append([]string{id}, ids...)
}
