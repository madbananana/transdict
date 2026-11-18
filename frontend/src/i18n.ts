// UI language. Every literal in this app is written in Chinese, so the
// dictionary is keyed by the Chinese text itself: a phrase with no English
// entry still renders as Chinese instead of leaving a blank button.
export type Vars = Record<string, string | number>

const STORE_KEY = 'transdict.uiLang'

function stored(): 'en' | 'zh-CN' {
  try {
    return localStorage.getItem(STORE_KEY) === 'en' ? 'en' : 'zh-CN'
  } catch {
    return 'zh-CN'
  }
}

let locale: 'en' | 'zh-CN' = stored()
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
  '确定': 'OK',
  '如需恢复，请在查词窗口使用快捷键 ⌘, 打开设置页，然后取消【隐藏菜单栏图标】选项。':
    'To restore it, press ⌘, in a translation window to open Settings, then turn off Hide menu bar icon.',
  '或右键取消': 'or right-click to cancel',
  '复制': 'Copy',
  '拷贝文本': 'Copy text',
  '朗读': 'Speak',
  '停止': 'Stop',
  '停止生成': 'Stop generating',
  '替换': 'Replace',
  '使用译文替换': 'Replace with translation',
  '在网页中打开': 'Open on the web',
  '词源：': 'Etymology:',
  '没有可替换的译文': 'There is no translation to insert',
  '请先回到要替换的应用': 'Switch back to the app whose text is being replaced first',
  '详情': 'Details',
  '打开主窗口': 'Open main window',
  '快捷动作': 'Quick actions',
  '添加到收藏': 'Add to favorites',
  '将换行符替换为空格': 'Replace newlines with spaces',
  '去除代码注释符号': 'Remove code comment symbols',
  '单词分词': 'Split words',
  '打开应用设置': 'Open app settings',
  '钉住窗口': 'Pin window',
  '取消钉住': 'Unpin window',
  '已清理 {n} 条': 'Cleared {n} entries',
  '没有可导出的记录': 'Nothing to export',
  '已导出到 {path}': 'Exported to {path}',
  '已取消导出': 'Export cancelled',
  '历史': 'History',
  '查询历史': 'History',
  '导出': 'Export',
  '已删除': 'Deleted',
  '清空所有历史？收藏会保留。': 'Clear all history? Favorites are kept.',
  '{n} 条': '{n} entries',
  '全部服务': 'All services',
  '导出历史': 'Export history',
  '导出收藏': 'Export favorites',
  '清理': 'Clear',
  '已清空 {n} 条收藏': 'Cleared {n} favorites',
  '清空所有收藏？此操作不可撤销。': 'Clear all favorites? This cannot be undone.',
  '确认清空？': 'Clear?',
  '确认导入？': 'Import?',
  '确认恢复？': 'Reset?',
  '暂无查询记录': 'No queries yet',
  '暂无收藏': 'No favorites yet',
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
  '输入或粘贴要翻译的文字，⌘/Ctrl + 回车 查询':
    'Type or paste the text to translate. ⌘/Ctrl + Enter queries.',
  '输入内容，Enter 查询，Shift + Enter 换行':
    'Type text; Enter to query, Shift + Enter for a new line',
  '识别为': 'Detected',
  '识别为 {lang}': 'Detected {lang}',
  '源语言': 'Source',
  '自动检测': 'Detect',
  '自动选择': 'Auto',
  '字号': 'Text size',
  '目标语言': 'Target',
  '仅查词典': 'Dictionary only',
  '查询': 'Query',
  '应用内查询': 'Look Up in App',
  '剪切': 'Cut',
  '粘贴': 'Paste',
  '全选': 'Select All',
  'AI 指令': 'AI prompts',
  '需要启用一个 AI 翻译服务': 'Enable an AI translation service first',
  '剪贴板': 'Clipboard',
  '结果': 'Result',
  '历史记录': 'History',
  '设置': 'Settings',
  '关闭': 'Close',
  '仅词典': 'Dictionary only',
  '清空输入': 'Clear input',
  '清空所有': 'Clear all',
  '清空': 'Clear',
  '只查词典，不调用翻译服务': 'Look up dictionaries only, without translation services',
  '截图翻译': 'Capture',
  '输入翻译': 'Type',
  '划词翻译': 'Selection',
  '鼠标取词': 'Lookup at pointer',
  '收起': 'Collapse',
  '最小化': 'Minimize',
  '最大化': 'Maximize',
  '展开': 'Unfold',
  '翻译中…': 'Translating…',
  '置顶': 'Keep on top',
  '{ok}/{all} 译文': '{ok}/{all} translated',
  '{n} 识别': '{n} OCR',
  '{n} 词典': '{n} dictionary',
  '已启用 {n} 个翻译服务': '{n} translation services enabled',
  '聚焦输入': 'focus the input',
  '交换语言': 'Swap languages',
  '正在准备截图…': 'Preparing the screenshot…',
  '拖拽框选文字区域，松开即翻译': 'Drag over the text; releasing starts the translation',
  '单击 D 键选择上一次截图区域': 'Press D to reuse the last capture area',
  '输入文字': 'Type text',
  '服务设置': 'Services',
  '选区截图': 'Capture',
  '原文': 'Source',
  '编辑': 'Edit',
  '未识别到文字': 'No text recognized',
  '用这段文字重新查询': 'Query with this text',
  '识别服务': 'OCR services',
  '重试': 'Retry',
  '失败': 'Failed',
  '参见 👉': 'Please see 👉',
  '无结果': 'No result',
  '未查询到结果': 'No results found',
  '错误': 'Error',
  '全部重新查询': 'Query all again',
  '服务': 'Services',
  '窗口': 'Windows',
  '查询语言': 'Query languages',
  '输入框': 'Input field',
  '自动查询': 'Auto query',
  '自动复制': 'Auto copy',
  '快捷功能': 'Quick actions',
  '应用设置': 'App settings',
  '字体': 'Font',
  '常规设置': 'General settings',
  '鼠标划词查询': 'Mouse selection query',
  'OCR 设置': 'OCR settings',
  '取词与文本替换': 'Text selection and replacement',
  '窗口管理': 'Window management',
  '查询文本处理': 'Query text processing',
  '注意，这不会修改原文，仅对查询内容进行处理':
    'This leaves the source text untouched; only the query is processed',
  '支持在翻译窗口通过快捷键 ⌘ +/- 修改字体大小':
    'Change the font size in a translation window with ⌘ +/-',
  '快捷键': 'Shortcuts',
  '通用': 'General',
  '读取配置…': 'Loading settings…',
  '排序…': 'Reordering…',
  '此分类还没有服务，从下方添加。': 'This category has no services yet. Add one below.',
  '启用 {name}': 'Enable {name}',
  '保存…': 'Saving…',
  '从云端获取模型': 'Fetch models from the endpoint',
  '获取本地模型': 'Fetch local models',
  '搜索模型': 'Search models',
  '没有匹配的模型': 'No matching models',
  '获取模型…': 'Fetching models…',
  '没有获取到模型': 'No models returned',
  '支持的模型': 'Supported models',
  '取消全选': 'Deselect All',
  '免费': 'Free',
  '缺配置': 'Incomplete',
  '显示名称': 'Display name',
  '该类型未在本平台注册，保留配置但不会被调用。':
    'This type is not registered on this platform. The settings are kept but it will not be called.',
  '添加服务': 'Add Service',
  '移除服务': 'Remove Service',
  '已添加全部服务': 'All services have been added',
  '服务列表': 'Service list',
  '从左侧选择一个服务查看配置。': 'Select a service on the left to see its settings.',
  '添加…': 'Adding…',
  '添加': 'Add',
  '移除': 'Remove',
  // 导入词典: MDict runs on a list of imported .mdx files, the way EasyDict does.
  '导入词典': 'Import dictionary',
  '导入词典…': 'Importing dictionaries…',
  '移除…': 'Removing…',
  '尚未导入任何词典，点击「导入词典」选择 .mdx 文件。':
    'No dictionaries imported yet — choose .mdx files with Import dictionary.',
  '未单独导入词典，使用文件夹中的全部 .mdx。':
    'No individual dictionaries imported; every .mdx in the folder is used.',
  '禁止名单': 'Blocklist',
  '名单为空：所有应用都会触发自动划词。': 'The list is empty: auto selection runs in every app.',
  '必填': 'required',
  '隐藏': 'Hide',
  '显示': 'Display',
  '本窗口运行 {n} 个服务，按勾选顺序显示。': 'This window runs {n} services, in the order shown.',
  '本窗口运行全部 {n} 个已启用服务；点掉一项即可排除它。':
    'This window runs all {n} enabled services; click one off to leave it out.',
  '全部': 'All',
  '每个阶段至少保留一个服务；要完全停用请到「服务」页。':
    'Keep at least one service per stage; turn it off on the Services page instead.',
  '{name} 已停用，请先在「服务」页启用。': '{name} is disabled; enable it on the Services page first.',
  ' 静默截图 OCR 只复制文字，不显示结果。': ' Silent Screenshot OCR only copies the text and shows no result.',
  '载入该窗口的服务清单…': 'Loading this window’s services…',
  '{name}（已停用）': '{name} (disabled)',
  '还没有 {stage} 服务': 'No {stage} services yet',
  // Result cards: what one answer can be asked to do.
  '慢速': 'Slow',
  '放慢速度再读一遍': 'Read it again, slower',
  '复制全文': 'Copy all',
  '已复制': 'Copied',
  '复制失败，请手动选择文字': 'The clipboard refused the write; select the text manually',
  '已收藏': 'Favorited',
  '已加入收藏': 'Added to favorites',
  '已取消收藏': 'Removed from favorites',
  // OCR 窗口: the picture, the lines found in it and the merged text.
  'OCR 预览': 'OCR Preview',
  '识别图片': 'Image',
  '识别结果': 'Lines',
  '合并文本': 'Merged text',
  '{n} 行': '{n} lines',
  '还没有识别结果': 'Nothing recognized yet',
  '用「截图 OCR」或「剪贴板 OCR」识别一张图片。': 'Use 截图 OCR or 剪贴板 OCR on a picture first.',
  '已译为{lang}': 'Translated into {lang}',
  '收藏这次查询，历史里可以只看收藏': 'Save this query; the history can show favorites only',
  // Screenshot editing: the selection survives the drag so it can be nudged, and
  // the crop can be pixelated before it goes back through 识别.
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
  // 验证 in a service's detail pane: EasyDict probes one service on its own so a
  // wrong key is named before the next translation fails.
  '验证': 'Validate',
  '验证中…': 'Validating…',
  '验证成功 🎉 {text}': 'Validation succeeded 🎉 {text}',
  '验证失败 💔 {err}': 'Validation failed 💔 {err}',
  '找不到这个服务': 'No such service',
  '朗读服务没有返回音频': 'The speech service returned no audio',
  'OCR 服务需要截图才能验证': 'An OCR service can only be validated with a screenshot',
  '服务返回了空结果': 'The service returned an empty answer',
  '朗读服务可用': 'The speech service works',
  // Checking for a newer build. The answer comes from a server the user named,
  // so both the settings page and the menu bar can only ever report what it said.
  '更新': 'Updates',
  '更新源地址': 'Update source',
  '一个 JSON 文档，写明最新版本号与下载地址；GitHub 的 Releases API 直接可用。':
    'A JSON document naming the newest version and where to get it; a GitHub Releases API URL works as it is.',
  '自动检查应用更新': 'Automatically check for updates',
  '启动时和运行期间定时检查；发现新版本时询问是否安装，也可忽略该版本。':
    'Checks at launch and periodically while running; a newer release asks whether to install it, and can be skipped.',
  '包括 Beta 版本': 'Include beta versions',
  '抢先体验新功能，但可能不稳定':
    'Get new features early, though they may be unstable',
  '检查更新': 'Check for updates',
  '立即检查': 'Check now',
  '下载并安装': 'Download and install',
  '正在下载更新…': 'Downloading update…',
  '正在下载更新… {n}%': 'Downloading update… {n}%',
  '打开发布页': 'Open release page',
  '忽略此版本': 'Skip this version',
  '已忽略 {v} 的自动提示': 'Automatic notices for {v} are off',
  '已忽略 {v} 的自动更新提示': 'Automatic update notices for {v} are off',
  '取消忽略': 'Stop skipping',
  '自动更新会替换当前应用；未签名版本可能需要重新授予录屏与辅助功能权限。':
    'An update replaces the app in place; an unsigned build may need Screen Recording and Accessibility permission again.',
  '当前版本 {v}': 'Version {v}',
  '检查更新…': 'Checking for updates…',
  '发现新版本 {v}（当前 {c}）': 'Version {v} is available (you have {c})',
  '已是最新版本 {v}': 'This build is up to date ({v})',
  // 软件更新: the release notes and the download in one window.
  '软件更新': 'Software Update',
  '新版本 {v}': 'Version {v}',
  '更新内容': 'What’s New',
  '本次发布没有写明更新内容。': 'This release does not include any notes.',
  '查看更新': 'View Update',
  '立即更新': 'Update Now',
  '稍后': 'Later',
  '正在安装新版本…': 'Installing the new version…',
  '安装完成后会自动重新打开 Transdict。':
    'Transdict will reopen automatically when the installation finishes.',
  '取消下载': 'Cancel Download',
  '未设置更新源': 'No update source is set',
  '更新源需要是一个 http(s) 地址': 'The update source must be an http(s) address',
  '自动复制第一个翻译结果': 'Copy the first translation automatically',
  '结果一出现就把第一条译文写入剪贴板；静默截图 OCR 与翻译替换按各自的方式处理。':
    'The first translation goes to the clipboard as soon as a result lands; ' +
    'Silent Screenshot OCR and replace-in-place keep handling the pasteboard their own way.',
  '恢复…': 'Restoring…',
  '改为全部启用': 'Use all enabled',
  '至少需要一个修饰键（⌘ ⌥ ⇧ Ctrl）': 'At least one modifier is required (⌘ ⌥ ⇧ Ctrl)',
  '绑定…': 'Binding…',
  '载入快捷键…': 'Loading shortcuts…',
  '全局快捷键': 'Global Shortcuts',
  '应用内快捷键': 'In-app shortcuts',
  '清空查询内容': 'Clear query text',
  '复制文本': 'Copy query text',
  '复制第一个翻译结果': 'Copy first translation',
  '聚焦': 'Focus',
  '播放单词发音': 'Play pronunciation',
  '播放音频': 'Play audio',
  '停止播放音频': 'Stop audio',
  '交换翻译语言': 'Swap languages',
  '增大字体': 'Increase font size',
  '减小字体': 'Decrease font size',
  '通过 Google 搜索': 'Search with Google',
  '未设置': 'Not set',
  '按组合键…': 'Press keys…',
  '录制': 'Record',
  '解绑…': 'Unbinding…',
  '{key} 注册失败，可能已被其他程序占用：{err}':
    '{key} could not be registered — another program may already own it: {err}',
  '清除旧快捷键失败：{err}': 'Could not release the previous shortcuts: {err}',
  '快捷键"{key}"，无法使用': 'The shortcut "{key}" cannot be used',
  '当前快捷键无法使用，因为它已被 "{name}"占用':
    'That shortcut is already taken by "{name}"',
  '清除': 'Clear',
  '跟随系统': 'Follow system',
  '浅色模式': 'Light',
  '深色模式': 'Dark',
  '不自动朗读': 'Never read aloud',
  '自动朗读原文': 'Read the source aloud',
  '自动朗读译文': 'Read the translation aloud',
  '简体中文': '简体中文',
  '语言': 'Language',
  '原文语言': 'Source language',
  '译文语言': 'Target language',
  '显示语言': 'Display language',
  '主题': 'Theme',
  '字体大小': 'Font size',
  '语种识别': 'Language detection',
  '仅使用系统语种识别': 'System detection only',
  '使用 Google 语种识别进行优化': 'Optimize with Google detection',
  '使用百度语种识别进行优化': 'Optimize with Baidu detection',
  '选择 Google 或百度后，识别会先请该服务读一小段文字；服务不可用时仍回到系统识别。':
    'With Google or Baidu selected, a short sample is read by that service first; when it is unavailable the system detection still answers.',
  '第一语言': 'First language',
  '第二语言': 'Second language',
  '第一语言不能与第二语言相同（{lang}）。{field}已被替换为{kept}。':
    'The first and second languages cannot be the same ({lang}). {field} has been changed to {kept}.',
  '短语难以判断语种时，优先认作这两种语言；「自动选择」译文也在这两者之间换算。':
    'Short text is detected as one of these two languages, and Auto target picks the other side of the pair.',
  '小': 'Small',
  '较小': 'Smaller',
  '中': 'Medium',
  '较大': 'Larger',
  '大': 'Large',
  '开机自启动': 'Launch at login',
  '需要 macOS 13 以上，且应用已签名安装到 /Applications':
    'Needs macOS 13 or later, with a signed build installed in /Applications',
  '写入当前用户的启动项': 'Writes a startup entry for the current user',
  '输入翻译时，清空查询内容': 'Clear the query when opening input translate',
  '按快捷键打开输入窗口时，从空白输入框开始':
    'The input shortcut starts with an empty box',
  '打开窗口时自动选中查询文本': 'Select the query text when the window opens',
  '窗口打开时选中输入框里的文字，直接输入即可替换':
    'The box text comes back selected, so typing replaces it',
  '划词翻译未选中文本时，保留上次结果':
    'Keep the last result when a selection shortcut finds nothing',
  '没有选中文字时不显示错误，窗口保持上一次的结果':
    'No error line; the window keeps what it was showing',
  '粘贴后自动查询': 'Translate after paste',
  '粘贴完成后立即翻译，不必按 Enter': 'Run the pasted text without pressing Enter',
  '输入时自动查询': 'Translate while typing',
  '停止输入片刻后自动翻译': 'Query once you pause typing',
  '查询英语单词后自动播放发音': 'Pronounce English words automatically',
  '查询单个英语单词时自动朗读原文，不必点喇叭':
    'Read a single English word aloud as soon as its answer arrives',
  '显示 Google 快捷图标': 'Show Google quick link',
  '标题栏一键用 Google 搜索当前文本': 'Search the current text on Google from the titlebar',
  '显示苹果词典快捷图标': 'Show Apple Dictionary quick link',
  '标题栏一键在系统词典中查当前文本': 'Look the current text up in the system dictionary from the titlebar',
  '显示欧路词典快捷图标': 'Show Eudic quick link',
  '标题栏一键在欧路词典中查当前文本': 'Look the current text up in Eudic from the titlebar',
  '显示快捷设置图标': 'Show quick settings icon',
  '标题栏显示快捷动作按钮：添加到收藏、文本改写与打开设置': 'Show the quick-action button in the titlebar: add to favorites, rewrite the text, open settings',
  '在苹果词典中打开': 'Open in Apple Dictionary',
  '在欧路词典中打开': 'Open in Eudic',
  '启动后隐藏主窗口': 'Hide main window at startup',
  '英语发音': 'English pronunciation',
  '默认 TTS 服务': 'Default TTS service',
  '自动选中所有文本': 'Select all text automatically',
  '如果当前输入框中未选中文本，在翻译并替换操作时，允许自动全选文本内容':
    'When nothing is highlighted in the input field, selecting everything before a translate-and-replace',
  '兼容模式替换': 'Compatibility replace',
  '启用后，文本替换将使用兼容模式，在部分应用中更可靠。':
    'Replacement goes through a compatibility path, which works more reliably in some apps.',
  '启用强制取词': 'Force text capture',
  '注意，这可能会影响剪贴板内容':
    'Note that this may affect the contents of the clipboard',
  '强制取词类型': 'Force capture method',
  '优先菜单栏动作复制取词': 'Prefer the menu bar Copy action',
  '优先模拟快捷键复制取词': 'Prefer a simulated copy shortcut',
  '优先使用 AppleScript API': 'Prefer AppleScript API',
  '在浏览器中，优先使用 AppleScript API 进行取词和文本替换。开启该选项，可提升取词的准确性和成功率。':
    'In browsers, capture the selection and replace text through AppleScript first. This can improve accuracy and success rate.',
  '英语单词优先使用有道发音': 'Prefer the Youdao voice for English words',
  '关闭后，英语单词发音将使用默认 TTS 服务':
    'When off, English words use the default TTS service',
  '美式发音': 'American',
  '英式发音': 'British',
  '启用 Markdown 渲染': 'Enable Markdown rendering',
  'AI 翻译结果将以 Markdown 格式呈现标题、列表与强调。点击单个结果旁的图标可临时切换。':
    'AI answers render headings, lists and emphasis as Markdown. Click the icon beside a single result to toggle it temporarily.',
  '切换到纯文本': 'Switch to plain text',
  '切换到 Markdown 渲染': 'Switch to Markdown rendering',
  '只在菜单栏运行，用快捷键或菜单打开主窗口，下次启动生效':
    'Stay in the menu bar; open the main window from a shortcut or the menu. Applies at next launch',
  '查询时，自动将单个单词文本分词为 key_value —> key value':
    'When querying, automatically split single word text into key_value —> key value',
  '单个代码标识符按 _ 与驼峰拆词后再查询':
    'Break a lone identifier at _ and camelCase before translating',
  '查询时，自动移除文本中的代码注释符号「/*#」':
    'When querying, automatically remove the code comment symbol "/*#" from the text.',
  '划词或粘贴代码时去掉 //、# 与 /* */ 再翻译':
    'Strip //, # and /* */ from code before translating',
  '查询时，自动将文本中的「换行符」替换为「空格」':
    'When querying, automatically replace the "newline" in the text with "space"',
  '多行文本合并成一行再翻译': 'Join multi-line text into one line before translating',
  '窗口最大高度百分比': 'Maximum window height',
  '当窗口显示时自动钉住': 'Pin the window whenever it shows',
  '主窗口每次出现时都置顶，方便对照原文':
    'Keeps the translation window on top each time it opens, so it can sit beside a document',
  '自动复制划词文本': 'Copy selected text automatically',
  '划词查询时把选中的文字写入剪贴板': 'Put the highlighted text on the clipboard when a selection is translated',
  '自动复制截图 OCR 结果': 'Copy the screenshot OCR text automatically',
  '截图翻译时把识别出的文字写入剪贴板': 'Put recognized screenshot text on the clipboard',
  '图片 OCR 后自动查询': 'Translate OCR text automatically',
  '关闭后截图只显示识别出的文字，再由「全部重新查询」翻译':
    'Off: a screenshot stops at the recognized text; use "Translate again" when ready',
  '启用 OCR 文本格式优化': 'Enable OCR text formatting optimization',
  'OCR 原始识别文本格式可能有误，开启该选项后，将会对文本进行格式优化处理。':
    'OCR text may be misformatted; when this is on, the text is cleaned up before it is used.',
  '有道 OCR 识别': 'Youdao OCR',
  '当默认OCR识别失败时，自动使用有道OCR':
    'When the default OCR fails, fall back to Youdao OCR',
  '隐藏截图提示': 'Hide the screenshot hint',
  '开始截图时隐藏提示视图。': 'Hide the hint view while taking a screenshot.',
  '划词后自动查询': 'Query after selecting text',
  '划词后自动显示查询图标': 'Show a query icon after selecting',
  '在选区旁显示查询按钮，点击即可翻译': 'Put a query button beside the selection; click it to translate',
  '仅当选中文本语言不是': 'Only when the selected language is not',
  '最小字符长度': 'Minimum characters',
  '仅当选中文本语言≠指定语言 且 字符数≥该值时显示查询图标。':
    'The icon appears only when the selection is in another language and has at least this many characters.',
  '点击划词查询图标时才查询（需隐藏主窗口）':
    'Query only when the icon is clicked (requires hiding the main window)',
  '划词后不再自动查询，只在点击图标时发问':
    'A selection no longer asks by itself; the icon is the only trigger',
  '主窗口': 'Main window',
  '选中文字即翻译，无需按快捷键':
    'Translate on select, no shortcut needed',
  '隐藏与原文相同的译文': 'Hide translations equal to the source',
  '源语言与目标语言相同时不再重复显示': 'Nothing is shown twice when source and target match',
  '在程序坞中显示图标': 'Show a Dock tile',
  '在任务栏中显示图标': 'Show a taskbar button',
  '关闭后只保留菜单栏入口，主窗口仍由快捷键唤起':
    'When off, only the menu bar entry remains and the shortcut still summons the window',
  '任务栏按钮属于窗口，改动在下次打开主窗口时生效':
    'The taskbar button belongs to the window, so this applies the next time it opens',
  '隐藏菜单栏图标': 'Hide the menu bar icon',
  '检测到当前未设置输入翻译快捷键或划词翻译快捷键！请先设置快捷键再隐藏菜单栏图标。':
    'Neither the input nor the selection shortcut is set. Set one before hiding the menu bar icon.',
  '菜单栏不再出现图标，只留快捷键与程序坞入口':
    'No menu bar icon; the shortcuts and the Dock tile remain',
  '隐藏系统托盘图标': 'Hide the tray icon',
  '托盘不再出现图标，只留快捷键与任务栏入口':
    'No tray icon; the shortcuts and the taskbar button remain',
  '自动朗读': 'Read aloud',
  '权限': 'Permissions',
  '屏幕录制': 'Screen Recording',
  '辅助功能': 'Accessibility',
  '已授权': 'Granted',
  '未授权': 'Not granted',
  '请求权限': 'Request Permission',
  '打开系统设置': 'Open System Settings',
  '先点「请求权限」让系统登记 Transdict；如果列表里仍然没有，点「打开系统设置」，用 + 添加 /Applications/Transdict.app。改完需要重启 Transdict 才生效。':
    'Choose Request Permission first so macOS registers Transdict; if it still is not listed, use Open System Settings and add /Applications/Transdict.app with +. Restart Transdict afterwards.',
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
  '取词时，移除 Books 摘录中的版权信息': 'Remove the copyright notice from Books excerpts when looking up',
  '从 Apple Books 划词时只查询引号内的正文，不翻译版权声明':
    'For a selection from Apple Books, query only the quoted text and leave the copyright notice out.',
  'HTTP 服务器': 'HTTP server',
  '开启 HTTP 服务器': 'Enable HTTP server',
  '修改端口后需要重新开启服务器': 'Restart the server after changing the port',
  '只监听本机 127.0.0.1，供其他应用调用翻译、OCR 与取词，修改后立即生效':
    'Listens on 127.0.0.1 only, so other apps can call translation, OCR and selection. Changes apply immediately.',
  'HTTP 端口': 'HTTP port',
  'POST /translate、POST /ocr、POST /detect、GET /selectedText。':
    'Endpoints: POST /translate, POST /ocr, POST /detect, GET /selectedText.',
  '数据': 'Data',
  '日志': 'Logs',
  '调试日志': 'Debug Log',
  '还没有日志': 'Nothing logged yet',
  '显示调试日志窗口': 'Show Debug Log',
  '导出日志': 'Export Log',
  '导出日志…': 'Exporting log…',
  '日志目录': 'Log folder',
  '日志目录不可用': 'Log folder unavailable',
  '打开日志目录…': 'Opening log folder…',
  '在 Finder 中显示': 'Reveal in Finder',
  '生词本': 'Vocabulary notebook',
  '将每次查询追加保存到本地 JSONL 文件，供外部回顾程序读取。':
    'Append every query to a local JSONL file that an external review tool can read.',
  '存储目录': 'Folder',
  '未选择': 'Not chosen',
  '所选目录不可写。': 'The chosen folder is not writable.',
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
  '请先输入要查询的文字': "Type something to look up first",
  '无法打开浏览器': "Could not open the browser",
  '无法打开系统词典': "Could not open the system dictionary",
  '未找到欧路词典，请确认已安装': "Eudic was not found; check that it is installed",
  '字号 {n}/5': "Text size {n}/5",
  '没有可播放的文字': "There is no text to read aloud",
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
  // Anki Connect: the settings page and the card action that writes to it.
  '启用 Anki Connect': 'Enable Anki Connect',
  '词典卡片出现「添加到 Anki」，把词条存进下面的牌组':
    'Dictionary cards get an Add to Anki button that saves the entry to this deck',
  '端点': 'Endpoint',
  '牌组': 'Deck',
  '笔记类型': 'Note type',
  '默认 Anki Connect 地址为 http://127.0.0.1:8765':
    'The default Anki Connect address is http://127.0.0.1:8765',
  '测试连接': 'Test connection',
  '连接中…': 'Connecting…',
  '已连接 AnkiConnect {v}': 'Connected to AnkiConnect {v}',
  'Anki Connect 未启用：卡片上不会出现添加按钮。':
    'Anki Connect is off, so cards have no add button.',
  'Anki 字段': 'Anki fields',
  '每行把一个字段名和内容模板配在一起；模板可用 {Word}、{Translation}、{Definition}、{Phonetic}、{Exchange}、{Related}、{FullResult}、{DictionaryText}、{DictionaryHTML}、{Etymology}，字段名要和笔记类型里的完全一致。':
    'Each row pairs a field name with a template; templates can use {Word}, {Translation}, {Definition}, {Phonetic}, {Exchange}, {Related}, {FullResult}, {DictionaryText}, {DictionaryHTML} and {Etymology}, and field names must match the note type exactly.',
  '单词': 'Word',
  '词典服务': 'Dictionary service',
  '请输入要预览的单词': 'Enter a word to preview',
  '请选择一个词典服务': 'Choose a dictionary service',
  '释义': 'Definition',
  '音标': 'Phonetic',
  '词形变化': 'Word Forms',
  '相关词': 'Related Words',
  '同义词': 'Synonyms',
  '短语': 'Phrases',
  '完整结果': 'Full Result',
  '词典文本': 'Dictionary Text',
  '词典 HTML': 'Dictionary HTML',
  '词源': 'Etymology',
  '插入变量': 'Insert variable',
  '插入变量…': 'Insert variable…',
  '字段名': 'Field name',
  '内容模板': 'Content template',
  '选择字段…': 'Choose a field…',
  '删除映射': 'Remove mapping',
  '添加映射': 'Add mapping',
  '从 Anki 读取字段': 'Read fields from Anki',
  '读取字段…': 'Reading fields…',
  '读取到 {n} 个字段': 'Read {n} fields',
  '笔记类型没有可用字段': 'The note type has no fields',
  '缺少密钥': 'Missing secret key',
  '苹果离线翻译需要先在系统设置中下载语言：':
    'Apple offline translation needs languages downloaded in System Settings first: ',
  '管理下载语言': 'Manage downloaded languages',
  '请前往服务官网注册申请个人的 API key。':
    'Please go to the official service website to register and apply for a personal API key.',
  '映射的 Anki 字段不能重复': 'Mapped Anki fields must be different',
  '预览': 'Preview',
  '你好': 'Hello',
  '添加到 Anki': 'Add to Anki',
  '已添加到 Anki': 'Added to Anki',
  'Anki Connect 未启用': 'Anki Connect is not enabled',
  '没有可添加到 Anki 的单词': 'There is no word to add to Anki',
  'Anki 中没有这个牌组': 'Anki has no deck with that name',
  'Anki 中没有这个模型': 'Anki has no note type with that name',
  '无法连接 AnkiConnect': 'Cannot reach AnkiConnect',
  'AnkiConnect 响应无法解析': "AnkiConnect's reply could not be read",
  'Anki Connect 地址无效': 'The Anki Connect address is not valid',
  '显示语言选择栏': 'Show language bar',
  '窗口配置': 'Window configuration',
  '服务与窗口': 'Services and windows',
  '显示文本输入框': 'Show text input',
  '窗口上方显示可编辑的原文，Enter 重新查询':
    'Show an editable source box above the answer; Enter re-queries',
  '标题栏显示译文语言，可直接切换':
    'Show the target language in the title bar and switch it there',
  '显示 OCR 菜单项': 'Show OCR menu items',
  '在菜单栏中显示 OCR 相关的菜单项（截图 OCR、剪贴板 OCR、OCR 窗口）。':
    'Show OCR-related items in the menu bar (Screenshot OCR, Clipboard OCR, OCR window).',
  '检测古文的最小文本长度': 'Minimum length to detect Classical Chinese',
  '关闭检测': 'Off',
  '文本达到该长度才判定是否为古文；设为 0 关闭古文检测。':
    'Only test for Classical Chinese once the text reaches this length; set 0 to turn the test off.',
  '关于': 'About',
  '版本': 'Version',
  '版本 {v}': 'Version {v}',
  'GitHub 项目主页': 'GitHub repository',
  '贡献者': 'Contributors',
  '隐私声明': 'Privacy Statement',
  '致谢': 'Acknowledgements',
  '界面基于 Wails 与 React；识别与语言判断使用苹果的 Vision 与自然语言框架；翻译、词典与朗读由你启用的服务提供。':
    'The interface is built on Wails and React; recognition and language detection use Apple’s Vision and Natural Language frameworks; translation, dictionaries and speech come from the services you enable.',
  '高级': 'Advanced',
  '隐私': 'Privacy',
  '启动与更新': 'Startup & Updates',
  '收藏的单词': 'Saved Words',
  '读取中…': 'Loading…',
  '禁止名单只影响鼠标划词，当前平台没有自动划词。':
    'The blocklist only affects selection queries, which this platform does not have.',
  '查询的文字与截图只会发送给「服务」页里启用的服务；历史、收藏、生词本与配置保存在本机用户目录，不会上传。':
    'Queried text and screenshots are sent only to the services enabled on the Services page; history, favorites, the notebook and settings stay in your local user folder and are never uploaded.',
  '好': 'OK',
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

/** uiLocale is for the few labels built from data rather than a sentence —
 * phonetics come back as US/UK and read 美/英 in a Chinese UI. */
export function uiLocale(): 'en' | 'zh-CN' {
  return locale
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
