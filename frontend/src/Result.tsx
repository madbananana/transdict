// Result rendering. One set of components draws every answer, so a change to
// how a dictionary entry looks reaches the whole app.
import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react'
import type { QuickLink } from './links'
import {
  Service, SettingsService, errMsg, langName, openQuickLink, openURL, stopSpeech, toggleSpeech, usePlayingKey,
  type DictResult, type Entry, type OCROutcome, type Response, type TranslateResult,
} from './api'
import { speechKey } from './speech'
import type { MaskRect } from '../bindings/transdict/internal/capture/models.js'
import { renderMarkdown } from './markdown'
import { t, uiLocale } from './i18n'
import { ankiNote, copyAll, firstResult, phones, plainEntry } from './copy'
import { linkSpans } from './links'
import { useWordMenu } from './wordmenu'

/** Notice tells the window why something did not happen. A button that fails
 * quietly reads as a broken app, so every action that can come back empty —
 * read-aloud with no voice, 收藏 on a query that was never recorded — reports here. */
type Notice = (msg: string) => void

// CopyButton answers in place: the label changes for a moment instead of a
// note line appearing above the cards, which would push the whole answer down
// every time something is copied. A refused write still gets the note.
// In icon mode the glyph turns into a check for the same moment, the way a
// card action in EasyDict confirms itself without adding words to the row.
function CopyButton({ text, label, notice, disabled, icon }: {
  text: string; label?: string; notice?: Notice; disabled?: boolean; icon?: boolean
}) {
  const [done, setDone] = useState(false)
  useEffect(() => {
    if (!done) return
    const timer = setTimeout(() => setDone(false), 1200)
    return () => clearTimeout(timer)
  }, [done])
  return (
    <button
      className={icon ? 'act' : `tiny${done ? ' copied' : ''}`}
      title={icon ? (done ? t('已复制') : label ?? t('拷贝文本')) : done ? t('已复制') : undefined}
      aria-label={icon ? t('拷贝文本') : undefined}
      disabled={disabled}
      onClick={() => {
        void Service.CopyText(text)
          .then((ok) => {
            if (ok) setDone(true)
            else notice?.(t('复制失败，请手动选择文字'))
          })
          .catch(() => notice?.(t('复制失败，请手动选择文字')))
      }}
    >
      {icon ? <ActIcon id={done ? 'check' : 'copy'} /> : label ?? t('复制')}
    </button>
  )
}

type Box = { x: number; y: number; w: number; h: number }

// An answer that links out must not navigate the window: the same click in a
// browser would lose the result the user is reading, so the address goes to the
// system browser instead.
export function followLinks(e: MouseEvent<HTMLDivElement>) {
  const link = (e.target as HTMLElement).closest?.('a')
  if (!link) return
  e.preventDefault()
  openURL(link.getAttribute('href') ?? '')
}

export function Dot({ bad }: { bad?: boolean }) {
  return <span className={`dot ${bad ? 'bad' : ''}`} />
}

// LoadingDots is EasyDict's EZLoadingAnimationView: five 4px dots that swell in
// sequence while a service is answering. The label is the text a screen reader
// announces; sighted readers see the wave instead of a "翻译中…" line.
export function LoadingDots({ label }: { label: string }) {
  return (
    <span className="loading-dots" role="status" aria-label={label}>
      <i /><i /><i /><i /><i />
    </span>
  )
}

// WarningGlyph is EasyDict's disabled.symbol — a slashed circle in the service
// orange. It marks an answer the service could not give, where a red failure
// mark would suggest something broke.
function WarningGlyph({ title }: { title?: string }) {
  return (
    <svg className="warn-glyph" viewBox="0 0 16 16" role="img" aria-label={t('无结果')}>
      <title>{title || t('无结果')}</title>
      <circle cx="8" cy="8" r="6.3" fill="none" stroke="currentColor" strokeWidth="1.7" />
      <line x1="3.9" y1="12.1" x2="12.1" y2="3.9" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
    </svg>
  )
}

// ErrorGlyph is EasyDict's red error image: a fault the retry button can fix,
// as opposed to the orange ⊘ of a service that simply had no answer.
function ErrorGlyph({ title }: { title?: string }) {
  return (
    <svg className="err-glyph" viewBox="0 0 16 16" role="img" aria-label={t('错误')}>
      <title>{title || t('错误')}</title>
      <circle cx="8" cy="8" r="6.3" fill="none" stroke="currentColor" strokeWidth="1.7" />
      <line x1="8" y1="4.4" x2="8" y2="8.9" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
      <circle cx="8" cy="11.4" r="0.95" fill="currentColor" />
    </svg>
  )
}


// failedText is EasyDict's query_failed prefix: every failure line opens with
// 💥 失败 so a broken service reads as broken before the sentence is parsed.
export function failedText(message: string) {
  return `💥 ${t('失败')}, ${message}`
}

// Speaker marks a phonetic that can be played: the symbol alone reads as text,
// and a click target with no visible affordance is one nobody finds.
function Speaker() {
  return (
    <svg
      className="spk"
      width="11"
      height="11"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M11 5 6 9H2v6h4l5 4z" />
      <path d="M15.5 8.5a5 5 0 0 1 0 7" />
      <path d="M18.5 5.5a9 9 0 0 1 0 13" />
    </svg>
  )
}

// Chevron marks a result card as foldable and turns with the card's state.
function Chevron({ open }: { open: boolean }) {
  return (
    <svg
      className={`chev${open ? ' open' : ''}`}
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m9 6 6 6-6 6" />
    </svg>
  )
}

