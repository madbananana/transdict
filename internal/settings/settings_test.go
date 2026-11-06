package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	stamped := []byte(`{"seed":1,"translate":[{"id":"translate-youdao-08ec","kind":"youdao","enabled":true}]}`)
	cfg = merge(mustDecode(t, stamped), presentOf(stamped))
	if hasKind(cfg.Translate, "bing") {
		t.Error("已经打过印记的文件不应再被补齐，删除过的服务不能复活")
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
	// floating defaults to true, so a file written before the switch existed
	// must not silently turn 悬浮窗 off — but a file that says false means it.
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if !old.Floating {
		t.Error("缺少 floating 键时应当采用默认值 true")
	}
	off := merge(mustDecode(t, []byte(`{"floating":false}`)), presentOf([]byte(`{"floating":false}`)))
	if off.Floating {
		t.Error("文件写明 false 时不得改回 true")
	}
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
}

func TestStoreRoundTripKeepsNewFields(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Update(func(c *Config) {
		c.MouseSelect = true
		c.LoginItem = true
		c.Timeout = 45
		c.FloatClose = 30
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
	if next.FloatClose != 30 {
		t.Errorf("悬浮窗收起 = %d, want 30", next.FloatClose)
	}
}

// The delay is a number, so zero cannot mean "new field" the way a bool can:
// the key's absence is what marks a file written before the setting existed.
func TestMergeDefaultsFloatCloseOnlyWhenAbsent(t *testing.T) {
	old := merge(mustDecode(t, []byte(`{"theme":"dark"}`)), presentOf([]byte(`{"theme":"dark"}`)))
	if old.FloatClose != 12 {
		t.Errorf("缺少 floatClose 时 = %d, want the default 12", old.FloatClose)
	}
	kept := merge(mustDecode(t, []byte(`{"floatClose":30}`)), presentOf([]byte(`{"floatClose":30}`)))
	if kept.FloatClose != 30 {
		t.Errorf("文件写明 30 时 = %d, want 30", kept.FloatClose)
	}
	low := merge(mustDecode(t, []byte(`{"floatClose":0}`)), presentOf([]byte(`{"floatClose":0}`)))
	if low.FloatClose != MinFloatClose {
		t.Errorf("0 秒 = %d, want the %d s floor", low.FloatClose, MinFloatClose)
	}
	high := merge(mustDecode(t, []byte(`{"floatClose":9999}`)), presentOf([]byte(`{"floatClose":9999}`)))
	if high.FloatClose != MaxFloatClose {
		t.Errorf("9999 秒 = %d, want the %d s ceiling", high.FloatClose, MaxFloatClose)
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
