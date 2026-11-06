package gui

import (
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"transdict/internal/capture"
	"transdict/internal/dock"
	"transdict/internal/history"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
)

// present surfaces the main window; the app keeps running without one, so the
// global shortcut can summon it again without a cold start.
//
// The app usually runs without a Dock tile, which makes it a background process:
// ordering the window front is not enough, the previous app would keep the
// keyboard. Activation is asked for after the ordering, so the handover — which
// AppKit completes a turn later — takes the window that is already on screen.
func (s *Service) present() {
	s.show(WindowMain, true)
	application.InvokeAsync(dock.Activate)
}

// ownChrome reports whether the header draws the window controls. macOS keeps
// the system traffic lights, so only Windows drops the native frame and lets
// the three document windows render minimize and maximize themselves.
func ownChrome() bool { return runtime.GOOS == "windows" }

// NewMainWindow builds the translation window. It is a recipe rather than a
// one-off because Wails destroys a window the user closes, and the shortcut has
// to keep working after that.
//
// It stays narrow on purpose: this window only receives queries, so a second
// column would be empty space most of the time. Preferences live in their own
// window (NewSettingsWindow) the way a Mac app separates a document from its
// settings.
func NewMainWindow(app *application.App, cfg *settings.Store) application.Window {
	width, height := 480, 600
	frame := cfg.Get()
	if frame.MainW >= 420 && frame.MainH >= 420 {
		width, height = frame.MainW, frame.MainH
	}
	opts := application.WebviewWindowOptions{
		Name:      WindowMain,
		Title:     "Transdict",
		Width:     width,
		Height:    height,
		MinWidth:  420,
		MinHeight: 420,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			// HiddenInset installs a toolbar, and a toolbar draws the window
			// title even with titleVisibility hidden — which lands on top of
			// the traffic lights at this width.
			TitleBar: application.MacTitleBarHidden,
			Backdrop: application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			// The Windows reading of the same switch: a taskbar button is per
			// window, so it is fixed when the window is built and a toggle
			// takes effect on the next launch.
			HiddenOnTaskbar: !cfg.Get().ShowInDock,
		},
		// A stored 置顶 must hold from the first frame, not from the first
		// result: the window is often opened to compare with what is behind it.
		AlwaysOnTop:      cfg.Get().Pinned,
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/panel",
		InitialPosition:  application.WindowCentered,
		Frameless:        ownChrome(),
	}
	// Placement has to ride on the options: a window's native view only exists
	// once the app starts, so SetBounds before that is dropped and the default
	// centering wins.
	if bounds, ok := visibleFrame(app, frame); ok {
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = bounds.X, bounds.Y
	}
	w := app.Window.NewWithOptions(opts)
	rememberFrame(w, cfg, frameMain)
	return w
}

// The three resizable windows each keep their own frame: the translation window
// is narrow, the settings form is wide, and the history list wants room for a
// service column. A shared frame would make each one open at a size it cannot
// use.
const (
	frameMain     = "main"
	frameSettings = "settings"
	frameHistory  = "history"
)

func savedFrame(c settings.Config, name string) (x, y, w, h int) {
	switch name {
	case frameSettings:
		return c.SettingsX, c.SettingsY, c.SettingsW, c.SettingsH
	case frameHistory:
		return c.HistoryX, c.HistoryY, c.HistoryW, c.HistoryH
	default:
		return c.MainX, c.MainY, c.MainW, c.MainH
	}
}

func storeFrame(c *settings.Config, name string, b application.Rect) {
	switch name {
	case frameSettings:
		c.SettingsX, c.SettingsY, c.SettingsW, c.SettingsH = b.X, b.Y, b.Width, b.Height
	case frameHistory:
		c.HistoryX, c.HistoryY, c.HistoryW, c.HistoryH = b.X, b.Y, b.Width, b.Height
	default:
		c.MainX, c.MainY, c.MainW, c.MainH = b.X, b.Y, b.Width, b.Height
	}
}

