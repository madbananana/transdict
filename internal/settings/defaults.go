package settings

// Config is everything the app persists. Keys stay camelCase because the
// generated TypeScript bindings reuse them verbatim.
type Config struct {
	OCR       []Provider `json:"ocr"`
	Translate []Provider `json:"translate"`
	Dict      []Provider `json:"dict"`
	TTS       []Provider `json:"tts"`

	// Seed records which set of default services this file has already been
	// offered. A build that ships a new service appends it once; a row the user
	// deletes afterwards stays deleted.
	Seed int `json:"seed"`

	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`

	// Shortcuts maps a slot (see internal/shortcut.Slots) to the accelerators
	// bound to it. A slot may hold two: Easydict users hit whichever is
	// nearer, and the second is what survives a conflict with another app.
	Shortcuts map[string][]string `json:"shortcuts"`

	// Selections maps a query context (main/floating/mini/screenshot/clipboard)
	// to the provider ids that run in it, which is how the same service set
	// can differ per window the way Easydict's checkboxes do.
	Selections map[string][]string `json:"selections"`

	Theme  string `json:"theme"`
	UILang string `json:"uiLang"`

	// Proxy is the outbound address for every online service. Empty means
	// 自动: ask the operating system, then the environment.
	Proxy string `json:"proxy"`

	// Timeout is how long one service may take, in seconds. Zero keeps the
	// built-in per-stage limits, which are longer for a screenshot than for a
	// sentence; the providers' own HTTP client timeouts follow it too.
	Timeout int `json:"timeout"`

	KeepOpen bool `json:"keepOpen"`
	// FloatClose is how many seconds a 划词 answer stays parked when 悬浮窗保持打开
	// is off. Twelve is long enough to finish a sentence and short enough that a
	// window left beside the pointer is not still there in the next app.
	FloatClose int `json:"floatClose"`
	// Pinned keeps the translation window above every other window. It is a
	// preference rather than a per-session flag, because a user who wants it
	// while comparing two documents wants it again tomorrow.
	Pinned       bool   `json:"pinned"`
	Floating     bool   `json:"floating"`
	MouseSelect  bool   `json:"mouseSelect"`
	TTSAuto      string `json:"ttsAuto"`
	MiniFont     int    `json:"miniFont"`
	HideSameLang bool   `json:"hideSameLang"`

	// MainX … MainH remember the translation window's frame in DIP. A zero
	// size means it was never moved, which keeps the centered default; a frame
	// on a display that is gone is ignored on the next launch.
	MainX int `json:"mainX"`
	MainY int `json:"mainY"`
	MainW int `json:"mainW"`
	MainH int `json:"mainH"`

	// The preferences and history windows remember their frames too. They are
	// separate because a settings form and a history list want different sizes,
	// and the translation window's narrow frame would fit neither.
	SettingsX int `json:"settingsX"`
	SettingsY int `json:"settingsY"`
	SettingsW int `json:"settingsW"`
	SettingsH int `json:"settingsH"`
	HistoryX  int `json:"historyX"`
	HistoryY  int `json:"historyY"`
	HistoryW  int `json:"historyW"`
	HistoryH  int `json:"historyH"`

	// UpdateURL is the release manifest 检查更新 reads. It starts empty because
	// only the user knows where their builds are published, and an address this
	// code guessed would fail forever without saying why.
	UpdateURL string `json:"updateURL"`

	// AutoUpdate asks once per launch and only speaks up when the source is
	// ahead. A manual check reports what it found either way.
	AutoUpdate bool `json:"autoUpdate"`

	// AutoCopy puts the first translation on the pasteboard as soon as a result
	// lands, which is the half of Easydict's 自动复制译文 that makes a floating
	// panel worth using. It defaults to off: taking the user's clipboard without
	// asking is a surprise, not a feature, until they switch it on.
	AutoCopy bool `json:"autoCopy"`

	// SaveHistory writes every answer to the history file. It stays on by
	// default — the log is what 历史 and 收藏 are — but a user translating
	// private text needs a way to say "not this one", and deleting rows
	// afterwards is not the same promise.
	SaveHistory bool `json:"saveHistory"`

	// ShowInDock is the reverse of the usual LSUIElement switch: the app lives in
	// the menu bar, so the Dock tile is the opt-in rather than the default.
	ShowInDock bool `json:"showInDock"`

	// LoginItem mirrors the operating system's login item, which is the fact
	// autostart.Status reports and the reason a plain bool is not enough.
	LoginItem bool `json:"loginItem"`
}

// defaultSeed is bumped whenever Default() grows a provider. Files stamped with
// an older value are offered the new rows once, on the next load.
const defaultSeed = 1

// Default is what a fresh install gets. Apple's on-device services are added by
// a per-platform hook: only a darwin build registers the kinds, and a row whose
// kind has no adapter is a switch the user can flip but never make work.
func Default() Config {
	c := Config{
		OCR: []Provider{
			{ID: "ocr-vision", Category: CatOCR, Kind: "vision", Label: "视觉大模型", Enabled: false,
				BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
			{ID: "ocr-baidu", Category: CatOCR, Kind: "baidu", Label: "百度通用文字识别", Enabled: false},
		},
		Translate: []Provider{
			{ID: "tr-google", Category: CatTranslate, Kind: "google", Label: "Google", Enabled: true},
			// 必应 is the third keyless translator: Google is blocked on many
			// mainland networks and 有道 throttles bursts, so a fresh install
			// should not depend on either one alone.
			{ID: "tr-bing", Category: CatTranslate, Kind: "bing", Label: "必应翻译", Enabled: true},
			{ID: "tr-youdao", Category: CatTranslate, Kind: "youdao", Label: "有道", Enabled: true},
			{ID: "tr-openai", Category: CatTranslate, Kind: "openai", Label: "AI 模型", Enabled: false,
				BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"},
		},
		Dict: []Provider{
			{ID: "dict-youdao", Category: CatDict, Kind: "youdao", Label: "有道词典", Enabled: true},
		},
		TTS: []Provider{
			// Google reads whole sentences; 有道 only pronounces dictionary
			// entries, so it is the fallback rather than the first voice.
			{ID: "tts-google", Category: CatTTS, Kind: "google", Label: "Google 朗读", Enabled: true},
			{ID: "tts-youdao", Category: CatTTS, Kind: "youdao", Label: "有道朗读", Enabled: true},
		},

		SourceLang: "auto",
		TargetLang: "zh-CN",

		Shortcuts: map[string][]string{
			SlotInput:     {"alt+shift+d"},
			SlotSelection: {"alt+shift+s"},
			SlotCapture:   {"alt+shift+t"},
			SlotSilentOCR: {"alt+shift+g"},
			SlotMini:      {"alt+shift+m"},
			SlotClipboard: {"alt+shift+v"},
			SlotReplace:   {"alt+shift+r"},
			SlotODict:     {"alt+shift+w"},
		},
		Selections: map[string][]string{
			ContextMain:     {"dict-youdao", "tr-google", "tr-bing", "tr-youdao"},
			ContextFloating: {"tr-google", "tr-bing", "tr-youdao", "dict-youdao"},
			ContextMini:     {"tr-google"},
			// 截图 OCR is a two-stage window: the picture is read and the text is
			// then translated, so the ticks name both stages. Leaving the second
			// out would show an English panel a Chinese paragraph and nothing else.
			ContextScreenshot: {"ocr-vision", "ocr-baidu", "tr-google", "tr-bing", "tr-youdao"},
			ContextClipboard:  {"dict-youdao", "tr-google", "tr-bing", "tr-youdao"},
			ContextSilentOCR:  {"ocr-baidu"},
			ContextReplace:    {"tr-google", "tr-bing", "tr-youdao"},
			ContextODict:      {"dict-youdao"},
		},

		Seed:        defaultSeed,
		Theme:       "auto",
		UILang:      "zh-CN",
		Floating:    true,
		MouseSelect: false,
		TTSAuto:     "off",
		MiniFont:    15,
		FloatClose:  12,
		AutoUpdate:  true,
		SaveHistory: true,
	}
	addAppleServices(&c)
	return c
}

// The 悬浮窗自动收起 slider offers this range. Shorter than five seconds is
// gone before a sentence is read; longer than a minute might as well be 保持打开.
const (
	MinFloatClose = 5
	MaxFloatClose = 60
)

// ClampFloatClose keeps a hand-edited config or a stale import inside the range
// the slider can show, rather than parking a window for an hour or for no time.
func ClampFloatClose(n int) int {
	if n < MinFloatClose {
		return MinFloatClose
	}
	if n > MaxFloatClose {
		return MaxFloatClose
	}
	return n
}

// Slots and contexts name the shortcut/selection keys. They are the contract
// between the Go wiring, the settings UI and the persisted file.
const (
	SlotInput     = "input"
	SlotSelection = "selection"
	SlotCapture   = "capture"
	SlotSilentOCR = "silentOcr"
	SlotMini      = "mini"
	SlotClipboard = "clipboard"
	SlotReplace   = "replace"
	SlotODict     = "odict"
	SlotSettings  = "settings"
)

const (
	ContextMain        = "main"
	ContextFloating    = "floating"
	ContextMini        = "mini"
	ContextScreenshot  = "screenshot"
	ContextClipboard   = "clipboard"
	ContextSilentOCR   = "silentOcr"
	ContextODict       = "odict"
	ContextReplace     = "replace"
	ContextTranslation = "translation"
)

// SlotLabels drives the shortcut table in the settings UI. Keeping the names
// here means the page lists exactly the slots Go can bind.
var SlotLabels = []Label{
	{SlotInput, "输入翻译"},
	{SlotSelection, "划词翻译"},
	{SlotCapture, "截图翻译"},
	{SlotSilentOCR, "截图 OCR（静默）"},
	{SlotMini, "迷你窗口"},
	{SlotClipboard, "剪贴板翻译"},
	{SlotReplace, "翻译并替换"},
	{SlotODict, "鼠标取词"},
	{SlotSettings, "打开设置"},
}

// ContextLabels drives the per-window service selection table.
var ContextLabels = []Label{
	{ContextMain, "主窗口"},
	{ContextFloating, "悬浮窗"},
	{ContextMini, "迷你窗"},
	{ContextScreenshot, "截图 OCR"},
	{ContextClipboard, "剪贴板"},
	{ContextSilentOCR, "静音 OCR"},
	{ContextReplace, "翻译并替换"},
	{ContextODict, "鼠标取词"},
	{ContextTranslation, "AI 润色"},
}

// CategoryLabels names the four pipeline stages in display order.
var CategoryLabels = []Label{
	{CatDict, "词典"},
	{CatTranslate, "翻译"},
	{CatOCR, "文字识别"},
	{CatTTS, "朗读"},
}

// Accel returns the accelerators bound to a slot.
func (c Config) Accel(slot string) []string { return c.Shortcuts[slot] }

// Selected lists the provider ids a query context runs. An empty selection
// means "everything enabled", so a fresh install works before the user has
// opened the settings page.
func (c Config) Selected(context string) []string {
	if ids := c.Selections[context]; len(ids) > 0 {
		return ids
	}
	return nil
}

func (c Config) List(category string) []Provider {
	switch category {
	case CatOCR:
		return c.OCR
	case CatTranslate:
		return c.Translate
	case CatDict:
		return c.Dict
	case CatTTS:
		return c.TTS
	}
	return nil
}

// CategoryOf maps a context to the pipeline stages it uses, so one lookup
// decides both which providers run and how results render. A stage a window does
// not list is a stage it has nowhere to show.
func CategoryOf(context string) []string {
	switch context {
	case ContextScreenshot:
		return []string{CatOCR, CatTranslate}
	case ContextSilentOCR:
		// 静音 OCR copies what it read and opens nothing: translating the text
		// would be a second decision the user did not ask for.
		return []string{CatOCR}
	case ContextODict:
		return []string{CatDict}
	case ContextMini, ContextReplace, ContextTranslation:
		// One line of text each, so the dictionary services would only add delay.
		return []string{CatTranslate}
	default:
		return []string{CatDict, CatTranslate}
	}
}

// The timeout range a user can ask for. Below five seconds even a healthy
// connection to Google loses, and past ten minutes the panel has been hung long
// enough that the user has closed it; a hand-edited config is clamped rather
// than trusted.
const (
	MinTimeout = 5
	MaxTimeout = 900
)

// ClampTimeout maps a requested number of seconds onto the usable range. Zero
// means "the built-in defaults" and stays zero.
func ClampTimeout(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	if seconds < MinTimeout {
		return MinTimeout
	}
	if seconds > MaxTimeout {
		return MaxTimeout
	}
	return seconds
}
