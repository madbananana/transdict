package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// presentOf reports the keys a stored file carries, which is how load decides
// whether an empty list is missing or deliberate.
func presentOf(data []byte) func(string) bool {
	keys := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &keys)
	return func(key string) bool {
		_, ok := keys[key]
		return ok
	}
}

func TestMergeSeedsCategoriesAnOldFileLacks(t *testing.T) {
	// The file a pre-dictionary build wrote: no dict or tts key at all.
	data := []byte(`{"ocr":[{"id":"ocr-apple","kind":"apple","label":"苹果","enabled":true}],
		"translate":[{"id":"tr-google","kind":"google","label":"Google","enabled":true}],
		"targetLang":"en"}`)
	cfg := merge(mustDecode(t, data), presentOf(data))

	if len(cfg.Dict) != len(Default().Dict) {
		t.Errorf("词典 = %d 项, want %d", len(cfg.Dict), len(Default().Dict))
	}
	if len(cfg.TTS) != len(Default().TTS) {
		t.Errorf("朗读 = %d 项, want %d", len(cfg.TTS), len(Default().TTS))
	}
	// The stored row stays first; the services this build ships follow it, so an
	// old file can see and enable what it never had.
	if got := ids(cfg.OCR); len(got) != len(Default().OCR) || got[0] != "ocr-apple" {
		t.Errorf("文字识别 = %v, want the stored entry first, then the shipped services", got)
	}
	if cfg.TargetLang != "en" {
		t.Errorf("目标语言 = %q, want the stored value", cfg.TargetLang)
	}
	// A stored entry carries no category field, and the pipeline only finds a
	// provider once the stage is stamped back on.
	for _, p := range cfg.OCR {
		if p.Category != CatOCR {
			t.Errorf("%s category = %q, want %q", p.ID, p.Category, CatOCR)
		}
	}
	for _, p := range cfg.Dict {
		if p.Category != CatDict {
			t.Errorf("%s category = %q, want %q", p.ID, p.Category, CatDict)
		}
	}
}

// A service added in a later build is offered to files that already have a
// list, matched by kind so an old random-suffixed id does not become a second
// row. The stamp makes the offer final: a row the user deleted stays deleted.
func TestMergeOffersANewServiceOnlyOnce(t *testing.T) {
	old := []byte(`{"translate":[{"id":"translate-youdao-08ec","kind":"youdao","enabled":true}]}`)
	cfg := merge(mustDecode(t, old), presentOf(old))
	if !hasKind(cfg.Translate, "bing") || !hasKind(cfg.Translate, "google") {
		t.Fatalf("旧配置 = %v, want 随新版发布的必应与 Google", ids(cfg.Translate))
	}
	if n := countKind(cfg.Translate, "youdao"); n != 1 {
		t.Errorf("有道 = %d 行, want 1（按 kind 去重，不重复旧 id）", n)
	}
	if cfg.Seed != defaultSeed {
		t.Errorf("seed = %d, want %d", cfg.Seed, defaultSeed)
	}

	stamped := []byte(`{"seed":` + strconv.Itoa(defaultSeed) +
		`,"translate":[{"id":"translate-youdao-08ec","kind":"youdao","enabled":true}]}`)
	cfg = merge(mustDecode(t, stamped), presentOf(stamped))
	if hasKind(cfg.Translate, "bing") {
		t.Error("已经打过印记的文件不应再被补齐，删除过的服务不能复活")
	}
}

