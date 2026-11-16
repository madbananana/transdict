// 交换 is EasyDict's toggleTranslationLanguages: a finished answer goes back as
// the next question, read in the reversed direction, so the swap shows the
// round trip instead of asking the same line twice. When there is nothing to
// feed back — a failed first service, a partial sketch, text edited after the
// answer — only the pair turns around and the text on screen is asked again.
import type { Response } from './api'

export type SwapPlan =
  | { kind: 'auto'; text: string }
  | { kind: 'reverse'; text: string; from: string; to: string }
  | { kind: 'pair'; from: string; to: string }
  | { kind: 'none' }

/** firstTranslation is EasyDict's firstTranslatedText: the first service's
 * finished answer for the query on screen. */
export function firstTranslation(res: Response | null | undefined): string {
  const first = res?.translate?.[0]
  if (!first || first.pending || first.error) return ''
  return (first.text ?? '').trim()
}

export function planSwap(
  res: Response | null,
  current: string,
  source: string,
  target: string,
): SwapPlan {
  const text = current.trim()
  const answered = !!res && !res.partial && text !== '' && text === (res.sourceText ?? '').trim()
  const back = answered ? firstTranslation(res) : ''
  const revFrom = res?.detected ?? ''
  const revTo = res?.targetLang ?? ''
  // Both sides on 自动: ask the answer again as it is, keeping the pair.
  if (back && source === 'auto' && target === 'auto') return { kind: 'auto', text: back }
  if (back && revFrom && revTo && revFrom !== 'auto' && revTo !== 'auto' && revFrom !== revTo) {
    return { kind: 'reverse', text: back, from: revTo, to: revFrom }
  }
  // 自动选择 has no concrete side to move left; the language the last answer
  // came back in is what the user was reading.
  const to = target === 'auto' ? (res?.targetLang || 'zh-CN') : target
  let from = source
  if (from === 'auto') {
    from = res?.detected && res.detected !== to ? res.detected : to === 'en' ? 'zh-CN' : 'en'
  }
  if (from === to) return { kind: 'none' }
  return { kind: 'pair', from, to }
}
