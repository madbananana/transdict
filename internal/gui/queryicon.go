package gui

import (
	"image"
	"log"
	"math"
	"time"
	"unicode/utf8"

	"transdict/internal/query"
	"transdict/internal/settings"
)

// The 划词查询图标 is a small 查询 button that appears beside a fresh mouse
// selection, which is Easydict's 划词后自动显示查询图标. It gives the watcher a
// click target when the user wants to ask again, and with 点击划词查询图标时才查询
// it becomes the only trigger, so a selection never fires a paid service by
// itself.
const (
	// queryIconSize is EasyDict's 23-point pop button plus a little, so the
	// magnifier stays legible without sitting on the text like a card.
	queryIconSize = 26
	// queryIconReach is how far the pointer may travel from the button before it
	// withdraws — Easydict's expandedRadius. A button that outlives the gesture
	// that made it is in the way of the next gesture.
	queryIconReach = 140
	// queryIconGrace keeps the button from vanishing on the first mouse move
	// after it appears: the pointer is often still travelling when the answer
	// lands, and a button that is gone by the time the hand reaches it is worse
	// than no button at all.
	queryIconGrace = 800 * time.Millisecond
	// queryIconMoveInterval is the least time between two reach checks, the same
	// throttle EasyDict puts on mouse-moved events.
	queryIconMoveInterval = 100 * time.Millisecond
	// queryIconScroll is how much wheel movement takes the button away: scrolling
	// means the user is reading on, not looking for a button.
	queryIconScroll = 80
	// queryIconKeyDelay is the beat a plain key press gets before the button
	// goes; a command chord dismisses it at once.
	queryIconKeyDelay = 100 * time.Millisecond
	// queryIconLifetime is the fallback when the event tap is unavailable: with
	// nothing watching the pointer, an ignored button must at least expire.
	queryIconLifetime = 8 * time.Second
)

// CGEventFlags, from <CoreGraphics/CGEventTypes.h>.
const (
	flagShift     uint64 = 1 << 17
	flagControl   uint64 = 1 << 18
	flagAlternate uint64 = 1 << 19
	flagCommand   uint64 = 1 << 20
	flagFunction  uint64 = 1 << 23
)

// kVK_ANSI_C: the one key that must not dismiss the button, because copying the
// selection is exactly what the button is for.
const keyCodeC = 8

// queryIconWanted answers whether a selection earns an icon: the switch is on,
// it has at least 最小字符长度 characters, and its language is not the one the
// user excluded (简体中文 by default, because a Chinese highlight next to the
// icon is usually the user's own note).
func queryIconWanted(cfg settings.Config, text string) bool {
	if !cfg.AutoShowQueryIcon {
		return false
	}
	if utf8.RuneCountInString(text) < cfg.QueryIconMinLength {
		return false
	}
	excluded := cfg.QueryIconExcludeLang
	return excluded == "" || detectLanguage(cfg, text) != excluded
}

// showQueryIcon places the button beside the pointer and starts watching for
// the gesture that ends its life. editable is carried along for the click: the
// answer may offer 替换 when the selection sat in a field that accepts typing.
func (s *Service) showQueryIcon(text string, editable bool) {
	if s.wails == nil {
		return
	}
	w := s.windowFor(WindowIcon)
	if w == nil {
		log.Print("transdict: 划词查询图标: 窗口创建失败")
		return
	}
	cursor, ok := s.cursorDip()
	if !ok {
		log.Print("transdict: 划词查询图标: 读不到鼠标位置")
		return
	}
	x, y := cursor.X+10, cursor.Y+18
	if bounds, ok := s.displayAt(cursor); ok {
		x, y = clampToDisplay(bounds, x, y, queryIconSize, queryIconSize)
	}
	w.SetPosition(x, y)
	w.SetSize(queryIconSize, queryIconSize)

	s.mu.Lock()
	if s.iconHide != nil {
		s.iconHide.Stop()
		s.iconHide = nil
	}
	if s.iconKeyTimer != nil {
		s.iconKeyTimer.Stop()
		s.iconKeyTimer = nil
	}
	s.iconText = text
	s.iconEditable = editable
	s.iconScroll = 0
	s.iconMoved = time.Time{}
	s.iconShown = time.Now()
	s.mu.Unlock()

	w.Show()
	w.SetAlwaysOnTop(true)
	log.Printf("transdict: 划词查询图标: 显示于 (%d,%d)，%dpt", x, y, queryIconSize)
	// The button never activates: the user is still in the middle of a sentence
	// somewhere else, and a click on it must not take the keyboard away.
	acceptFirstMouseInViews()
	s.mu.Lock()
	s.iconRect = image.Rect(x, y, x+queryIconSize, y+queryIconSize)
	s.mu.Unlock()
	// The tap is what withdraws the button: a click elsewhere, the pointer
	// leaving, a scroll, a key. Without it — 辅助功能 not granted — the lifetime
	// timer is the only way the button ever leaves.
	if !startIconTap(s) {
		s.mu.Lock()
		s.iconHide = time.AfterFunc(queryIconLifetime, s.hideQueryIcon)
		s.mu.Unlock()
	}
}

