package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// MaskRect is one region of a crop, in that crop's own pixel coordinates.
type MaskRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// MosaicCell is coarse enough that no glyph survives a cell: the point of 马赛克
// is that the pixels inside it may still be sent to an online recogniser without
// the words in them arriving.
const MosaicCell = 12

// Mosaic pixelates the listed regions of a PNG and re-encodes it. Rectangles that
// fall partly outside the image are trimmed, and a region that ends up with no
// pixels left is simply ignored — the user drew it off the edge, not at the text.
func Mosaic(pngBytes []byte, rects []MaskRect, cell int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	img := image.NewRGBA(bounds)
	draw.Draw(img, bounds, src, bounds.Min, draw.Src)

	if cell < 2 {
		cell = MosaicCell
	}
	for _, r := range rects {
		cellrects(img, r, bounds, cell)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cellrects(img *image.RGBA, r MaskRect, bounds image.Rectangle, cell int) {
	minX := clamp(r.X, bounds.Min.X, bounds.Max.X)
	minY := clamp(r.Y, bounds.Min.Y, bounds.Max.Y)
	maxX := clamp(r.X+r.W, bounds.Min.X, bounds.Max.X)
	maxY := clamp(r.Y+r.H, bounds.Min.Y, bounds.Max.Y)
	if maxX <= minX || maxY <= minY {
		return
	}

	for y := minY; y < maxY; y += cell {
		for x := minX; x < maxX; x += cell {
			// The cell may hang off the region's right or bottom edge.
			ex, ey := min(x+cell, maxX), min(y+cell, maxY)
			avg := average(img, x, y, ex, ey)
			for py := y; py < ey; py++ {
				for px := x; px < ex; px++ {
					img.Set(px, py, avg)
				}
			}
		}
	}
}

func average(img *image.RGBA, x0, y0, x1, y1 int) color.RGBA {
	var rs, gs, bs, as, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			rs += uint64(r)
			gs += uint64(g)
			bs += uint64(b)
			as += uint64(a)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	// RGBA() returns 16-bit channels; the division keeps them that way until the
	// conversion back to color.RGBA narrows them.
	return color.RGBA{
		R: uint8(rs / n >> 8), G: uint8(gs / n >> 8),
		B: uint8(bs / n >> 8), A: uint8(as / n >> 8),
	}
}
