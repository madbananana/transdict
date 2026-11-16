package settings

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"transdict/internal/appdir"
)

// watcher is declared once so the snapshot copy below stays readable.
type watcher = func(Config)

// Store owns one JSON file in the user config directory. Keys are stored
// plaintext with 0600 because the OS account is the trust boundary; a
// keychain would not survive the ad-hoc signing every local build uses.
type Store struct {
	mu       sync.Mutex
	path     string
	cfg      Config
	watchers []watcher
}

func NewStore() (*Store, error) {
	dir, err := appdir.Root()
	if err != nil {
		return nil, err
	}
	return NewStoreAt(filepath.Join(dir, "config.json"))
}

// NewStoreAt opens a config file at an explicit path. Tests use it so a fixture
// never lands in the settings a real user is reading; production uses NewStore.
func NewStoreAt(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s.saveLocked()
	}
	if err != nil {
		return err
	}
	cfg, err := parse(data)
	if err != nil {
		return err
	}
	s.cfg = cfg

	// parse hands back the new services and the stamp that remembers them; the
	// file has to carry both, or every launch would repeat the offer. A failed
	// write is not fatal — it only means the same rows are offered again.
	var stored struct {
		Seed         int `json:"seed"`
		ShortcutSeed int `json:"shortcutSeed"`
	}
	_ = json.Unmarshal(data, &stored)
	if stored.Seed != cfg.Seed || stored.ShortcutSeed != cfg.ShortcutSeed {
		return s.saveLocked()
	}
	return nil
}

// parse turns config bytes into a Config, folding in everything the file leaves
// out so a new option never needs a migration script.
func parse(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	// Which keys the file itself carries is the difference between "this config
	// predates the feature" and "the user removed every service in that list".
	// Only the first may be refilled.
	keys := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &keys)
	present := func(key string) bool { _, ok := keys[key]; return ok }

	// One release stored shortcuts as plain strings; fold them into the slots.
	var legacy struct {
		ShortcutCapture string `json:"shortcutCapture"`
		ShortcutInput   string `json:"shortcutInput"`
	}
	if json.Unmarshal(data, &legacy) == nil && (legacy.ShortcutCapture != "" || legacy.ShortcutInput != "") {
		if cfg.Shortcuts == nil {
			cfg.Shortcuts = map[string][]string{}
		}
		set := func(slot, accel string) {
			if accel != "" && len(cfg.Shortcuts[slot]) == 0 {
				cfg.Shortcuts[slot] = []string{accel}
			}
		}
		set(SlotCapture, legacy.ShortcutCapture)
		set(SlotInput, legacy.ShortcutInput)
	}
	return merge(cfg, present), nil
}

