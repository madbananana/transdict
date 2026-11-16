package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailOfKeepsTheLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	body := strings.Join([]string{"one", "two", "three", "four"}, "\n")
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tailOf(path, 2); got != "three\nfour" {
		t.Errorf("tailOf(2) = %q, want the last two lines", got)
	}
}

func TestTailOfHandlesEmptyAndMissingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tailOf(path, 50); got != "" {
		t.Errorf("empty file: %q", got)
	}
	if got := tailOf(filepath.Join(t.TempDir(), "nope"), 50); got != "" {
		t.Errorf("missing file: %q", got)
	}
	if got := tailOf("", 50); got != "" {
		t.Errorf("no path: %q", got)
	}
}
