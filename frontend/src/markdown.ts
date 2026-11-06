// A model answers in Markdown: headings, bullet lists, `code`, **bold**. The card
// that quotes it verbatim shows the user the markup instead of the answer, so this
// turns the subset a translation reply actually contains into HTML.
//
// Everything is escaped before a single tag is emitted, and only the tags below
// are ever produced, so a reply cannot carry a script, a style sheet or a link to
// a non-web scheme. Links come back with their address in `href`, and the click
// handler in Result.tsx routes them to the browser instead of navigating the
// window.

const escaped = new Map<string, string>([
  ['&', '&amp;'], ['<', '&lt;'], ['>', '&gt;'], ['"', '&quot;'],
])

function esc(text: string): string {
  let out = ''
  for (const ch of text) out += escaped.get(ch) ?? ch
  return out
}

const fence = /^\s*```/
const rule = /^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/
const heading = /^(#{1,6})\s+(.*)$/
const quote = /^>\s?(.*)$/
const bullet = /^\s*[-*+]\s+(.*)$/
const numbered = /^\s*\d+[.)]\s+(.*)$/
// A line that starts a block of its own ends the paragraph above it.
const blockStart = /^\s*(?:```|#{1,6}\s|[-*+]\s|\d+[.)]\s|>)/

export function renderMarkdown(source: string): string {
  const lines = source.replace(/\r\n?/g, '\n').split('\n')
  const out: string[] = []
  let list: 'ul' | 'ol' | null = null

  const closeList = () => {
    if (list) out.push(`</${list}>`)
    list = null
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    if (fence.test(line)) {
      closeList()
      const body: string[] = []
      i++
      while (i < lines.length && !fence.test(lines[i])) {
        body.push(lines[i])
        i++
      }
      // A reply's code block ends with the newline that closed it, which <pre>
      // would otherwise show as an empty line.
      out.push(`<pre><code>${esc(body.join('\n').replace(/\n$/, ''))}</code></pre>`)
      continue
    }
    if (rule.test(line)) {
      closeList()
      out.push('<hr>')
      continue
    }
    const h = heading.exec(line)
    if (h) {
      closeList()
      const level = h[1].length
      out.push(`<h${level}>${inline(h[2])}</h${level}>`)
      continue
    }
    const q = quote.exec(line)
    if (q) {
      closeList()
      out.push(`<blockquote>${inline(q[1])}</blockquote>`)
      continue
    }
    const ul = bullet.exec(line)
    const ol = ul ? null : numbered.exec(line)
    if (ul || ol) {
      const kind = ul ? 'ul' : 'ol'
      if (list !== kind) {
        closeList()
        out.push(`<${kind}>`)
        list = kind
      }
      out.push(`<li>${inline(ul ? ul[1] : ol![1])}</li>`)
      continue
    }
    if (!line.trim()) {
      closeList()
      continue
    }

    closeList()
    const paragraph = [line]
    while (i + 1 < lines.length && lines[i + 1].trim() && !blockStart.test(lines[i + 1])) {
      paragraph.push(lines[++i])
    }
    out.push(`<p>${inline(paragraph.join('\n')).replace(/\n/g, '<br>')}</p>`)
  }
  closeList()
  return out.join('')
}

// inline handles the spans a reply actually uses. Code spans are converted first
// so markup inside them stays literal.
function inline(text: string): string {
  let out = esc(text)
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>')
  out = out.replace(/\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2">$1</a>')
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  // Emphasis only where the stars hug the word: "3 * 4 * 5" is arithmetic, and
  // turning it into italics would hide the answer's own meaning.
  out = out.replace(/(^|[\s(（【《「])\*(\S[^*\n]*?)\*(?=[\s)）】。，、：！？,.:;!?]|$)/g, '$1<em>$2</em>')
  out = out.replace(/(^|[\s(（【《「])__([^_]+)__(?=[\s)）】。，、：！？,.:;!?]|$)/g, '$1<strong>$2</strong>')
  out = out.replace(/~~([^~]+)~~/g, '<del>$1</del>')
  return out
}
