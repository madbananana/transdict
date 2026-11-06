// 迷你窗: two lines that stay on top of everything — the source and the first
// translation. It is the reading companion for a language you are learning.
import { useEffect, useRef, useState } from 'react'
import { Service, SettingsService, onBusy, onResult, readAloud, speak, type Response } from './api'
import { applyLocale, t } from './i18n'
import { autoReadAnswer } from './autoread'

export default function Mini() {
  const [res, setRes] = useState<Response | null>(null)
  const [busy, setBusy] = useState('')
  const [font, setFont] = useState(15)
  // The result handler is registered before the config read returns, so 自动朗读
  // reaches it through a ref rather than the first render's props.
  const autoRef = useRef('off')

  useEffect(() => {
    const offResult = onResult('mini', (next) => {
      setRes(next)
      if (next.partial) return
      setBusy('')
      const read = autoReadAnswer(next, autoRef.current)
      if (read) void readAloud(read.text, read.lang)
    })
    const offBusy = onBusy('mini', setBusy)
    void Service.LastResult().then((last) => {
      if (last?.context === 'mini') setRes(last)
    })
    void SettingsService.Get().then((cfg) => {
      setFont(cfg.miniFont > 0 ? cfg.miniFont : 15)
      autoRef.current = cfg.ttsAuto || 'off'
      applyLocale(cfg.uiLang)
    })
    return () => {
      offResult()
      offBusy()
    }
  }, [])

  const source = res?.sourceText ?? ''
  const translated = (res?.translate ?? []).find((x) => !x.error)?.text ?? ''
  // The voice follows whichever line is actually read: a target-language voice
  // on the original text would pronounce it as gibberish.
  const spoken = translated ? res?.targetLang ?? '' : res?.detected ?? 'auto'

  return (
    <div className="mini" style={{ fontSize: font }}>
      <div className="mbar">
        <div className="drag">
          <span className="byline">{busy || (res ? `${res.elapsedMs} ms` : t('剪贴板'))}</span>
        </div>
        <button className="tiny" onClick={() => void speak(translated || source, spoken)}>{t('朗读')}</button>
        <button className="tiny" onClick={() => void Service.CopyText(translated || source)}>{t('复制')}</button>
        {/* The mini line carries only the first answer; 详情 is how the rest of
            it — every service, the dictionary — reaches the main window. */}
        <button className="tiny" onClick={() => void Service.OpenMainDetail()}>{t('详情')}</button>
        <button className="tiny close" onClick={() => void Service.CloseMini()}>×</button>
      </div>
      {source && <div className="msrc">{source}</div>}
      {translated && <div className="mtr">{translated}</div>}
      {!source && !busy && <div className="mempty">{t('按下迷你窗快捷键查询剪贴板')}</div>}
    </div>
  )
}
