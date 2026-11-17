package ocr

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestUpscaleForOCREnlargesASingleLine(t *testing.T) {
	out, scale, err := upscaleForOCR(encodeTestPNG(t, 300, 24))
	if err != nil {
		t.Fatalf("upscaleForOCR: %v", err)
	}
	if scale != 4 {
		t.Fatalf("scale = %d, want 4", scale)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width != 1200 || cfg.Height != 96 {
		t.Fatalf("size = %dx%d, want 1200x96", cfg.Width, cfg.Height)
	}
}

func TestUpscaleForOCRLeavesALargePictureAlone(t *testing.T) {
	in := encodeTestPNG(t, 800, 300)
	out, scale, err := upscaleForOCR(in)
	if err != nil {
		t.Fatalf("upscaleForOCR: %v", err)
	}
	if scale != 1 {
		t.Fatalf("scale = %d, want 1", scale)
	}
	if !bytes.Equal(in, out) {
		t.Fatal("a picture that is already large enough was re-encoded")
	}
}

func TestUpscaleForOCRStopsAtTheScaleLimit(t *testing.T) {
	out, scale, err := upscaleForOCR(encodeTestPNG(t, 20, 10))
	if err != nil {
		t.Fatalf("upscaleForOCR: %v", err)
	}
	if scale != 4 {
		t.Fatalf("scale = %d, want 4", scale)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width != 80 || cfg.Height != 40 {
		t.Fatalf("size = %dx%d, want 80x40", cfg.Width, cfg.Height)
	}
}

func TestUpscaleForOCRRejectsBrokenPNG(t *testing.T) {
	if _, _, err := upscaleForOCR([]byte("not a png")); err == nil {
		t.Fatal("want an error")
	}
}

func TestShrinkBlocksMapsBackToTheCrop(t *testing.T) {
	blocks := []Block{{Text: "one line", X: 40, Y: 168, Width: 800, Height: 80}}
	shrinkBlocks(blocks, 4)
	if blocks[0].X != 10 || blocks[0].Y != 42 || blocks[0].Width != 200 || blocks[0].Height != 20 {
		t.Fatalf("block = %+v", blocks[0])
	}
}
