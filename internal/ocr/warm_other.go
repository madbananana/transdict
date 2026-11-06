//go:build !darwin

package ocr

// Warm is a no-op where the recognizer is not on-device: the network services
// have no model to load.
func Warm() {}
