package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/capture"
	"transdict/internal/dock"
	"transdict/internal/query"
	"transdict/internal/settings"
)

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
	overlays := make([]*application.WebviewWindow, 0, len(screens))

	for i, scr := range screens {
		frame, ok := byIndex[i]
		if !ok {
			frame = frames[min(i, len(frames)-1)]
		}
		overlays = append(overlays, s.wails.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             fmt.Sprintf("capture-%d", i),
			Title:            "Transdict",
			Frameless:        true,
			AlwaysOnTop:      true,
			DisableResize:    true,
			X:                scr.Bounds.X,
			Y:                scr.Bounds.Y,
			Width:            scr.Bounds.Width,
			Height:           scr.Bounds.Height,
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: application.NewRGBA(0, 0, 0, 255),
			URL:              fmt.Sprintf("/#/capture/%d", frame.Index),
			Mac: application.MacWindow{
				// The screenshot must cover the menu bar and Dock, so a
				// floating panel is not enough — those live above it.
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
		}))
	}

	s.mu.Lock()
	s.session = id
	s.overlays = overlays
	s.mu.Unlock()

	for _, w := range overlays {
		w.Show()
		w.Focus()
	}
	// The marquee has to hear Esc and Return, and a background app only gets the
	// keyboard if it asks. This is the user's own shortcut, so taking focus is
	// the promise rather than an interruption.
	application.InvokeAsync(dock.Activate)
	return nil
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
	s.silent = false
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

	if silent {
		go s.runSilent(crop)
		return nil
	}
	runCtx, ticket, cancel := s.beginRun(settings.ContextScreenshot)
	go func() {
		defer cancel()
		s.execute(runCtx, query.Request{Context: settings.ContextScreenshot, Mode: "screenshot", Image: crop}, ticket)
	}()
	return nil
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
	return s.Ask(query.Request{Context: settings.ContextScreenshot, Mode: "screenshot", Image: masked})
}

// runSilent recognizes and copies without presenting anything. The one visible
// trace is the pasteboard plus a toast in the main window when it happens to be
// open, which is the point of 静音: no panel to dismiss.
func (s *Service) runSilent(crop []byte) {
	cfg := s.cfg.Get()
	req := query.Request{Context: settings.ContextSilentOCR, Mode: "silent", Image: crop,
		SourceLang: cfg.SourceLang, TargetLang: cfg.TargetLang}
	out := s.engine.OCRStage(context.Background(), cfg, req)
	text := query.PickText(out)
	if text == "" {
		s.busy(settings.ContextSilentOCR, "未识别到文字")
		return
	}
	s.wails.Clipboard.SetText(text)
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
		o.Hide()
		o.Close()
	}
	s.overlays = nil
	s.session = ""
}
