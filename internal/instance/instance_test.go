package instance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireIsExclusive(t *testing.T) {
	dir := tempDir(t)
	first, err := acquire(dir)
	if err != nil {
		t.Fatalf("首次获取锁失败: %v", err)
	}
	defer first.Close()

	if _, err := acquire(dir); !errors.Is(err, ErrBusy) {
		t.Errorf("第二次获取锁 = %v, 期望 ErrBusy", err)
	}

	first.Close()
	second, err := acquire(dir)
	if err != nil {
		t.Fatalf("释放后重新获取锁失败: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Errorf("释放锁失败: %v", err)
	}
}

func TestNotifyReachesTheHolder(t *testing.T) {
	dir := tempDir(t)
	lock, err := acquire(dir)
	if err != nil {
		t.Fatalf("获取锁失败: %v", err)
	}
	defer lock.Close()

	if err := notify(dir, "settings"); err != nil {
		t.Fatalf("唤醒失败: %v", err)
	}
	select {
	case got := <-lock.Commands():
		if got != "settings" {
			t.Errorf("收到命令 %q, 期望 settings", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("唤醒请求没有送达")
	}
}

// A launch that cannot reach the primary still has to be told so, rather than
// hanging on a socket nobody will answer.
func TestNotifyWithoutAPrimary(t *testing.T) {
	dir := tempDir(t)
	if err := notify(dir, "main"); err == nil {
		t.Error("无人监听时唤醒竟然成功了")
	}

	// A leftover file from a crashed primary must not look like a live peer.
	if err := os.WriteFile(filepath.Join(dir, sockName), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := acquire(dir)
	if err != nil {
		t.Fatalf("残留的 socket 文件挡住了获取锁: %v", err)
	}
	defer lock.Close()
	if err := notify(dir, "input"); err != nil {
		t.Errorf("接管残留 socket 后唤醒失败: %v", err)
	}
}

func TestEmptyCommandDefaultsToMain(t *testing.T) {
	dir := tempDir(t)
	lock, err := acquire(dir)
	if err != nil {
		t.Fatalf("获取锁失败: %v", err)
	}
	defer lock.Close()

	if err := notify(dir, ""); err != nil {
		t.Fatalf("唤醒失败: %v", err)
	}
	if got := <-lock.Commands(); got != "main" {
		t.Errorf("空命令解析为 %q, 期望 main", got)
	}
}

// macOS caps a unix socket path at 104 bytes and t.TempDir is already close to
// that, so these tests root their directories somewhere short.
func tempDir(t *testing.T) string {
	t.Helper()
	base := "/tmp"
	if _, err := os.Stat(base); err != nil {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "transdict-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
