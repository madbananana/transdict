// 识别为 is EasyDict's EZDetectLanguageButton: every query view names the
// language it recognized, and the name is what opens the override menu. The
// recogniser runs on its own, one second after the text settles — not only when
// an answer arrives — because the chip has to be usable before the first query.
// The interval is EZDelayDetectTextLanguageInterval (1.0 s).
import { useEffect, useRef, useState } from 'react'
import { Service } from './api'
import { noLanguage, type TextLanguage } from './detectlang'

export { detectedLabel, noLanguage, resolveDetected, type TextLanguage } from './detectlang'

const detectDelay = 1000

// useTypingDetection watches the text being typed and names its language after
// the pause EasyDict allows. Detection for an older text is dropped, the way
// detectQueryText ignores a callback whose query text already changed.
export function useTypingDetection(text: string, enabled = true): TextLanguage {
  const [found, setFound] = useState<TextLanguage>(noLanguage)
  const last = useRef('')
  useEffect(() => {
    const value = text.trim()
    if (!enabled || !value) {
      last.current = ''
      setFound(noLanguage)
      return
    }
    if (value === last.current) return
    last.current = value
    let alive = true
    const timer = window.setTimeout(() => {
      void Service.DetectText(value)
        .then((code) => {
          if (alive) setFound({ text: value, code: code || '' })
        })
        .catch(() => undefined)
    }, detectDelay)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
  }, [text, enabled])
  return found
}
