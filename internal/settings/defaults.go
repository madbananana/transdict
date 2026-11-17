package settings

import (
	"errors"
	"strconv"
	"strings"
)

// BlockedApp is one row of 禁止名单. The name is stored too so the settings page
// does not have to resolve a bundle id back to an app that may be uninstalled.
type BlockedApp struct {
	BundleID string `json:"bundleID"`
	Name     string `json:"name"`
}

// AnkiMapping is one note-model field and the template that fills it. The
// template may use EasyDict's variables ({Word}, {Translation}, {Definition},
// {Phonetic}, {Exchange}, {Related}, {FullResult}, {DictionaryText},
// {DictionaryHTML}, {Etymology}); an empty one means {Word} on the first row
// and {FullResult} on the rest.
type AnkiMapping struct {
	Field    string `json:"field"`
	Template string `json:"template"`
}

// WindowOptions is 窗口配置: what one window shows besides its answers. Easydict
// keys the same two switches by window type and starts both on.
type WindowOptions struct {
	Input   bool `json:"input"`
	LangBar bool `json:"langBar"`
}

// Window roles for 窗口配置. They are not query contexts: every answer lands in
// the translation window, which is the only window the app draws.
const (
	WindowMain = "main"
)

// DefaultWindowConfig is 窗口配置 for a fresh install: the translation window
// keeps its input and its language bar, which is where Easydict starts.
func DefaultWindowConfig() map[string]WindowOptions {
	return map[string]WindowOptions{
		WindowMain: {Input: true, LangBar: true},
	}
}

