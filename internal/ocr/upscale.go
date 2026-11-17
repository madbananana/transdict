package ocr

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
)

// Windows.Media.Ocr silently returns an empty result for pictures smaller than
// roughly 40x50. A single line of body text is only about 20px tall, so a
// normal one-line screenshot used to recognize nothing at all. Enlarging the
// crop before the helper sees it moves it out of that blind spot; a picture
// that is already big enough is returned unchanged.
const (
	ocrMinWidth  = 320
	ocrMinHeight = 96
	ocrMaxScale  = 4
)

func upscaleForOCR(pngBytes []byte) ([]byte, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, 1, err
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		return pngBytes, 1, nil
	}
	scale := 1
	for scale < ocrMaxScale && (cfg.Width*scale < ocrMinWidth || cfg.Height*scale < ocrMinHeight) {
		scale++
	}
	if scale == 1 {
		return pngBytes, 1, nil
	}

	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, 1, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, cfg.Width*scale, cfg.Height*scale))
	scaleBilinear(src, dst)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, 1, err
	}
	return buf.Bytes(), scale, nil
}

// scaleBilinear spreads each source pixel over the larger grid instead of
// duplicating it. Nearest-neighbour would work for the recognizer, but the
// smoothed edges read better on anti-aliased screen text.
func scaleBilinear(src image.Image, dst *image.RGBA) {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()

	flat := image.NewRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Src)

	for y := 0; y < dh; y++ {
		sy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		for x := 0; x < dw; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}

			i00 := flat.PixOffset(x0, y0)
			i10 := flat.PixOffset(x1, y0)
			i01 := flat.PixOffset(x0, y1)
			i11 := flat.PixOffset(x1, y1)
			di := dst.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				top := mix(flat.Pix[i00+c], flat.Pix[i10+c], fx)
				bottom := mix(flat.Pix[i01+c], flat.Pix[i11+c], fx)
				dst.Pix[di+c] = mix(top, bottom, fy)
			}
		}
	}
}

func mix(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

// shrinkBlocks maps boxes read from the enlarged picture back to the crop the
// preview shows.
func shrinkBlocks(blocks []Block, scale int) {
	if scale <= 1 {
		return
	}
	for i := range blocks {
		b := &blocks[i]
		b.X /= scale
		b.Y /= scale
		b.Width /= scale
		b.Height /= scale
	}
}
