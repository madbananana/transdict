//go:build darwin || linux

package instance

import (
	"errors"
	"os"
	"syscall"
)

// hold takes an exclusive, non-blocking flock. The descriptor stays open in
// Lock.file because flock belongs to the open file description: closing it —
// or exiting — is precisely what releases the lock.
func hold(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errLocked
		}
		return err
	}
	return nil
}

func release(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
