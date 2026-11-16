//go:build !darwin

package gui

// appIconData has no icon to read outside macOS; the list still works.
func appIconData(string) string { return "" }
