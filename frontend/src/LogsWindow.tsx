// 调试日志 is EasyDict's 显示调试日志窗口: the tail of the file the app writes,
// read live so a failure that just happened can be inspected without Finder.
import { useEffect, useRef, useState } from 'react'
import { Service } from './api'
import { t } from './i18n'
import WindowControls from './WindowControls'

export default function LogsWindow() {
  const [text, setText] = useState('')
  const [path, setPath] = useState('')
  const pre = useRef<HTMLPreElement>(null)
  // A reader who scrolled up is reading history; the next line must not yank
  // the view back to the end under their eyes.
  const stick = useRef(true)

  useEffect(() => {
    void Service.LogFile().then((p) => setPath(p ?? '')).catch(() => undefined)
    let live = true
    const load = () => {
      void Service.LogTail(600)
        .then((s) => { if (live) setText(s ?? '') })
        .catch(() => undefined)
    }
    load()
    const id = window.setInterval(load, 1000)
    return () => { live = false; window.clearInterval(id) }
  }, [])

  useEffect(() => {
    const el = pre.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [text])

  return (
    <div className="shell window-aux">
      <div className="dragbar">
        {/* The caption region stops before the controls so they stay clickable. */}
        <div className="drag">
          <span className="title">{t('调试日志')}</span>
        </div>
        <WindowControls onClose={() => void Service.CloseLogs()} />
      </div>
      <pre
        className="logview"
        ref={pre}
        onScroll={() => {
          const el = pre.current
          if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24
        }}
      >
        {text || t('还没有日志')}
      </pre>
      <div className="logfoot" title={path}>{path}</div>
    </div>
  )
}
