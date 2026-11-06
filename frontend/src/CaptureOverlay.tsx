import { useCallback, useEffect, useRef, useState } from 'react'
import { Service } from '../bindings/transdict/internal/gui/index.js'
import { t } from './i18n'

type Rect = { x: number; y: number; w: number; h: number }
type Edge = { l: boolean; t: boolean; r: boolean; b: boolean }
type Drag =
  | { kind: 'new'; from: { x: number; y: number } }
  | { kind: 'move'; start: Rect; from: { x: number; y: number } }
  | { kind: 'resize'; edge: Edge; start: Rect; from: { x: number; y: number } }

const MIN_SIZE = 6

// The eight handles, clockwise from the top-left corner.
const HANDLES: { edge: Edge; cursor: string; x: number; y: number }[] = [
  { edge: { l: true, t: true, r: false, b: false }, cursor: 'nwse-resize', x: 0, y: 0 },
  { edge: { l: false, t: true, r: false, b: false }, cursor: 'ns-resize', x: 0.5, y: 0 },
  { edge: { l: false, t: true, r: true, b: false }, cursor: 'nesw-resize', x: 1, y: 0 },
  { edge: { l: false, t: false, r: true, b: false }, cursor: 'ew-resize', x: 1, y: 0.5 },
  { edge: { l: false, t: false, r: true, b: true }, cursor: 'nwse-resize', x: 1, y: 1 },
  { edge: { l: false, t: false, r: false, b: true }, cursor: 'ns-resize', x: 0.5, y: 1 },
  { edge: { l: true, t: false, r: false, b: true }, cursor: 'nesw-resize', x: 0, y: 1 },
  { edge: { l: true, t: false, r: false, b: false }, cursor: 'ew-resize', x: 0, y: 0.5 },
]

