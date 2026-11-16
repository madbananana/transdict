package gui

import (
	"image"
	"testing"
)

func TestIconInReach(t *testing.T) {
	rect := image.Rect(100, 200, 140, 240) // centre 120,220
	cases := []struct {
		name string
		x, y float64
		want bool
	}{
		{"on the button", 110, 210, true},
		{"just inside the reach", 120 + queryIconReach - 1, 220, true},
		{"at the reach", 120 + queryIconReach, 220, true},
		{"just past it", 120 + queryIconReach + 1, 220, false},
		{"diagonally inside", 120 + 80, 220 + 80, true},
		{"diagonally outside", 120 + 90, 220 + 90, false},
	}
	for _, c := range cases {
		if got := iconInReach(rect, c.x, c.y); got != c.want {
			t.Errorf("%s: iconInReach(%v, %v) = %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
}

func TestIconKeyActionFor(t *testing.T) {
	cases := []struct {
		name    string
		keyCode int
		flags   uint64
		want    iconKeyAction
	}{
		{"plain letter waits a beat", 0, 0, iconKeyDismissSoon},
		{"command chord dismisses at once", 15, flagCommand, iconKeyDismiss}, // ⌘R
		{"shift chord dismisses at once", 15, flagShift, iconKeyDismiss},
		{"⌘C keeps the button", keyCodeC, flagCommand, iconKeyKeep},
		{"⌘⇧C keeps it too", keyCodeC, flagCommand | flagShift, iconKeyKeep},
		{"lone command says nothing", 55, flagCommand, iconKeyKeep},
		{"lone shift says nothing", 56, flagShift, iconKeyKeep},
		{"shift + letter dismisses", 0, flagShift, iconKeyDismiss},
	}
	for _, c := range cases {
		if got := iconKeyActionFor(c.keyCode, c.flags); got != c.want {
			t.Errorf("%s: iconKeyActionFor(%d, %d) = %v, want %v", c.name, c.keyCode, c.flags, got, c.want)
		}
	}
}
