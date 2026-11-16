// The translation window: one input, one answer. Preferences and history each
// live in their own window, which is what keeps this one narrow enough to sit
// beside the app the text was copied from.
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ResultColumns } from './Result'
import {
  Service, SettingsService, accelHint, accelLabel, applyTheme, errMsg, flagFor, isMac, langName, langOption, onBusy, onCommand, onConfig, onMenuAction, onResult,
  openQuickLink, pickableLangs, readAloud, roleOf, stopSpeech, toggleSpeech, useLangNames, usePlayingKey,
  type Config, type Lang, type Response,
} from './api'
import { speechKey } from './speech'
import { planSwap } from './swap'
import { appAccel, matchesAccel, type AppAction } from './appkeys'
import { applyLocale, t } from './i18n'
import { quickLink, type QuickLink } from './links'
import { autoReadAnswer } from './autoread'
import { firstResult } from './copy'
import { copySelection, cutSelection } from './editing'
import WindowControls from './WindowControls'
import { useInputMenu } from './inputmenu'
import { useAutoQuery } from './autoquery'
import { DetectChip } from './DetectChip'
import { QuickActionGlyph, SwapIcon } from './QueryTools'
import { DictionaryMark, EudicMark, GoogleMark } from './QuickLinkIcons'
import { detectedLabel, resolveDetected, useTypingDetection } from './detect'

// AI instructions reuse the same providers with a different prompt, so 润色 and
// 总结 need no second pipeline.
const instructions: { label: string; text: string }[] = [
  { label: t('润色'), text: t('在不改变原意的前提下润色下面的文字，使其更地道、通顺：') },
  { label: t('总结'), text: t('用三句以内的中文总结下面的文字：') },
  { label: t('更正式'), text: t('把下面的文字改写得更正式、书面化：') },
  { label: t('更口语'), text: t('把下面的文字改写得更口语、自然：') },
  { label: t('纠错'), text: t('纠正下面文字中的拼写、语法与标点错误，只输出修改后的文字：') },
  { label: t('扩写'), text: t('在保持原意的前提下扩写下面的文字：') },
]

