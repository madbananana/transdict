//go:build !darwin

package ocr

// Apple Vision is darwin-only, so other platforms do not register the apple
// kind at all: it stays out of the settings UI and the query planner instead of
// shipping a provider that would always error out. Windows brings its own
// engine (`Windows.Media.Ocr`) through winmedia_windows.go.
//
// The darwin build lives in apple_darwin.go behind //go:build darwin; the
// cross-compile target GOOS=windows therefore excludes the cgo file
// automatically without a stub symbol to satisfy.
