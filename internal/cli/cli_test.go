package cli

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"transdict/internal/l10n"
	"transdict/internal/settings"
)

// The default is the menu-bar app: a Dock tile is what the user opts into, and
// getting the polarity wrong turns every launch into a normal application.
func TestActivationPolicyFollowsTheDockSwitch(t *testing.T) {
	if got := activationPolicy(false); got != application.ActivationPolicyAccessory {
		t.Errorf("hidden = %d, want the accessory policy", got)
	}
	if got := activationPolicy(true); got != application.ActivationPolicyRegular {
		t.Errorf("shown = %d, want the regular policy", got)
	}
}

// Rebinding only on a real shortcut change keeps an unrelated settings edit from
// unregistering every global accelerator and registering it again.
func TestSameShortcuts(t *testing.T) {
	base := map[string][]string{
		"input":   {"alt+shift+d"},
		"capture": {"alt+shift+t", "ctrl+alt+t"},
	}
	same := map[string][]string{
		"capture": {"alt+shift+t", "ctrl+alt+t"},
		"input":   {"alt+shift+d"},
	}
	if !sameShortcuts(base, same) {
		t.Error("相同的槽位与顺序被判为不同，会导致每次保存都重绑快捷键")
	}

	cases := map[string]map[string][]string{
		"改了一个组合键":   {"input": {"alt+shift+e"}, "capture": {"alt+shift+t", "ctrl+alt+t"}},
		"少了一个备用组合键": {"input": {"alt+shift+d"}, "capture": {"alt+shift+t"}},
		"多了一个槽位":    {"input": {"alt+shift+d"}, "capture": {"alt+shift+t", "ctrl+alt+t"}, "mini": {"alt+shift+m"}},
		"清空了一个槽位":   {"input": {}, "capture": {"alt+shift+t", "ctrl+alt+t"}},
		"顺序变了":      {"input": {"alt+shift+d"}, "capture": {"ctrl+alt+t", "alt+shift+t"}},
	}
	for name, changed := range cases {
		if sameShortcuts(base, changed) || sameShortcuts(changed, base) {
			t.Errorf("%s：应当判为不同，才能重新绑定", name)
		}
	}
}

// The tray is the only entry an accessory app has, so its labels, keys and 置顶
// tick are behaviour, not decoration: a shortcut that never appears in the menu
// is one the user has no way to discover.
func TestTrayMenuShowsEveryKeyedActionAndThePinTick(t *testing.T) {
	l10n.Set("zh-CN")
	cfg := settings.Default()
	cfg.Shortcuts = map[string][]string{
		settings.SlotCapture:   {"alt+shift+t"},
		settings.SlotSilentOCR: {"alt+shift+g"},
		settings.SlotInput:     {"alt+shift+d"},
		settings.SlotSelection: {"alt+shift+s"},
		settings.SlotClipboard: {"alt+shift+v"},
		settings.SlotMini:      {"alt+shift+m"},
		settings.SlotReplace:   {"alt+shift+r"},
		settings.SlotODict:     {"alt+shift+w"},
		settings.SlotSettings:  {"alt+shift+o"},
	}
	cfg.Pinned = true
	menu := trayMenu(cfg, trayActions{})

	labels := map[string]string{
		settings.SlotCapture:   "截图翻译",
		settings.SlotSilentOCR: "静音 OCR",
		settings.SlotInput:     "输入翻译",
		settings.SlotSelection: "划词翻译",
		settings.SlotClipboard: "剪贴板翻译",
		settings.SlotMini:      "迷你窗",
		settings.SlotReplace:   "翻译并替换选区",
		settings.SlotODict:     "鼠标取词",
		settings.SlotSettings:  "服务设置",
	}
	for slot, label := range labels {
		it := menu.FindByLabel(l10n.T(label))
		if it == nil {
			t.Fatalf("菜单缺少 %q", label)
		}
		// The accelerator is native decoration with platform-specific spelling,
		// so the expectation comes from the same parser the menu used.
		want := application.NewMenuItem("probe").SetAccelerator(cfg.Shortcuts[slot][0]).GetAccelerator()
		if got := it.GetAccelerator(); got != want {
			t.Errorf("%s 的快捷键 = %q, want %q", label, got, want)
		}
	}

	pin := menu.FindByLabel(l10n.T("置顶译窗"))
	if pin == nil {
		t.Fatal("菜单缺少 置顶译窗")
	}
	if !pin.IsCheckbox() || !pin.Checked() {
		t.Errorf("置顶译窗 = checkbox:%v checked:%v, want 勾选的复选框", pin.IsCheckbox(), pin.Checked())
	}
	for _, name := range []string{"界面语言", "主题", "翻译历史记录", "退出 Transdict"} {
		if menu.FindByLabel(l10n.T(name)) == nil {
			t.Errorf("菜单缺少 %q", name)
		}
	}
}

// A cleared shortcut must leave the menu item bare rather than showing a key
// that no longer fires.
func TestTrayMenuOmitsClearedShortcuts(t *testing.T) {
	cfg := settings.Default()
	cfg.Shortcuts[settings.SlotInput] = nil
	cfg.Shortcuts[settings.SlotCapture] = []string{""}
	menu := trayMenu(cfg, trayActions{})
	for _, label := range []string{"输入翻译", "截图翻译"} {
		it := menu.FindByLabel(l10n.T(label))
		if it == nil {
			t.Fatalf("菜单缺少 %q", label)
		}
		if got := it.GetAccelerator(); got != "" {
			t.Errorf("%s 已清除快捷键，菜单却显示 %q", label, got)
		}
	}
}

// A second launch names the window it wants; anything else — Finder's -psn
// argument, a stray flag — brings up the panel rather than being mistaken for
// a window name.
func TestWakeCommandReadsOnlyKnownWindowNames(t *testing.T) {
	cases := map[string]string{
		"main":      "main",
		"input":     "input",
		"settings":  "settings",
		"history":   "history",
		"mini":      "mini",
		"clipboard": "clipboard",
		"select":    "select",
		"odict":     "odict",
		"replace":   "replace",
		"silent":    "silent",
		"capture":   "capture",
	}
	for arg, want := range cases {
		if got := wakeCommand([]string{"transdict", arg}); got != want {
			t.Errorf("wakeCommand(%q) = %q, 期望 %q", arg, got, want)
		}
	}
	for _, args := range [][]string{
		{"transdict"},
		{"transdict", "-psn_0_12345"},
		{"transdict", "miniature"},
		{"transdict", "--help"},
	} {
		if got := wakeCommand(args); got != "main" {
			t.Errorf("wakeCommand(%q) = %q, 期望默认 main", args, got)
		}
	}
}
