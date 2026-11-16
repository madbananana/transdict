// Package gui is the presentation layer: the bound services the WebView calls,
// the windows they own, and the events they emit. Platform capabilities live
// in their own packages (capture, ocr, translate, dict, tts, shortcut) and the
// pipeline lives in internal/query; this package only orchestrates and talks.
package gui

import (
	"context"
	"image"
	"net/http"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"transdict/internal/capture"
	"transdict/internal/history"
	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
	"transdict/internal/shortcut"
)

const (
	EventResult  = "query:result"
	EventBusy    = "query:busy"
	EventCommand = "app:command"
	// EventUILang announces an interface-language change. Each window reloads on
	// it: the bundle builds its string tables once, so a fresh load is what makes
	// the new language reach them.
	EventUILang = "app:ui-lang"
	// EventTheme announces a light/dark change. Unlike the language it needs no
	// reload — a stylesheet attribute is the whole change — so the panels follow
	// the menu bar the moment it is picked.
	EventTheme = "app:theme"
	// EventConfig announces a saved setting. 窗口配置 lives on each window's own
	// copy of the store, so a toggle has to reach the window already on screen.
	EventConfig = "app:config"
	// EventMenuAction carries a 快捷键 menu click. The menu bar belongs to the
	// app rather than to a webview, so Go picks the query window that holds the
	// keyboard and tells that one window which action to run.
	EventMenuAction = "app:menu-action"
)

// MenuAction names one 快捷键 menu item and the window it belongs to. The menu
// and the in-window keys answer the same action names, in EasyDict's
// ShortcutAction spelling.
type MenuAction struct {
	Target string `json:"target"`
	Action string `json:"action"`
}

// Busy is a progress line addressed to the translation window.
type Busy struct {
	Target string `json:"target"`
	Text   string `json:"text"`
}

// Window roles, mirrored by the frontend hash routes.
const (
	WindowMain     = "main"
	WindowIcon     = "queryicon"
	WindowInput    = "input"
	WindowCapture  = "capture"
	WindowSettings = "settings"
	WindowOCR      = "ocr"
	WindowLogs     = "logs"
)

type Service struct {
	wails   *application.App
	cfg     *settings.Store
	shots   *capture.Store
	engine  *query.Engine
	history *history.Store
	// version is the build the user is running, which is the left-hand side of
	// every 检查更新 answer.
	version string

	mu       sync.Mutex
	session  string
	overlays []captureOverlay
	lastCrop []byte
	last     *query.Response
	// requestedPage is the settings tab an opener (关于, 历史) asked for. The
	// window reads it when it mounts and a command names it when it is already
	// open, because the click can land before or after the webview exists.
	requestedPage string
	// fitGen numbers the translation window's height animation: a newer fit
	// cancels the one still running, so two answers cannot fight over the frame.
	fitGen uint64
	// runs numbers the latest query in each context, and runCancel stops the one
	// it replaced: an answer that arrives after the user has asked something else
	// must not overwrite the newer answer.
	runs      map[string]uint64
	runCancel map[string]context.CancelFunc
	// silent marks the pending capture as 静音 OCR: the crop is recognized and
	// copied, and no result window opens.
	silent bool
	// ocrOnly marks the pending capture as 截图 OCR: the panel shows the text and
	// stops there instead of translating it.
	ocrOnly bool
	// lastOCR keeps the newest recognized answer for the OCR 窗口 entry, which
	// reopens it even after a later text query replaced s.last.
	lastOCR *query.Response
	// ocrPinned is the OCR 窗口's own pin. EasyDict's OCRWindow keeps it on the
	// window: unpinned, losing key hides the preview; pinned, it stays on top.
	ocrPinned bool
	// watchCancel stops the 自动划词 poll; nil means it is not running.
	watchCancel context.CancelFunc
	// shortcutProblems is what the last native registration refused. The OS
	// reports a conflict when the key is registered — before any window exists
	// to show it — so the answer is kept until the 快捷键 page asks.
	shortcutProblems []shortcut.Problem
	// maxPct is the 窗口最大高度百分比 already applied; the store is written on
	// every window move, and only a change is worth re-capping the windows for.
	maxPct int
	// http is the running local API, nil while 启用 HTTP 服务 is off, and
	// httpPort remembers the port it bound so a change restarts it.
	http     *http.Server
	httpPort string
	// notebook holds the response ids already appended to the 生词本: a retry
	// answers under the same id and must not write the query twice.
	notebook map[string]bool
	// iconText is what the 划词查询图标 will ask about when clicked, iconHide
	// is the timer that takes the icon away if the user ignores it, iconRect is
	// the button's frame, which the click tap compares against, and iconMoved is
	// when the pointer was last checked against it. iconScroll is the wheel
	// movement seen so far, and iconKeyTimer is the beat a plain key press gets.
	iconText     string
	iconEditable bool
	iconHide     *time.Timer
	iconRect     image.Rectangle
	iconMoved    time.Time
	iconScroll   float64
	iconKeyTimer *time.Timer
}

func New(cfg *settings.Store, shots *capture.Store, history *history.Store, version string) *Service {
	return &Service{cfg: cfg, shots: shots, engine: query.New(cfg), history: history, version: version}
}

// Config exposes the current settings snapshot to bound methods without letting
// the frontend hold a store handle.
//
//wails:ignore
func (s *Service) Config() settings.Config { return s.cfg.Get() }

// SetShortcutProblems records the outcome of a native registration pass. It is
// called by the launcher, which owns the shortcut bindings.
//
//wails:ignore
func (s *Service) SetShortcutProblems(p []shortcut.Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shortcutProblems = append([]shortcut.Problem(nil), p...)
}

// ShortcutProblems lists the accelerators that are not live. A key held by
// another app fails silently otherwise: the press goes to that app and this one
// looks broken. The preferences window shows the list next to the recorder.
func (s *Service) ShortcutProblems() []shortcut.Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]shortcut.Problem{}, s.shortcutProblems...)
}

// Attach supplies the application handle once it exists; services are built
// before `application.New` because they are passed in as options. Anything that
// runs in the background starts here, because it needs the app to talk through.
//
//wails:ignore
func (s *Service) Attach(app *application.App) {
	s.wails = app
	// The record of who had the keyboard before us is built from activations,
	// so the observer has to be in place before the first window opens.
	selection.TrackFrontmostApps()
	// The 划词查询图标 never takes activation, so its button has to answer
	// the click that arrives while another app is frontmost.
	acceptFirstMouseInViews()
	s.cfg.Watch(s.syncMouseSelect)
	s.syncMouseSelect(s.cfg.Get())
	s.cfg.Watch(func(settings.Config) { s.wails.Event.Emit(EventConfig) })
	s.cfg.Watch(s.syncPin)
	s.cfg.Watch(s.syncMaxHeight)
	s.cfg.Watch(s.syncHTTPServer)
	s.syncHTTPServer(s.cfg.Get())
	// Wails re-shows the windows it still holds when the dock icon is clicked,
	// but a red-button click destroys one; the translation window is the app's
	// face, so a dock click has to rebuild it instead of doing nothing.
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		if _, ok := app.Window.GetByName(WindowMain); !ok {
			s.OpenMain()
		}
	})
}
