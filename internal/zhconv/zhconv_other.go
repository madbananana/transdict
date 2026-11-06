//go:build !darwin

// Package zhconv restates Chinese text in 繁体. Only macOS ships a conversion
// table; elsewhere the text is left as the service answered it.
package zhconv

// Traditional returns s unchanged: this platform has no built-in
// simplified-to-traditional table.
func Traditional(s string) string { return s }