export default function CaptureOverlay({ frame }: { frame: number }) {
  const [frameURL, setFrameURL] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)
  const [rect, setRect] = useState<Rect | null>(null)
  const [working, setWorking] = useState('')
  const drag = useRef<Drag | null>(null)
  const imgRef = useRef<HTMLImageElement | null>(null)
  // rectRef follows the marquee without waiting for a render: the mouseup that
  // ends a fast drag lands before React has re-rendered the last mousemove.
  const rectRef = useRef<Rect | null>(null)
  const place = (r: Rect | null) => {
    rectRef.current = r
    setRect(r)
  }

  const cancel = useCallback(() => {
    void Service.Cancel()
  }, [])

  const commit = useCallback(
    (r: Rect) => {
      const el = imgRef.current
      if (!el || !el.naturalWidth || r.w < MIN_SIZE || r.h < MIN_SIZE) {
        cancel()
        return
      }
      // The frozen screenshot is scaled to fill the window, so CSS pixels map to
      // source pixels by the display's backing scale.
      const k = el.naturalWidth / el.clientWidth
      setWorking(t('识别中…'))
      void Service.Commit(frame, Math.round(r.x * k), Math.round(r.y * k), Math.round(r.w * k), Math.round(r.h * k))
        .catch(() => cancel())
    },
    [frame, cancel],
  )

  useEffect(() => {
    Service.FrameDataURL(frame)
      .then(setFrameURL)
      .catch(() => setFailed(true))
  }, [frame])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        cancel()
      } else if (e.key === 'Enter' && rect) {
        e.preventDefault()
        commit(rect)
      }
    }
    const onUp = () => {
      const d = drag.current
      drag.current = null
      const cur = rectRef.current
      if (!d || d.kind !== 'new' || !cur) {
        return
      }
      if (cur.w < MIN_SIZE || cur.h < MIN_SIZE) {
        // A click, not a selection: leave nothing behind.
        place(null)
        return
      }
      // Recognizing starts on the release itself: 截图翻译 is one gesture, and
      // any delay here sits in front of OCR and every translator.
      commit(cur)
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mouseup', onUp)
    window.addEventListener('contextmenu', (e) => e.preventDefault())
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mouseup', onUp)
    }
  }, [rect, cancel, commit])

  const move = (e: React.MouseEvent) => {
    const d = drag.current
    if (!d) {
      return
    }
    const dx = e.clientX - d.from.x
    const dy = e.clientY - d.from.y
    if (d.kind === 'new') {
      const x0 = clamp(Math.min(d.from.x, e.clientX), 0, window.innerWidth)
      const y0 = clamp(Math.min(d.from.y, e.clientY), 0, window.innerHeight)
      const x1 = clamp(Math.max(d.from.x, e.clientX), 0, window.innerWidth)
      const y1 = clamp(Math.max(d.from.y, e.clientY), 0, window.innerHeight)
      place({ x: x0, y: y0, w: x1 - x0, h: y1 - y0 })
      return
    }
    if (d.kind === 'move') {
      place({
        x: clamp(d.start.x + dx, 0, window.innerWidth - d.start.w),
        y: clamp(d.start.y + dy, 0, window.innerHeight - d.start.h),
        w: d.start.w,
        h: d.start.h,
      })
      return
    }
    // Only the edges a handle owns follow the pointer; the opposite edge is
    // pinned, so dragging a corner never moves the corner you are holding.
    let l = d.start.x
    let tp = d.start.y
    let r = l + d.start.w
    let b = tp + d.start.h
    if (d.edge.l) {
      l = clamp(l + dx, 0, r)
    }
    if (d.edge.r) {
      r = clamp(r + dx, l, window.innerWidth)
    }
    if (d.edge.t) {
      tp = clamp(tp + dy, 0, b)
    }
    if (d.edge.b) {
      b = clamp(b + dy, tp, window.innerHeight)
    }
    place({ x: l, y: tp, w: r - l, h: b - tp })
  }

  // The frozen screen is shown as it is: no dim, no shaded bands outside the
  // selection. The marquee carries its own outline, so the frame stays
  // readable without masking what is being read.

  // The action bar flips above the selection when there is no room below it,
  // clearing the size badge that always sits on the top edge.
  const barY = rect ? (rect.y + rect.h + 52 < window.innerHeight ? rect.y + rect.h + 8 : rect.y - 76) : 0

  return (
    <div
      className="capture"
      onMouseDown={(e) => {
        if (e.button !== 0 || !loaded || working) {
          return
        }
        drag.current = { kind: 'new', from: { x: e.clientX, y: e.clientY } }
        place(null)
      }}
      onMouseMove={move}
    >
      {frameURL && (
        <img
          ref={imgRef}
          src={frameURL}
          alt=""
          draggable={false}
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      )}
      {rect && rect.w > 0 && (
        <div
          className="marquee"
          style={{ left: rect.x, top: rect.y, width: rect.w, height: rect.h }}
          onMouseDown={(e) => {
            if (e.button !== 0 || working) {
              return
            }
            e.stopPropagation()
            drag.current = { kind: 'move', start: rect, from: { x: e.clientX, y: e.clientY } }
          }}
        >
          <div className="size">
            {Math.round(rect.w)} × {Math.round(rect.h)}
          </div>
          {HANDLES.map((h, i) => (
            <span
              key={i}
              className="grip"
              style={{ left: `${h.x * 100}%`, top: `${h.y * 100}%`, cursor: h.cursor }}
              onMouseDown={(e) => {
                if (e.button !== 0 || working) {
                  return
                }
                e.stopPropagation()
                drag.current = { kind: 'resize', edge: h.edge, start: rect, from: { x: e.clientX, y: e.clientY } }
              }}
            />
          ))}
        </div>
      )}
      {rect && rect.w >= MIN_SIZE && !working && (
        <div
          className="shotbar"
          style={{ left: Math.max(6, rect.x + rect.w - 124), top: Math.max(6, barY) }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <button className="tiny primary" onClick={() => commit(rect)}>
            {t('翻译')}
          </button>
          <button className="tiny" onClick={cancel}>
            {t('取消')}
          </button>
        </div>
      )}
      {failed && <div className="busy">{t('无法读取截图，请在系统设置中授予屏幕录制权限后重试。')}</div>}
      {/* The frozen frame crosses the bridge as an image, so the first moment
          after the shortcut is black; naming it keeps that from reading as a
          failed capture. */}
      {!loaded && !failed && <div className="busy">{t('正在准备截图…')}</div>}
      {!failed && working && <div className="busy">{working}</div>}
      {loaded && !working && (
        <div className="hint">
          <span>{t('拖拽框选文字区域，松开即翻译')}</span>
          <span>
            <kbd>Enter</kbd> {t('翻译')}
          </span>
          <span>
            <kbd>Esc</kbd> {t('取消')}
          </span>
          {/* Once a box exists the action bar beside it owns the buttons; before
              that, Esc is the only exit the overlay offers. */}
          {!rect && (
            <span className="quit" onClick={cancel}>
              {t('取消')}
            </span>
          )}
        </div>
      )}
    </div>
  )
}

function clamp(v: number, lo: number, hi: number): number {
  if (hi < lo) {
    return lo
  }
  return Math.min(Math.max(v, lo), hi)
}