// The language row drops its flag emoji at the minimum width: an English
// "Simplified Chinese" needs those pixels to fit the label instead of being
// cut mid-word.
function useNarrow(): boolean {
  const [narrow, setNarrow] = useState(() => window.innerWidth < 460)
  useEffect(() => {
    const onResize = () => setNarrow(window.innerWidth < 460)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return narrow
}

export default function Panel() {
  const [res, setRes] = useState<Response | null>(null)
  // 识别为 is EasyDict's detect button: it reads like a label but opens the
  // language menu, and picking one asks again with that language instead of the
  // guessed one (EZQueryView's detectButton). The choice stays tied to the text
  // it was made for, the way specifiedTextLanguageDict does.
  const [forced, setForced] = useState<{ text: string; code: string } | null>(null)
  // Which clip the shared player is reading; the speakers turn into 停止 while
  // their own text is the one sounding, as EasyDict's EZAudioButton does.
  const playing = usePlayingKey()
  const [busy, setBusy] = useState('')
  // note is the window's one-line answer to "why did that button do nothing" —
  // a missing voice, an unrecorded query, a clipboard that refused the write.
  const [note, setNote] = useState('')
  // permNeed names the system grant an action was refused for, so the note can
  // carry the one button that fixes it instead of only naming a checkbox the
  // user then has to find.
  const [permNeed, setPermNeed] = useState('')
  const [cfg, setCfg] = useState<Config | null>(null)
  const [langs, setLangs] = useState<Lang[]>([])
  const [text, setText] = useState('')
  // The chip appears on the recognition pass EasyDict runs a second after the
  // text settles, not only once an answer comes back.
  const typed = useTypingDetection(text)
  const [source, setSource] = useState('auto')
  const [target, setTarget] = useState('zh-CN')
  const [dictOnly, setDictOnly] = useState(false)
  const [pinned, setPinned] = useState(false)
  // menu is the titlebar 快捷动作 popover.
  const [menu, setMenu] = useState(false)
  const narrow = useNarrow()
  // aiKinds is the set of translate kinds that take an instruction. Without one
  // of them the 润色/总结 row would send a plain sentence translator a prompt it
  // cannot honour and quietly return an ordinary translation.
  const [aiKinds, setAiKinds] = useState<Set<string>>(new Set())
  const langNames = useLangNames()
  const box = useRef<HTMLTextAreaElement | null>(null)
  // pinPending covers the window between the click and the store's answer: the
  // focus-event reload below reads the store, and an older snapshot arriving
  // after the toggle would repaint the pin in the state the user just left.
  const pinPending = useRef(false)
  // textRef and sent track what the box holds against what it last submitted,
  // so a result that did not come from the box can clear an untouched draft.
  const textRef = useRef('')
  const sent = useRef('')
  // wantSelect is 打开窗口时自动选中查询文本: the query comes back selected, so typing
  // replaces it. It stays armed until the text that was restored has landed, and
  // a keystroke disarms it so the user's own typing is never swallowed.
  const wantSelect = useRef(false)

  // 主窗高度跟随内容: EasyDict's query windows are as tall as their answer and
  // only the width is the user's to drag. The column scrolls, so its own
  // scrollHeight only echoes the frame; the cards inside it, the chrome above
  // them and the footer are the real content. Go keeps the top edge and caps the
  // result at 窗口最大高度百分比. Partial answers are skipped: a stream resizing
  // the window on every delta reads as a twitch, and the final answer fits once.
  useEffect(() => {
    if (res?.partial) return
    let timer = 0
    const fit = () => {
      window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        const shell = document.querySelector('.shell') as HTMLElement | null
        if (!shell) return
        const h = (el: Element | null) => el?.getBoundingClientRect().height ?? 0
        const content = document.querySelector('.content.compact')
        const col = content?.querySelector('.col') ?? null
        let inner = 0
        if (col) {
          const kids = Array.from(col.children) as HTMLElement[]
          const gap = parseFloat(getComputedStyle(col).rowGap) || 0
          inner = kids.reduce((n, k) => n + k.getBoundingClientRect().height, 0)
            + Math.max(0, kids.length - 1) * gap
        }
        if (content) {
          const cs = getComputedStyle(content)
          inner += parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom)
        }
        // EasyDict keeps a minimum results area under the input even when the
        // answer is empty, so the empty window is a small shell, not a strip.
        inner = Math.max(inner, 40)
        // Margins belong to the window height too: the compose card and the
        // language bar each sit 9pt above the next row, and a fit that counts
        // only border boxes ends up clipping the last action row.
        const outer = (el: Element | null) => {
          if (!el) return 0
          const cs = getComputedStyle(el)
          return h(el) + parseFloat(cs.marginTop) + parseFloat(cs.marginBottom)
        }
        const chrome = outer(shell.querySelector('.dragbar'))
          + outer(shell.querySelector('.compose'))
          + outer(shell.querySelector('.langbar'))
          + outer(shell.querySelector(':scope > .noteline'))
          + outer(shell.querySelector('.footer'))
        void Service.FitMain(Math.ceil(chrome + inner))
      }, 60)
    }
    const ro = new ResizeObserver(fit)
    const watch = (el: Element | null) => { if (el) ro.observe(el) }
    const shell = document.querySelector('.shell')
    for (const sel of ['.dragbar', '.compose', '.langbar', ':scope > .noteline', '.footer']) {
      watch(shell?.querySelector(sel) ?? null)
    }
    document.querySelectorAll('.content.compact .col > *').forEach(watch)
    window.addEventListener('resize', fit)
    // The panel also mounts while the window is hidden (启动后隐藏主窗口); the
    // first fit then belongs to the moment the window actually shows.
    window.addEventListener('focus', fit)
    document.addEventListener('visibilitychange', fit)
    fit()
    return () => {
      window.clearTimeout(timer)
      ro.disconnect()
      window.removeEventListener('resize', fit)
      window.removeEventListener('focus', fit)
      document.removeEventListener('visibilitychange', fit)
    }
  }, [res, note, cfg])

  // 应用内查询 on the input box, EasyDict's EZQueryMenuTextView: the rewritten
  // text is the same draft the onChange path stores, so a cut or paste from the
  // menu leaves textRef and the built-in 粘贴后自动查询 behaviour consistent.
  const inputMenu = useInputMenu({
    box,
    from: 'main',
    onChange: (next) => {
      wantSelect.current = false
      setText(next)
      textRef.current = next
    },
    onPasted: (next) => {
      // The menu is this box's paste path; it follows the same rule as ⌘V.
      cancelAuto()
      if (cfg?.autoQueryPaste) run(undefined, undefined, next)
      else sent.current = next.trim()
    },
    notice: setNote,
    speakLang: source !== 'auto' ? source : res?.sourceLang || res?.detected || '',
  })

  // cfgRef keeps the event handlers from closing over a stale config.
  const cfgRef = useRef<Config | null>(null)

  useEffect(() => {
    if (!note) return
    // A permission note is a sentence plus a trip through System Settings;
    // five seconds would clear it before the user finished reading.
    const wait = permNeed ? 20000 : 5000
    const timer = setTimeout(() => {
      setNote('')
      setPermNeed('')
    }, wait)
    return () => clearTimeout(timer)
  }, [note, permNeed])

  const applyCfg = useCallback((next: Config) => {
    cfgRef.current = next
    setCfg(next)
    setSource(next.sourceLang || 'auto')
    setTarget(next.targetLang)
    if (!pinPending.current) setPinned(!!next.pinned)
    applyTheme(next.theme)
    applyLocale(next.uiLang)
    document.documentElement.dataset.fs = next.resultFont || '1'
  }, [])

  // The Aa button walks the five reading sizes the way EasyDict's does; the
  // stylesheet keys them off the attribute so no px value lives in this file.
  // The note names the step: with no answer on screen the only thing a click
  // changes is the stored size, so silence would read as a dead button.
  const useFont = (size: string) => {
    if (size === (cfgRef.current?.resultFont || '1')) return
    document.documentElement.dataset.fs = size
    if (cfgRef.current) cfgRef.current = { ...cfgRef.current, resultFont: size }
    setCfg((c) => (c ? { ...c, resultFont: size } : c))
    void Service.SetResultFont(size)
      .then(() => setNote(t('字号 {n}/5', { n: size })))
      .catch((e) => setNote(errMsg(e)))
  }
  const cycleFont = () => {
    const order = ['1', '2', '3', '4', '5']
    useFont(order[(order.indexOf(cfgRef.current?.resultFont || '1') + 1) % order.length])
  }
  // ⌘+ and ⌘- stop at the ends rather than wrap, the way a size control reads.
  const stepFont = (delta: number) => {
    const order = ['1', '2', '3', '4', '5']
    const i = order.indexOf(cfgRef.current?.resultFont || '1') + delta
    useFont(order[Math.min(order.length - 1, Math.max(0, i))])
  }

  const flipPin = () => {
    pinPending.current = true
    void Service.TogglePin()
      .then((on) => {
        setPinned(on)
        if (cfgRef.current) cfgRef.current = { ...cfgRef.current, pinned: on }
      })
      .finally(() => {
        pinPending.current = false
      })
  }

  // 快捷动作 is EasyDict's titlebar menu: the same rewrites the query-time
  // options apply, on demand and against the draft in the composer. The menu
  // edits the question only — EasyDict rewrites inputText and leaves the
  // answer alone.
  const rewrite = (action: string) => {
    setMenu(false)
    const value = (text.trim() || res?.sourceText || '').trim()
    if (!value) return
    void Service.TransformText(value, action)
      .then((out) => {
        textRef.current = out
        setText(out)
        box.current?.focus()
      })
      .catch((e) => setNote(errMsg(e)))
  }

  // 添加到收藏 favourites the text in the box, not a history row: 记录查询历史
  // can be off and a streaming answer has no row yet, while EasyDict records the
  // favourite regardless. It only ever adds — dropping a favourite is the 收藏
  // tab's job — so the menu item stays put after use.
  const star = () => {
    setMenu(false)
    const source = (text.trim() || res?.sourceText || '').trim()
    if (!source) return
    void Service.FavoriteQuery(source, res ? firstResult(res) : '',
      res?.detected ?? '', res?.targetLang || cfgRef.current?.targetLang || '')
      .then(() => setNote(t('已加入收藏')))
      .catch((e) => setNote(errMsg(e)))
  }

  // A new answer starts with no menu left over from the last one.
  useEffect(() => {
    setMenu(false)
  }, [res?.id])

  // The preferences window can change the target language while this one is
  // open, so the select re-reads the store whenever the window comes forward.
  // 打开窗口时自动选中查询文本 rides the same focus: the text was restored when the
  // window loaded, and this is the moment the user is looking at it again.
  useEffect(() => {
    const sync = () => {
      void SettingsService.Get().then((fresh) => {
        applyCfg(fresh)
        if (!fresh?.selectTextOnOpen) return
        wantSelect.current = true
        requestAnimationFrame(() => {
          const el = box.current
          if (!wantSelect.current || !el || !el.value) return
          el.select()
          wantSelect.current = false
        })
      })
    }
    window.addEventListener('focus', sync)
    return () => window.removeEventListener('focus', sync)
  }, [applyCfg])

  // A restored query can land after the command that opened the window (the Go
  // side stores it while the bundle is still loading); select it when it does.
  useEffect(() => {
    if (!wantSelect.current || !text) return
    wantSelect.current = false
    box.current?.select()
  }, [text])

  useEffect(() => {
    const offResult = onResult('main', (next) => {
      setRes(next)
      // A partial answer carries the recognized text only; the translation
      // stage is still running, so the progress line stays and nothing is read.
      if (next.partial) return
      setBusy('')
      setNote('')
      setPermNeed('')
      // A clipboard, screenshot or selection answer replaces whatever the box
      // held: EasyDict's EZQueryView always shows the text that was asked, and
      // the box is where 拷贝文本 and the quick links read it from.
      if (next.mode !== 'input' && next.mode !== 'prompt' && next.sourceText) {
        textRef.current = next.sourceText
        sent.current = next.sourceText
        setText(next.sourceText)
      }
      autoRead(next, cfgRef.current, setNote)
    })
    const offBusy = onBusy('main', setBusy)
    // 窗口配置 decides whether the composer is drawn; a switch flipped in
    // settings has to reach the panel that is already open.
    const offCfg = onConfig(() => {
      void SettingsService.Get().then(applyCfg)
    })
    const offCommand = onCommand((name) => {
      if (name !== 'input') return
      box.current?.focus()
      // Read the store rather than the last snapshot: the shortcut fires right
      // after another window changed a switch, before this one has heard about
      // it, and both 输入框 options below are decided at this moment.
      void SettingsService.Get()
        .then((fresh) => {
          // 输入翻译时，清空查询内容 starts the box empty; the answer is left
          // alone, because the switch is about the question, not the result.
          if (fresh?.clearInputOnQuery) {
            setText('')
            textRef.current = ''
            sent.current = ''
          }
          wantSelect.current = !!fresh?.selectTextOnOpen
          if (!wantSelect.current) return
          // The webview may already hold the restored text, in which case there
          // is nothing to wait for; a rebuilt window fills the box a moment
          // later and the effect below finishes the selection.
          requestAnimationFrame(() => {
            const el = box.current
            if (!wantSelect.current || !el || !el.value) return
            el.select()
            wantSelect.current = false
          })
        })
        .catch(() => undefined)
    })
    void SettingsService.Get().then(applyCfg)
    void SettingsService.Languages().then((ls) => setLangs(ls ?? []))
    void SettingsService.ServiceTypes('translate').then((ts) =>
      setAiKinds(new Set((ts ?? []).filter((x) => x.prompt).map((x) => x.kind))))
    // The native close button destroys the window, so the webview that comes
    // back after the next shortcut is a fresh one. The answer is still in Go;
    // showing it again is what keeps 收起 and closing the red button from
    // throwing away what the user was reading.
    void Service.PanelLast().then((last) => {
      if (!last || roleOf(last.context) !== 'main') return
      setRes(last)
      // A restored answer quotes the text that was in the box, so put it back:
      // an empty input under a filled result looks like the query was never
      // entered. A clipboard answer carries that text just the same.
      if (last.mode !== 'prompt' && last.sourceText) {
        textRef.current = last.sourceText
        sent.current = last.sourceText
        setText(last.sourceText)
      }
    })

    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      // While an IME is composing, Esc belongs to the candidate window; closing
      // the panel there would be EasyDict's cancelOperation never running.
      if (e.isComposing) return
      // A popover open in front of the answer is what Escape means first; the
      // window only closes once there is nothing left to dismiss.
      if (menu) {
        setMenu(false)
        return
      }
      void Service.ClosePanel()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      offResult()
      offBusy()
      offCommand()
      offCfg()
      window.removeEventListener('keydown', onKey)
    }
  }, [applyCfg, menu])

  // draft lets a paste query the text it just inserted: the state update has
  // not landed when the paste handler decides to run.
  const run = (instruction?: string, langs?: { sourceLang: string; targetLang: string }, draft?: string) => {
    const value = (draft ?? text).trim()
    if (!value) {
      setNote(t('请输入文字'))
      box.current?.focus()
      return
    }
    sent.current = value
    if (instruction) {
      void Service.Prompt(value, instruction).catch((e) => setNote(errMsg(e)))
      return
    }
    // A language picked in 识别为 belongs to its text: asking that text again —
    // by hand, by 自动查询, by ⌘R through the saved request — keeps the choice,
    // as EasyDict's specifiedTextLanguageDict does. Even 自动检测 in the bar
    // re-applies the correction, because the dictionary overrides detection.
    const forcedLang = forced && value === forced.text ? forced.code : ''
    const asked = langs?.sourceLang
    void Service.Ask({
      context: 'main', mode: 'input', text: value,
      sourceLang: asked && asked !== 'auto' ? asked : (forcedLang || source),
      targetLang: langs?.targetLang ?? target,
      dictionaryOnly: dictOnly,
    }).catch((e) => setNote(errMsg(e)))
  }

  // 输入时自动查询 is EasyDict's 自动查询: fire a moment after typing stops, not
  // on every key. sent keeps it from repeating a query already run, and the
  // returned cancel drops a pending timer when Enter, paste or blur takes over.
  const cancelAuto = useAutoQuery(text, !!cfg?.autoQueryTyping, sent,
    (value) => run(undefined, undefined, value))

  // EasyDict's EZLayoutManager sizes the query text view from 75pt up to 30% of
  // the screen (inputViewMaxHeight), then scrolls; a pasted paragraph should be
  // readable in place instead of hiding behind three lines.
  useEffect(() => {
    const el = box.current
    if (!el) return
    el.style.height = 'auto'
    const maxHeight = Math.floor(window.screen.height * 0.3)
    el.style.height = `${Math.min(maxHeight, Math.max(75, el.scrollHeight))}px`
  }, [text, cfg?.windowConfig?.main?.input])

  // 交换 follows EasyDict's toggleTranslationLanguages; planSwap owns the
  // decision so both windows cannot drift.
  const swap = () => {
    const plan = planSwap(res, text, source, target)
    switch (plan.kind) {
      case 'auto':
        // Both sides on 自动: ask the answer again as it is, keeping the pair.
        stopSpeech()
        setText(plan.text)
        run(undefined, undefined, plan.text)
        break
      case 'reverse':
        stopSpeech()
        setSource(plan.from)
        setTarget(plan.to)
        setText(plan.text)
        void SettingsService.SetSourceLang(plan.from).catch((e) => setNote(errMsg(e)))
        void SettingsService.SetTargetLang(plan.to).catch((e) => setNote(errMsg(e)))
        run(undefined, { sourceLang: plan.from, targetLang: plan.to }, plan.text)
        break
      case 'pair':
        setSource(plan.to)
        setTarget(plan.from)
        void SettingsService.SetSourceLang(plan.to).catch((e) => setNote(errMsg(e)))
        void Service.SetTargetLang(plan.from).catch((e) => setNote(errMsg(e)))
        if (text.trim() !== '') {
          stopSpeech()
          run(undefined, { sourceLang: plan.to, targetLang: plan.from })
        }
        break
    }
  }

  // Picking a language in 识别为 names the reading for this text: the request
  // carries it as 源语言 while the picker half still says 自动检测, so the next
  // 自动查询 does not forget the correction for the same text. Choosing the
  // language already shown is a no-op, as selectedLanguageBlock only retries
  // when the choice differs from detectedLanguage.
  const pickDetected = (code: string) => {
    if (code === shownDetected) return
    const value = (textRef.current || res?.sourceText || '').trim()
    if (!value) return
    setForced({ text: value, code })
    stopSpeech()
    run(undefined, { sourceLang: code, targetLang: target }, value)
  }

  // fail reports a refused action and remembers when the reason was a missing
  // system grant, so the line can carry the button that opens the right pane.
  const fail = (e: unknown) => {
    const raw = e instanceof Error ? e.message : String(e ?? '')
    setPermNeed(raw.includes('辅助功能') ? 'access' : raw.includes('屏幕录制') ? 'screen' : '')
    setNote(errMsg(e))
  }

  // The 快捷键 menu and the ⌘ keys run the same actions. Keeping the steps in
  // one place is what stops a menu item from drifting away from the key it
  // advertises; the key handler below only decides which one was pressed.
  const clearInput = () => {
    // EasyDict's clearInput stops the audio player, so a clip read from the box
    // does not keep sounding after the text it belongs to is gone.
    stopSpeech()
    setText('')
    textRef.current = ''
    box.current?.focus()
  }
  const clearAll = () => {
    clearInput()
    sent.current = ''
    setRes(null)
    setNote('')
    // The Go side keeps the answer for 重试, so clearing the window has to
    // clear that too or ⌘R would resurrect what was just dismissed.
    void Service.ForgetLast().catch((x) => setNote(errMsg(x)))
  }
  const copyQuery = () => {
    const query = (textRef.current || res?.sourceText || '').trim()
    if (!query) return
    void Service.CopyText(query)
      .then((ok) => setNote(ok ? t('已复制') : t('复制失败，请手动选择文字')))
      .catch(() => setNote(t('复制失败，请手动选择文字')))
  }
  const copyFirst = () => {
    const first = res ? firstResult(res) : ''
    if (!first) return
    void Service.CopyText(first)
      .then((ok) => setNote(ok ? t('已复制') : t('复制失败，请手动选择文字')))
      .catch(() => setNote(t('复制失败，请手动选择文字')))
  }
  // A quick link that cannot open has to say why: an uninstalled Eudic or an
  // empty box would otherwise make the titlebar chips look dead.
  const quickLinkFailure: Record<QuickLink, string> = {
    google: '无法打开浏览器',
    apple: '无法打开系统词典',
    eudic: '未找到欧路词典，请确认已安装',
  }
  const openExternal = (kind: QuickLink) => {
    const query = (textRef.current || res?.sourceText || '').trim()
    if (!query) {
      setNote(t('请先输入要查询的文字'))
      box.current?.focus()
      return
    }
    void openQuickLink(quickLink(kind, query)).then((ok) => {
      if (!ok) setNote(t(quickLinkFailure[kind]))
    })
  }

  // The window answers the same ⌘ keys as EasyDict's main window: I focus,
  // K clears the box, ⇧K clears the answer too, ⇧C copies the query text, ⇧J
  // copies the first answer, T swaps the language pair, +/− resize, S speaks,
  // P pins, Y hides, R re-runs, ↩ opens Google, ⇧↩ opens Eudic, ⇧D opens the
  // system dictionary and , opens settings. Every one carries ⌘, so plain
  // typing is untouched. The keys come from the 快捷键 table rather than this
  // list: rebinding one in settings has to teach this handler too, because a
  // non-activating panel never sees the menu's key equivalents.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return
      const key = e.key.toLowerCase()
      const hit = (action: AppAction) => matchesAccel(e, appAccel(cfg, action))
      // Plain ⌘C/⌘X belong to the text selection, ⇧⌘C is 复制查询原文 below.
      if (key === 'c' && !e.shiftKey) {
        if (copySelection()) e.preventDefault()
        return
      }
      if (key === 'x') {
        if (cutSelection()) e.preventDefault()
        return
      }
      if (key === ',') {
        // ⌘, is where macOS apps keep preferences, and EasyDict documents it as
        // the way back to settings after 隐藏菜单栏图标.
        e.preventDefault()
        void Service.OpenSettings().catch((x) => setNote(errMsg(x)))
        return
      }
      if (hit('google')) {
        // ⌘↩ searches outside; ⇧↩ looks the word up in Eudic. Both read the
        // text currently in the box, the same text the quick-link icons use.
        e.preventDefault()
        openExternal('google')
        return
      }
      if (hit('eudic')) {
        e.preventDefault()
        openExternal('eudic')
        return
      }
      if (hit('appleDic')) {
        // dict:// is the macOS dictionary, so the key only exists on macOS,
        // matching the quick-link icon that the same setting controls.
        if (!isMac) return
        e.preventDefault()
        openExternal('apple')
        return
      }
      if (hit('copy')) {
        e.preventDefault()
        // ⌘⇧C is 复制查询原文, as EasyDict documents it; 复制全文 is the button
        // under the answer, so the shortcut does not have to guess at scope.
        copyQuery()
        return
      }
      if (hit('copyFirstResult')) {
        e.preventDefault()
        copyFirst()
        return
      }
      if (hit('focus')) {
        e.preventDefault()
        box.current?.focus()
        return
      }
      if (hit('clearAll')) {
        e.preventDefault()
        clearAll()
        return
      }
      if (hit('clearInput')) {
        e.preventDefault()
        clearInput()
        return
      }
      if (hit('decreaseFontSize')) {
        e.preventDefault()
        stepFont(-1)
        return
      }
      if (hit('increaseFontSize')) {
        e.preventDefault()
        stepFont(1)
        return
      }
      if (hit('play')) {
        e.preventDefault()
        if (res?.sourceText) void toggleSpeech(res.sourceText, res.detected ?? 'auto', false, setNote)
        return
      }
      if (hit('toggle')) {
        e.preventDefault()
        swap()
        return
      }
      if (hit('pin')) {
        e.preventDefault()
        flipPin()
        return
      }
      if (hit('hide')) {
        e.preventDefault()
        void Service.ClosePanel()
        return
      }
      if (hit('retry')) {
        e.preventDefault()
        // retryQueryWithLanguage stops the clip too: the answer it was reading
        // is about to be replaced.
        stopSpeech()
        void Service.ReTranslate().catch((x) => setNote(errMsg(x)))
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [res, cfg])

  // 快捷键 menu clicks arrive from Go, which addressed this window because it
  // holds the keyboard. The actions are the same as the ⌘ keys above; only one
  // of the two paths runs, since a key the menu carries never reaches the page.
  useEffect(() => {
    return onMenuAction('main', (action) => {
      switch (action) {
        case 'clearInput':
          clearInput()
          break
        case 'clearAll':
          clearAll()
          break
        case 'copy':
          copyQuery()
          break
        case 'copyFirstResult':
          copyFirst()
          break
        case 'focus':
          // ⌘I is EasyDict's focusInputTextView: with
          // 打开窗口时自动选中查询文本 on, focusing also selects the query.
          box.current?.focus()
          if (cfg?.selectTextOnOpen) box.current?.select()
          break
        case 'play':
          if (res?.sourceText) void toggleSpeech(res.sourceText, res.detected ?? 'auto', false, setNote)
          break
        case 'retry':
          stopSpeech()
          void Service.ReTranslate().catch((x) => setNote(errMsg(x)))
          break
        case 'toggle':
          swap()
          break
        case 'pin':
          flipPin()
          break
        case 'hide':
          void Service.ClosePanel()
          break
        case 'increaseFontSize':
          stepFont(1)
          break
        case 'decreaseFontSize':
          stepFont(-1)
          break
        case 'google':
          openExternal('google')
          break
        case 'eudic':
          openExternal('eudic')
          break
        case 'appleDic':
          openExternal('apple')
          break
      }
    })
  }, [res, source, target])

  const ocrCount = (res?.ocr ?? []).length
  const trCount = (res?.translate ?? []).length
  // A pending row is a reserved place, not an answer; counting it would show
  // 2/2 before the first service has replied.
  const okCount = useMemo(() => (res?.translate ?? []).filter((x) => !x.error && !x.pending).length, [res])
  const dictHits = useMemo(
    () => (res?.dict ?? []).filter((x) => !x.error && (x.entries ?? []).length > 0).length,
    [res],
  )
  // 自动检测 + Chinese target answers Chinese text in English, the way Easydict
  // does. The picker still shows the chosen target, so the footer says which
  // language the answer actually arrived in.
  const answeredIn = res && res.targetLang && target && res.targetLang !== target
    ? langName(res.targetLang, langNames) || res.targetLang
    : ''
  // 识别为 names what the services recognized, the way the side panel says
  // 识别为英语 instead of leaving the detection implicit. A language the user
  // picked by hand outranks the guess for that text, both while the pick waits
  // for its answer and afterwards: the answer carries sourceLang so a ⌘R retry
  // still reads the text that way. The typing pass is what lets the chip appear
  // before any answer exists.
  const shownDetected = resolveDetected(text.trim(), forced, typed, res)
  const detectedName = detectedLabel(shownDetected, (code) => code === 'auto'
    ? t('自动检测')
    : langName(code, langNames) || code)
  const canPrompt = (cfg?.translate ?? []).some((p) => p.enabled && aiKinds.has(p.kind))
  // The language bar shows one label per half, centered behind a native select.
  // 自动检测 wears 🌐 until the services name a language, then borrows that
  // language's flag; the picker's tooltip names what was recognized, as
  // EasyDict's EZSelectLanguageButton does.
  const sourceName = source === 'auto'
    ? t('自动检测')
    : langs.find((l) => l.code === source)?.name ?? source
  const targetName = target === 'auto' ? t('自动选择') : langs.find((l) => l.code === target)?.name ?? target
  const sourceFlag = source === 'auto'
    ? flagFor(shownDetected !== 'auto' ? shownDetected : undefined) || '🌐'
    : flagFor(source)
  const targetFlag = target === 'auto'
    ? flagFor(res?.targetLang) || '🌐'
    : flagFor(target)

  return (
    <div className="shell frame-own">
      <div className="dragbar">
        <button
          className={`ghost tiny icon pin ${pinned ? 'on' : ''}`}
          title={`${pinned ? t('取消钉住') : t('钉住窗口')}${accelHint(cfg, 'pin')}`}
          aria-pressed={pinned}
          onClick={flipPin}
        >
          <PinIcon filled={pinned} />
        </button>
        {/* EasyDict right-aligns its shortcut stack, which is where the quick
            links and the tool row stay. */}
        {/* Only the empty middle is the native caption region: a button inside
            it would be swallowed by window drags. */}
        <div className="drag" />
        <QuickLinks cfg={cfg} onOpen={openExternal} />
        <div className="headtools">
          <button className="ghost tiny icon" title={t('截图翻译')}
            onClick={() => void Service.Start().catch(fail)}>
            <ToolIcon id="capture" />
          </button>
          <button className="ghost tiny icon" title={t('划词翻译')}
            onClick={() => void Service.CatchSelection().catch(fail)}>
            <ToolIcon id="select" />
          </button>
          <button className="ghost tiny icon" title={t('剪贴板')}
            onClick={() => void Service.ClipboardQuery().catch((e) => setNote(errMsg(e)))}>
            <ToolIcon id="clipboard" />
          </button>
          <button className="ghost tiny icon"
            title={!res?.sourceText
              ? t('没有可播放的文字')
              : `${playing === speechKey(res.sourceText, res.detected ?? 'auto', false) ? t('停止播放音频') : t('播放音频')}${accelHint(cfg, 'play')}`}
            aria-label={playing === speechKey(res?.sourceText ?? '', res?.detected ?? 'auto', false) ? t('停止播放音频') : t('播放音频')}
            disabled={!res?.sourceText}
            onClick={() => void toggleSpeech(res?.sourceText ?? '', res?.detected ?? 'auto', false, setNote)}>
            <ToolIcon id={playing === speechKey(res?.sourceText ?? '', res?.detected ?? 'auto', false) ? 'stop' : 'speak'} />
          </button>
          <button className="ghost tiny icon fs" title={`${t('字号')} ${cfg?.resultFont || '1'}/5`}
            aria-label={t('字号')} onClick={cycleFont}>
            Aa
          </button>
          {cfg?.showQuickActionButton !== false && (
            <div className="qwrap">
              <button className="ghost tiny icon quickact" title={t('快捷动作')} aria-label={t('快捷动作')}
                aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu((v) => !v)}>
                <QuickActionGlyph />
              </button>
              {menu && (
                <>
                  <div className="qshade" onClick={() => setMenu(false)} />
                  <div className="qmenu" role="menu">
                    <button role="menuitem" disabled={!res && !text.trim()}
                      onClick={star}>{t('添加到收藏')}</button>
                    <button role="menuitem" onClick={() => rewrite('newline')}>{t('将换行符替换为空格')}</button>
                    <button role="menuitem" onClick={() => rewrite('comments')}>{t('去除代码注释符号')}</button>
                    <button role="menuitem" onClick={() => rewrite('words')}>{t('单词分词')}</button>
                    <div className="qsep" />
                    <button role="menuitem" onClick={() => {
                      setMenu(false)
                      void Service.OpenSettings().catch((e) => setNote(errMsg(e)))
                    }}>{t('打开应用设置')}</button>
                  </div>
                </>
              )}
            </div>
          )}
          <button className="ghost tiny icon" title={t('历史')} onClick={() => void Service.OpenHistory()}>
            <ToolIcon id="history" />
          </button>
        </div>
        <WindowControls />
        {/* The red traffic light is the close button on macOS; a second ✕ beside
            it read as a duplicate. Windows has no traffic lights, so there the
            header still needs one — and it hides rather than quits. */}
        {!isMac && (
          <button className="ghost tiny icon" title={t('收起')} onClick={() => void Service.ClosePanel()}>
            <CloseIcon />
          </button>
        )}
      </div>

      {/* 窗口配置 → 显示文本输入框: a fixed main window can be left with its
          answers only, which is how Easydict reads a parked result. */}
      {cfg?.windowConfig?.main?.input !== false && (
      <div className="compose">
        <textarea
          ref={box}
          className="compose-input"
          value={text}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          placeholder={t('输入内容，Enter 查询，Shift + Enter 换行')}
          onChange={(e) => {
            wantSelect.current = false
            setText(e.target.value)
            textRef.current = e.target.value
          }}
          // EasyDict's query view pastes plain text trimmed of the surrounding
          // whitespace, and a paste is not typing: 粘贴后自动查询 owns the query
          // decision, so the 输入时自动查询 timer is dropped either way.
          onPaste={(e) => {
            e.preventDefault()
            cancelAuto()
            const pasted = e.clipboardData.getData('text').trim()
            if (!pasted) return
            const el = e.currentTarget
            const start = el.selectionStart ?? text.length
            const end = el.selectionEnd ?? text.length
            const next = text.slice(0, start) + pasted + text.slice(end)
            wantSelect.current = false
            setText(next)
            textRef.current = next
            el.setSelectionRange(start + pasted.length, start + pasted.length)
            if (cfg?.autoQueryPaste) {
              run(undefined, undefined, next)
            } else {
              // The text now in the box must not trip the typing debounce that
              // the state update will schedule for this same value.
              sent.current = next.trim()
            }
          }}
          // Enter runs, Shift+Enter keeps the newline: the same bargain the
          // input window makes, and the gesture EasyDict trains on this box.
          onKeyDown={(e) => {
            // A ⌘↩ belongs to the window's external-search shortcut, not to
            // the box: without this guard the text would be queried as well.
            if (e.metaKey || e.ctrlKey) return
            if (e.key !== 'Enter' || e.shiftKey) return
            // An IME uses Enter to accept a candidate; translating there would
            // submit half a word.
            if (e.nativeEvent.isComposing) return
            e.preventDefault()
            // Enter is an explicit query: a pending 输入时自动查询 timer for the
            // same text would make it two requests, which EasyDict avoids.
            cancelAuto()
            run()
          }}
          onBlur={cancelAuto}
          onContextMenu={inputMenu.onContextMenu}
        />
        {inputMenu.inputMenu}
        {/* Speaker, copy, 识别为 and 清空 sit on one row at the bottom of the
            card. 清空 is EasyDict's clearAll (⌘⇧K): the box and every result
            leave together, which is why the button appears as soon as either
            is on screen. */}
        <div className="compose-tools">
          <button className="compose-icon"
            title={`${playing === speechKey(text, source, false) ? t('停止播放音频') : t('播放音频')}${accelHint(cfg, 'play')}`}
            aria-label={playing === speechKey(text, source, false) ? t('停止播放音频') : t('播放音频')}
            disabled={!text}
            onClick={() => void toggleSpeech(text, source, false, setNote)}>
            <ToolIcon id={playing === speechKey(text, source, false) ? 'stop' : 'speak'} />
          </button>
          <button className="compose-icon" title={`${t('拷贝文本')}${accelHint(cfg, 'copy')}`} disabled={!text}
            onClick={() => void Service.CopyText(text)}>
            <CopyGlyph />
          </button>
          {detectedName && (
            <DetectChip
              label={detectedName}
              current={shownDetected}
              // A source language picked by hand makes the correction moot,
              // so the chip stays readable but inert, like EasyDict's.
              disabled={source !== 'auto'}
              langs={langs}
              onPick={pickDetected}
            />
          )}
          {(text || res) && (
            <button
              className="compose-clear"
              title={`${t('清空')}${accelHint(cfg, 'clearAll')}`}
              aria-label={t('清空')}
              onClick={() => {
                stopSpeech()
                setText('')
                textRef.current = ''
                sent.current = ''
                setRes(null)
                setNote('')
                void Service.ForgetLast().catch(() => undefined)
                box.current?.focus()
              }}
            >
              ✕
            </button>
          )}
        </div>
        {canPrompt && text.trim() !== '' && (
          <div className="chips scroll">
            {instructions.map((i) => (
              <button key={i.label} className="chip" disabled={!!busy} onClick={() => run(i.text)}>
                {i.label}
              </button>
            ))}
          </div>
        )}
      </div>
      )}

      {cfg?.windowConfig?.main?.langBar !== false && (
      <div className="langbar">
        {/* Each half is a label plus a transparent native select stretched over
            it: the bar keeps the centered "▾ 自动检测 🇬🇧" reading of EasyDict's
            pickers while the menu itself stays the OS one. */}
        <label className="langitem"
          title={source === 'auto' && detectedName ? detectedName : undefined}>
          <span className="langvalue">
            <svg className="lchev" width="8" height="8" viewBox="0 0 8 8" aria-hidden="true">
              <path d="M1 2.5 4 6l3-3.5z" fill="currentColor" />
            </svg>
            <span className="lname">{sourceName}</span>
            {isMac && !narrow && sourceFlag && <span className="lflag">{sourceFlag}</span>}
          </span>
          <select
            value={source}
            aria-label={t('源语言')}
            onChange={(e) => {
              const code = e.target.value
              setSource(code)
              void SettingsService.SetSourceLang(code).catch((err) => setNote(errMsg(err)))
              // EasyDict re-enters the query when a language half changes: the
              // text does not change, but the question asked of it did.
              if (text.trim() !== '') {
                stopSpeech()
                run(undefined, { sourceLang: code, targetLang: target })
              }
            }}
          >
            <option value="auto">{langOption('auto', t('自动检测'))}</option>
            {pickableLangs(langs).map((l) => (
              <option key={l.code} value={l.code}>{langOption(l.code, l.name)}</option>
            ))}
          </select>
        </label>
        <button
          className="swap"
          title={`${t('交换语言')}${accelHint(cfg, 'toggle')}`}
          aria-label={t('交换语言')}
          onClick={swap}
        >
          <SwapIcon />
        </button>
        <label className="langitem"
          title={target === 'auto' && res?.targetLang
            ? (langName(res.targetLang, langNames) || res.targetLang)
            : undefined}>
          <span className="langvalue">
            <svg className="lchev" width="8" height="8" viewBox="0 0 8 8" aria-hidden="true">
              <path d="M1 2.5 4 6l3-3.5z" fill="currentColor" />
            </svg>
            <span className="lname">{targetName}</span>
            {isMac && !narrow && targetFlag && <span className="lflag">{targetFlag}</span>}
          </span>
          <select
            value={target}
            aria-label={t('目标语言')}
            onChange={(e) => {
              setTarget(e.target.value)
              void Service.SetTargetLang(e.target.value).catch((e2) => setNote(errMsg(e2)))
            }}
          >
            {[{ code: 'auto', name: t('自动选择') }, ...pickableLangs(langs)].map((l) => (
              <option key={l.code} value={l.code}>{langOption(l.code, l.name)}</option>
            ))}
          </select>
        </label>
      </div>
      )}

      {note && (
        <div className={`noteline${permNeed ? ' withaction' : ''}`}>
          <span>{note}</span>
          {permNeed && (
            <button className="tiny" onClick={() => void Service.OpenPermission(permNeed)}>
              {t('打开系统设置')}
            </button>
          )}
        </div>
      )}

      {!res ? <div className="blank" /> : (
        <ResultColumns
          compact
          res={res}
          busy={busy}
          notice={setNote}
          langNames={langNames}
          markdown={cfg?.markdownRender !== false}
          anki={!!cfg?.ankiEnabled}
          showSource={cfg?.windowConfig?.main?.input === false && res.mode !== 'input' && res.mode !== 'prompt'}
          links={[
            ...(cfg?.showGoogleLink ? (['google'] as QuickLink[]) : []),
            ...(isMac && cfg?.showAppleDictLink ? (['apple'] as QuickLink[]) : []),
            ...(cfg?.showEudicLink ? (['eudic'] as QuickLink[]) : []),
          ]}
        />
      )}

      <div className="footer">
        <label className="check" title={t('只查词典，不调用翻译服务')}>
          <input type="checkbox" checked={dictOnly} onChange={(e) => setDictOnly(e.target.checked)} />
          <span>{t('仅词典')}</span>
        </label>
        <span>
          {res
            ? [
                // A failed OCR has no translations to count; 0/0 译文 next to
                // the failure only adds noise.
                trCount > 0 ? t('{ok}/{all} 译文', { ok: okCount, all: trCount }) : '',
                ocrCount > 0 ? t('{n} 识别', { n: ocrCount }) : '',
                dictHits > 0 ? t('{n} 词典', { n: dictHits }) : '',
                answeredIn && okCount > 0 ? t('已译为{lang}', { lang: answeredIn }) : '',
              ].filter(Boolean).join(' · ')
            : t('已启用 {n} 个翻译服务', { n: (cfg?.translate ?? []).filter((p) => p.enabled).length })}
        </span>
        <span className="spacer" />
        <span>{busy || (res
          ? ''
          : [accelLabel(cfg, 'focus'), t('聚焦输入')].filter(Boolean).join(' '))}</span>
      </div>
    </div>
  )
}

