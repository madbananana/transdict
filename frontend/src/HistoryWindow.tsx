// 查询记录 lives in its own window, like the preferences: the list needs a
// service column and an export row that the narrow translation window cannot
// give it room for.
import { useState } from 'react'
import HistoryView from './History'
import { Service, isMac } from './api'
import { t } from './i18n'
import WindowControls from './WindowControls'

export default function HistoryWindow() {
  const [busy, setBusy] = useState('')
  return (
    <div className="shell window-aux">
      <div className="dragbar">
        {/* The caption region stops before the controls so they stay clickable. */}
        <div className="drag">
          <span className="title">{t('历史记录')}</span>
          {busy && <span className="status">{busy}</span>}
        </div>
        <WindowControls />
        {/* macOS closes with the red traffic light; only the frameless Windows
            window needs a close control of its own. */}
        {!isMac && (
          <button className="ghost tiny" onClick={() => void Service.CloseHistory()}>{t('关闭')}</button>
        )}
      </div>
      <HistoryView onBusy={setBusy} />
    </div>
  )
}
