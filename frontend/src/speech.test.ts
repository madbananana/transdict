// Run with `npm --prefix frontend test` — the play/stop rule is a pure decision,
// so EasyDict's toggle behaviour can be checked without a sound card.
import { speechKey, stopsPlayback } from './speech.ts'

const cases: [string, boolean, string, string, string, boolean, boolean][] = [
  // name, playing, playingKey, text, lang, slow, wantStop
  ['同一段正在播放时停止', true, speechKey('good', 'en', false), 'good', 'en', false, true],
  ['播放结束后再按是重新播放', false, speechKey('good', 'en', false), 'good', 'en', false, false],
  ['另一段文字打断当前播放', true, speechKey('good', 'en', false), 'better', 'en', false, false],
  ['语言不同不算同一段', true, speechKey('good', 'en', false), 'good', 'zh-CN', false, false],
  ['慢速是另一段音频', true, speechKey('good', 'en', false), 'good', 'en', true, false],
  ['慢速播放时按慢速停止', true, speechKey('good', 'en', true), 'good', 'en', true, true],
]

let failures = 0
for (const [name, playing, playingKey, text, lang, slow, want] of cases) {
  const got = stopsPlayback(playing, playingKey, text, lang, slow)
  if (got !== want) {
    failures++
    console.error(`FAIL ${name}\n  得到 ${got}\n  期望 ${want}`)
  }
}

// Keys are the identity a component would compare, so they must not collide.
if (speechKey('a|b', 'en', false) === speechKey('a', 'b|en', false)) {
  failures++
  console.error('FAIL 拼接后的键发生碰撞')
}

if (failures) throw new Error(`${failures} 个朗读切换用例未通过`)
console.log(`ok — ${cases.length} 个朗读切换用例通过`)
