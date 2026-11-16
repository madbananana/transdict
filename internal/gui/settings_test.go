package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"transdict/internal/settings"
)

func testStore(t *testing.T) *settings.Store {
	t.Helper()
	store, err := settings.NewStoreAt(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("设置存储: %v", err)
	}
	return store
}

// Save writes the config field by field, so a new switch has to be listed there
// or the settings page would offer a toggle that never survives a restart.
func TestSaveCarriesTheUpdateAndClipboardSwitches(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	cfg := store.Get()
	cfg.UpdateURL = "https://example.test/releases.json"
	cfg.AutoUpdate = false
	cfg.AutoCopy = true
	if err := svc.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	next := store.Get()
	if next.UpdateURL != cfg.UpdateURL {
		t.Errorf("updateURL = %q, want %q", next.UpdateURL, cfg.UpdateURL)
	}
	if next.AutoUpdate {
		t.Error("写下的 false 被存成了 true")
	}
	if !next.AutoCopy {
		t.Error("autoCopy 没有被保存")
	}
}

// The HTTP switch opens a port, so both the switch and the port have to survive
// a save; a bad port is refused rather than written and retried at launch.
func TestSaveCarriesTheHTTPServerSettings(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	cfg := store.Get()
	cfg.EnableHTTPServer = true
	cfg.HTTPPort = " 9090 "
	if err := svc.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	next := store.Get()
	if !next.EnableHTTPServer {
		t.Error("enableHTTPServer 没有被保存")
	}
	if next.HTTPPort != "9090" {
		t.Errorf("httpPort = %q, want 9090", next.HTTPPort)
	}

	next.HTTPPort = "not-a-port"
	if err := svc.Save(next); err == nil {
		t.Error("an unusable port was accepted")
	}
	if store.Get().HTTPPort != "9090" {
		t.Error("被拒绝的保存改动了端口")
	}
}

func TestSaveRefusesAnUpdateSourceThatIsNotAnAddress(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	cfg := store.Get()
	cfg.UpdateURL = "example.test/releases.json"
	if err := svc.Save(cfg); err == nil || !strings.Contains(err.Error(), "http(s)") {
		t.Errorf("err = %v, want 说明需要 http(s) 地址", err)
	}
	if store.Get().UpdateURL != "" {
		t.Error("被拒绝的保存改动了设置")
	}

	cfg.UpdateURL = "  "
	if err := svc.Save(cfg); err != nil {
		t.Errorf("清空地址不应该报错: %v", err)
	}
}

// A service added on the 服务 page has to reach the windows that already use
// its stage, or the user adds a translator and the answer never changes.
func TestAddingAServiceJoinsTheWindowsThatUseItsStage(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	if _, err := store.Update(func(c *settings.Config) {
		c.Selections[settings.ContextMain] = []string{"tr-google"}
		c.Selections[settings.ContextClipboard] = nil
	}); err != nil {
		t.Fatalf("准备设置: %v", err)
	}

	p, err := svc.AddProvider(settings.CatTranslate, "deepl")
	if err != nil {
		t.Fatalf("添加: %v", err)
	}
	got := store.Get().Selections[settings.ContextMain]
	if len(got) != 2 || got[1] != p.ID {
		t.Errorf("主窗口勾选 = %v, want 原有服务后面加上 %s", got, p.ID)
	}
	// An empty selection means "every enabled service"; leaving it empty keeps
	// that meaning instead of narrowing it to the new one.
	if ids := store.Get().Selections[settings.ContextClipboard]; len(ids) != 0 {
		t.Errorf("剪贴板勾选 = %v, want 仍为空", ids)
	}
}

