package gui

import (
	"errors"

	"transdict/internal/capture"
	"transdict/internal/selection"
)

// PermissionState is the two macOS grants the capture routes depend on. The
// settings page shows them so a button that cannot work says why, instead of
// failing the next time the user presses it.
type PermissionState struct {
	ScreenRecording bool `json:"screenRecording"`
	Accessibility   bool `json:"accessibility"`
}

// Permissions reports the current grants. Other platforms answer true, so the
// row never reports a problem that cannot exist there.
func (s *Service) Permissions() PermissionState {
	return PermissionState{
		ScreenRecording: capture.ScreenAccess(),
		Accessibility:   selection.Trusted(),
	}
}

// OpenPermission opens the system pane that holds one of the checkboxes. The
// app cannot flip it: macOS only honours a change after a relaunch, which is
// why the page says so beside the button.
func (s *Service) OpenPermission(name string) error {
	pane := accessibilityPane
	if name == "screen" {
		pane = screenPane
	}
	if pane == "" {
		return errors.New("当前平台没有该权限设置")
	}
	return s.wails.Browser.OpenURL(pane)
}
