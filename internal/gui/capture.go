package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/capture"
	"transdict/internal/dock"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
)

// captureRevealTimeout is how long the overlays stay hidden waiting for their
// pages to report ready. A page that fails to load never reports, and a frozen
// screen that cannot be dismissed is worse than a flash.
const captureRevealTimeout = 1500 * time.Millisecond

// captureOverlay pairs a display's selection window with the frame it shows,
// so CaptureReady can reveal the one the page has actually prepared.
type captureOverlay struct {
	frame int
	win   *application.WebviewWindow
}

// Start freezes every display and covers each with a borderless selection
// window. The user then drags a marquee on the frozen frame, which is what
// makes the crop coordinates independent of window positioning.
func (s *Service) Start() error {
	if err := s.start(); err != nil {
		s.fail(err)
		return err
	}
	return nil
}

func (s *Service) start() error {
	s.mu.Lock()
	s.teardownLocked()
	s.silent = false
	s.ocrOnly = false
	s.mu.Unlock()

	if !capture.ScreenAccess() {
		askScreen.demand(s)
		return errors.New("需要「屏幕录制」权限：请在 系统设置 → 隐私与安全性 → 录屏与系统录音 中打开 Transdict，然后重启应用")
	}

	frames, err := capture.Displays()
	if err != nil {
		return fmt.Errorf("截屏失败，请检查屏幕录制权限: %w", err)
	}
	screens := s.wails.Screen.GetAll()
	if len(screens) == 0 {
		return errors.New("未检测到显示器")
	}

	byIndex := make(map[int]capture.Frame, len(frames))
	for _, f := range frames {
		byIndex[f.Index] = f
	}

	id := s.shots.NewSession(frames)
	// The overlay asks for its display copy the moment its page mounts; encoding
	// it now keeps that decode off the wait between the shortcut and the frozen
	// screen.
	for _, f := range frames {
		go s.shots.Preview(id, f.Index)
	}
	overlays := make([]captureOverlay, 0, len(screens))

	for i, scr := range screens {
		frame, ok := byIndex[i]
		if !ok {
			frame = frames[min(i, len(frames)-1)]
		}
		win := s.wails.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:          fmt.Sprintf("capture-%d", i),
			Title:         "Transdict",
			Frameless:     true,
			AlwaysOnTop:   true,
			DisableResize: true,
			// Hidden until the page has its frozen frame: our own Show() used to
			// land before the webview had painted anything, and a WKWebView that
			// has not painted is white.
			Hidden:           true,
			X:                scr.Bounds.X,
			Y:                scr.Bounds.Y,
			Width:            scr.Bounds.Width,
			Height:           scr.Bounds.Height,
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: application.NewRGBA(0, 0, 0, 255),
			URL:              fmt.Sprintf("/#/capture/%d", frame.Index),
			Mac: application.MacWindow{
				// The screenshot must cover the menu bar and Dock, so a
				// floating window is not enough — those live below it.
				WindowLevel:        application.MacWindowLevelScreenSaver,
				CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary | application.MacWindowCollectionBehaviorIgnoresCycle,
				CornerType:         application.MacWindowCornerTypeSquare,
				DisableShadow:      true,
			},
			Windows: application.WindowsWindow{
				// The overlay is a moment, not a window: it must not flash a
				// taskbar button on the way to the selection.
				HiddenOnTaskbar: true,
			},
		})
		// Wails colours the NSWindow, but the webview sits on top of it with its
		// own opaque white surface until the page paints. Painting that surface
		// black keeps even the hidden window's first visible frame dark. AppKit
		// wants the main thread.
		application.InvokeSync(func() { paintOverlayBackdrop(win) })
		overlays = append(overlays, captureOverlay{frame: frame.Index, win: win})
	}

	s.mu.Lock()
	s.session = id
	s.overlays = overlays
	s.mu.Unlock()

	// The overlay reveals itself through CaptureReady once its frame has
	// decoded. The marquee listens for Esc and Return in the page, and a
	// background app only gets the keyboard once it is frontmost.
	time.AfterFunc(captureRevealTimeout, func() { s.revealSession(id) })
	return nil
}

// revealSession shows every overlay of one session at once, for a page that
// never reported ready: a frozen screen that cannot be dismissed is worse than
// whatever the page looks like.
func (s *Service) revealSession(id string) {
	s.mu.Lock()
	if s.session != id {
		s.mu.Unlock()
		return
	}
	wins := make([]*application.WebviewWindow, 0, len(s.overlays))
	for _, o := range s.overlays {
		wins = append(wins, o.win)
	}
	s.mu.Unlock()

	for _, w := range wins {
		w.Show()
		w.Focus()
	}
	application.InvokeAsync(dock.Activate)
}

// CaptureReady is the overlay reporting that its frozen frame has decoded and
// the window is ready to be seen. Showing only now means the first frame on
// screen is the screenshot, not the webview's white backdrop. The marquee has
// to hear Esc and Return, and a background app only gets the keyboard if it
// asks; this is the user's own shortcut, so taking focus is the promise rather
// than an interruption.
func (s *Service) CaptureReady(frameIndex int) {
	s.mu.Lock()
	session := s.session
	wins := make([]*application.WebviewWindow, 0, 1)
	for _, o := range s.overlays {
		if o.frame == frameIndex {
			wins = append(wins, o.win)
		}
	}
	s.mu.Unlock()
	if session == "" {
		return
	}

	for _, w := range wins {
		w.Show()
		w.Focus()
	}
	application.InvokeAsync(dock.Activate)
}

