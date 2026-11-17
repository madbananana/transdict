package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestChooseAssetPicksThePackageForTheOS(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "transdict.dmg", URL: "https://example.test/a.dmg"},
		{Name: "transdict-amd64-installer.exe", URL: "https://example.test/a.exe"},
		{Name: "transdict.deb", URL: "https://example.test/a.deb"},
		{Name: "transdict-x86_64.AppImage", URL: "https://example.test/a.AppImage"},
		{Name: "notes.txt", URL: "https://example.test/notes.txt"},
	}}
	cases := []struct{ goos, goarch, want string }{
		{"windows", "amd64", "transdict-amd64-installer.exe"},
		{"darwin", "arm64", "transdict.dmg"},
		{"linux", "amd64", "transdict-x86_64.AppImage"},
	}
	for _, c := range cases {
		if got := ChooseAsset(rel, c.goos, c.goarch); got == nil || got.Name != c.want {
			t.Errorf("ChooseAsset(%s/%s) = %+v, want %s", c.goos, c.goarch, got, c.want)
		}
	}
	if got := ChooseAsset(rel, "plan9", "amd64"); got != nil {
		t.Errorf("unsupported OS = %+v, want nil", got)
	}
	if got := ChooseAsset(nil, "linux", "amd64"); got != nil {
		t.Errorf("nil release = %+v, want nil", got)
	}
	// No arm64 package is published, so the installer must not be offered.
	if got := ChooseAsset(rel, "windows", "arm64"); got != nil {
		t.Errorf("arm64 Windows = %+v, want nil", got)
	}
}

func TestDownloadStreamsAndChecksTheSize(t *testing.T) {
	body := []byte("installer-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "15")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "update.bin")
	wantDigest := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(wantDigest[:])
	var lastDone int64
	err := Download(context.Background(), Asset{Name: "update.bin", URL: srv.URL, Size: int64(len(body)), Digest: digest}, path,
		func(done, _ int64) { lastDone = done })
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("download = %q, want %q", got, body)
	}
	if lastDone != int64(len(body)) {
		t.Errorf("progress done = %d, want %d", lastDone, len(body))
	}

	// A truncated transfer must not leave a plausible-looking installer behind.
	err = Download(context.Background(), Asset{URL: srv.URL, Size: int64(len(body)) + 1}, path, nil)
	if err == nil {
		t.Fatal("size mismatch should fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("a rejected download should be removed")
	}

	// A digest mismatch is the case that matters: the bytes arrived intact by
	// size but are not the file the release published.
	bad := "sha256:" + hex.EncodeToString(make([]byte, 32))
	err = Download(context.Background(), Asset{URL: srv.URL, Size: int64(len(body)), Digest: bad}, path, nil)
	if err == nil {
		t.Fatal("a digest mismatch should fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("a digest-mismatch download should be removed")
	}
}
