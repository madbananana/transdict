//go:build darwin

package update

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Launch replaces the running application with the copy inside the downloaded
// disk image, then reopens it. The swap waits for this process to exit: a
// bundle that is still open cannot be removed. If the folder is not writable or
// the build is not running from an app bundle, the image is opened in Finder so
// the update is still one drag away.
func Launch(path string) (bool, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return openImage(path, "无法定位当前程序，已打开安装镜像。")
	}
	target := appBundle(exe)
	if target == "" {
		return openImage(path, "当前不是从应用包运行，已打开安装镜像；请手动安装新版本。")
	}
	staged, err := stageFromImage(path, target)
	if err != nil {
		log.Printf("transdict: 自动替换应用失败: %v", err)
		return openImage(path, "无法自动替换应用，已打开安装镜像；请把 Transdict 拖到「应用程序」覆盖旧版本。")
	}
	if err := startReplacer(target, staged); err != nil {
		_ = os.RemoveAll(staged)
		return openImage(path, "无法启动自动安装，已打开安装镜像；请手动拖到「应用程序」覆盖旧版本。")
	}
	return true, "新版本已准备好，应用即将退出并自动重启。", nil
}

// appBundle walks up from Contents/MacOS/<binary> to the .app directory.
func appBundle(exe string) string {
	dir := exe
	for range 3 {
		dir = filepath.Dir(dir)
	}
	if strings.HasSuffix(strings.ToLower(dir), ".app") {
		return dir
	}
	return ""
}

// stageFromImage copies the .app out of the image and next to the running one,
// so the final swap is a rename on the same volume rather than a cross-volume
// copy that can leave a half-installed bundle behind.
func stageFromImage(dmg, target string) (string, error) {
	mount, err := os.MkdirTemp("", "transdict-dmg-")
	if err != nil {
		return "", fmt.Errorf("无法创建挂载点：%w", err)
	}
	defer os.RemoveAll(mount)

	attach := exec.Command("/usr/bin/hdiutil", "attach", "-nobrowse", "-noautoopen", "-readonly", "-mountpoint", mount, dmg)
	if out, err := attach.CombinedOutput(); err != nil {
		return "", fmt.Errorf("打开镜像失败：%s", firstLine(out, err))
	}
	defer exec.Command("/usr/bin/hdiutil", "detach", mount, "-quiet").Run()

	src, err := appInImage(mount, target)
	if err != nil {
		return "", err
	}
	staged := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".update")
	_ = os.RemoveAll(staged)
	if out, err := exec.Command("/usr/bin/ditto", src, staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(staged)
		return "", fmt.Errorf("复制应用失败：%s", firstLine(out, err))
	}
	signStaged(staged)
	return staged, nil
}

// signStaged gives the downloaded bundle this machine's code identity. macOS
// ties Screen Recording and Accessibility to that identity, so replacing an
// adhoc build with another adhoc build loses both grants on every update. A
// Developer ID build from CI is already stable and is left alone; a machine
// with no signing identity keeps the download as it is.
func signStaged(app string) {
	if strings.Contains(codeSignature(app), "Authority=Developer ID Application") {
		return
	}
	identity := signingIdentity()
	if identity == "" {
		log.Printf("transdict: 没有可用的签名证书，更新后需要在系统设置里重新授权")
		return
	}
	out, err := exec.Command("/usr/bin/codesign", "--force", "--deep", "--sign", identity,
		"--identifier", "app.transdict.desktop", app).CombinedOutput()
	if err != nil {
		log.Printf("transdict: 更新包签名失败，更新后可能需要重新授权: %s", firstLine(out, err))
		return
	}
	log.Printf("transdict: 已用 %q 签名更新包，以保留系统授权", identity)
}

func codeSignature(app string) string {
	out, _ := exec.Command("/usr/bin/codesign", "-dvvv", app).CombinedOutput()
	return string(out)
}

// signingIdentity prefers the identity a release would ship with, then the
// development certificate Xcode installs. The first of either is enough to keep
// the designated requirement stable across updates.
func signingIdentity() string {
	out, err := exec.Command("/usr/bin/security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return ""
	}
	var development, developerID string
	for _, line := range strings.Split(string(out), "\n") {
		start := strings.IndexByte(line, '"')
		end := strings.LastIndexByte(line, '"')
		if start < 0 || end <= start {
			continue
		}
		name := line[start+1 : end]
		switch {
		case strings.HasPrefix(name, "Developer ID Application:"):
			if developerID == "" {
				developerID = name
			}
		case strings.HasPrefix(name, "Apple Development:"):
			if development == "" {
				development = name
			}
		}
	}
	if developerID != "" {
		return developerID
	}
	return development
}

func appInImage(mount, target string) (string, error) {
	entries, err := os.ReadDir(mount)
	if err != nil {
		return "", fmt.Errorf("无法读取镜像：%w", err)
	}
	want := strings.ToLower(filepath.Base(target))
	first := ""
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
			continue
		}
		if strings.ToLower(entry.Name()) == want {
			return filepath.Join(mount, entry.Name()), nil
		}
		if first == "" {
			first = filepath.Join(mount, entry.Name())
		}
	}
	if first == "" {
		return "", errors.New("镜像中没有找到应用")
	}
	return first, nil
}

// startReplacer leaves a detached shell waiting on this process. It moves the
// old bundle aside, drops the staged one in its place, opens it, and only then
// deletes the backup; a failed swap puts the old bundle back.
func startReplacer(target, staged string) error {
	script := fmt.Sprintf(`#!/bin/sh
while kill -0 %d 2>/dev/null; do sleep 0.2; done
target="$1"
staged="$2"
old="${target}.old"
rm -rf "$old"
mv "$target" "$old" || { rm -rf "$staged"; exit 1; }
if mv "$staged" "$target"; then
  /usr/bin/open "$target"
  sleep 2
  rm -rf "$old"
else
  mv "$old" "$target"
fi
`, os.Getpid())
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("transdict-update-%d.sh", os.Getpid()))
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", scriptPath, target, staged)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func openImage(path, message string) (bool, string, error) {
	if err := exec.Command("/usr/bin/open", path).Run(); err != nil {
		return false, "", fmt.Errorf("无法打开安装镜像：%w", err)
	}
	return false, message, nil
}

func firstLine(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if text == "" {
		text = err.Error()
	}
	return text
}
