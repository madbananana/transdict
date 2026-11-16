// 应用内查询: right-clicking a word in a result looks it up without retyping
// it, the same first entry EasyDict's press-and-hold menu offers. The word is
// the current selection, or the text under the pointer when nothing is selected.
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Service, errMsg, readAloud } from './api'
import { t } from './i18n'
import { quickLink, type QuickLink } from './links'

interface MenuState {
  x: number
  y: number
  word: string
}

const wordChar = /[\p{L}\p{N}_'’-]/u

function caretAt(x: number, y: number): { node: Node; offset: number } | null {
  const doc = document as Document & {
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null
    caretRangeFromPoint?: (x: number, y: number) => Range | null
  }
  const pos = doc.caretPositionFromPoint?.(x, y)
  if (pos) return { node: pos.offsetNode, offset: pos.offset }
  const range = doc.caretRangeFromPoint?.(x, y)
  return range ? { node: range.startContainer, offset: range.startOffset } : null
}

// wordUnderPointer widens the caret to the surrounding word. CJK runs are
// widened too, which is what a right-click on 汉字 should hand the query.
function wordUnderPointer(x: number, y: number): string {
  const caret = caretAt(x, y)
  if (!caret || caret.node.nodeType !== Node.TEXT_NODE) return ''
  const text = caret.node.textContent ?? ''
  let start = Math.min(caret.offset, text.length)
  let end = start
  while (start > 0 && wordChar.test(text[start - 1])) start--
  while (end < text.length && wordChar.test(text[end])) end++
  return text.slice(start, end).trim()
}

export function useWordMenu(notice?: (msg: string) => void, speakLang = '', links: QuickLink[] = []) {
  const [menu, setMenu] = useState<MenuState | null>(null)
  const box = useRef<HTMLDivElement | null>(null)

  // The menu is wider than the window when the quoted word is long or the quick
  // links are on, and WebKit clips it at the frame edge. Once painted, nudge it
  // back inside the viewport; the click point is only a first guess.
  useEffect(() => {
    const el = box.current
    if (!menu || !el) return
    const r = el.getBoundingClientRect()
    const right = window.innerWidth - r.width - 8
    const bottom = window.innerHeight - r.height - 8
    const x = Math.min(menu.x, Math.max(8, right))
    const y = Math.min(menu.y, Math.max(8, bottom))
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

  const onContextMenu = (e: React.MouseEvent<HTMLElement>) => {
    const target = e.target as HTMLElement
    // Buttons and fields keep their own menus and actions.
    if (target.closest('button, a, input, textarea, select')) return
    e.preventDefault()
    const selection = window.getSelection()
    let word = selection && !selection.isCollapsed ? selection.toString().trim() : ''
    if (!word) word = wordUnderPointer(e.clientX, e.clientY)
    if (!word) {
      setMenu(null)
      return
    }
    setMenu({ x: e.clientX, y: e.clientY, word })
  }

  const lookUp = () => {
    if (!menu) return
    const word = menu.word
    setMenu(null)
    void Service.Ask({ text: word, context: 'main', mode: 'input', sourceLang: '', targetLang: '' })
      .catch((e) => notice?.(errMsg(e)))
  }

  const copy = () => {
    if (!menu) return
    const word = menu.word
    setMenu(null)
    void Service.CopyText(word).then((ok) => {
      if (!ok) notice?.(t('复制失败，请手动选择文字'))
    })
  }

  // 朗读 uses the language the answer was read as, so an English word gets the
  // English voice even when the interface is Chinese.
  const speak = () => {
    if (!menu) return
    const word = menu.word
    setMenu(null)
    void readAloud(word, speakLang, false, notice)
  }

  const open = (kind: QuickLink) => {
    if (!menu) return
    const url = quickLink(kind, menu.word)
    setMenu(null)
    if (url) void Service.OpenLink(url).catch((e) => notice?.(errMsg(e)))
  }

  const wordMenu: ReactNode = menu && (
    <div
      ref={box}
      className="wordmenu"
      style={{ left: menu.x, top: menu.y }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <button className="lookup" onClick={lookUp}>{`${t('应用内查询')} "${menu.word}"`}</button>
      <div className="qsep" />
      <button onClick={copy}>{t('复制')}</button>
      <button onClick={speak}>{t('朗读')}</button>
      {links.length > 0 && <div className="qsep" />}
      {links.includes('google') && <button onClick={() => open('google')}>{t('通过 Google 搜索')}</button>}
      {links.includes('eudic') && <button onClick={() => open('eudic')}>{t('在欧路词典中打开')}</button>}
      {links.includes('apple') && <button onClick={() => open('apple')}>{t('在苹果词典中打开')}</button>}
    </div>
  )

  return { onContextMenu, wordMenu }
}