// applySavedFrame puts a remembered frame on freshly built options. It reports
// false when there is nothing usable, which is when the caller centers instead.
func applySavedFrame(app *application.App, c settings.Config, name string, minW, minH int,
	opts *application.WebviewWindowOptions) bool {
	x, y, w, h := savedFrame(c, name)
	if w < minW || h < minH {
		return false
	}
	r := application.Rect{X: x, Y: y, Width: w, Height: h}
	if !frameReachable(app, r) {
		return false
	}
	opts.InitialPosition = application.WindowXY
	opts.X, opts.Y = x, y
	opts.Width, opts.Height = w, h
	return true
}

// visibleFrame returns the last frame when a draggable strip of it still lands
// on a connected display. A window left on an unplugged monitor would otherwise
// reopen off-screen, which reads as "the app did not start".
func visibleFrame(app *application.App, c settings.Config) (application.Rect, bool) {
	if c.MainW < 420 || c.MainH < 420 {
		return application.Rect{}, false
	}
	r := application.Rect{X: c.MainX, Y: c.MainY, Width: c.MainW, Height: c.MainH}
	return r, frameReachable(app, r)
}

// frameReachable keeps a window from reopening where nothing can reach it. The
// screen cache can still be empty while windows are being built, so the display
// API is asked directly in that case.
func frameReachable(app *application.App, r application.Rect) bool {
	screens := app.Screen.GetAll()
	if len(screens) == 0 {
		for _, b := range capture.Bounds() {
			if frameOnDisplay(r, application.Rect{X: b.Min.X, Y: b.Min.Y, Width: b.Dx(), Height: b.Dy()}) {
				return true
			}
		}
		return false
	}
	for _, scr := range screens {
		if frameOnDisplay(r, scr.Bounds) {
			return true
		}
	}
	return false
}

// frameOnDisplay reports whether enough of the frame is on the display to grab
// and drag back: the title strip and some of the body.
func frameOnDisplay(r, b application.Rect) bool {
	return r.X+r.Width > b.X+80 && r.X < b.X+b.Width-80 &&
		r.Y+r.Height > b.Y+44 && r.Y < b.Y+b.Height-44
}

// rememberFrame writes the window's frame back after a move or resize. Moving
// fires a burst of events, so the write waits for the pointer to settle; the
// store's watchers (the tray menu among them) then run once, not per pixel.
func rememberFrame(w application.Window, cfg *settings.Store, name string) {
	var mu sync.Mutex
	var pending *time.Timer
	save := func() {
		b := w.Bounds()
		x, y, width, height := savedFrame(cfg.Get(), name)
		// A window created with size N reports N-1: the frame and the content
		// size differ by a fraction that rounds down. Saving that reading and
		// requesting it next launch shrank the window by a pixel every start,
		// so a difference below the visible threshold is not a user change.
		if b.Width < 420 || b.Height < 420 {
			return
		}
		if near(x, b.X) && near(y, b.Y) && near(width, b.Width) && near(height, b.Height) {
			return
		}
		if _, err := cfg.Update(func(c *settings.Config) {
			storeFrame(c, name, b)
		}); err != nil {
			log.Printf("transdict: 记录窗口位置失败: %v", err)
		}
	}
	queue := func() {
		mu.Lock()
		defer mu.Unlock()
		if pending != nil {
			pending.Stop()
		}
		pending = time.AfterFunc(500*time.Millisecond, save)
	}
	w.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { queue() })
	w.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) { queue() })
}

// near reports whether two frame numbers describe the same frame within the
// rounding a hidden title bar and DIP conversion introduce.
func near(a, b int) bool {
	d := a - b
	return d >= -2 && d <= 2
}

// The title the window switcher shows is written once at creation, while the
// page is rebuilt on every language change; these keep the two in step.
func settingsWindowTitle() string { return l10n.T("Transdict 设置") }

func historyWindowTitle() string { return l10n.T("Transdict 历史记录") }

