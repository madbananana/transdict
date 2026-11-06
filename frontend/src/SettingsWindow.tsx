// The preferences window. It is a window of its own rather than a tab of the
// translation window: a sidebar plus forms needs width, and the translation
// window is deliberately narrow.
import Settings from './Settings'
import { Service, isMac } from './api'
import { t } from './i18n'
import WindowControls from './WindowControls'

export default function SettingsWindow() {
  return (
    <div className="shell window-aux">
      <div className="dragbar">
        {/* The caption region stops before the controls so they stay clickable. */}
        <div className="drag">
          <span className="title">{t('设置')}</span>
        </div>
        <WindowControls />
        {/* macOS closes with the red traffic light; only the frameless Windows
            window needs a close control of its own. */}
        {!isMac && (
          <button className="ghost tiny" onClick={() => void Service.CloseSettings()}>{t('关闭')}</button>
        )}
      </div>
      {/* The panel used to receive the saved config; the two windows each read
          the store themselves now, so there is nothing to hand back. */}
      <Settings onSaved={() => undefined} />
    </div>
  )
}