// MergeWindowConfig fills the roles a hand-edited file leaves out and drops
// roles this build does not draw. Only an absent role falls back to the default:
// an entry that says "input off, bar off" is a choice, not a gap.
func MergeWindowConfig(saved map[string]WindowOptions, defaults map[string]WindowOptions) map[string]WindowOptions {
	out := make(map[string]WindowOptions, len(defaults))
	for role, def := range defaults {
		if v, ok := saved[role]; ok {
			out[role] = v
			continue
		}
		out[role] = def
	}
	return out
}

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

	// ShortcutSeed stamps the one-time move to Easydict's shortcut defaults.
	// Without it a user who picks an old combination again would be moved off
	// it on every launch.
	ShortcutSeed int `json:"shortcutSeed"`

	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`

	// FirstLang and SecondLang are the two languages the user reads, and the
	// pair 自动选择 aims at. They also bias language detection: a three-word
	// query is genuinely ambiguous, and the answer the user expects is almost
	// always one of these two.
	FirstLang  string `json:"firstLang"`
	SecondLang string `json:"secondLang"`

	// DetectOptimize names the service that may refine the system detector:
	// "none" keeps it alone, "google"/"baidu" ask that configured translator to
	// read a sample of the text first, which is Easydict's 语种识别 choice.
	DetectOptimize string `json:"detectOptimize"`

	// Shortcuts maps a slot (see internal/shortcut.Slots) to the accelerators
	// bound to it. A slot may hold two: Easydict users hit whichever is
	// nearer, and the second is what survives a conflict with another app.
	Shortcuts map[string][]string `json:"shortcuts"`

	// AppShortcuts is the in-app table: one accelerator per 快捷键 menu action
	// (clear input, copy, retry …). These are menu keys, not global hotkeys —
	// rebinding one teaches the menu item a new key. An empty value keeps the
	// action in the menu without a key.
	AppShortcuts map[string]string `json:"appShortcuts"`

	// Selections maps a query context (main/screenshot/clipboard) to the
	// provider ids that run in it, which is how the same service set can
	// differ per trigger the way Easydict's checkboxes do.
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

	// Pinned keeps the translation window above every other window. It is a
	// preference rather than a per-session flag, because a user who wants it
	// while comparing two documents wants it again tomorrow.
	Pinned bool `json:"pinned"`
	// AutoPin is 当窗口显示时自动钉住: every time the translation window appears it
	// starts pinned, so comparing it with a document does not begin with a click.
	// Easydict leaves it off.
	AutoPin     bool `json:"autoPin"`
	MouseSelect bool `json:"mouseSelect"`
	// Blocklist is 禁止名单: apps where 自动监听鼠标划词 stays quiet. A password
	// manager should not answer every selection, so 钥匙串访问 ships blocked.
	Blocklist []BlockedApp `json:"blocklist"`
	TTSAuto   string       `json:"ttsAuto"`
	// DefaultTTS is 默认 TTS 服务: the voice 朗读 tries first. Empty means the
	// configured order decides, which is what a file from before the option says.
	DefaultTTS string `json:"defaultTTS"`
	// Pronunciation is 英文发音: which English voice reads an answer, "us" or
	// "uk". Easydict starts on the American voice.
	Pronunciation string `json:"pronunciation"`
	// ResultFont is the reading size of the panel's answers, as the Aa button
	// cycles it: "1" … "5", EasyDict's five steps. A token rather than pixels
	// because the stylesheet owns the numbers.
	ResultFont   string `json:"resultFont"`
	HideSameLang bool   `json:"hideSameLang"`

	// MainX … MainH remember the translation window's frame in DIP. A zero
	// size means it was never moved, which keeps the centered default; a frame
	// on a display that is gone is ignored on the next launch.
	MainX int `json:"mainX"`
	MainY int `json:"mainY"`
	MainW int `json:"mainW"`
	MainH int `json:"mainH"`

	// The preferences window remembers its frame too. It is separate because a
	// settings form wants a width the translation window would not use.
	SettingsX int `json:"settingsX"`
	SettingsY int `json:"settingsY"`
	SettingsW int `json:"settingsW"`
	SettingsH int `json:"settingsH"`

	// UpdateURL is the release manifest 检查更新 reads. Empty means the official
	// GitHub release feed; a custom address is for a fork or a mirror, because
	// the app cannot know where somebody else's builds are published.
	UpdateURL string `json:"updateURL"`

	// AutoUpdate asks at launch and every few hours, and only speaks up when the
	// source is ahead. A manual check reports what it found either way.
	AutoUpdate bool `json:"autoUpdate"`

	// SkippedUpdate is the version the user told the automatic check to ignore.
	// Only that exact version stays quiet: a later release asks again, and a
	// manual check still reports it.
	SkippedUpdate string `json:"skippedUpdate"`

	// IncludeBeta lets a pre-release answer 检查更新. Developers tag builds like
	// 1.4.0-beta.1 long before they are worth installing, so they stay hidden
	// until the user asks for them.
	IncludeBeta bool `json:"includeBeta"`

	// AutoCopy puts the first translation on the pasteboard as soon as a result
	// lands, which is the half of Easydict's 自动复制译文 worth keeping now that
	// every query opens in the main window. It defaults to off: taking the
	// user's clipboard without asking is a surprise, not a feature, until they
	// switch it on.
	AutoCopy bool `json:"autoCopy"`

	// AutoCopySelect and AutoCopyOCR are Easydict's 自动复制划词文本 and
	// 自动复制 OCR 文本: the selected text, or the text a screenshot recognized,
	// goes to the pasteboard alongside the answer. Both stay off — a selection
	// often came from the clipboard in the first place, and re-copying it would
	// overwrite whatever the user was carrying.
	AutoCopySelect bool `json:"autoCopySelect"`
	AutoCopyOCR    bool `json:"autoCopyOCR"`

	// PreferYoudaoWord is 英语单词优先使用有道发音: 有道's dictionary voice reads a
	// single English word better than a generic TTS, so it goes first for that
	// case even when another voice is configured. Easydict keeps it on.
	PreferYoudaoWord bool `json:"preferYoudaoWord"`

	// AutoPronounceWord is 查询英语单词后自动播放发音: a lone English word speaks
	// itself the moment its answer lands, without turning 自动朗读 on. Easydict
	// leaves it off, since a word looked up while typing would talk back.
	AutoPronounceWord bool `json:"autoPronounceWord"`

	// The 鼠标划词查询 group: AutoShowQueryIcon puts a small 查询 button beside
	// the selection, QueryIconMinLength and QueryIconExcludeLang decide when it
	// is worth showing, and QueryIconClickOnly makes the button the trigger
	// instead of the watcher querying by itself. Easydict shows the icon by
	// default and skips Chinese selections.
	AutoShowQueryIcon    bool   `json:"autoShowQueryIcon"`
	QueryIconClickOnly   bool   `json:"queryIconClickOnly"`
	QueryIconMinLength   int    `json:"queryIconMinLength"`
	QueryIconExcludeLang string `json:"queryIconExcludeLang"`

	// OCRTextNormalize is Easydict's 启用 OCR 文本规范化: the recognised text is
	// squeezed back into paragraphs before it is translated — wrapped lines
	// joined, hyphenated words reattached, spaces between Chinese characters
	// dropped. It stays off, as Easydict leaves it, because a screenshot of code
	// or a table reads better with its line breaks intact.
	OCRTextNormalize bool `json:"ocrTextNormalize"`

	// EnableYoudaoOCR is Easydict's 有道 OCR 识别: when every configured OCR
	// service fails, the public Youdao image endpoint is asked once more, which
	// is what rescues screenshots Apple's recogniser reads as empty. Off by
	// default, as it is in Easydict, because it sends the picture to a third
	// party.
	EnableYoudaoOCR bool `json:"enableYoudaoOCR"`

	// HideScreenshotTip is Easydict's 隐藏截图提示层: the marquee's hint bar —
	// drag to select, Enter to translate, Esc to cancel — is useful the first
	// few times and clutter after that. Off, so the tip shows.
	HideScreenshotTip bool `json:"hideScreenshotTip"`

	// AutoQueryPaste and AutoQueryTyping run the box's text without Enter, as
	// Easydict's 粘贴后自动查询 and 输入时自动查询 do. Both stay off by default:
	// a query costs a network round trip and the user may still be mid-sentence.
	AutoQueryPaste  bool `json:"autoQueryPaste"`
	AutoQueryTyping bool `json:"autoQueryTyping"`
	// AutoQueryOCR is 图片 OCR 后自动查询. Turning it off leaves 截图翻译 at the
	// recognized text, so the user decides when to spend a translation request.
	// Easydict keeps it on: the crop was taken in order to read it.
	AutoQueryOCR bool `json:"autoQueryOCR"`

	// ClassicalMinLength is 检测古文的最小文本长度: 文言文 is never tested below
	// this many characters, because a few characters cannot show a genre, and 0
	// turns the test off. Easydict's default is 20.
	ClassicalMinLength int `json:"classicalMinLength"`

	// The 输入框 group follows Easydict's General tab. ClearInputOnQuery starts
	// 输入翻译 empty instead of re-offering the last question; SelectTextOnOpen
	// leaves the restored text selected so typing replaces it; and
	// KeepLastOnNoSelection shows what the window was displaying when 划词翻译
	// finds nothing highlighted. Easydict leaves the first two off and the third
	// on.
	ClearInputOnQuery     bool `json:"clearInputOnQuery"`
	SelectTextOnOpen      bool `json:"selectTextOnOpen"`
	KeepLastOnNoSelection bool `json:"keepLastOnNoSelection"`

	// AutoRemoveComments strips comment markers from a query the way
	// Easydict's 查询时自动移除代码注释符号 does: a selection taken from source
	// code answers better without the //, # and /* */ around the sentence.
	// Easydict leaves it on, so the default here does too.
	AutoRemoveComments bool `json:"autoRemoveComments"`

	// ReplaceNewlineSpace joins a multi-line selection into one line, which is
	// 查询时自动替换换行符. It stays off: a paragraph's line breaks are often the
	// only layout the user meant to keep.
	ReplaceNewlineSpace bool `json:"replaceNewlineSpace"`

	// AutoSelectAllText is 自动选中所有文本: when 翻译并替换 finds no highlight,
	// it selects the whole field first, so the translation replaces the text
	// instead of being pasted at a caret. Easydict leaves it on.
	AutoSelectAllText bool `json:"autoSelectAllText"`

	// ReplaceCompatible is 兼容模式替换: the answer is typed as keystrokes
	// instead of pasted, for the few apps that ignore cmd+V. Easydict leaves it
	// off, so pasting stays the default.
	ReplaceCompatible bool `json:"replaceCompatible"`

	// ForceCopy is 启用强制取词: an app that exposes no selection through the
	// accessibility API still answers when its own Copy command is run. EasyDict
	// leaves it on, and it is what keeps 划词 working in Electron apps.
	ForceCopy bool `json:"forceCopy"`

	// ForceCopyKind is 强制取词类型. ForceCopyMenu runs the frontmost app's Copy
	// menu command, which also works where a synthesized chord is ignored;
	// ForceCopyShortcut goes straight to cmd+C. EasyDict tries the menu first.
	ForceCopyKind string `json:"forceCopyKind"`

	// PreferAppleScriptAPI is 优先使用 AppleScript API: in browsers the selection
	// is read through the app's own scripting interface before falling back to
	// 强制取词. Safari exposes no selection otherwise, and scripting avoids the
	// pasteboard side effects of the copy routes. EasyDict leaves it on.
	PreferAppleScriptAPI bool `json:"preferAppleScriptAPI"`

	// ShowGoogleLink, ShowAppleDictLink and ShowEudicLink are the quick link
	// icons in the titlebar: they open the current text in Google, the macOS
	// Dictionary app, or Eudic. Easydict shows all three, so they start on.
	ShowGoogleLink    bool `json:"showGoogleLink"`
	ShowAppleDictLink bool `json:"showAppleDictLink"`
	ShowEudicLink     bool `json:"showEudicLink"`

	// ShowQuickActionButton is 显示快捷设置图标: the titlebar switch.2 menu holding
	// 添加到收藏 and the three text rewrites. Easydict names the key for the
	// 打开应用设置 item inside that menu, not for a separate gear — the menu is
	// the only way in, so hiding it hides the way to settings from the titlebar.
	ShowQuickActionButton bool `json:"showQuickActionButton"`

	// HideMainWindow is 启动后隐藏主窗口: the app is a menu bar item, and the
	// translation window should open from a shortcut rather than at login.
	// Easydict hides it, so the default does too.
	HideMainWindow bool `json:"hideMainWindow"`

	// MarkdownRender is 启用 Markdown 渲染: an AI answer arrives with headings,
	// lists and fences, and the card can lay them out instead of quoting the
	// markup. Easydict leaves it on; each card has its own override.
	MarkdownRender bool `json:"markdownRender"`

	// EnableHTTPServer and HTTPPort are Easydict's local API: a small HTTP
	// service other apps can call to translate, OCR and read the selection. It
	// stays off — opening a port is a decision — and binds to 127.0.0.1 only.
	EnableHTTPServer bool   `json:"enableHTTPServer"`
	HTTPPort         string `json:"httpPort"`

	// MaxHeightPct is 窗口最大高度百分比: the share of the display a window may
	// grow to. Easydict offers 20%–100% and starts at 100%.
	MaxHeightPct int `json:"maxHeightPct"`

	// AutoWordSegment is 查询时自动分词: a single code identifier such as
	// key_value or anchoredDraggableState is asked as its words. Easydict keeps
	// it on, and the dictionary decides which capitalised words are real words.
	AutoWordSegment bool `json:"autoWordSegment"`

	// RemoveBooksExcerpt is 移除 Books 摘录版权信息: Apple Books appends an
	// excerpt notice to every copied highlight, and translating it wastes the
	// services on a paragraph nobody selected. Only texts taken from Books are
	// rewritten, so a notice typed by hand stays. Easydict keeps it on.
	RemoveBooksExcerpt bool `json:"removeBooksExcerpt"`

	// SaveHistory writes every answer to the history file. It stays on by
	// default — the log is what 历史 and 收藏 are — but a user translating
	// private text needs a way to say "not this one", and deleting rows
	// afterwards is not the same promise.
	SaveHistory bool `json:"saveHistory"`

	// VocabularyNotebook is 生词本: every finished answer is appended to a JSONL
	// file in VocabularyDir, which an external review app can read. It stays off
	// because it writes the user's queries to a second place, and with no
	// directory chosen there is nowhere to write.
	VocabularyNotebook bool   `json:"vocabularyNotebook"`
	VocabularyDir      string `json:"vocabularyDir"`

	// Anki Connect is Anki's local add-on API. 添加到 Anki on a dictionary card
	// sends the headword and its meaning to this deck. AnkiMappings is
	// Easydict's Anki 字段映射: each row names one field of the note model and
	// the template that fills it, so a custom note type works as well as Basic.
	AnkiEnabled  bool          `json:"ankiEnabled"`
	AnkiEndpoint string        `json:"ankiEndpoint"`
	AnkiDeck     string        `json:"ankiDeck"`
	AnkiModel    string        `json:"ankiModel"`
	AnkiMappings []AnkiMapping `json:"ankiMappings"`

	// WindowConfig is 窗口配置: whether the translation window carries a text input
	// and a target-language bar. Easydict keys the same two switches by window
	// type; this app draws one window, so it has one row.
	WindowConfig map[string]WindowOptions `json:"windowConfig"`

	// ShowInDock is the reverse of the usual LSUIElement switch: the app lives in
	// the menu bar, so the Dock tile is the opt-in rather than the default.
	ShowInDock bool `json:"showInDock"`

	// HideMenuBarIcon is 隐藏菜单栏图标: the menu bar entry goes away and the
	// shortcuts are the only way in, which is useful next to other status items.
	// The Dock tile, when it is on, stays.
	HideMenuBarIcon bool `json:"hideMenuBarIcon"`

	// ShowOCRMenu is 显示 OCR 菜单项: the tray grows 截图 OCR、剪贴板 OCR and
	// OCR 窗口 next to the translation entries. Easydict shows them by default;
	// they are a preference because they double the length of a short menu.
	ShowOCRMenu bool `json:"showOCRMenu"`

	// MenuBarIconStyle is 菜单栏图标: MenuBarIconRounded draws the bare glyph and
	// MenuBarIconSquare frames it. Easydict offers the same two images from the
	// tray and starts on the rounded one.
	MenuBarIconStyle string `json:"menuBarIconStyle"`

	// LoginItem mirrors the operating system's login item, which is the fact
	// autostart.Status reports and the reason a plain bool is not enough.
	LoginItem bool `json:"loginItem"`
}

// defaultSeed is bumped whenever Default() grows a provider. Files stamped with
// an older value are offered the new rows once, on the next load.
const defaultSeed = 4

// defaultShortcutSeed marks the release that adopted Easydict's shortcuts.
const defaultShortcutSeed = 1

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
			// 苹果翻译 works offline once the pair's language pack is
			// downloaded, so it is offered disabled rather than on: a query
			// with a missing pack answers with an actionable error.
			{ID: "tr-apple", Category: CatTranslate, Kind: "apple", Label: "苹果翻译", Enabled: false},
		},
		Dict: []Provider{
			{ID: "dict-youdao", Category: CatDict, Kind: "youdao", Label: "有道词典", Enabled: true},
		},
		TTS: []Provider{
			// Google reads whole sentences; 有道 only pronounces dictionary
			// entries, so it is the fallback rather than the first voice.
			{ID: "tts-google", Category: CatTTS, Kind: "google", Label: "Google 朗读", Enabled: true},
			{ID: "tts-youdao", Category: CatTTS, Kind: "youdao", Label: "有道朗读", Enabled: true},
			{ID: "tts-bing", Category: CatTTS, Kind: "bing", Label: "必应朗读", Enabled: false},
		},

		SourceLang: "auto",
		TargetLang: "zh-CN",

		// The accelerators follow Easydict's: ⌥A opens the input window, ⌥D
		// reads the selection, ⌥S captures and ⌥⇧S runs the silent OCR.
		// 剪贴板/替换/查词 have no Easydict counterpart, so they keep the
		// shift+option row they had.
		Shortcuts: map[string][]string{
			SlotInput:     {"alt+a"},
			SlotSelection: {"alt+d"},
			SlotCapture:   {"alt+s"},
			SlotSilentOCR: {"alt+shift+s"},
			SlotClipboard: {"alt+shift+v"},
			SlotReplace:   {"alt+shift+r"},
			SlotODict:     {"alt+shift+w"},
		},
		Selections: map[string][]string{
			ContextMain: {"dict-youdao", "tr-google", "tr-bing", "tr-youdao"},
			// 截图 OCR is a two-stage pipeline: the picture is read and the text is
			// then translated, so the ticks name both stages. Leaving the second
			// out would show an English panel a Chinese paragraph and nothing else.
			ContextScreenshot: {"ocr-vision", "ocr-baidu", "tr-google", "tr-bing", "tr-youdao"},
			ContextClipboard:  {"dict-youdao", "tr-google", "tr-bing", "tr-youdao"},
			ContextSilentOCR:  {"ocr-baidu"},
			ContextReplace:    {"tr-google", "tr-bing", "tr-youdao"},
			ContextODict:      {"dict-youdao"},
		},
		AppShortcuts: DefaultAppShortcuts(),

		Seed:             defaultSeed,
		ShortcutSeed:     defaultShortcutSeed,
		Theme:            "auto",
		UILang:           "zh-CN",
		FirstLang:        "en",
		SecondLang:       "zh-CN",
		DetectOptimize:   DetectOptimizeNone,
		MouseSelect:      false,
		TTSAuto:          "off",
		DefaultTTS:       "tts-youdao",
		Pronunciation:    PronunciationUS,
		ResultFont:       "1",
		ShowOCRMenu:      true,
		MenuBarIconStyle: MenuBarIconRounded,
		WindowConfig:     DefaultWindowConfig(),
		// Easydict ships one entry: a password manager must not answer a
		// selection, and 钥匙串访问 is where macOS keeps those secrets.
		Blocklist:             []BlockedApp{{BundleID: "com.apple.keychainaccess", Name: "钥匙串访问"}},
		HTTPPort:              DefaultHTTPPort,
		ShowGoogleLink:        true,
		ShowAppleDictLink:     true,
		ShowEudicLink:         true,
		ShowQuickActionButton: true,
		HideMainWindow:        true,
		MarkdownRender:        true,
		// Easydict keeps 查询时自动移除代码注释符号 on; the newline option is off.
		AutoRemoveComments:   true,
		AutoWordSegment:      true,
		AutoSelectAllText:    true,
		RemoveBooksExcerpt:   true,
		ForceCopy:            true,
		ForceCopyKind:        ForceCopyMenu,
		PreferAppleScriptAPI: true,
		// 划词翻译 keeps the previous answer when nothing is selected; the other
		// two 输入框 switches stay off, as Easydict leaves them.
		KeepLastOnNoSelection: true,
		// 图片 OCR 后自动查询 stays on: a screenshot is taken in order to read the
		// text, and a second gesture before the translation would break that.
		AutoQueryOCR:         true,
		ClassicalMinLength:   20,
		PreferYoudaoWord:     true,
		AutoShowQueryIcon:    true,
		QueryIconExcludeLang: "zh-CN",
		MaxHeightPct:         100,
		AutoUpdate:           true,
		SaveHistory:          true,
		// 添加到 Anki stays off; the rest is what a stock AnkiConnect serves, so
		// turning the switch on works without a visit to the fields below.
		AnkiEnabled:  false,
		AnkiEndpoint: "http://127.0.0.1:8765",
		AnkiDeck:     "Default",
		AnkiModel:    "Basic",
		AnkiMappings: []AnkiMapping{
			{Field: "Front", Template: "{Word}"},
			{Field: "Back", Template: "{FullResult}"},
		},
	}
	addAppleServices(&c)
	addPlatformServices(&c)
	return c
}

// 英文发音 offers two voices: the American one Easydict starts with and the
// British one. Other languages each have a single voice, so the choice only
// applies to English.
const (
	PronunciationUS = "us"
	PronunciationUK = "uk"
)

// 菜单栏图标 picks between the bare 文 mark and the same mark inside a square.
const (
	MenuBarIconRounded = "rounded"
	MenuBarIconSquare  = "square"
)

// ClampPronunciation keeps a hand-edited value on one of the two voices.
func ClampPronunciation(p string) string {
	if p == PronunciationUK {
		return PronunciationUK
	}
	return PronunciationUS
}

// MaxHeightPctStops are the choices 窗口最大高度百分比 offers, matching
// Easydict's picker: 20% to 100% in tens.
var MaxHeightPctStops = []int{20, 30, 40, 50, 60, 70, 80, 90, 100}

// ClampMaxHeightPct keeps a hand-edited config or a stale import on one of the
// stops. Anything below 20% would be smaller than a window's own minimum, so it
// falls back to the 100% default instead of a picker value nobody chose.
func ClampMaxHeightPct(n int) int {
	if n < MaxHeightPctStops[0] {
		return 100
	}
	if n > 100 {
		return 100
	}
	return n / 10 * 10
}

// QueryIconMaxLength is the top of 最小字符长度: beyond a couple of dozen
// characters the icon is about a paragraph, and 50 is Easydict's stop.
const QueryIconMaxLength = 50

func ClampQueryIconMinLength(n int) int {
	if n < 0 {
		return 0
	}
	if n > QueryIconMaxLength {
		return QueryIconMaxLength
	}
	return n / 10 * 10
}

// 强制取词类型 choices. The menu route is the one EasyDict prefers: apps that
// ignore a synthesized cmd+C still route their own Copy command.
const (
	ForceCopyMenu     = "menu"
	ForceCopyShortcut = "shortcut"
)

var ForceCopyKinds = []string{ForceCopyMenu, ForceCopyShortcut}

// ClampForceCopyKind keeps a hand-edited value on one of the two routes.
func ClampForceCopyKind(kind string) string {
	for _, k := range ForceCopyKinds {
		if kind == k {
			return kind
		}
	}
	return ForceCopyMenu
}

// DefaultHTTPPort is the port Easydict's local API listens on.
const DefaultHTTPPort = "8080"

// NormalizeHTTPPort turns a typed port into the string the server binds. Empty
// means the default; anything that is not a usable port is rejected rather than
// silently replaced, because a typo the user cannot see would look like the
// switch is broken.
func NormalizeHTTPPort(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return DefaultHTTPPort, nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("端口需要是 1–65535 之间的整数")
	}
	return strconv.Itoa(n), nil
}

// Slots and contexts name the shortcut/selection keys. They are the contract
// between the Go wiring, the settings UI and the persisted file.
const (
	SlotInput        = "input"
	SlotSelection    = "selection"
	SlotToggleSelect = "toggleSelect"
	SlotCapture      = "capture"
	SlotSilentOCR    = "silentOcr"
	SlotOCRShot      = "ocrShot"
	SlotOCRClipboard = "ocrClipboard"
	SlotOCRWindow    = "ocrWindow"
	SlotClipboard    = "clipboard"
	SlotReplace      = "replace"
	SlotPolish       = "polish"
	SlotODict        = "odict"
	SlotSettings     = "settings"
)

// App action names for the 快捷键 menu. They are the strings the menu sends to
// the focused window (gui.Act*), kept here as well so the persisted table can
// be validated without importing the GUI package.
const (
	AppActClearInput   = "clearInput"
	AppActClearAll     = "clearAll"
	AppActCopy         = "copy"
	AppActCopyFirst    = "copyFirstResult"
	AppActFocus        = "focus"
	AppActPlay         = "play"
	AppActRetry        = "retry"
	AppActToggle       = "toggle"
	AppActPin          = "pin"
	AppActHide         = "hide"
	AppActFontIncrease = "increaseFontSize"
	AppActFontDecrease = "decreaseFontSize"
	AppActGoogle       = "google"
	AppActEudic        = "eudic"
	AppActAppleDic     = "appleDic"
)

var appShortcutDefaults = map[string]string{
	AppActClearInput:   "cmd+k",
	AppActClearAll:     "cmd+shift+k",
	AppActCopy:         "cmd+shift+c",
	AppActCopyFirst:    "cmd+shift+j",
	AppActFocus:        "cmd+i",
	AppActPlay:         "cmd+s",
	AppActRetry:        "cmd+r",
	AppActToggle:       "cmd+t",
	AppActPin:          "cmd+p",
	AppActHide:         "cmd+y",
	AppActFontIncrease: "cmd+=",
	AppActFontDecrease: "cmd+-",
	AppActGoogle:       "cmd+return",
	AppActEudic:        "cmd+shift+return",
	AppActAppleDic:     "cmd+shift+d",
}

// DefaultAppShortcuts is a copy of EasyDict's MainMenuShortcutCommand table, so
// a caller cannot change the package's fallbacks by editing what it gets.
func DefaultAppShortcuts() map[string]string {
	out := make(map[string]string, len(appShortcutDefaults))
	for k, v := range appShortcutDefaults {
		out[k] = v
	}
	return out
}

// KnownAppAction reports whether action is one the 快捷键 menu can carry.
func KnownAppAction(action string) bool {
	_, ok := appShortcutDefaults[action]
	return ok
}

// WithAppShortcutDefaults fills actions a caller did not mention, so a page
// written before an action existed cannot unbind it by omission. An action
// stored as "" is a deliberate unbinding and stays empty.
func WithAppShortcutDefaults(got map[string]string) map[string]string {
	return mergeMap(got, appShortcutDefaults)
}

// 语种识别 modes. "none" is the system recogniser alone; the other two name the
// provider kind whose detection refines it, so the value doubles as the lookup.
const (
	DetectOptimizeNone   = "none"
	DetectOptimizeGoogle = "google"
	DetectOptimizeBaidu  = "baidu"
)

const (
	ContextMain        = "main"
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
	{SlotToggleSelect, "开关划词翻译"},
	{SlotCapture, "截图翻译"},
	{SlotSilentOCR, "静默截图 OCR"},
	{SlotOCRShot, "截图 OCR"},
	{SlotOCRClipboard, "剪贴板 OCR"},
	{SlotOCRWindow, "显示 OCR 窗口"},
	{SlotClipboard, "剪贴板翻译"},
	{SlotReplace, "翻译并替换"},
	{SlotPolish, "润色并替换"},
	{SlotODict, "鼠标取词"},
	{SlotSettings, "打开设置"},
}

// ContextLabels drives the per-window service selection table.
var ContextLabels = []Label{
	{ContextMain, "主窗口"},
	{ContextScreenshot, "截图 OCR"},
	{ContextClipboard, "剪贴板"},
	{ContextSilentOCR, "静默截图 OCR"},
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

// BlocksApp reports whether 禁止名单 excludes the frontmost app from 自动划词.
// An empty bundle id means the system could not tell, so nothing is blocked:
// silence there would look like the watcher had stopped.
func (c Config) BlocksApp(bundle string) bool {
	if bundle == "" {
		return false
	}
	for _, app := range c.Blocklist {
		if app.BundleID == bundle {
			return true
		}
	}
	return false
}

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
	case ContextReplace, ContextTranslation:
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
