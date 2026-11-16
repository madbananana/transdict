package logging

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetupAtMirrorsTheLog(t *testing.T) {
	defer log.SetOutput(os.Stderr)
	dir := t.TempDir()
	path, err := SetupAt(dir)
	if err != nil {
		t.Fatalf("SetupAt: %v", err)
	}
	if path != filepath.Join(dir, "transdict.log") {
		t.Errorf("path = %q", path)
	}
	log.Print("hello from the test")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "hello from the test") {
		t.Errorf("log file = %q, want the printed line", data)
	}
}

func TestSetupAtNeedsADirectory(t *testing.T) {
	if _, err := SetupAt(""); err == nil {
		t.Error("SetupAt(\"\") should fail")
	}
}

func TestDirIsThePlatformLogFolder(t *testing.T) {
	dir := Dir()
	if dir == "" {
		t.Fatal("Dir returned nothing")
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.HasSuffix(dir, filepath.Join("Library", "Logs", "Transdict")) {
			t.Errorf("Dir = %q", dir)
		}
	case "windows":
		if !strings.HasSuffix(dir, filepath.Join("Transdict", "logs")) {
			t.Errorf("Dir = %q", dir)
		}
	}
}
