// Windows has no traffic lights, so the three windows there draw their own
// caption buttons. Minimize and maximize are non-client regions: Wails reports
// each rectangle to the OS and Windows performs the action itself, so these
// never reach the webview and cannot steal the drag region. The close button is
// left to the window (收起 hides, it does not quit), which is why it stays in
// the caller's markup.
import type { CSSProperties } from 'react'
import { isMac } from './api'
import { t } from './i18n'

const region = (kind: 'minimize' | 'maximize'): CSSProperties =>
  ({ ['--wails-non-client-region']: kind } as CSSProperties)

export default function WindowControls() {
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
      <button className="winbtn" title={t('最大化')} aria-label={t('最大化')} style={region('maximize')}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
          <rect x="5" y="5" width="14" height="14" rx="1.5" />
        </svg>
      </button>
    </div>
  )
}
