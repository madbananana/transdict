// Package logging mirrors the process log to a per-user file, which is what
// EasyDict's 高级 → 日志 exposes: evidence for a user report without asking
// them to launch the app from a terminal.
package logging

import (
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// maxBytes caps the file at 2 MiB. A rotated copy is kept as .1, which is one
// support session; anything older is noise.
const maxBytes = 2 << 20

// Dir is where the platform keeps per-user logs: ~/Library/Logs/Transdict on
// macOS, %LOCALAPPDATA%\Transdict\logs on Windows. Empty means the machine
// would not name a home directory.
func Dir() string {
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "Transdict", "logs")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", "Transdict")
}

// Path is the file Setup writes, for the debug window that reads it back.
func Path() string {
	dir := Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "transdict.log")
}

// Setup installs the file beside the console copy. `open -g` discards stderr,
// so without the file a shipped build explains nothing when something fails.
func Setup() (string, error) {
	return SetupAt(Dir())
}

// SetupAt is Setup for a named directory, which is what a test needs: the real
// one belongs to the person running the app.
func SetupAt(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("日志目录不可用")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "transdict.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return path, nil
}

// Reveal opens the log directory in the platform's file manager, EasyDict's
// 在 Finder 中显示.
func Reveal() error {
	dir := Dir()
	if dir == "" {
		return errors.New("日志目录不可用")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
