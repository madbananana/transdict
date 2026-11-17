package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ChooseAsset picks the file this build can install. The release carries one
// asset per packaging format rather than one per OS, so the choice is by name:
// the installer on Windows, the disk image on macOS, and the portable AppImage
// before the Debian package on Linux because it works on every distribution.
// An architecture with no asset answers nil rather than handing a package the
// CPU cannot run to the installer.
func ChooseAsset(rel *Release, goos, goarch string) *Asset {
	if rel == nil {
		return nil
	}
	var suffixes []string
	switch goos {
	case "windows":
		suffixes = []string{".exe"}
	case "darwin":
		suffixes = []string{".dmg"}
	case "linux":
		suffixes = []string{".appimage", ".deb"}
	default:
		return nil
	}
	for _, suffix := range suffixes {
		for i := range rel.Assets {
			a := &rel.Assets[i]
			name := strings.ToLower(a.Name)
			if !strings.HasSuffix(name, suffix) {
				continue
			}
			// Windows also publishes the bare app next to the installer in some
			// builds; the installer is the one that can replace an install.
			if suffix == ".exe" && !strings.Contains(name, "installer") {
				continue
			}
			if !archFits(name, goarch) {
				continue
			}
			return a
		}
	}
	return nil
}

// archFits mirrors the architecture the filename advertises. A name that says
// nothing about it is accepted, which is how the universal macOS image and a
// distribution-independent .deb are published.
func archFits(name, goarch string) bool {
	stated := ""
	switch {
	case strings.Contains(name, "amd64"), strings.Contains(name, "x86_64"), strings.Contains(name, "x64"):
		stated = "amd64"
	case strings.Contains(name, "arm64"), strings.Contains(name, "aarch64"):
		stated = "arm64"
	}
	return stated == "" || stated == goarch
}

// Download fetches an asset to path. The progress callback is called while the
// bytes arrive, at most a few times a second; total is the release's advertised
// size when the server does not send a length.
func Download(ctx context.Context, asset Asset, path string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return fmt.Errorf("更新包地址无效：%w", err)
	}
	req.Header.Set("User-Agent", "transdict-updater")
	// No client timeout: the context bounds the whole download, and a slow
	// connection in a home network is still a working one. The shared transport
	// keeps the proxy settings.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载更新失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败：服务器返回 %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = asset.Size
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("无法写入更新包：%w", err)
	}
	sum := sha256.New()
	writer := &progressWriter{w: file, sum: sum, total: total, progress: progress}
	_, copyErr := io.Copy(writer, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(path)
		return fmt.Errorf("下载更新中断：%w", copyErr)
	}
	if closeErr != nil {
		os.Remove(path)
		return fmt.Errorf("无法写入更新包：%w", closeErr)
	}
	if asset.Size > 0 && writer.done != asset.Size {
		os.Remove(path)
		return fmt.Errorf("更新包大小不正确：收到 %d 字节，应为 %d", writer.done, asset.Size)
	}
	if err := verifyDigest(asset.Digest, sum); err != nil {
		os.Remove(path)
		return err
	}
	if progress != nil {
		progress(writer.done, max(writer.done, total))
	}
	return nil
}

// verifyDigest compares the download against GitHub's sha256:… entry. A source
// without a digest is trusted on size alone, which is what a custom flat
// manifest has always offered.
func verifyDigest(digest string, sum hash.Hash) error {
	want := strings.TrimSpace(digest)
	if want == "" {
		return nil
	}
	algorithm, value, ok := strings.Cut(want, ":")
	if !ok || !strings.EqualFold(algorithm, "sha256") {
		return nil
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(value)) {
		return fmt.Errorf("更新包校验失败：SHA-256 不匹配")
	}
	return nil
}

// progressWriter reports through a callback and throttles it so a fast link
// does not flood the window with one event per buffer. The same bytes feed the
// SHA-256 so verification costs no second read.
type progressWriter struct {
	w        io.Writer
	sum      hash.Hash
	total    int64
	done     int64
	last     time.Time
	progress func(done, total int64)
}

func (p *progressWriter) Write(buf []byte) (int, error) {
	n, err := p.w.Write(buf)
	if n > 0 && p.sum != nil {
		_, _ = p.sum.Write(buf[:n])
	}
	p.done += int64(n)
	if p.progress != nil && (time.Since(p.last) > 150*time.Millisecond || (p.total > 0 && p.done >= p.total)) {
		p.last = time.Now()
		p.progress(p.done, p.total)
	}
	return n, err
}

// SaveDir is where downloads are staged. Old files are removed best-effort: a
// mounted disk image or an installer the user left open must not block the next
// update before it has even started.
func SaveDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("无法定位更新缓存目录：%w", err)
	}
	dir := filepath.Join(root, "transdict", "updates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建更新缓存目录：%w", err)
	}
	if old, err := os.ReadDir(dir); err == nil {
		for _, entry := range old {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return dir, nil
}
