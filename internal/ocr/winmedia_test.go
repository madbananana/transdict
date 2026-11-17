package ocr

import (
	"net/url"
	"strings"
	"testing"
)

// winRecord mirrors the helper's protocol: fields separated by pipes, free text
// percent-encoded.
func winRecord(kind string, fields ...string) string {
	if len(fields) == 0 {
		return kind
	}
	return kind + "|" + strings.Join(fields, "|")
}

func TestDecodeWinOCRReadsBlocksInReadingOrder(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"engine|zh-Hans-CN",
		winRecord("block", "10", "40", "80", "20", "0", url.PathEscape("  第二行  ")),
		winRecord("block", "10", "8", "90", "20", "0", url.PathEscape("第一行")),
		winRecord("block", "0", "0", "0", "0", "0", ""),
	}, "\n"))
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
	if blocks[1].X != 10 || blocks[1].Y != 40 || blocks[1].Width != 80 || blocks[1].Height != 20 {
		t.Fatalf("box = %+v", blocks[1])
	}
}

func TestDecodeWinOCRKeepsNewlinesInsideALine(t *testing.T) {
	raw := []byte("engine|en-US\n" +
		winRecord("block", "0", "0", "10", "10", "0", url.PathEscape("第一段\n第二段")))
	blocks, err := decodeWinOCR(raw)
	if err != nil {
		t.Fatalf("decodeWinOCR: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text != "第一段\n第二段" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestDecodeWinOCRSurfacesTheHelperError(t *testing.T) {
	raw := []byte("\ufeff" + winRecord("error", url.PathEscape("系统没有安装任何 OCR 语言包")))
	if _, err := decodeWinOCR(raw); err == nil || !strings.Contains(err.Error(), "语言包") {
		t.Fatalf("err = %v, want the helper's message", err)
	}
}

func TestDecodeWinOCRRejectsGarbage(t *testing.T) {
	cases := map[string][]byte{
		"empty":  {},
		"blank":  []byte(" \n\r\n"),
		"noise":  []byte("not a record"),
		"legacy": []byte("eyJibG9ja3MiOltdfQ=="),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeWinOCR(raw); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestDecodeWinOCRSkipsMalformedBlocks(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"engine|en-US",
		winRecord("block", "0", "0", "bad", "10", "0", "ok"),
		winRecord("block", "0", "0", "10", "10", "0", "%ZZ"),
		winRecord("block", "0", "0", "10", "10", "0", url.PathEscape("good")),
	}, "\n"))
	blocks, err := decodeWinOCR(raw)
	if err != nil {
		t.Fatalf("decodeWinOCR: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text != "good" {
		t.Fatalf("blocks = %+v", blocks)
	}
}
