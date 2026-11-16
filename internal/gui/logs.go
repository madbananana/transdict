package gui

import (
	"errors"
	"os"
	"strings"

	"transdict/internal/logging"
)

// LogTail is the debug window's content: the last lines of the file Setup
// writes. The window polls it, so the page needs no file access of its own and
// a hidden window costs one read per second at most.
func (s *Service) LogTail(lines int) string {
	return tailOf(logging.Path(), lines)
}

// tailOf reads the last lines of a log file. The tail is capped both ways: a
// caller asking for everything gets 300 lines, and the window's 600 is the
// ceiling, because the debug view is for the most recent support case.
func tailOf(path string, lines int) string {
	if lines <= 0 {
		lines = 300
	}
	if lines > 2000 {
		lines = 2000
	}
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return ""
	}
	all := strings.Split(text, "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// LogFile is the path the debug window's footer names, so a copied line can be
// found again in Finder without guessing where it lives.
func (s *Service) LogFile() string { return logging.Path() }

// logBytes is 导出日志's source. Reading the live file rather than copying it
// keeps the export consistent even if the logger appends while the save panel
// is open; a build that has not logged yet is a choice of message, not a crash.
func logBytes() ([]byte, error) {
	path := logging.Path()
	if path == "" {
		return nil, errors.New("日志目录不可用")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("还没有日志")
		}
		return nil, err
	}
	return data, nil
}
