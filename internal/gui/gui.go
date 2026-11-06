// Package gui is the presentation layer: the bound services the WebView calls,
// the windows they own, and the events they emit. Platform capabilities live
// in their own packages (capture, ocr, translate, dict, tts, shortcut) and the
// pipeline lives in internal/query; this package only orchestrates and talks.
package gui

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"transdict/internal/capture"
	"transdict/internal/history"
	"transdict/internal/query"
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
)

// Busy is a progress line addressed to one window role, so the panel's
// "翻译中…" never leaks into the floating window.
type Busy struct {
	Target string `json:"target"`
	Text   string `json:"text"`
}

// Window roles, mirrored by the frontend hash routes.
const (
	WindowMain     = "main"
	WindowFloat    = "float"
	WindowMini     = "mini"
	WindowInput    = "input"
	WindowCapture  = "capture"
	WindowSettings = "settings"
	WindowHistory  = "history"
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
	overlays []*application.WebviewWindow
	lastCrop []byte
	last     *query.Response
	// panelLast is what the main window is showing, which is not always the
	// newest answer: a 划词 result goes to the floating panel, and 详情 promotes
	// it here without making the panel adopt every passing selection.
	panelLast *query.Response
	// runs numbers the latest query in each context, and runCancel stops the one
	// it replaced: an answer that arrives after the user has asked something else
	// must not overwrite the newer answer.
	runs      map[string]uint64
	runCancel map[string]context.CancelFunc
	// silent marks the pending capture as 静音 OCR: the crop is recognized and
	// copied, and no result window opens.
	silent bool
	// watchCancel stops the 自动划词 poll; nil means it is not running.
	watchCancel context.CancelFunc
	// shortcutProblems is what the last native registration refused. The OS
	// reports a conflict when the key is registered — before any window exists
	// to show it — so the answer is kept until the 快捷键 page asks.
	shortcutProblems []shortcut.Problem
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
	s.cfg.Watch(s.syncMouseSelect)
	s.syncMouseSelect(s.cfg.Get())
	s.cfg.Watch(s.syncPin)
	// Wails re-shows the windows it still holds when the dock icon is clicked,
	// but a red-button click destroys one; the translation window is the app's
	// face, so a dock click has to rebuild it instead of doing nothing.
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		if _, ok := app.Window.GetByName(WindowMain); !ok {
			s.OpenMain()
		}
	})
}
