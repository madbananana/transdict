// The translation window: one input, one answer. Preferences and history each
// live in their own window, which is what keeps this one narrow enough to sit
// beside the app the text was copied from.
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ResultColumns } from './Result'
import {
  Service, SettingsService, applyTheme, errMsg, isMac, langName, onBusy, onCommand, onResult,
  prettyShortcut, readAloud, roleOf, useLangNames, type Config, type Lang, type Response,
} from './api'
import { applyLocale, t } from './i18n'
import { autoReadAnswer } from './autoread'
import WindowControls from './WindowControls'

// AI instructions reuse the same providers with a different prompt, so 润色 and
// 总结 need no second pipeline.
const instructions: { label: string; text: string }[] = [
  { label: t('润色'), text: t('在不改变原意的前提下润色下面的文字，使其更地道、通顺：') },
  { label: t('总结'), text: t('用三句以内的中文总结下面的文字：') },
  { label: t('更正式'), text: t('把下面的文字改写得更正式、书面化：') },
  { label: t('更口语'), text: t('把下面的文字改写得更口语、自然：') },
  { label: t('纠错'), text: t('纠正下面文字中的拼写、语法与标点错误，只输出修改后的文字：') },
  { label: t('扩写'), text: t('在保持原意的前提下扩写下面的文字：') },
]

// Language flags make the pair readable before the names are: the picker is
// the first thing under the input, and 59 rows of text are slow to scan.
// Windows' emoji font draws regional indicators as two letters, so the flag is
// only prefixed on macOS.
const flags: Record<string, string> = {
  auto: '🌐', 'zh-CN': '🇨🇳', 'zh-TW': '🇹🇼', en: '🇺🇸', ja: '🇯🇵', ko: '🇰🇷',
  fr: '🇫🇷', de: '🇩🇪', es: '🇪🇸', ru: '🇷🇺', it: '🇮🇹', pt: '🇵🇹', th: '🇹🇭', vi: '🇻🇳',
  ar: '🇸🇦', nl: '🇳🇱', pl: '🇵🇱', tr: '🇹🇷', fa: '🇮🇷', hi: '🇮🇳', id: '🇮🇩', ms: '🇲🇾',
  sv: '🇸🇪', no: '🇳🇴', da: '🇩🇰', fi: '🇫🇮', cs: '🇨🇿', sk: '🇸🇰', hu: '🇭🇺', ro: '🇷🇴',
  bg: '🇧🇬', el: '🇬🇷', uk: '🇺🇦', he: '🇮🇱', ca: '🇦🇩', et: '🇪🇪', lv: '🇱🇻', lt: '🇱🇹',
  sl: '🇸🇮', hr: '🇭🇷', sr: '🇷🇸', mk: '🇲🇰', sq: '🇦🇱', hy: '🇦🇲', az: '🇦🇿', be: '🇧🇾',
  ka: '🇬🇪', kk: '🇰🇿', ky: '🇰🇬', lo: '🇱🇦', my: '🇲🇲', ne: '🇳🇵', mn: '🇲🇳', tl: '🇵🇭',
}

const langLabel = (code: string, name: string, withFlag = true) => {
  const flag = flags[code]
  return isMac && withFlag && flag ? `${flag} ${name}` : name
}

