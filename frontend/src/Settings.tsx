// The whole preferences surface: which services exist, which ones run in which
// window, which keys fire them, and how the app looks. Every list on this page
// is read from Go, so a new provider, shortcut slot or query context appears
// here without a frontend edit — the page cannot offer what the code cannot do.
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import {
  Service, SettingsService, applyTheme, errMsg, isMac, isWindows, langOption, onCommand, onUpdateChanged, openSystemSettings, openURL, pickableLangs, prettyShortcut, rememberView, restoreView,
  type AnkiMapping, type Config, type Field, type Label, type Lang, type Provider, type ServiceType,
} from './api'
import type { Entry } from '../bindings/transdict/internal/history/models.js'
import type { UpdateReport } from '../bindings/transdict/internal/gui/models.js'
import { applyLocale, t, uiLocale } from './i18n'
import { ConfirmButton } from './Confirm'
import { ankiNote } from './copy'
import { appActions, appAccel } from './appkeys'
import { SettingGlyph, iconOf, type IconName } from './SettingsGlyph'

type Page = 'general' | 'services' | 'favorites' | 'disabled' | 'shortcuts' | 'advanced' | 'privacy' | 'about'

// Changing the interface language reloads every window, because the string tables
// are built when the bundle loads. The page survives that through storage so the
// settings pane does not throw the user back to the first tab mid-edit.
const PAGE_KEY = 'transdict.settingsPage'

// EasyDict's tab order and labels. 收藏 and 禁止名单 are lists of their own, and
// the window-related and provider knobs live on 高级 rather than 通用, so the
// first tab stays the short "how do I query" form.
const pages: { id: Page; name: string }[] = [
  { id: 'general', name: t('通用') },
  { id: 'services', name: t('服务') },
  { id: 'favorites', name: t('收藏') },
  { id: 'disabled', name: t('禁止名单') },
  { id: 'shortcuts', name: t('快捷键') },
  { id: 'advanced', name: t('高级') },
  { id: 'privacy', name: t('隐私') },
  { id: 'about', name: t('关于') },
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

export default function Settings({ onSaved, onPage }: {
  onSaved: (cfg: Config) => void
  onPage?: (name: string) => void
}) {
  const [cfg, setCfg] = useState<Config | null>(null)
  const [cats, setCats] = useState<Label[]>([])
  const [slots, setSlots] = useState<Label[]>([])
  const [contexts, setContexts] = useState<Label[]>([])
  const [page, setPage] = useState<Page>(() =>
    restoreView(PAGE_KEY, pages.map((p) => p.id), 'general') as Page)
  const [status, setStatus] = useState('')

  // The download reports through the event bus, and the settings status line
  // follows it even while another tab is open: the window that shows the bar is
  // the update window, but a download started there should not make this page
  // look idle.
  useEffect(() => onUpdateChanged((i) => {
    if (i.state === 'downloading') {
      setStatus(i.percent > 0
        ? t('正在下载更新… {n}%', { n: i.percent })
        : t('正在下载更新…'))
    } else if (i.state === 'installing') {
      setStatus(t('正在安装新版本…'))
    } else if (i.state === 'error' && i.message) {
      setStatus(i.message)
    }
  }), [])

  useEffect(() => rememberView(PAGE_KEY, page), [page])

  // The window's caption names the open tab, as EasyDict's Settings scene does.
  useEffect(() => {
    onPage?.(pages.find((p) => p.id === page)?.name ?? '')
  }, [page, onPage])

  // 关于 in the tray opens this window on that page. The request can land before
  // the webview mounts (TakeSettingsPage) or while it is already open (command),
  // so both paths end here.
  useEffect(() => {
    const show = (id: string) => {
      if (id && pages.some((p) => p.id === id)) setPage(id as Page)
    }
    void Service.TakeSettingsPage()
      .then((id) => show(id ?? ''))
      .catch(() => undefined)
    return onCommand((name) => {
      const m = /^settings-page:(.+)$/.exec(name)
      if (m) show(m[1])
    })
  }, [])

  // A stored page id survives a reload, but the pane's scroll offset does not
  // belong to the next page: landing halfway down 通用 because 服务 was long
  // looks like the tab is missing its heading.
  const pane = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    pane.current?.scrollTo({ top: 0 })
  }, [page])

  // The window's size is fixed: a tab switch only renames it, so the frame
  // never jumps while the user is moving between pages.
  useEffect(() => {
    void Service.SetSettingsPage(page).catch(() => undefined)
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
      <div className="stabbar" role="tablist">
        {pages.map((p) => (
          <button
            key={p.id}
            role="tab"
            aria-selected={page === p.id}
            className={page === p.id ? 'active' : ''}
            onClick={() => setPage(p.id)}
          >
            <PageIcon id={p.id} />
            <span>{p.name}</span>
          </button>
        ))}
      </div>

      <div className="spane" ref={pane}>
        {status && (
          <div className="sstatus"><span className="error">{status}</span></div>
        )}

        {page === 'services' && <Services cfg={cfg} cats={cats} contexts={contexts} run={run} setStatus={setStatus} />}
        {page === 'favorites' && <Favorites setStatus={setStatus} />}
        {page === 'disabled' && <Blocklist cfg={cfg} run={run} setStatus={setStatus} />}
        {page === 'shortcuts' && <Shortcuts cfg={cfg} slots={slots} run={run} setStatus={setStatus} />}
        {page === 'general' && <General part="general" cfg={cfg} run={run} setStatus={setStatus} />}
        {page === 'advanced' && (
          <>
            <General part="advanced" cfg={cfg} run={run} setStatus={setStatus} />
            <Anki cfg={cfg} run={run} setStatus={setStatus} />
          </>
        )}
        {page === 'privacy' && <Privacy />}
        {page === 'about' && <About setStatus={setStatus} />}
      </div>
    </div>
  )
}

// Services is EasyDict's 服务 tab: a navigation list on the left, the selected
// item's form on the right, and +/− under the list to add or drop a service.
// 窗口配置 is a navigation tab of its own rather than a service: picking it turns
// the list into the window list, and picking a window shows which services that
// window runs.
const WINDOW_CONFIG = '__window_config__'

// Each window wears the glyph of what it does, so the list reads at a glance
// the way EasyDict's window picker does instead of showing seven identical
// window outlines.
const windowGlyph: Record<string, IconName> = {
  main: 'window',
  screenshot: 'ocr',
  clipboard: 'clipboard',
  silentOcr: 'bolt',
  replace: 'replace',
  odict: 'cursor',
  translation: 'font',
}

