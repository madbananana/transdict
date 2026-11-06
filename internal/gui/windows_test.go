package gui

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/shortcut"
	"transdict/internal/translate"
)

// A history row quotes the card the panel put first, not whichever service
// happened to answer last: a slower fallback translator must not overwrite the
// translation the user actually read.
func TestHistoryQuotesTheFirstTranslation(t *testing.T) {
	e := entryOf(&query.Response{
		SourceText: "hello",
		Translate: []translate.Result{
			{Label: "Google", Text: "你好"},
			{Label: "有道", Text: "您好 哈啰 喂"},
			{Label: "失败", Error: "超时"},
		},
		Dict: []dict.Result{
			{Label: "苹果词典"},
			{Label: "坏词典", Error: "未收录"},
		},
	})
	if e.Target != "你好" {
		t.Errorf("译文 = %q, want 第一条成功的译文", e.Target)
	}
	if got := strings.Join(e.Services, "/"); got != "Google/有道/苹果词典" {
		t.Errorf("服务 = %q, want 只列出成功的服务", got)
	}
}

// The window switcher and Mission Control show the native title, which is not
// part of the page the webview redraws on a language change.
func TestAuxWindowTitlesFollowTheInterfaceLanguage(t *testing.T) {
	defer l10n.Set("zh-CN")
	l10n.Set("zh-CN")
	if got := settingsWindowTitle(); got != "Transdict 设置" {
		t.Errorf("中文标题 = %q", got)
	}
	if got := historyWindowTitle(); got != "Transdict 历史记录" {
		t.Errorf("中文标题 = %q", got)
	}
	l10n.Set("en")
	if got := settingsWindowTitle(); got != "Transdict Settings" {
		t.Errorf("English 标题 = %q, want Transdict Settings", got)
	}
	if got := historyWindowTitle(); got != "Transdict History" {
		t.Errorf("English 标题 = %q, want Transdict History", got)
	}
}

// Streaming snapshots carry the id of the answer they belong to, so a partial
// that reached the recorder would either be replaced later or — if the query
// were cancelled — stay in the list as a row missing every slow service. A
// progress update is not an answer.
func TestPartialAnswersAreNotRecorded(t *testing.T) {
	cfg := settings.Default()
	cfg.SaveHistory = true
	partial := &query.Response{ID: "same", SourceText: "hello", Partial: true}
	if wantsHistory(cfg, partial) {
		t.Error("中间结果不应写入历史")
	}
	full := &query.Response{ID: "same", SourceText: "hello"}
	if !wantsHistory(cfg, full) {
		t.Error("完整结果应当写入历史")
	}
}

