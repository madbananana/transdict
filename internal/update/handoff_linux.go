//go:build linux

package update

import (
	"fmt"
	"os/exec"
)

// Launch hands the package to the desktop. Linux packages belong to the
// distribution's installer, so unlike Windows and macOS there is no safe
// in-place replacement to perform here.
func Launch(path string) (bool, string, error) {
	if err := exec.Command("xdg-open", path).Run(); err != nil {
		return false, "", fmt.Errorf("无法打开安装包：%w", err)
	}
	return false, "安装包已下载并交给系统打开。", nil
}