// StartSilent begins the same marquee flow but ends in the pasteboard: the crop
// is recognized with the 静音 OCR services and copied, and no window opens.
// It is the fastest way to lift text out of anything — a PDF, a video, a canvas.
func (s *Service) StartSilent() error {
	if err := s.start(); err != nil {
		s.fail(err)
		return err
	}
	s.mu.Lock()
	s.silent = true
	s.mu.Unlock()
	return nil
}

// StartOCR is 截图 OCR: the same marquee as 截图翻译, but the result is the OCR
// 窗口 — the picture with the lines found in it and the merged text — instead of
// a translation. EasyDict's OCR menu entries all end in that preview.
func (s *Service) StartOCR() error {
	if err := s.start(); err != nil {
		s.fail(err)
		return err
	}
	s.mu.Lock()
	s.ocrOnly = true
	s.mu.Unlock()
	return nil
}

// FrameDataURL hands the overlay window its frozen display image, as the small
// display copy rather than the lossless frame that OCR reads.
func (s *Service) FrameDataURL(frameIndex int) (string, error) {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == "" {
		return "", errors.New("没有进行中的截图")
	}
	data, ok := s.shots.Preview(session, frameIndex)
	if !ok {
		return "", fmt.Errorf("缺少显示器 %d 的截图", frameIndex)
	}
	return query.DataURL(data), nil
}

// Commit receives a selection already converted to image pixel coordinates.
func (s *Service) Commit(frameIndex int, x, y, w, h int) error {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == "" {
		return errors.New("没有进行中的截图")
	}

	png, ok := s.shots.Frame(session, frameIndex)
	if !ok {
		return errors.New("截图已过期，请重新截取")
	}

	// The overlay comes down before the crop is decoded: cropping a Retina PNG
	// costs ~70ms, and leaving the frozen screen up for it made the release look
	// like it had not registered.
	s.mu.Lock()
	silent := s.silent
	ocrOnly := s.ocrOnly
	s.silent = false
	s.ocrOnly = false
	s.teardownLocked()
	s.mu.Unlock()

	crop, err := capture.Crop(png, x, y, w, h)
	if err != nil {
		return err
	}

	s.shots.PutCrop(session, crop)
	s.mu.Lock()
	s.lastCrop = crop
	s.mu.Unlock()

	// 上一次截图区域: the next overlay offers D, which reuses this rectangle
	// instead of asking for the same drag again.
	if path, err := lastAreaPath(); err == nil {
		writeArea(path, CaptureArea{Frame: frameIndex, X: x, Y: y, W: w, H: h})
	}

	if silent {
		go s.runSilent(crop)
		return nil
	}
	if ocrOnly {
		// 截图 OCR is the OCR 窗口, not a translation: the picture and the lines
		// found in it open side by side, EasyDict's preview.
		go s.runOCRPreview(crop)
		return nil
	}
	// The answer opens beside the crop, EasyDict's way: 截图翻译 is read where
	// the picture was taken, and a panel on another display would be a window
	// the user did not ask for.
	return s.Ask(query.Request{Context: settings.ContextScreenshot, Mode: "screenshot", Image: crop})
}

// MaskAndRecognize 遮挡后重识别: pixelates the regions the user painted over the
// crop, then runs 截图翻译 again on what is left. A selection routinely carries
// something that should not leave the machine — a token, an address, a colleague's
// name — and covering it is quicker than retyping the rest. The masked picture
// becomes the crop, so the panel shows what was actually sent.
func (s *Service) MaskAndRecognize(rects []capture.MaskRect) error {
	s.mu.Lock()
	crop := s.lastCrop
	s.mu.Unlock()
	if len(crop) == 0 {
		return errors.New("截图已过期，请重新截取")
	}
	masked, err := capture.Mosaic(crop, rects, capture.MosaicCell)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.lastCrop = masked
	s.mu.Unlock()
	s.run(query.Request{Context: settings.ContextScreenshot, Mode: "screenshot", Image: masked})
	return nil
}

// runSilent recognizes and copies without presenting anything. EasyDict's
// silent screenshot OCR answers with only the ✓ toast at the pointer, so the
// copy is confirmed where the selection ended.
func (s *Service) runSilent(crop []byte) {
	cfg := s.cfg.Get()
	req := query.Request{Context: settings.ContextSilentOCR, Mode: "silent", Image: crop,
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang}
	out := s.engine.OCRStage(context.Background(), cfg, req)
	text := query.OCRText(cfg, out)
	if text == "" {
		msg := "OCR 结果为空。⚠️ 请手动选择识别语言再次尝试。"
		s.busy(settings.ContextSilentOCR, msg)
		ShowToast(l10n.T(msg))
		return
	}
	if s.wails.Clipboard.SetText(text) {
		ShowSuccessToast()
	}
	s.busy(settings.ContextSilentOCR,
		"已复制 "+strconv.Itoa(utf8.RuneCountInString(text))+" 字")
}

func (s *Service) Cancel() {
	s.mu.Lock()
	s.silent = false
	s.teardownLocked()
	s.mu.Unlock()
}

func (s *Service) teardownLocked() {
	for _, o := range s.overlays {
		// Hide before closing. These windows sit at the screen-saver level, and a
		// close that does not take effect leaves a dimmed, unclickable screen
		// behind the result; hiding is synchronous, so the screen comes back even
		// if the destroy is delayed.
		o.win.Hide()
		o.win.Close()
	}
	s.overlays = nil
	s.session = ""
}
