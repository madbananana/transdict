//go:build !darwin

package gui

// The 划词查询图标 only watches the pointer through a macOS event tap. Elsewhere
// there is none, so the caller's lifetime timer is the only dismissal route and
// these hooks stay empty.

func startIconTap(*Service) bool { return false }
func stopIconTap()               {}