// 清除 in the settings page used to delete the key, and merge() treats an absent
// key as a file written by an older build — so the default shortcut came back on
// the next launch and the clear looked broken.
func TestClearedShortcutSurvivesARelaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := settings.NewStoreAt(path)
	if err != nil {
		t.Fatalf("设置存储: %v", err)
	}
	if err := NewSettings(store, "").SetShortcut(settings.SlotCapture, ""); err != nil {
		t.Fatalf("清除快捷键: %v", err)
	}

	reopened, err := settings.NewStoreAt(path)
	if err != nil {
		t.Fatalf("重新打开配置: %v", err)
	}
	if got := reopened.Get().Accel(settings.SlotCapture); len(got) != 0 {
		t.Errorf("重启后截图快捷键 = %v, want 仍是已清除", got)
	}
}

// "改为全部启用" stores an empty list meaning "everything enabled". Deleting the
// key instead let merge refill the shipped default, which silently narrowed the
// window again and dropped services added since.
func TestClearedSelectionSurvivesARelaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := settings.NewStoreAt(path)
	if err != nil {
		t.Fatalf("设置存储: %v", err)
	}
	if err := NewSettings(store, "").SetSelection(settings.ContextMain, nil); err != nil {
		t.Fatalf("改为全部启用: %v", err)
	}

	reopened, err := settings.NewStoreAt(path)
	if err != nil {
		t.Fatalf("重新打开配置: %v", err)
	}
	ids, ok := reopened.Get().Selections[settings.ContextMain]
	if !ok || len(ids) != 0 {
		t.Errorf("重启后主窗口勾选 = %v（键存在=%v）, want 显式空表（全部启用）", ids, ok)
	}
}

// Save copies fields one by one, so a switch added to Config but not to that
// list looks saved in the page and is gone after the reload that follows.
func TestSaveCopiesEveryScalarToggle(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()
	next.SaveHistory = false
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}

	got := store.Get()
	if got.SaveHistory {
		t.Error("记录查询历史 关闭后应当写入存储")
	}
}

// The hand-written field list in Save is what lets a new switch go missing, so
// this walks the struct instead of naming fields: every scalar must survive a
// save. Only the values with a path of their own are exempt — a window frame is
// written when the window moves, the pin by TogglePin, the login item by the
// system.
func TestSaveCarriesEveryScalarField(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()

	exempt := map[string]bool{
		// The pin and the login item have their own paths, and Seed and
		// ShortcutSeed are the migration stamps merge() owns — the page must
		// not carry any of them.
		"Pinned": true, "LoginItem": true, "Seed": true, "ShortcutSeed": true,
		"MainX": true, "MainY": true, "MainW": true, "MainH": true,
		"SettingsX": true, "SettingsY": true, "SettingsW": true, "SettingsH": true,
	}
	fields := reflect.ValueOf(&next).Elem()
	for i := 0; i < fields.NumField(); i++ {
		if exempt[fields.Type().Field(i).Name] {
			continue
		}
		switch f := fields.Field(i); f.Kind() {
		case reflect.Bool:
			f.SetBool(!f.Bool())
		case reflect.Int:
			f.SetInt(f.Int() + 7)
		}
	}
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}

	want := reflect.ValueOf(next)
	got := reflect.ValueOf(store.Get())
	for i := 0; i < want.NumField(); i++ {
		name := want.Type().Field(i).Name
		if exempt[name] {
			continue
		}
		switch want.Field(i).Kind() {
		case reflect.Bool, reflect.Int:
			if !reflect.DeepEqual(want.Field(i).Interface(), got.Field(i).Interface()) {
				t.Errorf("%s = %v, want %v：Save 的字段清单漏掉了它",
					name, got.Field(i).Interface(), want.Field(i).Interface())
			}
		}
	}
}

// 菜单栏图标 is set from the tray and the settings page has no control for it,
// so a page that loaded before the switch must not save its stale copy back.
func TestSaveKeepsTheTrayMenuBarIconStyle(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	if _, err := store.Update(func(c *settings.Config) { c.MenuBarIconStyle = settings.MenuBarIconSquare }); err != nil {
		t.Fatal(err)
	}
	next := svc.Get()
	next.MenuBarIconStyle = settings.MenuBarIconRounded
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}
	if got := store.Get().MenuBarIconStyle; got != settings.MenuBarIconSquare {
		t.Errorf("保存后样式 = %q, want square", got)
	}
}

