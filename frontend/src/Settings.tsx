// The whole preferences surface: which services exist, which ones run in which
// window, which keys fire them, and how the app looks. Every list on this page
// is read from Go, so a new provider, shortcut slot or query context appears
// here without a frontend edit — the page cannot offer what the code cannot do.
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import {
  Service, SettingsService, applyTheme, errMsg, isMac, prettyShortcut, rememberView, restoreView,
  type Config, type Field, type Label, type Lang, type Provider, type ServiceType,
} from './api'
import { applyLocale, t } from './i18n'
import { ConfirmButton } from './Confirm'

type Page = 'services' | 'windows' | 'shortcuts' | 'general'

// Changing the interface language reloads every window, because the string tables
// are built when the bundle loads. The page survives that through storage so the
// settings pane does not throw the user back to the first tab mid-edit.
const PAGE_KEY = 'transdict.settingsPage'

const pages: { id: Page; name: string; note: string }[] = [
  { id: 'services', name: t('服务'), note: t('每个分类可启用多个服务，结果并列显示') },
  { id: 'windows', name: t('窗口'), note: t('勾选每个窗口实际调用哪些服务') },
  { id: 'shortcuts', name: t('快捷键'), note: t('全局生效，无需切换回应用') },
  { id: 'general', name: t('通用'), note: t('语言、外观与行为') },
]

// listOf / fieldOf / setField mirror settings.Provider.Get and Set: the UI
// addresses a value by the key its ServiceType declares, so no table of kinds
// and their fields has to be kept here.
function listOf(cfg: Config, category: string): Provider[] {
  switch (category) {
    case 'ocr': return cfg.ocr ?? []
    case 'translate': return cfg.translate ?? []
    case 'dict': return cfg.dict ?? []
    case 'tts': return cfg.tts ?? []
  }
  return []
}

const direct: Record<string, keyof Provider> = {
  apiKey: 'apiKey',
  secret: 'secret',
  appId: 'appId',
  baseUrl: 'baseUrl',
  model: 'model',
  prompt: 'prompt',
}

function fieldOf(p: Provider, key: string): string {
  const name = direct[key]
  if (name) return (p[name] as string | undefined) ?? ''
  return p.extra?.[key] ?? ''
}

function setField(p: Provider, key: string, value: string): Provider {
  if (direct[key]) return { ...p, [key]: value } as Provider
  return { ...p, extra: { ...(p.extra ?? {}), [key]: value } }
}

// usable mirrors settings.Provider.IsReady, so the dot the user sees means the
// same thing as the gate in the query planner.
function usable(p: Provider, typ?: ServiceType): boolean {
  if (!typ) return true
  return (typ.fields ?? []).every((f) => !f.required || fieldOf(p, f.key) !== '')
}

