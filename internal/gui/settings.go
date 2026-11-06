package gui

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"transdict/internal/autostart"
	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/langs"
	"transdict/internal/netp"
	"transdict/internal/ocr"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// SettingsService is the bound surface of the preferences window. It reads and
// writes the store directly rather than caching, so a second window never sees
// a stale snapshot.
type SettingsService struct {
	cfg     *settings.Store
	version string
}

func NewSettings(cfg *settings.Store, version string) *SettingsService {
	return &SettingsService{cfg: cfg, version: version}
}

// Version is the build the user is running. The preferences page shows it next
// to 检查更新, so a report of "已是最新" can be read against a version number.
func (s *SettingsService) Version() string { return s.version }

func (s *SettingsService) Languages() []langs.Lang { return localizedLangs(langs.All()) }

func (s *SettingsService) Get() settings.Config { return localizedProviders(s.cfg.Get()) }

// Categories lists the pipeline stages, in the order the settings page shows them.
func (s *SettingsService) Categories() []settings.Label {
	return localizedLabels(settings.CategoryLabels)
}

// Slots lists the actions that can carry a global accelerator.
func (s *SettingsService) Slots() []settings.Label { return localizedLabels(settings.SlotLabels) }

// Contexts lists the query windows that can carry a service selection.
func (s *SettingsService) Contexts() []settings.Label { return localizedLabels(settings.ContextLabels) }

// CategoriesFor names the stages one window runs. The 勾选 page groups providers
// by what it returns here, so it follows the pipeline instead of guessing in
// TypeScript which stage matters where.
func (s *SettingsService) CategoriesFor(context string) []settings.Label {
	want := map[string]bool{}
	for _, c := range settings.CategoryOf(context) {
		want[c] = true
	}
	// Speak() reads the main window's selection, so 朗读 is only configurable
	// there; a float window that offered the checkbox would be lying.
	if context == settings.ContextMain {
		want[settings.CatTTS] = true
	}
	var out []settings.Label
	for _, l := range settings.CategoryLabels {
		if want[l.Key] {
			l.Name = l10n.T(l.Name)
			out = append(out, l)
		}
	}
	return out
}

// ServiceTypes lists the services a category offers. The list is assembled from
// the provider packages' own registrations, so adding a service needs no UI edit
// and the settings page can never offer a kind the code cannot construct.
func (s *SettingsService) ServiceTypes(category string) []settings.ServiceType {
	switch category {
	case settings.CatOCR:
		return localizedTypes(ocr.Types())
	case settings.CatTranslate:
		return localizedTypes(translate.Types())
	case settings.CatDict:
		return localizedTypes(dict.Types())
	case settings.CatTTS:
		return localizedTypes(tts.Types())
	}
	return nil
}

// Save normalizes and persists the whole configuration. The settings page edits
// a working copy, so a rejected save leaves the store untouched.
func (s *SettingsService) Save(next settings.Config) error {
	if strings.TrimSpace(next.TargetLang) == "" {
		return errors.New("目标语言不能为空")
	}
	next.TargetLang = strings.TrimSpace(next.TargetLang)
	next.SourceLang = strings.TrimSpace(next.SourceLang)
	next.Proxy = strings.TrimSpace(next.Proxy)
	// The proxy is validated before the write: a typo saved to disk would
	// otherwise take every online service down with it on the next launch.
	if err := netp.Check(next.Proxy); err != nil {
		return err
	}
	// A hand-edited file can carry any number, and one that says "wait nine
	// hours" would hang the panel rather than configure it.
	next.Timeout = settings.ClampTimeout(next.Timeout)
	next.FloatClose = settings.ClampFloatClose(next.FloatClose)
	next.UpdateURL = strings.TrimSpace(next.UpdateURL)
	// The update source is checked for a scheme here rather than left to fail
	// later: a typo would otherwise surface as "无法连接更新源" forever, with no
	// hint that the address never was one.
	if next.UpdateURL != "" {
		u, err := url.Parse(next.UpdateURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("更新源需要是一个 http(s) 地址")
		}
	}
	next.OCR = normalize(next.OCR, settings.CatOCR)
	next.Translate = normalize(next.Translate, settings.CatTranslate)
	next.Dict = normalize(next.Dict, settings.CatDict)
	next.TTS = normalize(next.TTS, settings.CatTTS)

	// The login item is applied before anything is written: a config that claims
	// 开机自启 while the system refused it is a promise the app cannot keep, and
	// the checkbox would light up again on the next launch.
	if next.LoginItem != s.cfg.Get().LoginItem {
		if err := autostart.Set(next.LoginItem); err != nil {
			return err
		}
	}

	_, err := s.cfg.Update(func(c *settings.Config) {
		c.OCR = next.OCR
		c.Translate = next.Translate
		c.Dict = next.Dict
		c.TTS = next.TTS
		c.SourceLang = next.SourceLang
		c.TargetLang = next.TargetLang
		c.Shortcuts = next.Shortcuts
		c.Selections = next.Selections
		c.Theme = next.Theme
		c.UILang = next.UILang
		c.Proxy = next.Proxy
		c.Timeout = next.Timeout
		c.UpdateURL = next.UpdateURL
		c.AutoUpdate = next.AutoUpdate
		c.TTSAuto = next.TTSAuto
		c.MiniFont = next.MiniFont
		c.KeepOpen = next.KeepOpen
		c.FloatClose = next.FloatClose
		c.SaveHistory = next.SaveHistory
		c.Floating = next.Floating
		c.MouseSelect = next.MouseSelect
		c.HideSameLang = next.HideSameLang
		c.AutoCopy = next.AutoCopy
		c.ShowInDock = next.ShowInDock
		c.LoginItem = next.LoginItem
	})
	return err
}

