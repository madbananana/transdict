package gui

import (
	"testing"

	"transdict/internal/settings"
)

// 禁止名单 silences 自动监听鼠标划词 in the listed apps, and only there: a
// missing bundle id must not block everything.
func TestBlocklistSilencesOnlyListedApps(t *testing.T) {
	cfg := settings.Config{Blocklist: []settings.BlockedApp{
		{BundleID: "com.apple.keychainaccess", Name: "钥匙串访问"},
	}}
	old := frontmostBundleID
	defer func() { frontmostBundleID = old }()

	frontmostBundleID = func() string { return "com.apple.keychainaccess" }
	if watchesApp(cfg) {
		t.Error("禁止名单里的应用仍会触发自动划词")
	}

	frontmostBundleID = func() string { return "com.apple.Safari" }
	if !watchesApp(cfg) {
		t.Error("不在禁止名单的应用被挡住了")
	}

	frontmostBundleID = func() string { return "" }
	if !watchesApp(cfg) {
		t.Error("读不到应用标识时不该停掉自动划词")
	}
}

// The watcher feeds both features: auto query and the clickable icon. Turning
// 划词后自动查询 off while 划词后自动显示查询图标 stays on must keep watching,
// so the button can still be clicked.
func TestWatcherRunsForEitherAutoQueryOrIcon(t *testing.T) {
	cases := []struct {
		name string
		cfg  settings.Config
		want bool
	}{
		{"both on", settings.Config{MouseSelect: true, AutoShowQueryIcon: true}, true},
		{"auto query only", settings.Config{MouseSelect: true}, true},
		{"icon only", settings.Config{AutoShowQueryIcon: true}, true},
		{"both off", settings.Config{}, false},
	}
	for _, c := range cases {
		if got := watchSelectionWanted(c.cfg); got != c.want {
			t.Errorf("%s: watchSelectionWanted = %v, want %v", c.name, got, c.want)
		}
	}
}

// A finished selection asks by itself only when 划词后自动查询 is on and the
// icon is not the sole entry point.
func TestSelectionQueriesOnlyWithAutoQueryAndWithoutClickOnly(t *testing.T) {
	cases := []struct {
		name string
		cfg  settings.Config
		want bool
	}{
		{"auto query", settings.Config{MouseSelect: true, AutoShowQueryIcon: true}, true},
		{"click only", settings.Config{MouseSelect: true, QueryIconClickOnly: true}, false},
		{"icon only", settings.Config{AutoShowQueryIcon: true}, false},
		{"nothing", settings.Config{}, false},
	}
	for _, c := range cases {
		if got := selectionQueriesOnSelect(c.cfg); got != c.want {
			t.Errorf("%s: selectionQueriesOnSelect = %v, want %v", c.name, got, c.want)
		}
	}
}
