package ocr

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func winPayload(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return []byte(base64.StdEncoding.EncodeToString(data))
}

func TestDecodeWinOCRReadsBlocksInReadingOrder(t *testing.T) {
	raw := winPayload(t, winOCRResponse{
		Engine: "zh-Hans-CN",
		Blocks: []Block{
			{Text: "  第二行  ", X: 10, Y: 40, Width: 80, Height: 20},
			{Text: "第一行", X: 10, Y: 8, Width: 90, Height: 20},
			{Text: "   ", X: 0, Y: 0},
		},
	})
	blocks, err := decodeWinOCR(raw)
	if err != nil {
		t.Fatalf("decodeWinOCR: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (blank line dropped)", len(blocks))
	}
	if blocks[0].Text != "第一行" || blocks[1].Text != "第二行" {
		t.Fatalf("order = %q, %q", blocks[0].Text, blocks[1].Text)
	}
	if blocks[1].Text != "第二行" {
		t.Fatalf("text not trimmed: %q", blocks[1].Text)
	}
}

func TestDecodeWinOCRSurfacesTheHelperError(t *testing.T) {
	raw := winPayload(t, winOCRResponse{Error: "系统没有安装任何 OCR 语言包"})
	if _, err := decodeWinOCR(raw); err == nil || !strings.Contains(err.Error(), "语言包") {
		t.Fatalf("err = %v, want the helper's message", err)
	}
}

func TestDecodeWinOCRRejectsGarbage(t *testing.T) {
	cases := map[string][]byte{
		"empty":  {},
		"base64": []byte("not base64!!"),
		"json":   winPayload(t, "not an object"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeWinOCR(raw); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}