// Shortcuts now default to Easydict's. A file that still carries the old
// combinations is upgraded once, while a key the user chose survives even when
// it happens to equal an old default.
func TestMergeMovesOldShortcutDefaultsOnce(t *testing.T) {
	old := []byte(`{"shortcuts":{"input":["alt+shift+d"],"selection":["alt+shift+s"],
		"capture":["alt+shift+t"],"silentOcr":["alt+shift+g"],"mini":["alt+shift+m"]}}`)
	cfg := merge(mustDecode(t, old), presentOf(old))
	want := map[string]string{
		SlotInput:     "alt+a",
		SlotSelection: "alt+d",
		SlotCapture:   "alt+s",
		SlotSilentOCR: "alt+shift+s",
	}
	for slot, accel := range want {
		if got := cfg.Accel(slot); len(got) != 1 || got[0] != accel {
			t.Errorf("%s = %v, want [%s]", slot, got, accel)
		}
	}
	// The retired mini and floating window bindings are dropped instead of
	// being carried into a file this build cannot act on.
	if got := cfg.Accel("mini"); len(got) != 0 {
		t.Errorf("mini = %v, want removed", got)
	}
	if got := cfg.Accel("clipboardFloat"); len(got) != 0 {
		t.Errorf("clipboardFloat = %v, want removed", got)
	}
	if cfg.ShortcutSeed != defaultShortcutSeed {
		t.Errorf("shortcutSeed = %d, want %d", cfg.ShortcutSeed, defaultShortcutSeed)
	}

	// Stamped files keep whatever they hold — including the old defaults if the
	// user picks them again after the move.
	stamped := []byte(`{"shortcutSeed":1,"shortcuts":{"input":["alt+shift+d"],"selection":["ctrl+alt+1"]}}`)
	cfg = merge(mustDecode(t, stamped), presentOf(stamped))
	if got := cfg.Accel(SlotInput); len(got) != 1 || got[0] != "alt+shift+d" {
		t.Errorf("input = %v, want 用户自己的组合键", got)
	}
	if got := cfg.Accel(SlotSelection); len(got) != 1 || got[0] != "ctrl+alt+1" {
		t.Errorf("selection = %v, want 用户自己的组合键", got)
	}
}

