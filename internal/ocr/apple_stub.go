//go:build !darwin

package ocr

// Apple Vision is darwin-only: on Windows and Linux we intentionally register
// nothing, so the kind simply does not appear in the settings UI or the query
// planner. That beats shipping a provider that would always error out.
//
// The darwin build lives in apple_darwin.go behind //go:build darwin; the
// cross-compile target GOOS=windows therefore excludes the cgo file
// automatically without a stub symbol to satisfy.
