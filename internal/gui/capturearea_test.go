package gui

import (
	"os"
	"path/filepath"
	"testing"
)

// 上一次截图区域 is machine state beside config.json, so the round trip is the
// whole contract: what the last commit wrote is what the next overlay reads.
func TestCaptureAreaRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-capture.json")
	want := CaptureArea{Frame: 1, X: 20, Y: 40, W: 300, H: 120}
	writeArea(path, want)
	got, ok := readArea(path)
	if !ok || got != want {
		t.Fatalf("read %+v, %v; want %+v", got, ok, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("权限为 %o，应为 600", perm)
	}
}

func TestReadAreaRefusesMissingOrBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	if _, ok := readArea(filepath.Join(dir, "absent.json")); ok {
		t.Fatal("缺少文件时不应返回区域")
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readArea(broken); ok {
		t.Fatal("无法解析的文件不应返回区域")
	}

	// A zero-width rectangle cannot be cropped; it is the same as no record.
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"frame":0,"x":1,"y":2,"w":0,"h":9}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readArea(empty); ok {
		t.Fatal("宽高为零时不应返回区域")
	}
}

// The rectangle is stored in one display's pixels, so D must not offer it on
// another display's frozen frame.
func TestPickAreaIgnoresOtherDisplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-capture.json")
	writeArea(path, CaptureArea{Frame: 1, X: 0, Y: 0, W: 10, H: 10})

	got := pickArea(path, 1)
	if got == nil || got.W != 10 {
		t.Fatalf("本显示器应拿到区域，得到 %+v", got)
	}
	if got := pickArea(path, 0); got != nil {
		t.Fatalf("另一显示器的冻结画面不应复用该区域，得到 %+v", got)
	}
}
