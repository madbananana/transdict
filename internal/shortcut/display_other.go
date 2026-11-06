//go:build !darwin

package shortcut

import "strings"

// Display renders an accelerator the way Windows and Linux show it.
func Display(accel string) string {
	parts := strings.Split(strings.ToLower(accel), "+")
	for i, p := range parts {
		switch strings.TrimSpace(p) {
		case "cmd":
			parts[i] = "Ctrl"
		case "alt":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		case "ctrl":
			parts[i] = "Ctrl"
		case "super":
			parts[i] = "Win"
		default:
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "+")
}
