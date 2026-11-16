import { useCallback, useEffect, useRef, useState } from 'react'
import { Service, SettingsService, type CaptureArea } from '../bindings/transdict/internal/gui/index.js'
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
  const [hideTip, setHideTip] = useState(false)
  // 上一次截图区域 arrives in the frame's own pixels; the D key turns it into
  // the CSS rectangle the marquee is drawn in.
  const [lastArea, setLastArea] = useState<CaptureArea | null>(null)
  const drag = useRef<Drag | null>(null)
  const imgRef = useRef<HTMLImageElement | null>(null)
  // abandoned stops the one-second D preview: Esc during that second must not
  // let the delayed commit land on a capture the user already dismissed.
  const abandoned = useRef(false)
  // rectRef follows the marquee without waiting for a render: the mouseup that
  // ends a fast drag lands before React has re-rendered the last mousemove.
  const rectRef = useRef<Rect | null>(null)
  const place = (r: Rect | null) => {
    rectRef.current = r
    setRect(r)
  }

  // armed is the one-second D preview: the last area is drawn and the overlay
  // stays fully usable. Any new gesture disarms it instead of leaving a delayed
  // commit aimed at a rectangle the user has already moved on from.
  const [armed, setArmed] = useState(false)
  const armTimer = useRef<number | null>(null)
  const disarm = useCallback(() => {
    if (armTimer.current !== null) {
      window.clearTimeout(armTimer.current)
      armTimer.current = null
    }
    setArmed(false)
  }, [])
  useEffect(() => () => {
    if (armTimer.current !== null) {
      window.clearTimeout(armTimer.current)
    }
  }, [])

  const cancel = useCallback(() => {
    abandoned.current = true
    disarm()
    void Service.Cancel()
  }, [disarm])

  const commit = useCallback(
    (r: Rect) => {
      disarm()
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
    [frame, cancel, disarm],
  )

  useEffect(() => {
    Service.FrameDataURL(frame)
      .then((url) => {
        if (url) {
          setFrameURL(url)
        } else {
          setFailed(true)
        }
      })
      .catch(() => setFailed(true))
    void Service.LastCaptureArea(frame).then((a) => setLastArea(a ?? null))
    void SettingsService.Get().then((cfg) => setHideTip(!!cfg?.hideScreenshotTip))
  }, [frame])

  // The window stays hidden until this reports back, so the first thing the
  // user sees is the frozen frame rather than a white webview. A capture that
  // failed still reports: the error message is the overlay's job too.
  useEffect(() => {
    if (loaded || failed) {
      void Service.CaptureReady(frame).catch(() => undefined)
    }
  }, [loaded, failed, frame])

  // 选择上一次截图区域: D reuses the rectangle of the last capture on this
  // display. It stays up for a second before it commits, the way EasyDict does,
  // so the user sees what is about to be taken — without the dimmed busy layer,
  // which made that second read as a stall. Esc (or any new drag) is the way out.
  const reuseLast = useCallback(() => {
    const el = imgRef.current
    if (!lastArea || !el || !el.naturalWidth || !el.clientWidth) {
      return
    }
    const k = el.naturalWidth / el.clientWidth
    const r = { x: lastArea.x / k, y: lastArea.y / k, w: lastArea.w / k, h: lastArea.h / k }
    place(r)
    setArmed(true)
    armTimer.current = window.setTimeout(() => {
      armTimer.current = null
      setArmed(false)
      if (!abandoned.current) {
        commit(r)
      }
    }, 1000)
  }, [lastArea, commit])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        cancel()
      } else if (e.key === 'Enter' && rect) {
        e.preventDefault()
        commit(rect)
      } else if ((e.key === 'd' || e.key === 'D') && !e.metaKey && !e.ctrlKey && !e.altKey && lastArea && loaded && !working) {
        e.preventDefault()
        reuseLast()
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
    // 右键取消 is EasyDict's second exit beside Esc; without it a right-click
    // would only swallow the menu and leave the frozen screen up.
    const onContext = (e: MouseEvent) => {
      e.preventDefault()
      cancel()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mouseup', onUp)
    window.addEventListener('contextmenu', onContext)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mouseup', onUp)
      window.removeEventListener('contextmenu', onContext)
    }
  }, [rect, cancel, commit, lastArea, reuseLast])

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
      className={armed ? 'capture armed' : 'capture'}
      onMouseDown={(e) => {
        if (e.button !== 0 || !loaded || working) {
          return
        }
        // A fresh drag replaces the D preview rather than racing its timer.
        disarm()
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
            disarm()
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
                disarm()
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
      {loaded && !working && !hideTip && (
        <div className="hint">
          <span>{t('拖拽框选文字区域，松开即翻译')}</span>
          <span>
            <kbd>Enter</kbd> {t('翻译')}
          </span>
          <span>
            <kbd>Esc</kbd> {t('或右键取消')}
          </span>
          {/* The D tip only exists once a rectangle does; EasyDict words it as
              one sentence rather than a key label. */}
          {lastArea && <span>{t('单击 D 键选择上一次截图区域')}</span>}
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