// ActIcon draws the per-card actions. Icons keep every card's action row the
// same height whatever a service can do, the way EasyDict's cards do; the
// button's title carries the word.
function ActIcon({ id }: { id: 'speak' | 'stop' | 'halt' | 'copy' | 'check' | 'link' | 'retry' | 'anki' | 'replace' }) {  const paths = {
    speak: (
      <>
        <path d="M11 5 6 9H2v6h4l5 4z" />
        <path d="M15.5 8.5a5 5 0 0 1 0 7" />
        <path d="M18.5 5.5a9 9 0 0 1 0 13" />
      </>
    ),
    // pause.circle is the glyph EZAudioButton shows while its clip is playing.
    stop: (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M10 9.5v5M14 9.5v5" />
      </>
    ),
    // stop.circle is the glyph on EasyDict's 停止生成 button.
    halt: (
      <>
        <circle cx="12" cy="12" r="9" />
        <rect x="9" y="9" width="6" height="6" rx="1" fill="currentColor" stroke="none" />
      </>
    ),
    copy: (
      <>
        <rect x="8" y="8" width="12" height="12" rx="2.5" />
        <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" />
      </>
    ),
    check: <path d="m5 12.5 4.5 4.5L19 7" />,
    link: (
      <>
        <path d="M10.6 13.4a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1.2 1.2" />
        <path d="M13.4 10.6a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1.2-1.2" />
      </>
    ),
    retry: (
      <>
        <path d="M20 12a8 8 0 1 1-2.4-5.7" />
        <path d="M20 4v4h-4" />
      </>
    ),
    anki: (
      <>
        <path d="M5 4.5A1.5 1.5 0 0 1 6.5 3H18a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H6.5A1.5 1.5 0 0 1 5 19.5z" />
        <path d="M9 3v18" />
        <path d="m13 8 1.2 2.6L17 11.8l-2.8 1.2L13 15.6l-1.2-2.6L9 11.8l2.8-1.2z" />
      </>
    ),
    // arrow.forward.square is EasyDict's replaceTextButton glyph: this answer
    // goes back over the selection the query came from.
    replace: (
      <>
        <rect x="4" y="4" width="16" height="16" rx="3.5" />
        <path d="M8.5 12h7" />
        <path d="m12.5 9 3 3-3 3" />
      </>
    ),
  }
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[id]}
    </svg>
  )
}

// EasyDict's cards lead with the service's own mark; four grey service names
// stacked up read alike without one. The mark is drawn here as the label's
// first character on the service's colour — a stand-in, since the brand assets
// are not ours to ship.
const serviceColors: [RegExp, string][] = [
  [/google/, '#4285f4'],
  [/youdao/, '#d33a31'],
  [/bing/, '#0f7d8c'],
  [/apple|system/, '#7d7d84'],
  [/openai/, '#10a37f'],
  [/deepl/, '#0f2b46'],
  [/baidu/, '#2932e1'],
  [/tencent/, '#1a7af8'],
  [/volc|huoshan/, '#1664ff'],
]

function ServiceBadge({ provider, label }: { provider: string; label: string }) {
  const key = `${provider} ${label}`.toLowerCase()
  const color = serviceColors.find(([re]) => re.test(key))?.[1] ?? '#78808f'
  return (
    <span className="svc" style={{ background: color }} aria-hidden="true">
      {[...label][0] ?? '?'}
    </span>
  )
}

// EasyDict folds a result by tapping anywhere on its top bar (EZResultView's
// topBarTapRecognizer), not just the arrow. Buttons living in that bar keep
// their own click, so a click that lands on one is left alone.
function topBarClick(toggle: () => void) {
  return (e: MouseEvent<HTMLElement>) => {
    if ((e.target as HTMLElement).closest('button, a, input, textarea, select')) return
    toggle()
  }
}

// Links in recognized text: EasyDict's highlightAllLinks turns every URL the
// screenshot contained into a clickable link, and the anchor here opens the
// browser instead of navigating the panel away from the answer.
function withLinks(text: string): ReactNode[] {
  const spans = linkSpans(text)
  if (spans.length === 0) return [text]
  const parts: ReactNode[] = []
  let last = 0
  for (const s of spans) {
    if (s.start > last) parts.push(text.slice(last, s.start))
    parts.push(
      <a key={`${s.start}-${s.url}`} className="srclink" href={s.url}
        onClick={(e) => { e.preventDefault(); openURL(s.url) }}>
        {s.shown}
      </a>,
    )
    last = s.end
  }
  if (last < text.length) parts.push(text.slice(last))
  return parts
}

