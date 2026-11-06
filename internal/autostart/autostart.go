// Package autostart keeps the app present after a reboot, which is the whole
// point of a hotkey-driven translation tool: the shortcut has to exist in the
// session nobody prepared for.
//
// The operating system, not the config file, decides whether a login item is
// real, so callers consult Status and persist what Reconcile returns.
package autostart

import "errors"

// ErrUnsupported means this platform offers no login item we can manage.
var ErrUnsupported = errors.New("开机自启：当前系统不支持")

// State is what the operating system reports about this app's login item.
type State int

const (
	// Unknown means the system would not say: an unsupported platform, or a
	// build running outside an application bundle where login items are not a
	// concept yet.
	Unknown State = iota
	Enabled
	Disabled
)

// Reconcile decides what the stored flag should say once the system has been
// consulted. Under Unknown the stored value stands: overwriting it would erase a
// user's choice on a machine that simply cannot answer.
func Reconcile(stored bool, state State) (next bool, changed bool) {
	switch state {
	case Enabled:
		return true, !stored
	case Disabled:
		return false, stored
	}
	return stored, false
}
