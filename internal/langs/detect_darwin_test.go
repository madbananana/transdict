//go:build darwin

package langs

import "testing"

// 光 is one Han character the recogniser can call Chinese or Japanese. The
// reader's pair is what decides, which is the point of 语种识别.
func TestDetectWithHintsPrefersTheReadersPair(t *testing.T) {
	if got := DetectWithHints("光", "zh-CN", "en"); got != "zh-CN" {
		t.Errorf("以简体中文为首选时 DetectWithHints(光) = %q, want zh-CN", got)
	}
	if got := DetectWithHints("光", "ja", "en"); got != "ja" {
		t.Errorf("以日语为首选时 DetectWithHints(光) = %q, want ja", got)
	}
}

// A hint is a preference, not an override: English prose stays English even
// when both hints are Chinese.
func TestDetectWithHintsKeepsUnmistakableText(t *testing.T) {
	const prose = "The quick brown fox jumps over the lazy dog."
	if got := DetectWithHints(prose, "zh-CN", "ja"); got != "en" {
		t.Errorf("DetectWithHints(English) = %q, want en", got)
	}
}
