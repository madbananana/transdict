// The single place the frontend talks to Go. Bindings are generated, so this
// module only re-exports them and adds the parts every window needs: role
// filtering (Wails emits events to all windows), audio playback, and a few
// display helpers.
import { useEffect, useState, useSyncExternalStore } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import { applyLocale, t, uiLocale } from './i18n'
import { appAccel, type AppAction } from './appkeys'
import { speechKey, stopsPlayback } from './speech'
import { Service, SettingsService } from '../bindings/transdict/internal/gui/index.js'
import { AppCommand, AppTheme, AppUiLang, QueryBusy, QueryResult } from '../bindings/events/index.js'

import type { Busy, UpdateInfo } from '../bindings/transdict/internal/gui/models.js'
import type { Response } from '../bindings/transdict/internal/query/models.js'
import type { AnkiMapping, Config, Field, Label, Provider, ServiceType } from '../bindings/transdict/internal/settings/models.js'
import type { Entry, Result as DictResult } from '../bindings/transdict/internal/dict/models.js'
import type { Outcome as OCROutcome } from '../bindings/transdict/internal/ocr/models.js'
import type { Result as TranslateResult } from '../bindings/transdict/internal/translate/models.js'
import type { Lang } from '../bindings/transdict/internal/langs/models.js'

export { Service, SettingsService }
export type {
  AnkiMapping, Busy, Config, DictResult, Entry, Field, Label, Lang, OCROutcome, Provider, Response,
  ServiceType, TranslateResult, UpdateInfo,
}

// A language change rewrites Go's side of the text and the bundle reads its own
// string tables once, at load; reloading is what carries it into every window.
// applyLocale ignores a value the window already shows, so the window that asked
// for the change is not reloaded twice.
Events.On(AppUiLang, (ev: { data: string }) => applyLocale(ev.data))

// The theme is one attribute on the document element, so an open window can
// follow it in place — the reload that a language change needs would throw away
// the result the user is still looking at.
Events.On(AppTheme, (ev: { data: string }) => applyTheme(ev.data))

export type Role = 'main' | 'capture' | 'settings'

/** rememberView keeps the view a window was showing across a reload. Changing
 * the interface language reloads every window — the string tables are built when
 * the bundle loads — and a reload that lands back on the first tab throws away
 * where the user was. Storage can be unavailable; then the default view is used. */
export function rememberView(key: string, id: string) {
  try {
    localStorage.setItem(key, id)
  } catch {
    // Nothing to do: the window simply forgets its place next time.
  }
}

export function restoreView(key: string, valid: string[], fallback: string): string {
  try {
    const id = localStorage.getItem(key)
    if (id && valid.includes(id)) return id
  } catch {
    // Same fallback as an empty store.
  }
  return fallback
}

/** roleOf names the window a query context answers in: every context lands in
 * the translation window. */
export function roleOf(_context: string): Role {
  return 'main'
}

/** onResult subscribes to queries addressed to this window role. */
export function onResult(role: Role, fn: (res: Response) => void): () => void {
  return Events.On(QueryResult, (ev: { data: Response }) => {
    if (ev.data && roleOf(ev.data.context) === role) fn(ev.data)
  })
}

export function onBusy(role: Role, fn: (text: string) => void): () => void {
  return Events.On(QueryBusy, (ev: { data: Busy }) => {
    if (ev.data && ev.data.target === role) fn(ev.data.text ?? '')
  })
}

export function onCommand(fn: (name: string) => void): () => void {
  return Events.On(AppCommand, (ev: { data: string }) => {
    if (ev.data) fn(ev.data)
  })
}

/** MenuAction is one 快捷键 menu click, addressed to a window role. */
export interface MenuAction {
  target: Role
  action: string
}

/** onMenuAction receives a 快捷键 menu click meant for this window. The event
 * name is literal like app:config: the generator skips events emitted from a
 * //wails:ignore method. */
export function onMenuAction(role: Role, fn: (action: string) => void): () => void {
  return Events.On('app:menu-action', (ev: { data: MenuAction }) => {
    if (ev.data && ev.data.target === role) fn(ev.data.action)
  })
}

