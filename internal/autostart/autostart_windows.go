//go:build windows

package autostart

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	procRegSetKey    = advapi32.NewProc("RegSetKeyValueW")
	procRegDeleteKey = advapi32.NewProc("RegDeleteKeyValueW")
	procRegGetValue  = advapi32.NewProc("RegGetValueW")
)

const (
	hkeyCurrentUser   = uintptr(0x80000001)
	runKey            = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName         = "Transdict"
	regExpandSZ       = 2
	rrfRTRegString    = 0x2 | 0x4
	errorSuccess      = 0
	errorFileNotFound = 2
	errorNoMoreItems  = 259
)

func utf16Ptr(s string) uintptr {
	return uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(s)))
}

// Set writes or clears the HKCU Run entry. The path is quoted because Explorer
// executes the value as a command line, and "Program Files" contains a space.
func Set(enabled bool) error {
	if !enabled {
		r, _, callErr := procRegDeleteKey.Call(hkeyCurrentUser, utf16Ptr(runKey), utf16Ptr(valueName))
		switch int(r) {
		case errorSuccess, errorFileNotFound, errorNoMoreItems:
			return nil
		}
		return registryError("删除开机自启失败", callErr)
	}

	path, err := os.Executable()
	if err != nil {
		return err
	}
	command := syscall.StringToUTF16(`"` + path + `"`)
	r, _, callErr := procRegSetKey.Call(
		hkeyCurrentUser,
		utf16Ptr(runKey),
		utf16Ptr(valueName),
		uintptr(regExpandSZ),
		uintptr(unsafe.Pointer(&command[0])),
		uintptr(len(command)*2),
	)
	if int(r) != errorSuccess {
		return registryError("写入开机自启失败", callErr)
	}
	return nil
}

func Status() State {
	var size uint32
	r, _, _ := procRegGetValue.Call(
		hkeyCurrentUser,
		utf16Ptr(runKey),
		utf16Ptr(valueName),
		uintptr(rrfRTRegString),
		0,
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	switch int(r) {
	case errorSuccess:
		return Enabled
	case errorFileNotFound, errorNoMoreItems:
		return Disabled
	}
	return Unknown
}

func registryError(prefix string, callErr error) error {
	if callErr != nil && callErr != syscall.Errno(0) {
		return errors.New(prefix + ": " + callErr.Error())
	}
	return errors.New(prefix)
}
