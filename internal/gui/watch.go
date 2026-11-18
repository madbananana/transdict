package gui

import (
	"context"
	"log"
	"time"
	"unicode/utf8"

	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
)

// 自动鼠标划词 follows the gesture that made the selection: the goal is to read
// the highlight the moment the user lets go of it. mouseSelectInterval is only
// the fallback route — a poll of the focused application — for a session where
// the event tap cannot be installed.
const (
	mouseSelectInterval = 600 * time.Millisecond
	mouseSelectMaxRunes = 300
)

// watchSelectionWanted reports whether the selection watcher must run. Auto
// query needs it, and so does an icon that is meant to be clickable: EasyDict's
// 自动显示查询图标 is independent of 划词后自动查询, so turning the latter off
// must leave the icon (and its click) working.
func watchSelectionWanted(cfg settings.Config) bool {
	return cfg.MouseSelect || cfg.AutoShowQueryIcon
}

// selectionQueriesOnSelect is whether finishing a selection asks right away.
// 点击划词查询图标时才查询 withholds the automatic ask, and with 划词后自动查询
// off the icon click is the only way to ask.
func selectionQueriesOnSelect(cfg settings.Config) bool {
	return cfg.MouseSelect && !cfg.QueryIconClickOnly
}

// syncMouseSelect is a settings watcher, so flipping the switch in 通用 starts or
// stops the watcher without a relaunch.
func (s *Service) syncMouseSelect(cfg settings.Config) {
	if s.wails == nil {
		return
	}
	s.mu.Lock()
	running := s.watchCancel != nil
	wanted := watchSelectionWanted(cfg) && selection.Pollable
	if running == wanted {
		s.mu.Unlock()
		return
	}
	if !wanted {
		s.watchCancel()
		s.watchCancel = nil
		s.iconText = ""
		if s.iconHide != nil {
			s.iconHide.Stop()
			s.iconHide = nil
		}
		s.mu.Unlock()
		if w := s.window(WindowIcon); w != nil {
			w.Hide()
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.watchCancel = cancel
	s.mu.Unlock()
	go func() {
		// Watching the gesture is the EasyDict route and the reason the answer
		// arrives right after the mouse is released. Without the event tap —
		// 辅助功能 withheld — reading the focused element on a timer is all that
		// is left.
		err := selection.WatchPointer(ctx, s.autoSelect)
		if err == nil || ctx.Err() != nil {
			return
		}
		log.Print("transdict: 自动划词: 事件监听不可用，改用轮询")
		if err := selection.Watch(ctx, mouseSelectInterval, s.autoSelect); err != nil {
			s.busy(settings.ContextMain, "自动划词已停止："+err.Error())
		}
	}()
}

// watchesApp is the 禁止名单 gate, kept separate so the decision can be tested
// without a window server.
func watchesApp(cfg settings.Config) bool {
	return !cfg.BlocksApp(frontmostBundleID())
}

// autoSelect answers a highlight the watcher found. A long selection is almost
// always a stray drag over a paragraph, and firing every paid service at it —
// then showing a page of results nobody asked for — is worse than staying quiet.
func (s *Service) autoSelect(text string) {
	text = s.booksExcerptText(text)
	if n := utf8.RuneCountInString(text); n == 0 || n > mouseSelectMaxRunes {
		return
	}
	cfg := s.cfg.Get()
	// 禁止名单: a terminal or a password manager is where a stray selection is
	// most costly, so the watcher stays quiet before it spends a request.
	if !watchesApp(cfg) {
		return
	}
	// Editability has to be read now, while the source app still holds the
	// focused field: it decides whether each result card offers 替换.
	editable := selection.FocusedElementEditable()
	icon := queryIconWanted(cfg, text)
	log.Printf("transdict: 自动划词: 收到选区 %d 字（查询图标=%v）", utf8.RuneCountInString(text), icon)
	if icon {
		s.showQueryIcon(text, editable)
	}
	// The icon is the ask when the switches say so: 点击划词查询图标时才查询
	// holds the automatic query back, and with 划词后自动查询 off a selection
	// only offers the button. A selection that earned no icon has nothing to
	// click, so it stays quiet instead of firing every service.
	if !selectionQueriesOnSelect(cfg) {
		return
	}
	stage := settings.ContextMain
	if err := s.Ask(query.Request{Text: text, Context: stage, Mode: "auto-selection",
		Replaceable: editable}); err != nil {
		s.busy(stage, err.Error())
	}
}
