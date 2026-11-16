//go:build !darwin

package gui

// Only macOS swallows the first click on an inactive window; elsewhere the
// window manager delivers it and the button works on the first press.
func acceptFirstMouseInViews() {}
