//go:build !darwin

package gui

import (
	"errors"

	"transdict/internal/settings"
)

// 禁止名单 only gates the macOS mouse watcher, so there is no bundle to read
// elsewhere. The settings page still renders the stored list.
func readAppBundle(string) (settings.BlockedApp, error) {
	return settings.BlockedApp{}, errors.New("禁止名单只在 macOS 上生效")
}
