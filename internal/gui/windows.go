package gui

import (
	"errors"
	"log"
	"math"
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
	// The height is content-driven once the panel paints (FitMain), so a short
	// saved height is a real frame, not a damaged one.
	if frame.MainW >= 420 && frame.MainH >= 120 {
		width, height = frame.MainW, frame.MainH
	}
	opts := application.WebviewWindowOptions{
		Name:     WindowMain,
		Title:    "Transdict",
		Width:    width,
		Height:   height,
		MinWidth: 420,
		// EasyDict lets the query window drop to a one-row shell when the answer
		// is empty; the old 420 floor kept a large pane of glass around it.
		MinHeight: 120,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 34,
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
			// Frameless removes the native caption, so hit testing has to come
			// from the page's --wails-non-client-region rectangles. Wails only
			// turns that tracking on for composition-hosted windows; without
			// it the caption strip neither drags nor runs the custom buttons.
			WebView2CompositionHosting: true,
		},
		// A stored 置顶 must hold from the first frame, not from the first
		// result: the window is often opened to compare with what is behind it.
		AlwaysOnTop:      cfg.Get().Pinned,
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/panel",
		InitialPosition:  application.WindowCentered,
		// The query window is EasyDict's borderless panel: a frameless rounded
		// card with its own caption strip. Settings and history keep native chrome
		// because they are ordinary documents with close and zoom controls.
		Frameless: true,
		// 启动后隐藏主窗口: the first window is created hidden, and every later
		// OpenMain shows it. A rebuild after the user closed it is immediately
		// shown by show(), so the flag only changes what a launch looks like.
		Hidden: frame.HideMainWindow,
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

// The two resizable windows each keep their own frame: the translation window
// is narrow and the settings form is wide. A shared frame would make each one
// open at a size it cannot use.
const (
	frameMain     = "main"
	frameSettings = "settings"
)

func savedFrame(c settings.Config, name string) (x, y, w, h int) {
	switch name {
	case frameSettings:
		return c.SettingsX, c.SettingsY, c.SettingsW, c.SettingsH
	default:
		return c.MainX, c.MainY, c.MainW, c.MainH
	}
}

func storeFrame(c *settings.Config, name string, b application.Rect) {
	switch name {
	case frameSettings:
		c.SettingsX, c.SettingsY, c.SettingsW, c.SettingsH = b.X, b.Y, b.Width, b.Height
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
	if c.MainW < 420 || c.MainH < 120 {
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
		if b.Width < 420 || b.Height < minFrameHeight(name) {
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

// minFrameHeight is the "this is a real frame, not a half-built window" floor.
// The query window is content-sized and can be a short shell; the two document
// windows keep the height their forms need.
func minFrameHeight(name string) int {
	if name == frameMain {
		return 120
	}
	return 420
}

// The title the window switcher shows is written once at creation, while the
// page is rebuilt on every language change; these keep the two in step.
func settingsWindowTitle() string { return l10n.T("Transdict 设置") }

func ocrWindowTitle() string { return l10n.T("OCR 预览") }

func logsWindowTitle() string { return l10n.T("调试日志") }

// EasyDict's preferences window is one fixed width with a tab bar across the
// top. The height is fixed too: switching tabs must not make the window jump,
// even though 关于 and 隐私 are shorter forms than 高级.
const (
	settingsWindowWidth       = 960
	settingsWindowHeight      = 720
	settingsWindowDefaultPage = "general"
)

// EasyDict's Settings scene retitles the window with the selected tab (关于,
// 通用, …), so the window switcher names the page rather than the app.
var settingsTabNames = map[string]string{
	"general":   "通用",
	"services":  "服务",
	"favorites": "收藏",
	"disabled":  "禁止名单",
	"shortcuts": "快捷键",
	"advanced":  "高级",
	"privacy":   "隐私",
	"about":     "关于",
}

func settingsTabTitle(page string) string {
	name, ok := settingsTabNames[page]
	if !ok {
		name = settingsTabNames[settingsWindowDefaultPage]
	}
	return l10n.T(name)
}

// NewSettingsWindow is the preferences window: a tab bar over a scrolling pane,
// sized for forms rather than for a paragraph of translated text.
func NewSettingsWindow(app *application.App, cfg *settings.Store) application.Window {
	opts := application.WebviewWindowOptions{
		Name:          WindowSettings,
		Title:         settingsTabTitle(settingsWindowDefaultPage),
		Width:         settingsWindowWidth,
		Height:        settingsWindowHeight,
		DisableResize: true,
		// EasyDict disables the zoom button on its preferences window; the size
		// is fixed, so a fullscreen shelf of empty pane is a dead end.
		FullscreenButtonState: application.ButtonDisabled,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 34,
			TitleBar:                application.MacTitleBarHidden,
			Backdrop:                application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:            !cfg.Get().ShowInDock,
			WebView2CompositionHosting: ownChrome(),
		},
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/settings",
		Frameless:        ownChrome(),
	}
	// Only the position survives a relaunch: one size serves every tab, so a
	// saved frame can never reopen the window at a page-specific height.
	saved := applySavedFrame(app, cfg.Get(), frameSettings, 740, 520, &opts)
	opts.Width, opts.Height = settingsWindowWidth, settingsWindowHeight
	w := app.Window.NewWithOptions(opts)
	if !saved {
		w.Center()
	}
	rememberFrame(w, cfg, frameSettings)
	return w
}

// SetSettingsPage names the window after the tab the page restored, the way a
// SwiftUI settings scene does. The size stays put: switching tabs must not make
// the window jump between heights.
func (s *Service) SetSettingsPage(page string) {
	w := s.window(WindowSettings)
	if w == nil {
		return
	}
	w.SetTitle(settingsTabTitle(page))
}

// NewLogsWindow is EasyDict's 显示调试日志窗口: the tail of the file Setup
// writes, for reading in place instead of opening Finder first.
func NewLogsWindow(app *application.App, cfg *settings.Store) application.Window {
	opts := application.WebviewWindowOptions{
		Name:      WindowLogs,
		Title:     logsWindowTitle(),
		Width:     760,
		Height:    520,
		MinWidth:  520,
		MinHeight: 320,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 34,
			TitleBar:                application.MacTitleBarHidden,
			Backdrop:                application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:            !cfg.Get().ShowInDock,
			WebView2CompositionHosting: ownChrome(),
		},
		BackgroundColour: application.NewRGB(10, 12, 18),
		URL:              "/#/logs",
		Frameless:        ownChrome(),
	}
	w := app.Window.NewWithOptions(opts)
	w.Center()
	return w
}

// NewOCRWindow is EasyDict's OCR preview: a wide window that shows the picture
// that was recognized, where every line was found, and the merged text. It is
// opened by 截图 OCR / 剪贴板 OCR when they finish and by 显示 OCR 窗口; its size
// is three quarters of the display, anchored at the top-left corner the way
// OCRWindowManager.calculateWindowFrame puts it.
func NewOCRWindow(app *application.App, cfg *settings.Store) application.Window {
	x, y, width, height := ocrWindowFrame(app)
	opts := application.WebviewWindowOptions{
		Name:   WindowOCR,
		Title:  ocrWindowTitle(),
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
		// The three panes need 300+250+280 plus two 12pt gaps and the page's
		// 32pt of side padding = 886; a narrower floor overflows the grid.
		MinWidth: 890,
		// Zero InitialPosition means "centered", so the top-left anchor has to
		// be asked for explicitly or the 75% frame lands in the middle.
		InitialPosition: application.WindowXY,
		MinHeight:       560,
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:            !cfg.Get().ShowInDock,
			WebView2CompositionHosting: true,
		},
		// EasyDict builds this window with .borderless and a 12pt rounded sheet,
		// so the caption is the page's own: no traffic lights, a pin at the left
		// and the title in the middle.
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/#/ocr",
		Frameless:        true,
	}
	return app.Window.NewWithOptions(opts)
}

// ocrWindowFrame sizes the OCR window at 75% of the primary display's work
// area, its top-left corner on the work area's top-left. A screen cache with no
// entries yet (the window can be built before the display list is ready) falls
// back to a frame a laptop fits.
func ocrWindowFrame(app *application.App) (x, y, w, h int) {
	scr := app.Screen.GetPrimary()
	if scr == nil {
		if all := app.Screen.GetAll(); len(all) > 0 {
			scr = all[0]
		}
	}
	if scr == nil {
		return 80, 80, 1000, 720
	}
	b := scr.WorkArea
	if b.Width <= 0 || b.Height <= 0 {
		b = scr.Bounds
	}
	return b.X, b.Y, b.Width * 3 / 4, b.Height * 3 / 4
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
	if name == WindowMain {
		// 当窗口显示时自动钉住: the showing itself is what pins the window, so the
		// stored preference becomes the state the titlebar reports. The watcher
		// re-applies the level, but this showing must be on top now.
		if cur := s.cfg.Get(); cur.AutoPin && !cur.Pinned {
			if _, err := s.cfg.Update(func(c *settings.Config) { c.Pinned = true }); err != nil {
				log.Printf("transdict: 自动钉住写入失败: %v", err)
			}
			w.SetAlwaysOnTop(true)
		}
	}
	if name == WindowOCR {
		// The lift drops the level back to normal; a pinned preview has to keep
		// floating above the app its text was read from.
		w.SetAlwaysOnTop(s.OCRWindowPinned())
	}
	if name != WindowIcon {
		// The 划词查询图标 alone stays unfocused: it is a button beside a
		// selection, not a window the user is reading.
		w.Focus()
	}
	if name == WindowIcon {
		// WebKit's content classes only exist once a page has been loaded, so the
		// first-mouse patch is re-applied each time the button comes up.
		acceptFirstMouseInViews()
	}
	switch name {
	case WindowMain, WindowSettings:
		// The percentage rides on the window itself, so a window rebuilt after a
		// close gets its ceiling again without another settings save.
		s.capWindow(w, 420)
	}
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
	case WindowOCR:
		w := NewOCRWindow(s.wails, s.cfg)
		if runtime.GOOS == "darwin" {
			// OCRWindow.windowDidResignKey: without the pin the preview orders
			// itself out as soon as the user is back in the app it was read from.
			w.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
				if s.OCRWindowPinned() {
					return
				}
				w.Hide()
			})
		}
		return w
	case WindowLogs:
		return NewLogsWindow(s.wails, s.cfg)
	case WindowIcon:
		return s.ensure(WindowIcon, "/#/queryicon", queryIconSize, queryIconSize, true)
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
	if w := s.window(WindowOCR); w != nil {
		w.SetTitle(ocrWindowTitle())
	}
	if w := s.window(WindowLogs); w != nil {
		w.SetTitle(logsWindowTitle())
	}
}

