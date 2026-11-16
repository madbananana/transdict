package query

import (
	"testing"

	"transdict/internal/settings"
)

// An API caller names the service it wants; the window's checkboxes are the
// user's business, not the caller's.
func TestPickKindRunsTheNamedServiceOnly(t *testing.T) {
	cfg := settings.Default()
	if got := pickKind(cfg, settings.CatTranslate, "google"); len(got) != 1 || got[0].Kind != "google" {
		t.Fatalf("pickKind(google) = %+v, want the Google row alone", got)
	}
	if got := pickKind(cfg, settings.CatDict, "youdao"); len(got) != 1 || got[0].Kind != "youdao" {
		t.Fatalf("pickKind(dict youdao) = %+v, want the 有道词典 row alone", got)
	}
	if got := pickKind(cfg, settings.CatTranslate, "nope"); got != nil {
		t.Errorf("pickKind(nope) = %+v, want nothing", got)
	}

	off := cfg
	for i := range off.Translate {
		if off.Translate[i].Kind == "google" {
			off.Translate[i].Enabled = false
		}
	}
	if got := pickKind(off, settings.CatTranslate, "google"); got != nil {
		t.Errorf("a disabled service was still run: %+v", got)
	}
}
