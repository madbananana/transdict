// The pure half of 识别为: naming the language a query box currently holds. It
// is split from the React hook so the decision can be tested with `node
// src/detect.test.ts`, without loading the Wails runtime or a DOM.
import type { Response } from './api'

export type TextLanguage = { text: string; code: string }

export const noLanguage: TextLanguage = { text: '', code: '' }

// resolveDetected answers which language the chip should name for the text
// currently in the box. A pick the user made outranks everything (EasyDict's
// specifiedTextLanguageDict), then the language the last answer was asked in,
// then the standalone typing pass. A detection or answer about another text is
// stale and must not label what the user is typing now.
export function resolveDetected(
  value: string,
  forced: TextLanguage | null,
  typed: TextLanguage,
  res: Response | null,
): string {
  if (!value) return ''
  if (forced && forced.text === value) return forced.code
  if (res && (res.sourceText ?? '').trim() === value) {
    const code = res.sourceLang || res.detected
    if (code) return code
  }
  return typed.text === value ? typed.code : ''
}

// detectedLabel maps a detected code to the name the chip shows. 自动检测 is the
// honest label while the recogniser has no concrete answer, which is exactly
// when the override menu matters most.
export function detectedLabel(code: string, name: (code: string) => string): string {
  if (!code) return ''
  return code === 'auto' ? name('auto') : name(code) || code
}
