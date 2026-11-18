//go:build windows

package gui

import (
	"errors"
	"path/filepath"
	"strings"

	"transdict/internal/settings"
)

// blockedAppPattern is what the picker's file dialog filters on.
const blockedAppPattern = "*.exe"

// readAppBundle answers the row 禁止名单 stores for a Windows program: the
// executable name, which is what the watcher compares against, and the file
// name without its extension for the list to draw.
func readAppBundle(path string) (settings.BlockedApp, error) {
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return settings.BlockedApp{}, errors.New("请选择一个 .exe 程序")
	}
	base := filepath.Base(path)
	return settings.BlockedApp{
		BundleID: strings.ToLower(base),
		Name:     strings.TrimSuffix(base, filepath.Ext(base)),
	}, nil
}