// ToolIcon gives the query row a shape to find before reading the label. The
// icons stay monochrome so the primary 翻译 button is still the one colour.
function ToolIcon({ id }: { id: 'capture' | 'select' | 'clipboard' | 'speak' | 'stop' | 'history' }) {
  const paths: Record<string, ReactNode> = {
    capture: (
      <>
        <path d="M4 8V6a2 2 0 0 1 2-2h2M16 4h2a2 2 0 0 1 2 2v2M20 16v2a2 2 0 0 1-2 2h-2M8 20H6a2 2 0 0 1-2-2v-2" />
        <path d="M8 12h8" />
      </>
    ),
    select: <path d="m5 4 6 16 2.4-6.6L20 11 5 4Z" />,
    clipboard: (
      <>
        <rect x="8" y="3" width="8" height="4" rx="1" />
        <path d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2" />
      </>
    ),
    speak: (
      <>
        <path d="M11 5 7 9H4v6h3l4 4V5Z" />
        <path d="M15.5 8.5a5 5 0 0 1 0 7" />
      </>
    ),
    stop: (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M10 9.5v5M14 9.5v5" />
      </>
    ),
    history: (
      <>
        <circle cx="12" cy="12" r="8" />
        <path d="M12 8v4l3 2" />
      </>
    ),
  }
  return (
    <svg
      className="pico"
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[id]}
    </svg>
  )
}