// NewSettingsWindow is the preferences window: a sidebar plus a scrolling pane,
// sized for forms rather than for a paragraph of translated text.
func NewSettingsWindow(app *application.App, cfg *settings.Store) application.Window {
	opts := application.WebviewWindowOptions{
		Name:      WindowSettings,
		Title:     settingsWindowTitle(),
		Width:     860,
		Height:    640,
		MinWidth:  740,
		MinHeight: 520,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			TitleBar:                application.MacTitleBarHidden,
			Backdrop:                application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: !cfg.Get().ShowInDock,
		},
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/settings",
		Frameless:        ownChrome(),
	}
	saved := applySavedFrame(app, cfg.Get(), frameSettings, 740, 520, &opts)
	w := app.Window.NewWithOptions(opts)
	if !saved {
		w.Center()
	}
	rememberFrame(w, cfg, frameSettings)
	return w
}

// NewHistoryWindow shows the recorded queries at a width a list needs; the
// translation window is too narrow for the service column.
func NewHistoryWindow(app *application.App, cfg *settings.Store) application.Window {
	opts := application.WebviewWindowOptions{
		Name:      WindowHistory,
		Title:     historyWindowTitle(),
		Width:     780,
		Height:    620,
		MinWidth:  660,
		MinHeight: 480,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			TitleBar:                application.MacTitleBarHidden,
			Backdrop:                application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: !cfg.Get().ShowInDock,
		},
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/history",
		Frameless:        ownChrome(),
	}
	saved := applySavedFrame(app, cfg.Get(), frameHistory, 660, 480, &opts)
	w := app.Window.NewWithOptions(opts)
	if !saved {
		w.Center()
	}
	rememberFrame(w, cfg, frameHistory)
	return w
}

func (s *Service) show(name string, lift bool) {
	w := s.windowFor(name)
	if w == nil {
		return
	}
	w.Show()
	w.UnMinimise()
	if lift {
		// Re-applying the level is the only portable way to lift a window that
		// was ordered out from under a fullscreen app. A pinned translation
		// window must stop there instead of dropping back to the normal level.
		w.SetAlwaysOnTop(true)
		if name != WindowMain || !s.cfg.Get().Pinned {
			w.SetAlwaysOnTop(false)
		}
	}
	w.Focus()
}

// windowFor returns the named window, building it when it is not there.
// Closing a window in Wails destroys it rather than hiding it, so without this
// the first red-button click would cost the shortcut its target for the rest
// of the run.
func (s *Service) windowFor(name string) application.Window {
	if w, ok := s.wails.Window.GetByName(name); ok {
		return w
	}
	switch name {
	case WindowMain:
		return NewMainWindow(s.wails, s.cfg)
	case WindowSettings:
		return NewSettingsWindow(s.wails, s.cfg)
	case WindowHistory:
		return NewHistoryWindow(s.wails, s.cfg)
	case WindowFloat:
		w, h := s.floatSize()
		return s.ensure(WindowFloat, "/#/float", w, h, true)
	case WindowMini:
		return s.ensure(WindowMini, "/#/mini", 380, 110, true)
	}
	return nil
}

func (s *Service) window(name string) application.Window {
	w, ok := s.wails.Window.GetByName(name)
	if !ok {
		return nil
	}
	return w
}

// SyncWindowTitles repaints the native titles after the interface language
// changes. Only windows that are already open are touched; one created later
// reads the language in its constructor.
//
//wails:ignore
func (s *Service) SyncWindowTitles() {
	if w := s.window(WindowSettings); w != nil {
		w.SetTitle(settingsWindowTitle())
	}
	if w := s.window(WindowHistory); w != nil {
		w.SetTitle(historyWindowTitle())
	}
}

// target maps a query context to the window that should render it. Wails v3
// has no per-window event channel, so every window receives the result and the
// frontend keeps the ones addressed to it.
func target(context string) string {
	switch context {
	case settings.ContextFloating, settings.ContextODict:
		return WindowFloat
	case settings.ContextMini:
		return WindowMini
	default:
		return WindowMain
	}
}

