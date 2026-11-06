package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCropKeepsSourcePixels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}

	out, err := Crop(encode(t, src), 1, 1, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("crop bounds = %v, want 2x2", b)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			got, want := img.At(x, y), src.At(x+1, y+1)
			if got != want {
				t.Errorf("pixel(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestCropClampsOutOfBoundsSelection(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 3))
	out, err := Crop(encode(t, src), 1, 1, 99, 99)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("clamped bounds = %v, want 2x2", b)
	}
}

func TestStoreRoundTripsFramesAndCrops(t *testing.T) {
	s := NewStore()
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	enc := encode(t, src)
	id := s.NewSession([]Frame{{Index: 0, PNG: enc, Width: 2, Height: 2}})

	data, ok := s.Frame(id, 0)
	if !ok || !bytes.Equal(data, enc) {
		t.Fatalf("frame lookup failed: ok=%v len=%d", ok, len(data))
	}
	if _, ok := s.Frame(id, 7); ok {
		t.Errorf("unknown frame index should miss")
	}

	s.PutCrop(id, enc)
	if crop, ok := s.Crop(id); !ok || !bytes.Equal(crop, enc) {
		t.Errorf("crop lookup failed")
	}
}

func TestStoreEvictsOldSessions(t *testing.T) {
	s := NewStore()
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	ids := make([]string, keepSessions+2)
	for i := range ids {
		ids[i] = s.NewSession([]Frame{{Index: 0, PNG: encode(t, src), Width: 1, Height: 1}})
	}
	if _, ok := s.Session(ids[0]); ok {
		t.Errorf("oldest session should have been evicted")
	}
	if _, ok := s.Session(ids[len(ids)-1]); !ok {
		t.Errorf("newest session should still be present")
	}
}
