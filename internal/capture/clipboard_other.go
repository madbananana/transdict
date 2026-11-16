//go:build !darwin && !windows

package capture

import "errors"

// ClipboardImage has no portable implementation yet: reading a picture off the
// pasteboard needs the platform clipboard API, and only macOS and Windows have
// one wired up.
func ClipboardImage() ([]byte, error) {
	return nil, errors.New("剪贴板里没有图片")
}
