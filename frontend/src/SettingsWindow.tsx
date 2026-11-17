// The preferences window. It is a window of its own rather than a tab of the
// translation window: a form with eight tabs needs width, and the translation
// window is deliberately narrow.
import { useState } from 'react'
import Settings from './Settings'
import { Service } from './api'
import { t } from './i18n'
import WindowControls from './WindowControls'

export default function SettingsWindow() {
  const [tab, setTab] = useState('')
  return (
    <div className="shell window-aux">
      <div className="dragbar">
        {/* The caption region stops before the controls so they stay clickable. */}
        <div className="drag">
          <span className="title">{tab || t('设置')}</span>
        </div>
        <WindowControls maximizable={false} onClose={() => void Service.CloseSettings()} />
      </div>
      {/* The panel used to receive the saved config; the two windows each read
          the store themselves now, so there is nothing to hand back. */}
      <Settings onSaved={() => undefined} onPage={setTab} />
    </div>
  )
}