// iconClickedAt answers a left or right mouse down anywhere. The event tap
// thread calls it, so the dismissal runs on its own goroutine: the click the
// user is making must not wait for a window to hide. A click that lands on one
// of our own windows is not "elsewhere" — opening a result menu or the panel
// while the button waits is no reason to take the button away.
func (s *Service) iconClickedAt(x, y float64, ownWindow bool) {
	s.mu.Lock()
	active := s.iconText != ""
	rect := s.iconRect
	s.mu.Unlock()
	if !active || ownWindow || image.Pt(int(x), int(y)).In(rect) {
		return
	}
	go s.hideQueryIcon()
}

// iconMovedTo watches the pointer while the button is visible. Moving more than
// queryIconReach away from it is the commonest way the button should go: the
// user finished with the selection and is doing something else.
func (s *Service) iconMovedTo(x, y float64) {
	s.mu.Lock()
	active := s.iconText != ""
	rect := s.iconRect
	now := time.Now()
	due := now.Sub(s.iconMoved) >= queryIconMoveInterval
	if due {
		s.iconMoved = now
	}
	grace := now.Sub(s.iconShown) < queryIconGrace
	s.mu.Unlock()
	if !active || !due || grace {
		return
	}
	if !iconInReach(rect, x, y) {
		go s.hideQueryIcon()
	}
}

// iconInReach answers whether the pointer is still close enough to the button
// for it to stay: inside it, or within queryIconReach of its centre.
func iconInReach(rect image.Rectangle, x, y float64) bool {
	cx := float64(rect.Min.X+rect.Max.X) / 2
	cy := float64(rect.Min.Y+rect.Max.Y) / 2
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= queryIconReach*queryIconReach
}

// iconScrolled adds wheel movement up; past queryIconScroll the button goes.
func (s *Service) iconScrolled(delta float64) {
	s.mu.Lock()
	s.iconScroll += math.Abs(delta)
	over := s.iconScroll > queryIconScroll
	active := s.iconText != ""
	s.mu.Unlock()
	if active && over {
		go s.hideQueryIcon()
	}
}

// iconKeyAction is what one key press means for the button.
type iconKeyAction int

const (
	iconKeyKeep iconKeyAction = iota
	iconKeyDismiss
	iconKeyDismissSoon
)

// iconKeyActionFor mirrors EasyDict's rules: ⌘C keeps the button (copying the
// selection is what it is for), a command chord takes it away at once, a plain
// key after a short beat, and a lone modifier says nothing yet.
func iconKeyActionFor(keyCode int, flags uint64) iconKeyAction {
	if flags&flagCommand != 0 && keyCode == keyCodeC {
		return iconKeyKeep
	}
	if modifierKey(keyCode) {
		return iconKeyKeep
	}
	if flags&(flagShift|flagControl|flagAlternate|flagCommand|flagFunction) != 0 {
		return iconKeyDismiss
	}
	return iconKeyDismissSoon
}

// modifierKey names the keys that are only modifiers, which never dismiss.
func modifierKey(code int) bool {
	switch code {
	case 54, 55, 56, 57, 58, 59, 60, 61, 62, 63:
		return true
	}
	return false
}

// iconKeyDown applies iconKeyActionFor to the running button.
func (s *Service) iconKeyDown(keyCode int, flags uint64) {
	switch iconKeyActionFor(keyCode, flags) {
	case iconKeyDismiss:
		s.mu.Lock()
		active := s.iconText != ""
		s.mu.Unlock()
		if active {
			go s.hideQueryIcon()
		}
	case iconKeyDismissSoon:
		s.mu.Lock()
		if s.iconText == "" {
			s.mu.Unlock()
			return
		}
		if s.iconKeyTimer != nil {
			s.iconKeyTimer.Stop()
		}
		s.iconKeyTimer = time.AfterFunc(queryIconKeyDelay, s.hideQueryIcon)
		s.mu.Unlock()
	}
}

// hideQueryIcon withdraws the button and forgets what it would have asked.
func (s *Service) hideQueryIcon() {
	s.mu.Lock()
	if s.iconHide != nil {
		s.iconHide.Stop()
		s.iconHide = nil
	}
	if s.iconKeyTimer != nil {
		s.iconKeyTimer.Stop()
		s.iconKeyTimer = nil
	}
	s.iconText = ""
	s.iconEditable = false
	s.iconScroll = 0
	s.mu.Unlock()
	stopIconTap()
	if s.wails == nil {
		return
	}
	if w := s.window(WindowIcon); w != nil {
		w.Hide()
	}
}

// IconQuery is the button itself: the text was captured by the watcher, so the
// click only has to ask. It is a bound method so the icon window can call it.
func (s *Service) IconQuery() error {
	s.mu.Lock()
	text := s.iconText
	editable := s.iconEditable
	s.mu.Unlock()
	s.hideQueryIcon()
	if text == "" {
		return nil
	}
	stage := settings.ContextMain
	return s.Ask(query.Request{Text: text, Context: stage, Mode: "auto-selection",
		Replaceable: editable})
}
