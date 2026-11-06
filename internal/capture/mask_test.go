package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// textPNG draws black strokes on white: a recogniser can read it, and after a
// mosaic over the same area nothing should be left to read.
func textPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := 2; y < 10; y++ {
		for x := 2; x < 6; x++ {
			img.Set(x, y, color.Black)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码测试图: %v", err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码结果: %v", err)
	}
	out := image.NewNRGBA(img.Bounds())
	drawInto(out, img)
	return out
}

func drawInto(dst *image.NRGBA, src image.Image) {
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
}

func isBlack(img *image.NRGBA, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r < 0x2000 && g < 0x2000 && b < 0x2000
}

// 马赛克 is a privacy control: the bytes that go to an online OCR service must not
// still spell anything out.
func TestMosaicFlattensEveryCellItCovers(t *testing.T) {
	src := textPNG(t, 24, 16)
	masked, err := Mosaic(src, []MaskRect{{X: 0, Y: 0, W: 12, H: 12}}, 4)
	if err != nil {
		t.Fatalf("Mosaic: %v", err)
	}
	img := decode(t, masked)

	for y0 := 0; y0 < 12; y0 += 4 {
		for x0 := 0; x0 < 12; x0 += 4 {
			ref := img.At(x0, y0)
			for y := y0; y < y0+4; y++ {
				for x := x0; x < x0+4; x++ {
					if img.At(x, y) != ref {
						t.Fatalf("格子 (%d,%d) 内的 (%d,%d) 没被铺平，马赛克没盖住", x0, y0, x, y)
					}
				}
			}
		}
	}
	// The strokes lived entirely inside the masked area, so none may survive: a
	// pixel that is still black is a pixel a recogniser can still read.
	for y := 2; y < 10; y++ {
		for x := 2; x < 6; x++ {
			if isBlack(img, x, y) {
				t.Errorf("(%d,%d) 仍是黑色，笔画没被打散", x, y)
			}
		}
	}
}

// Only what the user painted over is hidden — the rest of the frame still has to be
// worth recognising.
func TestMosaicKeepsEverythingOutsideTheRect(t *testing.T) {
	src := textPNG(t, 24, 16)
	masked, err := Mosaic(src, []MaskRect{{X: 0, Y: 0, W: 12, H: 12}}, 4)
	if err != nil {
		t.Fatalf("Mosaic: %v", err)
	}
	before, after := decode(t, src), decode(t, masked)
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			if x < 12 && y < 12 {
				continue
			}
			if before.At(x, y) != after.At(x, y) {
				t.Errorf("(%d,%d) 在遮挡区外却被改了", x, y)
			}
		}
	}
}

// A rectangle dragged off the edge is trimmed, not a panic: the crop is scaled by
// the browser, so the webview can hand back coordinates past the border.
func TestMosaicTrimsRectanglesThatLeaveTheImage(t *testing.T) {
	src := textPNG(t, 24, 16)
	before := decode(t, src)
	cases := []struct {
		name        string
		rect        MaskRect
		wantChanged bool
	}{
		{"完全在图外", MaskRect{X: 100, Y: 100, W: 8, H: 8}, false},
		{"零面积", MaskRect{X: 4, Y: 4, W: 0, H: 9}, false},
		// Clipped to (0,0)-(6,6), which is where the strokes are.
		{"负坐标", MaskRect{X: -6, Y: -6, W: 12, H: 12}, true},
		// The part that overlaps the image is plain white, so the average is white.
		{"超出右下", MaskRect{X: 20, Y: 12, W: 40, H: 40}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			masked, err := Mosaic(src, []MaskRect{c.rect}, 4)
			if err != nil {
				t.Fatalf("Mosaic: %v", err)
			}
			after := decode(t, masked)
			if after.Bounds() != before.Bounds() {
				t.Fatalf("尺寸变了：%v → %v", before.Bounds(), after.Bounds())
			}
			changed := 0
			for y := 0; y < 16; y++ {
				for x := 0; x < 24; x++ {
					if after.At(x, y) != before.At(x, y) {
						changed++
					}
				}
			}
			if c.wantChanged && changed == 0 {
				t.Error("矩形与笔画有交集却没有改动任何像素")
			}
			if !c.wantChanged && changed != 0 {
				t.Errorf("不该有像素被改动，得到 %d 个", changed)
			}
		})
	}
}

// An empty list is how 遮挡重识别 is cancelled: the picture must come back as it
// went in, so the retry recognises the whole selection again.
func TestMosaicWithNothingToHideReturnsTheSamePicture(t *testing.T) {
	src := textPNG(t, 24, 16)
	masked, err := Mosaic(src, nil, MosaicCell)
	if err != nil {
		t.Fatalf("Mosaic: %v", err)
	}
	before, after := decode(t, src), decode(t, masked)
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			if before.At(x, y) != after.At(x, y) {
				t.Fatalf("(%d,%d) 在没有遮挡时被改动", x, y)
			}
		}
	}
}

// A cell smaller than two pixels cannot average anything, and a caller that passes
// one — or zero — means the default block size.
func TestMosaicRepairsAnImpossibleCell(t *testing.T) {
	src := textPNG(t, 24, 16)
	before := decode(t, src)
	masked, err := Mosaic(src, []MaskRect{{X: 0, Y: 0, W: 12, H: 12}}, 0)
	if err != nil {
		t.Fatalf("Mosaic: %v", err)
	}
	after := decode(t, masked)
	if after.At(3, 3) == before.At(3, 3) {
		t.Error("格子尺寸退化为 0 时没有铺平笔画")
	}
}
