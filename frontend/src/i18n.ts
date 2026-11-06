// UI language. Every literal in this app is written in Chinese, so the
// dictionary is keyed by the Chinese text itself: a phrase with no English
// entry still renders as Chinese instead of leaving a blank button.
export type Vars = Record<string, string | number>

const STORE_KEY = 'transdict.uiLang'

function stored(): string {
  try {
    return localStorage.getItem(STORE_KEY) === 'en' ? 'en' : 'zh-CN'
  } catch {
    return 'zh-CN'
  }
}

let locale = stored()
// The stylesheet keys a few width trade-offs on the language (English labels
// are longer), so the document has to say which one it is.
if (typeof document !== 'undefined') document.documentElement.lang = locale

const en: Record<string, string> = {
  '识别中…': 'Recognizing…',
  '无法读取截图，请在系统设置中授予屏幕录制权限后重试。':
    'Cannot read the screenshot. Grant Screen Recording permission in System Settings and try again.',
  '拖拽框选文字区域': 'Drag to select a region',
  '翻译': 'Translate',
  '取消': 'Cancel',
  '复制': 'Copy',
  '朗读': 'Speak',
  '详情': 'Details',
  '等待划词…': 'Waiting for a selection…',
  '已清理 {n} 条': 'Cleared {n} entries',
  '没有可导出的记录': 'Nothing to export',
  '已复制 {n} 字，可粘贴到笔记': 'Copied {n} characters, ready to paste',
  '已导出到 {path}': 'Exported to {path}',
  '已取消导出': 'Export cancelled',
  '历史': 'History',
  '{n} 条': '{n} entries',
  '搜索原文或译文': 'Search source or translation',
  '只看收藏': 'Favorites only',
  '按服务筛选': 'Filter by service',
  '全部服务': 'All services',
  '导出格式': 'Export format',
  '导出文件': 'Export to a file',
  '复制到剪贴板': 'Copy to clipboard',
  '清理范围': 'Clear range',
  '全部（保留收藏）': 'All (keep favorites)',
  '一天前': 'Older than 1 day',
  '一周前': 'Older than 1 week',
  '一月前': 'Older than 1 month',
  '清理': 'Clear',
  '清理「{scope}」的记录？此操作不可撤销。': 'Clear records from “{scope}”? This cannot be undone.',
  '确认清理？': 'Clear?',
  '确认删除？': 'Delete?',
  '确认导入？': 'Import?',
  '确认恢复？': 'Reset?',
  '还没有记录': 'No entries yet',
  '没有匹配的记录': 'No matching entries',
  '取消收藏': 'Remove from favorites',
  '收藏': 'Favorite',
  '删除': 'Delete',
  '润色': 'Polish',
  '在不改变原意的前提下润色下面的文字，使其更地道、通顺：':
    'Polish the text below so it reads naturally, without changing the meaning:',
  '总结': 'Summarize',
  '用三句以内的中文总结下面的文字：': 'Summarize the text below in no more than three sentences:',
  '更正式': 'More formal',
  '把下面的文字改写得更正式、书面化：': 'Rewrite the text below in a more formal, written register:',
  '更口语': 'More casual',
  '把下面的文字改写得更口语、自然：': 'Rewrite the text below in a more casual, natural tone:',
  '纠错': 'Fix errors',
  '纠正下面文字中的拼写、语法与标点错误，只输出修改后的文字：':
    'Correct the spelling, grammar and punctuation in the text below, and output only the corrected text:',
  '扩写': 'Expand',
  '在保持原意的前提下扩写下面的文字：': 'Expand the text below without changing its meaning:',
  '请输入文字': 'Enter some text',
  '查词典中…': 'Looking up…',
  '查询中…': 'Querying…',
  '输入': 'Input',
  '{n} 字': '{n} characters',
  '输入或粘贴要翻译的文字，⌘ + Return 查询':
    'Type or paste the text to translate. ⌘ + Return queries.',
  '输入或粘贴要翻译的文字，Ctrl + Enter 查询':
    'Type or paste the text to translate. Ctrl + Enter queries.',
  '输入或粘贴要翻译的文字，⌘/Ctrl + 回车 查询':
    'Type or paste the text to translate. ⌘/Ctrl + Enter queries.',
  '源语言': 'Source',
  '自动检测': 'Detect',
  '目标语言': 'Target',
  '仅查词典': 'Dictionary only',
  '查询': 'Query',
  'AI 指令': 'AI prompts',
  '需要启用一个 AI 翻译服务': 'Enable an AI translation service first',
  '剪贴板': 'Clipboard',
  '按下迷你窗快捷键查询剪贴板': 'Press the mini-window shortcut to query the clipboard',
  '结果': 'Result',
  '历史记录': 'History',
  '设置': 'Settings',
  '关闭': 'Close',
  '仅词典': 'Dictionary only',
  '清空输入': 'Clear input',
  '只查词典，不调用翻译服务': 'Look up dictionaries only, without translation services',
  '截图翻译': 'Capture',
  '输入翻译': 'Type',
  '划词翻译': 'Selection',
  '截图取字并复制': 'OCR to clipboard',
  '迷你窗看剪贴板': 'Clipboard in mini window',
  '翻译剪贴板': 'Translate clipboard',
  '翻译并替换选区': 'Translate and replace',
  '鼠标取词': 'Lookup at pointer',
  '耗时 {n} ms': '{n} ms',
  '就绪': 'Ready',
  '译为 {lang}': 'Translate to {lang}',
  '收起': 'Collapse',
  '最小化': 'Minimize',
  '最大化': 'Maximize',
  '折叠': 'Fold',
  '展开': 'Unfold',
  '翻译中…': 'Translating…',
  '置顶': 'Keep on top',
  '取消置顶': 'Stop keeping on top',
  '会话 {id} · {mode}': 'Session {id} · {mode}',
  '未查询': 'Not queried',
  '{ok}/{all} 译文': '{ok}/{all} translated',
  '{n} 识别': '{n} OCR',
  '{n} 词典': '{n} dictionary',
  '已启用 {n} 个翻译服务': '{n} translation services enabled',
  '聚焦输入': 'focus the input',
  '交换语言': 'Swap languages',
  '正在准备截图…': 'Preparing the screenshot…',
  '拖拽框选文字区域，松开即翻译': 'Drag over the text; releasing starts the translation',
  '没有查到词条': 'No entry found',
  '词典未收录：{names}': 'No dictionary entry: {names}',
  '框选屏幕上的任何文字': 'Select any text on screen',
  '识别、翻译并查词典，结果留在这一屏': 'Recognized, translated and looked up, all in this window',
  '随时截屏翻译': 'Capture and translate at any time',
  '立即截图': 'Capture now',
  '输入文字': 'Type text',
  '服务设置': 'Services',
  '{key} 打开输入 · Esc 收起窗口 · 识别有误时可在结果里编辑原文重查':
    '{key} opens the input · Esc hides the window · edit the source text in a result to re-query',
  '选区截图': 'Capture',
  '原文': 'Source',
  '编辑': 'Edit',
  '未识别到文字': 'No text recognized',
  '用这段文字重新查询': 'Query with this text',
  '识别服务': 'OCR services',
  '重试': 'Retry',
  '网义': 'Web sense',
  '全部重新查询': 'Query all again',
  '服务': 'Services',
  '每个分类可启用多个服务，结果并列显示':
    'Enable several services per category; results are shown side by side',
  '窗口': 'Windows',
  '勾选每个窗口实际调用哪些服务': 'Choose which services each window actually calls',
  '快捷键': 'Shortcuts',
  '全局生效，无需切换回应用': 'Work globally, no need to focus the app',
  '通用': 'General',
  '语言、外观与行为': 'Language, appearance and behavior',
  '读取配置…': 'Loading settings…',
  '排序…': 'Reordering…',
  '此分类还没有服务，从下方添加。': 'This category has no services yet. Add one below.',
  '启用 {name}': 'Enable {name}',
  '保存…': 'Saving…',
  '免费': 'Free',
  '缺配置': 'Incomplete',
  '配置': 'Configure',
  '删除…': 'Deleting…',
  '删除「{name}」及其配置？': 'Delete “{name}” and its settings?',
  '显示名称': 'Display name',
  '该类型未在本平台注册，保留配置但不会被调用。':
    'This type is not registered on this platform. The settings are kept but it will not be called.',
  '选择服务类型': 'Choose a service type',
  '添加服务…': 'Add service…',
  '（免费）': ' (free)',
  '添加…': 'Adding…',
  '添加': 'Add',
  '必填': 'required',
  '隐藏': 'Hide',
  '显示': 'Show',
  '本窗口运行 {n} 个服务，按勾选顺序显示。': 'This window runs {n} services, in the order shown.',
  '本窗口运行全部 {n} 个已启用服务；点掉一项即可排除它。':
    'This window runs all {n} enabled services; click one off to leave it out.',
  '全部': 'All',
  '每个阶段至少保留一个服务；要完全停用请到「服务」页。':
    'Keep at least one service per stage; turn it off on the Services page instead.',
  '{name} 已停用，请先在「服务」页启用。': '{name} is disabled; enable it on the Services page first.',
  ' 静音 OCR 只复制文字，不显示结果。': ' Silent OCR only copies the text and shows no result.',
  '载入该窗口的服务清单…': 'Loading this window’s services…',
  '{name}（已停用）': '{name} (disabled)',
  '还没有 {stage} 服务': 'No {stage} services yet',
  // Result cards: what one answer can be asked to do.
  '慢速': 'Slow',
  '放慢速度再读一遍': 'Read it again, slower',
  '网页版': 'On the web',
  '复制全文': 'Copy all',
  '已复制': 'Copied',
  '已复制全部内容': 'Copied everything',
  '复制失败，请手动选择文字': 'The clipboard refused the write; select the text manually',
  '已收藏': 'Favourited',
  '已加入收藏': 'Added to favourites',
  '已取消收藏': 'Removed from favourites',
  '已译为{lang}': 'Translated into {lang}',
  '收藏这次查询，历史里可以只看收藏': 'Save this query; the history can show favourites only',
  // Screenshot editing: the selection survives the drag so it can be nudged, and
  // the crop can be pixelated before it goes back through 识别.
  '拖动边角微调，或按住框内移动': 'Drag a corner to fine-tune, or hold inside the box to move it',
  '遮挡': 'Redact',
  '拖出方块遮挡要隐去的内容': 'Drag squares over what must not be recognized',
  '清除遮挡': 'Clear redactions',
  '重新识别': 'Recognize again',
  // Moving preferences between machines, and the folder picker a dictionary
  // service asks for. The export carries service keys, so it says so.
  '浏览…': 'Browse…',
  '选择文件夹…': 'Choosing a folder…',
  '导出设置': 'Export settings',
  '导入设置': 'Import settings',
  '导出设置…': 'Exporting settings…',
  '导入设置…': 'Importing settings…',
  '已导出设置到 {path}': 'Settings exported to {path}',
  '已导入 {path}': 'Settings imported from {path}',
  '已取消': 'Cancelled',
  '导入会覆盖当前全部设置，继续吗？':
    'Importing replaces every setting you have now. Continue?',
  '导出的文件包含各服务的密钥，请自行保管。':
    'The exported file contains every service key — keep it somewhere you trust.',
  '这个文件不像 Transdict 的配置': 'That file does not look like a Transdict config',
  // Checking for a newer build. The answer comes from a server the user named,
  // so both the settings page and the menu bar can only ever report what it said.
  '更新': 'Updates',
  '更新源地址': 'Update source',
  '一个 JSON 文档，写明最新版本号与下载地址；GitHub 的 Releases API 直接可用。':
    'A JSON document naming the newest version and where to get it; a GitHub Releases API URL works as it is.',
  '启动时自动检查': 'Check on launch',
  '只在发现新版本时提示；地址留空即完全不联网。':
    'Only a newer release produces a notice; with no address nothing is contacted.',
  '检查更新': 'Check for updates',
  '当前版本 {v}': 'Version {v}',
  '检查更新…': 'Checking for updates…',
  '发现新版本 {v}（当前 {c}）': 'Version {v} is available (you have {c})',
  '已是最新版本 {v}': 'This build is up to date ({v})',
  '未设置更新源': 'No update source is set',
  '更新源需要是一个 http(s) 地址': 'The update source must be an http(s) address',
  '自动复制译文': 'Copy the translation automatically',
  '结果一出现就把第一条译文写入剪贴板；静音 OCR 与翻译替换按各自的方式处理。':
    'The first translation goes to the clipboard as soon as a result lands; ' +
    'silent OCR and replace-in-place keep handling the pasteboard their own way.',
  '恢复…': 'Restoring…',
  '改为全部启用': 'Use all enabled',
  '至少需要一个修饰键（⌘ ⌥ ⇧ Ctrl）': 'At least one modifier is required (⌘ ⌥ ⇧ Ctrl)',
  '绑定…': 'Binding…',
  '载入快捷键…': 'Loading shortcuts…',
  '点击「录制」后按下组合键，Esc 取消。快捷键在应用未聚焦时同样有效；若与其他程序冲突，会在保存后提示。':
    'Click Record, then press the combination. Esc cancels. Shortcuts work while the app is unfocused; ' +
    'a conflict with another program is reported after saving.',
  '未设置': 'Not set',
  '按组合键…': 'Press keys…',
  '录制': 'Record',
  '解绑…': 'Unbinding…',
  '{key} 注册失败，可能已被其他程序占用：{err}':
    '{key} could not be registered — another program may already own it: {err}',
  '清除旧快捷键失败：{err}': 'Could not release the previous shortcuts: {err}',
  '清除': 'Clear',
  '跟随系统': 'Follow system',
  '浅色': 'Light',
  '深色': 'Dark',
  '不自动朗读': 'Never read aloud',
  '自动朗读原文': 'Read the source aloud',
  '自动朗读译文': 'Read the translation aloud',
  '简体中文': '简体中文',
  '语言': 'Language',
  '原文语言': 'Source language',
  '译文语言': 'Target language',
  '界面语言': 'Interface language',
  '外观': 'Appearance',
  '主题': 'Theme',
  '迷你窗字号': 'Mini window size',
  '行为': 'Behavior',
  '开机自动启动': 'Launch at login',
  '需要 macOS 13 以上，且应用已签名安装到 /Applications':
    'Needs macOS 13 or later, with a signed build installed in /Applications',
  '写入当前用户的启动项': 'Writes a startup entry for the current user',
  '划词显示悬浮窗': 'Show a floating window on selection',
  '关闭后划词与取词的结果进入主窗口': 'When off, selection and lookup results go to the main window',
  '悬浮窗保持打开': 'Keep the floating window open',
  '关闭后悬浮窗会在几秒后自动收起': 'When off, the floating window closes itself after a few seconds',
  '悬浮窗自动收起': 'Auto-close floating window after',
  '自动监听鼠标划词': 'Watch selections automatically',
  '选中文字即翻译，无需按快捷键（仅 macOS）':
    'Translate on select, no shortcut needed (macOS only)',
  '隐藏与原文相同的译文': 'Hide translations equal to the source',
  '源语言与目标语言相同时不再重复显示': 'Nothing is shown twice when source and target match',
  '在程序坞中显示图标': 'Show a Dock tile',
  '在任务栏中显示图标': 'Show a taskbar button',
  '关闭后只保留菜单栏入口，主窗口仍由快捷键唤起':
    'When off, only the menu bar entry remains and the shortcut still summons the window',
  '任务栏按钮属于窗口，改动在下次打开主窗口时生效':
    'The taskbar button belongs to the window, so this applies the next time it opens',
  '自动朗读': 'Read aloud',
  '权限': 'Permissions',
  '屏幕录制': 'Screen Recording',
  '辅助功能': 'Accessibility',
  '已授权': 'Granted',
  '未授权': 'Not granted',
  '打开系统设置': 'Open System Settings',
  '划词、取词与截图依赖这两项授权；在系统设置里改完，需要重启 Transdict 才生效。':
    'Selection lookup, pointer lookup and screenshot rely on these two grants; restart Transdict after changing them in System Settings.',
  '重启 Transdict': 'Restart Transdict',
  '网络': 'Network',
  '网络代理': 'Proxy',
  '留空即跟随系统代理；只影响在线服务，苹果 OCR 与苹果词典不走这条链路。':
    'Leave empty to follow the system proxy. Only online services use it — Apple OCR and Apple Dictionary do not.',
  '代理地址无法解析': 'The proxy address cannot be parsed',
  '代理只支持 http、https 与 socks5': 'The proxy must be http, https or socks5',
  '代理缺少地址': 'The proxy has no address',
  '服务响应超时': 'The service timed out',
  '服务超时': 'Service timeout',
  '内置默认': 'Built-in default',
  '{n} 秒': '{n} s',
  '一个服务等待结果的最长时间。默认识别两分钟、文本三分钟；接入 AI 服务时可以调高。':
    'How long one service may take before it is given up on. Defaults to two minutes for recognition and three for text; raise it when an AI provider is slow.',
  '数据': 'Data',
  '配置保存于用户目录，删除后不可恢复。': 'Settings live in your user folder and cannot be recovered once deleted.',
  '记录查询历史': 'Save query history',
  '关闭后新的查询不再写入历史；已有记录保留，可随时清理。':
    'When off, new queries are not written to history; existing entries stay and can be cleared at any time.',
  '恢复默认设置会清空所有服务与快捷键，继续吗？':
    'Restoring defaults clears all services and shortcuts. Continue?',
  '恢复默认…': 'Restoring defaults…',
  '恢复默认设置': 'Restore defaults',
  '刚刚': 'just now',
  '{n} 分钟前': '{n} min ago',
  '{n} 小时前': '{n} h ago',
  '{n} 天前': '{n} d ago',
  'AI 处理中…': "AI is working…",
  'Apple Vision（离线）': "Apple Vision (offline)",
  'OpenAI 兼容接口，Grok 系列': "OpenAI-compatible endpoint, Grok models",
  'OpenAI 兼容接口，Llama 系列速度快': "OpenAI-compatible endpoint, fast Llama models",
  'OpenAI 兼容接口，Mistral 系列': "OpenAI-compatible endpoint, Mistral models",
  'OpenAI 兼容接口，deepseek-chat / deepseek-reasoner': "OpenAI-compatible endpoint, deepseek-chat / deepseek-reasoner",
  'OpenAI 兼容接口，国内可达': "OpenAI-compatible endpoint, reachable from China",
  'OpenAI 兼容接口，文本模型 MiniMax-Text': "OpenAI-compatible endpoint, MiniMax-Text",
  'OpenAI 兼容接口，聚合开源模型': "OpenAI-compatible endpoint, aggregated open models",
  'OpenAI 兼容端点（paas/v4），非官方 Python SDK 协议': "OpenAI-compatible endpoint (paas/v4), not the official Python SDK protocol",
  'Transdict 取词': "Transdict lookup",
  '任何实现 chat/completions 的服务：vLLM、Ollama /v1、One-API、企业网关': "Anything that implements chat/completions: vLLM, Ollama /v1, One-API, gateways",
  '写入剪贴板失败，无法替换': "Cannot write the clipboard, so nothing was replaced",
  '剪贴板为空': "The clipboard is empty",
  '历史不可用': "History is unavailable",
  '原文已是目标语言': "The text is already in the target language",
  '已有另一个 Transdict 在运行': "Another Transdict is already running",
  '快捷键名称不能为空': "The shortcut name cannot be empty",
  '截图已过期，请重新截取': "The capture expired; take it again",
  '智谱 GLM': "Zhipu GLM",
  '有道 OCR': "Youdao OCR",
  '服务不存在': "The service does not exist",
  '未检测到显示器': "No display detected",
  '未读取到选区，请在目标应用中重新选择文字': "No selection found; select the text again in that app",
  '格式应为 ctrl+alt+a 这类组合键': "Use a combination such as ctrl+alt+a",
  '没有内容': "Nothing selected",
  '没有内容可查询': "Nothing to query",
  '没有可用的服务': "No service is available",
  '没有可用的文字识别服务': "No text recognition service is available",
  '尚未配置：{names}': 'Needs setup: {names}',
  '没有可用的朗读服务': "No read-aloud service is available",
  '没有可用的翻译服务，请在设置中勾选': "No translation service is enabled; pick one in settings",
  '没有可用译文，无法替换': "There is no translation to replace with",
  '没有可翻译的文本': "There is nothing to translate",
  '没有进行中的截图': "There is no capture in progress",
  '没有选中内容': "Nothing is selected",
  '火山 veOCR': "Volc veOCR",
  '百度': "Baidu",
  '百度 OCR': "Baidu OCR",
  '目标语言不能为空': "The target language cannot be empty",
  '硅基流动': "SiliconFlow",
  '自定义 OpenAI 兼容服务': "Custom OpenAI-compatible service",
  '识别文字中…': "Recognizing text…",
  '该服务已存在，请直接修改': "This service already exists; edit it instead",
  '语言代码不能为空': "The language code cannot be empty",
  '返回空译文': "Returned an empty translation",
  '重新识别…': "Recognizing again…",
  '需要「屏幕录制」权限：请在 系统设置 → 隐私与安全性 → 录屏与系统录音 中打开 Transdict，然后重启应用': "Screen Recording permission is required: switch Transdict on in System Settings → Privacy & Security → Screen Recording, then restart the app",
  '需要「辅助功能」权限：请在 系统设置 → 隐私与安全性 → 辅助功能 中勾选 Transdict，然后重启应用': "Accessibility permission is required: tick Transdict in System Settings → Privacy & Security → Accessibility, then restart the app",
  '鼠标不在任何显示器上': "The pointer is not on any display",
  '鼠标取词需要「屏幕录制」权限': "Looking up at the pointer needs Screen Recording permission",
  '鼠标处没有识别到单词': "No word recognized under the pointer",
  // Go's own wording, kept here as well as in internal/l10n/en.go. A phrase
  // inside a result payload is translated by Go before it crosses the bridge; one
  // that arrives as a rejected binding call reaches this bundle as Chinese text,
  // so the sentences a bound method can return need both sides.
  '记录不存在': "No such history entry",
  '无法定位鼠标': "Could not locate the pointer",
  '缺少该显示器的截图': "That display has no capture",
  '开机自启设置失败': "Could not set launch at login",
  '开机自启：当前系统不支持': "Launch at login: not supported on this system",
  '取词：当前系统不支持读取其他应用的选区': "Word capture: this system cannot read another app's selection",
  '取词：模拟 Ctrl+C 失败': "Word capture: could not send Ctrl+C",
}

function substitute(text: string, vars?: Vars): string {
  if (!vars) return text
  return text.replace(/\{(\w+)\}/g, (all, key: string) =>
    Object.prototype.hasOwnProperty.call(vars, key) ? String(vars[key]) : all,
  )
}

export function t(zh: string, vars?: Vars): string {
  return substitute(locale === 'en' ? en[zh] ?? zh : zh, vars)
}

/** applyLocale records the interface language. Nothing in this bundle can be
 * re-translated in place — the string tables are built when the module loads — so
 * a value that differs from the one already drawn means a reload. That is how a
 * language switch reaches every window, including the one that did not ask.
 *
 * The value is kept in localStorage because the capture overlay window never
 * reads config: it must still show the right language the first time it is drawn. */
export function applyLocale(uiLang: string) {
  const next = uiLang === 'en' ? 'en' : 'zh-CN'
  if (next === locale) return
  locale = next
  document.documentElement.lang = next
  try {
    localStorage.setItem(STORE_KEY, next)
  } catch {
    // Private-mode storage only costs the next cold start its remembered hint.
  }
  location.reload()
}
