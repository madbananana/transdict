package gui

import (
	"context"
	"time"

	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/settings"
)

// OCRPreview is what the OCR 窗口 draws: the picture that was recognized, every
// service's lines with their boxes in crop pixels, and the merged text that came
// out of them. EasyDict shows the same three pieces side by side; nothing here
// is translated, because the window is the OCR answer itself.
type OCRPreview struct {
	Image     string        `json:"image"`
	Text      string        `json:"text"`
	Outcomes  []ocr.Outcome `json:"outcomes"`
	ElapsedMs int64         `json:"elapsedMs"`
	Error     string        `json:"error,omitempty"`
}

// OCRPreview hands the window the newest recognition. An empty payload is a
// valid answer: 显示 OCR 窗口 can be pressed before anything has been read, and
// the page then shows its empty state the way EasyDict opens a blank preview.
func (s *Service) OCRPreview() OCRPreview {
	s.mu.Lock()
	last := s.lastOCR
	s.mu.Unlock()
	return previewOf(last)
}

func previewOf(res *query.Response) OCRPreview {
	if res == nil {
		return OCRPreview{}
	}
	return OCRPreview{
		Image:     res.ImageURL,
		Text:      res.SourceText,
		Outcomes:  res.OCR,
		ElapsedMs: res.ElapsedMs,
		Error:     res.Error,
	}
}

// showOCRPreview raises the OCR 窗口 and lets the page re-read what is in it: the
// window is hidden rather than destroyed between shows, so a second recognition
// would otherwise leave the first picture on screen.
func (s *Service) showOCRPreview() {
	s.wails.Event.Emit(EventCommand, "ocr-preview")
	s.show(WindowOCR, true)
}

// runOCRPreview recognizes a screenshot or clipboard picture for the OCR 窗口.
// It is the whole answer for 截图 OCR and 剪贴板 OCR: EasyDict does not translate
// these, it opens the preview with the recognized text selectable beside the
// image.
func (s *Service) runOCRPreview(png []byte) {
	cfg := s.cfg.Get()
	start := time.Now()
	out := s.engine.OCRStage(context.Background(), cfg, query.Request{
		Context: settings.ContextScreenshot, Mode: "ocr", Image: png,
		SourceLang: cfg.SourceLang,
	})
	res := &query.Response{
		Context:    settings.ContextScreenshot,
		Mode:       "ocr",
		SourceText: query.OCRText(cfg, out),
		TargetLang: cfg.TargetLang,
		ImageURL:   query.DataURL(png),
		OCR:        out,
		ElapsedMs:  time.Since(start).Milliseconds(),
	}
	if len(out) == 0 {
		// No engine ran at all, so the empty window would read as a broken one.
		res.Error = "没有可用的文字识别服务"
	}
	s.mu.Lock()
	s.lastOCR = res
	s.mu.Unlock()
	s.showOCRPreview()
}
