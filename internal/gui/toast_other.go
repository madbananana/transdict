//go:build !darwin

package gui

// ShowToast and ShowToastAtMouse have no counterpart here: every caller is a
// macOS-only action, and Windows would want a native notification instead of an
// imitation of one.
func ShowToast(string)        {}
func ShowToastAtMouse(string) {}
func ShowSuccessToast()       {}
