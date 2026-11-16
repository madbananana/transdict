//go:build !darwin

package selection

import "context"

// WatchPointer needs a global event tap, which only the macOS build installs.
// Every other platform keeps the polling route, so this reports that it cannot
// watch and lets the caller fall back to Watch.
func WatchPointer(context.Context, func(string)) error { return ErrUnsupported }