// ensure creates a utility window on first use. Floating and mini windows are
// lazy because most sessions never invoke 划词, and an empty hidden window on
// every display would be visible clutter in Mission Control.
// floatSize is the panel a selection answer needs: wide enough for a paragraph,
// short enough to stay out of the way.
func (s *Service) floatSize() (int, int) {
	cfg := s.cfg.Get()
	w, h := 420, 260
	if cfg.MiniFont > 0 {
		h += (cfg.MiniFont - 15) * 8
		if h < 180 {
			h = 180
		}
	}
	return w, h
}

func (s *Service) ensure(name, url string, width, height int, top bool) application.Window {
	if w := s.window(name); w != nil {
		return w
	}
	opts := application.WebviewWindowOptions{
		Name:             name,
		Title:            "Transdict",
		Width:            width,
		Height:           height,
		Frameless:        true,
		AlwaysOnTop:      top,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              url,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			InvisibleTitleBarHeight: 0,
			TitleBar:                application.MacTitleBarHidden,
		},
		Windows: application.WindowsWindow{
			// A selection answer is a panel, not a window the user switches to:
			// it should never occupy a taskbar button or an Alt-Tab slot.
			HiddenOnTaskbar: true,
		},
	}
	if name == WindowFloat {
		// The floating panel must survive Spaces and fullscreens: 划词 happens
		// inside other apps, most of which are fullscreen when used.
		opts.Mac.WindowLevel = application.MacWindowLevelFloating
		opts.Mac.CollectionBehavior = application.MacWindowCollectionBehaviorCanJoinAllSpaces |
			application.MacWindowCollectionBehaviorFullScreenAuxiliary |
			application.MacWindowCollectionBehaviorIgnoresCycle
		opts.Mac.DisableShadow = false
	}
	w := s.wails.Window.NewWithOptions(opts)
	if name == WindowFloat {
		w.Center()
	}
	return w
}

// placeFloat puts the panel beside the pointer, clamped to the display the
// pointer is on. A selection happens under the cursor, so that is where the
// answer belongs.
func (s *Service) placeFloat(width, height int) {
	win := s.window(WindowFloat)
	if win == nil {
		return
	}
	cursor := capture.Cursor()
	if cursor.X == 0 && cursor.Y == 0 {
		win.Center()
		return
	}
	scr := s.wails.Screen.ScreenNearestPhysicalPoint(application.Point{X: cursor.X, Y: cursor.Y})
	if scr == nil {
		win.Center()
		return
	}
	// Wails positions windows in dip (logical) coordinates.
	dip := s.wails.Screen.PhysicalToDipPoint(application.Point{X: cursor.X, Y: cursor.Y})
	bounds := s.wails.Screen.PhysicalToDipRect(scr.PhysicalBounds)

	x, y := dip.X+14, dip.Y+18
	if maxX := bounds.X + bounds.Width - width - 8; x > maxX {
		x = maxX
	}
	if maxY := bounds.Y + bounds.Height - height - 8; y > maxY {
		y = maxY
	}
	if x < bounds.X+8 {
		x = bounds.X + 8
	}
	if y < bounds.Y+8 {
		y = bounds.Y + 8
	}
	win.SetPosition(x, y)
	win.SetSize(width, height)
}

func (s *Service) snapshot() *query.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	r := *s.last
	r.OCR = append(r.OCR[:0:0], s.last.OCR...)
	r.Translate = append(r.Translate[:0:0], s.last.Translate...)
	r.Dict = append(r.Dict[:0:0], s.last.Dict...)
	return &r
}

// emitPartial shows the recognized text before the translation services answer.
// It skips history, auto-copy and s.last on purpose: the final emit writes the
// record once, and a half-answer must never become the thing 重试 retries.
func (s *Service) emitPartial(res *query.Response) {
	s.wails.Event.Emit(EventResult, localizedResponse(*res))
	switch target(res.Context) {
	case WindowMain:
		s.present()
	case WindowFloat:
		s.show(WindowFloat, false)
	case WindowMini:
		s.show(WindowMini, false)
	}
}

