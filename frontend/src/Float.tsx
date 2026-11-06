// 悬浮窗: the answer to a 划词 or 鼠标取词, parked beside the pointer. It stays
// compact on purpose — the reading position in the other app matters more than
// showing every service.
import { useEffect, useRef, useState } from 'react'
import { DictCard, SkippedHint, SourceCard, TranslateCard, followLinks } from './Result'
import { Service, SettingsService, onBusy, onResult, readAloud, useLangNames, type Config, type Response } from './api'
import { renderMarkdown } from './markdown'
import { applyLocale, t } from './i18n'
import { autoReadAnswer } from './autoread'

// floatLife is how long a result stays on screen when 悬浮窗保持打开 is off: long
// enough to read, short enough that a parked window does not become wallpaper.
// 窗口 → 悬浮窗自动收起 overrides it, and the default matches the fallback here.
const floatLife = 12000

export default function Float() {
  const [res, setRes] = useState<Response | null>(null)
  const [busy, setBusy] = useState('')
  const [keep, setKeep] = useState(true)
  const [life, setLife] = useState(floatLife)
  const [note, setNote] = useState('')
  const langNames = useLangNames()
  // autoRef holds 自动朗读 for the result handler, which was registered before the
  // first config read came back.
  const autoRef = useRef('off')

  // Re-read on every answer: the switch is meant to take effect on the next
  // 划词, not on the next launch. The interface language rides the same call.
  const adopt = (cfg: Config) => {
    setKeep(cfg.keepOpen)
    setLife(cfg.floatClose > 0 ? cfg.floatClose * 1000 : floatLife)
    autoRef.current = cfg.ttsAuto || 'off'
    applyLocale(cfg.uiLang)
  }

  useEffect(() => {
    if (!note) return
    const timer = setTimeout(setNote, 5000, '')
    return () => clearTimeout(timer)
  }, [note])

  useEffect(() => {
    const offResult = onResult('float', (next) => {
      setRes(next)
      setNote('')
      if (!next.partial) {
        setBusy('')
        // 自动朗读 belongs to the window that shows the answer: a parked panel
        // the user is reading from is where hearing the word helps most. A
        // missing voice reports through the same line as the buttons.
        const read = autoReadAnswer(next, autoRef.current)
        if (read) void readAloud(read.text, read.lang, false, setNote)
      }
      void SettingsService.Get().then(adopt)
    })
    const offBusy = onBusy('float', setBusy)
    void Service.LastResult().then((last) => {
      if (last && (last.context === 'floating' || last.context === 'odict')) setRes(last)
    })
    void SettingsService.Get().then(adopt)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') void Service.CloseFloat()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      offResult()
      offBusy()
      window.removeEventListener('keydown', onKey)
    }
  }, [])

  useEffect(() => {
    // The clock starts on the finished answer: a partial sketch is replaced a
    // moment later, and counting from it would close the panel mid-translation.
    if (keep || !res || res.partial) return
    const timer = setTimeout(() => void Service.CloseFloat(), life)
    return () => clearTimeout(timer)
  }, [keep, res, life])

  const translations = (res?.translate ?? []).filter((x) => !x.error)
  const dictionaries = (res?.dict ?? []).filter((d) => !d.error && (d.entries ?? []).length > 0)

  return (
    <div className="float">
      <div className="fbar">
        <div className="drag">
          <span className="fbusy">{busy || (res ? `${res.elapsedMs} ms` : '')}</span>
        </div>
        {res?.sourceText && (
          <>
            <button className="tiny" onClick={() => void Service.CopyText(res.sourceText)}>{t('复制')}</button>
            <button className="tiny" onClick={() => void readAloud(res.sourceText, res.detected || 'auto', false, setNote)}>{t('朗读')}</button>
          </>
        )}
        <button className="tiny" onClick={() => void Service.OpenMainDetail()}>{t('详情')}</button>
        <button className="tiny close" onClick={() => void Service.CloseFloat()}>×</button>
      </div>

      {!res && !busy && <div className="fempty">{t('等待划词…')}</div>}

      {res && (
        <div className="fbody">
          {/* The answer alone loses the phrase it answers; the muted line above
              it is what Easydict parks beside the pointer. */}
          {translations.length > 0 && res.sourceText && <div className="fsrc">{res.sourceText}</div>}
          {translations.length > 0 ? (
            translations[0].markdown ? (
              // A model's reply carries its own markup; parked beside the pointer
              // it still has to be readable as an answer rather than as source.
              <div className="fmain prose" onClick={followLinks}
                dangerouslySetInnerHTML={{ __html: renderMarkdown(translations[0].text) }} />
            ) : (
              <div className="fmain">{translations[0].text}</div>
            )
          ) : res.error ? (
            <>
              <div className="error">{res.error}</div>
              <SkippedHint names={res.skipped ?? []} />
            </>
          ) : (
            <SourceCard res={res} notice={setNote} langNames={langNames} />
          )}
          {translations.length > 1 && (
            <div className="falt">
              {translations.slice(1, 3).map((x) => (
                <div key={x.provider}>
                  <span className="byline">{x.label}</span> {x.text}
                </div>
              ))}
            </div>
          )}
          {dictionaries.length > 0 && (
            <div className="fdict">
              <DictCard item={dictionaries[0]} notice={setNote} lang={res.detected} />
            </div>
          )}
          {(res.translate ?? []).filter((x) => x.error).slice(0, 1).map((x) => (
            <TranslateCard key={x.provider} item={x} compact notice={setNote} />
          ))}
          {note && <div className="noteline">{note}</div>}
        </div>
      )}
    </div>
  )
}