// normalize trims every user-typed field, fills a missing label from the
// registration, and guarantees an id so retries and selections can address a
// service stably.
func normalize(items []settings.Provider, category string) []settings.Provider {
	out := make([]settings.Provider, 0, len(items))
	for _, p := range items {
		p.Category = category
		p.Label = strings.TrimSpace(p.Label)
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.Secret = strings.TrimSpace(p.Secret)
		p.AppID = strings.TrimSpace(p.AppID)
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		p.Model = strings.TrimSpace(p.Model)
		p.Prompt = strings.TrimSpace(p.Prompt)
		if t, ok := metaOf(category, p.Kind); ok && p.Label == "" {
			p.Label = t.Name
		}
		// A row that still carries a translated default goes back to the Chinese
		// the store keeps, so a later language switch can word it again. A name the
		// user typed is not in the table and stays exactly as written.
		if zh := l10n.Zh(p.Label); zh != "" {
			p.Label = zh
		}
		if p.ID == "" {
			p.ID = randomID()
		}
		out = append(out, p)
	}
	return out
}

func metaOf(category, kind string) (settings.ServiceType, bool) {
	switch category {
	case settings.CatOCR:
		return ocr.Meta(kind)
	case settings.CatTranslate:
		return translate.Meta(kind)
	case settings.CatDict:
		return dict.Meta(kind)
	case settings.CatTTS:
		return tts.Meta(kind)
	}
	return settings.ServiceType{}, false
}

func randomID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "p" + hex.EncodeToString([]byte(err.Error()))[:12]
	}
	return hex.EncodeToString(b)
}

// AddProvider creates one configured instance of a service type, pre-filled with
// the defaults its package registered.
func (s *SettingsService) AddProvider(category, kind string) (settings.Provider, error) {
	t, ok := metaOf(category, kind)
	if !ok {
		return settings.Provider{}, errors.New("该服务尚未实现：" + kind)
	}
	if !t.Multi {
		for _, p := range s.cfg.Get().List(category) {
			if p.Kind == kind {
				return p, errors.New("该服务已存在，请直接修改")
			}
		}
	}
	p := t.Default
	p.ID = category + "-" + kind + "-" + randomID()[:4]
	p.Category = category
	p.Kind = kind
	p.Label = t.Name
	p.Enabled = true
	_, err := s.cfg.Update(func(c *settings.Config) {
		list := append(c.List(category), p)
		setList(c, category, list)
		// A window that has already opted into this stage should get the new
		// service too; otherwise adding it would change nothing in the panel the
		// user is looking at. Windows with an empty selection run everything and
		// need no entry.
		for context, ids := range c.Selections {
			if len(ids) == 0 {
				continue
			}
			for _, id := range ids {
				if categoryOf(c, id) == category {
					c.Selections[context] = append(ids, p.ID)
					break
				}
			}
		}
	})
	return p, err
}

// categoryOf names the stage a selected provider belongs to, so a new service
// can join the windows that already use that stage.
func categoryOf(c *settings.Config, id string) string {
	for _, list := range [][]settings.Provider{c.OCR, c.Translate, c.Dict, c.TTS} {
		for _, p := range list {
			if p.ID == id {
				return p.Category
			}
		}
	}
	return ""
}

