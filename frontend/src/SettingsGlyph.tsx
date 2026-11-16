// Small tinted glyphs that anchor one settings row, the way EasyDict's form
// rows carry an SF Symbol. Each glyph is drawn here so the colour is fixed per
// meaning rather than per row.
import type { ReactNode } from 'react'

export type IconName =
  | 'globe' | 'speaker' | 'document' | 'cursor' | 'clipboard'
  | 'replace' | 'select' | 'book' | 'return' | 'code'
  | 'scissors' | 'ocr' | 'window' | 'pin' | 'network'
  | 'server' | 'folder' | 'history' | 'notebook' | 'bolt'
  | 'copy' | 'link' | 'eye' | 'power' | 'refresh'
  | 'font' | 'keyboard' | 'input' | 'cards' | 'theme'

const tint: Record<IconName, string> = {
  globe: '#0a84ff',
  speaker: '#0a84ff',
  document: '#af52de',
  cursor: '#0a84ff',
  clipboard: '#30b0c7',
  replace: '#5856d6',
  select: '#5856d6',
  book: '#ff9500',
  return: '#5856d6',
  code: '#5856d6',
  scissors: '#5856d6',
  ocr: '#ff9500',
  window: '#5856d6',
  pin: '#5856d6',
  network: '#0a84ff',
  server: '#30b0c7',
  folder: '#ff9500',
  history: '#34c759',
  notebook: '#ff9500',
  bolt: '#34c759',
  copy: '#30b0c7',
  link: '#ff9500',
  eye: '#af52de',
  power: '#8e8e93',
  refresh: '#34c759',
  font: '#af52de',
  keyboard: '#5856d6',
  input: '#5856d6',
  cards: '#0a84ff',
  theme: '#af52de',
}

const glyphs: Record<IconName, ReactNode> = {
  globe: <>
    <circle cx="12" cy="12" r="8.5" />
    <ellipse cx="12" cy="12" rx="4" ry="8.5" />
    <path d="M3.5 12h17" />
  </>,
  speaker: <>
    <path d="M4.5 9.5h3l4.5-3.6v12.2L7.5 14.5h-3z" />
    <path d="M16 9.2a4 4 0 0 1 0 5.6" />
    <path d="M18.4 6.8a7.4 7.4 0 0 1 0 10.4" />
  </>,
  document: <>
    <path d="M6.5 4.5h7l4 4v11h-11z" />
    <path d="M13.5 4.5v4h4" />
    <path d="M9 12h6M9 15.5h4" />
  </>,
  cursor: <path d="M6.2 3.8l11.4 8.5-4.9 1 2.6 5.6-2.3 1.1-2.7-5.7-4.1 3z" />,
  clipboard: <>
    <rect x="5" y="4.8" width="14" height="15.7" rx="2" />
    <rect x="9" y="2.8" width="6" height="4" rx="1.2" />
    <path d="M12 11v5.5M9.6 14.1L12 16.5l2.4-2.4" />
  </>,
  replace: <path d="M4.5 8.5h13l-3-3M19.5 15.5h-13l3 3" />,
  select: <>
    <rect x="4" y="4" width="16" height="16" rx="2.5" strokeDasharray="2.6 2.4" />
    <path d="M9.5 9.5h5M12 9.5v6.5" />
  </>,
  book: <>
    <path d="M5 5.2A2.2 2.2 0 0 1 7.2 3H19v15.6H7.4A2.4 2.4 0 0 0 5 21z" />
    <path d="M5 5.2v15.8" />
  </>,
  return: <path d="M19 6v6.5H6.5M9.5 8.5L6 12l3.5 3.5" />,
  code: <path d="M9 7.5L4.5 12 9 16.5M15 7.5L19.5 12 15 16.5" />,
  scissors: <>
    <circle cx="6.5" cy="17" r="2.4" />
    <circle cx="6.5" cy="7" r="2.4" />
    <path d="M8.4 8.4L19.5 18M8.4 15.6L19.5 6" />
  </>,
  ocr: <>
    <path d="M4 8.5V5.5A1.5 1.5 0 0 1 5.5 4h3M15.5 4h3A1.5 1.5 0 0 1 20 5.5v3M20 15.5v3a1.5 1.5 0 0 1-1.5 1.5h-3M8.5 20h-3A1.5 1.5 0 0 1 4 18.5v-3" />
    <path d="M8 10h8M8 14h5" />
  </>,
  window: <>
    <rect x="4" y="5" width="16" height="14" rx="2" />
    <path d="M4 9h16M7.5 7h.01M10.5 7h.01" />
  </>,
  pin: <>
    <path d="M9 3.5h6l-.8 6.2 3 2.3v1.8H6.8V12l3-2.3z" />
    <path d="M12 13.8v6" />
  </>,
  network: <>
    <circle cx="5.5" cy="6.5" r="2.2" />
    <circle cx="18.5" cy="6.5" r="2.2" />
    <circle cx="12" cy="18" r="2.2" />
    <path d="M7.5 7.8l3.2 8.2M16.5 7.8l-3.2 8.2" />
  </>,
  server: <>
    <rect x="4" y="4.5" width="16" height="6" rx="1.8" />
    <rect x="4" y="13.5" width="16" height="6" rx="1.8" />
    <path d="M7.5 7.5h.01M7.5 16.5h.01" />
  </>,
  folder: <path d="M4 7a2 2 0 0 1 2-2h4l2 2.2h6a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />,
  history: <>
    <circle cx="12" cy="12" r="8.2" />
    <path d="M12 7.8V12l3 2" />
  </>,
  notebook: <>
    <rect x="5.5" y="3.5" width="13" height="17" rx="2" />
    <path d="M9 3.5v17M12 8h4M12 11.5h4" />
  </>,
  bolt: <path d="M13.2 3.2L6 13.2h5l-1.2 7.6L17.5 10.5h-5.1z" />,
  copy: <>
    <rect x="8.5" y="8.5" width="11" height="11" rx="2" />
    <path d="M15.5 5.5h-9a2 2 0 0 0-2 2v9" />
  </>,
  link: <>
    <path d="M10.2 13.8a3.6 3.6 0 0 1 0-5.1l2-2a3.6 3.6 0 0 1 5.1 5.1l-1 1" />
    <path d="M13.8 10.2a3.6 3.6 0 0 1 0 5.1l-2 2a3.6 3.6 0 0 1-5.1-5.1l1-1" />
  </>,
  eye: <>
    <path d="M2.8 12S6.3 6.2 12 6.2 21.2 12 21.2 12 17.7 17.8 12 17.8 2.8 12 2.8 12z" />
    <circle cx="12" cy="12" r="2.6" />
  </>,
  power: <>
    <path d="M12 3.8v7.4" />
    <path d="M7.4 6.6a7 7 0 1 0 9.2 0" />
  </>,
  refresh: <>
    <path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3" />
    <path d="M19.8 4.5v4.3h-4.3" />
  </>,
  font: <path d="M5 19l5.2-13.4h1.2L16.6 19M7.4 14.4h8" />,
  keyboard: <>
    <rect x="3.2" y="6.8" width="17.6" height="10.4" rx="2" />
    <path d="M7 10.5h.01M10.4 10.5h.01M13.8 10.5h.01M17.2 10.5h.01M8 14h8" />
  </>,
  input: <>
    <rect x="3.8" y="6" width="16.4" height="12" rx="2" />
    <path d="M7.5 10h9M7.5 14h5" />
  </>,
  cards: <>
    <rect x="7.5" y="4" width="12" height="15" rx="2" />
    <path d="M4.5 7v11a2 2 0 0 0 2 2h9" />
  </>,
  theme: <>
    <circle cx="12" cy="12" r="4.2" />
    <path d="M12 2.8v2M12 19.2v2M2.8 12h2M19.2 12h2M5.5 5.5l1.4 1.4M17.1 17.1l1.4 1.4M18.5 5.5l-1.4 1.4M6.9 17.1l-1.4 1.4" />
  </>,
}