func TestSaveKeepsEveryAnkiMapping(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()
	next.AnkiMappings = []settings.AnkiMapping{
		{Field: " 单词 ", Template: " {Word} — {Translation} "},
		{Field: "释义", Template: "{Translation}"},
	}
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}
	got := store.Get().AnkiMappings
	if len(got) != 2 || got[0].Field != "单词" || got[0].Template != "{Word} — {Translation}" ||
		got[1].Field != "释义" || got[1].Template != "{Translation}" {
		t.Errorf("映射没有按行保存并去掉空白: %#v", got)
	}
}

func TestSaveRefusesTheSameLanguageTwice(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()
	next.FirstLang = "ja"
	next.SecondLang = "ja"
	if err := svc.Save(next); err == nil {
		t.Fatal("相同的两个语种被保存了")
	}
}

// With the tray hidden an accessory app has no visible entry left, so a
// configuration without an input or selection shortcut must not be allowed to
// lose it; one working key is enough.
func TestSaveRefusesHidingTheTrayWithoutAnEntryShortcut(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	next := svc.Get()
	next.HideMenuBarIcon = true
	next.Shortcuts[settings.SlotInput] = nil
	next.Shortcuts[settings.SlotSelection] = []string{" "}
	if err := svc.Save(next); err == nil {
		t.Fatal("没有入口快捷键时不应允许隐藏菜单栏图标")
	}
	if store.Get().HideMenuBarIcon {
		t.Error("被拒绝的保存不应写进配置")
	}

	next.Shortcuts[settings.SlotSelection] = []string{"alt+shift+s"}
	if err := svc.Save(next); err != nil {
		t.Fatalf("有划词快捷键时应允许隐藏: %v", err)
	}
	if !store.Get().HideMenuBarIcon {
		t.Error("hideMenuBarIcon 没有被保存")
	}
}

// 语种识别 is carried like every other option, and a name this build does not
// know is clamped before the write instead of being left in the file.
// 所选目录不可写。 is shown before the switch is left on: a folder that exists
// but refuses a probe counts as broken, while a path that does not exist yet is
// created by the notebook on its first write.
func TestVocabularyDirWritable(t *testing.T) {
	svc := NewSettings(testStore(t), "")
	if !svc.VocabularyDirWritable("") {
		t.Error("空路径不应报不可写")
	}
	if !svc.VocabularyDirWritable(filepath.Join(t.TempDir(), "words")) {
		t.Error("尚未创建的目录应可用")
	}
	dir := t.TempDir()
	if !svc.VocabularyDirWritable(dir) {
		t.Error("普通临时目录应可写")
	}
	file := filepath.Join(dir, "a.jsonl")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if svc.VocabularyDirWritable(file) {
		t.Error("文件路径不应被当成可写目录")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if svc.VocabularyDirWritable(ro) {
		t.Error("只读目录应报不可写")
	}
}

func TestSaveCarriesAndClampsDetectOptimize(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()
	next.DetectOptimize = " bing "
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}
	if got := store.Get().DetectOptimize; got != settings.DetectOptimizeNone {
		t.Errorf("未知方式 = %q, want %q", got, settings.DetectOptimizeNone)
	}

	next = svc.Get()
	next.DetectOptimize = settings.DetectOptimizeGoogle
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存 google: %v", err)
	}
	if got := store.Get().DetectOptimize; got != settings.DetectOptimizeGoogle {
		t.Errorf("detectOptimize = %q, want google", got)
	}
}