// emit publishes a result and records it. Recording happens here rather than in
// the caller so a retry that replaces one service's outcome still lands once.
func (s *Service) emit(res *query.Response) {
	s.mu.Lock()
	s.last = res
	if target(res.Context) == WindowMain {
		s.panelLast = res
	}
	s.mu.Unlock()

	s.wails.Event.Emit(EventResult, localizedResponse(*res))
	s.record(res)
	s.autoCopy(res)

	switch target(res.Context) {
	case WindowMain:
		s.present()
	case WindowFloat:
		s.show(WindowFloat, false)
	case WindowMini:
		s.show(WindowMini, false)
	}
}

func (s *Service) record(res *query.Response) {
	if s.history == nil || !wantsHistory(s.cfg.Get(), res) {
		return
	}
	if err := s.history.Put(entryOf(res)); err != nil {
		log.Printf("transdict: 记录历史失败: %v", err)
	}
}

// wantsHistory is the 记录查询历史 switch plus the two answers that never make
// a row. Off means the text never reaches the file, not that the list is
// cleaned afterwards. A progress update is not an answer: the services still
// running would be missing from the row, and the last partial to arrive would
// overwrite the finished one under the same id.
func wantsHistory(cfg settings.Config, res *query.Response) bool {
	return cfg.SaveHistory && res != nil && res.SourceText != "" && !res.Partial
}

// entryOf turns one answer into the list row. Target is the first translation
// the panel shows, not the last service to answer: a history row that quoted a
// fallback translator would disagree with the card the user just read.
func entryOf(res *query.Response) history.Entry {
	entry := history.Entry{
		ID:         res.ID,
		Kind:       res.Mode,
		Context:    res.Context,
		Source:     res.SourceText,
		Detected:   res.Detected,
		TargetLang: res.TargetLang,
		CreatedAt:  nowUnix(),
		ElapsedMs:  res.ElapsedMs,
	}
	for _, t := range res.Translate {
		if t.Error == "" && t.Text != "" {
			if entry.Target == "" {
				entry.Target = t.Text
			}
			entry.Services = append(entry.Services, t.Label)
		}
	}
	for _, d := range res.Dict {
		if d.Error == "" {
			entry.Services = append(entry.Services, d.Label)
		}
	}
	return entry
}

// autoCopy is the 自动复制译文 switch: the first usable translation goes to the
// pasteboard the moment a result lands. Two contexts are left alone because they
// already own the clipboard — 静音 OCR puts the recognized text there and 翻译替换
// pastes the answer into the app being edited — and a third write would only
// race them.
func (s *Service) autoCopy(res *query.Response) {
	if res == nil || !s.cfg.Get().AutoCopy {
		return
	}
	switch res.Context {
	case settings.ContextSilentOCR, settings.ContextReplace:
		return
	}
	answer := query.FirstTranslation(res)
	if answer == "" {
		return
	}
	if !s.wails.Clipboard.SetText(answer) {
		log.Print("transdict: 自动复制失败")
	}
}

// fail shows a stage failure in the panel instead of only the log: the shortcut
// often fires while the window is hidden, and a bare log line would look like
// the keypress did nothing at all.
func (s *Service) fail(err error) {
	s.emit(&query.Response{Context: settings.ContextMain,
		TargetLang: s.cfg.Get().TargetLang, Error: err.Error()})
}

// command tells an already-open window to switch views. The hotkey handlers run
// in Go, so the only way to focus the input tab is to ask the webview for it.
func (s *Service) command(name string) {
	s.wails.Event.Emit(EventCommand, name)
}

// OpenInput shows the translation window with the input box focused.
// OpenMain shows the panel without switching focus, which is what the tray's
// 打开主窗口 means.
func (s *Service) OpenMain() { s.present() }