// The stamp lives in the file: a load that seeds must write the result back, or
// the next launch offers the same rows again.
func TestLoadPersistsSeededServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"tts":[{"id":"tts-youdao","kind":"youdao","enabled":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openStore(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := readStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(cfg.TTS, "google") {
		t.Errorf("朗读 = %v, want the seeded Google voice on disk", ids(cfg.TTS))
	}
	if cfg.Seed != defaultSeed {
		t.Errorf("文件里的 seed = %d, want %d", cfg.Seed, defaultSeed)
	}
}

func hasKind(list []Provider, kind string) bool { return countKind(list, kind) > 0 }

func countKind(list []Provider, kind string) int {
	n := 0
	for _, p := range list {
		if strings.EqualFold(p.Kind, kind) {
			n++
		}
	}
	return n
}

func TestMergeKeepsADeliberatelyEmptiedCategory(t *testing.T) {
	data := []byte(`{"dict":[],"translate":[{"id":"tr-google","kind":"google","enabled":true}]}`)
	cfg := merge(mustDecode(t, data), presentOf(data))

	if len(cfg.Dict) != 0 {
		t.Errorf("词典 = %v, want the empty list the user left", ids(cfg.Dict))
	}
}

func TestMergeDefaultsABoolOnlyWhenItsKeyIsAbsent(t *testing.T) {
	// The same rule for 记录查询历史: old files keep logging, an explicit
	// false is the user's choice and must survive a load.
	saved := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !saved.SaveHistory {
		t.Error("缺少 saveHistory 键时应当采用默认值 true")
	}
	quiet := merge(mustDecode(t, []byte(`{"saveHistory":false}`)), presentOf([]byte(`{"saveHistory":false}`)))
	if quiet.SaveHistory {
		t.Error("文件写明不记录历史时不得改回 true")
	}
	// 优先使用 AppleScript API also starts on; a file that says off keeps it off.
	scripted := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !scripted.PreferAppleScriptAPI {
		t.Error("缺少 preferAppleScriptAPI 键时应当采用默认值 true")
	}
	plain := merge(mustDecode(t, []byte(`{"preferAppleScriptAPI":false}`)), presentOf([]byte(`{"preferAppleScriptAPI":false}`)))
	if plain.PreferAppleScriptAPI {
		t.Error("文件写明关闭 AppleScript 取词时不得改回 true")
	}
}

func TestStoreRoundTripKeepsNewFields(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Update(func(c *Config) {
		c.MouseSelect = true
		c.LoginItem = true
		c.Timeout = 45
	}); err != nil {
		t.Fatal(err)
	}
	next, err := readStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if !next.MouseSelect || !next.LoginItem {
		t.Errorf("重新读取 = %+v, want both switches", next)
	}
	if next.Timeout != 45 {
		t.Errorf("超时 = %d, want 45", next.Timeout)
	}
}

// 禁止名单 ships with 钥匙串访问 blocked, and a file written before the list
// existed still gets that default; an explicitly emptied list stays empty.
func TestMergeShipsTheBlocklistDefaultOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if len(old.Blocklist) != 1 || old.Blocklist[0].BundleID != "com.apple.keychainaccess" {
		t.Errorf("缺少 blocklist 时 = %+v, want 钥匙串访问", old.Blocklist)
	}

	empty := merge(mustDecode(t, []byte(`{"blocklist":[]}`)), presentOf([]byte(`{"blocklist":[]}`)))
	if len(empty.Blocklist) != 0 {
		t.Errorf("文件写明空名单时 = %+v, want 空", empty.Blocklist)
	}

	// An emptied list must be written out, not omitted: the next launch would
	// read the missing key as "before the setting existed" and put 钥匙串访问 back.
	data, err := json.Marshal(Config{})
	if err != nil {
		t.Fatalf("序列化配置: %v", err)
	}
	if !presentOf(data)("blocklist") {
		t.Errorf("空名单没有写进配置: %s", data)
	}

	if !(Config{Blocklist: old.Blocklist}).BlocksApp("com.apple.keychainaccess") {
		t.Error("默认名单没挡住钥匙串访问")
	}
	if (Config{}).BlocksApp("") {
		t.Error("空标识不该被挡住")
	}
}

func TestMergeDefaultsMaxHeightPercentOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if old.MaxHeightPct != 100 {
		t.Errorf("缺少 maxHeightPct 时 = %d, want 100", old.MaxHeightPct)
	}
	kept := merge(mustDecode(t, []byte(`{"maxHeightPct":60}`)), presentOf([]byte(`{"maxHeightPct":60}`)))
	if kept.MaxHeightPct != 60 {
		t.Errorf("文件写明 60 时 = %d, want 60", kept.MaxHeightPct)
	}
}

func TestMergeDefaultsHTTPPortOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if old.HTTPPort != DefaultHTTPPort {
		t.Errorf("缺少 httpPort 时 = %q, want %q", old.HTTPPort, DefaultHTTPPort)
	}
	kept := merge(mustDecode(t, []byte(`{"httpPort":"9090"}`)), presentOf([]byte(`{"httpPort":"9090"}`)))
	if kept.HTTPPort != "9090" {
		t.Errorf("文件写明 9090 时 = %q, want 9090", kept.HTTPPort)
	}
	bad := merge(mustDecode(t, []byte(`{"httpPort":"abc"}`)), presentOf([]byte(`{"httpPort":"abc"}`)))
	if bad.HTTPPort != DefaultHTTPPort {
		t.Errorf("不可用的端口 = %q, want the default", bad.HTTPPort)
	}
}

func TestMergeDefaultsBooksExcerptOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !old.RemoveBooksExcerpt {
		t.Error("缺少 removeBooksExcerpt 时应默认开启")
	}
	off := merge(mustDecode(t, []byte(`{"removeBooksExcerpt":false}`)),
		presentOf([]byte(`{"removeBooksExcerpt":false}`)))
	if off.RemoveBooksExcerpt {
		t.Error("文件写明的 false 被改回了 true")
	}
}

func TestMergeDefaultsPreferYoudaoWordOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !old.PreferYoudaoWord {
		t.Error("缺少 preferYoudaoWord 时应默认开启")
	}
	off := merge(mustDecode(t, []byte(`{"preferYoudaoWord":false}`)),
		presentOf([]byte(`{"preferYoudaoWord":false}`)))
	if off.PreferYoudaoWord {
		t.Error("文件写明的 false 被改回了 true")
	}
}

