package ocr

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSortBlocksReadingOrder(t *testing.T) {
	cases := []struct {
		name string
		in   []Block
		want []string
	}{
		{
			name: "top to bottom",
			in: []Block{
				{Text: "second", Y: 40, Height: 20},
				{Text: "first", Y: 10, Height: 20},
				{Text: "third", Y: 70, Height: 20},
			},
			want: []string{"first", "second", "third"},
		},
		{
			name: "same band falls back to x",
			in: []Block{
				{Text: "right", X: 200, Y: 10, Height: 20},
				{Text: "left", X: 20, Y: 12, Height: 20},
				{Text: "middle", X: 100, Y: 11, Height: 20},
			},
			want: []string{"left", "middle", "right"},
		},
		{
			name: "coordinate-less blocks keep input order",
			in: []Block{
				{Text: "a"},
				{Text: "b"},
				{Text: "c"},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "different heights in one band still sort by x",
			in: []Block{
				{Text: "big-heading", X: 10, Y: 5, Height: 60},
				{Text: "caption", X: 200, Y: 20, Height: 14},
			},
			// dy = 15 and the band threshold is max(60, 14) / 2 = 30, so the
			// two blocks land in the same horizontal band and X decides.
			want: []string{"big-heading", "caption"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := append([]Block(nil), tc.in...)
			sortBlocksReadingOrder(cp)
			got := make([]string, len(cp))
			for i, b := range cp {
				got[i] = b.Text
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sortBlocksReadingOrder = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVendorErrorFormats(t *testing.T) {
	cases := []struct {
		name        string
		prefix      string
		code        string
		message     string
		body        string
		wantContain string
	}{
		{"both", "腾讯 OCR", "FailedOperation", "quota exhausted", `{}`, "腾讯 OCR FailedOperation: quota exhausted"},
		{"message only", "腾讯 OCR", "", "quota exhausted", `{}`, "腾讯 OCR: quota exhausted"},
		{"code only", "腾讯 OCR", "FailedOperation", "", `{}`, "返回错误码 FailedOperation"},
		{"neither", "腾讯 OCR", "", "", `{"raw":"body"}`, "响应片段"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vendorError(tc.prefix, tc.code, tc.message, tc.body)
			if err == nil {
				t.Fatal("vendorError returned nil")
			}
			if got := err.Error(); !strings.Contains(got, tc.wantContain) {
				t.Fatalf("err = %q, want to contain %q", got, tc.wantContain)
			}
		})
	}
}

func TestLangHintContext(t *testing.T) {
	base := context.Background()
	if got := LangFromContext(base); got != "" {
		t.Fatalf("empty ctx should return \"\", got %q", got)
	}
	// "auto" and blank codes are the sentinel values from langs.Auto; they must
	// be a no-op so downstream providers see no hint at all.
	for _, code := range []string{"", "  ", "auto"} {
		if got := LangFromContext(ContextWithLang(base, code)); got != "" {
			t.Fatalf("ContextWithLang(%q) should not tag the ctx, got %q", code, got)
		}
	}
	if got := LangFromContext(ContextWithLang(base, "zh-Hans")); got != "zh-Hans" {
		t.Fatalf("LangFromContext = %q, want %q", got, "zh-Hans")
	}
}