// OpenMainDetail shows the answer the floating panel is displaying in the main
// window. 详情 means "this answer, larger"; presenting the panel on whatever it
// answered last would show a different query.
func (s *Service) OpenMainDetail() {
	s.mu.Lock()
	detail, ok := detailOf(s.last)
	source := ""
	if ok {
		s.panelLast = detail
		source = target(s.last.Context)
	}
	s.mu.Unlock()
	if ok {
		s.wails.Event.Emit(EventResult, localizedResponse(*detail))
		// 详情 hands the answer over: leaving the floating or mini copy on top
		// would cover the window that now shows the same thing, larger.
		if w := s.window(source); w != nil {
			w.Hide()
		}
	}
	s.present()
}

// detailOf promotes a floating, 鼠标取词 or 迷你窗 answer into something the main
// panel renders. A query that already belongs to the panel is left alone, and
// so is an empty store: there is nothing to show larger.
func detailOf(last *query.Response) (*query.Response, bool) {
	if last == nil {
		return nil, false
	}
	switch target(last.Context) {
	case WindowFloat, WindowMini:
	default:
		return nil, false
	}
	d := *last
	d.Context = settings.ContextMain
	d.OCR = append(d.OCR[:0:0], last.OCR...)
	d.Translate = append(d.Translate[:0:0], last.Translate...)
	d.Dict = append(d.Dict[:0:0], last.Dict...)
	return &d, true
}

func (s *Service) OpenInput() {
	s.present()
	s.command("input")
}

// OpenSettings and OpenHistory each raise their own window. Preferences are not
// a tab of the translation window: it is narrow by design, and a sidebar plus
// forms would squeeze the paragraph the user is reading.
func (s *Service) OpenSettings() { s.show(WindowSettings, true) }

func (s *Service) OpenHistory() { s.show(WindowHistory, true) }

// TogglePin flips 置顶 and reports the new state. The store is the one place
// both the window button and the tray checkbox read, so the two never disagree.
func (s *Service) TogglePin() bool {
	next := false
	if _, err := s.cfg.Update(func(c *settings.Config) {
		c.Pinned = !c.Pinned
		next = c.Pinned
	}); err != nil {
		log.Printf("transdict: 置顶设置写入失败: %v", err)
		return s.cfg.Get().Pinned
	}
	return next
}

// applyPin gives the translation window its stored level. Settings are watched
// in Attach, so a change made anywhere — button, tray, config import — lands
// here and the window agrees with the menu.
func (s *Service) applyPin(pinned bool) {
	if w := s.window(WindowMain); w != nil {
		w.SetAlwaysOnTop(pinned)
	}
}

func (s *Service) syncPin(next settings.Config) { s.applyPin(next.Pinned) }

func (s *Service) CloseSettings() {
	if w := s.window(WindowSettings); w != nil {
		w.Hide()
	}
}

func (s *Service) CloseHistory() {
	if w := s.window(WindowHistory); w != nil {
		w.Hide()
	}
}

// The progress line is Go's own wording, so it is translated on the way out;
// a composed sentence with no entry simply stays Chinese.
func (s *Service) busy(context, text string) {
	s.wails.Event.Emit(EventBusy, Busy{Target: target(context), Text: l10n.T(text)})
}

// ClosePanel hides the window without ending the session.
func (s *Service) ClosePanel() {
	if w := s.window(WindowMain); w != nil {
		w.Hide()
	}
}

func (s *Service) CloseFloat() {
	if w := s.window(WindowFloat); w != nil {
		w.Hide()
	}
}

// CloseMini hides the mini window. Hiding rather than closing keeps its webview
// warm, so the next 迷你窗 shortcut is instant.
func (s *Service) CloseMini() {
	if w := s.window(WindowMini); w != nil {
		w.Hide()
	}
}

func (s *Service) CopyText(text string) bool {
	return s.wails.Clipboard.SetText(text)
}

func nowUnix() int64 { return time.Now().Unix() }