// onConfig reports every saved change. A window reads 窗口配置 when it mounts,
// so without this a toggle in settings would only reach the next launch.
export function onConfig(fn: () => void): () => void {
  return Events.On('app:config', () => fn())
}

/** onUpdateChanged reports the whole state of the update window: the release,
 * its notes and the download's byte count. One event keeps the window and the
 * settings status line from disagreeing about a download they both follow. */
export function onUpdateChanged(fn: (i: UpdateInfo) => void): () => void {
  return Events.On('app:update-changed', (ev: { data: UpdateInfo }) => {
    if (ev.data) fn(ev.data)
  })
}

// A single element reused for every read-aloud: creating one per click leaves
// the previous voice playing underneath the new one.
let clip: HTMLAudioElement | null = null
// playingKey names the request the shared player is on. Speaker buttons read it
// through usePlayingKey to turn into 停止 while that clip sounds, the way
// EasyDict's EZAudioButton swaps its glyph, and ⌘S uses it to decide play or stop.
let playingKey = ''
const playingSubscribers = new Set<() => void>()

function announcePlaying(key: string) {
  if (playingKey === key) return
  playingKey = key
  for (const notify of playingSubscribers) notify()
}

/** usePlayingKey is the key of the clip currently sounding, or ''. Components
 * compare it with speechKey(...) to show the stop state on their own button. */
export function usePlayingKey(): string {
  return useSyncExternalStore(
    (notify) => {
      playingSubscribers.add(notify)
      return () => {
        playingSubscribers.delete(notify)
      }
    },
    () => playingKey,
    () => '',
  )
}

export async function speak(text: string, lang: string, slow = false): Promise<string> {
  if (!text.trim()) return ''
  const result = await Service.Speak(text, lang, slow)
  const source = result?.clip?.url
  if (!source || result.error) {
    return result.error || '没有可用的朗读服务'
  }
  if (!clip) {
    clip = new Audio()
    // A clip that runs to its end is no longer playing, so the button goes back
    // to 朗读 without the user having to press stop on silence.
    clip.addEventListener('ended', () => announcePlaying(''))
  }
  clip.src = source
  clip.playbackRate = slow ? 0.7 : 1
  clip.onerror = null
  await clip.play().catch(() => undefined)
  announcePlaying(speechKey(text, lang, slow))
  return ''
}

export function stopSpeech() {
  if (clip) {
    clip.pause()
    clip.src = ''
  }
  announcePlaying('')
}

/** readAloud speaks and reports what stopped it. A missing voice used to fail in
 * silence, which reads as a broken button rather than a service nobody configured. */
export function readAloud(text: string, lang: string, slow = false, notice?: (msg: string) => void) {
  return speak(text, lang, slow).then((message) => {
    if (message && notice) notice(t(message))
  })
}

/** toggleSpeech is what a speaker button or ⌘S presses: EasyDict's audio button
 * stops the clip it is already playing, and only then does it start a new one. */
export function toggleSpeech(text: string, lang: string, slow = false, notice?: (msg: string) => void) {
  const playing = !!clip && !clip.paused
  if (stopsPlayback(playing, playingKey, text, lang, slow)) {
    stopSpeech()
    return Promise.resolve()
  }
  return readAloud(text, lang, slow, notice)
}

/** openURL hands a link to the browser. An anchor inside a model's answer would
 * otherwise navigate the window away from the result the user is still reading. */
