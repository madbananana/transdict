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
		Seed int `json:"seed"`
	}
	_ = json.Unmarshal(data, &stored)
	if stored.Seed != cfg.Seed {
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
	if cfg.Theme == "" {
		cfg.Theme = d.Theme
	}
	if cfg.UILang == "" {
		cfg.UILang = d.UILang
	}
	if cfg.TTSAuto == "" {
		cfg.TTSAuto = d.TTSAuto
	}
	if cfg.MiniFont <= 0 {
		cfg.MiniFont = d.MiniFont
	}
	// Zero in a file written before the field existed is "unset", not "close at
	// once": the key's absence is the only evidence, since 0 is a valid int.
	if !present("floatClose") {
		cfg.FloatClose = d.FloatClose
	}
	cfg.FloatClose = ClampFloatClose(cfg.FloatClose)

	cfg.Shortcuts = mergeMap(cfg.Shortcuts, d.Shortcuts)
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
	if !cfg.Floating && !present("floating") {
		cfg.Floating = d.Floating
	}
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
