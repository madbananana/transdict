// OCR 窗口 is EasyDict's OCR preview: the picture that was read, where every
// line was found, and the text merged from those lines. 截图 OCR and 剪贴板 OCR
// end here instead of in a translation panel; 显示 OCR 窗口 reopens whatever was
// read last, including nothing before the first recognition.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Service, errMsg, isMac, onCommand } from './api'
import type { OCRPreview } from '../bindings/transdict/internal/gui/models.js'
import type { Block } from '../bindings/transdict/internal/ocr/models.js'
import { t } from './i18n'
import WindowControls from './WindowControls'

// keyOf names one line across every provider's result: two services can find
// the same sentence, and the box and the row still have to stay in step.
function keyOf(provider: string, index: number): string {
  return `${provider}\u0000${index}`
}

export default function OcrWindow() {
  const [preview, setPreview] = useState<OCRPreview | null>(null)
  const [picked, setPicked] = useState('')
  const [note, setNote] = useState('')
  const [pinned, setPinned] = useState(false)
  const [natural, setNatural] = useState({ w: 0, h: 0 })
  const list = useRef<HTMLDivElement | null>(null)

  // EasyDict's OCRWindow is borderless and its page paints a 12pt rounded sheet,
  // so the window behind the sheet has to be transparent. Windows keeps its own
  // caption buttons and an opaque frame.
  useEffect(() => {
    if (!isMac) return
    document.documentElement.classList.add('ocr-window')
    return () => document.documentElement.classList.remove('ocr-window')
  }, [])

  useEffect(() => {
    if (!isMac) return
    void Service.OCRWindowPinned()
      .then((v) => setPinned(!!v))
      .catch(() => {})
  }, [])

  const load = useCallback(() => {
    void Service.OCRPreview()
      .then((p) => setPreview(p))
      .catch((e) => setNote(errMsg(e)))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // A recognition that finishes while the window is up re-reads it; the preview
  // is Go's newest answer, not whatever the page happened to load first.
  useEffect(
    () =>
      onCommand((name) => {
        if (name !== 'ocr-preview') return
        setPicked('')
        setNatural({ w: 0, h: 0 })
        load()
      }),
    [load],
  )

  const outcomes = preview?.outcomes ?? []
  const lines = outcomes.flatMap((o) =>
    (o.blocks ?? []).map((b, i) => ({ key: keyOf(o.provider, i), provider: o.provider, block: b })),
  )
  const boxed = (b: Block) => natural.w > 0 && natural.h > 0 && b.width > 0 && b.height > 0

  const pick = (key: string) => {
    setPicked((cur) => (cur === key ? '' : key))
    // The list scrolls to the line the picture pointed at, so a click on a tiny
    // box still shows the text that belongs to it.
    requestAnimationFrame(() => {
      list.current?.querySelector(`[data-key="${CSS.escape(key)}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  }

  const copy = () => {
    if (!preview?.text) return
    void Service.CopyText(preview.text)
      .then(() => setNote(t('已复制')))
      .catch((e) => setNote(errMsg(e)))
  }

  // The pin keeps the preview above the app it was read from and stops the
  // window from hiding when it loses key, which is what EasyDict's OCRWindow
  // does with its isPinned flag.
  const flipPin = () => {
    void Service.SetOCRWindowPinned(!pinned)
      .then((v) => setPinned(!!v))
      .catch((e) => setNote(errMsg(e)))
  }

  return (
    <div className={`shell window-aux${isMac ? ' ocr-shell' : ''}`}>
      {isMac ? (
        <div className="ocrbar">
          <button
            className={`pinbtn ${pinned ? 'on' : ''}`}
            title={pinned ? t('取消钉住') : t('钉住窗口')}
            aria-pressed={pinned}
            onClick={flipPin}
          >
            <PinIcon filled={pinned} />
          </button>
          <div className="drag">
            <span className="title">{t('OCR 预览')}</span>
          </div>
          <span className="pinspacer" aria-hidden="true" />
          {note && <span className="status">{note}</span>}
        </div>
      ) : (
        <div className="dragbar">
          <div className="drag">
            <span className="title">{t('OCR 预览')}</span>
            {note && <span className="status">{note}</span>}
          </div>
          <WindowControls onClose={() => void Service.CloseOCR()} />
        </div>
      )}

      {!preview?.image ? (
        <div className="ocrempty">
          <svg viewBox="0 0 24 24" width="44" height="44" fill="none" stroke="currentColor"
            strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <rect x="3" y="4" width="18" height="14" rx="2" />
            <path d="M3 15l4.5-4.5 3 3L14 10l7 7" />
            <path d="M8 21h8" />
          </svg>
          <p>{t('还没有识别结果')}</p>
          <p className="hint">{t('用「截图 OCR」或「剪贴板 OCR」识别一张图片。')}</p>
          {preview?.error && <p className="ocre">{preview.error}</p>}
        </div>
      ) : (
        <div className="ocrpage">
          <section className="ocrpane">
            <h3>{t('识别图片')}</h3>
            <div className="ocrimgbox">
              <div className="ocrshot">
                <img
                  src={preview.image}
                  alt={t('识别图片')}
                  onLoad={(e) => {
                    const el = e.currentTarget
                    setNatural({ w: el.naturalWidth, h: el.naturalHeight })
                  }}
                />
                {lines.map(({ key, block }) =>
                  boxed(block) ? (
                    <button
                      key={key}
                      className={`ocrbox${picked === key ? ' on' : ''}`}
                      style={{
                        left: `${(block.x / natural.w) * 100}%`,
                        top: `${(block.y / natural.h) * 100}%`,
                        width: `${(block.width / natural.w) * 100}%`,
                        height: `${(block.height / natural.h) * 100}%`,
                      }}
                      title={block.text}
                      aria-label={block.text}
                      onClick={() => pick(key)}
                    />
                  ) : null,
                )}
              </div>
            </div>
          </section>

          <section className="ocrpane">
            <h3>{t('识别结果')}</h3>
            <div className="ocrlist" ref={list}>
              {outcomes.map((o) => (
                <div key={o.provider} className="ocrsvcgroup">
                  <div className="ocrsvc">
                    <strong>{o.label || o.provider}</strong>
                    <span className="spacer" />
                    <span>{t('{n} 行', { n: (o.blocks ?? []).length })}</span>
                  </div>
                  {o.error && <div className="ocre">{o.error}</div>}
                  {(o.blocks ?? []).map((b, i) => {
                    const key = keyOf(o.provider, i)
                    return (
                      <button
                        key={key}
                        data-key={key}
                        className={`ocrline${picked === key ? ' on' : ''}`}
                        onClick={() => pick(key)}
                      >
                        <span className="idx">{i + 1}</span>
                        <span className="txt">{b.text}</span>
                      </button>
                    )
                  })}
                </div>
              ))}
            </div>
          </section>

          <section className="ocrpane">
            <h3>
              {t('合并文本')}
              <span className="spacer" />
              <button className="tiny" onClick={copy} disabled={!preview.text}>{t('拷贝文本')}</button>
            </h3>
            <textarea className="ocrtext" readOnly spellCheck={false} value={preview.text} aria-label={t('合并文本')} />
            {preview.error && <div className="ocre ocrfoot">{preview.error}</div>}
          </section>
        </div>
      )}
    </div>
  )
}

// PinIcon is EasyDict's new_pin glyph: filled and blue while the preview stays
// on top, outlined and grey while it hides on the next click elsewhere.
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
