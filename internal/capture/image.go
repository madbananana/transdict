package capture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"github.com/kbinani/screenshot"
)

func captureViaLib(idx int) (Frame, error) {
	rect := screenshot.GetDisplayBounds(idx)
	img, err := screenshot.CaptureRect(rect)
	if err != nil {
		return Frame{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Frame{}, err
	}
	return frameFromPNG(idx, buf.Bytes())
}

func frameFromPNG(idx int, data []byte) (Frame, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Frame{}, fmt.Errorf("capture: decode display %d: %w", idx, err)
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		return Frame{}, errors.New("capture: empty display image")
	}
	return Frame{Index: idx, PNG: data, Width: cfg.Width, Height: cfg.Height}, nil
}

// MaxEdge keeps OCR payloads bounded; a full Retina display is ~11 MB of PNG.
const MaxEdge = 2400

// Crop extracts a sub-rectangle in image pixel coordinates and re-encodes it.
func Crop(pngBytes []byte, x, y, w, h int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	if w <= 0 || h <= 0 {
		return nil, errors.New("capture: empty selection")
	}
	x, y = clamp(x, 0, b.Dx()-1), clamp(y, 0, b.Dy()-1)
	w = clamp(w, 1, b.Dx()-x)
	h = clamp(h, 1, b.Dy()-y)

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), src, image.Pt(b.Min.X+x, b.Min.Y+y), draw.Src)

	if scale := float64(MaxEdge) / float64(max(w, h)); scale < 1 {
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		out = resizeNearest(out, nw, nh)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resizeNearest(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sx := float64(sb.Dx()) / float64(w)
	sy := float64(sb.Dy()) / float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := src.PixOffset(sb.Min.X+int(float64(x)*sx), sb.Min.Y+int(float64(y)*sy))
			di := dst.PixOffset(x, y)
			dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] =
				src.Pix[si], src.Pix[si+1], src.Pix[si+2], src.Pix[si+3]
		}
	}
	return dst
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
