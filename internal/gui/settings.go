package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"transdict/internal/autostart"
	"transdict/internal/dict"
	"transdict/internal/l10n"
	"transdict/internal/langs"
	"transdict/internal/logging"
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

// ServiceHelp is 缺少密钥's 参见 👉 link: where the key this service still needs
// is created. A keyless kind or a custom host has no such page and gets "".
type ServiceHelp struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func NewSettings(cfg *settings.Store, version string) *SettingsService {
	return &SettingsService{cfg: cfg, version: version}
}

// FetchModels is 从云端获取模型: the saved endpoint's own catalog, so a proxy or
// gateway can answer for itself instead of every model being typed by hand.
func (s *SettingsService) FetchModels(id string) ([]string, error) {
	c := s.cfg.Get()
	for _, list := range [][]settings.Provider{c.Translate, c.Dict, c.OCR, c.TTS} {
		for _, p := range list {
			if p.ID != id {
				continue
			}
			prov, err := translate.New(p)
			if err != nil {
				return nil, err
			}
			lister, ok := prov.(translate.ModelLister)
			if !ok {
				return nil, errors.New("此服务不支持从云端获取模型")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return lister.ListModels(ctx)
		}
	}
	return nil, errors.New("服务不存在")
}

// Version is the build the user is running. The preferences page shows it next
// to 检查更新, so a report of "已是最新" can be read against a version number.
func (s *SettingsService) Version() string { return s.version }

// ServiceHelp names the site where an empty key is created, so 缺少密钥 can
// point at it instead of only saying the service cannot run yet.
func (s *SettingsService) ServiceHelp(kind, baseURL string) ServiceHelp {
	title, link := translate.HelpPage(kind, strings.TrimSpace(baseURL))
	return ServiceHelp{Title: title, URL: link}
}

// LogFolder is 高级 → 日志's path. EasyDict shows 日志目录 with a reveal button;
// the row is drawn from this string so the folder cannot drift from the one
// the process actually writes.
func (s *SettingsService) LogFolder() string { return logging.Dir() }

// RevealLogs is 在 Finder 中显示: the folder holding transdict.log, created on
// demand so the button works before anything has been logged.
func (s *SettingsService) RevealLogs() error { return logging.Reveal() }

func (s *SettingsService) Languages() []langs.Lang { return localizedLangs(langs.All()) }

// VocabularyDirWritable is 生词本's 所选目录不可写。warning. A path that does not
// exist yet stays usable: the notebook creates the folder on the first write,
// so probing here must not treat "not created" as "cannot create".
func (s *SettingsService) VocabularyDirWritable(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return true
	}
	info, err := os.Stat(dir)
	if err != nil {
		return true
	}
	if !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".transdict-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

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
	// Speak() reads the translation window's selection, so 朗读 is only
	// configurable there.
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

// SetWindowOption is 窗口配置: one switch of one window, saved the moment it is
// flipped so the next answer in that window already follows it.
func (s *SettingsService) SetWindowOption(role, option string, on bool) error {
	if _, ok := settings.DefaultWindowConfig()[role]; !ok {
		return errors.New("未知窗口")
	}
	if option != "input" && option != "langBar" {
		return errors.New("未知窗口配置")
	}
	_, err := s.cfg.Update(func(c *settings.Config) {
		cfg := settings.MergeWindowConfig(c.WindowConfig, settings.DefaultWindowConfig())
		opts := cfg[role]
		if option == "input" {
			opts.Input = on
		} else {
			opts.LangBar = on
		}
		cfg[role] = opts
		c.WindowConfig = cfg
	})
	return err
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
	port, portErr := settings.NormalizeHTTPPort(next.HTTPPort)
	if portErr != nil {
		return portErr
	}
	next.HTTPPort = port
	next.UpdateURL = strings.TrimSpace(next.UpdateURL)
	next.VocabularyDir = strings.TrimSpace(next.VocabularyDir)
	next.FirstLang = strings.TrimSpace(next.FirstLang)
	next.SecondLang = strings.TrimSpace(next.SecondLang)
	if next.FirstLang != "" && next.FirstLang == next.SecondLang {
		return errors.New("第一语言不能与第二语言相同")
	}
	// The page can only send the three known names; anything else is a hand
	// edit, and an unknown mode would silently stop 语种识别 from doing anything.
	next.DetectOptimize = strings.TrimSpace(next.DetectOptimize)
	switch next.DetectOptimize {
	case settings.DetectOptimizeGoogle, settings.DetectOptimizeBaidu:
	default:
		next.DetectOptimize = settings.DetectOptimizeNone
	}
	// 菜单栏图标 is set from the tray, which this page has no control for: a save
	// must carry whatever the menu chose instead of a copy the page loaded earlier.
	next.MenuBarIconStyle = s.cfg.Get().MenuBarIconStyle
	// 隐藏菜单栏图标 takes away the only always-visible entry of an accessory
	// app: EasyDict refuses the save unless 输入翻译 or 划词翻译 can summon it.
	if next.HideMenuBarIcon && !hasEntryShortcut(next) {
		return errors.New("隐藏菜单栏图标前，请先给「输入翻译」或「划词翻译」设置快捷键")
	}
	// The Anki endpoint is a URL without a path; a trailing slash would be sent
	// to AnkiConnect verbatim ("/") and answer 404.
	next.AnkiEndpoint = strings.TrimRight(strings.TrimSpace(next.AnkiEndpoint), "/")
	next.AnkiDeck = strings.TrimSpace(next.AnkiDeck)
	next.AnkiModel = strings.TrimSpace(next.AnkiModel)
	for i := range next.AnkiMappings {
		next.AnkiMappings[i].Field = strings.TrimSpace(next.AnkiMappings[i].Field)
		next.AnkiMappings[i].Template = strings.TrimSpace(next.AnkiMappings[i].Template)
	}
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
		c.FirstLang = next.FirstLang
		c.SecondLang = next.SecondLang
		c.DetectOptimize = next.DetectOptimize
		c.Shortcuts = next.Shortcuts
		c.AppShortcuts = settings.WithAppShortcutDefaults(next.AppShortcuts)
		c.Selections = next.Selections
		c.Theme = next.Theme
		c.UILang = next.UILang
		c.Proxy = next.Proxy
		c.Timeout = next.Timeout
		c.UpdateURL = next.UpdateURL
		c.AutoUpdate = next.AutoUpdate
		c.IncludeBeta = next.IncludeBeta
		c.TTSAuto = next.TTSAuto
		c.DefaultTTS = next.DefaultTTS
		c.ResultFont = next.ResultFont
		c.SaveHistory = next.SaveHistory
		c.VocabularyNotebook = next.VocabularyNotebook
		c.VocabularyDir = next.VocabularyDir
		c.AnkiEnabled = next.AnkiEnabled
		c.AnkiEndpoint = next.AnkiEndpoint
		c.AnkiDeck = next.AnkiDeck
		c.AnkiModel = next.AnkiModel
		c.AnkiMappings = append([]settings.AnkiMapping(nil), next.AnkiMappings...)
		c.AutoPin = next.AutoPin
		c.MouseSelect = next.MouseSelect
		c.Blocklist = next.Blocklist
		c.HideSameLang = next.HideSameLang
		c.AutoCopy = next.AutoCopy
		c.AutoCopySelect = next.AutoCopySelect
		c.AutoCopyOCR = next.AutoCopyOCR
		c.PreferYoudaoWord = next.PreferYoudaoWord
		c.AutoPronounceWord = next.AutoPronounceWord
		c.AutoShowQueryIcon = next.AutoShowQueryIcon
		c.QueryIconClickOnly = next.QueryIconClickOnly
		c.QueryIconMinLength = next.QueryIconMinLength
		c.ClassicalMinLength = next.ClassicalMinLength
		c.QueryIconExcludeLang = next.QueryIconExcludeLang
		c.OCRTextNormalize = next.OCRTextNormalize
		c.EnableYoudaoOCR = next.EnableYoudaoOCR
		c.HideScreenshotTip = next.HideScreenshotTip
		c.AutoQueryPaste = next.AutoQueryPaste
		c.AutoQueryTyping = next.AutoQueryTyping
		c.AutoQueryOCR = next.AutoQueryOCR
		c.ClearInputOnQuery = next.ClearInputOnQuery
		c.SelectTextOnOpen = next.SelectTextOnOpen
		c.KeepLastOnNoSelection = next.KeepLastOnNoSelection
		c.AutoRemoveComments = next.AutoRemoveComments
		c.ReplaceNewlineSpace = next.ReplaceNewlineSpace
		c.AutoSelectAllText = next.AutoSelectAllText
		c.ReplaceCompatible = next.ReplaceCompatible
		c.ForceCopy = next.ForceCopy
		c.ForceCopyKind = next.ForceCopyKind
		c.PreferAppleScriptAPI = next.PreferAppleScriptAPI
		c.WindowConfig = settings.MergeWindowConfig(next.WindowConfig, settings.DefaultWindowConfig())
		c.MaxHeightPct = next.MaxHeightPct
		c.EnableHTTPServer = next.EnableHTTPServer
		c.HTTPPort = next.HTTPPort
		c.AutoWordSegment = next.AutoWordSegment
		c.RemoveBooksExcerpt = next.RemoveBooksExcerpt
		c.ShowGoogleLink = next.ShowGoogleLink
		c.ShowAppleDictLink = next.ShowAppleDictLink
		c.ShowEudicLink = next.ShowEudicLink
		c.ShowQuickActionButton = next.ShowQuickActionButton
		c.HideMainWindow = next.HideMainWindow
		c.Pronunciation = next.Pronunciation
		c.MarkdownRender = next.MarkdownRender
		c.ShowInDock = next.ShowInDock
		c.HideMenuBarIcon = next.HideMenuBarIcon
		c.ShowOCRMenu = next.ShowOCRMenu
		c.MenuBarIconStyle = next.MenuBarIconStyle
		c.LoginItem = next.LoginItem
	})
	return err
}