export default function Settings({ onSaved }: { onSaved: (cfg: Config) => void }) {
  const [cfg, setCfg] = useState<Config | null>(null)
  const [cats, setCats] = useState<Label[]>([])
  const [slots, setSlots] = useState<Label[]>([])
  const [contexts, setContexts] = useState<Label[]>([])
  const [page, setPage] = useState<Page>(() =>
    restoreView(PAGE_KEY, pages.map((p) => p.id), 'services') as Page)
  const [status, setStatus] = useState('')

  useEffect(() => rememberView(PAGE_KEY, page), [page])

  // A stored page id survives a reload, but the pane's scroll offset does not
  // belong to the next page: landing halfway down 通用 because 服务 was long
  // looks like the tab is missing its heading.
  const pane = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    pane.current?.scrollTo({ top: 0 })
  }, [page])

  // keep adopts a config the store just wrote: the panel's copy and this one
  // must not disagree about what is enabled.
  const keep = useCallback((next: Config) => {
    setCfg(next)
    applyTheme(next.theme)
    applyLocale(next.uiLang)
    onSaved(next)
  }, [onSaved])

  useEffect(() => {
    void Promise.all([
      SettingsService.Get(),
      SettingsService.Categories(),
      SettingsService.Slots(),
      SettingsService.Contexts(),
    ])
      .then(([c, cs, ss, xs]) => {
        setCfg(c)
        applyLocale(c.uiLang)
        setCats(cs ?? [])
        setSlots(ss ?? [])
        setContexts(xs ?? [])
      })
      .catch((e) => setStatus(errMsg(e)))
  }, [onSaved])

  // run wraps one call, reloads, and turns a Go error into a visible sentence.
  // after says what the call produced — the path a file panel chose — and is
  // written once the reload is done, so the sentence outlives the refresh.
  const run = useCallback((label: string, call: () => Promise<unknown>, after?: (path: string) => string) => {
    setStatus(label)
    void call()
      .then((v) => after?.(typeof v === 'string' ? v : '') ?? '')
      .then((msg) => SettingsService.Get().then((fresh) => {
        keep(fresh)
        setStatus(msg)
      }))
      .catch((e) => setStatus(errMsg(e)))
      .catch(() => undefined)
  }, [keep])

  if (!cfg) {
    return <div className="settings"><div className="body muted">{t('读取配置…')}</div></div>
  }

  return (
    <div className="settings">
      <div className="spine">
        {pages.map((p) => (
          <button key={p.id} className={page === p.id ? 'active' : ''} onClick={() => setPage(p.id)}>
            <PageIcon id={p.id} />
            {p.name}
          </button>
        ))}
      </div>

      <div className="spane" ref={pane}>
        <div className="shead">
          <div>
            <h2>{pages.find((p) => p.id === page)?.name}</h2>
            <span className="byline">{pages.find((p) => p.id === page)?.note}</span>
          </div>
          <div className="spacer" />
          {status && <span className="error">{status}</span>}
        </div>

        {page === 'services' && <Services cfg={cfg} cats={cats} run={run} />}
        {page === 'windows' && <Windows cfg={cfg} cats={cats} contexts={contexts} run={run} setStatus={setStatus} />}
        {page === 'shortcuts' && <Shortcuts cfg={cfg} slots={slots} run={run} setStatus={setStatus} />}
        {page === 'general' && <General cfg={cfg} run={run} setStatus={setStatus} />}
      </div>
    </div>
  )
}

