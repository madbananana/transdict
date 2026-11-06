// Run with `npm --prefix frontend test` — plain node, because 复制全文 is a pure
// text transform and a browser is not needed to know what a paste will contain.
import { copyAll, plainEntry } from './copy.ts'

const cases: [string, Parameters<typeof plainEntry>[0], string][] = [
  // Apple's entry repeats the headword and separates sub-senses with ▸; the card
  // hides both, so the copy has to as well.
  ['苹果词典的纯文本条目', {
    word: 'apple',
    text: "apple | BrE ˈapl, AmE ˈæp(ə)l | noun (fruit) 苹果; (tree) 苹果树 ▸ the apple of sb's eye figurative 掌上明珠",
  }, "BrE ˈapl, AmE ˈæp(ə)l | noun (fruit) 苹果; (tree) 苹果树\n▸ the apple of sb's eye figurative 掌上明珠"],

  // An MDX article arrives as HTML and has to paste as words.
  ['MDX 条目的标签被去掉', {
    word: 'apple',
    text: '<p>第一段</p><p>第二段<br>续行</p><script>bad()</script>',
  }, '第一段\n第二段\n续行'],

  ['结构化条目按块分行', {
    word: 'apple',
    phonetics: [{ symbol: 'ˈæp(ə)l' }],
    exchange: ['复数：apples'],
    definitions: [{ pos: 'n.', meanings: ['苹果', '苹果树'] }],
    examples: [{ source: 'He ate the apple.', translation: '他吃了苹果。' }],
  }, 'apple [ˈæp(ə)l]\n复数：apples\nn. 苹果；苹果树\nHe ate the apple.\n他吃了苹果。'],

  ['网络释义不会被丢掉', {
    word: '云端',
    web: [{ value: 'cloud', conts: ['云端', '云'] }],
  }, '云端\ncloud：云端\ncloud：云'],
]

let failures = 0
for (const [name, entry, want] of cases) {
  const got = plainEntry(entry)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(got)}\n  期望 ${JSON.stringify(want)}`)
  }
}

const all = copyAll({
  id: 'x', context: 'main', mode: 'input', sourceText: 'apple', detected: 'en',
  targetLang: 'zh-CN', elapsedMs: 1,
  translate: [
    { provider: 'google', label: 'Google', text: ' 苹果 ', detected: '', elapsedMs: 1 },
    { provider: 'youdao', label: '有道', text: '', detected: '', error: '超时', elapsedMs: 1 },
  ],
  dict: [
    { provider: 'apple', label: '苹果词典', elapsedMs: 1, entries: [{ word: 'apple', text: 'apple | noun 苹果' }] },
    { provider: 'youdao', label: '有道词典', elapsedMs: 1, entries: [] },
  ],
})
const wantAll = 'apple\n\nGoogle: 苹果\n\n苹果词典: noun 苹果'
if (all !== wantAll) {
  failures++
  console.error(`FAIL 复制全文的组合顺序\n  得到 ${JSON.stringify(all)}\n  期望 ${JSON.stringify(wantAll)}`)
}

if (failures) throw new Error(`${failures} 个复制用例未通过`)
console.log(`ok — ${cases.length + 1} 个复制用例通过`)
