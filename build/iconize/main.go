// iconize replaces the tiny Windows icon layers with a simplified mark.
//
// The app icon is 文A on a gradient tile. That composition is legible from 32px
// up, but Windows also asks for 16, 20 and 24px (the taskbar at 100% DPI uses
// 24), where both glyphs turn into a grey smudge after the 1024px source is
// downsampled. The small sizes therefore draw 文 alone at the target pixel size,
// so the strokes are rendered once instead of being resampled.
//
// It runs from `wails3 task generate:icons` after the CLI has produced the
// large layers, and writes both the merged app icon and the notification-area
// icon used by the Windows tray.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"sort"
)

const (
	appSmall  = "16,20,24"
	traySizes = "16,20,24,32,40,48"
)

func main() {
	iconPath := flag.String("icon", "windows/icon.ico", "app icon to rewrite")
	trayPath := flag.String("tray", "trayicon-windows.ico", "tray icon to write")
	flag.Parse()

	existing, err := os.ReadFile(*iconPath)
	if err != nil {
		log.Fatalf("iconize: reading %s: %v", *iconPath, err)
	}
	layers, err := readICO(existing)
	if err != nil {
		log.Fatalf("iconize: parsing %s: %v", *iconPath, err)
	}

	small := parseSizes(appSmall)
	merged := make([]layer, 0, len(layers)+len(small))
	for _, l := range layers {
		if !contains(small, l.size) {
			merged = append(merged, l)
		}
	}
	for _, size := range small {
		merged = append(merged, layer{size: size, data: bitmap(render(size))})
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].size > merged[j].size })
	if err := os.WriteFile(*iconPath, writeICO(merged), 0o644); err != nil {
		log.Fatalf("iconize: writing %s: %v", *iconPath, err)
	}

	tray := make([]layer, 0, 6)
	for _, size := range parseSizes(traySizes) {
		tray = append(tray, layer{size: size, data: bitmap(render(size))})
	}
	if err := os.WriteFile(*trayPath, writeICO(tray), 0o644); err != nil {
		log.Fatalf("iconize: writing %s: %v", *trayPath, err)
	}
}

type layer struct {
	size int
	data []byte
}

func parseSizes(s string) []int {
	var sizes []int
	var n int
	for _, part := range bytes.Split([]byte(s), []byte(",")) {
		if _, err := fmt.Sscanf(string(part), "%d", &n); err == nil && n > 0 && n <= 256 {
			sizes = append(sizes, n)
		}
	}
	sort.Ints(sizes)
	return sizes
}

func contains(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func readICO(data []byte) ([]layer, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("file too short")
	}
	_, typ, count := binary.LittleEndian.Uint16(data[0:]), binary.LittleEndian.Uint16(data[2:]), binary.LittleEndian.Uint16(data[4:])
	if typ != 1 {
		return nil, fmt.Errorf("not an icon")
	}
	layers := make([]layer, 0, count)
	for i := 0; i < int(count); i++ {
		off := 6 + 16*i
		if off+16 > len(data) {
			return nil, fmt.Errorf("truncated directory")
		}
		w := int(data[off])
		if w == 0 {
			w = 256
		}
		size := int(binary.LittleEndian.Uint32(data[off+8:]))
		start := int(binary.LittleEndian.Uint32(data[off+12:]))
		if start < 0 || size < 0 || start+size > len(data) {
			return nil, fmt.Errorf("truncated layer %d", i)
		}
		layers = append(layers, layer{size: w, data: data[start : start+size]})
	}
	return layers, nil
}

func writeICO(layers []layer) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(layers)))
	offset := 6 + 16*len(layers)
	for _, l := range layers {
		b := byte(l.size)
		if l.size == 256 {
			b = 0
		}
		buf.Write([]byte{b, b, 0, 0})
		binary.Write(&buf, binary.LittleEndian, uint16(1))
		binary.Write(&buf, binary.LittleEndian, uint16(32))
		binary.Write(&buf, binary.LittleEndian, uint32(len(l.data)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(l.data)
	}
	for _, l := range layers {
		buf.Write(l.data)
	}
	return buf.Bytes()
}

// 文 drawn as four round-capped strokes: the top tick, the horizontal, and the
// two falling strokes. Fractions are of the tile size.
type stroke struct {
	x0, y0, x1, y1, w float64
}

var mark = []stroke{
	{0.53, 0.15, 0.46, 0.28, 0.075},
	{0.19, 0.41, 0.81, 0.41, 0.095},
	{0.50, 0.41, 0.21, 0.84, 0.095},
	{0.53, 0.53, 0.83, 0.84, 0.095},
}

func render(n int) *image.NRGBA {
	const ss = 6
	w := n * ss
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/ss) / float64(n)
					py := (float64(y) + (float64(sy)+0.5)/ss) / float64(n)
					cov := clamp(0.5 - roundBox(px*float64(w), py*float64(w), float64(w), 0.235*float64(w))/ss)
					if cov <= 0 {
						continue
					}
					cr := 0.50 + (0.42-0.50)*py
					cg := 0.58 + (0.35-0.58)*py
					cb := 1.00 + (0.92-1.00)*py
					d := math.Inf(1)
					for _, s := range mark {
						if ds := segment(px*float64(w), py*float64(w),
							s.x0*float64(w), s.y0*float64(w), s.x1*float64(w), s.y1*float64(w), s.w*float64(w)); ds < d {
							d = ds
						}
					}
					ink := clamp(0.5 - d/ss)
					r += cr*(1-ink) + ink
					g += cg*(1-ink) + ink
					b += cb*(1-ink) + ink
					a += cov
				}
			}
			if a > 0 {
				img.Set(x, y, color.NRGBA{
					R: uint8(math.Round(r / a * 255)),
					G: uint8(math.Round(g / a * 255)),
					B: uint8(math.Round(b / a * 255)),
					A: uint8(math.Round(a / (ss * ss) * 255)),
				})
			}
		}
	}
	return img
}

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func roundBox(px, py, size, radius float64) float64 {
	dx := math.Abs(px-size/2) - (size/2 - radius)
	dy := math.Abs(py-size/2) - (size/2 - radius)
	ax, ay := math.Max(dx, 0), math.Max(dy, 0)
	return math.Hypot(ax, ay) + math.Min(math.Max(dx, dy), 0) - radius
}

func segment(px, py, x0, y0, x1, y1, width float64) float64 {
	vx, vy := x1-x0, y1-y0
	wx, wy := px-x0, py-y0
	t := (wx*vx + wy*vy) / (vx*vx + vy*vy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(wx-t*vx, wy-t*vy) - width/2
}

// bitmap encodes the image as a 32bpp bottom-up DIB with its AND mask, the
// layer format Windows expects inside an ICO for the sizes below 256.
func bitmap(img *image.NRGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	maskRow := ((w + 31) / 32) * 4
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, int32(w))
	binary.Write(&buf, binary.LittleEndian, int32(h*2))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(32))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(w*h*4+maskRow*h))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	mask := make([]byte, maskRow*h)
	for y := 0; y < h; y++ {
		row := mask[(h-1-y)*maskRow:]
		for x := 0; x < w; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				row[x/8] |= 1 << (7 - uint(x%8))
			}
		}
	}
	buf.Write(mask)
	return buf.Bytes()
}
