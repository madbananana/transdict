//go:build !darwin

package langs

import "unicode"

// detectText is the script-level fallback for platforms without a language
// model. It separates writing systems — which is what a client needs to pick a
// target language — but cannot tell Latin languages apart.
// detectTextHinted has no language model to bias, so the script counts decide
// alone; the hints would only pretend to be used.
func detectTextHinted(text string, _ []string) string {
	return detectText(text)
}

func detectText(text string) string {
	counts := map[string]int{}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			counts["ja"] += 2
		case unicode.Is(unicode.Hangul, r):
			counts["ko"] += 2
		case unicode.Is(unicode.Han, r):
			counts["zh-CN"]++
		case unicode.Is(unicode.Cyrillic, r):
			counts["ru"]++
		case unicode.Is(unicode.Arabic, r):
			counts["ar"]++
		case unicode.Is(unicode.Thai, r):
			counts["th"]++
		case unicode.Is(unicode.Hebrew, r):
			counts["he"]++
		case unicode.Is(unicode.Greek, r):
			counts["el"]++
		case unicode.Is(unicode.Devanagari, r):
			counts["hi"]++
		case unicode.IsLetter(r):
			counts["en"]++
		}
	}
	best, bestCount := "", 0
	for code, n := range counts {
		if n > bestCount {
			best, bestCount = code, n
		}
	}
	return best
}
