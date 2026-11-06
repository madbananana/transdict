//go:build windows

package instance

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procLockFile   = kernel32.NewProc("LockFileEx")
	procUnlockFile = kernel32.NewProc("UnlockFileEx")
)

const (
	lockExclusive       = 0x00000002
	lockFailImmediately = 0x00000001
	errorLockViolation  = 33
)

// hold takes an exclusive, non-blocking lock over the first byte of the file.
// Windows releases it when the handle closes, including when the process dies,
// which is the property the whole scheme rests on.
//
// kernel32 is reached through a lazy DLL because the syscall package does not
// export these two functions, and a dependency for a lock is not worth it.
func hold(f *os.File) error {
	var ov syscall.Overlapped
	r, _, err := procLockFile.Call(uintptr(syscall.Handle(f.Fd())),
		lockExclusive|lockFailImmediately, 0, 1, 0, uintptr(unsafe.Pointer(&ov)))
	if r != 0 {
		return nil
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorLockViolation {
		return errLocked
	}
	return err
}

func release(f *os.File) error {
	var ov syscall.Overlapped
	procUnlockFile.Call(uintptr(syscall.Handle(f.Fd())), 0, 1, 0,
		uintptr(unsafe.Pointer(&ov)))
	return nil
}