// QuickLinks mirrors Easydict's titlebar icons: Google, the system dictionary
// and Eudic, each opening the text currently in the box. EZTitlebar stacks
// them right-to-left, so 欧路 ends up leftmost; each is its own 显示…快捷图标
// switch in 通用.
function QuickLinks({ cfg, onOpen }: { cfg: Config | null; onOpen: (kind: QuickLink) => void }) {
  const items: { kind: QuickLink; on: boolean; title: string; body: ReactNode }[] = [
    { kind: 'google', on: !!cfg?.showGoogleLink, title: `${t('通过 Google 搜索')}${accelHint(cfg, 'google')}`, body: <GoogleMark /> },
    {
      kind: 'apple',
      on: isMac && !!cfg?.showAppleDictLink,
      title: `${t('在苹果词典中打开')}${accelHint(cfg, 'appleDic')}`,
      body: <DictionaryMark />,
    },
    { kind: 'eudic', on: !!cfg?.showEudicLink, title: `${t('在欧路词典中打开')}${accelHint(cfg, 'eudic')}`, body: <EudicMark /> },
  ]
  const shown = items.filter((i) => i.on).reverse()
  if (shown.length === 0) return null
  // The chips stay coloured when the box is empty; the click itself explains
  // what is missing, which is better than a disabled chip with no reason.
  return (
    <div className="quicklinks">
      {shown.map((i) => (
        <button
          key={i.kind}
          className={`ghost tiny icon ql ${i.kind}`}
          title={i.title}
          onClick={() => onOpen(i.kind)}
        >
          {i.body}
        </button>
      ))}
    </div>
  )
}