// merge fills in whatever a file from an older build does not have, so adding a
// option never needs a migration script. present reports the keys the stored
// file really carried.
func merge(cfg Config, present func(key string) bool) Config {
	d := Default()
	if cfg.TargetLang == "" {
		cfg.TargetLang = d.TargetLang
	}
	if cfg.SourceLang == "" {
		cfg.SourceLang = d.SourceLang
	}
	if cfg.FirstLang == "" {
		cfg.FirstLang = d.FirstLang
	}
	if cfg.SecondLang == "" {
		cfg.SecondLang = d.SecondLang
	}
	// A mode this build does not know would leave 语种识别 doing nothing at all;
	// falling back to the system recogniser is the same answer an older file
	// expects.
	switch cfg.DetectOptimize {
	case DetectOptimizeGoogle, DetectOptimizeBaidu:
	default:
		cfg.DetectOptimize = d.DetectOptimize
	}
	// 第一语言 and 第二语言 are the two guesses the detector falls back to, so
	// the same language twice says nothing. A hand-edited file gets the default
	// pair rather than a detector that cannot break a tie.
	if cfg.FirstLang == cfg.SecondLang {
		cfg.SecondLang = d.SecondLang
		if cfg.SecondLang == cfg.FirstLang {
			cfg.SecondLang = d.FirstLang
		}
	}
	if cfg.Theme == "" {
		cfg.Theme = d.Theme
	}
	if cfg.UILang == "" {
		cfg.UILang = d.UILang
	}
	if cfg.TTSAuto == "" {
		cfg.TTSAuto = d.TTSAuto
	}
	if cfg.DefaultTTS == "" {
		cfg.DefaultTTS = d.DefaultTTS
	}
	switch cfg.ResultFont {
	case "1", "2", "3", "4", "5":
	// Files from before the scale had five steps: the old middle was 14.5px,
	// which is the new first step, and the old large was closest to the second.
	case "s", "m":
		cfg.ResultFont = "1"
	case "l":
		cfg.ResultFont = "2"
	default:
		cfg.ResultFont = d.ResultFont
	}
	if !present("autoQueryPaste") {
		cfg.AutoQueryPaste = d.AutoQueryPaste
	}
	if !present("autoQueryTyping") {
		cfg.AutoQueryTyping = d.AutoQueryTyping
	}
	if !present("autoQueryOCR") {
		cfg.AutoQueryOCR = d.AutoQueryOCR
	}
	if !present("clearInputOnQuery") {
		cfg.ClearInputOnQuery = d.ClearInputOnQuery
	}
	if !present("selectTextOnOpen") {
		cfg.SelectTextOnOpen = d.SelectTextOnOpen
	}
	if !present("keepLastOnNoSelection") {
		cfg.KeepLastOnNoSelection = d.KeepLastOnNoSelection
	}
	if !present("autoCopySelect") {
		cfg.AutoCopySelect = d.AutoCopySelect
	}
	if !present("autoCopyOCR") {
		cfg.AutoCopyOCR = d.AutoCopyOCR
	}
	if !present("preferYoudaoWord") {
		cfg.PreferYoudaoWord = d.PreferYoudaoWord
	}
	if !present("autoShowQueryIcon") {
		cfg.AutoShowQueryIcon = d.AutoShowQueryIcon
	}
	if !present("queryIconClickOnly") {
		cfg.QueryIconClickOnly = d.QueryIconClickOnly
	}
	if !present("queryIconMinLength") {
		cfg.QueryIconMinLength = d.QueryIconMinLength
	}
	if !present("queryIconExcludeLang") {
		cfg.QueryIconExcludeLang = d.QueryIconExcludeLang
	}
	cfg.QueryIconMinLength = ClampQueryIconMinLength(cfg.QueryIconMinLength)
	if cfg.QueryIconExcludeLang == "" {
		cfg.QueryIconExcludeLang = d.QueryIconExcludeLang
	}
	if !present("classicalMinLength") {
		cfg.ClassicalMinLength = d.ClassicalMinLength
	}
	if !present("autoRemoveComments") {
		cfg.AutoRemoveComments = d.AutoRemoveComments
	}
	if !present("replaceNewlineSpace") {
		cfg.ReplaceNewlineSpace = d.ReplaceNewlineSpace
	}
	if !present("autoSelectAllText") {
		cfg.AutoSelectAllText = d.AutoSelectAllText
	}
	if !present("replaceCompatible") {
		cfg.ReplaceCompatible = d.ReplaceCompatible
	}
	if !present("forceCopy") {
		cfg.ForceCopy = d.ForceCopy
	}
	if !present("forceCopyKind") {
		cfg.ForceCopyKind = d.ForceCopyKind
	}
	cfg.ForceCopyKind = ClampForceCopyKind(cfg.ForceCopyKind)
	if !present("preferAppleScriptAPI") {
		cfg.PreferAppleScriptAPI = d.PreferAppleScriptAPI
	}
	if !present("windowConfig") {
		cfg.WindowConfig = d.WindowConfig
	} else {
		cfg.WindowConfig = MergeWindowConfig(cfg.WindowConfig, d.WindowConfig)
	}
	if !present("showOCRMenu") {
		cfg.ShowOCRMenu = d.ShowOCRMenu
	}
	// An older file has no icon style, and an unknown one must not leave the
	// menu offering a checkmark on neither entry.
	switch cfg.MenuBarIconStyle {
	case MenuBarIconSquare:
	default:
		cfg.MenuBarIconStyle = d.MenuBarIconStyle
	}
	// The Anki destination is three strings and the field mapping. A file from
	// before the feature has none of them, and an empty mapping would otherwise
	// send addNote a note with no field name at all.
	if !present("ankiEnabled") {
		cfg.AnkiEnabled = d.AnkiEnabled
	}
	if !present("ankiEndpoint") || strings.TrimSpace(cfg.AnkiEndpoint) == "" {
		cfg.AnkiEndpoint = d.AnkiEndpoint
	}
	if !present("ankiDeck") || strings.TrimSpace(cfg.AnkiDeck) == "" {
		cfg.AnkiDeck = d.AnkiDeck
	}
	if !present("ankiModel") || strings.TrimSpace(cfg.AnkiModel) == "" {
		cfg.AnkiModel = d.AnkiModel
	}
	if !present("ankiMappings") || len(cfg.AnkiMappings) == 0 {
		cfg.AnkiMappings = append([]AnkiMapping(nil), d.AnkiMappings...)
	}
	if !present("maxHeightPct") {
		cfg.MaxHeightPct = d.MaxHeightPct
	}
	cfg.MaxHeightPct = ClampMaxHeightPct(cfg.MaxHeightPct)
	if !present("autoWordSegment") {
		cfg.AutoWordSegment = d.AutoWordSegment
	}
	if !present("removeBooksExcerpt") {
		cfg.RemoveBooksExcerpt = d.RemoveBooksExcerpt
	}
	if !present("blocklist") {
		cfg.Blocklist = d.Blocklist
	}
	if !present("showGoogleLink") {
		cfg.ShowGoogleLink = d.ShowGoogleLink
	}
	if !present("showAppleDictLink") {
		cfg.ShowAppleDictLink = d.ShowAppleDictLink
	}
	if !present("showEudicLink") {
		cfg.ShowEudicLink = d.ShowEudicLink
	}
	if !present("showQuickActionButton") {
		cfg.ShowQuickActionButton = d.ShowQuickActionButton
	}
	if !present("hideMainWindow") {
		cfg.HideMainWindow = d.HideMainWindow
	}
	if !present("pronunciation") {
		cfg.Pronunciation = d.Pronunciation
	}
	cfg.Pronunciation = ClampPronunciation(cfg.Pronunciation)
	if !present("markdownRender") {
		cfg.MarkdownRender = d.MarkdownRender
	}
	if !present("httpPort") || strings.TrimSpace(cfg.HTTPPort) == "" {
		cfg.HTTPPort = d.HTTPPort
	} else if port, err := NormalizeHTTPPort(cfg.HTTPPort); err == nil {
		cfg.HTTPPort = port
	} else {
		cfg.HTTPPort = d.HTTPPort
	}

	// The 迷你窗 and the floating query window are gone. Their keys are dropped
	// instead of carried along, so an old file does not keep a binding for a
	// window the app no longer draws.
	delete(cfg.Shortcuts, "mini")
	delete(cfg.Shortcuts, "clipboardFloat")
	delete(cfg.Selections, "mini")
	delete(cfg.Selections, "floating")

	// The shortcuts moved to Easydict's defaults. A file that still carries the
	// old combination is one nobody customized, so it follows; a key the user
	// changed is left as it is. The stamp makes the move final.
	if cfg.ShortcutSeed < defaultShortcutSeed {
		legacy := map[string][2]string{
			SlotInput:     {"alt+shift+d", "alt+a"},
			SlotSelection: {"alt+shift+s", "alt+d"},
			SlotCapture:   {"alt+shift+t", "alt+s"},
			SlotSilentOCR: {"alt+shift+g", "alt+shift+s"},
		}
		for slot, pair := range legacy {
			if list := cfg.Shortcuts[slot]; len(list) == 1 && list[0] == pair[0] {
				cfg.Shortcuts[slot] = []string{pair[1]}
			}
		}
		cfg.ShortcutSeed = defaultShortcutSeed
	}

	cfg.Shortcuts = mergeMap(cfg.Shortcuts, d.Shortcuts)
	cfg.AppShortcuts = mergeMap(cfg.AppShortcuts, d.AppShortcuts)
	cfg.Selections = mergeMap(cfg.Selections, d.Selections)

	// A category with no key at all was written before the category existed, so
	// its default services are what that build would have shipped. An empty
	// list under an existing key is a choice, and choices survive.
	seed := func(list *[]Provider, def []Provider, key string) {
		if len(*list) == 0 && !present(key) {
			*list = def
		}
	}
	seed(&cfg.OCR, d.OCR, "ocr")
	seed(&cfg.Translate, d.Translate, "translate")
	seed(&cfg.Dict, d.Dict, "dict")
	seed(&cfg.TTS, d.TTS, "tts")

	// A service added in a later build reaches the files that already have a
	// list: rows are matched by kind, so an old random-suffixed id does not
	// produce a duplicate. The stamp makes the offer final.
	if cfg.Seed < defaultSeed {
		cfg.OCR = addMissingKinds(cfg.OCR, d.OCR)
		cfg.Translate = addMissingKinds(cfg.Translate, d.Translate)
		cfg.Dict = addMissingKinds(cfg.Dict, d.Dict)
		cfg.TTS = addMissingKinds(cfg.TTS, d.TTS)
		cfg.Seed = defaultSeed
	}

	// A bool whose default is true cannot be detected from its zero value, so
	// an absent key is the only evidence that the field is new.
	if !cfg.AutoUpdate && !present("autoUpdate") {
		cfg.AutoUpdate = d.AutoUpdate
	}
	// A file written before 记录查询历史 shipped has no key, and starting to
	// keep no log would look like the history was lost.
	if !cfg.SaveHistory && !present("saveHistory") {
		cfg.SaveHistory = d.SaveHistory
	}

	stamp := func(list []Provider, category string) []Provider {
		out := append([]Provider(nil), list...)
		for i := range out {
			if out[i].Category == "" {
				out[i].Category = category
			}
		}
		return out
	}
	cfg.OCR = stamp(cfg.OCR, CatOCR)
	cfg.Translate = stamp(cfg.Translate, CatTranslate)
	cfg.Dict = stamp(cfg.Dict, CatDict)
	cfg.TTS = stamp(cfg.TTS, CatTTS)
	return cfg
}

