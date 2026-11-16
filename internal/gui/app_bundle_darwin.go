package gui

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"transdict/internal/settings"
)

// readAppBundle turns a path the 禁止名单 panel returned into the row the
// settings store keeps. A panel can hand back a file inside the bundle, so the
// nearest .app ancestor is the app the user actually chose.
func readAppBundle(path string) (settings.BlockedApp, error) {
	for p := filepath.Clean(path); p != "/" && p != "."; p = filepath.Dir(p) {
		if strings.HasSuffix(p, ".app") {
			return appBundleInfo(p)
		}
	}
	return settings.BlockedApp{}, errors.New("请选择一个 .app 应用")
}

// appBundleInfo reads CFBundleIdentifier with plutil, which understands both
// XML and binary Info.plist files. shelling out keeps the picker free of an
// Objective-C bridge for one string.
func appBundleInfo(bundle string) (settings.BlockedApp, error) {
	out, err := exec.Command("plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-",
		filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil {
		return settings.BlockedApp{}, fmt.Errorf("读取应用标识失败: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return settings.BlockedApp{}, errors.New("应用没有 Bundle ID")
	}
	return settings.BlockedApp{
		BundleID: id,
		Name:     strings.TrimSuffix(filepath.Base(bundle), ".app"),
	}, nil
}
