// The in-app shortcut table. EasyDict's 快捷键 menu documents these keys and
// its 快捷键 settings page lets each one be recorded again; the menu item and
// the window's own key handler must agree, so both read this one mapping.

export type AppAction =
  | 'clearInput'
  | 'clearAll'
  | 'copy'
  | 'copyFirstResult'
  | 'focus'
  | 'play'
  | 'retry'
  | 'toggle'
  | 'pin'
  | 'hide'
  | 'increaseFontSize'
  | 'decreaseFontSize'
  | 'google'
  | 'eudic'
  | 'appleDic'

// Same order and keys as EasyDict's MainMenuShortcutCommand.
export const appActions: { action: AppAction; label: string }[] = [
  { action: 'clearInput', label: '清空查询内容' },
  { action: 'clearAll', label: '清空所有' },
  { action: 'copy', label: '复制文本' },
  { action: 'copyFirstResult', label: '复制第一个翻译结果' },
  { action: 'focus', label: '聚焦' },
  { action: 'play', label: '播放单词发音' },
  { action: 'retry', label: '重试' },
  { action: 'toggle', label: '交换翻译语言' },
  { action: 'pin', label: '钉住窗口' },
  { action: 'hide', label: '隐藏' },
  { action: 'increaseFontSize', label: '增大字体' },
  { action: 'decreaseFontSize', label: '减小字体' },
  { action: 'google', label: '通过 Google 搜索' },
  { action: 'eudic', label: '在欧路词典中打开' },
  { action: 'appleDic', label: '在苹果词典中打开' },
]

export const defaultAppShortcuts: Record<AppAction, string> = {
  clearInput: 'cmd+k',
  clearAll: 'cmd+shift+k',
  copy: 'cmd+shift+c',
  copyFirstResult: 'cmd+shift+j',
  focus: 'cmd+i',
  play: 'cmd+s',
  retry: 'cmd+r',
  toggle: 'cmd+t',
  pin: 'cmd+p',
  hide: 'cmd+y',
  increaseFontSize: 'cmd+=',
  decreaseFontSize: 'cmd+-',
  google: 'cmd+return',
  eudic: 'cmd+shift+return',
  appleDic: 'cmd+shift+d',
}

// appAccel reads the user's table first. A stored empty string is a deliberate
// unbinding and must stay empty, so presence — not truthiness — decides.
export function appAccel(
  cfg: { appShortcuts?: Record<string, string | undefined> | null } | null | undefined,
  action: AppAction,
): string {
  const stored = cfg?.appShortcuts?.[action]
  if (stored !== undefined) return stored
  return defaultAppShortcuts[action]
}

const aliases: Record<string, string> = {
  enter: 'return',
  escape: 'esc',
  ' ': 'space',
  arrowup: 'up',
  arrowdown: 'down',
  arrowleft: 'left',
  arrowright: 'right',
  ',': 'comma',
}

// keyOf names a pressed key the way the recorder stores it. ⇧= produces '+'
// and ⇧- produces '_'; they are the same physical keys as '=' and '-', and the
// recorder cannot store '+' at all, so both fold back onto the unshifted name.
function keyOf(e: KeyboardEvent): { name: string; producedShift: boolean } {
  const raw = e.key.toLowerCase()
  if (raw === '+' || raw === '_') {
    return { name: raw === '+' ? '=' : '-', producedShift: true }
  }
  return { name: aliases[raw] ?? raw, producedShift: false }
}

// matchesAccel reports whether a keydown is the accelerator, including exact
// modifier state: ⌘K must not fire on ⌘⇧K. An empty accelerator matches
// nothing, which is what an unbound action in the menu does.
export function matchesAccel(e: KeyboardEvent, accel: string): boolean {
  if (!accel) return false
  const parts = accel.toLowerCase().split('+').filter(Boolean)
  if (parts.length < 2) return false
  const key = parts[parts.length - 1]
  const mods = new Set(parts.slice(0, -1))

  const meta = mods.has('cmd') || mods.has('command') || mods.has('super') || mods.has('win')
  const ctrl = mods.has('ctrl')
  const alt = mods.has('alt') || mods.has('option')
  const shift = mods.has('shift')

  const { name, producedShift } = keyOf(e)
  if (name !== key) return false
  return (
    e.metaKey === meta &&
    e.ctrlKey === ctrl &&
    e.altKey === alt &&
    (e.shiftKey && !producedShift) === shift
  )
}
