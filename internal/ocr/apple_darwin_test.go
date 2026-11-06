//go:build darwin

package ocr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"transdict/internal/settings"
)

// TestAppleVisionRegistered checks that the kind is present in the shared
// registry only on darwin. That is the whole point of the stub split: Windows
// users should never see the entry at all, not see an always-erroring one.
func TestAppleVisionRegistered(t *testing.T) {
	if _, ok := Meta("apple"); !ok {
		t.Fatal(`kind "apple" missing from registry on darwin`)
	}
	m, _ := Meta("apple")
	if !m.Free {
		t.Fatal("apple Vision must be flagged Free — it needs no key")
	}
	if len(m.Fields) != 2 {
		t.Fatalf("expected the two optional fields, got %d", len(m.Fields))
	}
}

// TestAppleVisionTinyPNG exercises the cgo wrapper against a 1x1 image. Vision
// has no text to find; the wrapper must return either zero blocks or a clean
// Chinese error, and must not panic or leak. The point is not the OCR result
// — it is that the C boundary is intact end-to-end.
func TestAppleVisionTinyPNG(t *testing.T) {
	if !visionAvailable() {
		t.Skip("Vision unavailable on this host (CI or old SDK); skipping cgo smoke test")
	}

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Gray16{Y: 0xffff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	impl, err := settingsProviderForOCR(settings.Provider{Kind: "apple"})
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := impl.Recognize(context.Background(), buf.Bytes())
	if err != nil {
		// Vision is allowed to reject an absurdly small image; what matters is
		// that the failure surfaces as a Chinese sentence rather than a crash.
		t.Logf("Apple Vision returned a clean error for 1x1: %v", err)
		if !strings.Contains(err.Error(), "Apple Vision") {
			t.Fatalf("error should be prefixed with provider name, got %q", err)
		}
		return
	}
	if len(blocks) != 0 {
		t.Fatalf("expected zero blocks from a 1x1 image, got %d: %+v", len(blocks), blocks)
	}
}

// TestAppleVisionEmptyPng makes sure the wrapper short-circuits before the
// Objective-C call so we do not hand a NULL to C.
func TestAppleVisionEmptyPng(t *testing.T) {
	impl, err := settingsProviderForOCR(settings.Provider{Kind: "apple"})
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := impl.Recognize(context.Background(), nil)
	if err != nil {
		t.Fatalf("nil png should not error, got %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("nil png should yield zero blocks, got %d", len(blocks))
	}
}

// TestAppleLangHintIsThreaded confirms the provider-side context hint reads
// through the same path Apple Vision uses internally.
func TestAppleLangHintIsThreaded(t *testing.T) {
	ctx := ContextWithLang(context.Background(), "zh-Hans")
	if LangFromContext(ctx) != "zh-Hans" {
		t.Fatal("lang hint not stored on ctx")
	}
}

// TestVisionLangCodes checks the table codes reach Vision in the form it
// accepts; "zh-CN" is not a Vision tag, and passing it used to make the whole
// recognition list fall back to English.
func TestVisionLangCodes(t *testing.T) {
	cases := map[string]string{
		"zh-CN": "zh-Hans", "zh-TW": "zh-Hant", "en": "en-US", "ja": "ja-JP",
		"ko": "ko-KR", "zh-Hans": "zh-Hans", "vi": "vi",
	}
	for in, want := range cases {
		if got := visionLang(in); got != want {
			t.Errorf("visionLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// visionAvailable is defined in a helper file below so the smoke test can skip
// on hosts where Vision.framework refuses to load (rare, but a CI runner on an
// unsupported macOS release would otherwise fail the whole package).
func visionAvailable() bool {
	return CAppleVisionAvailable() == 1
}
