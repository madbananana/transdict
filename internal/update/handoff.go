package update

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Launch hands the downloaded package to the system. It reports whether the
// running app must quit: on Windows the installer replaces the executable, so
// the app has to leave first; on macOS the disk image and on Linux the package
// are opened beside the running app.
func Launch(path string) (quit bool, err error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command(path)
		quit = true
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return false, fmt.Errorf("当前系统不支持在线安装")
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("无法启动安装程序：%w", err)
	}
	// The child outlives this process on every platform, so it does not need to
	// be waited for; releasing only drops the handle.
	_ = cmd.Process.Release()
	return quit, nil
}

// LaunchMessage is what the settings page shows once the package was handed
// over, before the window (on Windows) disappears.
func LaunchMessage() string {
	switch runtime.GOOS {
	case "windows":
		return "安装程序已启动，应用即将退出；按提示完成安装即可。"
	case "darwin":
		return "安装镜像已打开，请把 Transdict 拖到「应用程序」替换旧版本，然后重新打开。"
	case "linux":
		return "安装包已下载并交给系统打开。"
	}
	return "安装包已下载。"
}
