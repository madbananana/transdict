package gui

import (
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
		c.Selections[settings.ContextMini] = nil
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
	if ids := store.Get().Selections[settings.ContextMini]; len(ids) != 0 {
		t.Errorf("迷你窗勾选 = %v, want 仍为空", ids)
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
	next.FloatClose = 30
	next.KeepOpen = true
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}

	got := store.Get()
	if got.SaveHistory {
		t.Error("记录查询历史 关闭后应当写入存储")
	}
	if got.FloatClose != 30 {
		t.Errorf("悬浮窗收起 = %d, want 30", got.FloatClose)
	}
	if !got.KeepOpen {
		t.Error("悬浮窗保持打开 应当写入存储")
	}
}

// The delay is a slider, but a hand-edited config or an import can carry any
// number; a value the page cannot show must not survive a save.
func TestSaveClampsFloatClose(t *testing.T) {
	store := testStore(t)
	svc := NewSettings(store, "")
	next := svc.Get()
	next.FloatClose = 9999
	if err := svc.Save(next); err != nil {
		t.Fatalf("保存: %v", err)
	}
	if got := store.Get().FloatClose; got != settings.MaxFloatClose {
		t.Errorf("9999 秒保存后 = %d, want %d", got, settings.MaxFloatClose)
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
		// The pin and the login item have their own paths, and Seed is the
		// migration stamp merge() owns — the page must not carry any of them.
		"Pinned": true, "LoginItem": true, "Seed": true,
		"MainX": true, "MainY": true, "MainW": true, "MainH": true,
		"SettingsX": true, "SettingsY": true, "SettingsW": true, "SettingsH": true,
		"HistoryX": true, "HistoryY": true, "HistoryW": true, "HistoryH": true,
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