// icons maps a config key to the glyph its row wears, so a row added later only
// needs an entry here rather than a prop on every call site.
const icons: Partial<Record<string, IconName>> = {
  sourceLang: 'globe', targetLang: 'globe', firstLang: 'globe', secondLang: 'globe',
  detectOptimize: 'globe', queryIconExcludeLang: 'globe',
  uiLang: 'globe', theme: 'theme',
  pronunciation: 'speaker', ttsAuto: 'speaker', defaultTTS: 'speaker',
  preferYoudaoWord: 'speaker', autoPronounceWord: 'speaker',
  classicalMinLength: 'document',
  autoShowQueryIcon: 'cursor', queryIconMinLength: 'cursor', queryIconClickOnly: 'cursor',
  mouseSelect: 'cursor',
  clearInputOnQuery: 'input', keepLastOnNoSelection: 'input', selectTextOnOpen: 'select',
  autoQueryOCR: 'bolt', autoQueryPaste: 'bolt', autoQueryTyping: 'bolt',
  autoCopySelect: 'copy', autoCopyOCR: 'copy', autoCopy: 'copy',
  showGoogleLink: 'link', showAppleDictLink: 'link', showEudicLink: 'link',
  showQuickActionButton: 'link',
  markdownRender: 'font', hideSameLang: 'eye',
  loginItem: 'power', hideMenuBarIcon: 'power', showInDock: 'window', hideMainWindow: 'window',
  updateURL: 'refresh', autoUpdate: 'refresh', includeBeta: 'refresh',
  forceCopy: 'clipboard', forceCopyKind: 'clipboard',
  preferAppleScriptAPI: 'code', replaceCompatible: 'replace', autoSelectAllText: 'select',
  removeBooksExcerpt: 'book',
  replaceNewlineSpace: 'return', autoRemoveComments: 'code', autoWordSegment: 'scissors',
  enableYoudaoOCR: 'ocr', ocrTextNormalize: 'ocr', showOCRMenu: 'ocr', hideScreenshotTip: 'eye',
  maxHeightPct: 'window', autoPin: 'pin',
  proxy: 'network', timeout: 'network',
  enableHTTPServer: 'server', httpPort: 'server',
  saveHistory: 'history', vocabularyNotebook: 'notebook', vocabularyDir: 'folder',
  resultFont: 'font',
}

export function iconOf(key: string): IconName | undefined {
  return icons[key]
}

export function SettingGlyph({ name }: { name: IconName }) {
  return (
    <span className="sicon" style={{ background: tint[name] }} aria-hidden="true">
      <svg
        width="13"
        height="13"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {glyphs[name]}
      </svg>
    </span>
  )
}
