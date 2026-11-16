// Run with `npm --prefix frontend test` — plain node, because the quick links
// are string transforms and a browser is not needed to know what will open.
import { quickLink } from './links.ts'

const cases: [string, Parameters<typeof quickLink>[0], string, string][] = [
  ['Google 搜索多词', 'google', '  hello world ', 'https://www.google.com/search?q=hello%20world'],
  ['Google 不把整条 URL 再搜一遍', 'google', 'https://example.com/a?b=1', 'https://example.com/a?b=1'],
  ['苹果词典用 dict 方案', 'apple', 'apple', 'dict://apple'],
  ['欧路词典的路径要转义', 'eudic', 'hello world', 'eudic://dict/hello%20world'],
  ['空文本什么都不打开', 'google', '   ', ''],
]

let failures = 0
for (const [name, kind, text, want] of cases) {
  const got = quickLink(kind, text)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(got)}\n  期望 ${JSON.stringify(want)}`)
  }
}

if (failures > 0) throw new Error(`${failures} 个快捷链接用例未通过`)
console.log(`ok — ${cases.length} 个快捷链接用例通过`)

// A screenshot can carry links; EasyDict highlights them, so the spans have to
// name exactly the URL, without the sentence's punctuation.
import { linkSpans } from './links.ts'

const spanCases: [string, string, { shown: string; url: string }[]][] = [
  ['句中的链接', '见 https://example.com/a?b=1 的说明', [{ shown: 'https://example.com/a?b=1', url: 'https://example.com/a?b=1' }]],
  ['裸 www 补上 https', '打开 www.apple.com 查看', [{ shown: 'www.apple.com', url: 'https://www.apple.com' }]],
  ['句号不属于链接', '这里是 https://example.com/x.', [{ shown: 'https://example.com/x', url: 'https://example.com/x' }]],
  ['两个链接', 'https://a.cn 和 https://b.cn', [
    { shown: 'https://a.cn', url: 'https://a.cn' },
    { shown: 'https://b.cn', url: 'https://b.cn' },
  ]],
  ['没有链接', 'hello world', []],
]

for (const [name, text, want] of spanCases) {
  const got = linkSpans(text)
  const brief = got.map((s) => ({ shown: s.shown, url: s.url }))
  if (JSON.stringify(brief) !== JSON.stringify(want)) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(brief)}\n  期望 ${JSON.stringify(want)}`)
  }
}

if (failures > 0) throw new Error(`${failures} 个链接用例未通过`)
console.log(`ok — ${spanCases.length} 个链接用例通过`)
