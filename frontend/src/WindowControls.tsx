// Windows has no traffic lights, so the three windows there draw their own
// caption buttons. Minimize and maximize are non-client regions: Wails reports
// each rectangle to the OS and Windows performs the action itself, so these
// never reach the webview and cannot steal the drag region. Close is a plain
// click handler when the caller passes one: these windows hide instead of
// closing, and a native close region would destroy them.
import type { CSSProperties } from 'react'
import { isMac } from './api'
import { t } from './i18n'

const region = (kind: 'minimize' | 'maximize'): CSSProperties =>
  ({ ['--wails-non-client-region']: kind } as CSSProperties)

export default function WindowControls({
  onClose,
  maximizable = true,
}: {
  onClose?: () => void
  maximizable?: boolean
}) {
  if (isMac) {
    return null
  }
  return (
    <div className="wincontrols">
      <button className="winbtn" title={t('最小化')} aria-label={t('最小化')} style={region('minimize')}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
          <path d="M5 12h14" />
        </svg>
      </button>
      {maximizable && (
        <button className="winbtn" title={t('最大化')} aria-label={t('最大化')} style={region('maximize')}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
            <rect x="5" y="5" width="14" height="14" rx="1.5" />
          </svg>
        </button>
      )}
      {onClose && (
        <button className="winbtn close" title={t('关闭')} aria-label={t('关闭')} onClick={onClose}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      )}
    </div>
  )
}
