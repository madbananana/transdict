// Bound methods that need a native file panel. The panels run on the main
// thread inside Wails, so a bound method may wait on one from its own goroutine
// and still show it in front of the window the user is looking at.
package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"transdict/internal/history"
	"transdict/internal/l10n"
)

// PickDirectory answers a field declared Folder: true. Typing the path to a
// dictionary folder is the one thing nobody gets right first time.
func (s *Service) PickDirectory() (string, error) {
	return s.wails.Dialog.OpenFile().
		SetMessage(l10n.T("选择文件夹")).
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}

// ExportConfigFile writes the settings — service keys included, since a config
// without them is not worth moving between machines — and returns the path.
// An empty path means the panel was closed, which is a choice, not a failure.
func (s *Service) ExportConfigFile() (string, error) {
	data, err := json.MarshalIndent(s.cfg.Get(), "", "  ")
	if err != nil {
		return "", err
	}
	path, err := s.wails.Dialog.SaveFile().
		SetMessage(l10n.T("导出设置")).
		SetFilename("transdict-config.json").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	path = withExt(path, "json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ImportConfigFile reads a config the user picked and applies it whole. The
// store refuses a file that shares no field with a Config rather than resetting
// the app with whatever JSON was selected.
func (s *Service) ImportConfigFile() (string, error) {
	path, err := s.wails.Dialog.OpenFile().
		SetMessage(l10n.T("导入设置")).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if _, err := s.cfg.Import(data); err != nil {
		var unknown *json.UnmarshalTypeError
		if errors.As(err, &unknown) {
			return "", fmt.Errorf("这个文件不像 Transdict 的配置: %w", err)
		}
		return "", err
	}
	return path, nil
}

// SaveHistoryFile asks where the log should go and writes it there, returning
// the path chosen.
func (s *Service) SaveHistoryFile(needle string, favoritesOnly bool, service, format string) (string, error) {
	if s.history == nil {
		return "", errors.New("历史不可用")
	}
	filter := history.Filter{Needle: needle, Favorites: favoritesOnly, Service: service}
	if len(s.history.List(filter, 0)) == 0 {
		return "", errors.New("没有可导出的记录")
	}
	ext := exportExt(format)
	path, err := s.wails.Dialog.SaveFile().
		SetMessage(l10n.T("导出记录")).
		SetFilename("transdict-history." + ext).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	path = withExt(path, ext)
	if err := os.WriteFile(path, []byte(s.history.Export(filter, format)), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func exportExt(format string) string {
	switch format {
	case history.FormatJSON:
		return "json"
	case history.FormatCSV:
		return "csv"
	default:
		return "md"
	}
}

// withExt keeps the extension the format asked for: a panel that lets the name
// be typed without one would otherwise produce a file nothing recognises.
func withExt(path, ext string) string {
	if filepath.Ext(path) == "" {
		return path + "." + ext
	}
	return path
}