// addMissingKinds appends the defaults whose kind the stored list does not
// have. An empty list is the user's choice to run nothing from that stage, so
// it stays empty; a file that predates the category was handled by seed.
func addMissingKinds(have, def []Provider) []Provider {
	if len(have) == 0 {
		return have
	}
	known := make(map[string]bool, len(have))
	for _, p := range have {
		known[strings.ToLower(p.Kind)] = true
	}
	out := append([]Provider(nil), have...)
	for _, p := range def {
		kind := strings.ToLower(p.Kind)
		if known[kind] {
			continue
		}
		known[kind] = true
		out = append(out, p)
	}
	return out
}

func mergeMap[T any](got, want map[string]T) map[string]T {
	if got == nil {
		got = map[string]T{}
	}
	for k, v := range want {
		if _, ok := got[k]; !ok {
			got[k] = v
		}
	}
	return got
}

// Get returns a copy so callers cannot mutate shared state by accident.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clone(s.cfg)
}

// Watch registers a callback fired after every successful Update. Subsystems
// that cache derived state — shortcuts, autostart, theme — subscribe here.
func (s *Store) Watch(fn watcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, fn)
}

func (s *Store) Update(fn func(*Config)) (Config, error) {
	s.mu.Lock()
	cfg := s.clone(s.cfg)
	fn(&cfg)
	s.cfg = cfg
	err := s.saveLocked()
	watchers := append([]watcher(nil), s.watchers...)
	s.mu.Unlock()

	if err == nil {
		for _, w := range watchers {
			w(cfg)
		}
	}
	return cfg, err
}

