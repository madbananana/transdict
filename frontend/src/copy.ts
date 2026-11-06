// What 复制全文 turns an answer into. Kept out of the component so the text a
// paste produces can be checked without a browser.
import type { Entry, Response } from './api'

// bracketed wraps a transcription in the brackets the card prints. Some
// dictionaries hand the symbol back already wrapped, and adding a second pair
// read as [[ˈæpl]].
function bracketed(symbol: string): string {
  const s = (symbol ?? '').trim()
  return `[${s.startsWith('[') && s.endsWith(']') ? s.slice(1, -1) : s}]`
}

export type Phone = { text: string; lang: string; audio?: string }

// phones drops the second copy of a transcription some dictionaries return for
// both accents, and only labels UK/US when the two actually differ — otherwise
// the row read "UK [ˈæp(ə)l] US [ˈæp(ə)l]" as a typo.
export function phones(entry: Entry): Phone[] {
  const seen = new Set<string>()
  const out: Phone[] = []
  for (const p of entry.phonetics ?? []) {
    if (!p.symbol) continue
    const text = bracketed(p.symbol)
    if (seen.has(text)) continue
    seen.add(text)
    out.push({ text, lang: p.lang ?? '', audio: p.audio })
  }
  if (out.length < 2) for (const p of out) p.lang = ''
  return out
}

/** plainEntry is what a dictionary card contributes to 复制全文: the text the card
 * shows, with nothing of the layout in it. */
export function plainEntry(entry: Entry): string {
  if (entry.text) {
    // An MDX article arrives as HTML. Copied into a note it should paste the
    // words, not the markup, so block boundaries become newlines and the tags
    // and entities are dropped. Plain Apple definitions pass through unchanged.
    let body = entry.text
      .replace(/<(script|style)[^>]*>[\s\S]*?<\/\1>/gi, '')
      .replace(/<br\s*\/?>/gi, '\n')
      .replace(/<\/(p|div|li|h[1-6]|tr)>/gi, '\n')
      .replace(/<[^>]*>/g, '')
      .replace(/&nbsp;/g, ' ')
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/\s*▸\s*/g, '\n▸ ')
      .replace(/\n{3,}/g, '\n\n')
      .trim()
    // Apple's definition repeats the headword before the facts ("apple | BrE …").
    // The card drops it because the heading already says the word; the copy starts
    // with the query itself, so it drops it for the same reason.
    if (entry.word) {
      const bar = body.indexOf('|')
      if (bar > 0 && body.slice(0, bar).trim().toLowerCase() === entry.word.trim().toLowerCase()) {
        body = body.slice(bar + 1).trim()
      }
    }
    return body
  }
  // The card lays a structured entry out in blocks, so the copy puts each of
  // them on its own line too: headword and phonetics, then inflections, senses,
  // web senses and the example pairs, in the order the card shows them.
  const lines: string[] = []
  const head = [entry.word, ...phones(entry).map((p) => (p.lang ? `${p.lang} ${p.text}` : p.text))]
    .filter(Boolean).join(' ')
  if (head) lines.push(head)
  if (entry.exchange?.length) lines.push(entry.exchange.filter(Boolean).join('　'))
  for (const d of entry.definitions ?? []) {
    const sense = [d.pos, (d.meanings ?? []).join('；')].filter(Boolean).join(' ')
    if (sense) lines.push(sense)
    for (const e of d.examples ?? []) if (e) lines.push(e)
  }
  for (const w of entry.web ?? []) {
    for (const c of w.conts ?? []) {
      const sense = [w.value, c].filter(Boolean).join('：')
      if (sense) lines.push(sense)
    }
  }
  for (const e of entry.examples ?? []) {
    if (e.source) lines.push(e.source)
    if (e.translation) lines.push(e.translation)
  }
  return lines.join('\n')
}

/** copyAll gathers the answer in reading order, so one paste carries the source
 * and every service that answered. */
export function copyAll(res: Response): string {
  const parts: string[] = []
  if (res.sourceText) parts.push(res.sourceText)
  for (const x of res.translate ?? []) if (!x.error && x.text) parts.push(`${x.label}: ${x.text.trim()}`)
  for (const d of res.dict ?? []) {
    const lines = (d.entries ?? []).map(plainEntry).filter(Boolean)
    if (lines.length) parts.push(`${d.label}: ${lines.join('\n\n')}`)
  }
  return parts.join('\n\n')
}