function Services({ cfg, cats, contexts, run, setStatus }: {
  cfg: Config
  cats: Label[]
  contexts: Label[]
  run: (label: string, call: () => Promise<unknown>) => void
  setStatus: (msg: string) => void
}) {
  const [nav, setNav] = useState(WINDOW_CONFIG)
  const [types, setTypes] = useState<Record<string, ServiceType[]>>({})
  const [selected, setSelected] = useState('')
  const [win, setWin] = useState(contexts[0]?.key ?? 'main')
  const [adding, setAdding] = useState(false)
  const [picked, setPicked] = useState<string[]>([])
  const drag = useRef(-1)

  const showWindow = nav === WINDOW_CONFIG
  const category = !showWindow && cats.some((c) => c.key === nav) ? nav : cats[0]?.key ?? 'dict'

  useEffect(() => {
    if (types[category]) return
    void SettingsService.ServiceTypes(category)
      .then((ts) => setTypes((prev) => ({ ...prev, [category]: ts ?? [] })))
      .catch(() => undefined)
  }, [category, types])

  const of = (kind: string) => types[category]?.find((s) => s.kind === kind)
  const list = showWindow ? [] : listOf(cfg, category)
  const sel = showWindow ? null : list.find((p) => p.id === selected) ?? null
  const activeWin = contexts.some((c) => c.key === win) ? win : contexts[0]?.key ?? 'main'
  const winName = contexts.find((c) => c.key === activeWin)?.name ?? ''

  // 缺少密钥 comes with the page that fixes it: the site EasyDict's
  // insufficient_quota_prompt names. A keyless kind or a custom host has none.
  const [help, setHelp] = useState<{ title: string; url: string } | null>(null)
  useEffect(() => {
    setHelp(null)
    if (!sel) return
    let live = true
    void SettingsService.ServiceHelp(sel.kind, sel.baseUrl ?? '')
      .then((h) => { if (live) setHelp(h && h.url ? h : null) })
      .catch(() => undefined)
    return () => { live = false }
  }, [sel?.kind, sel?.baseUrl, category])
  // A single-instance service cannot be added twice, so the sheet hides it
  // rather than offering a row the backend would reject.
  const addable = (types[category] ?? []).filter(
    (s) => s.multi || !list.some((p) => p.kind === s.kind),
  )

  const reorder = (from: number, to: number) => {
    if (from === to || from < 0 || to < 0 || from >= list.length || to >= list.length) return
    const next = [...list]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item)
    run(t('排序…'), () => saveList(category, next))
  }

  const addPicked = () => {
    const kinds = [...picked]
    setPicked([])
    setAdding(false)
    run(t('添加…'), async () => {
      for (const kind of kinds) await SettingsService.AddProvider(category, kind)
    })
  }

  return (
    <div className="svcwrap">
      <div className="svcside">
        <div className="segbar" role="tablist" aria-label={t('服务与窗口')}>
          <button
            role="tab"
            aria-selected={showWindow}
            className={showWindow ? 'active' : ''}
            onClick={() => setNav(WINDOW_CONFIG)}
          >
            {t('窗口配置')}
          </button>
          {cats.map((c) => (
            <button
              key={c.key}
              role="tab"
              aria-selected={!showWindow && category === c.key}
              className={!showWindow && category === c.key ? 'active' : ''}
              onClick={() => setNav(c.key)}
            >
              {c.name}
            </button>
          ))}
        </div>

        <div className="svcrows" role="listbox" aria-label={showWindow ? t('窗口') : t('服务列表')}>
          {showWindow && contexts.map((c) => (
            <div
              key={c.key}
              role="option"
              aria-selected={activeWin === c.key}
              tabIndex={0}
              className={`svcrow${activeWin === c.key ? ' on' : ''}`}
              onClick={(e) => { e.currentTarget.focus(); setWin(c.key) }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setWin(c.key) }
              }}
            >
              <SettingGlyph name={windowGlyph[c.key] ?? 'window'} />
              <strong>{c.name}</strong>
            </div>
          ))}
          {!showWindow && list.length === 0 && <div className="svcempty">{t('此分类还没有服务，从下方添加。')}</div>}
          {list.map((p, i) => {
            const meta = of(p.kind)
            return (
              <div
                key={p.id}
                role="option"
                aria-selected={sel?.id === p.id}
                tabIndex={0}
                className={`svcrow${sel?.id === p.id ? ' on' : ''}`}
                draggable
                onDragStart={() => { drag.current = i }}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => { e.preventDefault(); reorder(drag.current, i); drag.current = -1 }}
                onClick={(e) => { e.currentTarget.focus(); setSelected(p.id) }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setSelected(p.id) }
                  if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
                    e.preventDefault()
                    reorder(i, i + (e.key === 'ArrowUp' ? -1 : 1))
                  }
                }}
              >
                <ServiceIcon label={p.label} kind={p.kind} />
                <strong title={p.label}>{p.label}</strong>
                <RequirementBadge requirement={requirementOf(p.kind, meta)} />
                <input
                  type="checkbox"
                  className="svcswitch"
                  checked={p.enabled}
                  aria-label={t('启用 {name}', { name: p.label })}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => run(t('保存…'), () => SettingsService.UpdateProvider({ ...p, enabled: e.target.checked }))}
                />
              </div>
            )
          })}
        </div>

        {!showWindow && (
          <div className="svcctrls">
            <div className="segctl">
              <button aria-label={t('添加服务')} title={t('添加服务')}
                onClick={() => { setPicked([]); setAdding(true) }}>
                <PlusGlyph />
              </button>
              <button aria-label={t('移除服务')} title={t('移除服务')} disabled={!sel}
                onClick={() => {
                  if (!sel) return
                  setSelected('')
                  run(t('移除…'), () => SettingsService.RemoveProvider(sel.id))
                }}>
                <MinusGlyph />
              </button>
            </div>
          </div>
        )}
      </div>

      <div className="svcdetail">
        {showWindow && (
          <WindowConfigCard cfg={cfg} context={activeWin} name={winName} run={run} setStatus={setStatus} />
        )}
        {!showWindow && !sel && <div className="svcempty big">{t('从左侧选择一个服务查看配置。')}</div>}
        {sel && (
          <div className="svccard">
            <div className="svchead">
              <ServiceIcon label={sel.label} kind={sel.kind} big />
              <div className="svcnames">
                <strong>{sel.label}</strong>
                {of(sel.kind) && of(sel.kind)!.name !== sel.label && <span className="kind">{of(sel.kind)!.name}</span>}
              </div>
              <RequirementBadge requirement={requirementOf(sel.kind, of(sel.kind))} />
              {!usable(sel, of(sel.kind)) && <span className="badge warn">{t('缺配置')}</span>}
            </div>
            <div className="sbody">
              {of(sel.kind)?.note && <p className="noteline">{of(sel.kind)!.note}</p>}
              <ProviderField
                field={{ key: 'label', label: t('显示名称'), secret: false, required: true }}
                value={sel.label}
                run={run}
                onSave={(v) => run(t('保存…'), () => SettingsService.UpdateProvider({ ...sel, label: v }))}
              />
              {(of(sel.kind)?.fields ?? []).map((f) => (
                <div className="fieldpair" key={f.key}>
                  <ProviderField field={f} value={fieldOf(sel, f.key)} run={run} onSave={(v) => {
                    run(t('保存…'), () => SettingsService.UpdateProvider(setField(sel, f.key, v)))
                  }} />
                  {f.key === 'supportedModels' && of(sel.kind)?.fetchModels && (
                    <ModelFetch p={sel} run={run} local={!!of(sel.kind)?.fetchLocalModels} />
                  )}
                </div>
              ))}
              {!usable(sel, of(sel.kind)) && requirementOf(sel.kind, of(sel.kind)) === 'key' && (
                <p className="noteline missingkey">
                  {t('缺少密钥')}：{t('请前往服务官网注册申请个人的 API key。')}
                  {help && (
                    <>
                      {' '}
                      <a href={help.url} onClick={(e) => { e.preventDefault(); openURL(help.url) }}>
                        {t('参见 👉')} {help.title}
                      </a>
                    </>
                  )}
                </p>
              )}
              {!of(sel.kind) && <p className="noteline">{t('该类型未在本平台注册，保留配置但不会被调用。')}</p>}
              {sel.kind === 'apple' && sel.category === 'translate' && isMac && (
                <p className="noteline">
                  {t('苹果离线翻译需要先在系统设置中下载语言：')}
                  <a
                    href="x-apple.systempreferences:com.apple.Localization-Settings.extension?translation"
                    onClick={(e) => {
                      e.preventDefault()
                      openSystemSettings('x-apple.systempreferences:com.apple.Localization-Settings.extension?translation')
                    }}
                  >
                    {t('管理下载语言')}
                  </a>
                </p>
              )}
              {sel.kind === 'mdict' && <DictFiles p={sel} run={run} />}
              <ProviderProbe id={sel.id} />
            </div>
          </div>
        )}
      </div>

      {adding && (
        <AddServiceSheet
          types={addable}
          picked={picked}
          onToggle={(kind) => setPicked((prev) => prev.includes(kind) ? prev.filter((k) => k !== kind) : [...prev, kind])}
          onCancel={() => { setAdding(false); setPicked([]) }}
          onAdd={addPicked}
        />
      )}
    </div>
  )
}

