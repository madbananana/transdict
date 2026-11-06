//go:build !darwin

package gui

// Where the screen and the keyboard are handed to every process, a missing
// grant is not a thing that happens, so there is nothing to point the user at.
const (
	screenPane        = ""
	accessibilityPane = ""
)

func (s *Service) offerPermission(title, body, pane string) {}

// Restart exists for the settings page's button; elsewhere a grant change does
// not need a relaunch, so there is nothing to do.
func (s *Service) Restart() {}
