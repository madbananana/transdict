// One webview bundle serves every window, so the hash decides which view a
// window renders. Go owns the URLs; adding a view here without a window that
// asks for it is harmless, and vice versa would show a blank panel.
import CaptureOverlay from './CaptureOverlay'
import Float from './Float'
import HistoryWindow from './HistoryWindow'
import Mini from './Mini'
import Panel from './Panel'
import SettingsWindow from './SettingsWindow'
import { useEffect } from 'react'
import { SettingsService, applyTheme } from './api'
import { applyLocale } from './i18n'

export type View = 'panel' | 'float' | 'mini' | 'capture' | 'settings' | 'history'

export function hashRoute(): { view: View; frame: number } {
  const parts = window.location.hash.replace(/^#\/?/, '').split('/')
  switch (parts[0]) {
    case 'capture':
      return { view: 'capture', frame: Number(parts[1] || '0') || 0 }
    case 'float':
      return { view: 'float', frame: 0 }
    case 'mini':
      return { view: 'mini', frame: 0 }
    case 'settings':
      return { view: 'settings', frame: 0 }
    case 'history':
      return { view: 'history', frame: 0 }
  }
  return { view: 'panel', frame: 0 }
}

export default function App() {
  // Every window needs the saved appearance at load, not only the ones that
  // happen to render a preference form. Without this the history window and
  // the settings window itself started in the system palette even when the
  // app was set to 深色.
  useEffect(() => {
    void SettingsService.Get()
      .then((cfg) => {
        applyTheme(cfg.theme)
        applyLocale(cfg.uiLang)
      })
      .catch(() => undefined)
  }, [])

  const route = hashRoute()
  if (route.view === 'capture') return <CaptureOverlay frame={route.frame} />
  if (route.view === 'float') return <Float />
  if (route.view === 'mini') return <Mini />
  if (route.view === 'settings') return <SettingsWindow />
  if (route.view === 'history') return <HistoryWindow />
  return <Panel />
}