// ensure creates the 划词查询图标 on first use. It is lazy because most sessions
// never select text, and a hidden window on every display would be visible
// clutter in Mission Control.
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
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarHidden,
		},
		Windows: application.WindowsWindow{
			// The button is a utility, not a window the user switches to: it
			// should never occupy a taskbar button or an Alt-Tab slot.
			HiddenOnTaskbar: true,
		},
	}
	// The 划词查询图标 must survive Spaces and fullscreens: 划词 happens inside
	// other apps, most of which are fullscreen when used.
	opts.Mac.WindowLevel = application.MacWindowLevelFloating
	opts.Mac.CollectionBehavior = application.MacWindowCollectionBehaviorCanJoinAllSpaces |
		application.MacWindowCollectionBehaviorFullScreenAuxiliary |
		application.MacWindowCollectionBehaviorIgnoresCycle
	// An NSPanel that never activates the app: showing it must not pull the
	// keyboard out of the field the selection came from.
	opts.Mac.WindowClass = application.MacWindowClassPanel
	opts.Mac.PanelPreferences = application.MacPanelPreferences{
		FloatingPanel:          true,
		NonActivating:          true,
		BecomesKeyOnlyIfNeeded: true,
	}
	opts.Mac.DisableShadow = true
	// The button is positioned before it is shown, so it must not appear at
	// the default corner for a frame first.
	opts.Hidden = true
	// The panel is placed at the pointer, which is also where a result at the
	// right edge lands: one level higher keeps the button on top of it.
	opts.Mac.WindowLevel = application.MacWindowLevelModalPanel
	return s.wails.Window.NewWithOptions(opts)
}

