// 自动朗读 decides what a window should say without a button press. It lives
// apart from the windows because three of them share the rule, and the choice is
// easy to get wrong: a partial answer carries only OCR text, and 静音 OCR exists
// precisely so nothing is heard.
export type AutoReadAnswer = { text: string; lang: string }

type Readable = {
  partial?: boolean
  context?: string
  sourceText?: string
  detected?: string
  targetLang?: string
  translate?: { text?: string; error?: string }[] | null
}

export function autoReadAnswer(res: Readable | null, mode: string): AutoReadAnswer | null {
  if (!res || !mode || mode === 'off') return null
  // A partial answer is the recognised text before any translation exists.
  if (res.partial) return null
  // 静音 OCR copies its text and shows no panel; a voice would break the silence
  // the mode is named for.
  if (res.context === 'silentOcr') return null

  if (mode === 'source') {
    const text = (res.sourceText ?? '').trim()
    return text ? { text, lang: res.detected || 'auto' } : null
  }

  const answer = (res.translate ?? []).find((x) => !x.error && (x.text ?? '').trim())
  if (!answer?.text) return null
  // The answer is in the target language; detected is only a fallback for a
  // result that never carried one.
  return { text: answer.text, lang: res.targetLang || res.detected || 'auto' }
}
