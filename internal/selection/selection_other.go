//go:build !darwin && !windows

package selection

// Neither an accessibility read nor a verified input synthesis exists on this
// platform, so every entry point declines and Grab reports ErrUnsupported.

func Trusted() bool                { return false }
func RequestTrust() bool           { return false }
func SelectedText() (string, bool) { return "", false }

func Copy() error  { return ErrUnsupported }
func Paste() error { return ErrUnsupported }

// ChangeCount has no portable equivalent; -1 tells Grab to compare text instead.
func ChangeCount() int64 { return -1 }

const Pollable = false

func FrontmostIsSelf() bool { return false }