// cursorDip is the pointer in the dip points Wails positions windows with.
// capture.Cursor is native: CGEvent returns points on macOS — already dip — and
// physical pixels on Windows, which Wails converts.
func (s *Service) cursorDip() (application.Point, bool) {
	c := capture.Cursor()
	if c.X == 0 && c.Y == 0 {
		return application.Point{}, false
	}
	p := application.Point{X: c.X, Y: c.Y}
	if runtime.GOOS == "darwin" {
		return p, true
	}
	return s.wails.Screen.PhysicalToDipPoint(p), true
}

// clampToDisplay keeps a whole window inside one display; a window taller or
// wider than the display starts at its top-left corner instead of disappearing.
func clampToDisplay(bounds application.Rect, x, y, width, height int) (int, int) {
	if x > bounds.X+bounds.Width-width {
		x = bounds.X + bounds.Width - width
	}
	if y > bounds.Y+bounds.Height-height {
		y = bounds.Y + bounds.Height - height
	}
	if x < bounds.X {
		x = bounds.X
	}
	if y < bounds.Y {
		y = bounds.Y
	}
	return x, y
}

// FitMain sets the translation window's height to what its content reports.
// EasyDict sizes every query window to its answer and leaves only the width to
// the user; the main window grew to 600 regardless of whether the answer was
// one line or a page.
func (s *Service) FitMain(height int) {
	s.fitHeight(WindowMain, height, 120)
}

