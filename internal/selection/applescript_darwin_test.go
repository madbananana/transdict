//go:build darwin

package selection

import (
	"strings"
	"testing"
)

func TestBrowserSelectionScriptNamesBrowsersOnly(t *testing.T) {
	safari := browserSelectionScript("com.apple.Safari")
	if !strings.Contains(safari, "do JavaScript") || !strings.Contains(safari, "com.apple.Safari") {
		t.Errorf("Safari script = %q", safari)
	}
	chrome := browserSelectionScript("com.google.Chrome")
	if !strings.Contains(chrome, "front window's active tab") || !strings.Contains(chrome, "com.google.Chrome") {
		t.Errorf("Chrome script = %q", chrome)
	}
	if got := browserSelectionScript("com.apple.TextEdit"); got != "" {
		t.Errorf("TextEdit script = %q, want none", got)
	}
}
