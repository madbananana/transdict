//go:build !darwin && !windows

package netp

// systemProxy has nothing to report where the app is not built to run; the
// environment variables stay in charge.
func systemProxy() string { return "" }
