// Result rendering. Every window that shows an answer — the main panel, the
// floating panel, the mini window — composes these, so one change to how a
// dictionary entry looks reaches all of them.
import { useEffect, useRef, useState, type MouseEvent } from 'react'
import {
  Service, errMsg, langName, openURL, readAloud,
  type DictResult, type Entry, type OCROutcome, type Response, type TranslateResult,
} from './api'
import type { MaskRect } from '../bindings/transdict/internal/capture/models.js'
import { renderMarkdown } from './markdown'
import { t } from './i18n'
import { copyAll, phones, plainEntry } from './copy'

/** Notice tells the window why something did not happen. A button that fails
 * quietly reads as a broken app, so every action that can come back empty —
 * read-aloud with no voice, 收藏 on a query that was never recorded — reports here. */
type Notice = (msg: string) => void

// CopyButton answers in place: the label changes for a moment instead of a
// note line appearing above the cards, which would push the whole answer down
// every time something is copied. A refused write still gets the note.
function CopyButton({ text, label, notice, disabled }: {
  text: string; label?: string; notice?: Notice; disabled?: boolean
}) {
  const [done, setDone] = useState(false)
  useEffect(() => {
    if (!done) return
    const timer = setTimeout(() => setDone(false), 1200)
    return () => clearTimeout(timer)
  }, [done])
  return (
    <button
      className={`tiny${done ? ' copied' : ''}`}
      title={done ? t('已复制') : undefined}
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
      {label ?? t('复制')}
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

export function SourceCard({ res, onEdit, notice, langNames = {} }: {
  res: Response; onEdit?: (text: string) => void; notice?: Notice; langNames?: Record<string, string>
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const [masking, setMasking] = useState(false)
  const text = res.sourceText ?? ''
  const ocr = (res.ocr ?? []).filter((o) => !o.error)
  const lang = langName(res.detected, langNames)

  // A new answer is a new crop; the editor from the previous one has nothing to
  // hide any more.
  useEffect(() => setMasking(false), [res.id])

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
            <button className="tiny" onClick={() => void readAloud(text, res.detected || 'auto', false, notice)}>{t('朗读')}</button>
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
            <div className={`body ${text ? '' : 'muted'}`}>{text || t('未识别到文字')}</div>
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

export function TranslateCard({ item, compact, notice, resetKey, lang }: {
  item: TranslateResult; compact?: boolean; notice?: Notice; resetKey?: string; lang?: string
}) {
  const failed = !!item.error
  // A pending row is a reserved place in a progress update: the service has not
  // answered, so it gets a name and a hint but no time or actions yet.
  const pending = !!item.pending
  // A long answer can be folded away to compare the others. Every new query
  // starts open, because a card that stayed folded would hide its new result.
  const [open, setOpen] = useState(true)
  useEffect(() => setOpen(true), [resetKey, item.provider])
  return (
    <div className={`card tr ${failed ? 'failed' : ''}${pending ? ' pending' : ''}${open ? '' : ' folded'}`}>
      <h3>
        {item.label}
        <span className="spacer" />
        {!pending && <span className="byline">{item.elapsedMs}ms</span>}
        <button
          className="card-toggle"
          title={open ? t('折叠') : t('展开')}
          aria-label={open ? t('折叠') : t('展开')}
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          <Chevron open={open} />
        </button>
      </h3>
      {open && (pending ? (
        <div className="body muted">{t('翻译中…')}</div>
      ) : failed || !item.markdown ? (
        <div className={`body ${failed ? 'error' : ''} ${compact ? 'clamp' : ''}`}>{failed ? item.error : item.text}</div>
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
      {/* Actions sit under the answer: the header stays a name and a time, so
          every service's header lines up whatever it can do. */}
      {open && !pending && (
        <div className="card-actions">
          {!failed && (
            <>
              <CopyButton text={item.text} notice={notice} />
              {/* item.text is the translation, so it is read with the target
                  voice; item.detected is the language the source was in. */}
              <button className="tiny" onClick={() => void readAloud(item.text, lang || 'auto', false, notice)}>{t('朗读')}</button>
              <button
                className="tiny"
                title={t('放慢速度再读一遍')}
                onClick={() => void readAloud(item.text, lang || 'auto', true, notice)}
              >
                {t('慢速')}
              </button>
              {item.site && (
                <button className="tiny" onClick={() => openURL(item.site!)}>{t('网页版')}</button>
              )}
            </>
          )}
          <button className="tiny" onClick={() => void Service.Retry(item.provider)}>{t('重试')}</button>
        </div>
      )}
    </div>
  )
}

export function DictCard({ item, notice, resetKey, lang }: {
  item: DictResult; notice?: Notice; resetKey?: string; lang?: string
}) {
  const [open, setOpen] = useState(true)
  useEffect(() => setOpen(true), [resetKey, item.provider])
  const entries = item.entries ?? []
  const copyText = entries.map(plainEntry).filter(Boolean).join('\n\n')
  const headwords = entries.map((e) => e.word).filter(Boolean).join('，')
  return (
    <div className={`card dict ${item.error ? 'failed' : ''}${open ? '' : ' folded'}`}>
      <h3>
        {item.label}
        <span className="spacer" />
        <span className="byline">{item.elapsedMs}ms</span>
        <button
          className="card-toggle"
          title={open ? t('折叠') : t('展开')}
          aria-label={open ? t('折叠') : t('展开')}
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          <Chevron open={open} />
        </button>
      </h3>
      {open && (
        <div className="body plain">
          {item.error ? (
            // A dictionary that has no entry for a sentence is not a failure the
            // user has to act on, so it reads as a quiet note rather than a fault.
            <div className="miss">{item.error}</div>
          ) : (item.entries ?? []).length === 0 ? (
            // A dictionary can answer 200 with an empty entry (a phrase lookup on
            // a word list); an empty card body reads as a broken panel.
            <div className="miss">{t('没有查到词条')}</div>
          ) : (
            (item.entries ?? []).map((entry, i) => <EntryView key={`${entry.word}-${i}`} entry={entry} notice={notice} />)
          )}
        </div>
      )}
      {open && (
        <div className="card-actions">
          {!item.error && copyText && (
            <CopyButton text={copyText} notice={notice} />
          )}
          {/* The headword is in the source language, so it is read with that
            voice; 'auto' fell back to the system default and sounded English
            syllables over a Chinese word. */}
          {!item.error && headwords && (
            <button className="tiny" onClick={() => void readAloud(headwords, lang || 'auto', false, notice)}>{t('朗读')}</button>
          )}
          {!item.error && item.site && (
            <button className="tiny" onClick={() => openURL(item.site!)}>{t('网页版')}</button>
          )}
          <button className="tiny" onClick={() => void Service.Retry(item.provider)}>{t('重试')}</button>
        </div>
      )}
    </div>
  )
}

/** DictMisses is the one-line answer for dictionaries that had nothing: the
 * services keep their names and the reason is in the tooltip, without three
 * empty cards standing between the translation and 复制全文. */
function DictMisses({ items }: { items: DictResult[] }) {
  if (items.length === 0) return null
  const detail = items.map((d) => `${d.label}：${d.error || t('没有查到词条')}`).join('\n')
  return (
    <div className="dict-miss" title={detail}>
      {t('词典未收录：{names}', { names: items.map((d) => d.label).join('、') })}
    </div>
  )
}

export function EntryView({ entry, notice }: { entry: Entry; notice?: Notice }) {
  // A sentence dictionary repeats the whole query as every entry's headword.
  // Printing it in bold three times pushes the translations it introduces down
  // the card, so a long headword with nothing attached is skipped.
  const word = entry.word && (entry.word.trim().length <= 24 || entry.phonetics?.length) ? entry.word : ''
  const speakable = (url?: string) => {
    if (!url) return
    const el = new Audio(url)
    void el.play().catch(() => undefined)
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
  return (
    <div className="entry">
      {(word || spoken.length > 0) && (
        <div className="word">
          {word}
          {spoken.map((p, i) => (
            <span
              className={`ph${p.audio ? ' playable' : ''}`}
              key={i}
              title={p.audio ? t('朗读') : undefined}
              onClick={() => speakable(p.audio)}
            >
              {p.lang ? `${p.lang} ` : ''}{p.text}
              {p.audio && <Speaker />}
            </span>
          ))}
        </div>
      )}
      {entry.exchange?.length ? <div className="exchange">{entry.exchange.filter(Boolean).join('　')}</div> : null}
      {entry.definitions?.map((d, i) => (
        <div className="sense" key={i}>
          {d.pos && <span className="pos">{d.pos}</span>}
          <ol>
            {(d.meanings ?? []).map((m, j) => (
              <li key={j}>{m}</li>
            ))}
          </ol>
          {d.examples?.map((e, j) => (
            <div className="eg" key={j}>{e}</div>
          ))}
        </div>
      ))}
      {entry.web?.map((w, i) => (
        <div className="sense" key={`w${i}`}>
          <span className="pos">{t('网义')}</span>
          <ol>
            {(w.conts ?? []).map((c, j) => (
              <li key={j}>
                {w.value}：{c}
              </li>
            ))}
          </ol>
        </div>
      ))}
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
export function ResultColumns({ res, busy, notice, compact, langNames }: {
  res: Response; busy: string; notice?: Notice; compact?: boolean; langNames?: Record<string, string>
}) {
  const edit = (text: string) => void Service.QueryText(text, res.context)
  // null means "this answer's state is unknown": a new query is a new record, and
  // the star from the previous one must not carry over.
  const [saved, setSaved] = useState<boolean | null>(null)
  useEffect(() => setSaved(null), [res.id])

  // A new answer starts at its first line. The sketch that grows into the
  // finished answer can leave the column scrolled past the source header, so
  // both the new query and the moment translations land return to the top.
  const scroller = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (compact) scroller.current?.scrollTo({ top: 0 })
  }, [res.id, res.partial, compact])

  const source = (
    <>
      <SourceCard res={res} onEdit={edit} notice={notice} langNames={langNames} />
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
      {(res.translate ?? []).map((it) => (
        <TranslateCard key={it.provider} item={it} notice={notice} resetKey={res.id} lang={res.targetLang} />
      ))}
      {(res.dict ?? []).filter((d) => (d.entries ?? []).length > 0).map((d) => (
        <DictCard key={d.provider} item={d} notice={notice} resetKey={res.id} lang={res.detected} />
      ))}
      {/* Dictionaries with nothing for this text are one quiet line, not a
          stack of cards: a sentence makes every word list answer 未收录, and
          three cards of that buried the translations they were under. */}
      <DictMisses items={(res.dict ?? []).filter((d) => (d.entries ?? []).length === 0)} />
      {res.sourceText && (
        <div className="row-end">
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
          <button disabled={!!busy} onClick={() => void Service.ReTranslate()}>{t('全部重新查询')}</button>
        </div>
      )}
    </>
  )

  if (compact) {
    return (
      <div className="content compact">
        <div className="col" ref={scroller}>
          {source}
          {answer}
        </div>
      </div>
    )
  }

  return (
    <div className="content">
      <div className="col">{source}</div>
      <div className="col">{answer}</div>
    </div>
  )
}