// fitHeight is the shared "make the window as tall as this much content"
// gesture: keep the top edge, cap at 窗口最大高度百分比, and ease the change.
func (s *Service) fitHeight(name string, content, floor int) {
	win := s.window(name)
	if win == nil || content <= 0 {
		return
	}
	// A hidden window has no content to show, and resizing it would overwrite
	// the frame the user left behind with the height of an answer they never
	// saw. The page fits again when it gains focus.
	if !win.IsVisible() {
		return
	}
	x, y := win.Position()
	w, cur := win.Size()
	maxH := s.maxHeightFor(application.Point{X: x, Y: y}, floor)
	workBottom := 0
	if b, ok := s.displayAt(application.Point{X: x, Y: y}); ok {
		workBottom = b.Y + b.Height
		// The work area leaves the Dock and menu bar out; growing under them
		// would make AppKit shift the window instead of keeping its top edge.
		if scr := s.wails.Screen.ScreenNearestDipPoint(application.Point{X: x, Y: y}); scr != nil && scr.WorkArea.Height > 0 {
			workBottom = scr.WorkArea.Y + scr.WorkArea.Height
		}
	}
	height := fittedHeight(content, floor, maxH, workBottom, y)
	if height == cur {
		return
	}
	s.mu.Lock()
	s.fitGen++
	gen := s.fitGen
	s.mu.Unlock()
	go s.animateWindowHeight(gen, name, w, cur, height)
}

