// 应用内查询, on the query box: EasyDict's EZQueryMenuTextView prepends
// `应用内查询 "<selection>"` to the text view's context menu, and a collapsed
// caret uses the word under it. A WebKit textarea's menu cannot be extended, so
// a right-click on a word gets this menu instead, carrying the edit actions the
// field needs so replacing the native menu costs nothing.
import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react'
import { Service, errMsg, readAloud } from './api'
import { t } from './i18n'

const wordChar = /[\p{L}\p{N}_'’-]/u

interface MenuState {
  x: number
  y: number
  word: string
  start: number
  end: number
}

// wordAround widens a caret to the surrounding word, the same rule
// wordmenu.tsx uses on result labels.
function wordAround(text: string, caret: number): [number, number] {
  let start = Math.min(caret, text.length)
  let end = start
  while (start > 0 && wordChar.test(text[start - 1])) start--
  while (end < text.length && wordChar.test(text[end])) end++
  return [start, end]
}

export function useInputMenu(opts: {
  box: RefObject<HTMLTextAreaElement | null>
  from: 'main'
  onChange: (text: string) => void
  notice?: (msg: string) => void
  speakLang?: string
  beforeLookUp?: () => void
  // onPasted runs after 粘贴 rewrote the value. The menu is this field's only
  // paste path (the native menu is replaced), so it is where 粘贴后自动查询
  // and the 输入时自动查询 cancellation hook in — the same place EasyDict's
  // EZTextView.paste does it.
  onPasted?: (text: string) => void
}) {
  const [menu, setMenu] = useState<MenuState | null>(null)
  const boxRef = useRef<HTMLDivElement | null>(null)

  // Keep the replacing menu inside the frame: with 朗读 its height passes the
  // old fixed guess, and a long quoted word can pass the width too.
  useEffect(() => {
    const el = boxRef.current
    if (!menu || !el) return
    const r = el.getBoundingClientRect()
    const x = Math.min(menu.x, Math.max(8, window.innerWidth - r.width - 8))
    const y = Math.min(menu.y, Math.max(8, window.innerHeight - r.height - 8))
    if (x !== menu.x || y !== menu.y) setMenu({ ...menu, x, y })
  }, [menu])

  useEffect(() => {
    if (!menu) return
    const close = () => setMenu(null)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    window.addEventListener('mousedown', close)
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
      window.removeEventListener('keydown', onKey)
    }
  }, [menu])

  const onContextMenu = (e: React.MouseEvent<HTMLTextAreaElement>) => {
    const el = opts.box.current
    if (!el || e.target !== el) return
    // The browser has already put the caret or selection where the click
    // landed; a collapsed one still names the word the menu is about.
    let start = el.selectionStart ?? 0
    let end = el.selectionEnd ?? start
    if (start === end) [start, end] = wordAround(el.value, start)
    const word = el.value.slice(start, end).trim()
    if (!word) {
      setMenu(null)
      return
    }
    e.preventDefault()
    setMenu({ x: e.clientX, y: e.clientY, word, start, end })
  }

  // apply rewrites the controlled value and leaves the caret where the edit
  // ended; the state update has not reached the DOM yet, so the caret is set on
  // the next frame.
  const apply = (next: string, caret: number) => {
    opts.onChange(next)
    requestAnimationFrame(() => {
      const el = opts.box.current
      if (!el) return
      el.focus()
      el.setSelectionRange(caret, caret)
    })
  }

  const lookUp = () => {
    const current = menu
    setMenu(null)
    if (!current) return
    opts.beforeLookUp?.()
    void Service.QueryInApp(current.word, opts.from).catch((e) => opts.notice?.(errMsg(e)))
  }

  const copy = () => {
    const current = menu
    setMenu(null)
    if (!current) return
    void Service.CopyText(current.word).then((ok) => {
      if (!ok) opts.notice?.(t('复制失败，请手动选择文字'))
    })
  }

  const cut = () => {
    const current = menu
    const el = opts.box.current
    setMenu(null)
    if (!current || !el) return
    apply(el.value.slice(0, current.start) + el.value.slice(current.end), current.start)
    void Service.CopyText(current.word).catch(() => undefined)
  }

  const paste = () => {
    const current = menu
    const el = opts.box.current
    setMenu(null)
    if (!current || !el) return
    const value = el.value
    void Service.ClipboardText()
      .then((clip) => {
        const text = clip.trim()
        if (!text) return
        const next = value.slice(0, current.start) + text + value.slice(current.end)
        apply(next, current.start + text.length)
        opts.onPasted?.(next)
      })
      .catch((e) => opts.notice?.(errMsg(e)))
  }

  const selectAll = () => {
    setMenu(null)
    const el = opts.box.current
    if (!el) return
    el.focus()
    el.select()
  }

  // 朗读 is the one NSTextView menu entry the webview cannot draw for us, so
  // the replacing menu keeps it: EasyDict's native text menu offers Speech too.
  const speak = () => {
    const current = menu
    setMenu(null)
    if (!current) return
    void readAloud(current.word, opts.speakLang ?? '', false, opts.notice)
  }

  const inputMenu: ReactNode = menu && (
    <div
      ref={boxRef}
      className="wordmenu"
      style={{ left: menu.x, top: menu.y }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <button className="lookup" onClick={lookUp}>{`${t('应用内查询')} "${menu.word}"`}</button>
      <div className="qsep" />
      <button onClick={cut}>{t('剪切')}</button>
      <button onClick={copy}>{t('复制')}</button>
      <button onClick={paste}>{t('粘贴')}</button>
      <button onClick={selectAll}>{t('全选')}</button>
      <div className="qsep" />
      <button onClick={speak}>{t('朗读')}</button>
    </div>
  )

  return { onContextMenu, inputMenu }
}