// Import replaces the settings with the contents of a file the user chose, and
// returns what took effect. A file from an older build behaves like the one on
// disk: whatever it leaves out comes back as the default. Watchers fire exactly
// as they do for an edit made in the settings window, so shortcuts, autostart
// and the mouse-word poll all pick the new state up without a restart.
func (s *Store) Import(data []byte) (Config, error) {
	keys := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &keys); err != nil {
		return Config{}, err
	}
	// Any JSON object unmarshals into a Config, and one that shares no field with
	// it would quietly reset the whole app. A package.json is not a mistake worth
	// punishing.
	known := 0
	for name := range keys {
		if configKeys()[name] {
			known++
		}
	}
	if known == 0 {
		return Config{}, errors.New("这个文件不像 Transdict 的配置")
	}
	cfg, err := parse(data)
	if err != nil {
		return Config{}, err
	}
	return s.Update(func(c *Config) { *c = cfg })
}

// configKeys are the top-level names a Config writes. A file carrying none of
// them did not come out of this app.
func configKeys() map[string]bool {
	t := reflect.TypeOf(Config{})
	out := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) clone(c Config) Config {
	out := c
	out.OCR = cloneProviders(c.OCR)
	out.Translate = cloneProviders(c.Translate)
	out.Dict = cloneProviders(c.Dict)
	out.TTS = cloneProviders(c.TTS)
	out.Shortcuts = cloneLists(c.Shortcuts)
	out.Selections = cloneLists(c.Selections)
	return out
}

// Extra is the one field a Provider owns by reference, so a shallow copy would
// let two snapshots of the config share it — and a settings edit then show up
// in whatever a watcher was handed earlier.
func cloneProviders(list []Provider) []Provider {
	out := make([]Provider, len(list))
	for i, p := range list {
		if p.Extra != nil {
			p.Extra = maps.Clone(p.Extra)
		}
		out[i] = p
	}
	return out
}

func cloneLists(m map[string][]string) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = append([]string(nil), v...)
	}
	return out
}
