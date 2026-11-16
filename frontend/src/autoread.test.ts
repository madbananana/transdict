// Run with `npm --prefix frontend test`. 自动朗读 is a pure decision, so the
// cases that used to depend on which window happened to call it are checked here.
import { autoReadAnswer } from './autoread.ts'

type Case = [
  string,
  Parameters<typeof autoReadAnswer>[0],
  string,
  unknown,
  Parameters<typeof autoReadAnswer>[2]?,
]

const cases: Case[] = [
  ['不自动朗读', { sourceText: 'hello', translate: [{ text: '你好' }] }, 'off', null],
  ['未设置时什么都不读', { sourceText: 'hello', translate: [{ text: '你好' }] }, '', null],
  // A partial answer carries the recognized text but no translation yet.
  ['阶段结果不朗读', { partial: true, sourceText: 'hello', translate: [] }, 'target', null],
  // 静音 OCR exists so nothing is heard; the voice setting must not override it.
  ['静音 OCR 不朗读', { context: 'silentOcr', sourceText: 'hello' }, 'source', null],

  ['朗读原文用识别出的语言', { sourceText: ' hello ', detected: 'en' }, 'source',
    { text: 'hello', lang: 'en' }],
  ['原文为空的源语言用自动检测', { sourceText: 'hello' }, 'source',
    { text: 'hello', lang: 'auto' }],
  ['没有原文可读时返回空', { translate: [{ text: '你好' }] }, 'source', null],

  ['朗读译文用目标语言', { targetLang: 'zh-CN', translate: [{ text: '你好' }] }, 'target',
    { text: '你好', lang: 'zh-CN' }],
  ['跳过报错和空译文', {
    targetLang: 'zh-CN',
    translate: [{ error: '超时' }, { text: '' }, { text: '你好' }],
  }, 'target', { text: '你好', lang: 'zh-CN' }],
  ['译文缺少语言时退回识别语言', { detected: 'en', translate: [{ text: '你好' }] }, 'target',
    { text: '你好', lang: 'en' }],
  ['没有任何译文时返回空', { targetLang: 'zh-CN', translate: [{ error: '超时' }] }, 'target', null],

  // 查询英语单词后自动播放发音: only a single English word, and only while
  // 自动朗读 itself is off, so the two switches never speak over each other.
  ['英语单词自动发音', { sourceText: ' hello ', detected: 'en' }, 'off',
    { text: 'hello', lang: 'en' }, { pronounceEnglishWord: true }],
  ['带撇号或连字符的单词', { sourceText: "don't", detected: 'en' }, 'off',
    { text: "don't", lang: 'en' }, { pronounceEnglishWord: true }],
  ['句子不按单词发音', { sourceText: 'hello world', detected: 'en' }, 'off', null,
    { pronounceEnglishWord: true }],
  ['中文不按单词发音', { sourceText: '你好', detected: 'zh-CN' }, 'off', null,
    { pronounceEnglishWord: true }],
  ['数字不按单词发音', { sourceText: '42', detected: 'en' }, 'off', null,
    { pronounceEnglishWord: true }],
  ['开关关闭时不发音', { sourceText: 'hello', detected: 'en' }, 'off', null],
  ['自动朗读优先于单词发音', { sourceText: 'hello', detected: 'en', targetLang: 'zh-CN',
    translate: [{ text: '你好' }] }, 'target', { text: '你好', lang: 'zh-CN' },
    { pronounceEnglishWord: true }],
]

let failures = 0
for (const [name, res, mode, want, opts] of cases) {
  const got = autoReadAnswer(res, mode, opts)
  if (JSON.stringify(got) !== JSON.stringify(want)) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(got)}\n  期望 ${JSON.stringify(want)}`)
  }
}

if (failures) throw new Error(`${failures} 个自动朗读用例未通过`)
console.log(`ok — ${cases.length} 个自动朗读用例通过`)
