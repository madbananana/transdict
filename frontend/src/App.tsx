// One webview bundle serves every window, so the hash decides which view a
// window renders. Go owns the URLs; adding a view here without a window that
// asks for it is harmless, and vice versa would show a blank panel.
import CaptureOverlay from './CaptureOverlay'
import LogsWindow from './LogsWindow'
import OcrWindow from './OcrWindow'
import Panel from './Panel'
import QueryIcon from './QueryIcon'
import SettingsWindow from './SettingsWindow'
import UpdateWindow from './UpdateWindow'
import { useEffect } from 'react'
import { SettingsService, applyTheme } from './api'
import { applyLocale } from './i18n'

export type View = 'panel' | 'queryicon' | 'capture' | 'settings' | 'ocr' | 'logs' | 'update'

export function hashRoute(): { view: View; frame: number } {
  const parts = window.location.hash.replace(/^#\/?/, '').split('/')
  switch (parts[0]) {
    case 'capture':
      return { view: 'capture', frame: Number(parts[1] || '0') || 0 }
    case 'queryicon':
      return { view: 'queryicon', frame: 0 }
    case 'settings':
      return { view: 'settings', frame: 0 }
    case 'ocr':
      return { view: 'ocr', frame: 0 }
    case 'logs':
      return { view: 'logs', frame: 0 }
    case 'update':
      return { view: 'update', frame: 0 }
  }
  return { view: 'panel', frame: 0 }
}

export default function App() {
  const route = hashRoute()
  // Every window needs the saved appearance at load, not only the ones that
  // happen to render a preference form. Without this the settings window and
  // the OCR preview started in the system palette even when the app was set to
  // 深色. The screenshot overlay is the exception: it draws dark chrome over a
  // picture, and a light theme would paint its hints dark on dark.
  useEffect(() => {
    void SettingsService.Get()
      .then((cfg) => {
        if (route.view !== 'capture') applyTheme(cfg.theme)
        applyLocale(cfg.uiLang)
      })
      .catch(() => undefined)
  }, [])

  if (route.view === 'capture') return <CaptureOverlay frame={route.frame} />
  if (route.view === 'queryicon') return <QueryIcon />
  if (route.view === 'settings') return <SettingsWindow />
  if (route.view === 'ocr') return <OcrWindow />
  if (route.view === 'logs') return <LogsWindow />
  if (route.view === 'update') return <UpdateWindow />
  return <Panel />
}
