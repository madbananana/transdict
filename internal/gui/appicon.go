package gui

import "sync"

// appIcons keeps the data URLs handed to 禁止名单. The list is short and icon
// PNGs are a few KB each, so one cache per process is enough; a failed lookup
// is cached as empty so the page does not retry on every render.
var appIcons sync.Map

// AppIcon answers a data: URL for the app's Finder icon, which is what
// EasyDict's DisabledAppTab draws beside each name. An unknown bundle or a
// non-macOS build answers empty and the page falls back to a letter tile.
func (s *Service) AppIcon(bundleID string) string {
	if bundleID == "" {
		return ""
	}
	if v, ok := appIcons.Load(bundleID); ok {
		return v.(string)
	}
	url := appIconData(bundleID)
	appIcons.Store(bundleID, url)
	return url
}
