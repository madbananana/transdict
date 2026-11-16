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
	l10n.Set("en")
	if got := settingsWindowTitle(); got != "Transdict Settings" {
		t.Errorf("English 标题 = %q, want Transdict Settings", got)
	}
}

// The settings window keeps one size whichever tab is open: switching pages
// must not make the frame jump, so 关于 and 隐私 get the same column as 高级.
func TestSettingsWindowKeepsOneSizeAcrossTabs(t *testing.T) {
	if settingsWindowWidth != 960 {
		t.Errorf("宽 = %d, want 960", settingsWindowWidth)
	}
	if settingsWindowHeight != 720 {
		t.Errorf("高 = %d, want 720", settingsWindowHeight)
	}
}

// The settings window carries the tab name, not the app name, in its title.
func TestSettingsTabTitlesFollowTheSelectedPage(t *testing.T) {
	defer l10n.Set("zh-CN")
	l10n.Set("zh-CN")
	names := map[string]string{
		"general": "通用", "services": "服务", "favorites": "收藏", "disabled": "禁止名单",
		"shortcuts": "快捷键", "advanced": "高级", "privacy": "隐私", "about": "关于",
	}
	for page, want := range names {
		if got := settingsTabTitle(page); got != want {
			t.Errorf("%s = %q, want %q", page, got, want)
		}
	}
	if got := settingsTabTitle("old-page"); got != "通用" {
		t.Errorf("unknown page = %q, want the default 通用", got)
	}
	l10n.Set("en")
	if got := settingsTabTitle("about"); got != "About" {
		t.Errorf("English = %q, want About", got)
	}
}

// 关于 hands the settings page over exactly once: raising preferences from the
// tray afterwards must not land on 关于 again.
func TestTakeSettingsPageHandsOverOnce(t *testing.T) {
	s := &Service{}
	s.mu.Lock()
	s.requestedPage = "about"
	s.mu.Unlock()
	if got := s.TakeSettingsPage(); got != "about" {
		t.Errorf("第一次 = %q, want about", got)
	}
	if got := s.TakeSettingsPage(); got != "" {
		t.Errorf("第二次 = %q, want 空", got)
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

// 自动复制 OCR 文本 is for the screenshot answer only: the silent OCR already
// writes the clipboard, and a partial emit has not recognized everything yet.
func TestOCRTextIsCopiedOnlyForScreenshotAnswers(t *testing.T) {
	shot := &query.Response{Context: settings.ContextScreenshot, SourceText: "hello"}
	if got := ocrCopyText(settings.Config{AutoCopyOCR: true}, shot); got != "hello" {
		t.Errorf("开启后 = %q, want the recognized text", got)
	}
	if got := ocrCopyText(settings.Config{}, shot); got != "" {
		t.Errorf("关闭后 = %q, want nothing", got)
	}
	partial := &query.Response{Context: settings.ContextScreenshot, SourceText: "hello", Partial: true}
	if got := ocrCopyText(settings.Config{AutoCopyOCR: true}, partial); got != "" {
		t.Errorf("半成品 = %q, want nothing", got)
	}
	elsewhere := &query.Response{Context: settings.ContextMain, SourceText: "hello"}
	if got := ocrCopyText(settings.Config{AutoCopyOCR: true}, elsewhere); got != "" {
		t.Errorf("主窗口查询 = %q, want nothing", got)
	}
	if got := ocrCopyText(settings.Config{AutoCopyOCR: true}, nil); got != "" {
		t.Errorf("没有结果 = %q, want nothing", got)
	}
}

// 自动复制划词文本 only applies to the two selection modes: an input-box query
// and a link must not overwrite the clipboard.
func TestSelectedTextIsCopiedOnlyForSelectionQueries(t *testing.T) {
	cfg := settings.Config{AutoCopySelect: true}
	if got := selectionCopyText(cfg, "selection", "hello"); got != "hello" {
		t.Errorf("快捷键划词 = %q, want the text", got)
	}
	if got := selectionCopyText(cfg, "auto-selection", "hello"); got != "hello" {
		t.Errorf("鼠标划词 = %q, want the text", got)
	}
	if got := selectionCopyText(cfg, "input", "hello"); got != "" {
		t.Errorf("输入框查询 = %q, want nothing", got)
	}
	if got := selectionCopyText(settings.Config{}, "selection", "hello"); got != "" {
		t.Errorf("关闭后 = %q, want nothing", got)
	}
	if got := selectionCopyText(cfg, "selection", ""); got != "" {
		t.Errorf("空文本 = %q, want nothing", got)
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

// EasyDict keeps every query window's top edge put when its answer grows: once
// the window reaches the screen bottom the answer scrolls inside it, instead of
// the window sliding up and covering the text the user selected.
func TestFittedHeightKeepsTheTopEdge(t *testing.T) {
	cases := []struct {
		name                                        string
		content, floor, maxH, workBottom, top, want int
	}{
		{"content drives the height", 240, 140, 940, 904, 100, 240},
		{"floor wins over a short answer", 40, 140, 940, 904, 100, 140},
		{"percentage caps the height", 2000, 140, 600, 904, 100, 600},
		{"screen bottom caps it, not the top edge", 800, 140, 940, 904, 700, 196},
		{"floor survives the screen test", 800, 140, 940, 904, 880, 140},
		{"no screen reading keeps the content", 800, 140, 940, 0, 700, 800},
	}
	for _, c := range cases {
		if got := fittedHeight(c.content, c.floor, c.maxH, c.workBottom, c.top); got != c.want {
			t.Errorf("%s: fittedHeight(%d,%d,%d,%d,%d) = %d, want %d",
				c.name, c.content, c.floor, c.maxH, c.workBottom, c.top, got, c.want)
		}
	}
}