// 窗口配置 saves one switch of the window; the switch beside it must keep what
// it had.
func TestSetWindowOptionTouchesOneSwitch(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	if err := svc.SetWindowOption(settings.WindowMain, "langBar", false); err != nil {
		t.Fatalf("SetWindowOption: %v", err)
	}
	got := store.Get().WindowConfig
	if got[settings.WindowMain].LangBar {
		t.Error("主窗口的显示语言选择栏没有被关掉")
	}
	if !got[settings.WindowMain].Input {
		t.Error("同一窗口的显示文本输入框不应改动")
	}
	if err := svc.SetWindowOption("nope", "input", false); err == nil {
		t.Error("未知窗口应被拒绝")
	}
	if err := svc.SetWindowOption(settings.WindowMain, "zoom", false); err == nil {
		t.Error("未知配置项应被拒绝")
	}
}

// 应用内快捷键 is a table rather than a scalar: the menu needs one key per
// action, a rebind must survive the next save, and an action the page omits
// must keep its EasyDict default instead of being unbound by omission.
func TestAppShortcutRebindKeepsDefaultsForTheRest(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")

	if got := store.Get().AppShortcuts[settings.AppActRetry]; got != "cmd+r" {
		t.Fatalf("默认重试键 = %q，期望 cmd+r", got)
	}
	if err := svc.SetAppShortcut(settings.AppActRetry, "alt+r"); err != nil {
		t.Fatalf("SetAppShortcut: %v", err)
	}
	if got := store.Get().AppShortcuts[settings.AppActRetry]; got != "alt+r" {
		t.Errorf("重试键 = %q，期望 alt+r", got)
	}
	if err := svc.SetAppShortcut("nope", "alt+x"); err == nil {
		t.Error("未知动作应被拒绝")
	}
	// ⌘P is already 钉住窗口, so handing it to 重试 would make AppKit pick one.
	if err := svc.SetAppShortcut(settings.AppActRetry, "cmd+p"); err == nil {
		t.Error("与其他应用内快捷键冲突应被拒绝")
	}

	// An empty value is a deliberate unbinding and survives the settings save,
	// which sends the whole table back on every unrelated toggle.
	if err := svc.SetAppShortcut(settings.AppActRetry, ""); err != nil {
		t.Fatalf("解绑: %v", err)
	}
	cfg := store.Get()
	cfg.AppShortcuts = map[string]string{settings.AppActRetry: ""}
	if err := svc.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := store.Get().AppShortcuts
	if v, ok := got[settings.AppActRetry]; !ok || v != "" {
		t.Errorf("解绑后的重试键 = %q（存在 %v），期望空串", v, ok)
	}
	if got[settings.AppActPin] != "cmd+p" {
		t.Errorf("未提到的钉住键 = %q，期望默认 cmd+p", got[settings.AppActPin])
	}
}

// The card's model menu stores the choice on one service only; the other
// services in the same list keep the model they had.
func TestSelectModelChangesOneService(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	cfg := store.Get()
	cfg.Translate = []settings.Provider{
		{ID: "a", Category: settings.CatTranslate, Kind: "openai", Label: "AI", Enabled: true, Model: "gpt-4o-mini"},
		{ID: "b", Category: settings.CatTranslate, Kind: "openai", Label: "AI 2", Enabled: true, Model: "gpt-4o"},
	}
	if err := svc.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.SelectModel("a", "  o3-mini  "); err != nil {
		t.Fatalf("SelectModel: %v", err)
	}
	got := store.Get().Translate
	if got[0].Model != "o3-mini" {
		t.Errorf("选中服务的模型 = %q, 期望 o3-mini", got[0].Model)
	}
	if got[1].Model != "gpt-4o" {
		t.Errorf("另一个服务的模型 = %q, 期望保持 gpt-4o", got[1].Model)
	}
	if err := svc.SelectModel("a", "   "); err == nil {
		t.Error("空模型应当报错")
	}
	if err := svc.SelectModel("missing", "o3-mini"); err != nil {
		t.Errorf("未知 id 不应报错: %v", err)
	}
}
