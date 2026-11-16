import { useCallback, useEffect, useRef, useState } from 'react'

// autoQueryDelay is EZQueryView's EZAutoQueryWhenTextChangedDelay: 0.8 s is
// long enough that a sentence is not queried on every key, short enough that a
// typist who stops is answered without reaching for Enter.
const autoQueryDelay = 800

// useAutoQuery is EasyDict's 输入时自动查询, shared by every query box. The
// caller owns the text and the last asked text; this hook only schedules.
//
// The returned cancel is what makes the behaviour match: EasyDict drops the
// pending debounce on Enter, on paste (that gesture is governed by
// 粘贴后自动查询 instead) and when the box loses focus. Without it the timer
// would fire again after the answer already started and ask twice.
export function useAutoQuery(
  text: string,
  enabled: boolean,
  asked: { current: string },
  run: (value: string) => void,
): () => void {
  const runRef = useRef(run)
  runRef.current = run
  const timer = useRef<number | undefined>(undefined)
  // EasyDict's autoQueryWhenTextChanged bails while the text view still has
  // marked text, so a pause mid-pinyin is never queried. The composition
  // events fire on the textarea and bubble to the window, which is where the
  // three query windows can watch them without wiring each box.
  const [composing, setComposing] = useState(false)

  useEffect(() => {
    const start = () => setComposing(true)
    const end = () => setComposing(false)
    window.addEventListener('compositionstart', start)
    window.addEventListener('compositionend', end)
    return () => {
      window.removeEventListener('compositionstart', start)
      window.removeEventListener('compositionend', end)
    }
  }, [])

  const cancel = useCallback(() => {
    if (timer.current === undefined) return
    window.clearTimeout(timer.current)
    timer.current = undefined
  }, [])

  useEffect(() => {
    cancel()
    // compositionend re-runs this effect with the committed text, so the
    // debounce starts once the IME has settled rather than being lost.
    if (!enabled || composing) return
    const value = text.trim()
    if (!value || value === asked.current) return
    timer.current = window.setTimeout(() => {
      timer.current = undefined
      // The text may have been asked by hand while the timer was pending; the
      // timer is not a second request for the same words.
      if (asked.current === value) return
      runRef.current(value)
    }, autoQueryDelay)
    return cancel
  }, [text, enabled, composing, asked, cancel])

  return cancel
}
