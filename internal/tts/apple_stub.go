//go:build !darwin

package tts

// On non-macOS builds the "apple" kind is intentionally absent: `say` and the
// built-in voices are macOS-only, so no registration happens here and the kind
// simply never appears in the Windows service list.
