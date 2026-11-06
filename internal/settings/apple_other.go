//go:build !darwin

package settings

// addAppleServices does nothing off macOS. The ocr, dict and tts packages leave
// their "apple" kinds out of these builds on purpose, so a default row naming
// one would be a service the settings page offers, the plan can never run, and
// the user has no way to switch on.
func addAppleServices(*Config) {}