// hasEntryShortcut reports whether a trivially reachable action still has a
// global key: with the tray hidden, one of these is the way back into the app.
func hasEntryShortcut(cfg settings.Config) bool {
	for _, slot := range []string{settings.SlotInput, settings.SlotSelection} {
		for _, accel := range cfg.Shortcuts[slot] {
			if strings.TrimSpace(accel) != "" {
				return true
			}
		}
	}
	return false
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

// SelectModel switches the model one service answers with, from its card's
// model menu. Like EasyDict's menu it only stores the choice: the answer on
// screen keeps the words the old model wrote, and the next query or 重试 asks
// with the new one.
func (s *SettingsService) SelectModel(providerID, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("模型不能为空")
	}
	_, err := s.cfg.Update(func(c *settings.Config) {
		for _, list := range [][]settings.Provider{c.Translate, c.OCR, c.Dict, c.TTS} {
			for i := range list {
				if list[i].ID == providerID {
					list[i].Model = model
				}
			}
		}
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

// SetAppShortcut rebinds one 快捷键 menu action; an empty accelerator leaves
// the action in the menu without a key. The store notifies watchers, so the
// menu items take the new key without a restart.
func (s *SettingsService) SetAppShortcut(action, accel string) error {
	if !settings.KnownAppAction(action) {
		return errors.New("未知的应用内快捷键")
	}
	accel = strings.ToLower(strings.TrimSpace(accel))
	if accel != "" && !looksLikeAccel(accel) {
		return errors.New("格式应为 ctrl+alt+a 这类组合键")
	}
	if accel != "" {
		// Two menu items on one key means AppKit runs whichever it meets first,
		// which is not a choice the user made. A swap needs an unbind between.
		for name, other := range settings.WithAppShortcutDefaults(s.cfg.Get().AppShortcuts) {
			if name != action && other == accel {
				return errors.New("该组合键已用于其他应用内快捷键")
			}
		}
	}
	_, err := s.cfg.Update(func(c *settings.Config) {
		if c.AppShortcuts == nil {
			c.AppShortcuts = map[string]string{}
		}
		c.AppShortcuts[action] = accel
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