// PinIcon is the 置顶 toggle: a pushpin standing up and filled when the window
// stays on top, outlined and dim when it does not.
function PinIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M12 17v5" />
      <path d="M9 10.8a2 2 0 0 1-1.1 1.8l-1.8.9A2 2 0 0 0 5 15.2V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.8a2 2 0 0 0-1.1-1.8l-1.8-.9A2 2 0 0 1 15 10.8V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1Z" />
    </svg>
  )
}

// The header's controls are drawn at one weight, so the icon row reads as
// one group instead of a pin beside two text glyphs.
function CloseIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  )
}

// CopyGlyph is the input card's copy affordance, drawn at the same weight as
// the speaker beside it.
function CopyGlyph() {
  return (
    <svg
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="10" height="10" rx="2" />
      <path d="M5 15V6a1 1 0 0 1 1-1h9" />
    </svg>
  )
}

// autoRead is 自动朗读: the answer speaks itself, so a learner hears the word
// without reaching for a button.
function autoRead(res: Response, cfg: Config | null, notice: (msg: string) => void) {
  const read = autoReadAnswer(res, cfg?.ttsAuto ?? 'off',
    { pronounceEnglishWord: !!cfg?.autoPronounceWord })
  if (read) void readAloud(read.text, read.lang, false, notice)
}
