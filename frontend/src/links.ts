// 快捷链接: the titlebar icons hand the current text to another tool. Google
// takes it as a search — unless the text is already a link, which Easydict
// opens directly — and the dictionaries take it in their own URL schemes.
import { t } from './i18n.ts'

export type QuickLink = 'google' | 'apple' | 'eudic'

/** quickTitle is the tooltip every query window's titlebar uses, with the key
 * that runs the same link from the keyboard. */
export function quickTitle(kind: QuickLink): string {
  switch (kind) {
    case 'google':
      return `${t('通过 Google 搜索')}, ⌘↩`
    case 'apple':
      return `${t('在苹果词典中打开')}, ⌘⇧D`
    case 'eudic':
      return `${t('在欧路词典中打开')}, ⌘⇧↩`
  }
}

export function quickLink(kind: QuickLink, text: string): string {
  const query = text.trim()
  if (!query) return ''
  switch (kind) {
    case 'google':
      return /^https?:\/\/\S+$/i.test(query)
        ? query
        : `https://www.google.com/search?q=${encodeURIComponent(query)}`
    case 'apple':
      return `dict://${encodeURIComponent(query)}`
    case 'eudic':
      return `eudic://dict/${encodeURIComponent(query)}`
  }
}

/** A URL found in recognized text. `shown` drops sentence punctuation that
 * belongs to the sentence rather than the link; `url` is what opens. */
export type LinkSpan = { start: number; end: number; shown: string; url: string }

const linkPattern = /(?:https?:\/\/|www\.)[^\s<>"'“”‘’（）()【】[\]]+/g

// linkSpans is EasyDict's highlightAllLinks for the text a screenshot carried:
// every URL in the recognized text becomes something to click.
export function linkSpans(text: string): LinkSpan[] {
  const spans: LinkSpan[] = []
  for (const m of text.matchAll(linkPattern)) {
    const shown = m[0].replace(/[.,;:!?]+$/, '')
    const start = m.index ?? 0
    spans.push({
      start, end: start + shown.length, shown,
      url: shown.startsWith('www.') ? `https://${shown}` : shown,
    })
  }
  return spans
}
