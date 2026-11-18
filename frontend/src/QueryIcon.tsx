// 划词查询图标: a small 查询 button that the watcher shows beside a selection.
// The click is the whole surface — the selection itself was read by the watcher,
// so nothing here has to touch the source application.
import { Service } from '../bindings/transdict/internal/gui/index.js'
import { t } from './i18n'

export default function QueryIcon() {
  // The transparent background is set in index.html before the app mounts; the
  // window must not flash a square of page surface on its first frame.
  return (
    <button
      className="qicon"
      title={t('查询')}
      aria-label={t('查询')}
      onClick={() => void Service.IconQuery()}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
        strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <circle cx="10.4" cy="10.4" r="6.4" />
        <path d="m15.3 15.3 4.3 4.3" />
        <path d="M7.5 8.6h5.6M7.5 11.4h4.1" />
      </svg>
    </button>
  )
}
