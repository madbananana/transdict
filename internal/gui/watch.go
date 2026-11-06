package gui

import (
	"context"
	"time"
	"unicode/utf8"

	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
)

// 自动鼠标划词 polls the focused application for a new highlight. The interval is
// short enough to feel instant and long enough that a text-heavy app never
// notices the read.
const (
	mouseSelectInterval = 600 * time.Millisecond
	mouseSelectMaxRunes = 300
)

// syncMouseSelect is a settings watcher, so flipping the switch in 通用 starts or
// stops the poll without a relaunch.
func (s *Service) syncMouseSelect(cfg settings.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wails == nil {
		return
	}
	running := s.watchCancel != nil
	wanted := cfg.MouseSelect && selection.Pollable
	if running == wanted {
		return
	}
	if !wanted {
		s.watchCancel()
		s.watchCancel = nil
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.watchCancel = cancel
	go func() {
		if err := selection.Watch(ctx, mouseSelectInterval, s.autoSelect); err != nil {
			s.busy(settings.ContextMain, "自动划词已停止："+err.Error())
		}
	}()
}

// autoSelect answers a highlight the watcher found. A long selection is almost
// always a stray drag over a paragraph, and firing every paid service at it —
// then showing a page of results nobody asked for — is worse than staying quiet.
func (s *Service) autoSelect(text string) {
	if n := utf8.RuneCountInString(text); n == 0 || n > mouseSelectMaxRunes {
		return
	}
	stage := s.selectionContext(settings.ContextFloating)
	if err := s.Ask(query.Request{Text: text, Context: stage, Mode: "auto-selection"}); err != nil {
		s.busy(stage, err.Error())
	}
}