function Services({ cfg, cats, run }: {
  cfg: Config
  cats: Label[]
  run: (label: string, call: () => Promise<unknown>) => void
}) {
  const [cat, setCat] = useState(cats[0]?.key ?? 'dict')
  const [types, setTypes] = useState<Record<string, ServiceType[]>>({})
  const [open, setOpen] = useState('')
  const [pick, setPick] = useState('')

  const category = cats.find((c) => c.key === cat) ? cat : cats[0]?.key ?? 'dict'

  useEffect(() => {
    if (types[category]) return
    void SettingsService.ServiceTypes(category)
      .then((ts) => setTypes((prev) => ({ ...prev, [category]: ts ?? [] })))
      .catch(() => undefined)
  }, [category, types])

  const of = (kind: string) => types[category]?.find((s) => s.kind === kind)
  const list = listOf(cfg, category)
  // A single-instance service cannot be added twice, so the picker hides it
  // rather than offering a row the backend would reject.
  const addable = (types[category] ?? []).filter(
    (s) => s.multi || !list.some((p) => p.kind === s.kind),
  )

  const move = (index: number, delta: number) => {
    const next = [...list]
    const to = index + delta
    if (to < 0 || to >= next.length) return
    ;[next[index], next[to]] = [next[to], next[index]]
    run(t('排序…'), () => saveList(category, next))
  }

  return (
    <div className="swrap">
      <div className="snav">
        {cats.map((c) => (
          <button
            key={c.key}
            className={category === c.key ? 'active' : ''}
            onClick={() => { setCat(c.key); setOpen('') }}
          >
            {c.name}
            <em>{listOf(cfg, c.key).filter((p) => p.enabled).length}</em>
          </button>
        ))}
      </div>

      <div className="slist">
        {list.length === 0 && <div className="muted">{t('此分类还没有服务，从下方添加。')}</div>}
        {list.map((p, i) => {
          const meta = of(p.kind)
          const ok = usable(p, meta)
          return (
            <div className={`srow${open === p.id ? ' open' : ''}`} key={p.id}>
              <div className="srowtop">
                <input
                  type="checkbox"
                  checked={p.enabled}
                  aria-label={t('启用 {name}', { name: p.label })}
                  onChange={(e) => run(t('保存…'), () => SettingsService.UpdateProvider({ ...p, enabled: e.target.checked }))}
                />
                <span className={`dot${p.enabled ? (ok ? '' : ' bad') : ' off'}`} />
                <strong>{p.label}</strong>
                {meta && meta.name !== p.label && <span className="kind">{meta.name}</span>}
                {meta?.free && <span className="badge">{t('免费')}</span>}
                {!ok && <span className="badge warn">{t('缺配置')}</span>}
                <div className="spacer" />
                <button className="tiny" onClick={() => move(i, -1)} disabled={i === 0}>↑</button>
                <button className="tiny" onClick={() => move(i, 1)} disabled={i === list.length - 1}>↓</button>
                <button className="tiny" onClick={() => setOpen(open === p.id ? '' : p.id)}>
                  {open === p.id ? t('收起') : t('配置')}
                </button>
                <ConfirmButton
                  className="tiny danger"
                  label={t('删除')}
                  question={t('确认删除？')}
                  title={t('删除「{name}」及其配置？', { name: p.label })}
                  onConfirm={() => run(t('删除…'), () => SettingsService.RemoveProvider(p.id))}
                />
              </div>

              {open === p.id && (
                <div className="sbody">
                  {meta?.note && <p className="noteline">{meta.note}</p>}
                  <ProviderField
                    field={{ key: 'label', label: t('显示名称'), secret: false, required: true }}
                    value={p.label}
                    run={run}
                    onSave={(v) => run(t('保存…'), () => SettingsService.UpdateProvider({ ...p, label: v }))}
                  />
                  {(meta?.fields ?? []).map((f) => (
                    <ProviderField key={f.key} field={f} value={fieldOf(p, f.key)} run={run} onSave={(v) => {
                      run(t('保存…'), () => SettingsService.UpdateProvider(setField(p, f.key, v)))
                    }} />
                  ))}
                  {!meta && <p className="noteline">{t('该类型未在本平台注册，保留配置但不会被调用。')}</p>}
                </div>
              )}
            </div>
          )
        })}

        {addable.length > 0 && (
          <div className="addrow">
            <select value={pick} onChange={(e) => setPick(e.target.value)} aria-label={t('选择服务类型')}>
              <option value="">{t('添加服务…')}</option>
              {addable.map((m) => (
                <option key={m.kind} value={m.kind}>
                  {m.name}
                  {m.free ? t('（免费）') : ''}
                </option>
              ))}
            </select>
            <button
              className="primary"
              disabled={!pick}
              onClick={() => {
                run(t('添加…'), () => SettingsService.AddProvider(category, pick))
                setPick('')
              }}
            >
              {t('添加')}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

// ProviderField owns its draft and writes on blur: a bound input that
// round-tripped to Go on every keystroke would fight the caret.
function ProviderField({ field, value, onSave, run }: {
  field: Field
  value: string
  onSave: (v: string) => void
  run: (label: string, call: () => Promise<unknown>) => void
}) {
  const [draft, setDraft] = useState(value)
  const [shown, setShown] = useState(false)
  useEffect(() => setDraft(value), [value])

  return (
    <label className="field">
      <span>
        {field.label}
        {field.required && <em className="req">{t('必填')}</em>}
      </span>
      {field.text ? (
        <textarea
          rows={4}
          value={draft}
          placeholder={field.placeholder}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => draft !== value && onSave(draft)}
        />
      ) : (
        <span className="withbtn">
          <input
            type={field.secret && !shown ? 'password' : 'text'}
            value={draft}
            placeholder={field.placeholder}
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
            onChange={(e) => setDraft(e.target.value)}
            onBlur={() => draft !== value && onSave(draft)}
          />
          {field.secret && (
            <button className="tiny" onClick={() => setShown(!shown)}>
              {shown ? t('隐藏') : t('显示')}
            </button>
          )}
          {/* The panel is the only part this page cannot do itself; saving the
              answer goes back through onSave, which is the same path as typing. */}
          {field.folder && (
            <button
              className="tiny"
              onClick={() => run(t('选择文件夹…'), async () => {
                const dir = await Service.PickDirectory()
                if (dir) onSave(dir)
              })}
            >
              {t('浏览…')}
            </button>
          )}
        </span>
      )}
    </label>
  )
}

// saveList rewrites one category through Save, which normalizes and validates.
async function saveList(category: string, list: Provider[]) {
  const fresh = await SettingsService.Get()
  const next: Config = { ...fresh }
  if (category === 'ocr') next.ocr = list
  if (category === 'translate') next.translate = list
  if (category === 'dict') next.dict = list
  if (category === 'tts') next.tts = list
  await SettingsService.Save(next)
}

function Windows({ cfg, cats, contexts, run, setStatus }: {
  cfg: Config
  cats: Label[]
  contexts: Label[]
  run: (label: string, call: () => Promise<unknown>) => void
  setStatus: (msg: string) => void
}) {
  const [byContext, setByContext] = useState<Record<string, Label[]>>({})
  const [active, setActive] = useState(contexts[0]?.key ?? 'main')

  useEffect(() => {
    void Promise.all(
      contexts.map(async (c) => [c.key, (await SettingsService.CategoriesFor(c.key)) ?? []] as const),
    )
      .then(Object.fromEntries)
      .then(setByContext)
      .catch(() => undefined)
  }, [contexts])

  const context = contexts.some((c) => c.key === active) ? active : contexts[0]?.key ?? 'main'
  const selected = cfg.selections?.[context] ?? []
  const stages = byContext[context] ?? []

  // The stored list is an *exclusion* list: an empty selection means "run
  // everything", and a stage with no picked service also runs everything. The
  // chips therefore show on for services that will run even when the store has
  // no entry for them; otherwise a window that runs 朗读 would draw them gray.
  const explicit = selected.length > 0
  const enabledIds = (key: string) => listOf(cfg, key).filter((p) => p.enabled).map((p) => p.id)
  const contextIds = stages.flatMap((s) => enabledIds(s.key))
  // What the window will actually run: a stage with explicit picks contributes
  // those, a stage without contributes all of its enabled services. The count
  // is the number of chips that read as on, not the length of the stored list.
  const running = stages.reduce((n, s) => {
    const ids = enabledIds(s.key)
    const picked = selected.filter((id) => ids.includes(id))
    return n + (explicit && picked.length > 0 ? picked.length : ids.length)
  }, 0)

  const toggle = (stageKey: string, p: Provider) => {
    if (!p.enabled) {
      setStatus(t('{name} 已停用，请先在「服务」页启用。', { name: p.label }))
      return
    }
    const stageIds = enabledIds(stageKey)
    const stagePicked = selected.filter((id) => stageIds.includes(id))
    let next: string[]
    if (!explicit) {
      next = contextIds.filter((id) => id !== p.id)
    } else if (stagePicked.length === 0) {
      // This stage follows the default; excluding one service means writing
      // out the rest of the stage explicitly.
      next = [...selected, ...stageIds.filter((id) => id !== p.id)]
      if (next.length === selected.length) {
        setStatus(t('每个阶段至少保留一个服务；要完全停用请到「服务」页。'))
        return
      }
    } else if (selected.includes(p.id)) {
      if (stagePicked.length === 1) {
        setStatus(t('每个阶段至少保留一个服务；要完全停用请到「服务」页。'))
        return
      }
      next = selected.filter((id) => id !== p.id)
    } else {
      next = [...selected, p.id]
    }
    run(t('保存…'), () => SettingsService.SetSelection(context, next))
  }

  return (
    <div className="swrap">
      <div className="snav">
        {contexts.map((c) => (
          <button key={c.key} className={context === c.key ? 'active' : ''} onClick={() => setActive(c.key)}>
            {c.name}
          </button>
        ))}
      </div>

      <div className="slist">
        <div className="slistbar">
          <p className="noteline">
            {explicit
              ? t('本窗口运行 {n} 个服务，按勾选顺序显示。', { n: running })
              : t('本窗口运行全部 {n} 个已启用服务；点掉一项即可排除它。', { n: running })}
            {context === 'silentOcr' && t(' 静音 OCR 只复制文字，不显示结果。')}
          </p>
          {explicit && (
            <button className="tiny ghost" onClick={() => run(t('恢复…'), () => SettingsService.SetSelection(context, []))}>
              {t('改为全部启用')}
            </button>
          )}
        </div>

        {stages.length === 0 && <div className="muted">{t('载入该窗口的服务清单…')}</div>}
        {stages.map((stage) => {
          const stageIds = enabledIds(stage.key)
          const stagePicked = selected.filter((id) => stageIds.includes(id))
          const autoStage = explicit && stagePicked.length === 0
          return (
            <div className="group" key={stage.key}>
              <h4>{stage.name}{autoStage && <em className="auto">{t('全部')}</em>}</h4>
              <div className="chips">
                {order(listOf(cfg, stage.key), selected).map((p) => {
                  const on = p.enabled && (!explicit || autoStage || selected.includes(p.id))
                  return (
                    <button
                      key={p.id}
                      className={`chip${on ? ' on' : ''}${p.enabled ? '' : ' off'}`}
                      onClick={() => toggle(stage.key, p)}
                      title={p.enabled ? p.label : t('{name}（已停用）', { name: p.label })}
                    >
                      {p.label}
                    </button>
                  )
                })}
                {listOf(cfg, stage.key).length === 0 && (
                  <span className="muted">{t('还没有 {stage} 服务', { stage: stage.name })}</span>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// order lists selected providers first, in the order the planner will use, then
// the rest — so the chip row reads like the result column.
function order(list: Provider[], selected: string[]): Provider[] {
  const by = new Map(list.map((p) => [p.id, p]))
  const picked = selected.map((id) => by.get(id)).filter((p): p is Provider => !!p)
  for (const p of picked) by.delete(p.id)
  return [...picked, ...by.values()]
}

const namedKeys: Record<string, string> = {
  ' ': 'space',
  arrowup: 'up',
  arrowdown: 'down',
  arrowleft: 'left',
  arrowright: 'right',
  enter: 'return',
  escape: 'esc',
  ',': 'comma',
}

// accelOf turns a keydown into the lowercase accelerator Go stores. A press
// with no modifier or an unnameable key returns '', which the UI reports rather
// than binding a keystroke that would fire in every app.
function accelOf(e: KeyboardEvent): string {
  const mods: string[] = []
  if (e.ctrlKey) mods.push('ctrl')
  if (e.altKey) mods.push('alt')
  if (e.shiftKey) mods.push('shift')
  if (e.metaKey) mods.push(isMac ? 'cmd' : 'win')
  const key = e.key.toLowerCase()
  const name = namedKeys[key] ?? (nameable(key) ? key : '')
  if (!mods.length || !name) return ''
  return [...mods, name].join('+')
}

function nameable(key: string): boolean {
  return /^[a-z0-9]$/.test(key) || /^f([1-9]|1[0-2])$/.test(key) || /^[/=;.'`\[\]-]$/.test(key)
}

function Shortcuts({ cfg, slots, run, setStatus }: {
  cfg: Config
  slots: Label[]
  run: (label: string, call: () => Promise<unknown>) => void
  setStatus: (s: string) => void
}) {
  const [rec, setRec] = useState('')
  // A key another app already owns fails at registration, not when the panel
  // opens; without this list the press would reach that app and this one would
  // look broken. Both the recorder and the launch-time refusal end up here.
  const [dead, setDead] = useState<{ accel: string; err: string }[]>([])

  const refreshDead = useCallback(() => {
    void Service.ShortcutProblems().then((p) => setDead(p ?? [])).catch(() => setDead([]))
  }, [])

  useEffect(refreshDead, [refreshDead])

  const rebind = (slot: string, accel: string, label = t('绑定…')) =>
    run(label, async () => {
      await SettingsService.SetShortcut(slot, accel)
      // The store's watcher rebinds before Update returns, so the list is the
      // answer to this keystroke rather than to the previous one.
      refreshDead()
    })

  useEffect(() => {
    if (!rec) return
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === 'Escape') {
        setRec('')
        setStatus('')
        return
      }
      const accel = accelOf(e)
      if (!accel) {
        setStatus(t('至少需要一个修饰键（⌘ ⌥ ⇧ Ctrl）'))
        return
      }
      rebind(rec, accel)
      setRec('')
    }
    // capture, so a recording press never reaches the app's own handlers.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [rec, run, setStatus, refreshDead])

  if (slots.length === 0) return <div className="muted">{t('载入快捷键…')}</div>

  return (
    <div className="slist">
      <p className="noteline">
        {t('点击「录制」后按下组合键，Esc 取消。快捷键在应用未聚焦时同样有效；若与其他程序冲突，会在保存后提示。')}
      </p>
      {dead.length > 0 && (
        <div className="deadkeys">
          {dead.map((p, i) => (
            <div className="error" key={`${p.accel}-${i}`}>
              {p.accel
                ? t('{key} 注册失败，可能已被其他程序占用：{err}', { key: prettyShortcut(p.accel), err: p.err })
                : t('清除旧快捷键失败：{err}', { err: p.err })}
            </div>
          ))}
        </div>
      )}
      {slots.map((s) => {
        const accels = cfg.shortcuts?.[s.key] ?? []
        return (
          <div className={`krow${rec === s.key ? ' rec' : ''}`} key={s.key}>
            <span className="label">{s.name}</span>
            <div className="spacer" />
            <div className="keys inline">
              {accels.length === 0
                ? <kbd className="off">{t('未设置')}</kbd>
                : accels.map((a) => <kbd key={a}>{prettyShortcut(a)}</kbd>)}
            </div>
            <button className="tiny" onClick={() => setRec(rec === s.key ? '' : s.key)}>
              {rec === s.key ? t('按组合键…') : t('录制')}
            </button>
            {accels.length > 0 && (
              <button className="tiny ghost" onClick={() => rebind(s.key, '', t('解绑…'))}>
                {t('清除')}
              </button>
            )}
            {accels.length === 0 && <button className="tiny ghost clear-slot" disabled>{t('清除')}</button>}
          </div>
        )
      })}
    </div>
  )
}

const themes = [
  { key: 'auto', name: t('跟随系统') },
  { key: 'light', name: t('浅色') },
  { key: 'dark', name: t('深色') },
]

const readAloud = [
  { key: 'off', name: t('不自动朗读') },
  { key: 'source', name: t('自动朗读原文') },
  { key: 'target', name: t('自动朗读译文') },
]

const uiLangs = [
  { key: 'zh-CN', name: t('简体中文') },
  { key: 'en', name: 'English' },
]

// Zero means "keep the built-in per-stage limits"; the rest are the waits a
// slow AI provider is worth giving the room for.
const timeouts = [0, 30, 60, 120, 300]

// A hand-edited config can hold a number the list does not offer, and a select
// with no matching option shows the wrong value. Adding it keeps the page
// truthful about what is actually saved.
const choices = (current: number) =>
  timeouts.includes(current) ? timeouts : [...timeouts, current].sort((a, b) => a - b)

// Permissions names the two macOS grants the capture routes need. A missing one
// otherwise only shows up as "划词翻译 did nothing", and the checkbox lives two
// panes deep in System Settings.
function Permissions() {
  const [state, setState] = useState<{ screenRecording: boolean; accessibility: boolean } | null>(null)
  useEffect(() => {
    // The grant can change while this window is open; coming back from System
    // Settings fires focus, which is the moment to re-read it.
    const load = () => void Service.Permissions().then(setState).catch(() => undefined)
    load()
    window.addEventListener('focus', load)
    return () => window.removeEventListener('focus', load)
  }, [])

  const row = (name: string, ok: boolean | undefined, pane: string) => (
    <div className="actionrow" key={pane}>
      <span className={`dot${ok === false ? ' bad' : ok ? '' : ' off'}`} />
      <span className="byline">{name}</span>
      <span className={ok === false ? 'miss' : 'byline'}>{ok ? t('已授权') : t('未授权')}</span>
      <button className="tiny" onClick={() => void Service.OpenPermission(pane)}>
        {t('打开系统设置')}
      </button>
    </div>
  )

  return (
    <div className="group">
      <h4>{t('权限')}</h4>
      {row(t('屏幕录制'), state?.screenRecording, 'screen')}
      {row(t('辅助功能'), state?.accessibility, 'access')}
      <p className="noteline withaction">
        <span>{t('划词、取词与截图依赖这两项授权；在系统设置里改完，需要重启 Transdict 才生效。')}</span>
        {state && (!state.screenRecording || !state.accessibility) && (
          <button className="tiny" onClick={() => void Service.Restart()}>{t('重启 Transdict')}</button>
        )}
      </p>
    </div>
  )
}

function General({ cfg, run, setStatus }: {
  cfg: Config
  run: (label: string, call: () => Promise<unknown>, after?: (path: string) => string) => void
  setStatus: (text: string) => void
}) {
  const [langs, setLangs] = useState<Lang[]>([])
  const [checking, setChecking] = useState(false)
  const [version, setVersion] = useState('')
  useEffect(() => {
    void SettingsService.Languages().then((ls) => setLangs(ls ?? []))
    void SettingsService.Version().then((v) => setVersion(v ?? ''))
  }, [])

  // Scalars go through Save, which validates and normalizes. Reading the store
  // again first means a service edited in the other window is not rolled back
  // by this page's older snapshot.
  const patch = (change: Partial<Config>) =>
    run(t('保存…'), async () => {
      const fresh = await SettingsService.Get()
      await SettingsService.Save({ ...fresh, ...change } as Config)
    })

  // A check writes nothing, so it goes around run: the answer belongs in the
  // status line and a config reload would only clear it.
  const checkUpdate = () => {
    setChecking(true)
    setStatus(t('检查更新…'))
    void Service.CheckUpdate()
      .then((r) => setStatus(r?.version
        ? t('发现新版本 {v}（当前 {c}）', { v: r.version, c: r.current })
        : t('已是最新版本 {v}', { v: r?.current ?? '' })))
      .catch((e) => setStatus(errMsg(e)))
      .finally(() => setChecking(false))
  }

  const choose = (key: keyof Config, name: string, options: { key: string; name: string }[]) => (
    <label className="field" key={key}>
      <span>{name}</span>
      <select
        value={(cfg[key] as string | number | undefined) ?? ''}
        onChange={(e) => patch({ [key]: e.target.value } as Partial<Config>)}
      >
        {options.map((o) => (
          <option key={o.key} value={o.key}>{o.name}</option>
        ))}
      </select>
    </label>
  )

  const toggle = (key: keyof Config, name: string, note: string) => (
    <label className="switch" key={key}>
      <input
        type="checkbox"
        checked={!!cfg[key]}
        aria-label={name}
        onChange={(e) => patch({ [key]: e.target.checked } as Partial<Config>)}
      />
      <span>
        <strong>{name}</strong>
        <em>{note}</em>
      </span>
    </label>
  )

  return (
    <div className="slist">
      <div className="group">
        <h4>{t('语言')}</h4>
        <div className="fields">
          {choose('sourceLang', t('原文语言'), [{ key: 'auto', name: t('自动检测') }, ...langs.filter((l) => l.code !== 'auto')
            .map((l) => ({ key: l.code, name: l.name }))])}
          {choose('targetLang', t('译文语言'), langs.filter((l) => l.code !== 'auto')
            .map((l) => ({ key: l.code, name: l.name })))}
          {choose('uiLang', t('界面语言'), uiLangs)}
        </div>
      </div>

      <div className="group">
        <h4>{t('外观')}</h4>
        <div className="fields">{choose('theme', t('主题'), themes)}</div>
        <label className="field slider-field">
          <span>{t('迷你窗字号')}</span>
          <input
            type="range"
            min={12}
            max={28}
            value={cfg.miniFont}
            onChange={(e) => patch({ miniFont: Number(e.target.value) })}
          />
          <span className="slider-val">{cfg.miniFont}</span>
        </label>
      </div>

      <div className="group">
        <h4>{t('行为')}</h4>
        {toggle('loginItem', t('开机自动启动'),
          isMac ? t('需要 macOS 13 以上，且应用已签名安装到 /Applications') : t('写入当前用户的启动项'))}
        {toggle('showInDock',
          isMac ? t('在程序坞中显示图标') : t('在任务栏中显示图标'),
          isMac ? t('关闭后只保留菜单栏入口，主窗口仍由快捷键唤起')
            : t('任务栏按钮属于窗口，改动在下次打开主窗口时生效'))}
        {toggle('floating', t('划词显示悬浮窗'), t('关闭后划词与取词的结果进入主窗口'))}
        {toggle('keepOpen', t('悬浮窗保持打开'), t('关闭后悬浮窗会在几秒后自动收起'))}
        <label className={`field slider-field${cfg.keepOpen ? ' off' : ''}`}>
          <span>{t('悬浮窗自动收起')}</span>
          <input
            type="range"
            min={5}
            max={60}
            value={cfg.floatClose > 0 ? cfg.floatClose : 12}
            disabled={cfg.keepOpen}
            onChange={(e) => patch({ floatClose: Number(e.target.value) })}
          />
          <span className="slider-val">{t('{n} 秒', { n: cfg.floatClose > 0 ? cfg.floatClose : 12 })}</span>
        </label>
        {toggle('mouseSelect', t('自动监听鼠标划词'), t('选中文字即翻译，无需按快捷键（仅 macOS）'))}
        {toggle('hideSameLang', t('隐藏与原文相同的译文'), t('源语言与目标语言相同时不再重复显示'))}
        {toggle('autoCopy', t('自动复制译文'), t('结果一出现就把第一条译文写入剪贴板；静音 OCR 与翻译替换按各自的方式处理。'))}
        <div className="fields">{choose('ttsAuto', t('自动朗读'), readAloud)}</div>
      </div>

      {isMac && <Permissions />}

      <div className="group">
        <h4>{t('网络')}</h4>
        <ProviderField
          field={{ key: 'proxy', label: t('网络代理'), secret: false, required: false,
            placeholder: 'http://127.0.0.1:7890' }}
          value={cfg.proxy ?? ''}
          run={run}
          onSave={(v) => patch({ proxy: v })}
        />
        <p className="noteline">{t('留空即跟随系统代理；只影响在线服务，苹果 OCR 与苹果词典不走这条链路。')}</p>
        <label className="field">
          <span>{t('服务超时')}</span>
          <select
            value={String(cfg.timeout ?? 0)}
            onChange={(e) => patch({ timeout: Number(e.target.value) })}
          >
            {choices(cfg.timeout ?? 0).map((s) => (
              <option key={s} value={String(s)}>
                {s === 0 ? t('内置默认') : t('{n} 秒', { n: s })}
              </option>
            ))}
          </select>
        </label>
        <p className="noteline">{t('一个服务等待结果的最长时间。默认识别两分钟、文本三分钟；接入 AI 服务时可以调高。')}</p>
      </div>

      <div className="group">
        <h4>{t('更新')}</h4>
        <ProviderField
          field={{ key: 'updateURL', label: t('更新源地址'), secret: false, required: false }}
          value={cfg.updateURL ?? ''}
          run={run}
          onSave={(v) => patch({ updateURL: v })}
        />
        <p className="noteline">{t('一个 JSON 文档，写明最新版本号与下载地址；GitHub 的 Releases API 直接可用。')}</p>
        {toggle('autoUpdate', t('启动时自动检查'), t('只在发现新版本时提示；地址留空即完全不联网。'))}
        <div className="actionrow">
          <span className="byline">
            {version ? t('当前版本 {v}', { v: version }) : ''}
          </span>
          <div className="btnrow">
            <button className="tiny" disabled={checking} onClick={checkUpdate}>
              {t('检查更新')}
            </button>
          </div>
        </div>
      </div>

      <div className="group">
        <h4>{t('数据')}</h4>
        {toggle('saveHistory', t('记录查询历史'), t('关闭后新的查询不再写入历史；已有记录保留，可随时清理。'))}
        <div className="actionrow">
          <span className="byline">{t('配置保存于用户目录，删除后不可恢复。')}</span>
          <div className="btnrow">
            <button
              className="tiny"
              onClick={() => run(t('导出设置…'), () => Service.ExportConfigFile(),
                (path) => path ? t('已导出设置到 {path}', { path }) : t('已取消'))}
            >
              {t('导出设置')}
            </button>
            <ConfirmButton
              className="tiny"
              label={t('导入设置')}
              question={t('确认导入？')}
              title={t('导入会覆盖当前全部设置，继续吗？')}
              onConfirm={() => run(t('导入设置…'), () => Service.ImportConfigFile(),
                (path) => path ? t('已导入 {path}', { path }) : t('已取消'))}
            />
            <ConfirmButton
              className="danger"
              label={t('恢复默认设置')}
              question={t('确认恢复？')}
              title={t('恢复默认设置会清空所有服务与快捷键，继续吗？')}
              onConfirm={() => run(t('恢复默认…'), () => SettingsService.Reset())}
            />
          </div>
        </div>
        <p className="noteline">{t('导出的文件包含各服务的密钥，请自行保管。')}</p>
      </div>
    </div>
  )
}

// PageIcon is the sidebar's visual index: a sliders, window, keyboard and gear
// glyph. They are inline so the icon colour follows the active row.
function PageIcon({ id }: { id: Page }) {
  const paths: Record<Page, ReactNode> = {
    services: (
      <>
        <path d="M4 6h16M4 12h16M4 18h16" />
        <circle cx="9" cy="6" r="2" />
        <circle cx="15" cy="12" r="2" />
        <circle cx="7" cy="18" r="2" />
      </>
    ),
    windows: (
      <>
        <rect x="3" y="4" width="18" height="16" rx="2.5" />
        <path d="M3 9h18" />
      </>
    ),
    shortcuts: (
      <>
        <rect x="2.5" y="6" width="19" height="12" rx="2.5" />
        <path d="M6.5 10h.01M10 10h.01M13.5 10h.01M17 10h.01M7.5 14h9" />
      </>
    ),
    general: (
      <>
        <circle cx="12" cy="12" r="3.2" />
        <path d="M12 2.8v2.4M12 18.8v2.4M2.8 12h2.4M18.8 12h2.4M5.5 5.5l1.7 1.7M16.8 16.8l1.7 1.7M18.5 5.5l-1.7 1.7M7.2 16.8l-1.7 1.7" />
      </>
    ),
  }
  return (
    <svg
      className="pico"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[id]}
    </svg>
  )
}
