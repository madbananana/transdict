package gui

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/capture"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
)

// pasteboard adapts the toolkit clipboard to the narrow interface
// internal/selection needs, so that package never imports Wails.
type pasteboard struct{ s *Service }

func (p pasteboard) Read() (string, bool) { return p.s.wails.Clipboard.Text() }
func (p pasteboard) Write(text string) bool {
	return p.s.wails.Clipboard.SetText(text)
}

// CatchSelection answers what the user just highlighted — 划词翻译. The answer
// lands in the floating panel beside the pointer rather than in the main window,
// because the reading position should not move; 通用 → 划词显示悬浮窗 turns that off
// for people who would rather see every service at once.
func (s *Service) CatchSelection() error {
	stage := s.selectionContext(settings.ContextFloating)
	text, err := s.grabSelection()
	if err != nil {
		s.busy(stage, err.Error())
		return err
	}
	return s.Ask(query.Request{Text: text, Context: stage, Mode: "selection"})
}

// selectionContext sends a pointer-anchored query to the float, or to the main
// window when the floating panel is switched off.
func (s *Service) selectionContext(floating string) string {
	if s.cfg.Get().Floating {
		return floating
	}
	return settings.ContextMain
}

// ReplaceSelection translates the selection and types the answer back into the
// field it came from — 翻译并替换. The translation is left on the pasteboard, so
// a missed paste is one keystroke away from being redoable.
func (s *Service) ReplaceSelection() error {
	text, err := s.grabSelection()
	if err != nil {
		s.busy(settings.ContextReplace, err.Error())
		return err
	}
	go s.replace(text)
	return nil
}

func (s *Service) replace(text string) {
	cfg := s.cfg.Get()
	res := s.engine.Run(context.Background(), query.Request{
		Text: text, Context: settings.ContextReplace, Mode: "replace",
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang,
	})
	answer := query.FirstTranslation(res)
	if answer == "" {
		s.busy(settings.ContextReplace, firstOf(res.Error, "没有可用译文，无法替换"))
		s.emit(res)
		return
	}
	if !s.wails.Clipboard.SetText(answer) {
		s.busy(settings.ContextReplace, "写入剪贴板失败，无法替换")
		return
	}
	// The pasteboard write and the synthesized keystroke are separate system
	// calls; a target app that grabs the clipboard immediately needs the pause.
	time.Sleep(80 * time.Millisecond)
	if err := selection.Paste(); err != nil {
		s.busy(settings.ContextReplace, "替换失败: "+err.Error())
		return
	}
	s.emit(res)
}

// grabSelection reads the highlight, asking for the permission both routes need
// on macOS the first time it is missing.
func (s *Service) grabSelection() (string, error) {
	if !selection.Trusted() {
		askTrust.demand(s)
		return "", errors.New("需要「辅助功能」权限：请在 系统设置 → 隐私与安全性 → 辅助功能 中勾选 Transdict，然后重启应用")
	}
	return selection.Grab(pasteboard{s})
}

// WordUnderCursor reads the word beneath the pointer with OCR — 鼠标取词. This is
// the only route that works where there is no text layer to ask: a PDF image, a
// canvas, a video frame.
func (s *Service) WordUnderCursor() error {
	stage := s.selectionContext(settings.ContextODict)
	word, err := s.wordUnderPointer()
	if err != nil {
		s.busy(stage, err.Error())
		return err
	}
	return s.Ask(query.Request{Text: word, Context: stage,
		Mode: "odict", DictionaryOnly: true})
}

// The OCR slice handed to 鼠标取词: one line tall, wide enough for a long word
// plus its neighbours so the pointer's horizontal position inside a block means
// something.
const (
	lineBandWidthDIP  = 520
	lineBandHeightDIP = 46
)

func (s *Service) wordUnderPointer() (string, error) {
	if !capture.ScreenAccess() {
		askScreen.demand(s)
		return "", errors.New("鼠标取词需要「屏幕录制」权限")
	}
	cursor := capture.Cursor()
	if cursor.X == 0 && cursor.Y == 0 {
		return "", errors.New("无法定位鼠标")
	}
	frames, err := capture.Displays()
	if err != nil {
		return "", err
	}
	byIndex := make(map[int]capture.Frame, len(frames))
	for _, f := range frames {
		byIndex[f.Index] = f
	}

	dip := s.wails.Screen.PhysicalToDipPoint(application.Point{X: cursor.X, Y: cursor.Y})
	for i, scr := range s.wails.Screen.GetAll() {
		b := scr.Bounds
		if dip.X < b.X || dip.X > b.X+b.Width || dip.Y < b.Y || dip.Y > b.Y+b.Height {
			continue
		}
		frame, ok := byIndex[i]
		if !ok || frame.Width <= 0 || b.Width <= 0 {
			return "", errors.New("缺少该显示器的截图")
		}
		// The captured image is in device pixels while Wails speaks DIP; the
		// ratio between them is the display scale.
		scale := float64(frame.Width) / float64(b.Width)
		centerX := int(math.Round(float64(dip.X-b.X) * scale))
		centerY := int(math.Round(float64(dip.Y-b.Y) * scale))

		cw := clampInt(int(math.Round(float64(lineBandWidthDIP)*scale)), 1, frame.Width)
		ch := clampInt(int(math.Round(float64(lineBandHeightDIP)*scale)), 1, frame.Height)
		x := clampInt(centerX-cw/2, 0, frame.Width-1)
		y := clampInt(centerY-ch/2, 0, frame.Height-1)
		cw = clampInt(cw, 1, frame.Width-x)
		ch = clampInt(ch, 1, frame.Height-y)

		crop, err := capture.Crop(frame.PNG, x, y, cw, ch)
		if err != nil {
			return "", err
		}
		// Crop downscales oversized images; the pointer has to follow.
		kept := math.Min(1, float64(capture.MaxEdge)/float64(max(cw, ch)))
		return s.wordFromCrop(crop,
			int(math.Round(float64(centerX-x)*kept)), int(math.Round(float64(centerY-y)*kept)))
	}
	return "", errors.New("鼠标不在任何显示器上")
}

func (s *Service) wordFromCrop(crop []byte, x, y int) (string, error) {
	cfg := s.cfg.Get()
	req := query.Request{Context: settings.ContextODict, Mode: "odict", Image: crop,
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang}
	for _, outcome := range s.engine.OCRStage(context.Background(), cfg, req) {
		if outcome.Error != "" {
			continue
		}
		if word := ocr.WordAt(outcome.Blocks, x, y); word != "" {
			return word, nil
		}
	}
	return "", errors.New("鼠标处没有识别到单词")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