// fittedHeight is the height a content-sized window takes. EasyDict keeps every
// query window's top edge put: when the answer is taller than the space below,
// the window stops at the screen bottom and the answer scrolls inside it
// instead of pushing the window up. A zero workBottom skips the screen test;
// the floor wins over the screen so a window near the bottom still has room.
func fittedHeight(content, floor, maxH, workBottom, top int) int {
	h := content
	if h < floor {
		h = floor
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	if workBottom > 0 {
		if limit := workBottom - 8 - top; h > limit {
			h = limit
		}
	}
	if h < floor {
		h = floor
	}
	return h
}

// animateWindowHeight runs the eased resize. Each step re-reads the window, so
// a panel closed mid-animation stops the loop instead of resizing a dead
// window.
func (s *Service) animateWindowHeight(gen uint64, name string, width, from, to int) {
	// EasyDict's animation is a short easeOut; ten frames at 18ms is 180ms,
	// long enough to read as motion and short enough not to feel lagged behind
	// the answer that caused it.
	const steps = 10
	for i := 1; i <= steps; i++ {
		time.Sleep(18 * time.Millisecond)
		s.mu.Lock()
		stale := s.fitGen != gen
		s.mu.Unlock()
		if stale {
			return
		}
		win := s.window(name)
		if win == nil {
			return
		}
		t := float64(i) / steps
		ease := 1 - math.Pow(1-t, 3)
		h := from + int(math.Round(float64(to-from)*ease))
		win.SetSize(width, h)
	}
}

// maxHeightFor is 窗口最大高度百分比 applied to the display a point sits on.
// The floor is the window's own minimum, the way Easydict never lets the
// percentage make a window smaller than its type allows.
func (s *Service) maxHeightFor(point application.Point, floor int) int {
	maxH := 0
	if b, ok := s.displayAt(point); ok {
		pct := settings.ClampMaxHeightPct(s.cfg.Get().MaxHeightPct)
		maxH = (b.Height - 16) * pct / 100
	}
	if maxH < floor {
		return floor
	}
	return maxH
}

// capWindow gives an open window the current ceiling and brings it down when a
// lowered percentage leaves it taller than allowed.
func (s *Service) capWindow(w application.Window, floor int) {
	x, y := w.Position()
	maxH := s.maxHeightFor(application.Point{X: x, Y: y}, floor)
	w.SetMaxSize(0, maxH)
	if width, h := w.Size(); h > maxH {
		w.SetSize(width, maxH)
	}
}

// applyMaxHeight re-runs the ceiling for every window after the percentage
// changes, so the setting lands in the open windows and not only the next ones.
func (s *Service) applyMaxHeight() {
	for _, name := range []string{WindowMain, WindowSettings} {
		if w := s.window(name); w != nil {
			s.capWindow(w, minFrameHeight(name))
		}
	}
}

// syncMaxHeight is the store watcher behind 窗口最大高度百分比. Every resize
// writes the store, so it compares instead of re-capping on each write.
func (s *Service) syncMaxHeight(c settings.Config) {
	pct := settings.ClampMaxHeightPct(c.MaxHeightPct)
	s.mu.Lock()
	changed := pct != s.maxPct
	s.maxPct = pct
	s.mu.Unlock()
	if changed {
		s.applyMaxHeight()
	}
}

// displayAt returns the display a point falls on, in the top-left point space
// window positions use. The bounds come from the display server rather than the
// screen cache: the cache keeps the mode it saw at launch, so a resolution
// change would leave it letting panels hang off the real screen.
func (s *Service) displayAt(cursor application.Point) (application.Rect, bool) {
	if runtime.GOOS == "darwin" {
		for _, b := range capture.Bounds() {
			if cursor.X >= b.Min.X && cursor.X <= b.Max.X && cursor.Y >= b.Min.Y && cursor.Y <= b.Max.Y {
				return application.Rect{X: b.Min.X, Y: b.Min.Y, Width: b.Dx(), Height: b.Dy()}, true
			}
		}
	}
	if scr := s.wails.Screen.ScreenNearestDipPoint(cursor); scr != nil {
		return scr.Bounds, true
	}
	return application.Rect{}, false
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
	s.present()
}

// emit publishes a result and records it. Recording happens here rather than in
// the caller so a retry that replaces one service's outcome still lands once.
func (s *Service) emit(res *query.Response) {
	s.mu.Lock()
	s.last = res
	if len(res.OCR) > 0 {
		s.lastOCR = res
	}
	s.mu.Unlock()

	s.wails.Event.Emit(EventResult, localizedResponse(*res))
	s.record(res)
	s.saveNotebook(res)
	// OCR text first: when 自动复制译文 is also on, the translation is the fresher
	// write and EasyDict has the same order — recognize, copy, then translate.
	s.autoCopyOCR(res)
	s.autoCopy(res)

	s.present()
}

func (s *Service) record(res *query.Response) {
	if s.history == nil || !wantsHistory(s.cfg.Get(), res) {
		return
	}
	if err := s.history.Put(entryOf(res)); err != nil {
		log.Printf("transdict: 记录历史失败: %v", err)
		return
	}
	s.command("history-changed")
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

// autoCopyOCR is 自动复制 OCR 文本: when a screenshot answer recognizes text, the
// recognized text goes on the pasteboard so it can be pasted while the panel is
// still open. 静音 OCR is not here because copying is its whole job, and a
// partial emit is not an answer yet.
func (s *Service) autoCopyOCR(res *query.Response) {
	text := ocrCopyText(s.cfg.Get(), res)
	if text == "" {
		return
	}
	if !s.wails.Clipboard.SetText(text) {
		log.Print("transdict: 自动复制 OCR 文本失败")
	}
}

func ocrCopyText(cfg settings.Config, res *query.Response) string {
	if res == nil || res.Partial || !cfg.AutoCopyOCR || res.Context != settings.ContextScreenshot {
		return ""
	}
	return res.SourceText
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

func (s *Service) OpenInput() {
	s.present()
	s.command("input")
}

// Preferences are not a tab of the translation window: it is narrow by design,
// and a sidebar plus forms would squeeze the paragraph the user is reading.
// OpenSettings raises it on whichever tab it last showed.
func (s *Service) OpenSettings() { s.show(WindowSettings, true) }

// OpenAbout raises preferences on 关于, which is where this app keeps the version,
// privacy note and credits Easydict shows in its About panel.
func (s *Service) OpenAbout() {
	s.openSettingsPage("about")
}

// OpenHistory raises preferences on 收藏, where 历史 and 收藏 share one list. A
// history of its own window would be a second copy of that page.
func (s *Service) OpenHistory() {
	s.openSettingsPage("favorites")
	// The list is mounted but may have been read before a query answered behind
	// it; a raise re-reads the store, the way the standalone window did.
	s.command("history-shown")
}

func (s *Service) openSettingsPage(page string) {
	s.mu.Lock()
	s.requestedPage = page
	s.mu.Unlock()
	s.command("settings-page:" + page)
	s.show(WindowSettings, true)
}

// TakeSettingsPage hands over the page an opener asked for, once: a settings
// window that mounts later must not reopen on that tab every time it is raised.
func (s *Service) TakeSettingsPage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := s.requestedPage
	s.requestedPage = ""
	return page
}

// OpenLogs is 显示调试日志窗口: the same tail 高级 → 日志 points at.
func (s *Service) OpenLogs() { s.show(WindowLogs, true) }

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

// OCRWindowPinned reports the preview's own pin. EasyDict's OCRWindow keeps the
// flag on the window rather than in preferences: a new recognition reuses the
// window, and the level it was left at is the level it comes back with.
func (s *Service) OCRWindowPinned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ocrPinned
}

// SetOCRWindowPinned flips the OCR 窗口's pin and reports the new state. Pinned
// has both halves EasyDict gives it: a floating level and no hiding on blur.
func (s *Service) SetOCRWindowPinned(pinned bool) bool {
	s.mu.Lock()
	s.ocrPinned = pinned
	s.mu.Unlock()
	if w := s.window(WindowOCR); w != nil {
		w.SetAlwaysOnTop(pinned)
	}
	return pinned
}

// SetResultFont stores the reading size the panel's Aa button cycles. The value
// is a token rather than pixels so the stylesheet keeps its own numbers.
func (s *Service) SetResultFont(size string) error {
	switch size {
	case "1", "2", "3", "4", "5":
	default:
		return errors.New("字号只能是 1 到 5")
	}
	_, err := s.cfg.Update(func(c *settings.Config) { c.ResultFont = size })
	return err
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

func (s *Service) CloseOCR() {
	if w := s.window(WindowOCR); w != nil {
		w.Hide()
	}
}

func (s *Service) CloseLogs() {
	if w := s.window(WindowLogs); w != nil {
		w.Hide()
	}
}

// The progress line is Go's own wording, so it is translated on the way out;
// a composed sentence with no entry simply stays Chinese.
func (s *Service) busy(context, text string) {
	s.wails.Event.Emit(EventBusy, Busy{Target: WindowMain, Text: l10n.T(text)})
}

// ClosePanel hides the window without ending the session.
func (s *Service) ClosePanel() {
	if w := s.window(WindowMain); w != nil {
		w.Hide()
	}
}

func (s *Service) CopyText(text string) bool {
	return s.wails.Clipboard.SetText(text)
}

// ClipboardText backs the query box's 粘贴 item: the webview cannot read the
// pasteboard itself, so the menu takes the text from Go and inserts it at the
// selection. An empty pasteboard and a failed read look the same to the caller,
// which then does nothing.
func (s *Service) ClipboardText() string {
	text, ok := s.wails.Clipboard.Text()
	if !ok {
		return ""
	}
	return text
}

func nowUnix() int64 { return time.Now().Unix() }
