// 自动朗读 decides what a window should say without a button press. It lives
// apart from the windows because three of them share the rule, and the choice is
// easy to get wrong: a partial answer carries only OCR text, and 静音 OCR exists
// precisely so nothing is heard.
export type AutoReadAnswer = { text: string; lang: string }

export type AutoReadOptions = {
  // 查询英语单词后自动播放发音: a lone English word speaks itself even when
  // 自动朗读 is off. It never overrides an explicit 自动朗读 choice.
  pronounceEnglishWord?: boolean
}

type Readable = {
  partial?: boolean
  context?: string
  sourceText?: string
  detected?: string
  targetLang?: string
  translate?: { text?: string; error?: string }[] | null
}

export function autoReadAnswer(
  res: Readable | null,
  mode: string,
  opts: AutoReadOptions = {},
): AutoReadAnswer | null {
  if (!res) return null
  // A partial answer is the recognised text before any translation exists.
  if (res.partial) return null
  // 静音 OCR copies its text and shows no panel; a voice would break the silence
  // the mode is named for.
  if (res.context === 'silentOcr') return null

  if (!mode || mode === 'off') {
    const word = opts.pronounceEnglishWord ? englishWord(res) : ''
    return word ? { text: word, lang: 'en' } : null
  }

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

// englishWord is the 单词 case the auto-pronounce switch is about: one token of
// Latin letters, possibly carrying an apostrophe or a hyphen. A sentence, a
// number or a CJK word is left for 自动朗读 to decide.
export function englishWord(res: Readable | null): string {
  if (res?.detected !== 'en') return ''
  const text = (res?.sourceText ?? '').trim()
  return /^[A-Za-z][A-Za-z'’-]*$/.test(text) ? text : ''
}
