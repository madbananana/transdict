// The single place the frontend talks to Go. Bindings are generated, so this
// module only re-exports them and adds the parts every window needs: role
// filtering (Wails emits events to all windows), audio playback, and a few
// display helpers.
import { useEffect, useState } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import { applyLocale, t } from './i18n'
import { Service, SettingsService } from '../bindings/transdict/internal/gui/index.js'
import { AppCommand, AppTheme, AppUiLang, QueryBusy, QueryResult } from '../bindings/events/index.js'

import type { Busy } from '../bindings/transdict/internal/gui/models.js'
import type { Response } from '../bindings/transdict/internal/query/models.js'
import type { Config, Field, Label, Provider, ServiceType } from '../bindings/transdict/internal/settings/models.js'
import type { Entry, Result as DictResult } from '../bindings/transdict/internal/dict/models.js'
import type { Outcome as OCROutcome } from '../bindings/transdict/internal/ocr/models.js'
import type { Result as TranslateResult } from '../bindings/transdict/internal/translate/models.js'
import type { Lang } from '../bindings/transdict/internal/langs/models.js'

export { Service, SettingsService }
export type {
  Busy, Config, DictResult, Entry, Field, Label, Lang, OCROutcome, Provider, Response, ServiceType,
  TranslateResult,
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

export type Role = 'main' | 'float' | 'mini' | 'capture' | 'settings'

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

/** roleOf mirrors gui.target(): which window a query context answers in. */
export function roleOf(context: string): Role {
  if (context === 'floating' || context === 'odict') return 'float'
  if (context === 'mini') return 'mini'
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

// A single element reused for every read-aloud: creating one per click leaves
// the previous voice playing underneath the new one.
let clip: HTMLAudioElement | null = null

export async function speak(text: string, lang: string, slow = false): Promise<string> {
  if (!text.trim()) return ''
  const result = await Service.Speak(text, lang, slow)
  const source = result?.clip?.url
  if (!source || result.error) {
    return result.error || '没有可用的朗读服务'
  }
  if (!clip) clip = new Audio()
  clip.src = source
  clip.playbackRate = slow ? 0.7 : 1
  clip.onerror = null
  await clip.play().catch(() => undefined)
  return ''
}

export function stopSpeech() {
  if (clip) {
    clip.pause()
    clip.src = ''
  }
}

/** readAloud speaks and reports what stopped it. A missing voice used to fail in
 * silence, which reads as a broken button rather than a service nobody configured. */
export function readAloud(text: string, lang: string, slow = false, notice?: (msg: string) => void) {
  return speak(text, lang, slow).then((message) => {
    if (message && notice) notice(t(message))
  })
}

/** openURL hands a link to the browser. An anchor inside a model's answer would
 * otherwise navigate the window away from the result the user is still reading. */
export function openURL(url: string) {
  if (!/^https?:\/\//i.test(url)) return
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
  return new Date(unixSeconds * 1000).toLocaleDateString()
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
