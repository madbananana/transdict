//go:build darwin

package shortcut

import "strings"

var glyphs = map[string]string{
	"cmd": "⌘", "alt": "⌥", "option": "⌥", "shift": "⇧", "ctrl": "⌃", "super": "",
}

// Display renders an accelerator the way macOS shows it.
func Display(accel string) string {
	parts := strings.Split(strings.ToLower(accel), "+")
	for i, p := range parts {
		g, ok := glyphs[strings.TrimSpace(p)]
		if !ok {
			return strings.ToUpper(accel)
		}
		parts[i] = g
	}
	return strings.Join(parts, "")
}
