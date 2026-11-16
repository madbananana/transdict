package gui

import (
	"testing"

	"transdict/internal/ocr"
	"transdict/internal/query"
)

func TestOCRPreviewHandsOverTheLastRecognition(t *testing.T) {
	res := &query.Response{
		ImageURL:   "data:image/png;base64,AA",
		SourceText: "hello",
		OCR: []ocr.Outcome{{
			Provider: "apple-ocr", Label: "苹果 OCR", ElapsedMs: 12,
			Blocks: []ocr.Block{{Text: "hello", X: 1, Y: 2, Width: 30, Height: 4, Score: 0.95}},
		}},
		ElapsedMs: 42,
	}
	got := previewOf(res)
	if got.Image != res.ImageURL || got.Text != "hello" || got.ElapsedMs != 42 {
		t.Fatalf("预览 = %+v, want 上一次识别", got)
	}
	if len(got.Outcomes) != 1 || len(got.Outcomes[0].Blocks) != 1 {
		t.Fatalf("预览结果 = %+v, want 1 个服务的 1 行", got.Outcomes)
	}
	if b := got.Outcomes[0].Blocks[0]; b.X != 1 || b.Y != 2 || b.Width != 30 || b.Height != 4 {
		t.Errorf("文字框 = %+v, want 与识别时相同", b)
	}
}

func TestOCRPreviewBeforeAnyRecognitionIsEmpty(t *testing.T) {
	got := previewOf(nil)
	if got.Image != "" || got.Text != "" || len(got.Outcomes) != 0 {
		t.Errorf("空预览 = %+v, want 空", got)
	}
}