export function SourceCard({ res, onEdit, notice, langNames = {} }: {
  res: Response; onEdit?: (text: string) => void; notice?: Notice; langNames?: Record<string, string>
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const [masking, setMasking] = useState(false)
  const text = res.sourceText ?? ''
  const ocr = (res.ocr ?? []).filter((o) => !o.error)
  const lang = langName(res.detected, langNames)
  // The speaker is a play/stop toggle while this card's text is the clip in the
  // shared player, as EasyDict's EZAudioButton draws itself.
  const playing = usePlayingKey()
  const speakKey = speechKey(text, res.detected || 'auto', false)

  // A new answer is a new crop; the editor from the previous one belongs to
  // text that is no longer on screen.
  useEffect(() => {
    setMasking(false)
    setDraft(null)
  }, [res.id])

  if (!text && !res.cropDataURL) return null
  return (
    <div className="card source">
      {/* The header comes before the crop: the crop is evidence for the text,
          not a separate section whose title would float underneath it. */}
      <h3>
        {t('原文')}{lang ? ` · ${lang}` : ''}
        <span className="spacer" />
        {ocr.length > 0 && <span className="byline">{ocr.map((o) => o.label).join(' / ')}</span>}
        {res.cropDataURL && draft === null && (
          <button className="tiny" disabled={masking || res.partial} onClick={() => setMasking(!masking)}>{t('遮挡')}</button>
        )}
        {text && draft === null && (
          <>
            <CopyButton text={text} notice={notice} />
            <button className="tiny" onClick={() => void toggleSpeech(text, res.detected || 'auto', false, notice)}>{playing === speakKey ? t('停止') : t('朗读')}</button>
            {onEdit && <button className="tiny" onClick={() => setDraft(text)}>{t('编辑')}</button>}
          </>
        )}
      </h3>
      {res.cropDataURL && (
        <MaskableShot src={res.cropDataURL} masking={masking} onDone={() => setMasking(false)} notice={notice} />
      )}
      {draft === null ? (
        // A crop-only sketch is the panel opening before OCR answers; naming
        // 未识别到文字 there would be a verdict on work that is still running.
        res.partial && !text ? null : (
          // When the query failed, the error card below already says why; a body
          // repeating 未识别到文字 under the crop made the failure look doubled.
          (text || !res.error) && (
            <div className={`body ${text ? '' : 'muted'}`}>{text ? withLinks(text) : t('未识别到文字')}</div>
          )
        )
      ) : (
        <div className="edit">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={4}
            autoFocus
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
          />
          <div className="row-end">
            <button className="ghost" onClick={() => setDraft(null)}>{t('取消')}</button>
            <button
              className="primary"
              onClick={() => {
                const next = draft.trim()
                setDraft(null)
                if (next && next !== text) onEdit?.(next)
              }}
            >
              {t('用这段文字重新查询')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

/** MaskableShot shows the crop and, while 遮挡 is on, lets the user drag squares
 * over what must not leave the machine. The boxes are kept in CSS pixels because
 * that is the space the overlay is drawn in; Go pixelates in the crop's own
 * pixels, so the scale factor comes from the image's natural width. */
function MaskableShot({ src, masking, onDone, notice }:
  { src: string; masking: boolean; onDone: () => void; notice?: Notice }) {
  const [boxes, setBoxes] = useState<Box[]>([])
  const [draft, setDraft] = useState<Box | null>(null)
  const [busy, setBusy] = useState(false)
  const imgRef = useRef<HTMLImageElement | null>(null)
  const origin = useRef<{ x: number; y: number } | null>(null)

  // Closing 遮挡 throws away what was painted: reopening should show the crop,
  // not last round's boxes waiting to be applied twice.
  useEffect(() => {
    if (!masking) {
      setBoxes([])
      setDraft(null)
    }
  }, [masking])

  const paint = (e: MouseEvent<HTMLDivElement>) => {
    if (!origin.current) {
      return
    }
    const { clientWidth: cw, clientHeight: ch } = e.currentTarget
    const at = (v: number, hi: number) => Math.max(0, Math.min(v, hi))
    const x0 = at(Math.min(origin.current.x, e.nativeEvent.offsetX), cw)
    const y0 = at(Math.min(origin.current.y, e.nativeEvent.offsetY), ch)
    const x1 = at(Math.max(origin.current.x, e.nativeEvent.offsetX), cw)
    const y1 = at(Math.max(origin.current.y, e.nativeEvent.offsetY), ch)
    setDraft({ x: x0, y: y0, w: x1 - x0, h: y1 - y0 })
  }

  const release = () => {
    if (draft && draft.w > 2 && draft.h > 2) setBoxes([...boxes, draft])
    origin.current = null
    setDraft(null)
  }

  const apply = () => {
    const el = imgRef.current
    if (!el || !el.naturalWidth) {
      onDone()
      return
    }
    const k = el.naturalWidth / el.clientWidth
    const rects = boxes
      .filter((b) => b.w > 2 && b.h > 2)
      .map((b): MaskRect => ({
        x: Math.round(b.x * k), y: Math.round(b.y * k),
        w: Math.round(b.w * k), h: Math.round(b.h * k),
      }))
    if (rects.length === 0) {
      onDone()
      return
    }
    setBusy(true)
    void Service.MaskAndRecognize(rects)
      .then(onDone)
      .catch((e: unknown) => notice?.(errMsg(e)))
      .finally(() => setBusy(false))
  }

  const style = (b: Box) => ({ left: b.x, top: b.y, width: b.w, height: b.h })

  return (
    <>
      <div className="shot">
        <div className="shotwrap">
          <img ref={imgRef} src={src} alt={t('选区截图')} draggable={false} />
          {masking && (
            <div
              className="masklayer"
              onMouseDown={(e) => {
                if (e.button !== 0) return
                origin.current = { x: e.nativeEvent.offsetX, y: e.nativeEvent.offsetY }
                setDraft(null)
              }}
              onMouseMove={paint}
              onMouseUp={release}
            >
              {boxes.map((b, i) => (
                <div key={i} className="maskbox" style={style(b)} />
              ))}
              {draft && <div className="maskbox drawing" style={style(draft)} />}
            </div>
          )}
        </div>
      </div>
      {masking && (
        <div className="maskbar">
          <span className="hintline">{t('拖出方块遮挡要隐去的内容')}</span>
          <span className="spacer" />
          <button className="ghost tiny" disabled={busy} onClick={() => setBoxes([])}>
            {t('清除遮挡')}
          </button>
          <button className="tiny" disabled={busy} onClick={onDone}>
            {t('取消')}
          </button>
          <button className="primary tiny" disabled={busy || boxes.length === 0} onClick={apply}>
            {t('重新识别')}
          </button>
        </div>
      )}
    </>
  )
}

export function OCRList({ items, disabled }: { items: OCROutcome[]; disabled: boolean }) {
  // A successful OCR needs no report: the text below it is the answer, and a
  // card saying "苹果视觉 120ms" only pushes the translation down. A failed one
  // does, because it is the reason the source card is empty.
  if (!items.some((o) => !!o.error)) return null
  return (
    <div className="card">
      <h3>{t('识别服务')}</h3>
      <div className="body plain">
        {items.map((o) => (
          <div className="meta" key={o.provider}>
            <Dot bad={!!o.error} />
            <span>{o.label}</span>
            <time>{o.elapsedMs}ms</time>
            <div className="spacer" />
            {o.error && <span className="error">{o.error}</span>}
            <button className="tiny" disabled={disabled} onClick={() => void Service.RetryOCR(o.provider)}>
              {t('重试')}
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}

export function TranslateCard({ item, compact, notice, resetKey, lang, markdown = true, replace, replaced, partial, context }: {
  item: TranslateResult; compact?: boolean; notice?: Notice; resetKey?: string; lang?: string
  markdown?: boolean; replace?: (text: string) => void; replaced?: boolean; partial?: boolean
  context?: string
}) {
  const failed = !!item.error
  // A pending row is a reserved place in a progress update: the service has not
  // answered, so it gets a name and a hint but no time or actions yet.
  const pending = !!item.pending
  // Warning is an answer with nothing in it — the service replied, the text was
  // just unsupported or empty. EasyDict folds such a card to its header row and
  // drops 重试; a red failure card is for a fault the button can fix.
  const warning = !pending && !!item.warning
  const errorText = item.error ? failedText(item.error) : ''
  // A long answer can be folded away to compare the others. Every new query
  // starts open, because a card that stayed folded would hide its new result.
  const [open, setOpen] = useState(!warning)
  // elapsedMs is in the deps because every answer re-files the fold: a warning
  // that comes back from 展开-again re-folds, exactly as EasyDict collapses a
  // service whose second try also had nothing. A streamed sketch (partial text,
  // not a Pending placeholder) is not a new answer — its elapsedMs changes on
  // every chunk. A card folded by hand stays folded through the stream's finish
  // too, because EasyDict's isShowing is not raised when the last token lands;
  // only 重试 or a new query brings it back.
  // 展开-again keeps the card open: EasyDict sets manualShow when the user
  // expands an empty card, so the requery it starts cannot fold the row again
  // even if the service still has nothing. A new query clears the flag.
  const manualShow = useRef(false)
  const userFolded = useRef(false)
  useEffect(() => {
    manualShow.current = false
    userFolded.current = false
  }, [resetKey, item.provider])
  useEffect(() => {
    if (partial && !pending) return
    if (userFolded.current) return
    setOpen(!item.warning || manualShow.current)
  }, [resetKey, item.provider, item.warning, item.elapsedMs, partial, pending])
  // 重试 answers in place, so the card itself has to say it is working: EasyDict
  // turns the result into its loading animation until the retried service lands.
  // A new answer — this one or a whole new query — ends the wait.
  const [retrying, setRetrying] = useState(false)
  useEffect(() => setRetrying(false), [resetKey, item.provider, item.elapsedMs])
  const retry = () => {
    stopSpeech()
    userFolded.current = false
    // EasyDict's resetCellWithService raises isShowing before it reloads, so a
    // folded card opens while the retried service is still thinking.
    setOpen(true)
    setRetrying(true)
    Service.Retry(item.provider).catch((e) => { setRetrying(false); notice?.(errMsg(e)) })
  }
  // 停止生成 is EasyDict's stop.circle: the service keeps the words that already
  // reached the card and finishes there. The button goes away the moment it is
  // used; the final answer arrives like any other.
  const [stopped, setStopped] = useState(false)
  useEffect(() => setStopped(false), [resetKey, item.provider])
  const stop = () => {
    setStopped(true)
    void Service.StopTranslate(context ?? '', item.provider).catch((e) => notice?.(errMsg(e)))
  }
  // EasyDict's model menu: the chip names the model that answered, and picking
  // another one only changes what the next request asks for. The words already
  // on the card were written by the old model and stay until 重试 or a new query.
  const [model, setModel] = useState(item.model ?? '')
  const [modelMenu, setModelMenu] = useState<{ left: number; top: number } | null>(null)
  useEffect(() => {
    setModel(item.model ?? '')
    setModelMenu(null)
  }, [resetKey, item.provider, item.model])
  useEffect(() => {
    if (!modelMenu) return
    const close = () => setModelMenu(null)
    // Escape dismisses the menu, not the window behind it. EasyDict's menu is
    // the native one, which eats Esc itself; capture keeps the panel's own
    // Escape handler from reading the same key as "close the window".
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.isComposing) return
      e.stopPropagation()
      close()
    }
    window.addEventListener('click', close)
    window.addEventListener('keydown', onKey, true)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('keydown', onKey, true)
    }
  }, [modelMenu])
  // The menu is position:fixed because the card clips its own overflow; a folded
  // card is header height only, so an in-flow menu would be invisible.
  const openModelMenu = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation()
    if (modelMenu) {
      setModelMenu(null)
      return
    }
    const r = e.currentTarget.getBoundingClientRect()
    setModelMenu({ left: Math.max(6, Math.min(r.left, window.innerWidth - 168)), top: r.bottom + 2 })
  }
  const chooseModel = (m: string) => {
    setModelMenu(null)
    if (m === model) return
    setModel(m)
    void SettingsService.SelectModel(item.provider, m).catch((e) => notice?.(errMsg(e)))
  }
  // Opening a warning row asks again, as EasyDict's handleArrowToggle does for a
  // service whose card has nothing to show; otherwise the row is a dead end.
  const toggle = () => {
    const next = !open
    setOpen(next)
    manualShow.current = next
    userFolded.current = !next
    if (next && warning) retry()
  }
  // 启用 Markdown 渲染 has a per-card override: one answer's markup can be noise
  // while the next one is a list that reads better laid out.
  const [plain, setPlain] = useState(false)
  useEffect(() => setPlain(false), [resetKey, item.provider])
  // The speaker and 慢速 buttons each own one clip; whichever is sounding shows
  // the stop state, the way EZAudioButton swaps to pause.circle.
  const playing = usePlayingKey()
  const normalKey = speechKey(item.text ?? '', lang || 'auto', false)
  const slowKey = speechKey(item.text ?? '', lang || 'auto', true)
  return (
    <div className={`card tr ${failed && !warning ? 'failed' : ''}${pending ? ' pending' : ''}${open ? '' : ' folded'}`}>
      <h3 onClick={topBarClick(toggle)}>
        <ServiceBadge provider={item.provider} label={item.label} />
        <span className="svcname" title={item.label}>{item.label}</span>
        {model && (
          <span className="modelwrap">
            <button className="model-chip" title={model} onClick={openModelMenu}>
              {model}
            </button>
            {modelMenu && (
              <div className="modelmenu" role="menu" style={{ left: modelMenu.left, top: modelMenu.top }}>
                {(item.models && item.models.length ? item.models : [model]).map((m) => (
                  <button
                    key={m}
                    type="button"
                    role="menuitemradio"
                    aria-checked={m === model}
                    className="modelitem"
                    onClick={(e) => { e.stopPropagation(); chooseModel(m) }}
                  >
                    <span className="modelcheck">{m === model ? '✓' : ''}</span>
                    {m}
                  </button>
                ))}
              </div>
            )}
          </span>
        )}
        {(pending || retrying) && <LoadingDots label={t('翻译中…')} />}
        {failed && !warning && !retrying && <ErrorGlyph title={errorText || undefined} />}
        {warning && !retrying && <WarningGlyph title={errorText || undefined} />}
        <span className="spacer" />
        {item.streaming && !stopped && (
          <button className="card-act" title={t('停止')} aria-label={t('停止')}
            onClick={stop}>
            <ActIcon id="halt" />
          </button>
        )}
        {failed && !pending && !warning && !retrying && !item.streaming && (
          <button className="card-act" title={t('重试')} aria-label={t('重试')}
            onClick={retry}>
            <ActIcon id="retry" />
          </button>
        )}
        <button
          className="card-toggle"
          title={open ? t('隐藏') : t('显示')}
          aria-label={open ? t('隐藏') : t('显示')}
          aria-expanded={open}
          onClick={toggle}
        >
          <Chevron open={open} />
        </button>
      </h3>
      {open && !pending && !retrying && (warning ? (
        <div className="body muted" title={errorText || undefined}>{errorText || t('无结果')}</div>
      ) : failed || !item.text || !item.markdown || !markdown || plain ? (
        <div className={`body ${failed ? 'error' : ''} ${compact ? 'clamp' : ''}`}>
          {failed ? errorText : item.text || t('无结果')}
        </div>
      ) : (
        // A model's answer arrives with headings, lists and fences in it. Quoting it
        // would show the markup instead of the translation, so the card lays it out
        // and keeps the text selectable.
        <div
          className={`body prose ${compact ? 'clamp' : ''}`}
          onClick={followLinks}
          dangerouslySetInnerHTML={{ __html: renderMarkdown(item.text) }}
        />
      ))}
      {/* 参见 👉 is EasyDict's promptURL: a refused key or a spent quota names
          the page where that service fixes it. */}
      {open && !pending && !retrying && failed && !warning && item.promptUrl && (
        <div className="card-prompt">
          <span>{t('参见 👉')}</span>
          <a className="srclink" href={item.promptUrl} title={item.promptUrl}
            onClick={(e) => { e.preventDefault(); openURL(item.promptUrl!) }}>
            {item.promptTitle || item.promptUrl}
          </a>
        </div>
      )}
      {/* Actions sit under the answer: the header stays a name and a time, so
          every service's header lines up whatever it can do. 慢速 keeps a word
          because a second speaker glyph would not say what it does. */}
      {open && !pending && !retrying && !failed && !!item.text && (
        <div className="card-actions">
          {/* EZWordResultView's bottom row runs audio, copy, link, replace; the
              markdown switch closes it for a streamed answer. */}
          <button className="act" title={playing === normalKey ? t('停止播放音频') : t('播放音频')}
            aria-label={playing === normalKey ? t('停止播放音频') : t('播放音频')}
            onClick={() => void toggleSpeech(item.text, lang || 'auto', false, notice)}>
            <ActIcon id={playing === normalKey ? 'stop' : 'speak'} />
          </button>
          <CopyButton text={item.text} notice={notice} icon />
          <button
            className="tiny"
            title={t('放慢速度再读一遍')}
            onClick={() => void toggleSpeech(item.text, lang || 'auto', true, notice)}
          >
            {playing === slowKey ? t('停止') : t('慢速')}
          </button>
          {item.site && (
            <button className="act" title={t('在网页中打开')} aria-label={t('在网页中打开')}
              onClick={() => openURL(item.site!)}>
              <ActIcon id="link" />
            </button>
          )}
          {/* 替换 puts this card's answer back over the selection the query came
              from. EasyDict stops offering it once one answer has been written
              back, because there is no longer a selection to replace. */}
          {replace && (
            <button className="act" title={t('使用译文替换')} aria-label={t('使用译文替换')} disabled={replaced}
              onClick={() => replace(item.text ?? '')}>
              <ActIcon id="replace" />
            </button>
          )}
          {item.markdown && (
            <button
              className="act"
              title={plain ? t('切换到 Markdown 渲染') : t('切换到纯文本')}
              aria-label={plain ? t('切换到 Markdown 渲染') : t('切换到纯文本')}
              onClick={() => setPlain(!plain)}
            >
              <span className="md-toggle">M</span>
            </button>
          )}
        </div>
      )}
    </div>
  )
}

export function DictCard({ item, notice, resetKey, lang, onEdit, anki, translation, replace, replaced }: {
  item: DictResult; notice?: Notice; resetKey?: string; lang?: string; onEdit?: (text: string) => void
  anki?: boolean; translation?: string; replace?: (text: string) => void; replaced?: boolean
}) {
  const entries = item.entries ?? []
  // A dictionary with no entry is EasyDict's noResult warning — the service
  // answered, the word simply is not in it — so the card folds to a ⊘ row and
  // 重试 disappears. A query the dictionary never answers (a whole sentence)
  // also draws ⊘ but stays open with 重试, and a real fault is the red icon.
  const unsupported = !!item.unsupported
  const warning = !!item.warning || (!item.error && entries.length === 0)
  const errorText = item.error ? failedText(item.error) : ''
  const [open, setOpen] = useState(!warning)
  // Like the translation card: expanding an empty dictionary asks it again, and
  // manualShow keeps the row open when the second answer is empty as well.
  const manualShow = useRef(false)
  useEffect(() => { manualShow.current = false }, [resetKey, item.provider])
  useEffect(() => setOpen(!warning || manualShow.current),
    [resetKey, item.provider, warning, item.elapsedMs])
  // 重试 runs in place: EasyDict shows the service's loading animation in the
  // card until the retried dictionary answers, then puts the entry back.
  const [retrying, setRetrying] = useState(false)
  useEffect(() => setRetrying(false), [resetKey, item.provider, item.elapsedMs])
  const retry = () => {
    stopSpeech()
    setOpen(true)
    setRetrying(true)
    Service.Retry(item.provider).catch((e) => { setRetrying(false); notice?.(errMsg(e)) })
  }
  const toggle = () => {
    const next = !open
    setOpen(next)
    manualShow.current = next
    if (next && warning) retry()
  }
  // 添加到 Anki answers through a toast at the pointer, so the button itself only
  // has to stop a second click while the note is being written. A failure puts it
  // back; a success leaves it spent until the next query, as EasyDict does.
  const [ankiBusy, setAnkiBusy] = useState(false)
  useEffect(() => setAnkiBusy(false), [resetKey, item.provider])
  const copyText = entries.map(plainEntry).filter(Boolean).join('\n\n')
  const headwords = entries.map((e) => e.word).filter(Boolean).join('，')
  const note = ankiNote(entries, translation ?? '')
  const playing = usePlayingKey()
  const wordsKey = speechKey(headwords, lang || 'auto', false)
  return (
    <div className={`card dict ${item.error && !warning && !unsupported ? 'failed' : ''}${open ? '' : ' folded'}`}>
      <h3 onClick={topBarClick(toggle)}>
        <ServiceBadge provider={item.provider} label={item.label} />
        <span className="svcname" title={item.label}>{item.label}</span>
        {retrying && <LoadingDots label={t('翻译中…')} />}
        {(warning || unsupported) && !retrying && <WarningGlyph title={errorText || undefined} />}
        {item.error && !retrying && !warning && !unsupported && <ErrorGlyph title={errorText || undefined} />}
        <span className="spacer" />
        {!!item.error && !warning && !retrying && (
          <button className="card-act" title={t('重试')} aria-label={t('重试')}
            onClick={retry}>
            <ActIcon id="retry" />
          </button>
        )}
        <button
          className="card-toggle"
          title={open ? t('隐藏') : t('显示')}
          aria-label={open ? t('隐藏') : t('显示')}
          aria-expanded={open}
          onClick={toggle}
        >
          <Chevron open={open} />
        </button>
      </h3>
      {open && !retrying && (
        <div className="body plain">
          {item.error ? (
            // A dictionary that has no entry for a sentence is not a failure the
            // user has to act on, so it reads as a quiet note rather than a fault.
            <div className="miss">{errorText || item.error}</div>
          ) : (item.entries ?? []).length === 0 ? (
            // A dictionary can answer 200 with an empty entry (a phrase lookup on
            // a word list); an empty card body reads as a broken panel.
            <div className="miss">{t('未查询到结果')}</div>
          ) : (
            (item.entries ?? []).map((entry, i) => <EntryView key={`${entry.word}-${i}`} entry={entry} notice={notice} onEdit={onEdit} />)
          )}
        </div>
      )}
      {open && !item.error && !warning && !retrying && (
        <div className="card-actions">
          {/* The headword is in the source language, so it is read with that
            voice; 'auto' fell back to the system default and sounded English
            syllables over a Chinese word. */}
          {headwords && (
            <button className="act" title={playing === wordsKey ? t('停止播放音频') : t('播放音频')}
              aria-label={playing === wordsKey ? t('停止播放音频') : t('播放音频')}
              onClick={() => void toggleSpeech(headwords, lang || 'auto', false, notice)}>
              <ActIcon id={playing === wordsKey ? 'stop' : 'speak'} />
            </button>
          )}
          {copyText && <CopyButton text={copyText} notice={notice} icon />}
          {/* A local dictionary answers on its own scheme: the Apple card's
              link is 在苹果词典中打开, as EZWordResultView labels it, and the
              web dictionaries keep 在网页中打开. */}
          {(item.openUrl || item.site) && (
            <button className="act"
              title={item.openUrl ? t('在苹果词典中打开') : t('在网页中打开')}
              aria-label={item.openUrl ? t('在苹果词典中打开') : t('在网页中打开')}
              onClick={() => openQuickLink(item.openUrl || item.site!)}>
              <ActIcon id="link" />
            </button>
          )}
          {/* 替换 writes this entry back over the selection, as EasyDict's
              replaceTextButton does; one answer per selection. */}
          {replace && copyText && (
            <button className="act" title={t('使用译文替换')} aria-label={t('使用译文替换')} disabled={replaced}
              onClick={() => replace(copyText)}>
              <ActIcon id="replace" />
            </button>
          )}
          {/* 添加到 Anki is the review step EasyDict puts on a word card: the
              note carries every value the field templates can name, so a note
              type mapped for EasyDict fills the same way. */}
          {anki && headwords && (
            <button className="act" title={t('添加到 Anki')} aria-label={t('添加到 Anki')}
              disabled={ankiBusy}
              onClick={() => {
                setAnkiBusy(true)
                void Service.AddToAnki(note)
                  .then(() => Service.Toast(t('已添加到 Anki')))
                  .catch((e) => { setAnkiBusy(false); Service.Toast(errMsg(e)) })
              }}>
              <ActIcon id="anki" />
            </button>
          )}
        </div>
      )}
    </div>
  )
}

export function EntryView({ entry, notice, onEdit }: {
  entry: Entry; notice?: Notice; onEdit?: (text: string) => void
}) {
  // A sentence dictionary repeats the whole query as every entry's headword.
  // Printing it in bold three times pushes the translations it introduces down
  // the card, so a long headword with nothing attached is skipped.
  const word = entry.word && (entry.word.trim().length <= 24 || entry.phonetics?.length) ? entry.word : ''
  // EasyDict gives every phonetic row its own EZAudioButton: one click plays
  // that accent, a second stops it. The URL is the dictionary's own clip, not
  // the TTS service the bottom speaker uses.
  const [playingPhone, setPlayingPhone] = useState<number | null>(null)
  const phoneAudio = useRef<HTMLAudioElement | null>(null)
  useEffect(() => () => { phoneAudio.current?.pause() }, [])
  const playPhone = (index: number, url?: string) => {
    if (!url) return
    if (playingPhone === index && phoneAudio.current) {
      phoneAudio.current.pause()
      phoneAudio.current.currentTime = 0
      setPlayingPhone(null)
      return
    }
    phoneAudio.current?.pause()
    const el = new Audio(url)
    phoneAudio.current = el
    setPlayingPhone(index)
    el.onended = () => setPlayingPhone((p) => (p === index ? null : p))
    void el.play().catch(() => setPlayingPhone((p) => (p === index ? null : p)))
  }
  // A service that returns prose (an Apple Dictionary paragraph, an MDX article)
  // has no structure to lay out, so its preformatted text is the rendering.
  if (!entry.definitions?.length && !entry.phonetics?.length && entry.text) {
    // Apple hands back one line with ▸ between senses. It is plain text, so it
    // is rendered as text rather than HTML, and the separators become breaks
    // the panel can lay out; an MDX article with real markup keeps the HTML path.
    const plain = !/[<>]/.test(entry.text)
    let body = entry.text.replace(/\s*▸\s*/g, '\n▸ ').trim()
    // The definition opens with the headword ("apple | BrE …") although the
    // card already prints it as the heading.
    const bar = body.indexOf('|')
    if (entry.word && bar > 0 && body.slice(0, bar).trim().toLowerCase() === entry.word.trim().toLowerCase()) {
      body = body.slice(bar + 1).trim()
    }
    return (
      <div className="entry">
        {word && <div className="word">{word}</div>}
        {plain ? (
          <div className="prose plaintext">{body}</div>
        ) : (
          <div className="prose" onClick={followLinks} dangerouslySetInnerHTML={{ __html: sanitize(entry.text) }} />
        )}
      </div>
    )
  }
  const spoken = phones(entry)
  // EasyDict reads the American row first; the payload lists UK first.
  const phonesShown = [...spoken].sort((a, b) => phoneRank(a.lang) - phoneRank(b.lang))
  const gloss = primaryGloss(entry)
  return (
    <div className="entry">
      {word && <div className="word">{word}</div>}
      {gloss && <div className="gloss">{gloss}</div>}
      {phonesShown.length > 0 && (
        <div className="phones">
          {phonesShown.map((p, i) => {
            const inner = (
              <>
                {p.lang && <span className="ph-lang">{phoneLabel(p.lang)}</span>}
                <span className="ph-sym">{phoneText(p)}</span>
                {p.audio && <Speaker />}
              </>
            )
            if (!p.audio) {
              return <span className="ph" key={i}>{inner}</span>
            }
            const label = `${playingPhone === i ? t('停止播放音频') : t('播放音频')} ${phoneLabel(p.lang)}`.trim()
            return (
              <button type="button" className="ph playable" key={i}
                title={label} aria-label={label} aria-pressed={playingPhone === i}
                onClick={() => playPhone(i, p.audio)}>
                {inner}
              </button>
            )
          })}
        </div>
      )}
      {entry.exams?.length ? (
        <div className="exams">{entry.exams.map((x) => <span key={x}>{x}</span>)}</div>
      ) : null}
      {entry.definitions?.map((d, i) => (
        <div className="sense" key={i}>
          {d.pos && <span className="pos">{d.pos}</span>}
          <span className="def">{(d.meanings ?? []).join('；')}</span>
          {d.examples?.map((e, j) => (
            <div className="eg" key={j}>{e}</div>
          ))}
        </div>
      ))}
      {entry.exchange?.length ? (
        <div className="exchange">
          {entry.exchange.filter(Boolean).map((x, i) => {
            const [name, value] = splitExchange(x)
            return (
              <div className="wf" key={i}>
                {name && <span className="wf-name">{name}</span>}
                {value && (
                  <button className="wf-val" onClick={() => onEdit?.(value)}>{value}</button>
                )}
              </div>
            )
          })}
        </div>
      ) : null}
      {/* The sections 有道 returns below the senses. Each word is a lookup, the
          same as the related-word rows, so tapping one asks the panel about it. */}
      {entry.synonyms?.length ? (
        <div className="dictsec">
          <div className="dictsec-title">{t('同义词')}</div>
          {entry.synonyms.map((s, i) => (
            <div className="syno" key={i}>
              {s.pos && <span className="pos">{s.pos}</span>}
              <span className="syno-words">
                {(s.words ?? []).map((w, j) => (
                  <button className="syno-w" key={j} onClick={() => onEdit?.(w)}>{w}</button>
                ))}
              </span>
              {s.tran && <span className="dim">{s.tran}</span>}
            </div>
          ))}
        </div>
      ) : null}
      {entry.phrases?.length ? (
        <div className="dictsec">
          <div className="dictsec-title">{t('短语')}</div>
          {entry.phrases.map((p, i) => (
            <div className="phr" key={i}>
              <button className="phr-hw" onClick={() => onEdit?.(p.headword)}>{p.headword}</button>
              {p.translation && <span className="dim">{p.translation}</span>}
            </div>
          ))}
        </div>
      ) : null}
      {entry.etymology ? (
        <div className="dictsec">
          <div className="dictsec-title">{t('词源：')}</div>
          <div className="etym">{entry.etymology}</div>
        </div>
      ) : null}
      {entry.web?.map((w, i) => <RelatedWord key={`w${i}`} item={w} onEdit={onEdit} />)}
      {entry.related?.map((w, i) => <RelatedWord key={`r${i}`} item={w} onEdit={onEdit} />)}
      {entry.examples?.length ? (
        <div className="sentences">
          {entry.examples.map((e, i) => (
            <div key={i}>
              <div>{e.source}</div>
              {e.translation && <div className="dim">{e.translation}</div>}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

// RelatedWord is one of the site's compound entries ("Hello Kitty"). The
// headword is the lookup — clicking it asks the same panel about that word.
function RelatedWord({ item, onEdit }: {
  item: { value: string; conts?: string[] | null }; onEdit?: (text: string) => void
}) {
  const text = (item.conts ?? []).join('；')
  return (
    <button className="rel" onClick={() => onEdit?.(item.value)}>
      <b>{item.value}</b>
      {text && <span>{text}</span>}
    </button>
  )
}

// primaryGloss is the one-line meaning under the headword. The first sense
// carries qualifiers in parentheses ("喂，你好（用于问候…）") that belong to the
// full definition, not to a glance-sized line.
function primaryGloss(entry: Entry): string {
  const first = entry.definitions?.[0]?.meanings?.[0] ?? ''
  return first.replace(/（[^）]*）|\([^)]*\)/g, '').replace(/[，,；;：:。\s]+$/u, '').trim()
}

// The payload labels the phonetic rows UK/US; the card reads them in Chinese
// and puts 美 first, the way EasyDict does.
function phoneLabel(lang: string): string {
  if (lang === 'US') return uiLocale() === 'en' ? 'US' : '美'
  if (lang === 'UK') return uiLocale() === 'en' ? 'UK' : '英'
  return lang
}

function phoneRank(lang: string): number {
  if (lang === 'US' || lang === '美') return 0
  if (lang === 'UK' || lang === '英') return 1
  return 2
}

// The dictionary's own pages write IPA between slashes, which is also how
// EasyDict shows it; the payload's brackets stay for the copyable text.
function phoneText(p: { text: string; lang: string }): string {
  if (p.lang !== 'US' && p.lang !== 'UK') return p.text
  return p.text.replace(/^\[(.*)\]$/u, '/$1/')
}

// splitExchange turns "复数：hellos" into its label and its linked value.
function splitExchange(text: string): [string, string] {
  const at = text.search(/[：:]/)
  if (at < 0) return ['', text]
  return [text.slice(0, at + 1), text.slice(at + 1).trim()]
}

// Services hand back HTML from vendor dictionaries. Providers are trusted, but a
// pasted string can reach an article, so tags that execute or phone home are
// dropped rather than rendered.
function sanitize(html: string): string {
  return html
    .replace(/<(script|style|iframe|object|embed)[^>]*>[\s\S]*?<\/\1>/gi, '')
    .replace(/<\/?(script|style|iframe|object|embed|link|meta)[^>]*>/gi, '')
    .replace(/\son\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, '')
    .replace(/(href|src)\s*=\s*("|')?\s*javascript:[^"'>\s]*/gi, '$1="#"')
}

/** SkippedHint says which services a query needed but could not run. Go words
 * the names for the interface language, so only the sentence around them is the
 * bundle's; an empty list means nothing was waiting on a setting. */
export function SkippedHint({ names }: { names: string[] }) {
  if (names.length === 0) return null
  return <div className="noteline">{t('尚未配置：{names}', { names: names.join('、') })}</div>
}

// compact stacks the two columns: the translation window is narrower than a
// side-by-side layout needs, so 原文 and 译文 follow each other in one scroller.
// showSource is off when the window already shows the same text in its input
// box: a second copy of the source above the answer would repeat it.
export function ResultColumns({ res, busy, notice, compact, langNames, showSource = true, markdown = true, anki = false, links }: {
  res: Response; busy: string; notice?: Notice; compact?: boolean; langNames?: Record<string, string>
  showSource?: boolean; markdown?: boolean; anki?: boolean; links?: QuickLink[]
}) {
  const edit = (text: string) => void Service.QueryText(text, res.context)
  // 应用内查询: a right-click on either column offers the word under the pointer,
  // so a term in a translation does not have to be retyped to look it up. The
  // same menu carries 朗读 and the titlebar's quick links, acting on that word.
  const menu = useWordMenu(notice, res.sourceLang || res.detected, links)
  // null means "this answer's state is unknown": a new query is a new record, and
  // the star from the previous one must not carry over.
  const [saved, setSaved] = useState<boolean | null>(null)
  useEffect(() => setSaved(null), [res.id])
  // 替换 is offered only when the query came from a live, editable selection. Once
  // an answer is written back the selection is gone, so every card's button goes
  // quiet until the next query, as EasyDict's disableReplaceTextButton does.
  const [replaced, setReplaced] = useState(false)
  useEffect(() => setReplaced(false), [res.id])
  const replace = res.replaceable
    ? (text: string) => {
      if (!text.trim()) return
      void Service.ReplaceWithText(text)
        .then(() => setReplaced(true))
        .catch((e) => notice?.(errMsg(e)))
    }
    : undefined

  // A new answer starts at its first line. The sketch that grows into the
  // finished answer can leave the column scrolled past the source header, so
  // both the new query and the moment translations land return to the top.
  const scroller = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (compact) scroller.current?.scrollTo({ top: 0 })
  }, [res.id, res.partial, compact])

  const source = (
    <>
      {showSource && <SourceCard res={res} onEdit={edit} notice={notice} langNames={langNames} />}
      <OCRList items={res.ocr ?? []} disabled={!!busy} />
    </>
  )

  const answer = (
    <>
      {res.error && (
        <div className="card">
          <div className="body error">{res.error}</div>
          <SkippedHint names={res.skipped ?? []} />
        </div>
      )}
      {/* EasyDict puts dictionaries above the translate cards: for a single
          word the entry is the answer, and the machine translations repeat one
          line of it. A dictionary with nothing to show keeps its row, folded
          and marked ⊘, the way EasyDict hides a noResult warning instead of
          dropping the service from the answer. */}
      {(res.dict ?? []).map((d) => (
        <DictCard key={d.provider} item={d} notice={notice} resetKey={res.id} lang={res.detected} onEdit={edit}
          anki={anki} translation={firstResult(res)} replace={replace} replaced={replaced} />
      ))}
      {(res.translate ?? []).map((it) => (
        <TranslateCard key={it.provider} item={it} notice={notice} resetKey={res.id} lang={res.targetLang}
          markdown={markdown} replace={replace} replaced={replaced} partial={res.partial} context={res.context} />
      ))}
      {res.sourceText && (
        <div className="row-end foot">
          <CopyButton text={copyAll(res)} label={t('复制全文')} notice={notice} disabled={!!busy} />
          <button
            disabled={!!busy}
            title={t('收藏这次查询，历史里可以只看收藏')}
            onClick={() => void Service.StarHistory(res.id)
              .then((on) => {
                setSaved(on)
                notice?.(on ? t('已加入收藏') : t('已取消收藏'))
              })
              .catch((e) => notice?.(errMsg(e)))}
          >
            {saved ? t('已收藏') : t('收藏')}
          </button>
          <button disabled={!!busy} onClick={() => { stopSpeech(); void Service.ReTranslate() }}>{t('全部重新查询')}</button>
        </div>
      )}
    </>
  )

  if (compact) {
    return (
      <div className="content compact" onContextMenu={menu.onContextMenu}>
        <div className="col" ref={scroller}>
          {source}
          {answer}
        </div>
        {menu.wordMenu}
      </div>
    )
  }

  return (
    <div className="content" onContextMenu={menu.onContextMenu}>
      <div className="col">{source}</div>
      <div className="col">{answer}</div>
      {menu.wordMenu}
    </div>
  )
}
