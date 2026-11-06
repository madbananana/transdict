//go:build !darwin

package dock

// Windows has no Dock: a taskbar button belongs to a window, not to the
// process, so the switch lives in the window options instead (see
// application.WindowsWindow.HiddenOnTaskbar) and is read when a window is
// built. Nothing changes for an app that is already running.
func Set(bool) {}

// Activate is a no-op. Every Windows app is a foreground app, so focusing a
// window already brings the keyboard with it.
func Activate() {}
