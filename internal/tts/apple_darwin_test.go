//go:build darwin

package tts

import (
	"context"
	"testing"

	"transdict/internal/settings"
)

// These assertions cover the voice-selection table only; nothing here runs the
// real `say` synthesis, per the darwin test rule.
func TestAppleVoiceTable(t *testing.T) {
	cases := map[string]string{
		"zh-CN": "Ting-Ting",
		"zh-HK": "Sin-ji",
		"en":    "Alex",
		"en-GB": "Moira",
		"en-AU": "Karen",
		"ja":    "Kyoko",
		"fr":    "Thomas",
		"de":    "Anna",
		"en-NZ": "Alex", // falls back to the base prefix lookup
		"xx-YY": "",     // unknown: system default (empty)
	}
	for lang, want := range cases {
		if got := appleVoice(lang); got != want {
			t.Errorf("appleVoice(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestAppleRates(t *testing.T) {
	if appleRateNormal != 190 {
		t.Errorf("normal rate = %d, want 190", appleRateNormal)
	}
	if appleRateSlow != 120 {
		t.Errorf("slow rate = %d, want 120", appleRateSlow)
	}
}

func TestAppleEmptyTextShortCircuits(t *testing.T) {
	p, err := New(settings.Provider{Kind: "apple", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	// Empty text returns before exec, so this never shells out to `say`.
	if _, err := p.Speak(context.Background(), "", "en", false); err == nil {
		t.Fatal("expected an error for empty text, and no synthesis run")
	}
}