// The language row drops its flag emoji at the minimum width: an English
// "Simplified Chinese" needs those pixels to fit the select instead of being
// cut mid-word by the native control.
function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => window.innerWidth < 460)
  useEffect(() => {
    const onResize = () => setNarrow(window.innerWidth < 460)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return narrow
}

export default function Panel() {
  const [res, setRes] = useState<Response | null>(null)
  const [busy, setBusy] = useState('')
  // note is the window's one-line answer to "why did that button do nothing" —
  // a missing voice, an unrecorded query, a clipboard that refused the write.
  const [note, setNote] = useState('')
  // permNeed names the system grant an action was refused for, so the note can
  // carry the one button that fixes it instead of only naming a checkbox the
  // user then has to find.
  const [permNeed, setPermNeed] = useState('')
  const [cfg, setCfg] = useState<Config | null>(null)
  const [langs, setLangs] = useState<Lang[]>([])
  const [text, setText] = useState('')
  const [source, setSource] = useState('auto')
  const [target, setTarget] = useState('zh-CN')
  const [dictOnly, setDictOnly] = useState(false)
  const [pinned, setPinned] = useState(false)
  const narrow = useNarrow()
  // aiKinds is the set of translate kinds that take an instruction. Without one
  // of them the 润色/总结 row would send a plain sentence translator a prompt it
  // cannot honour and quietly return an ordinary translation.
  const [aiKinds, setAiKinds] = useState<Set<string>>(new Set())
  const langNames = useLangNames()
  const box = useRef<HTMLTextAreaElement | null>(null)
  // pinPending covers the window between the click and the store's answer: the
  // focus-event reload below reads the store, and an older snapshot arriving
  // after the toggle would repaint the pin in the state the user just left.
  const pinPending = useRef(false)
  // textRef and sent track what the box holds against what it last submitted,
  // so a result that did not come from the box can clear an untouched draft.
  const textRef = useRef('')
  const sent = useRef('')

  // cfgRef keeps the event handlers from closing over a stale config.
  const cfgRef = useRef<Config | null>(null)

  useEffect(() => {
    if (!note) return
    // A permission note is a sentence plus a trip through System Settings;
    // five seconds would clear it before the user finished reading.
    const wait = permNeed ? 20000 : 5000
    const timer = setTimeout(() => {
      setNote('')
      setPermNeed('')
    }, wait)
    return () => clearTimeout(timer)
  }, [note, permNeed])

  const applyCfg = useCallback((next: Config) => {
    cfgRef.current = next
    setCfg(next)
    setSource(next.sourceLang || 'auto')
    setTarget(next.targetLang)
    if (!pinPending.current) setPinned(!!next.pinned)
    applyTheme(next.theme)
    applyLocale(next.uiLang)
  }, [])

  const flipPin = () => {
    pinPending.current = true
    void Service.TogglePin()
      .then((on) => {
        setPinned(on)
        if (cfgRef.current) cfgRef.current = { ...cfgRef.current, pinned: on }
      })
      .finally(() => {
        pinPending.current = false
      })
  }

  // The preferences window can change the target language while this one is
  // open, so the select re-reads the store whenever the window comes forward.
  useEffect(() => {
    const sync = () => void SettingsService.Get().then(applyCfg)
    window.addEventListener('focus', sync)
    return () => window.removeEventListener('focus', sync)
  }, [applyCfg])

  useEffect(() => {
    const offResult = onResult('main', (next) => {
      setRes(next)
      // A partial answer carries the recognized text only; the translation
      // stage is still running, so the progress line stays and nothing is read.
      if (next.partial) return
      setBusy('')
      setNote('')
      setPermNeed('')
      // 截图、剪贴板与划词 answers have nothing to do with the text still in the
      // box, and leaving it there reads as if it were the source. A draft the
      // user has started editing is left alone.
      if (next.mode !== 'input' && next.mode !== 'prompt' && textRef.current === sent.current) {
        textRef.current = ''
        sent.current = ''
        setText('')
      }
      autoRead(next, cfgRef.current, setNote)
    })
    const offBusy = onBusy('main', setBusy)
    const offCommand = onCommand((name) => {
      if (name === 'input') box.current?.focus()
    })
    void SettingsService.Get().then(applyCfg)
    void SettingsService.Languages().then((ls) => setLangs(ls ?? []))
    void SettingsService.ServiceTypes('translate').then((ts) =>
      setAiKinds(new Set((ts ?? []).filter((x) => x.prompt).map((x) => x.kind))))
    // The native close button destroys the window, so the webview that comes
    // back after the next shortcut is a fresh one. The answer is still in Go;
    // showing it again is what keeps 收起 and closing the red button from
    // throwing away what the user was reading.
    void Service.PanelLast().then((last) => {
      if (!last || roleOf(last.context) !== 'main') return
      setRes(last)
      // A restored answer usually quotes the text that was in the box, so put
      // it back: an empty input under a filled result looks like the query was
      // never entered.
      if (last.mode === 'input' && last.sourceText) {
        textRef.current = last.sourceText
        sent.current = last.sourceText
        setText(last.sourceText)
      }
    })

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') void Service.ClosePanel()
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        box.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      offResult()
      offBusy()
      offCommand()
      window.removeEventListener('keydown', onKey)
    }
  }, [applyCfg])

  const run = (instruction?: string, langs?: { sourceLang: string; targetLang: string }) => {
    const value = text.trim()
    if (!value) {
      setNote(t('请输入文字'))
      box.current?.focus()
      return
    }
    sent.current = value
    if (instruction) {
      void Service.Prompt(value, instruction).catch((e) => setNote(errMsg(e)))
      return
    }
    void Service.Ask({
      context: 'main', mode: 'input', text: value,
      sourceLang: langs?.sourceLang ?? source, targetLang: langs?.targetLang ?? target,
      dictionaryOnly: dictOnly,
    }).catch((e) => setNote(errMsg(e)))
  }

  // swap turns the pair around and asks the same text the other way. With the
  // source on 自动检测 the detected language stands in; when that is already the
  // target (Chinese text translated into Chinese) the useful other side is the
  // language the user reads, not a no-op.
  const swap = () => {
    let from = source
    if (from === 'auto') {
      from = res?.detected && res.detected !== target ? res.detected : target === 'en' ? 'zh-CN' : 'en'
    }
    if (from === target) return
    setSource(target)
    setTarget(from)
    void SettingsService.SetSourceLang(target).catch((e) => setNote(errMsg(e)))
    void Service.SetTargetLang(from).catch((e) => setNote(errMsg(e)))
    if (text.trim() !== '') run(undefined, { sourceLang: target, targetLang: from })
  }

  // fail reports a refused action and remembers when the reason was a missing
  // system grant, so the line can carry the button that opens the right pane.
  const fail = (e: unknown) => {
    const raw = e instanceof Error ? e.message : String(e ?? '')
    setPermNeed(raw.includes('辅助功能') ? 'access' : raw.includes('屏幕录制') ? 'screen' : '')
    setNote(errMsg(e))
  }

  const ocrCount = (res?.ocr ?? []).length
  const trCount = (res?.translate ?? []).length
  // A pending row is a reserved place, not an answer; counting it would show
  // 2/2 before the first service has replied.
  const okCount = useMemo(() => (res?.translate ?? []).filter((x) => !x.error && !x.pending).length, [res])
  const dictHits = useMemo(
    () => (res?.dict ?? []).filter((x) => !x.error && (x.entries ?? []).length > 0).length,
    [res],
  )
  // 自动检测 + Chinese target answers Chinese text in English, the way Easydict
  // does. The picker still shows the chosen target, so the footer says which
  // language the answer actually arrived in.
  const answeredIn = res && res.targetLang && target && res.targetLang !== target
    ? langName(res.targetLang, langNames) || res.targetLang
    : ''
  const canPrompt = (cfg?.translate ?? []).some((p) => p.enabled && aiKinds.has(p.kind))

  return (
    <div className="shell">
      <div className="dragbar">
        {/* Only this group is the native caption region: the buttons beside it
            would otherwise be swallowed by window drags. */}
        <div className="drag">
          <span className="title">Transdict</span>
          <span className="status">
            {busy || (res ? t('耗时 {n} ms', { n: res.elapsedMs }) : t('就绪'))}
          </span>
        </div>
        <button
          className={`ghost tiny icon pin ${pinned ? 'on' : ''}`}
          title={pinned ? t('取消置顶') : t('置顶')}
          aria-pressed={pinned}
          onClick={flipPin}
        >
          <PinIcon filled={pinned} />
        </button>
        <button className="ghost tiny icon" title={t('设置')} onClick={() => void Service.OpenSettings()}>
          <GearIcon />
        </button>
        <WindowControls />
        {/* The red traffic light is the close button on macOS; a second ✕ beside
            it read as a duplicate. Windows has no traffic lights, so there the
            header still needs one — and it hides rather than quits. */}
        {!isMac && (
          <button className="ghost tiny icon" title={t('收起')} onClick={() => void Service.ClosePanel()}>
            <CloseIcon />
          </button>
        )}
      </div>

      <div className="compose">
        <textarea
          ref={box}
          className="compose-input"
          value={text}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          placeholder={isMac
            ? t('输入或粘贴要翻译的文字，⌘ + Return 查询')
            : t('输入或粘贴要翻译的文字，Ctrl + Enter 查询')}
          onChange={(e) => {
            setText(e.target.value)
            textRef.current = e.target.value
          }}
          // ⌘/Ctrl+↵ runs; a plain ↵ stays a newline, which multi-line text needs.
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              run()
            }
          }}
        />
        {/* Matches the clear affordance in the input's corner: the answer can
            stay on screen while the box is emptied for the next query. */}
        {text && (
          <button
            className="compose-clear"
            title={t('清空输入')}
            aria-label={t('清空输入')}
            onClick={() => {
              setText('')
              textRef.current = ''
              box.current?.focus()
            }}
          >
            ✕
          </button>
        )}
        <div className="compose-row">
          <select
            value={source}
            aria-label={t('源语言')}
            onChange={(e) => {
              setSource(e.target.value)
              void SettingsService.SetSourceLang(e.target.value).catch((err) => setNote(errMsg(err)))
            }}
          >
            <option value="auto">{langLabel('auto', t('自动检测'), !narrow)}</option>
            {langs.filter((l) => l.code !== 'auto').map((l) => (
              <option key={l.code} value={l.code}>{langLabel(l.code, l.name, !narrow)}</option>
            ))}
          </select>
          <button
            className="swap"
            title={t('交换语言')}
            aria-label={t('交换语言')}
            onClick={swap}
          >⇄</button>
          <select
            value={target}
            aria-label={t('目标语言')}
            onChange={(e) => {
              setTarget(e.target.value)
              void Service.SetTargetLang(e.target.value).catch((e2) => setNote(errMsg(e2)))
            }}
          >
            {langs.filter((l) => l.code !== 'auto').map((l) => (
              <option key={l.code} value={l.code}>{langLabel(l.code, l.name, !narrow)}</option>
            ))}
          </select>
          <button className="primary" onClick={() => run()}>{t('翻译')}</button>
        </div>
        {canPrompt && text.trim() !== '' && (
          <div className="chips scroll">
            {instructions.map((i) => (
              <button key={i.label} className="chip" disabled={!!busy} onClick={() => run(i.text)}>
                {i.label}
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="toolbar">
        <button className="tiny ghost" onClick={() => void Service.Start().catch(fail)}>
          <ToolIcon id="capture" />{t('截图翻译')}
        </button>
        <button className="tiny ghost" onClick={() => void Service.CatchSelection().catch(fail)}>
          <ToolIcon id="select" />{t('划词翻译')}
        </button>
        <button className="tiny ghost" onClick={() => void Service.ClipboardQuery().catch((e) => setNote(errMsg(e)))}>
          <ToolIcon id="clipboard" />{t('剪贴板')}
        </button>
        <button
          className="tiny ghost"
          disabled={!res?.sourceText}
          onClick={() => void readAloud(res?.sourceText ?? '', res?.detected ?? 'auto', false, setNote)}
        >
          <ToolIcon id="speak" />{t('朗读')}
        </button>
        {/* The mode and History travel together: when the row runs out of
            width, the pair drops as one right-aligned group instead of leaving
            History alone under a row of tools. */}
        <div className="toolbar-end">
          <label className="check toolbar-check" title={t('只查词典，不调用翻译服务')}>
            <input type="checkbox" checked={dictOnly} onChange={(e) => setDictOnly(e.target.checked)} />
            <span>{t('仅词典')}</span>
          </label>
          <button className="tiny ghost" onClick={() => void Service.OpenHistory()}>
            <ToolIcon id="history" />{t('历史')}
          </button>
        </div>
      </div>

      {note && (
        <div className={`noteline${permNeed ? ' withaction' : ''}`}>
          <span>{note}</span>
          {permNeed && (
            <button className="tiny" onClick={() => void Service.OpenPermission(permNeed)}>
              {t('打开系统设置')}
            </button>
          )}
        </div>
      )}

      {!res ? (
        <EmptyState cfg={cfg} onFocus={() => box.current?.focus()} />
      ) : (
        <ResultColumns compact res={res} busy={busy} notice={setNote} langNames={langNames} />
      )}

      <div className="footer">
        <span>
          {res
            ? [
                // A failed OCR has no translations to count; 0/0 译文 next to
                // the failure only adds noise.
                trCount > 0 ? t('{ok}/{all} 译文', { ok: okCount, all: trCount }) : '',
                ocrCount > 0 ? t('{n} 识别', { n: ocrCount }) : '',
                dictHits > 0 ? t('{n} 词典', { n: dictHits }) : '',
                answeredIn && okCount > 0 ? t('已译为{lang}', { lang: answeredIn }) : '',
              ].filter(Boolean).join(' · ')
            : t('已启用 {n} 个翻译服务', { n: (cfg?.translate ?? []).filter((p) => p.enabled).length })}
        </span>
        <span className="spacer" />
        {!res && <span>{isMac ? '⌘K' : 'Ctrl+K'} {t('聚焦输入')}</span>}
      </div>
    </div>
  )
}

// ToolIcon gives the query row a shape to find before reading the label. The
// icons stay monochrome so the primary 翻译 button is still the one colour.
function ToolIcon({ id }: { id: 'capture' | 'select' | 'clipboard' | 'speak' | 'history' }) {
  const paths: Record<string, ReactNode> = {
    capture: (
      <>
        <path d="M4 8V6a2 2 0 0 1 2-2h2M16 4h2a2 2 0 0 1 2 2v2M20 16v2a2 2 0 0 1-2 2h-2M8 20H6a2 2 0 0 1-2-2v-2" />
        <path d="M8 12h8" />
      </>
    ),
    select: <path d="m5 4 6 16 2.4-6.6L20 11 5 4Z" />,
    clipboard: (
      <>
        <rect x="8" y="3" width="8" height="4" rx="1" />
        <path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2" />
      </>
    ),
    speak: (
      <>
        <path d="M11 5 7 9H4v6h3l4 4V5Z" />
        <path d="M15.5 8.5a5 5 0 0 1 0 7" />
      </>
    ),
    history: (
      <>
        <circle cx="12" cy="12" r="8" />
        <path d="M12 8v4l3 2" />
      </>
    ),
  }
  return (
    <svg
      className="pico"
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[id]}
    </svg>
  )
}

// PinIcon is the 置顶 toggle: a pushpin standing up and filled when the window
// stays on top, outlined and dim when it does not.
function PinIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M12 17v5" />
      <path d="M9 10.8a2 2 0 0 1-1.1 1.8l-1.8.9A2 2 0 0 0 5 15.2V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.8a2 2 0 0 0-1.1-1.8l-1.8-.9A2 2 0 0 1 15 10.8V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1Z" />
    </svg>
  )
}

// The header's three controls are drawn at one weight, so the icon row reads as
// one group instead of a pin beside two text glyphs.
function GearIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.09a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.09a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
    </svg>
  )
}

function CloseIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  )
}

// autoRead is 自动朗读: the answer speaks itself, so a learner hears the word
// without reaching for a button.
function autoRead(res: Response, cfg: Config | null, notice: (msg: string) => void) {
  const read = autoReadAnswer(res, cfg?.ttsAuto ?? 'off')
  if (read) void readAloud(read.text, read.lang, false, notice)
}

function EmptyState({ cfg, onFocus }: { cfg: Config | null; onFocus: () => void }) {
  return (
    <div className="empty">
      <div className="big">{t('框选屏幕上的任何文字')}</div>
      <div className="dimline">{t('识别、翻译并查词典，结果留在这一屏')}</div>
      <div className="cta">
        <button className="primary" onClick={() => void Service.Start()}>{t('立即截图')}</button>
        <button onClick={onFocus}>{t('输入文字')}</button>
      </div>
      <div className="hint">
        <kbd>{prettyShortcut(cfg?.shortcuts?.capture?.[0] ?? '')}</kbd>
        <span>{t('随时截屏翻译')}</span>
      </div>
    </div>
  )
}
