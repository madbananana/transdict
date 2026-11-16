package langs

import "strings"

// Detect names the language of a piece of text for the local HTTP API, which
// answers with a BCP-47 code the way Easydict's /detect does. macOS recognizes
// with the system model; every other platform falls back to scripts, which
// separates the major writing systems but cannot tell French from English.
func Detect(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return Auto
	}
	return normalizeCode(detectText(text))
}

// DetectWithHints names the language of text while telling the recogniser
// which languages the caller expects. A word like 光 could be Chinese or
// Japanese; hinting at the two languages the user reads picks the one they
// meant, which is the whole of Easydict's 语种识别 section.
func DetectWithHints(text string, hints ...string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return Auto
	}
	hs := cleanHints(hints)
	if len(hs) == 0 {
		return normalizeCode(detectText(text))
	}
	return normalizeCode(detectTextHinted(text, hs))
}

// cleanHints drops the codes no recogniser can be pointed at: the detection
// sentinel, 古文 (a genre the model names on its own) and empty entries.
func cleanHints(hints []string) []string {
	out := make([]string, 0, len(hints))
	seen := map[string]bool{}
	for _, h := range hints {
		h = strings.TrimSpace(h)
		if h == "" || h == Auto || h == Classical || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// normalizeCode folds the recogniser's codes into the table the app speaks:
// Chinese arrives as zh-Hans/zh-Hant, and Norwegian as nb.
func normalizeCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "":
		return Auto
	case "zh-hans", "zh-cn", "zh":
		return "zh-CN"
	case "zh-hant", "zh-tw", "zh-hk":
		return "zh-TW"
	case "nb", "nn":
		return "no"
	case "iw":
		return "he"
	case "in":
		return "id"
	}
	return code
}
