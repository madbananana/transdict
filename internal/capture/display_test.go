package capture

import (
	"bytes"
	"image"
	"image/png"
	"math/rand"
	"testing"
)

// noisePNG is an image of random pixels, which is the case PNG and JPEG
// disagree about most: the point of the preview is that this case gets much
// smaller.
func noisePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(7))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rnd.Read(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG: %v", err)
	}
	return buf.Bytes()
}

func TestForDisplayKeepsSizeAndShrinks(t *testing.T) {
	orig := noisePNG(t, 320, 200)
	preview, err := ForDisplay(orig)
	if err != nil {
		t.Fatalf("ForDisplay: %v", err)
	}
	if !bytes.HasPrefix(preview, []byte{0xFF, 0xD8}) {
		t.Fatal("预览不是 JPEG")
	}
	img, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("预览无法解码: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 200 {
		t.Errorf("预览尺寸 = %dx%d, want 320x200（坐标换算依赖原尺寸）", b.Dx(), b.Dy())
	}
	if len(preview) >= len(orig) {
		t.Errorf("预览 %d 字节未小于原图 %d 字节", len(preview), len(orig))
	}
}

func TestPreviewIsCachedPerFrame(t *testing.T) {
	frames := []Frame{{Index: 0, PNG: noisePNG(t, 80, 60)}}
	s := NewStore()
	id := s.NewSession(frames)

	first, ok := s.Preview(id, 0)
	if !ok {
		t.Fatal("Preview 未找到第 0 帧")
	}
	second, _ := s.Preview(id, 0)
	if !bytes.Equal(first, second) {
		t.Error("同一帧两次预览内容不同，应命中缓存")
	}
	if _, ok := s.Preview(id, 9); ok {
		t.Error("不存在的帧不应有预览")
	}
}