// A window created at N reports N-1; treating that as an adjustment shrank the
// window by a pixel per launch.
func TestNearIgnoresFrameRounding(t *testing.T) {
	cases := []struct {
		a, b int
		want bool
	}{
		{519, 518, true},
		{519, 517, true},
		{519, 516, false},
		{300, 302, true},
		{300, 304, false},
	}
	for _, c := range cases {
		if got := near(c.a, c.b); got != c.want {
			t.Errorf("near(%d, %d) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// A saved frame is only reused while a draggable part of it is still on a
// display; otherwise the window would reopen where nothing can reach it.
func TestFrameNeedsAReachablePartOnScreen(t *testing.T) {
	screen := application.Rect{X: 0, Y: 0, Width: 1440, Height: 900}
	cases := []struct {
		name  string
		frame application.Rect
		want  bool
	}{
		{"centered", application.Rect{X: 480, Y: 150, Width: 480, Height: 600}, true},
		{"second monitor to the right", application.Rect{X: 1500, Y: 100, Width: 480, Height: 600}, false},
		{"below the screen", application.Rect{X: 400, Y: 950, Width: 480, Height: 600}, false},
		{"mostly off the left edge", application.Rect{X: -440, Y: 150, Width: 480, Height: 600}, false},
		{"a strip on the right edge", application.Rect{X: 1400, Y: 150, Width: 480, Height: 600}, false},
	}
	for _, c := range cases {
		if got := frameOnDisplay(c.frame, screen); got != c.want {
			t.Errorf("%s: frameOnDisplay = %v, want %v", c.name, got, c.want)
		}
	}
}

// 详情 means "this answer, larger": a floating-panel reply has to travel to the
// main window intact, while an answer the panel already owns is left alone.
func TestDetailOfPromotesOnlyFloatingAnswers(t *testing.T) {
	base := &query.Response{
		ID: "q1", Context: settings.ContextFloating, Mode: "selection",
		SourceText: "hello",
		Translate:  []translate.Result{{Label: "Google", Text: "你好"}},
		Dict:       []dict.Result{{Label: "苹果词典"}},
	}
	d, ok := detailOf(base)
	if !ok || d.Context != settings.ContextMain {
		t.Fatalf("detail = %+v ok=%v, want the answer promoted to the main window", d, ok)
	}
	if d.SourceText != "hello" || d.Mode != "selection" || len(d.Translate) != 1 || len(d.Dict) != 1 {
		t.Errorf("promoted answer lost part of itself: %+v", d)
	}
	if base.Context != settings.ContextFloating {
		t.Error("the stored answer was rewritten instead of copied")
	}
	// A retry may write into the stored snapshot; the panel's copy must not move.
	base.Translate[0].Text = "changed"
	if d.Translate[0].Text != "你好" {
		t.Errorf("promoted translation = %q, want an independent copy", d.Translate[0].Text)
	}

	if _, ok := detailOf(&query.Response{Context: settings.ContextMain}); ok {
		t.Error("a main-window answer does not need promoting")
	}
	if _, ok := detailOf(nil); ok {
		t.Error("nothing to promote")
	}
	if odict, ok := detailOf(&query.Response{Context: settings.ContextODict}); !ok || odict.Context != settings.ContextMain {
		t.Error("鼠标取词 answers reach the same floating panel and must promote too")
	}
	if mini, ok := detailOf(&query.Response{Context: settings.ContextMini}); !ok || mini.Context != settings.ContextMain {
		t.Error("迷你窗 answers show one line and still need 详情")
	}
}

// 记录查询历史 off must keep the text out of the file entirely, and an answer
// that has no source (a failed recognition) is not a row either.
func TestHistoryIsWrittenOnlyWhenEnabledAndAnswerable(t *testing.T) {
	res := &query.Response{SourceText: "hello"}
	if !wantsHistory(settings.Config{SaveHistory: true}, res) {
		t.Error("开启时应当记录")
	}
	if wantsHistory(settings.Config{SaveHistory: false}, res) {
		t.Error("关闭时不得写入历史")
	}
	if wantsHistory(settings.Config{SaveHistory: true}, nil) {
		t.Error("空结果不得写入历史")
	}
	if wantsHistory(settings.Config{SaveHistory: true}, &query.Response{}) {
		t.Error("没有原文的结果不得写入历史")
	}
}

// The list is copied in and out: the page holds a snapshot, and a later
// registration must not edit the slice it is already displaying.
func TestShortcutProblemsAreCopiedBothWays(t *testing.T) {
	s := &Service{}
	given := []shortcut.Problem{{Accel: "alt+shift+d", Err: "already taken"}}
	s.SetShortcutProblems(given)

	got := s.ShortcutProblems()
	if len(got) != 1 || got[0] != given[0] {
		t.Fatalf("ShortcutProblems = %v, want %v", got, given)
	}
	got[0].Accel = "changed"
	if again := s.ShortcutProblems(); again[0].Accel != "alt+shift+d" {
		t.Errorf("a caller edited the stored problem: %v", again)
	}
	given[0].Accel = "also changed"
	if again := s.ShortcutProblems(); again[0].Accel != "alt+shift+d" {
		t.Errorf("the stored problem aliases the caller's slice: %v", again)
	}
	s.SetShortcutProblems(nil)
	if got := s.ShortcutProblems(); len(got) != 0 {
		t.Errorf("cleared list = %v, want empty", got)
	}
}
