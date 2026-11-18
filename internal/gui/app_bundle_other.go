//go:build !darwin && !windows

package gui

import (
	"errors"

	"transdict/internal/settings"
)

// 禁止名单 gates the mouse watcher, which needs a platform read of the focused
// element; without one there is nothing to pick. The settings page still
// renders the stored list.
func readAppBundle(string) (settings.BlockedApp, error) {
	return settings.BlockedApp{}, errors.New("禁止名单只在 macOS 与 Windows 上生效")
}

// blockedAppPattern is what the picker's file dialog filters on.
const blockedAppPattern = "*"
