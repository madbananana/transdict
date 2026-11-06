package tts

import (
	"encoding/json"
	"strings"
	"testing"
)

// The bridge sends only Clip.URL, so a provider that answered with bytes has
// to be folded into a data: URL before the window sees it.
func TestPlayableTurnsBytesIntoADataURL(t *testing.T) {
	r := Result{Clip: Clip{MIME: "audio/mpeg", Data: []byte("ID3fake")}}
	got := r.Playable()

	if !strings.HasPrefix(got.Clip.URL, "data:audio/mpeg;base64,") {
		t.Errorf("URL = %q, want a data: URL", got.Clip.URL)
	}
	if got.Clip.Data != nil {
		t.Errorf("Data = %d bytes, want it cleared after folding", len(got.Clip.Data))
	}
	if r.Clip.URL != "" || len(r.Clip.Data) == 0 {
		t.Error("Playable must not mutate the receiver")
	}

	// This is the shape the webview actually reads.
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"url":"data:audio/mpeg;base64,`) {
		t.Errorf("bridge JSON = %s, want the data URL in clip.url", b)
	}
}

func TestPlayableKeepsAProviderURL(t *testing.T) {
	r := Result{Clip: Clip{MIME: "audio/mpeg", URL: "https://speech.test/a.mp3"}}
	got := r.Playable()
	if got.Clip.URL != "https://speech.test/a.mp3" {
		t.Errorf("URL = %q, want the provider URL untouched", got.Clip.URL)
	}
}

func TestPlayableLeavesAnEmptyClipEmpty(t *testing.T) {
	got := Result{Error: "没有可用的朗读服务"}.Playable()
	if got.Clip.URL != "" || got.Clip.Data != nil {
		t.Errorf("clip = %+v, want empty", got.Clip)
	}
}