// The floating window is gone. A file that still names its selection list or
// its shortcut loses those keys on load, so the saved file matches the windows
// the app actually draws.
func TestMergeDropsRetiredFloatingKeys(t *testing.T) {
	data := []byte(`{"selections":{"floating":["tr-google"],"main":["dict-youdao"]},
		"shortcuts":{"clipboardFloat":["alt+f"],"clipboard":["alt+shift+v"]}}`)
	cfg := merge(mustDecode(t, data), presentOf(data))
	if _, ok := cfg.Selections["floating"]; ok {
		t.Error("floating 选择集不应保留")
	}
	if got := cfg.Selections[ContextMain]; len(got) != 1 || got[0] != "dict-youdao" {
		t.Errorf("主窗口选择集 = %v, want 保留", got)
	}
	if _, ok := cfg.Shortcuts["clipboardFloat"]; ok {
		t.Error("clipboardFloat 快捷键不应保留")
	}
	if got := cfg.Accel(SlotClipboard); len(got) != 1 || got[0] != "alt+shift+v" {
		t.Errorf("剪贴板快捷键 = %v, want 保留", got)
	}
}

func TestNormalizeHTTPPortKeepsOnlyUsablePorts(t *testing.T) {
	ok := map[string]string{
		"":       DefaultHTTPPort,
		"  ":     DefaultHTTPPort,
		"8080":   "8080",
		" 9090 ": "9090",
		"1":      "1",
		"65535":  "65535",
	}
	for in, want := range ok {
		if got, err := NormalizeHTTPPort(in); err != nil || got != want {
			t.Errorf("NormalizeHTTPPort(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "65536", "-1", "8o80", "80 80"} {
		if _, err := NormalizeHTTPPort(in); err == nil {
			t.Errorf("NormalizeHTTPPort(%q) was accepted", in)
		}
	}
}

func TestClampMaxHeightPctUsesThePickerStops(t *testing.T) {
	cases := []struct{ in, want int }{
		{-40, 100}, // a hand-edit below the picker falls back to the default
		{0, 100},   // zero is the zero value of a file that predates the option
		{25, 20},   // between stops snaps to the one below
		{60, 60},
		{100, 100},
		{140, 100},
	}
	for _, c := range cases {
		if got := ClampMaxHeightPct(c.in); got != c.want {
			t.Errorf("ClampMaxHeightPct(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampTimeoutKeepsZeroMeaningDefault(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0},  // 内置默认
		{-1, 0}, // a file from a hand-edit, not a request to never wait
		{3, MinTimeout},
		{60, 60},
		{MaxTimeout, MaxTimeout},
		{1 << 20, MaxTimeout},
	}
	for _, c := range cases {
		if got := ClampTimeout(c.in); got != c.want {
			t.Errorf("ClampTimeout(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestStoreLoadFillsAConfigFromAnEarlierBuild(t *testing.T) {
	s := newTestStore(t)
	legacy := `{"ocr":[],"translate":[{"id":"google-1","kind":"google","enabled":true}],` +
		`"shortcutCapture":"ctrl+shift+2","targetLang":"ja"}`
	if err := os.WriteFile(s.path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := reopened.Get()

	if cfg.TargetLang != "ja" {
		t.Errorf("目标语言 = %q, want ja", cfg.TargetLang)
	}
	// The one legacy accelerator is kept, and the slots the new build adds are
	// filled from the defaults.
	if got := cfg.Accel(SlotCapture); len(got) != 1 || got[0] != "ctrl+shift+2" {
		t.Errorf("截图快捷键 = %v, want the stored ctrl+shift+2", got)
	}
	if got := cfg.Accel(SlotODict); len(got) == 0 {
		t.Error("鼠标取词快捷键 = 空, want the default")
	}
	if len(cfg.TTS) == 0 {
		t.Error("朗读 = 空, want the defaults for a file that never had the key")
	}
	// An empty-but-present ocr list is the user's doing, so it stays empty.
	if len(cfg.OCR) != 0 {
		t.Errorf("文字识别 = %v, want the empty list", ids(cfg.OCR))
	}
}

// The platform hook adds rows, so this checks the shape it has to keep: every
// service the default selections name must exist, or the settings page would
// tick a checkbox for a row it cannot show.
func TestDefaultSelectionsNameDefaultProviders(t *testing.T) {
	cfg := Default()
	known := map[string]bool{}
	for _, category := range []Category{CatOCR, CatTranslate, CatDict, CatTTS} {
		list := cfg.List(category)
		if len(list) == 0 {
			t.Errorf("%s = 空, want at least one default service", category)
		}
		for _, p := range list {
			if p.ID == "" || p.Kind == "" || p.Label == "" {
				t.Errorf("%s 条目缺少 id/kind/label：%+v", category, p)
			}
			if p.Category != category {
				t.Errorf("%s 的阶段 = %q, want %q", p.ID, p.Category, category)
			}
			known[p.ID] = true
		}
	}
	for context, checked := range cfg.Selections {
		seen := map[string]bool{}
		for _, id := range checked {
			if !known[id] {
				t.Errorf("%s 勾选了不存在的 %s", context, id)
			}
			if seen[id] {
				t.Errorf("%s 勾选了两次 %s", context, id)
			}
			seen[id] = true
		}
	}
}

// A window runs exactly the stages CategoryOf lists, and an unticked stage means
// "no preference", which is every enabled service of it. So the default ticks have
// to name one service per stage the window runs — otherwise 服务设置 shows an empty
// group beside a stage that is quietly running, and a group with ticks beside a
// stage that never runs.
func TestDefaultSelectionsCoverEveryStageAWindowRuns(t *testing.T) {
	cfg := Default()
	stageOf := map[string]string{}
	for _, category := range []string{CatOCR, CatTranslate, CatDict, CatTTS} {
		for _, p := range cfg.List(category) {
			stageOf[p.ID] = category
		}
	}

	for _, label := range ContextLabels {
		checked := cfg.Selections[label.Key]
		if len(checked) == 0 {
			continue
		}
		ticked := map[string]bool{}
		for _, id := range checked {
			ticked[stageOf[id]] = true
		}
		for _, stage := range CategoryOf(label.Key) {
			if !ticked[stage] {
				t.Errorf("%s 跑 %s 阶段，却没有勾选这一阶段的任何服务", label.Key, stage)
			}
		}
		for id := range ticked {
			if !containsStage(CategoryOf(label.Key), id) {
				t.Errorf("%s 勾选了 %s 阶段的服务，但它并不跑这一阶段", label.Key, id)
			}
		}
	}
}

func containsStage(stages []string, stage string) bool {
	for _, s := range stages {
		if s == stage {
			return true
		}
	}
	return false
}

func TestWatchersSeeEveryUpdate(t *testing.T) {
	s := newTestStore(t)
	var seen []string
	s.Watch(func(c Config) { seen = append(seen, c.TargetLang) })
	s.Watch(func(c Config) { seen = append(seen, "second:"+c.Theme) })

	for _, lang := range []string{"en", "ja"} {
		if _, err := s.Update(func(c *Config) { c.TargetLang = lang }); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"en", "second:auto", "ja", "second:auto"}
	if len(seen) != len(want) {
		t.Fatalf("watchers = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("watchers = %v, want %v", seen, want)
		}
	}
}

func TestGetDoesNotShareProviderExtras(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Update(func(c *Config) {
		p := Provider{ID: "dict-mdx", Category: CatDict, Kind: "mdx", Label: "MDX", Enabled: true}
		p.Set("path", "/tmp/a.mdx")
		c.Dict = append(c.Dict, p)
	}); err != nil {
		t.Fatal(err)
	}

	first := s.Get()
	second := s.Get()
	if len(first.Dict) != len(second.Dict) {
		t.Fatalf("两次读取的词典数量不同: %d / %d", len(first.Dict), len(second.Dict))
	}
	for i := range first.Dict {
		if first.Dict[i].Extra == nil {
			continue
		}
		first.Dict[i].Set("path", "/tmp/changed.mdx")
		if got := second.Dict[i].Get("path"); got == "/tmp/changed.mdx" {
			t.Fatal("Get 返回的快照与存储配置共享了 extra 映射")
		}
	}
}

// 语种识别 is a new option, so a file written before it has no opinion and gets
// the system detector; a mode this build does not know must not leave the
// detector pointing at a service that cannot answer.
func TestParseFallsBackForUnknownDetectOptimize(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{}`, DetectOptimizeNone},
		{`{"detectOptimize":""}`, DetectOptimizeNone},
		{`{"detectOptimize":"bing"}`, DetectOptimizeNone},
		{`{"detectOptimize":"google"}`, DetectOptimizeGoogle},
		{`{"detectOptimize":"baidu"}`, DetectOptimizeBaidu},
	}
	for _, tc := range cases {
		cfg, err := parse([]byte(tc.raw))
		if err != nil {
			t.Fatalf("parse(%s): %v", tc.raw, err)
		}
		if cfg.DetectOptimize != tc.want {
			t.Errorf("parse(%s).DetectOptimize = %q, want %q", tc.raw, cfg.DetectOptimize, tc.want)
		}
	}
}

// 菜单栏图标 is a two-image choice, so a file without it — or with a name this
// build never draws — has to land on the bare glyph Easydict ships.
func TestParseFallsBackForUnknownMenuBarIconStyle(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{}`, MenuBarIconRounded},
		{`{"menuBarIconStyle":""}`, MenuBarIconRounded},
		{`{"menuBarIconStyle":"round"}`, MenuBarIconRounded},
		{`{"menuBarIconStyle":"square"}`, MenuBarIconSquare},
	}
	for _, tc := range cases {
		cfg, err := parse([]byte(tc.raw))
		if err != nil {
			t.Fatalf("parse(%s): %v", tc.raw, err)
		}
		if cfg.MenuBarIconStyle != tc.want {
			t.Errorf("parse(%s).MenuBarIconStyle = %q, want %q", tc.raw, cfg.MenuBarIconStyle, tc.want)
		}
	}
}

func mustDecode(t *testing.T, data []byte) Config {
	t.Helper()
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{path: filepath.Join(t.TempDir(), "config.json"), cfg: Default()}
}

func openStore(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	return s, s.load()
}

func readStore(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func ids(list []Provider) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.ID
	}
	return out
}

// An imported file is treated exactly like the one on disk: what it leaves out
// comes back as the default, what it carries on purpose — even an empty list —
// stays, and the watchers hear about it so shortcuts and the mouse-word poll
// follow without a restart.
func TestImportFillsDefaultsAndNotifiesWatchers(t *testing.T) {
	s := newTestStore(t)
	seen := 0
	s.Watch(func(Config) { seen++ })

	data := []byte(`{"ocr":[],"translate":[{"id":"google-1","kind":"google","enabled":true}],` +
		`"shortcutCapture":"ctrl+shift+2","targetLang":"ja"}`)
	cfg, err := s.Import(data)
	if err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Errorf("watchers 收到 %d 次, want 1", seen)
	}
	if cfg.TargetLang != "ja" || len(cfg.OCR) != 0 || len(cfg.TTS) == 0 {
		t.Errorf("导入结果不对: target=%q ocr=%v tts=%d", cfg.TargetLang, ids(cfg.OCR), len(cfg.TTS))
	}
	if got := cfg.Accel(SlotCapture); len(got) != 1 || got[0] != "ctrl+shift+2" {
		t.Errorf("旧版快捷键没有被折叠: %v", got)
	}

	reopened, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Get().TargetLang != "ja" {
		t.Error("导入的配置没有落盘")
	}
}

// Any JSON object decodes into a Config — a package.json would arrive as an
// entirely default settings, resetting the app because the user picked the
// wrong file. A file with nothing in common with a config is refused instead.
func TestImportRefusesAFileThatIsNotAConfig(t *testing.T) {
	s := newTestStore(t)
	before := s.Get()

	for _, data := range []string{
		`{"name":"transdict","version":"1.0.0","dependencies":{}}`,
		`[1,2,3]`,
		`not json at all`,
	} {
		if _, err := s.Import([]byte(data)); err == nil {
			t.Errorf("导入 %q 应当被拒绝", data)
		}
	}
	after := s.Get()
	if after.TargetLang != before.TargetLang || len(after.Translate) != len(before.Translate) {
		t.Error("被拒绝的导入改动了设置")
	}
}

// 检查更新 adds a bool that defaults to true, which is the one shape a merge
// cannot read off a zero value: an absent key is the only evidence the file
// predates the field.
func TestAutoUpdateSeedsOnlyForAFileThatPredatesIt(t *testing.T) {
	data := []byte(`{"targetLang":"en"}`)
	cfg := merge(mustDecode(t, data), presentOf(data))
	if !cfg.AutoUpdate {
		t.Error("旧文件缺少 autoUpdate 时应该按新装处理：默认开启")
	}
	if cfg.UpdateURL != "" {
		t.Errorf("updateURL = %q, want 空 —— 地址只有用户知道", cfg.UpdateURL)
	}

	// Writing false is a decision. Refilling it from the default would turn the
	// switch back on behind the user's back on the next launch.
	data = []byte(`{"autoUpdate":false,"updateURL":"https://example.test/r.json","targetLang":"en"}`)
	cfg = merge(mustDecode(t, data), presentOf(data))
	if cfg.AutoUpdate {
		t.Error("写下的 false 不应该被默认值覆盖")
	}
	if cfg.UpdateURL != "https://example.test/r.json" {
		t.Errorf("updateURL = %q", cfg.UpdateURL)
	}
}

func TestMergeDefaultsQueryIconOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !old.AutoShowQueryIcon {
		t.Error("缺少 autoShowQueryIcon 时应默认开启")
	}
	if old.QueryIconClickOnly || old.QueryIconMinLength != 0 || old.QueryIconExcludeLang != "zh-CN" {
		t.Errorf("默认 = %v/%d/%q, want false/0/zh-CN",
			old.QueryIconClickOnly, old.QueryIconMinLength, old.QueryIconExcludeLang)
	}

	raw := []byte(`{"autoShowQueryIcon":false,"queryIconClickOnly":true,"queryIconMinLength":33,"queryIconExcludeLang":"en"}`)
	set := merge(mustDecode(t, raw), presentOf(raw))
	if set.AutoShowQueryIcon || !set.QueryIconClickOnly {
		t.Errorf("文件写明的开关被覆盖了: %v/%v", set.AutoShowQueryIcon, set.QueryIconClickOnly)
	}
	if set.QueryIconMinLength != 30 {
		t.Errorf("QueryIconMinLength = %d, 期望按 10 对齐到 30", set.QueryIconMinLength)
	}
	if set.QueryIconExcludeLang != "en" {
		t.Errorf("QueryIconExcludeLang = %q, 期望 en", set.QueryIconExcludeLang)
	}
}

func TestMergeInputBoxDefaultsOnlyWhenAbsent(t *testing.T) {
	raw := []byte(`{"theme":"dark"}`)
	old := merge(mustDecode(t, raw), presentOf(raw))
	if !old.KeepLastOnNoSelection {
		t.Error("缺少 keepLastOnNoSelection 时应默认保留上次结果")
	}
	if old.ClearInputOnQuery || old.SelectTextOnOpen {
		t.Errorf("ClearInputOnQuery/SelectTextOnOpen = %v/%v, want false/false",
			old.ClearInputOnQuery, old.SelectTextOnOpen)
	}

	raw = []byte(`{"clearInputOnQuery":true,"selectTextOnOpen":true,"keepLastOnNoSelection":false}`)
	set := merge(mustDecode(t, raw), presentOf(raw))
	if !set.ClearInputOnQuery || !set.SelectTextOnOpen || set.KeepLastOnNoSelection {
		t.Errorf("文件写明的开关被覆盖了: %v/%v/%v",
			set.ClearInputOnQuery, set.SelectTextOnOpen, set.KeepLastOnNoSelection)
	}
}

func TestLoadSeparatesTheTwoDetectionLanguages(t *testing.T) {
	s := newTestStore(t)
	// The detector falls back to these two guesses, so the same code twice says
	// nothing; a hand-edited file gets a usable pair back.
	dup := `{"firstLang":"ja","secondLang":"ja"}`
	if err := os.WriteFile(s.path, []byte(dup), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := reopened.Get()
	if cfg.FirstLang == cfg.SecondLang {
		t.Errorf("两个语种仍是 %q", cfg.FirstLang)
	}
	if cfg.SecondLang != "zh-CN" {
		t.Errorf("第二语言 = %q，want zh-CN", cfg.SecondLang)
	}

	// When the duplicate is already the default second language, the first one
	// takes the default instead of leaving the pair equal.
	dup = `{"firstLang":"zh-CN","secondLang":"zh-CN"}`
	if err := os.WriteFile(s.path, []byte(dup), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err = openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	cfg = reopened.Get()
	if cfg.FirstLang != "zh-CN" || cfg.SecondLang != "en" {
		t.Errorf("语言对 = %q / %q，want zh-CN / en", cfg.FirstLang, cfg.SecondLang)
	}
}

// 窗口配置 is a map keyed by window role: an older file has none and gets the
// switches on, an explicit off stays off, and a role this build does not draw
// is dropped instead of lingering in the saved file.
func TestParseMergesWindowConfig(t *testing.T) {
	cfg, err := parse([]byte(`{"windowConfig":{
		"main":{"input":false,"langBar":true},
		"floating":{"input":true},
		"legacy":{"input":true}}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.WindowConfig[WindowMain].Input {
		t.Error("明确的关闭被改回了开启")
	}
	if !cfg.WindowConfig[WindowMain].LangBar {
		t.Errorf("缺少的开关应回落到默认: %#v", cfg.WindowConfig)
	}
	if _, ok := cfg.WindowConfig["floating"]; ok {
		t.Error("已移除的悬浮窗角色不应保留")
	}
	if _, ok := cfg.WindowConfig["legacy"]; ok {
		t.Error("未知窗口角色不应保留")
	}
	if len(cfg.WindowConfig) != 1 {
		t.Errorf("窗口数量 = %d, want 1: %#v", len(cfg.WindowConfig), cfg.WindowConfig)
	}

	fresh, err := parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(fresh.WindowConfig) != 1 || !fresh.WindowConfig[WindowMain].Input || !fresh.WindowConfig[WindowMain].LangBar {
		t.Errorf("没有该键时两个开关都应打开: %#v", fresh.WindowConfig)
	}
}

// 字体大小 used to be s/m/l; the current scale has five steps. An old file must
// land on the nearest new step, and a token this build does not know must fall
// back to the default instead of leaving the Aa button with no size.
func TestParseMigratesReadingSize(t *testing.T) {
	cases := []struct{ stored, want string }{
		{"1", "1"},
		{"3", "3"},
		{"5", "5"},
		{"s", "1"},
		{"m", "1"},
		{"l", "2"},
		{"", "1"},
		{"huge", "1"},
	}
	for _, c := range cases {
		cfg, err := parse([]byte(`{"resultFont":"` + c.stored + `"}`))
		if err != nil {
			t.Fatalf("parse(%q): %v", c.stored, err)
		}
		if cfg.ResultFont != c.want {
			t.Errorf("resultFont %q → %q, want %q", c.stored, cfg.ResultFont, c.want)
		}
	}
}
