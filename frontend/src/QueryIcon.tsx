// 划词查询图标: a small 查询 button that the watcher shows beside a selection.
// The click is the whole surface — the selection itself was read by the watcher,
// so nothing here has to touch the source application.
import { useEffect } from 'react'
import { Service } from '../bindings/transdict/internal/gui/index.js'
import { t } from './i18n'

export default function QueryIcon() {
  // The window is transparent; only the round button paints.
  useEffect(() => {
    document.documentElement.classList.add('qicon-window')
    return () => document.documentElement.classList.remove('qicon-window')
  }, [])

  return (
    <button
      className="qicon"
      title={t('查询')}
      aria-label={t('查询')}
      onClick={() => void Service.IconQuery()}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2"
        strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <circle cx="10.5" cy="10.5" r="6.5" />
        <path d="m15.5 15.5 4.5 4.5" />
        <path d="M7.6 8.4h5.8M7.6 11.1h4.2" />
      </svg>
    </button>
  )
}
