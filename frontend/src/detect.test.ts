// Run with `npm --prefix frontend test`. resolveDetected is the pure decision
// behind 识别为: which language the chip names for the text currently in a box.
import { detectedLabel, noLanguage, resolveDetected } from './detectlang.ts'
import type { Response } from './api'

type Case = [string, string, { text: string; code: string } | null,
  { text: string; code: string }, Partial<Response> | null, string]

const cases: Case[] = [
  ['空输入不命名', '', null, noLanguage, null, ''],
  ['输入即用检测结果', 'hello', null, { text: 'hello', code: 'en' }, null, 'en'],
  ['检测为自动检测也显示', 'hello', null, { text: 'hello', code: 'auto' }, null, 'auto'],
  // A stale detection pass (or answer) about other text must not label the box.
  ['旧检测不匹配当前文本', 'world', null, { text: 'hello', code: 'en' }, null, ''],
  ['旧答案不匹配当前文本', 'world', null, noLanguage,
    { sourceText: 'hello', detected: 'en' }, ''],
  ['答案里的识别语言优先于预检', 'hello', null, { text: 'hello', code: 'en' },
    { sourceText: 'hello', detected: 'fr' }, 'fr'],
  ['答案带着用户指定的源语言', 'hello', null, noLanguage,
    { sourceText: 'hello', sourceLang: 'ja', detected: 'en' }, 'ja'],
  // specifiedTextLanguageDict outranks everything, but only for its own text.
  ['手动选择的结果优先', 'hello', { text: 'hello', code: 'ja' },
    { text: 'hello', code: 'en' }, { sourceText: 'hello', detected: 'en' }, 'ja'],
  ['手动选择不粘到别的文本', 'world', { text: 'hello', code: 'ja' },
    { text: 'world', code: 'en' }, null, 'en'],
]

let failed = 0
for (const [name, value, forced, typed, res, want] of cases) {
  const got = resolveDetected(value, forced, typed, res as Response | null)
  if (got !== want) {
    failed++
    console.error(`FAIL ${name}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`)
  }
}

const labels: [string, string, string][] = [
  ['', '', ''],
  ['en', '英语', '英语'],
  ['auto', '自动检测', '自动检测'],
  ['xx', 'xx', 'xx'],
]
for (const [code, label, want] of labels) {
  const got = detectedLabel(code, () => label)
  if (got !== want) {
    failed++
    console.error(`FAIL label ${code}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`)
  }
}

if (failed > 0) throw new Error(`${failed} 个识别为用例未通过`)
else console.log(`detect: ${cases.length + labels.length} cases passed`)
