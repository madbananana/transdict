//go:build !darwin && !windows

package autostart

// Linux is not a shipping target yet, and every route there (autostart .desktop,
// systemd user unit, display manager) is a different convention. Declining is
// honest; guessing is not.
func Set(bool) error { return ErrUnsupported }

func Status() State { return Unknown }
