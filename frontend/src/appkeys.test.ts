// Run with `npm --prefix frontend test`. The table decides which keys a query
// window answers, so it is worth pinning: a matcher that fires on ⌘K for ⌘⇧K
// would clear the box while the user asked to clear the answer.
import { appAccel, defaultAppShortcuts, matchesAccel } from './appkeys.ts'

type Press = {
  key: string
  metaKey?: boolean
  ctrlKey?: boolean
  altKey?: boolean
  shiftKey?: boolean
}

function press(k: Press): KeyboardEvent {
  return {
    key: k.key,
    metaKey: !!k.metaKey,
    ctrlKey: !!k.ctrlKey,
    altKey: !!k.altKey,
    shiftKey: !!k.shiftKey,
  } as KeyboardEvent
}

const cases: [string, Press, string, boolean][] = [
  ['默认 ⌘K 清空输入', { key: 'k', metaKey: true }, 'cmd+k', true],
  ['⌘⇧K 不是 ⌘K', { key: 'k', metaKey: true, shiftKey: true }, 'cmd+k', false],
  ['⌘⇧K 清空所有', { key: 'k', metaKey: true, shiftKey: true }, 'cmd+shift+k', true],
  ['Enter 记为 return', { key: 'Enter', metaKey: true }, 'cmd+return', true],
  ['⌘↩ 不是 ⇧↩', { key: 'Enter', metaKey: true }, 'cmd+shift+return', false],
  ['⇧↩ 打开欧路词典', { key: 'Enter', metaKey: true, shiftKey: true }, 'cmd+shift+return', true],
  ['⌘= 增大字体', { key: '=', metaKey: true }, 'cmd+=', true],
  ['⌘⇧= 不算 ⌘=', { key: '=', metaKey: true, shiftKey: true }, 'cmd+=', false],
  ['⌘+ 与 ⌘= 同一个键', { key: '+', metaKey: true, shiftKey: true }, 'cmd+=', true],
  ['⌘- 减小字体', { key: '-', metaKey: true }, 'cmd+-', true],
  ['⌘_ 与 ⌘- 同一个键', { key: '_', metaKey: true, shiftKey: true }, 'cmd+-', true],
  ['Ctrl 不等于 ⌘', { key: 'k', ctrlKey: true }, 'cmd+k', false],
  ['ctrl+alt+a 只认 ctrl', { key: 'a', ctrlKey: true, altKey: true }, 'ctrl+alt+a', true],
  ['多余的 Option 不匹配', { key: 'k', metaKey: true, altKey: true }, 'cmd+k', false],
  ['空快捷键不匹配', { key: 'k', metaKey: true }, '', false],
]

let failures = 0
for (const [name, k, accel, want] of cases) {
  const got = matchesAccel(press(k), accel)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}: 得到 ${got}，期望 ${want}`)
  }
}

const accelCases: [string, Record<string, string> | undefined, string][] = [
  ['未设置时用默认', undefined, defaultAppShortcuts.retry],
  ['设置后用用户的键', { retry: 'alt+r' }, 'alt+r'],
  ['空串是解绑而不是默认', { retry: '' }, ''],
]
for (const [name, table, want] of accelCases) {
  const got = appAccel({ appShortcuts: table }, 'retry')
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}: 得到 ${got}，期望 ${want}`)
  }
}

if (failures) {
  throw new Error(`${failures} 个快捷键用例未通过`)
}
console.log(`appkeys: ${cases.length + accelCases.length} 个用例通过`)
