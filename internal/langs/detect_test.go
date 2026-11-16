package langs

import "testing"

func TestDetectNamesTheMajorLanguages(t *testing.T) {
	cases := map[string]string{
		"今天天气很好，我们去公园吧。":            "zh-CN",
		"こんにちは世界":                   "ja",
		"안녕하세요 반갑습니다":               "ko",
		"Hello, how are you today?": "en",
		"Привет, как дела?":         "ru",
		"مرحبا بالعالم":             "ar",
	}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %q, want %q", text, got, want)
		}
	}
	if got := Detect("   "); got != Auto {
		t.Errorf("Detect(blank) = %q, want %q", got, Auto)
	}
}

// 语种识别 may be left unset; the recogniser must then run exactly as before.
func TestDetectWithHintsFallsBackWhenNoHintIsSet(t *testing.T) {
	const text = "今天天气很好，我们去公园吧。"
	if got := DetectWithHints(text, "", "auto", "zh-classical"); got != Detect(text) {
		t.Errorf("DetectWithHints(无提示) = %q, want %q", got, Detect(text))
	}
	if got := DetectWithHints("  ", "en"); got != Auto {
		t.Errorf("DetectWithHints(空白) = %q, want %q", got, Auto)
	}
}

func TestCleanHintsDropsWhatTheRecogniserCannotUse(t *testing.T) {
	got := cleanHints([]string{"en", "", "auto", "zh-classical", "en", "zh-CN"})
	want := []string{"en", "zh-CN"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("cleanHints = %v, want %v", got, want)
	}
}

func TestNormalizeCodeSpeaksTheAppTable(t *testing.T) {
	cases := map[string]string{
		"zh-Hans": "zh-CN",
		"zh-Hant": "zh-TW",
		"zh":      "zh-CN",
		"nb":      "no",
		"iw":      "he",
		"en":      "en",
		"":        Auto,
	}
	for in, want := range cases {
		if got := normalizeCode(in); got != want {
			t.Errorf("normalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
}
