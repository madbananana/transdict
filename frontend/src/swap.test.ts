// Run with `npm --prefix frontend test` — plain node, because the swap plan is
// pure data: which text to ask and in which direction is decided before a
// window touches the query service.
import { firstTranslation, planSwap } from './swap.ts'
import type { Response } from './api'

function answer(over: Partial<Response> = {}): Response {
  return {
    id: 'q1', context: 'main', mode: 'input', sourceText: 'serendipity',
    detected: 'en', targetLang: 'zh-CN', elapsedMs: 12,
    translate: [{ provider: 'google', label: 'Google', text: '机缘巧合', elapsedMs: 12 }],
    ...over,
  }
}

const cases: [string, Response | null, string, string, string, ReturnType<typeof planSwap>][] = [
  ['回译用首条译文', answer(), 'serendipity', 'auto', 'zh-CN',
    { kind: 'reverse', text: '机缘巧合', from: 'zh-CN', to: 'en' }],
  ['两侧都自动时只换文本', answer(), 'serendipity', 'auto', 'auto',
    { kind: 'auto', text: '机缘巧合' }],
  ['首条服务失败只换语言对', answer({ translate: [{ provider: 'google', label: 'Google', text: '', error: '超时', elapsedMs: 9 }] }),
    'serendipity', 'auto', 'zh-CN', { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['首条还在查询中不抢文本', answer({ translate: [{ provider: 'google', label: 'Google', text: '', pending: true, elapsedMs: 0 }] }),
    'serendipity', 'auto', 'zh-CN', { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['半成品结果不拿来回译', answer({ partial: true }), 'serendipity', 'auto', 'zh-CN',
    { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['文本改过以后只换语言对', answer(), 'another word', 'auto', 'zh-CN',
    { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['检测语言就是目标语言时不回译', answer({ detected: 'zh-CN', targetLang: 'zh-CN' }),
    'serendipity', 'auto', 'zh-CN', { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['没有结果时只换语言对', null, 'serendipity', 'en', 'zh-CN',
    { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['空文本仍可换语言对', null, '   ', 'en', 'zh-CN', { kind: 'pair', from: 'en', to: 'zh-CN' }],
  ['同一种语言不交换', null, 'hi', 'en', 'en', { kind: 'none' }],
]

let failures = 0
for (const [name, res, text, source, target, want] of cases) {
  const got = planSwap(res, text, source, target)
  if (JSON.stringify(got) !== JSON.stringify(want)) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(got)}\n  期望 ${JSON.stringify(want)}`)
  }
}

const firstCases: [string, Response | null, string][] = [
  ['取首条服务的译文', answer(), '机缘巧合'],
  ['首条失败时没有回译文本', answer({ translate: [{ provider: 'g', label: 'G', text: '', error: 'x', elapsedMs: 1 }] }), ''],
  ['半条译文不算', answer({ translate: [{ provider: 'g', label: 'G', text: 'x', pending: true, elapsedMs: 1 }] }), ''],
  ['没有响应', null, ''],
]
for (const [name, res, want] of firstCases) {
  const got = firstTranslation(res)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${JSON.stringify(got)}\n  期望 ${JSON.stringify(want)}`)
  }
}

if (failures > 0) throw new Error(`${failures} 个交换用例未通过`)
console.log(`ok — ${cases.length + firstCases.length} 个交换用例通过`)