export function openURL(url: string) {
  if (!/^https?:\/\//i.test(url)) return
  void Browser.OpenURL(url).catch(() => undefined)
}

/** openQuickLink is the titlebar's 快捷链接: besides web links it accepts the
 * dictionary apps' own schemes (dict://, eudic://), which openURL refuses
 * because a link inside an answer has no business starting another app. Go
 * waits for the system to accept the URL, so a Mac without Eudic can say the
 * chip has nothing to open instead of doing nothing. */
export function openQuickLink(url: string): Promise<boolean> {
  if (!/^(https?:\/\/|dict:\/\/|eudic:\/\/)/i.test(url)) return Promise.resolve(false)
  return Service.OpenQuickLink(url).then(() => true, () => false)
}

/** openSystemSettings opens a macOS System Settings pane. 苹果离线翻译 points at
 * 管理下载语言, whose only correct destination is that pane. */
export function openSystemSettings(url: string) {
  if (!/^x-apple\.systempreferences:/i.test(url)) return
  void Browser.OpenURL(url).catch(() => undefined)
}

/** errMsg renders a rejected binding call. Wails wraps the Go error in a JS
 * exception, so without this the class name reads as part of the sentence:
 * "RuntimeError: 需要「辅助功能」权限…" tells the user nothing about the class. */
export function errMsg(e: unknown): string {
  const text = e instanceof Error ? e.message : String(e ?? '')
  // Go owns this wording, so it goes through the same table the rest of the
  // screen text does. An error that already carries details reads
  // "截屏失败，请检查屏幕录制权限: …" and has no exact key, so it stays Chinese.
  return t(text.replace(/^(RuntimeError|TypeError|DOMException|Error):\s*/, ''))
}

const macGlyphs: Record<string, string> = {
  cmd: '⌘', alt: '⌥', shift: '⇧', ctrl: '⌃', super: '',
}
const winLabels: Record<string, string> = {
  cmd: 'Ctrl', alt: 'Alt', shift: 'Shift', ctrl: 'Ctrl', super: 'Win',
}

export const isMac = /Mac|iPhone|iPad/.test(navigator.platform)

/** prettyShortcut renders a Wails accelerator the way the OS would. */
export function prettyShortcut(accel: string): string {
  if (!accel) return t('未设置')
  const map = isMac ? macGlyphs : winLabels
  return accel
    .split('+')
    .map((p) => map[p.trim().toLowerCase()] ?? p.toUpperCase())
    .join(isMac ? '' : '+')
}

/** accelLabel is the bare "⌘P" for inline hints; unbound actions return ''. */
export function accelLabel(
  cfg: { appShortcuts?: Record<string, string | undefined> | null } | null | undefined,
  action: AppAction,
): string {
  const accel = appAccel(cfg, action)
  return accel ? prettyShortcut(accel) : ''
}

/** accelHint is the ", ⌘P" tail tooltips append; an unbound action adds
 * nothing, because naming a key that no longer fires would be a lie. */
export function accelHint(
  cfg: { appShortcuts?: Record<string, string | undefined> | null } | null | undefined,
  action: AppAction,
): string {
  const label = accelLabel(cfg, action)
  return label ? `, ${label}` : ''
}

/** applyTheme is the one place that decides light versus dark; `auto` follows
 * the OS, which Wails already reports through the WebView's media query. */
export function applyTheme(theme: string) {
  document.documentElement.dataset.theme = theme === 'light' || theme === 'dark' ? theme : 'auto'
}

export function timeAgo(unixSeconds: number): string {
  const seconds = Math.max(0, Math.floor(Date.now() / 1000) - unixSeconds)
  if (seconds < 60) return t('刚刚')
  if (seconds < 3600) return t('{n} 分钟前', { n: Math.floor(seconds / 60) })
  if (seconds < 86400) return t('{n} 小时前', { n: Math.floor(seconds / 3600) })
  if (seconds < 86400 * 7) return t('{n} 天前', { n: Math.floor(seconds / 86400) })
  return new Date(unixSeconds * 1000).toLocaleDateString(uiLocale() === 'en' ? 'en-US' : undefined)
}

// One fetch of the language table serves every window: the codes services
// answer with ("en", "zh-CN") never change while the app runs.
let langNamesPromise: Promise<Record<string, string>> | null = null

/** useLangNames maps a service's language tag to the name the pickers show.
 * A card header that printed the raw tag read like a service name, so every
 * view resolves it through the same table the language pickers use. */
export function useLangNames(): Record<string, string> {
  const [names, setNames] = useState<Record<string, string>>({})
  useEffect(() => {
    langNamesPromise ??= SettingsService.Languages()
      .then((ls) => Object.fromEntries((ls ?? []).map((l) => [l.code, l.name])))
      .catch(() => ({}))
    let live = true
    void langNamesPromise.then((m) => {
      if (live) setNames(m)
    })
    return () => {
      live = false
    }
  }, [])
  return names
}

/** langName resolves a detected tag for display. A tag outside the table stays
 * blank rather than surfacing as something that looks like a service name. */
export function langName(code: string | undefined, names: Record<string, string>): string {
  if (!code || code === 'auto') return ''
  return names[code] ?? names[code.toLowerCase()] ?? ''
}

/** pickableLangs drops the codes a picker must not offer: 自动检测 is a
 * source-only sentinel, and 古文 is a genre the recogniser reports rather than a
 * language a service accepts. */
export function pickableLangs<T extends { code: string }>(langs: T[]): T[] {
  return langs.filter((l) => l.code !== 'auto' && l.code !== 'zh-classical')
}

// The flags are EasyDict's own table (EZLanguageModel), down to English being
// 🇬🇧 and 繁体中文 🇭🇰: the picker is a language chooser, not a locale switch.
// 自动 wears 🌐 until a query has been detected.
// Windows' emoji font draws regional indicators as two letters, so flags are
// only drawn on macOS.
const languageFlags: Record<string, string> = {
  auto: '🌐',
  'zh-CN': '🇨🇳', 'zh-TW': '🇭🇰', 'zh-classical': '📜', en: '🇬🇧', ja: '🇯🇵', ko: '🇰🇷',
  fr: '🇫🇷', de: '🇩🇪', es: '🇪🇸', ru: '🇷🇺', it: '🇮🇹', pt: '🇵🇹', 'pt-BR': '🇧🇷', th: '🇹🇭', vi: '🇻🇳',
  ar: '🇸🇦', nl: '🇳🇱', pl: '🇵🇱', tr: '🇹🇷', fa: '🇮🇷', hi: '🇮🇳', id: '🇮🇩', ms: '🇲🇾',
  sv: '🇸🇪', no: '🇳🇴', da: '🇩🇰', fi: '🇫🇮', cs: '🇨🇿', sk: '🇸🇰', hu: '🇭🇺', ro: '🇷🇴',
  bg: '🇧🇬', el: '🇬🇷', uk: '🇺🇦', he: '🇮🇱', ca: '🇪🇸', et: '🇪🇪', lv: '🇱🇻', lt: '🇱🇹',
  sl: '🇸🇮', hr: '🇭🇷', sr: '🇷🇸', mk: '🇲🇰', sq: '🇦🇱', hy: '🇦🇲', az: '🇦🇿', be: '🇧🇾',
  ka: '🇬🇪', kk: '🇰🇿', ky: '🇰🇬', lo: '🇱🇦', my: '🇲🇲', ne: '🇳🇵', mn: '🇲🇳', tl: '🇵🇭',
}

/** flagFor is the emoji a language wears in the pickers, blank on Windows and
 * for tags with no entry. */
export function flagFor(code: string | undefined): string {
  if (!isMac || !code) return ''
  return languageFlags[code] ?? languageFlags[code.toLowerCase()] ?? ''
}

/** langOption is one row of a picker: EasyDict's menus print the flag before
 * the name ("🇬🇧 English"), and the closed button keeps name then flag. */
export function langOption(code: string, name: string): string {
  const flag = flagFor(code)
  return flag ? `${flag} ${name}` : name
}

/** useWhere names the contexts and shortcut slots a result can come from. A
 * history row or a status line says where a query was made with a key Go owns,
 * so the display name — and its language — has to come from Go's tables too. */
export function useWhere(): Record<string, string> {
  const [labels, setLabels] = useState<Record<string, string>>({})
  useEffect(() => {
    void Promise.all([SettingsService.Contexts(), SettingsService.Slots()])
      .then(([cs, ss]) => {
        const map: Record<string, string> = {}
        for (const l of [...(cs ?? []), ...(ss ?? [])] as Label[]) map[l.key] = l.name
        setLabels(map)
      })
      .catch(() => setLabels({}))
  }, [])
  return labels
}