// WindowConfigCard is what 窗口配置 shows for the window picked in the list: the
// services that window runs, plus 主窗口's own chrome. EasyDict keeps this on
// the same page as the service list, so the window a service is assigned to and
// the service's own configuration sit one click apart.
function WindowConfigCard({ cfg, context, name, run, setStatus }: {
  cfg: Config
  context: string
  name: string
  run: (label: string, call: () => Promise<unknown>) => void
  setStatus: (msg: string) => void
}) {
  const [stages, setStages] = useState<Label[] | null>(null)

  useEffect(() => {
    let live = true
    setStages(null)
    void SettingsService.CategoriesFor(context)
      .then((ts) => { if (live) setStages(ts ?? []) })
      .catch(() => { if (live) setStages([]) })
    return () => { live = false }
  }, [context])

  const selected = cfg.selections?.[context] ?? []
  // 窗口配置 describes the translation window's own chrome, so it belongs to
  // the 主窗口 tab rather than to every context that now answers there.
  const role = context === 'main' ? 'main' : ''
  const winOpts = (role && cfg.windowConfig?.[role]) || { input: true, langBar: true }

  // The stored list is an *exclusion* list: an empty selection means "run
  // everything", and a stage with no picked service also runs everything. The
  // chips therefore show on for services that will run even when the store has
  // no entry for them; otherwise a window that runs 朗读 would draw them gray.
  const explicit = selected.length > 0
  const stageList = stages ?? []
  const enabledIds = (key: string) => listOf(cfg, key).filter((p) => p.enabled).map((p) => p.id)
  const contextIds = stageList.flatMap((s) => enabledIds(s.key))
  // What the window will actually run: a stage with explicit picks contributes
  // those, a stage without contributes all of its enabled services. The count
  // is the number of chips that read as on, not the length of the stored list.
  const running = stageList.reduce((n, s) => {
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
    <div className="svccard">
      <div className="svchead">
        <div className="svcnames">
          <strong>{t('窗口配置')}</strong>
          <span className="kind">{name}</span>
        </div>
      </div>
      <div className="sbody">
        <div className="winsummary">
          <p className="noteline">
            {explicit
              ? t('本窗口运行 {n} 个服务，按勾选顺序显示。', { n: running })
              : t('本窗口运行全部 {n} 个已启用服务；点掉一项即可排除它。', { n: running })}
            {context === 'silentOcr' && t(' 静默截图 OCR 只复制文字，不显示结果。')}
          </p>
          {explicit && (
            <button className="tiny ghost" onClick={() => run(t('恢复…'), () => SettingsService.SetSelection(context, []))}>
              {t('改为全部启用')}
            </button>
          )}
        </div>

        {role && (
          <div className="winopts">
            <label className="switch">
              <input
                type="checkbox"
                checked={winOpts.input}
                aria-label={t('显示文本输入框')}
                onChange={(e) => run(t('保存…'), () => SettingsService.SetWindowOption(role, 'input', e.target.checked))}
              />
              <span className="swbody">
                <SettingGlyph name="input" />
                <span>
                  <strong>{t('显示文本输入框')}</strong>
                  <em>{t('窗口上方显示可编辑的原文，Enter 重新查询')}</em>
                </span>
              </span>
            </label>
            <label className="switch">
              <input
                type="checkbox"
                checked={winOpts.langBar}
                aria-label={t('显示语言选择栏')}
                onChange={(e) => run(t('保存…'), () => SettingsService.SetWindowOption(role, 'langBar', e.target.checked))}
              />
              <span className="swbody">
                <SettingGlyph name="globe" />
                <span>
                  <strong>{t('显示语言选择栏')}</strong>
                  <em>{t('标题栏显示译文语言，可直接切换')}</em>
                </span>
              </span>
            </label>
          </div>
        )}

        {stages === null && <div className="muted">{t('载入该窗口的服务清单…')}</div>}
        {stageList.map((stage) => {
          const stageIds = enabledIds(stage.key)
          const stagePicked = selected.filter((id) => stageIds.includes(id))
          const autoStage = explicit && stagePicked.length === 0
          return (
            <div className="winstage" key={stage.key}>
              <h5>
                {stage.name}
                {autoStage && <em className="auto">{t('全部')}</em>}
              </h5>
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

// EasyDict sorts the add sheet by what a service needs to work: built-in first,
// then keyless, then CLI agents, then the ones that want a key.
const requirementOrder = ['built-in', 'no-key', 'cli', 'key'] as const
type Requirement = typeof requirementOrder[number]

function requirementOf(kind: string, meta?: ServiceType): Requirement {
  if (kind.endsWith('-cli')) return 'cli'
  if (meta?.free) return 'no-key'
  if (!meta) return 'built-in'
  if ((meta.fields ?? []).some((f) => f.required)) return 'key'
  return 'built-in'
}

function RequirementBadge({ requirement }: { requirement: Requirement }) {
  return <span className={`reqbadge ${requirement}`}>{requirement}</span>
}

// ServiceIcon stands in for EasyDict's per-service artwork: a rounded tinted
// square with the name's initial, tinted from the kind so a service keeps the
// same colour in the list and in the detail head.
function ServiceIcon({ label, kind, big }: { label: string; kind: string; big?: boolean }) {
  let h = 0
  for (let i = 0; i < kind.length; i++) h = (h * 31 + kind.charCodeAt(i)) % 360
  const text = (label || kind).trim()
  return (
    <span className={`svcicon${big ? ' big' : ''}`} style={{ background: `hsl(${h} 52% 46%)` }}>
      {(text[0] ?? '?').toUpperCase()}
    </span>
  )
}

function PlusGlyph() {
  return (
    <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
      <path d="M8 2v12M2 8h12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

function MinusGlyph() {
  return (
    <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
      <path d="M2 8h12" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

function AddServiceSheet({ types, picked, onToggle, onCancel, onAdd }: {
  types: ServiceType[]
  picked: string[]
  onToggle: (kind: string) => void
  onCancel: () => void
  onAdd: () => void
}) {
  const byKind = (kind: string) => types.find((s) => s.kind === kind)
  return (
    <div className="svcsheet" role="dialog" aria-modal="true" aria-label={t('添加服务')}>
      <div className="svcsheetcard">
        <h2>{t('添加服务')}</h2>
        <div className="svcsheetbody">
          {types.length === 0 && (
            <p className="noteline svcempty">{t('已添加全部服务')}</p>
          )}
          {requirementOrder.map((req) => {
            const items = types.filter((s) => requirementOf(s.kind, s) === req)
            if (items.length === 0) return null
            return (
              <div className="svcgroup" key={req}>
                <h3>{req}</h3>
                <div className="svcgrid">
                  {items.map((s) => (
                    <button
                      key={s.kind}
                      className={`svctile${picked.includes(s.kind) ? ' on' : ''}`}
                      onClick={() => onToggle(s.kind)}
                    >
                      <ServiceIcon label={s.name} kind={s.kind} />
                      <span title={s.name}>{s.name}</span>
                      {byKind(s.kind)?.free && <em>{t('免费')}</em>}
                    </button>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
        <div className="svcsheetfoot">
          <button onClick={onCancel}>{t('取消')}</button>
          <button className="primary" disabled={picked.length === 0} onClick={onAdd}>{t('好')}</button>
        </div>
      </div>
    </div>
  )
}

// DictFiles is EasyDict's 词典列表: MDict answers from imported .mdx files, and
// the folder box above is only the older way of pointing at a shelf of them.
// The list reloads whenever the saved Extra changes, because 导入词典 and 移除
// both go through the parent's run(), which refreshes the whole config.
function DictFiles({ p, run }: {
  p: Provider
  run: (label: string, call: () => Promise<unknown>) => void
}) {
  const [files, setFiles] = useState<string[]>([])
  const saved = p.extra?.files ?? ''

  useEffect(() => {
    void SettingsService.DictFiles(p.id)
      .then((list) => setFiles(list ?? []))
      .catch(() => undefined)
  }, [p.id, saved])

  const add = () => {
    void Service.PickDictFiles()
      .then((paths) => {
        if (!paths || paths.length === 0) return
        run(t('导入词典…'), () => SettingsService.AddDictFiles(p.id, paths))
      })
      .catch((e) => run(t('导入词典…'), () => Promise.reject(e)))
  }

  const base = (path: string) => path.split(/[\\/]/).pop() || path

  return (
    <div className="dictfiles">
      {files.map((file) => (
        <div className="dfrow" key={file}>
          <span className="dfname" title={file}>{base(file)}</span>
          <button className="tiny" onClick={() => run(t('移除…'), () => SettingsService.RemoveDictFile(p.id, file))}>
            {t('移除')}
          </button>
        </div>
      ))}
      {files.length === 0 && (
        <p className="noteline">
          {p.extra?.path
            ? t('未单独导入词典，使用文件夹中的全部 .mdx。')
            : t('尚未导入任何词典，点击「导入词典」选择 .mdx 文件。')}
        </p>
      )}
      <div className="dfactions">
        <button className="tiny" onClick={add}>{t('导入词典')}</button>
      </div>
    </div>
  )
}

// ProviderProbe is EasyDict's 验证 button: one small query through the saved
// settings, so a wrong key or endpoint is named here instead of surfacing as a
// failed translation later. The reply stays beside the button, not in the
// page-wide status line, because the two services can be tested in a row.
function ProviderProbe({ id }: { id: string }) {
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  return (
    <div className="probe">
      <button
        className="tiny"
        disabled={busy}
        onClick={() => {
          setBusy(true)
          setMsg(null)
          SettingsService.TestProvider(id)
            .then((text) => setMsg({ ok: true, text }))
            .catch((e) => setMsg({ ok: false, text: errMsg(e) }))
            .finally(() => setBusy(false))
        }}
      >
        {busy ? t('验证中…') : t('验证')}
      </button>
      {msg && (
        <span className={msg.ok ? 'ok' : 'bad'}>
          {msg.ok
            ? t('验证成功 🎉 {text}', { text: msg.text })
            : t('验证失败 💔 {err}', { err: msg.text })}
        </span>
      )}
    </div>
  )
}

// ProviderField owns its draft and writes on blur: a bound input that
// round-tripped to Go on every keystroke would fight the caret.
function ProviderField({ field, value, onSave, run, icon }: {
  field: Field
  value: string
  onSave: (v: string) => void
  run: (label: string, call: () => Promise<unknown>) => void
  icon?: IconName
}) {
  const [draft, setDraft] = useState(value)
  const [shown, setShown] = useState(false)
  useEffect(() => setDraft(value), [value])

  return (
    <label className="field">
      <span className="fname">
        {icon && <SettingGlyph name={icon} />}
        {field.label}
        {field.required && <em className="req">{t('必填')}</em>}
      </span>
      {field.options?.length ? (
        <select
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value)
            onSave(e.target.value)
          }}
        >
          {field.options.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      ) : field.text ? (
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
      {field.note && <span className="fieldnote">{field.note}</span>}
    </label>
  )
}

// ModelFetch is EasyDict's 从云端获取模型: the endpoint's own catalog, opened with
// the saved choices ticked so a fetch refines the list instead of replacing a
// curated one blindly. With nothing saved the whole catalog is offered, which
// is what the card's model menu would show anyway.
function ModelFetch({ p, run, local }: {
  p: Provider
  run: (label: string, call: () => Promise<unknown>) => void
  local?: boolean
}) {
  const [models, setModels] = useState<string[] | null>(null)
  const [picked, setPicked] = useState<string[]>([])
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  // 获取本地模型 reads what the local daemon pulled; the cloud button is the
  // wording for a vendor's catalog. Both open the same list.
  const action = local ? t('获取本地模型') : t('从云端获取模型')

  const fetchModels = () => {
    setBusy(true)
    setErr('')
    setQuery('')
    void SettingsService.FetchModels(p.id)
      .then((list) => {
        const all = list ?? []
        const saved = fieldOf(p, 'supportedModels')
          .split(/[,，]/).map((m) => m.trim()).filter(Boolean)
        setModels(all)
        setPicked(saved.length ? all.filter((m) => saved.includes(m)) : all)
      })
      .catch((e) => setErr(errMsg(e)))
      .finally(() => setBusy(false))
  }

  const shown = (models ?? []).filter((m) => m.toLowerCase().includes(query.trim().toLowerCase()))
  const allShown = shown.length > 0 && shown.every((m) => picked.includes(m))

  return (
    <div className="modelfetch">
      <button className="tiny" disabled={busy} onClick={fetchModels}>
        {busy ? t('获取模型…') : action}
      </button>
      {err && <span className="error">{err}</span>}
      {models && (
        <div className="svcsheet" role="dialog" aria-modal="true" aria-label={action}>
          <div className="svcsheetcard">
            <h2>{action}</h2>
            <div className="svcsheetbody">
              {models.length === 0 ? <p className="noteline">{t('没有获取到模型')}</p> : (
                <>
                  <input
                    className="modelsearch"
                    value={query}
                    placeholder={t('搜索模型')}
                    aria-label={t('搜索模型')}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                  {shown.length === 0 ? <p className="noteline">{t('没有匹配的模型')}</p> : (
                    <div className="modelpick" role="group" aria-label={t('支持的模型')}>
                      {shown.map((m) => (
                        <label key={m} className="modelrow">
                          <input
                            type="checkbox"
                            checked={picked.includes(m)}
                            onChange={() => setPicked((prev) => prev.includes(m)
                              ? prev.filter((x) => x !== m)
                              : [...prev, m])}
                          />
                          <span>{m}</span>
                        </label>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>
            <div className="svcsheetfoot">
              <button onClick={() => setPicked(allShown
                ? picked.filter((m) => !shown.includes(m))
                : [...new Set([...picked, ...shown])])}>
                {allShown ? t('取消全选') : t('全选')}
              </button>
              <span className="spacer" />
              <button onClick={() => setModels(null)}>{t('取消')}</button>
              <button className="primary" onClick={() => {
                setModels(null)
                run(t('保存…'), () =>
                  SettingsService.UpdateProvider(setField(p, 'supportedModels', picked.join(', '))))
              }}>{t('好')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
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
  // EasyDict refuses a combo another action already owns: binding both would
  // leave one of them dead, and the recorder would look like it worked. The row
  // names the owner so the user knows which key to move instead.
  const [conflict, setConflict] = useState<{ key: string; name: string } | null>(null)

  const refreshDead = useCallback(() => {
    void Service.ShortcutProblems().then((p) => setDead(p ?? [])).catch(() => setDead([]))
  }, [])

  useEffect(refreshDead, [refreshDead])

  const rebind = (slot: string, accel: string, label = t('绑定…')) =>
    run(label, async () => {
      // In-app actions share the recorder with the global slots; the prefix
      // keeps one id space without a second recording state.
      if (slot.startsWith('app:')) {
        await SettingsService.SetAppShortcut(slot.slice(4), accel)
      } else {
        await SettingsService.SetShortcut(slot, accel)
      }
      // The store's watcher rebinds before Update returns, so the list is the
      // answer to this keystroke rather than to the previous one.
      refreshDead()
    })

  const conflictOwner = (accel: string, slot: string): string | undefined => {
    for (const s of slots) {
      if (s.key === slot) continue
      if ((cfg.shortcuts?.[s.key] ?? []).includes(accel)) return s.name
    }
    for (const a of appActions) {
      if (`app:${a.action}` === slot) continue
      if (appAccel(cfg, a.action) === accel) return t(a.label)
    }
    return undefined
  }

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
      const owner = conflictOwner(accel, rec)
      if (owner) {
        setConflict({ key: prettyShortcut(accel), name: owner })
        setRec('')
        setStatus('')
        return
      }
      setConflict(null)
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
      {conflict && (
        <div className="deadkeys">
          <div className="error">
            {t('快捷键"{key}"，无法使用', { key: conflict.key })}
            <br />
            {t('当前快捷键无法使用，因为它已被 "{name}"占用', { name: conflict.name })}
          </div>
        </div>
      )}
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
      <div className="group">
        <h4>{t('全局快捷键')}</h4>
        {[...slots].sort((a, b) => slotRank(a.key) - slotRank(b.key)).map((s) => {
          const accels = cfg.shortcuts?.[s.key] ?? []
          return (
            <div className="krow" key={s.key}>
              <span className="label">{s.name}</span>
              <div
                className={`keybox${rec === s.key ? ' rec' : ''}${accels.length ? '' : ' empty'}`}
                onClick={() => { setConflict(null); setRec(rec === s.key ? '' : s.key) }}
              >
                <span className="keytext">
                  {rec === s.key
                    ? t('按组合键…')
                    : accels.length
                      ? accels.map(prettyShortcut).join(' / ')
                      : t('未设置')}
                </span>
                {accels.length > 0 && rec !== s.key && (
                  <button
                    className="keyclear"
                    aria-label={t('清除')}
                    title={t('清除')}
                    onClick={(e) => { e.stopPropagation(); rebind(s.key, '', t('解绑…')) }}
                  >×</button>
                )}
              </div>
            </div>
          )
        })}
      </div>
      <div className="group">
        <h4>{t('应用内快捷键')}</h4>
        {appActions.map(({ action, label }) => {
          const id = `app:${action}`
          const accel = appAccel(cfg, action)
          return (
            <div className="krow" key={action}>
              <span className="label">{t(label)}</span>
              <div
                className={`keybox${rec === id ? ' rec' : ''}${accel ? '' : ' empty'}`}
                onClick={() => setRec(rec === id ? '' : id)}
              >
                <span className="keytext">
                  {rec === id ? t('按组合键…') : accel ? prettyShortcut(accel) : t('未设置')}
                </span>
                {accel && rec !== id && (
                  <button
                    className="keyclear"
                    aria-label={t('清除')}
                    title={t('清除')}
                    onClick={(e) => { e.stopPropagation(); rebind(id, '', t('解绑…')) }}
                  >×</button>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// EasyDict lists the global actions in query order (input, screenshot, selection,
// clipboard, replace, OCR) rather than alphabetically; unknown slots go last.
const slotOrder = [
  'input', 'capture', 'selection', 'toggleSelect', 'clipboard',
  'polish', 'replace', 'silentOcr', 'ocrShot', 'ocrClipboard', 'ocrWindow',
  'odict', 'settings',
]

function slotRank(key: string): number {
  const i = slotOrder.indexOf(key)
  return i === -1 ? slotOrder.length : i
}

const themes = [
  { key: 'auto', name: t('跟随系统') },
  { key: 'light', name: t('浅色模式') },
  { key: 'dark', name: t('深色模式') },
]

const readAloud = [
  { key: 'off', name: t('不自动朗读') },
  { key: 'source', name: t('自动朗读原文') },
  { key: 'target', name: t('自动朗读译文') },
]

const englishVoices = [
  { key: 'us', name: t('美式发音') },
  { key: 'uk', name: t('英式发音') },
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

  // macOS only lists an app in these panes after it has asked; the prompt is
  // what registers Transdict, so the row offers that before the pane shortcut.
  const request = (kind: 'screen' | 'access') => {
    void Service.RequestPermission(kind).then(setState).catch(() => undefined)
  }

  const row = (name: string, ok: boolean | undefined, pane: string, icon: IconName, kind: 'screen' | 'access') => (
    <div className="actionrow prow" key={pane}>
      <SettingGlyph name={icon} />
      <span className="pname">{name}</span>
      <span className={`dot${ok === false ? ' bad' : ok ? '' : ' off'}`} />
      <span className={ok === false ? 'miss' : 'byline'}>{ok ? t('已授权') : t('未授权')}</span>
      {ok === false && (
        <button className="tiny" onClick={() => request(kind)}>
          {t('请求权限')}
        </button>
      )}
      <button className="tiny" onClick={() => void Service.OpenPermission(pane)}>
        {t('打开系统设置')}
      </button>
    </div>
  )

  return (
    <div className="group">
      <h4>{t('权限')}</h4>
      {row(t('屏幕录制'), state?.screenRecording, 'screen', 'window', 'screen')}
      {row(t('辅助功能'), state?.accessibility, 'access', 'select', 'access')}
      <p className="noteline withaction">
        <span>{t('先点「请求权限」让系统登记 Transdict；如果列表里仍然没有，点「打开系统设置」，用 + 添加 /Applications/Transdict.app。改完需要重启 Transdict 才生效。')}</span>
        {state && (!state.screenRecording || !state.accessibility) && (
          <button className="tiny" onClick={() => void Service.Restart()}>{t('重启 Transdict')}</button>
        )}
      </p>
    </div>
  )
}

// The two forms EasyDict keeps apart: 通用 answers "how do I query", 高级 holds
// the knobs a user only touches to fix something (windows, mouse selection, OCR,
// text processing, network, notebook). One component renders either half so the
// save/choose/toggle helpers stay in one place.
function General({ part, cfg, run, setStatus }: {
  part: 'general' | 'advanced'
  cfg: Config
  run: (label: string, call: () => Promise<unknown>, after?: (path: string) => string) => void
  setStatus: (text: string) => void
}) {
  const [langs, setLangs] = useState<Lang[]>([])
  const [checking, setChecking] = useState(false)
  const [update, setUpdate] = useState<UpdateReport | null>(null)
  const [version, setVersion] = useState('')
  const [hideWarn, setHideWarn] = useState(false)
  const [hideRefuse, setHideRefuse] = useState(false)
  const [logDir, setLogDir] = useState('')
  useEffect(() => {
    void SettingsService.Languages().then((ls) => setLangs(ls ?? []))
    void SettingsService.Version().then((v) => setVersion(v ?? ''))
  }, [])

  // 日志目录 only exists on 高级, and it is the process's own path, so it is
  // read when that page opens rather than guessed here.
  useEffect(() => {
    if (part !== 'advanced') return
    void SettingsService.LogFolder().then((d) => setLogDir(d ?? '')).catch(() => undefined)
  }, [part])

  // Scalars go through Save, which validates and normalizes. Reading the store
  // again first means a service edited in the other window is not rolled back
  // by this page's older snapshot.
  const patch = (change: Partial<Config>) =>
    run(t('保存…'), async () => {
      const fresh = await SettingsService.Get()
      await SettingsService.Save({ ...fresh, ...change } as Config)
    })

  // A check writes nothing, so it goes around run: the answer belongs in the
  // status line and a config reload would only clear it. A newer release also
  // raises the update window, which is where its notes and download live.
  const checkUpdate = () => {
    setChecking(true)
    setStatus(t('检查更新…'))
    void Service.CheckUpdate()
      .then((r) => {
        setUpdate(r?.version ? r : null)
        setStatus(r?.version
          ? t('发现新版本 {v}（当前 {c}）', { v: r.version, c: r.current })
          : t('已是最新版本 {v}', { v: r?.current ?? '' }))
        if (r?.version) void Service.OpenUpdateWindow()
      })
      .catch((e) => setStatus(errMsg(e)))
      .finally(() => setChecking(false))
  }

  const openUpdate = () => void Service.OpenUpdateWindow()

  const openUpdatePage = () => {
    void Service.OpenUpdatePage().catch((e) => setStatus(errMsg(e)))
  }

  // 忽略此版本 writes the field the automatic prompt also writes, so the note
  // under the buttons stays the one place that says what is being skipped.
  const ignoreUpdate = () => {
    if (!update) return
    const v = update.version
    setUpdate(null)
    run(t('保存…'), async () => {
      const fresh = await SettingsService.Get()
      await SettingsService.Save({ ...fresh, skippedUpdate: v } as Config)
    }, () => t('已忽略 {v} 的自动提示', { v }))
  }

  const clearIgnore = () => patch({ skippedUpdate: '' })

  // 所选目录不可写 is EasyDict's warning under the folder picker: the notebook
  // cannot append there, so the switch would look on while every query failed to
  // save. A path that does not exist yet is not a problem — Go creates it on the
  // first write.
  const [dirBad, setDirBad] = useState(false)
  useEffect(() => {
    const dir = (cfg.vocabularyDir ?? '').trim()
    if (!cfg.vocabularyNotebook || !dir) {
      setDirBad(false)
      return
    }
    let live = true
    void SettingsService.VocabularyDirWritable(dir)
      .then((ok) => { if (live) setDirBad(!ok) })
      .catch(() => { if (live) setDirBad(false) })
    return () => { live = false }
  }, [cfg.vocabularyNotebook, cfg.vocabularyDir])

  const choose = (key: keyof Config, name: string, options: { key: string; name: string }[]) => {
    const icon = iconOf(String(key))
    return (
      <label className="field" key={key}>
        <span className="fname">
          {icon && <SettingGlyph name={icon} />}
          {name}
        </span>
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
  }

  // 第一语言 and 第二语言 cannot be the same: the detector falls back to them
  // for a short phrase, and a repeated guess breaks no tie. EasyDict swaps the
  // other language back to the old value and says so; the same happens here.
  const langName = (code: string) =>
    pickableLangs(langs).find((l) => l.code === code)?.name ?? code
  const pickPair = (which: 'firstLang' | 'secondLang', next: string) => {
    const first = cfg.firstLang || 'en'
    const second = cfg.secondLang || 'zh-CN'
    const mine = which === 'firstLang' ? first : second
    const other = which === 'firstLang' ? 'secondLang' : 'firstLang'
    const otherNow = which === 'firstLang' ? second : first
    if (next === otherNow) {
      patch({ [which]: next, [other]: mine } as Partial<Config>)
      setStatus(t('第一语言不能与第二语言相同（{lang}）。{field}已被替换为{kept}。', {
        lang: langName(next),
        field: which === 'firstLang' ? t('第二语言') : t('第一语言'),
        kept: langName(mine),
      }))
      return
    }
    patch({ [which]: next } as Partial<Config>)
  }
  const pairSelect = (which: 'firstLang' | 'secondLang', name: string) => (
    <label className="field" key={which}>
      <span className="fname">
        <SettingGlyph name="globe" />
        {name}
      </span>
      <select value={cfg[which] ?? ''} onChange={(e) => pickPair(which, e.target.value)}>
        {pickableLangs(langs).map((l) => (
          <option key={l.code} value={l.code}>{langOption(l.code, l.name)}</option>
        ))}
      </select>
    </label>
  )

  const toggle = (key: keyof Config, name: string, note: string) => {
    const icon = iconOf(String(key))
    return (
      <label className="switch" key={key}>
        <input
          type="checkbox"
          checked={!!cfg[key]}
          aria-label={name}
          onChange={(e) => patch({ [key]: e.target.checked } as Partial<Config>)}
        />
        <span className="swbody">
          {icon && <SettingGlyph name={icon} />}
          <span>
            <strong>{name}</strong>
            <em>{note}</em>
          </span>
        </span>
      </label>
    )
  }

  return (
    <div className="slist">
      {part === 'general' && (<>
      <div className="group">
        <h4>{t('查询语言')}</h4>
        <div className="fields">
          {choose('sourceLang', t('原文语言'), [{ key: 'auto', name: t('自动检测') }, ...pickableLangs(langs)
            .map((l) => ({ key: l.code, name: langOption(l.code, l.name) }))])}
          {choose('targetLang', t('译文语言'), [{ key: 'auto', name: t('自动选择') }, ...pickableLangs(langs)
            .map((l) => ({ key: l.code, name: langOption(l.code, l.name) }))])}
        </div>
        <div className="fields">
          {pairSelect('firstLang', t('第一语言'))}
          {pairSelect('secondLang', t('第二语言'))}
          {choose('detectOptimize', t('语种识别'), [
            { key: 'none', name: t('仅使用系统语种识别') },
            { key: 'google', name: t('使用 Google 语种识别进行优化') },
            { key: 'baidu', name: t('使用百度语种识别进行优化') },
          ])}
        </div>
        <p className="noteline">
          {t('短语难以判断语种时，优先认作这两种语言；「自动选择」译文也在这两者之间换算。')}
        </p>
        <p className="noteline">
          {t('选择 Google 或百度后，识别会先请该服务读一小段文字；服务不可用时仍回到系统识别。')}
        </p>
      </div>

      <div className="group">
        <h4>{t('输入框')}</h4>
        {toggle('clearInputOnQuery', t('输入翻译时，清空查询内容'),
          t('按快捷键打开输入窗口时，从空白输入框开始'))}
        {toggle('keepLastOnNoSelection', t('划词翻译未选中文本时，保留上次结果'),
          t('没有选中文字时不显示错误，窗口保持上一次的结果'))}
        {toggle('selectTextOnOpen', t('打开窗口时自动选中查询文本'),
          t('窗口打开时选中输入框里的文字，直接输入即可替换'))}
      </div>

      <div className="group">
        <h4>{t('自动查询')}</h4>
        {(isMac || isWindows) && toggle('mouseSelect', t('划词后自动查询'), t('选中文字即翻译，无需按快捷键'))}
        {toggle('autoQueryOCR', t('图片 OCR 后自动查询'),
          t('关闭后截图只显示识别出的文字，再由「全部重新查询」翻译'))}
        {toggle('autoQueryPaste', t('粘贴后自动查询'), t('粘贴完成后立即翻译，不必按 Enter'))}
        {toggle('autoQueryTyping', t('输入时自动查询'), t('停止输入片刻后自动翻译'))}
        {toggle('autoPronounceWord', t('查询英语单词后自动播放发音'),
          t('查询单个英语单词时自动朗读原文，不必点喇叭'))}
        <div className="fields">
          {choose('pronunciation', t('英语发音'), englishVoices)}
          {choose('ttsAuto', t('自动朗读'), readAloud)}
        </div>
      </div>

      <div className="group">
        <h4>{t('自动复制')}</h4>
        {toggle('autoCopySelect', t('自动复制划词文本'), t('划词查询时把选中的文字写入剪贴板'))}
        {toggle('autoCopyOCR', t('自动复制截图 OCR 结果'), t('截图翻译时把识别出的文字写入剪贴板'))}
        {toggle('autoCopy', t('自动复制第一个翻译结果'), t('结果一出现就把第一条译文写入剪贴板；静默截图 OCR 与翻译替换按各自的方式处理。'))}
      </div>

      <div className="group">
        <h4>{t('快捷功能')}</h4>
        {toggle('showGoogleLink', t('显示 Google 快捷图标'), t('标题栏一键用 Google 搜索当前文本'))}
        {isMac && toggle('showAppleDictLink', t('显示苹果词典快捷图标'), t('标题栏一键在系统词典中查当前文本'))}
        {toggle('showEudicLink', t('显示欧路词典快捷图标'), t('标题栏一键在欧路词典中查当前文本'))}
        {toggle('showQuickActionButton', t('显示快捷设置图标'), t('标题栏显示快捷动作按钮：添加到收藏、文本改写与打开设置'))}
      </div>

      <div className="group">
        <h4>{t('显示')}</h4>
        {toggle('markdownRender', t('启用 Markdown 渲染'),
          t('AI 翻译结果将以 Markdown 格式呈现标题、列表与强调。点击单个结果旁的图标可临时切换。'))}
        {toggle('hideSameLang', t('隐藏与原文相同的译文'), t('源语言与目标语言相同时不再重复显示'))}
      </div>

      <div className="group">
        <h4>{t('应用设置')}</h4>
        <div className="fields">
          {choose('uiLang', t('显示语言'), uiLangs)}
          {choose('theme', t('主题'), themes)}
        </div>
        {toggle('loginItem', t('开机自启动'),
          isMac ? t('需要 macOS 13 以上，且应用已签名安装到 /Applications') : t('写入当前用户的启动项'))}
        <label className="switch">
          <input
            type="checkbox"
            checked={!!cfg.hideMenuBarIcon}
            aria-label={isMac ? t('隐藏菜单栏图标') : t('隐藏系统托盘图标')}
            onChange={(e) => {
              if (!e.target.checked) {
                setHideRefuse(false)
                patch({ hideMenuBarIcon: false })
                return
              }
              // EasyDict refuses to hide the only easy way in: with neither the
              // input nor the selection shortcut set, the icon is how the app is
              // still reachable.
              const input = cfg.shortcuts?.input ?? []
              const selection = cfg.shortcuts?.selection ?? []
              if (input.length === 0 && selection.length === 0) {
                setHideWarn(false)
                setHideRefuse(true)
                return
              }
              setHideWarn(true)
            }}
          />
          <span className="swbody">
            <SettingGlyph name="power" />
            <span>
              <strong>{isMac ? t('隐藏菜单栏图标') : t('隐藏系统托盘图标')}</strong>
              <em>{isMac ? t('菜单栏不再出现图标，只留快捷键与程序坞入口')
                : t('托盘不再出现图标，只留快捷键与任务栏入口')}</em>
            </span>
          </span>
        </label>
        {hideRefuse && (
          <div className="row-end">
            <span className="noteline">
              {t('检测到当前未设置输入翻译快捷键或划词翻译快捷键！请先设置快捷键再隐藏菜单栏图标。')}
            </span>
            <button className="ghost tiny" onClick={() => setHideRefuse(false)}>{t('确定')}</button>
          </div>
        )}
        {hideWarn && !cfg.hideMenuBarIcon && (
          <div className="row-end">
            <span className="noteline">
              {t('如需恢复，请在查词窗口使用快捷键 ⌘, 打开设置页，然后取消【隐藏菜单栏图标】选项。')}
            </span>
            <button className="ghost tiny" onClick={() => setHideWarn(false)}>{t('取消')}</button>
            <button className="primary tiny" onClick={() => {
              setHideWarn(false)
              patch({ hideMenuBarIcon: true })
            }}>{t('确定')}</button>
          </div>
        )}
        {toggle('showInDock',
          isMac ? t('在程序坞中显示图标') : t('在任务栏中显示图标'),
          isMac ? t('关闭后只保留菜单栏入口，主窗口仍由快捷键唤起')
            : t('任务栏按钮属于窗口，改动在下次打开主窗口时生效'))}
        {toggle('hideMainWindow', t('启动后隐藏主窗口'), t('只在菜单栏运行，用快捷键或菜单打开主窗口，下次启动生效'))}
      </div>

      <div className="group">
        <h4>{t('启动与更新')}</h4>
        <ProviderField
          field={{ key: 'updateURL', label: t('更新源地址'), secret: false, required: false, placeholder: 'https://api.github.com/repos/madbananana/transdict/releases' }}
          value={cfg.updateURL ?? ''}
          run={run}
          icon="refresh"
          onSave={(v) => patch({ updateURL: v })}
        />
        <p className="noteline">{t('一个 JSON 文档，写明最新版本号与下载地址；GitHub 的 Releases API 直接可用。')}</p>
        {toggle('autoUpdate', t('自动检查应用更新'), t('启动时和运行期间定时检查；发现新版本时询问是否安装，也可忽略该版本。'))}
        {toggle('includeBeta', t('包括 Beta 版本'), t('抢先体验新功能，但可能不稳定'))}
        <div className="actionrow">
          <span className="byline">
            {version ? t('当前版本 {v}', { v: version }) : ''}
          </span>
          <div className="btnrow">
            {update && (
              <>
                <button className="primary tiny" onClick={openUpdate}>
                  {t('查看更新')}
                </button>
                <button className="tiny" onClick={openUpdatePage}>
                  {t('打开发布页')}
                </button>
                <button className="tiny" onClick={ignoreUpdate}>
                  {t('忽略此版本')}
                </button>
              </>
            )}
            <button className="tiny" disabled={checking} onClick={checkUpdate}>
              {t('立即检查')}
            </button>
          </div>
        </div>
        {cfg.skippedUpdate && (
          <div className="row-end">
            <span className="noteline">{t('已忽略 {v} 的自动更新提示', { v: cfg.skippedUpdate })}</span>
            <button className="ghost tiny" onClick={clearIgnore}>{t('取消忽略')}</button>
          </div>
        )}
        {isMac && (
          <p className="noteline">{t('自动更新会替换当前应用；未签名版本可能需要重新授予录屏与辅助功能权限。')}</p>
        )}
      </div>

      <div className="group">
        <h4>{t('字体')}</h4>
        <label className="field slider-field">
          <span className="fname">
            <SettingGlyph name="font" />
            {t('字体大小')}
          </span>
          <input
            type="range"
            min={1}
            max={5}
            step={1}
            value={Number(cfg.resultFont || '1')}
            onChange={(e) => patch({ resultFont: e.target.value })}
          />
          <span className="slider-val">
            {({ '1': t('小'), '2': t('较小'), '3': t('中'), '4': t('较大'), '5': t('大') } as Record<string, string>)[
              cfg.resultFont || '1'
            ] ?? t('中')}
          </span>
        </label>
        <p className="noteline">{t('支持在翻译窗口通过快捷键 ⌘ +/- 修改字体大小')}</p>
      </div>
      </>)}

      {part === 'advanced' && (<>
      <div className="group">
        <h4>{t('常规设置')}</h4>
        {(cfg.tts ?? []).some((p) => p.enabled) && (
          <div className="fields">{choose('defaultTTS', t('默认 TTS 服务'),
            (cfg.tts ?? []).filter((p) => p.enabled)
              .map((p) => ({ key: p.id, name: p.label || p.kind })))}</div>
        )}
        {toggle('preferYoudaoWord', t('英语单词优先使用有道发音'),
          t('关闭后，英语单词发音将使用默认 TTS 服务'))}
        <label className="field slider-field">
          <span className="fname">
            <SettingGlyph name="document" />
            {t('检测古文的最小文本长度')}
          </span>
          <input
            type="range"
            min={0}
            max={100}
            step={10}
            value={cfg.classicalMinLength ?? 20}
            onChange={(e) => patch({ classicalMinLength: Number(e.target.value) })}
          />
          <span className="slider-val">
            {(cfg.classicalMinLength ?? 20) === 0
              ? t('关闭检测')
              : t('{n} 字', { n: cfg.classicalMinLength ?? 20 })}
          </span>
        </label>
        <p className="noteline">
          {t('文本达到该长度才判定是否为古文；设为 0 关闭古文检测。')}
        </p>
      </div>

      {(isMac || isWindows) && (
        <div className="group">
          <h4>{t('鼠标划词查询')}</h4>
          {toggle('autoShowQueryIcon', t('划词后自动显示查询图标'),
            t('在选区旁显示查询按钮，点击即可翻译'))}
          {cfg.autoShowQueryIcon && (
            <>
              <div className="fields">
                {choose('queryIconExcludeLang', t('仅当选中文本语言不是'),
                  pickableLangs(langs).map((l) => ({ key: l.code, name: l.name })))}
              </div>
              <label className="field slider-field">
                <span className="fname">
                  <SettingGlyph name="cursor" />
                  {t('最小字符长度')}
                </span>
                <input
                  type="range"
                  min={0}
                  max={50}
                  step={10}
                  value={cfg.queryIconMinLength ?? 0}
                  onChange={(e) => patch({ queryIconMinLength: Number(e.target.value) })}
                />
                <span className="slider-val">{cfg.queryIconMinLength ?? 0}</span>
              </label>
              <p className="noteline">
                {t('仅当选中文本语言≠指定语言 且 字符数≥该值时显示查询图标。')}
              </p>
            </>
          )}
          {toggle('queryIconClickOnly', t('点击划词查询图标时才查询（需隐藏主窗口）'),
            t('划词后不再自动查询，只在点击图标时发问'))}
        </div>
      )}

      <div className="group">
        <h4>{t('取词与文本替换')}</h4>
        {toggle('forceCopy', t('启用强制取词'),
          t('注意，这可能会影响剪贴板内容'))}
        {cfg.forceCopy && (
          <div className="fields">{choose('forceCopyKind', t('强制取词类型'), [
            { key: 'menu', name: t('优先菜单栏动作复制取词') },
            { key: 'shortcut', name: t('优先模拟快捷键复制取词') },
          ])}</div>
        )}
        {isMac && toggle('preferAppleScriptAPI', t('优先使用 AppleScript API'),
          t('在浏览器中，优先使用 AppleScript API 进行取词和文本替换。开启该选项，可提升取词的准确性和成功率。'))}
        {toggle('replaceCompatible', t('兼容模式替换'),
          t('启用后，文本替换将使用兼容模式，在部分应用中更可靠。'))}
        {toggle('autoSelectAllText', t('自动选中所有文本'),
          t('如果当前输入框中未选中文本，在翻译并替换操作时，允许自动全选文本内容'))}
        {toggle('removeBooksExcerpt', t('取词时，移除 Books 摘录中的版权信息'),
          t('从 Apple Books 划词时只查询引号内的正文，不翻译版权声明'))}
      </div>

      <div className="group">
        <h4>{t('查询文本处理')}</h4>
        {toggle('replaceNewlineSpace', t('查询时，自动将文本中的「换行符」替换为「空格」'), t('多行文本合并成一行再翻译'))}
        {toggle('autoRemoveComments', t('查询时，自动移除文本中的代码注释符号「/*#」'), t('划词或粘贴代码时去掉 //、# 与 /* */ 再翻译'))}
        {toggle('autoWordSegment', t('查询时，自动将单个单词文本分词为 key_value —> key value'), t('单个代码标识符按 _ 与驼峰拆词后再查询'))}
        <p className="noteline">{t('注意，这不会修改原文，仅对查询内容进行处理')}</p>
      </div>

      <div className="group">
        <h4>{t('OCR 设置')}</h4>
        {toggle('enableYoudaoOCR', t('有道 OCR 识别'),
          t('当默认OCR识别失败时，自动使用有道OCR'))}
        {toggle('ocrTextNormalize', t('启用 OCR 文本格式优化'),
          t('OCR 原始识别文本格式可能有误，开启该选项后，将会对文本进行格式优化处理。'))}
        {isMac && toggle('showOCRMenu', t('显示 OCR 菜单项'),
          t('在菜单栏中显示 OCR 相关的菜单项（截图 OCR、剪贴板 OCR、OCR 窗口）。'))}
        {toggle('hideScreenshotTip', t('隐藏截图提示'), t('开始截图时隐藏提示视图。'))}
      </div>

      <div className="group">
        <h4>{t('窗口管理')}</h4>
        <label className="field">
          <span className="fname">
            <SettingGlyph name="window" />
            {t('窗口最大高度百分比')}
          </span>
          <select
            value={cfg.maxHeightPct || 100}
            onChange={(e) => patch({ maxHeightPct: Number(e.target.value) })}
          >
            {[20, 30, 40, 50, 60, 70, 80, 90, 100].map((n) => (
              <option key={n} value={n}>{t('{n}%', { n })}</option>
            ))}
          </select>
        </label>
        {toggle('autoPin', t('当窗口显示时自动钉住'), t('主窗口每次出现时都置顶，方便对照原文'))}
      </div>

      <div className="group">
        <h4>{t('网络')}</h4>
        <ProviderField
          field={{ key: 'proxy', label: t('网络代理'), secret: false, required: false,
            placeholder: 'http://127.0.0.1:7890' }}
          value={cfg.proxy ?? ''}
          run={run}
          icon="network"
          onSave={(v) => patch({ proxy: v })}
        />
        <p className="noteline">{t('留空即跟随系统代理；只影响在线服务，苹果 OCR 与苹果词典不走这条链路。')}</p>
        <label className="field">
          <span className="fname">
            <SettingGlyph name="network" />
            {t('服务超时')}
          </span>
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
        <h4>{t('HTTP 服务器')}</h4>
        {toggle('enableHTTPServer', t('开启 HTTP 服务器'),
          t('只监听本机 127.0.0.1，供其他应用调用翻译、OCR 与取词，修改后立即生效'))}
        <ProviderField
          field={{ key: 'httpPort', label: t('HTTP 端口'), secret: false, required: false, placeholder: '8080' }}
          value={cfg.httpPort ?? '8080'}
          run={run}
          icon="server"
          onSave={(v) => patch({ httpPort: v })}
        />
        <p className="noteline">{t('修改端口后需要重新开启服务器')}</p>
        <p className="noteline">
          {t('POST /translate、POST /ocr、POST /detect、GET /selectedText。')}
        </p>
      </div>

      <div className="group">
        <h4>{t('日志')}</h4>
        <div className="field">
          <span className="fname">
            <SettingGlyph name="folder" />
            {t('日志目录')}
          </span>
          <span className="pathval" title={logDir}>{logDir || t('日志目录不可用')}</span>
        </div>
        <div className="row-end">
          <button className="tiny" onClick={() => void Service.OpenLogs()}>
            {t('显示调试日志窗口')}
          </button>
          <button className="tiny" disabled={!logDir}
            onClick={() => run(t('导出日志…'), () => Service.SaveLogFile(),
              (path) => path ? t('已导出到 {path}', { path }) : t('已取消导出'))}>
            {t('导出日志')}
          </button>
          <button className="tiny" disabled={!logDir}
            onClick={() => run(t('打开日志目录…'), () => SettingsService.RevealLogs())}>
            {t('在 Finder 中显示')}
          </button>
        </div>
      </div>

      <div className="group">
        <h4>{t('数据')}</h4>
        {toggle('saveHistory', t('记录查询历史'), t('关闭后新的查询不再写入历史；已有记录保留，可随时清理。'))}
        {toggle('vocabularyNotebook', t('生词本'),
          t('将每次查询追加保存到本地 JSONL 文件，供外部回顾程序读取。'))}
        {cfg.vocabularyNotebook && (
          <ProviderField
            field={{ key: 'vocabularyDir', label: t('存储目录'), secret: false, required: false, folder: true, placeholder: t('未选择') }}
            value={cfg.vocabularyDir ?? ''}
            run={run}
            icon="folder"
            onSave={(v) => patch({ vocabularyDir: v })}
          />
        )}
        {cfg.vocabularyNotebook && dirBad && (
          <p className="error">{t('所选目录不可写。')}</p>
        )}
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
      </>)}
    </div>
  )
}

// 收藏: the words starred in a query window or 历史. EasyDict keeps this list in
// settings as well, next to the history it comes from, so a word can be found
// without leaving the preferences window.
// 收藏 mirrors EasyDict's FavoritesTab: a 150pt segmented 历史/收藏 picker on
// top, a card whose header holds the section title plus trash and export icons,
// and rows that replay or delete their record. Empty sections get a large glyph
// instead of a sentence, which is how EasyDict draws them.
function Favorites({ setStatus }: { setStatus: (s: string) => void }) {
  const [tab, setTab] = useState<'favorites' | 'history'>('favorites')
  const [rows, setRows] = useState<Entry[] | null>(null)
  const [menu, setMenu] = useState(false)
  const fav = tab === 'favorites'

  const load = useCallback(() => {
    setRows(null)
    void Service.History('', fav, '', 2000)
      .then((list) => setRows(list ?? []))
      .catch(() => setRows([]))
  }, [fav])
  useEffect(() => { load() }, [load])
  // A favorite added by a quick action or a star in a query window lands in
  // the same store this page lists; reload on it the way EasyDict's tab does.
  useEffect(
    () =>
      onCommand((name) => {
        if (name === 'history-changed' || name === 'history-shown') load()
      }),
    [load],
  )
  useEffect(() => setMenu(false), [tab])

  const clear = () => {
    const call = fav ? Service.ClearFavorites() : Service.ClearHistory(0)
    void call
      .then((n) => {
        setStatus(fav ? t('已清空 {n} 条收藏', { n }) : t('已清理 {n} 条', { n }))
        load()
      })
      .catch((e) => setStatus(errMsg(e)))
  }

  const exportRecords = (wantFavorites: boolean) => {
    setMenu(false)
    void Service.SaveHistoryFile('', wantFavorites, '', 'csv')
      .then((path) => setStatus(path ? t('已导出到 {path}', { path }) : t('已取消导出')))
      .catch((e) => setStatus(errMsg(e)))
  }

  const remove = (id: string) => {
    void Service.DeleteHistory(id)
      .then(() => { setStatus(t('已删除')); load() })
      .catch((e) => setStatus(errMsg(e)))
  }

  const replay = (text: string) => {
    void Service.QueryText(text, 'main').catch((e) => setStatus(errMsg(e)))
  }

  const empty = rows !== null && rows.length === 0

  return (
    <div className="favpage">
      <div className="segbar favpick" role="tablist" aria-label={t('收藏')}>
        <button role="tab" aria-selected={!fav} className={fav ? '' : 'active'} onClick={() => setTab('history')}>
          {t('历史')}
        </button>
        <button role="tab" aria-selected={fav} className={fav ? 'active' : ''} onClick={() => setTab('favorites')}>
          {t('收藏')}
        </button>
      </div>

      <div className="favcard">
        <div className="favhead">
          <h3>{fav ? t('收藏的单词') : t('查询历史')}</h3>
          <div className="favactions">
            <ConfirmButton
              className="iconbtn danger"
              label={<TrashGlyph />}
              question={t('确认清空？')}
              title={fav ? t('清空所有收藏？此操作不可撤销。') : t('清空所有历史？收藏会保留。')}
              disabled={empty}
              onConfirm={clear}
            />
            <div className="menuwrap">
              <button
                className="iconbtn"
                title={t('导出')}
                aria-label={t('导出')}
                disabled={empty}
                onClick={() => setMenu(!menu)}
              >
                <ExportGlyph />
              </button>
              {menu && (
                <div className="qmenu">
                  <button onClick={() => exportRecords(true)}>{t('导出收藏')}</button>
                  <button onClick={() => exportRecords(false)}>{t('导出历史')}</button>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="favlist">
          {rows === null ? (
            <div className="favempty"><p>{t('读取中…')}</p></div>
          ) : empty ? (
            <div className="favempty">
              {fav ? <StarSlashGlyph /> : <ClockXGlyph />}
              <p>{fav ? t('暂无收藏') : t('暂无查询记录')}</p>
            </div>
          ) : (
            rows.map((r) => (
              <div className="favrow" key={r.id}>
                <div className="favtext">
                  <strong>{r.source}</strong>
                  {r.target && <span className="favtarget">{r.target}</span>}
                  <span className="favtime">{stamp(r.createdAt)}</span>
                </div>
                <div className="favactions">
                  <button className="iconbtn" title={t('查询')} onClick={() => replay(r.source)}>
                    <SearchGlyph /><span>{t('查询')}</span>
                  </button>
                  <button className="iconbtn danger" title={t('删除')} aria-label={t('删除')} onClick={() => remove(r.id)}>
                    <TrashGlyph />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

// EasyDict shows a fixed timestamp (dateStyle .medium / timeStyle .short) in
// this list rather than the relative "1 天前" the query window uses.
function stamp(sec: number): string {
  // The date follows the interface, not the OS: an English window must not
  // print 2026年11月15日.
  const locale = uiLocale() === 'en' ? 'en-US' : undefined
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(sec * 1000))
}

function TrashGlyph() {
  return (
    <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true">
      <path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.5 8h6l.5-8" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function ExportGlyph() {
  return (
    <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true">
      <path d="M8 10V2.5M5.2 5.2 8 2.4l2.8 2.8M3 10.5v2.2c0 .5.4.8.8.8h8.4c.4 0 .8-.3.8-.8v-2.2" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function SearchGlyph() {
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
      <circle cx="7" cy="7" r="4.2" fill="none" stroke="currentColor" strokeWidth="1.4" />
      <path d="m10.2 10.2 3.1 3.1" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

function StarSlashGlyph() {
  return (
    <svg viewBox="0 0 48 48" width="48" height="48" aria-hidden="true">
      <path d="M24 8.5l4.4 9 9.9 1.4-7.2 7 1.7 9.9-8.8-4.7-8.8 4.7 1.7-9.9-7.2-7 9.9-1.4z" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      <path d="M10 40 38 8" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
  )
}

function ClockXGlyph() {
  return (
    <svg viewBox="0 0 48 48" width="48" height="48" aria-hidden="true">
      <circle cx="24" cy="24" r="16.5" fill="none" stroke="currentColor" strokeWidth="2" />
      <path d="M24 14.5V24l6.5 4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="m33 33 8 8M41 33l-8 8" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
  )
}

// 禁止名单: apps whose selections never start a query. EasyDict keeps it on its
// own tab because it is a list that grows, not a switch on the mouse page.
function Blocklist({ cfg, run, setStatus }: {
  cfg: Config
  run: (label: string, call: () => Promise<unknown>, after?: (path: string) => string) => void
  setStatus: (text: string) => void
}) {
  const [selected, setSelected] = useState('')
  const [icons, setIcons] = useState<Record<string, string>>({})
  const list = cfg.blocklist ?? []

  // EasyDict draws the app's own Finder icon in each row; the Go side renders
  // it once and the page keeps the data URL for the life of the window.
  useEffect(() => {
    for (const app of list) {
      if (icons[app.bundleID] !== undefined) continue
      void Service.AppIcon(app.bundleID)
        .then((url) => setIcons((prev) => ({ ...prev, [app.bundleID]: url ?? '' })))
        .catch(() => setIcons((prev) => ({ ...prev, [app.bundleID]: '' })))
    }
  }, [list, icons])

  const patch = (change: Partial<Config>) =>
    run(t('保存…'), async () => {
      const fresh = await SettingsService.Get()
      await SettingsService.Save({ ...fresh, ...change } as Config)
    })

  if (!isMac && !isWindows) {
    return (
      <div className="slist">
        <div className="group">
          <h4>{t('禁止名单')}</h4>
          <p className="noteline">{t('禁止名单只影响鼠标划词，当前平台没有自动划词。')}</p>
        </div>
      </div>
    )
  }

  const add = () => {
    void Service.PickBlockedApp()
      .then((app) => {
        if (!app?.bundleID) return
        if (list.some((a) => a.bundleID === app.bundleID)) return
        return patch({ blocklist: [...list, app] })
      })
      .catch((e) => setStatus(errMsg(e)))
  }

  const removeSelected = () => {
    if (!selected) return
    setSelected('')
    void patch({ blocklist: list.filter((a) => a.bundleID !== selected) })
  }

  return (
    <div className="blkpage">
      <h3>{t('禁止名单')}</h3>
      <div className="blkcard">
        <div className="blklist" role="listbox" aria-label={t('禁止名单')}>
          {list.length === 0 && (
            <p className="noteline blkempty">{t('名单为空：所有应用都会触发自动划词。')}</p>
          )}
          {list.map((app) => {
            const name = app.name || app.bundleID
            return (
              <div
                key={app.bundleID}
                role="option"
                aria-selected={selected === app.bundleID}
                tabIndex={0}
                className={`blkrow${selected === app.bundleID ? ' on' : ''}`}
                onClick={(e) => { e.currentTarget.focus(); setSelected(app.bundleID) }}
                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setSelected(app.bundleID) } }}
              >
                {icons[app.bundleID]
                  ? <img className="blkapp" src={icons[app.bundleID]} alt="" />
                  : <span className="blkicon">{(name.trim()[0] ?? '?').toUpperCase()}</span>}
                <span className="blkname">{name}</span>
              </div>
            )
          })}
        </div>
        <div className="blktools">
          <button aria-label={t('添加')} title={t('添加')} onClick={add}><PlusGlyph /></button>
          <span className="blksep" />
          <button aria-label={t('移除')} title={t('移除')} disabled={!selected} onClick={removeSelected}><MinusGlyph /></button>
        </div>
      </div>
    </div>
  )
}

// 隐私: what leaves the machine and what stays. EasyDict also has crash log and
// analytics switches here; this app has neither, so the statement and the two
// macOS grants are the whole page.
function Privacy() {
  return (
    <div className="slist">
      <div className="group">
        <h4>{t('隐私声明')}</h4>
        <p className="noteline">
          {t('查询的文字与截图只会发送给「服务」页里启用的服务；历史、收藏、生词本与配置保存在本机用户目录，不会上传。')}
        </p>
      </div>
      {isMac && <Permissions />}
    </div>
  )
}

// The ten values an Anki template can name, in EasyDict's own order. The token
// is what the template writes; the label is what the menu offers.
const ankiVars: [string, string][] = [
  ['Word', t('单词')],
  ['Translation', t('翻译')],
  ['Definition', t('释义')],
  ['Phonetic', t('音标')],
  ['Exchange', t('词形变化')],
  ['Related', t('相关词')],
  ['FullResult', t('完整结果')],
  ['DictionaryText', t('词典文本')],
  ['DictionaryHTML', t('词典 HTML')],
  ['Etymology', t('词源')],
]

// ankiNames maps the note object ankiNote returns to the variables a template
// spells, so 预览 fills {Word} from note.word.
const ankiNames: Record<string, string> = {
  word: 'Word',
  translation: 'Translation',
  definition: 'Definition',
  phonetic: 'Phonetic',
  exchange: 'Exchange',
  related: 'Related',
  fullResult: 'FullResult',
  dictionaryText: 'DictionaryText',
  dictionaryHTML: 'DictionaryHTML',
  etymology: 'Etymology',
}

// Anki is EasyDict's review loop: a looked-up word goes straight into a deck
// through AnkiConnect, which runs inside Anki and speaks a local HTTP API. The
// fields below describe one destination; 添加到 Anki on a dictionary card is the
// only writer, so nothing is sent until the user asks for it.
function Anki({ cfg, run, setStatus }: {
  cfg: Config
  run: (label: string, call: () => Promise<unknown>) => void
  setStatus: (text: string) => void
}) {
  const [testing, setTesting] = useState(false)
  const [fields, setFields] = useState<string[]>([])
  const [reading, setReading] = useState(false)
  // 预览 runs one word through a dictionary, as EasyDict's preview row does:
  // a canned sample could not show a template that names a variable the chosen
  // dictionary never answers with.
  const dicts = (cfg.dict ?? []).filter((p) => p.enabled)
  const [previewSvc, setPreviewSvc] = useState('')
  const [previewWord, setPreviewWord] = useState('')
  const [previewing, setPreviewing] = useState(false)
  const [previewNote, setPreviewNote] = useState<Record<string, string> | null>(null)
  const [previewErr, setPreviewErr] = useState('')
  const svcID = dicts.some((d) => d.id === previewSvc) ? previewSvc : dicts[0]?.id ?? ''

  const patch = (change: Partial<Config>) =>
    run(t('保存…'), async () => {
      const fresh = await SettingsService.Get()
      await SettingsService.Save({ ...fresh, ...change } as Config)
    })

  // The check reads the saved config, so an unsaved address is not what gets
  // tested; the status line says what came back rather than opening a dialog.
  const test = () => {
    setTesting(true)
    setStatus(t('连接中…'))
    void Service.AnkiTest()
      .then((v) => setStatus(t('已连接 AnkiConnect {v}', { v: String(v) })))
      .catch((e) => setStatus(errMsg(e)))
      .finally(() => setTesting(false))
  }

  const field = (key: keyof Config, label: string, icon: IconName, placeholder?: string) => (
    <label className="field" key={key}>
      <span className="fname">
        <SettingGlyph name={icon} />
        {label}
      </span>
      <input
        value={(cfg[key] as string | undefined) ?? ''}
        placeholder={placeholder}
        spellCheck={false}
        onChange={(e) => patch({ [key]: e.target.value } as Partial<Config>)}
      />
    </label>
  )

  // AnkiConnect knows how the note model spells its fields, so once they are
  // read each row becomes a picker — a typed field name that does not match
  // makes a note whose content sits in no visible field.
  const mappings = cfg.ankiMappings ?? []
  const setMappings = (list: AnkiMapping[]) => patch({ ankiMappings: list })
  const setMapping = (i: number, change: Partial<AnkiMapping>) =>
    setMappings(mappings.map((m, j) => (j === i ? { ...m, ...change } : m)))
  const defaultTemplate = (i: number) => (i === 0 ? '{Word}' : '{FullResult}')

  // EasyDict's duplicate_fields: a note field holds one value, so two rows
  // naming the same field would silently lose the first template.
  const seenFields = new Set<string>()
  const dupField = mappings.some((m) => {
    const name = (m.field ?? '').trim()
    if (!name) return false
    if (seenFields.has(name)) return true
    seenFields.add(name)
    return false
  })

  const readFields = () => {
    setReading(true)
    setStatus(t('读取字段…'))
    void Service.AnkiFields()
      .then((names) => {
        const list = names ?? []
        setFields(list)
        // EasyDict's insufficient_fields: a note type with no fields cannot
        // hold anything, so the count would read as a successful read.
        setStatus(list.length === 0
          ? t('笔记类型没有可用字段')
          : t('读取到 {n} 个字段', { n: String(list.length) }))
      })
      .catch((e) => setStatus(errMsg(e)))
      .finally(() => setReading(false))
  }

  const fillNote = (note: Record<string, string>, tpl: string | undefined) =>
    Object.entries(note).reduce(
      (text, [key, value]) => text.split(`{${ankiNames[key] ?? key}}`).join(value),
      tpl ?? '',
    )

  const runPreview = () => {
    if (!previewWord.trim()) {
      setPreviewErr(t('请输入要预览的单词'))
      setPreviewNote(null)
      return
    }
    if (!svcID) {
      setPreviewErr(t('请选择一个词典服务'))
      setPreviewNote(null)
      return
    }
    setPreviewing(true)
    setPreviewErr('')
    void Service.AnkiPreview(svcID, previewWord.trim())
      .then((entries) => setPreviewNote(ankiNote(entries ?? [], '')))
      .catch((e) => { setPreviewErr(errMsg(e)); setPreviewNote(null) })
      .finally(() => setPreviewing(false))
  }

  return (
    <div className="slist">
      <div className="group">
        <h4>{t('Anki Connect')}</h4>
        <label className="switch">
          <input
            type="checkbox"
            checked={!!cfg.ankiEnabled}
            aria-label={t('启用 Anki Connect')}
            onChange={(e) => patch({ ankiEnabled: e.target.checked })}
          />
          <span className="swbody">
            <SettingGlyph name="cards" />
            <span>
              <strong>{t('启用 Anki Connect')}</strong>
              <em>{t('词典卡片出现「添加到 Anki」，把词条存进下面的牌组')}</em>
            </span>
          </span>
        </label>
        <div className="fields">
          {field('ankiEndpoint', t('端点'), 'server', 'http://127.0.0.1:8765')}
          {field('ankiDeck', t('牌组'), 'cards', 'Default')}
          {field('ankiModel', t('笔记类型'), 'document', 'Basic')}
        </div>
        <p className="noteline">{t('默认 Anki Connect 地址为 http://127.0.0.1:8765')}</p>
        <div className="row-end">
          <button className="tiny" disabled={testing} onClick={test}>{t('测试连接')}</button>
        </div>
        {!cfg.ankiEnabled && <p className="noteline">{t('Anki Connect 未启用：卡片上不会出现添加按钮。')}</p>}
      </div>

      <div className="group">
        <h4>{t('Anki 字段')}</h4>
        <p className="noteline">{t('每行把一个字段名和内容模板配在一起；模板可用 {Word}、{Translation}、{Definition}、{Phonetic}、{Exchange}、{Related}、{FullResult}、{DictionaryText}、{DictionaryHTML}、{Etymology}，字段名要和笔记类型里的完全一致。')}</p>
        {mappings.map((m, i) => (
          <div className="ankimap" key={i}>
            {fields.length > 0 ? (
              <select
                value={fields.includes(m.field) ? m.field : ''}
                aria-label={t('字段名')}
                onChange={(e) => setMapping(i, { field: e.target.value })}
              >
                <option value="">{t('选择字段…')}</option>
                {fields.map((f) => (
                  <option key={f} value={f}>{f}</option>
                ))}
              </select>
            ) : (
              <input
                value={m.field ?? ''}
                placeholder={i === 0 ? 'Front' : 'Back'}
                aria-label={t('字段名')}
                spellCheck={false}
                onChange={(e) => setMapping(i, { field: e.target.value })}
              />
            )}
            <input
              value={m.template ?? ''}
              placeholder={defaultTemplate(i)}
              aria-label={t('内容模板')}
              spellCheck={false}
              onChange={(e) => setMapping(i, { template: e.target.value })}
            />
            <select
              className="ankivar"
              value=""
              aria-label={t('插入变量')}
              onChange={(e) => {
                if (!e.target.value) return
                setMapping(i, { template: `${m.template ?? ''}{${e.target.value}}` })
              }}
            >
              <option value="">{t('插入变量…')}</option>
              {ankiVars.map(([token, label]) => (
                <option key={token} value={token}>{`${label} {${token}}`}</option>
              ))}
            </select>
            <button
              className="tiny"
              aria-label={t('删除映射')}
              disabled={mappings.length <= 1}
              onClick={() => setMappings(mappings.filter((_, j) => j !== i))}
            >
              ×
            </button>
          </div>
        ))}
        {dupField && <div className="error">{t('映射的 Anki 字段不能重复')}</div>}
        <div className="row-end">
          <button className="tiny" onClick={() => setMappings([...mappings, { field: '', template: '' }])}>
            {t('添加映射')}
          </button>
          <button className="tiny" disabled={reading} onClick={readFields}>
            {t('从 Anki 读取字段')}
          </button>
        </div>
        <div className="ankipreview">
          <span className="byline">{t('预览')}</span>
          <div className="ankipreviewbar">
            <select
              value={svcID}
              aria-label={t('词典服务')}
              onChange={(e) => { setPreviewSvc(e.target.value); setPreviewNote(null); setPreviewErr('') }}
            >
              {dicts.map((d) => (
                <option key={d.id} value={d.id}>{d.label || d.kind}</option>
              ))}
            </select>
            <input
              value={previewWord}
              placeholder={t('单词')}
              aria-label={t('单词')}
              spellCheck={false}
              onChange={(e) => { setPreviewWord(e.target.value); setPreviewNote(null); setPreviewErr('') }}
            />
            <button className="tiny" disabled={previewing} onClick={runPreview}>
              {t('预览')}
            </button>
          </div>
          {previewErr && <div className="error">{previewErr}</div>}
          {previewNote && mappings.map((m, i) => (
            <div key={i}>
              <strong>{m.field || (i === 0 ? 'Front' : 'Back')}</strong>
              <p>{fillNote(previewNote, m.template || defaultTemplate(i))}</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// About is EasyDict's 关于 page: a 100pt logo, the name, the version and the
// copyright, then three links — project page, contributors, acknowledgements.
// It is a card, not a form: the privacy statement has its own tab now.
function About({ setStatus }: { setStatus: (text: string) => void }) {
  const [version, setVersion] = useState('')
  const [ack, setAck] = useState(false)
  useEffect(() => {
    void SettingsService.Version().then((v) => setVersion(v ?? ''))
  }, [])

  return (
    <div className="about">
      <img className="abouticon" src="/appicon.png" alt="" width={100} height={100} />
      <div className="aboutcol">
        <h2>Transdict</h2>
        <p className="aboutversion">{version ? t('当前版本 {v}', { v: version }) : ''}</p>
        <p className="aboutcopyright">Copyright © 2026 Transdict. All rights reserved.</p>
        <div className="aboutlinks">
          <button onClick={() => void Service.OpenProjectPage().catch((e) => setStatus(errMsg(e)))}>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth="1.6" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 3.6l2.5 5.1 5.6.8-4 3.9.9 5.6-5-2.6-5 2.6.9-5.6-4-3.9 5.6-.8z" />
            </svg>
            {t('GitHub 项目主页')}
          </button>
          <button onClick={() => void Service.OpenContributorsPage().catch((e) => setStatus(errMsg(e)))}>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth="1.6" strokeLinecap="round" aria-hidden="true">
              <circle cx="9" cy="8.2" r="2.6" />
              <circle cx="16.3" cy="9.2" r="2.1" />
              <path d="M3.6 18.4c0-2.6 2.4-4.3 5.4-4.3s5.4 1.7 5.4 4.3" />
              <path d="M15.2 14.6c2.6.2 4.8 1.7 4.8 3.8" />
            </svg>
            {t('贡献者')}
          </button>
          <button onClick={() => setAck(true)}>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 3l2.2 1.7 2.8-.2 1 2.6 2.4 1.4-.7 2.7 1.3 2.5-2 1.9-.4 2.8-2.7.6-1.6 2.3-2.6-1-2.6 1-1.6-2.3-2.7-.6-.4-2.8-2-1.9L4.3 12l-.7-2.7L6 7.9l1-2.6 2.8.2z" />
              <path d="M8.9 12.3l2.1 2.1 4.2-4.3" />
            </svg>
            {t('致谢')}
          </button>
        </div>
      </div>

      {ack && (
        <div className="aboutack" role="dialog" aria-modal="true" aria-label={t('致谢')}>
          <div className="aboutackcard">
            <h3>{t('致谢')}</h3>
            <p className="noteline">
              {t('界面基于 Wails 与 React；识别与语言判断使用苹果的 Vision 与自然语言框架；翻译、词典与朗读由你启用的服务提供。')}
            </p>
            <div className="btnrow">
              <button className="primary tiny" onClick={() => setAck(false)}>{t('好')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// PageIcon is the sidebar's visual index: a sliders, window, keyboard and gear
// glyph. They are inline so the icon colour follows the active row.
function PageIcon({ id }: { id: Page }) {
  const paths: Record<Page, ReactNode> = {
    general: (
      <>
        <path d="M20.50 15.52L17.23 17.23L15.52 20.50L12.00 19.40L8.48 20.50L6.77 17.23L3.50 15.52L4.60 12.00L3.50 8.48L6.77 6.77L8.48 3.50L12.00 4.60L15.52 3.50L17.23 6.77L20.50 8.48L19.40 12.00Z" />
        <circle cx="12" cy="12" r="3.2" />
      </>
    ),
    services: (
      <>
        <rect x="3" y="7" width="18" height="13" rx="2.5" />
        <path d="M8.5 7V5.5A1.5 1.5 0 0 1 10 4h4a1.5 1.5 0 0 1 1.5 1.5V7M3 12.5h18" />
      </>
    ),
    favorites: (
      <path d="M12 3.6l2.5 5.1 5.6.8-4 3.9.9 5.6-5-2.6-5 2.6.9-5.6-4-3.9 5.6-.8z" />
    ),
    disabled: (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M5.8 5.8l12.4 12.4" />
      </>
    ),
    shortcuts: (
      <>
        <rect x="3" y="3" width="18" height="18" rx="4" />
        <g transform="translate(12 12) scale(0.6) translate(-12 -12)">
          <path
            d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3"
            strokeWidth="2.2"
          />
        </g>
      </>
    ),
    advanced: (
      <>
        <path d="M14.80 14.60L13.01 16.73L12.47 19.45L9.69 19.38L7.22 20.64L5.54 18.43L3.01 17.29L3.70 14.60L3.01 11.91L5.54 10.77L7.22 8.56L9.69 9.82L12.47 9.75L13.01 12.47Z" />
        <circle cx="8.6" cy="14.6" r="2.1" />
        <path d="M21.40 7.20L20.08 8.59L19.82 10.48L17.91 10.32L16.27 11.29L15.20 9.70L13.42 9.02L14.00 7.20L13.42 5.38L15.20 4.70L16.27 3.11L17.91 4.08L19.82 3.92L20.08 5.81Z" />
        <circle cx="17.2" cy="7.2" r="1.4" />
      </>
    ),
    privacy: (
      <>
        <rect x="3" y="3" width="18" height="18" rx="4" />
        <path d="M9 11.5V7.6a1 1 0 0 1 2 0v3.4M11 11V6.6a1 1 0 0 1 2 0V11M13 11.2V7.8a1 1 0 0 1 2 0v5.4c0 2.5-1.6 4.3-4 4.3s-4-2-4-4.3v-1.6a1 1 0 0 1 2 0" />
      </>
    ),
    about: (
      <>
        <path d="M21 11.6a8.4 8.4 0 0 1-9.2 8.3L5 21l1.1-3.4a8.4 8.4 0 1 1 14.9-6z" />
        <path d="M12 10.6v5M12 7.6h.01" />
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