func setList(c *settings.Config, category string, list []settings.Provider) {
	switch category {
	case settings.CatOCR:
		c.OCR = list
	case settings.CatTranslate:
		c.Translate = list
	case settings.CatDict:
		c.Dict = list
	case settings.CatTTS:
		c.TTS = list
	}
}

// UpdateProvider writes one row, which keeps a save from racing another window's
// edit of an unrelated service.
func (s *SettingsService) UpdateProvider(p settings.Provider) error {
	p = normalize([]settings.Provider{p}, p.Category)[0]
	if _, ok := metaOf(p.Category, p.Kind); !ok {
		return errors.New("未知的服务类型：" + p.Kind)
	}
	_, err := s.cfg.Update(func(c *settings.Config) {
		list := c.List(p.Category)
		for i := range list {
			if list[i].ID == p.ID {
				list[i] = p
				setList(c, p.Category, list)
				return
			}
		}
		setList(c, p.Category, append(list, p))
	})
	return err
}

// RemoveProvider deletes a provider by id from every category, along with any
// window selection still naming it.
func (s *SettingsService) RemoveProvider(id string) error {
	_, err := s.cfg.Update(func(c *settings.Config) {
		c.OCR = filterOut(c.OCR, id)
		c.Translate = filterOut(c.Translate, id)
		c.Dict = filterOut(c.Dict, id)
		c.TTS = filterOut(c.TTS, id)
		for context, ids := range c.Selections {
			c.Selections[context] = filterStrings(ids, id)
		}
	})
	return err
}

func filterOut(items []settings.Provider, id string) []settings.Provider {
	out := make([]settings.Provider, 0, len(items))
	for _, p := range items {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}

func filterStrings(items []string, drop string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}

// SetSelection stores which services a window runs, in display order. An empty
// list means "all enabled services", which is what a fresh install uses.
func (s *SettingsService) SetSelection(context string, ids []string) error {
	_, err := s.cfg.Update(func(c *settings.Config) {
		if c.Selections == nil {
			c.Selections = map[string][]string{}
		}
		// "Every enabled service" is an empty list, and it has to be stored as
		// one: merge refills a context the file omits with the shipped default,
		// which would quietly narrow the window again after a relaunch (and drop
		// services the user added since).
		if len(ids) == 0 {
			c.Selections[context] = []string{}
			return
		}
		c.Selections[context] = append([]string(nil), ids...)
	})
	return err
}

// SetShortcut rebinds one action; an empty accelerator unbinds it. The store
// notifies watchers, so the registration changes without a restart.
func (s *SettingsService) SetShortcut(slot, accel string) error {
	if slot == "" {
		return errors.New("快捷键名称不能为空")
	}
	accel = strings.ToLower(strings.TrimSpace(accel))
	if accel != "" && !looksLikeAccel(accel) {
		return errors.New("格式应为 ctrl+alt+a 这类组合键")
	}
	_, err := s.cfg.Update(func(c *settings.Config) {
		if c.Shortcuts == nil {
			c.Shortcuts = map[string][]string{}
		}
		// A cleared slot is stored as an empty list, not deleted: merge refills a
		// key the file omits, so deleting would bring the default shortcut back
		// on the next launch.
		if accel == "" {
			c.Shortcuts[slot] = []string{}
			return
		}
		c.Shortcuts[slot] = []string{accel}
	})
	return err
}

func looksLikeAccel(accel string) bool {
	parts := strings.Split(accel, "+")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl", "alt", "option", "shift", "cmd", "command", "super", "win":
		default:
			return false
		}
	}
	last := parts[len(parts)-1]
	return last != ""
}

// SetTargetLang is also reachable from the panel; keeping it here means the
// settings page and the panel cannot disagree about what was saved.
func (s *SettingsService) SetTargetLang(code string) error {
	if code == "" {
		return errors.New("语言代码不能为空")
	}
	_, err := s.cfg.Update(func(c *settings.Config) { c.TargetLang = code })
	return err
}

// SetSourceLang stores the input language the panel was last set to, so the
// 原文语言 choice in 服务设置 and the panel's own selector stay the same
// setting instead of resetting to 自动检测 on every launch.
func (s *SettingsService) SetSourceLang(code string) error {
	if code == "" {
		code = "auto"
	}
	_, err := s.cfg.Update(func(c *settings.Config) { c.SourceLang = code })
	return err
}

// Reset restores出厂配置, including the service list, so a broken key cannot
// strand the app.
func (s *SettingsService) Reset() error {
	_, err := s.cfg.Update(func(c *settings.Config) { *c = settings.Default() })
	return err
}
