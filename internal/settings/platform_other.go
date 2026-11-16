//go:build !windows

package settings

// addPlatformServices is a no-op where the platform's own engines are added by
// addAppleServices or are not present at all.
func addPlatformServices(*Config) {}
