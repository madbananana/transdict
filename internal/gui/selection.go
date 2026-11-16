package gui

import (
	"context"
	"errors"
	"log"
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

// errTrust is the refusal both selection actions give while macOS has not
// granted accessibility: without it neither the AX read nor the copy chord can
// see another app's text.
var errTrust = errors.New("需要「辅助功能」权限：请在 系统设置 → 隐私与安全性 → 辅助功能 中勾选 Transdict，然后重启应用")

// pasteboard adapts the toolkit clipboard to the narrow interface
// internal/selection needs, so that package never imports Wails.
type pasteboard struct{ s *Service }

func (p pasteboard) Read() (string, bool) { return p.s.wails.Clipboard.Text() }
func (p pasteboard) Write(text string) bool {
	return p.s.wails.Clipboard.SetText(text)
}

// CatchSelection answers what the user just highlighted — 划词翻译. The answer
// lands in the translation window, the only query window the app draws.
func (s *Service) CatchSelection() error {
	stage := settings.ContextMain
	if !selection.Trusted() {
		askTrust.demand(s)
		s.busy(stage, errTrust.Error())
		return errTrust
	}
	resume := s.yieldKeyboard()
	defer resume()
	text, err := s.grabSelection()
	if err != nil {
		// 划词翻译未选中文本时，保留上次结果: the window the user was reading stays
		// as it was instead of turning into an error line. The error still goes
		// back to the caller so the toolbar button can put a note on screen:
		// the window flashing and keeping its old answer looked like a dead
		// button when nothing could be read.
		if s.cfg.Get().KeepLastOnNoSelection && noSelection(err) && s.keepLast() {
			return err
		}
		s.busy(stage, err.Error())
		return err
	}
	return s.Ask(query.Request{Text: text, Context: stage, Mode: "selection",
		Replaceable: selection.FocusedElementEditable()})
}

// noSelection separates "there was nothing highlighted" from a failed read.
// The accessibility route answers ErrNothingSelected; an app that exposes no
// selection falls back to the copy chord, whose silence is ErrTimeout.
func noSelection(err error) bool {
	return errors.Is(err, selection.ErrNothingSelected) || errors.Is(err, selection.ErrTimeout)
}

// keepLast re-shows what the translation window was already displaying. It
// reports false when there is nothing to keep.
func (s *Service) keepLast() bool {
	if s.snapshot() == nil {
		return false
	}
	s.show(WindowMain, true)
	return true
}

// yieldKeyboard steps the app aside when the press came from one of our own
// windows: the accessibility read looks at the frontmost application, and the
// click that fired 划词翻译 has already made that this window — the read would
// land in our own text box and come back empty. Handing the keyboard back to the
// app the user was reading fixes that without hiding anything, and it is what
// EasyDict does when the selection window appears: the source app stays in
// front. The shortcut route never gets here, because that app is still in front.
func (s *Service) yieldKeyboard() func() {
	if !selection.FrontmostIsSelf() {
		return func() {}
	}
	if s.reactivateSource() {
		return func() {}
	}
	// Nothing to hand back to (the session began inside Transdict): hiding is
	// the only way to expose whatever window sits underneath, and our windows
	// come straight back once the text is in hand.
	application.InvokeSync(func() { s.wails.Hide() })
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && selection.FrontmostIsSelf() {
		time.Sleep(15 * time.Millisecond)
	}
	return func() {
		application.InvokeSync(func() { s.wails.Show() })
	}
}

// reactivateSource hands the keyboard back to the app the selection came from.
// It reports false when there is nothing to reactivate, which leaves the caller
// to fall back (hiding, or asking the user to click back into the app).
func (s *Service) reactivateSource() bool {
	if !selection.ReactivateLastOther() {
		return false
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && selection.FrontmostIsSelf() {
		time.Sleep(15 * time.Millisecond)
	}
	return !selection.FrontmostIsSelf()
}

// copySelection is 自动复制划词文本: the highlight that started the query goes on
// the pasteboard, so it can be pasted somewhere after the panel closes. The
// shortcut and the mouse watcher both arrive through Ask, which is why the check
// lives on the mode rather than in each caller.
func (s *Service) copySelection(mode, text string) {
	text = selectionCopyText(s.cfg.Get(), mode, text)
	if text == "" {
		return
	}
	if !s.wails.Clipboard.SetText(text) {
		log.Print("transdict: 自动复制划词文本失败")
	}
}

func selectionCopyText(cfg settings.Config, mode, text string) string {
	if text == "" || !cfg.AutoCopySelect {
		return ""
	}
	if mode != "selection" && mode != "auto-selection" {
		return ""
	}
	return text
}

// ReplaceSelection translates the selection and types the answer back into the
// field it came from — 翻译并替换. The translation is left on the pasteboard, so
// a missed paste is one keystroke away from being redoable.
func (s *Service) ReplaceSelection() error {
	text, err := s.selectionForReplace()
	if err != nil {
		s.busy(settings.ContextReplace, err.Error())
		return err
	}
	go s.replace(text)
	return nil
}

// ReplaceWithText is the result card's 替换 button (EasyDict's EZReplaceTextButton):
// the answer the user picked goes back over the selection the query was made
// from. Only a query about a live, editable selection carries Replaceable, which
// is what puts the button on the cards in the first place.
func (s *Service) ReplaceWithText(text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("没有可替换的译文")
	}
	if !selection.Trusted() {
		askTrust.demand(s)
		return errTrust
	}
	// The click took the keyboard for our window; the paste has to land in the
	// app the selection came from, so that app goes back in front first.
	if selection.FrontmostIsSelf() && !s.reactivateSource() {
		return errors.New("请先回到要替换的应用")
	}
	return writeBack(s.cfg.Get(), s.wails.Clipboard.SetText, text)
}

// shouldWiden is 自动选中所有文本's gate: only an empty selection is widened,
// and only when the option is on. A permission or platform refusal keeps its
// own error so the panel can name the real problem.
func shouldWiden(cfg settings.Config, err error) bool {
	return cfg.AutoSelectAllText && noSelection(err)
}

// selectionForReplace reads 翻译并替换's input. 自动选中所有文本 widens an empty
// selection to the whole field and reads again; without it the paste would land
// at a caret and leave the original text in front of the translation.
func (s *Service) selectionForReplace() (string, error) {
	// A click on one of our own windows takes the keyboard with it; the text this
	// replaces lives in the app the user was reading in. Hand it back and leave it
	// there — the paste has to land in the same place the selection came from.
	s.yieldKeyboard()
	if selection.FrontmostIsSelf() {
		return "", errors.New("请先回到要替换的应用")
	}
	text, err := s.grabSelection()
	if !shouldWiden(s.cfg.Get(), err) {
		return text, err
	}
	if selErr := selection.SelectAll(); selErr != nil {
		return "", err
	}
	// cmd+A has to reach the app before the read asks what is highlighted.
	time.Sleep(80 * time.Millisecond)
	return s.grabSelection()
}

// polishInstruction is the ad-hoc prompt behind 润色并替换, the same wording the
// 润色 chip in the panels uses.
const polishInstruction = "在不改变原意的前提下润色下面的文字，使其更地道、通顺："

// PolishReplaceSelection is 润色并替换: the selection is rewritten by an AI
// service under the 润色 instruction, and the answer takes its place. Reading
// and widening the selection works exactly as 翻译并替换.
func (s *Service) PolishReplaceSelection() error {
	text, err := s.selectionForReplace()
	if err != nil {
		s.busy(settings.ContextReplace, err.Error())
		return err
	}
	go s.replaceWith(polishReplaceRequest(text))
	return nil
}

// polishReplaceRequest runs on the AI 润色 window's services: a plain translator
// has no instruction to follow.
func polishReplaceRequest(text string) query.Request {
	return promptRequest(text, polishInstruction)
}

func (s *Service) replace(text string) {
	cfg := s.cfg.Get()
	s.replaceWith(query.Request{
		Text: text, Context: settings.ContextReplace, Mode: "replace",
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang,
	})
}

// replaceWith is the half both replace actions share: run the query, take the
// first answer and paste or type it back over the selection.
func (s *Service) replaceWith(req query.Request) {
	res := s.engine.Run(context.Background(), req)
	answer := query.FirstTranslation(res)
	if answer == "" {
		s.busy(settings.ContextReplace, firstOf(res.Error, "没有可用译文，无法替换"))
		s.emit(res)
		return
	}
	if err := writeBack(s.cfg.Get(), s.wails.Clipboard.SetText, answer); err != nil {
		s.busy(settings.ContextReplace, err.Error())
		return
	}
	s.emit(res)
}

// writeBack hands the answer back to the field the selection came from. Pasting
// is the default and leaves the translation on the pasteboard, so a missed paste
// is one keystroke from being redone; 兼容模式替换 writes it through the
// accessibility API instead, for the apps that ignore cmd+V. A refusal falls
// back to pasting rather than leaving the field alone. Hooks are variables so
// both routes are testable without a window server.
func writeBack(cfg settings.Config, setClipboard func(string) bool, answer string) error {
	if cfg.ReplaceCompatible {
		if err := typeAnswer(answer); err == nil {
			return nil
		}
	}
	if !setClipboard(answer) {
		return errors.New("写入剪贴板失败，无法替换")
	}
	// The pasteboard write and the synthesized keystroke are separate system
	// calls; a target app that grabs the clipboard immediately needs the pause.
	time.Sleep(80 * time.Millisecond)
	if err := pasteAnswer(); err != nil {
		return errors.New("替换失败: " + err.Error())
	}
	return nil
}

var (
	pasteAnswer = selection.Paste
	typeAnswer  = selection.TypeText
)

// grabSelection reads the highlight, asking for the permission both routes need
// on macOS the first time it is missing.
func (s *Service) grabSelection() (string, error) {
	if !selection.Trusted() {
		askTrust.demand(s)
		return "", errTrust
	}
	text, err := selection.Grab(pasteboard{s})
	if err != nil {
		return "", err
	}
	return s.booksExcerptText(text), nil
}

// WordUnderCursor reads the word beneath the pointer with OCR — 鼠标取词. This is
// the only route that works where there is no text layer to ask: a PDF image, a
// canvas, a video frame.
func (s *Service) WordUnderCursor() error {
	stage := settings.ContextODict
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
	cursor, ok := s.cursorDip()
	if !ok {
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

	dip := cursor
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
