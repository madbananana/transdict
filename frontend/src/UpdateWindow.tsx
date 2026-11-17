// 软件更新: the release a check found, what changed, and the download as it
// happens. Magpie puts its progress bar and retry button on its About page; a
// window of its own is the same idea with room for the notes.
import { useEffect, useMemo, useRef, useState, type MouseEvent as ReactMouseEvent } from 'react'
import {
  Service, isMac, errMsg, onUpdateChanged, openURL, type UpdateInfo,
} from './api'
import { t } from './i18n'
import { renderMarkdown } from './markdown'
import WindowControls from './WindowControls'

const mb = (n: number) =>
  n >= 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.max(0, Math.round(n / 1024))} KB`

export default function UpdateWindow() {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [error, setError] = useState('')
  const [speed, setSpeed] = useState(0)
  // The last sample of the byte count, so the next event can say how fast the
  // installer is arriving rather than only how much of it has.
  const last = useRef({ at: 0, received: 0, speed: 0 })

  useEffect(() => {
    let live = true
    void Service.UpdateInfo()
      .then((i) => { if (live) setInfo(i) })
      .catch((e) => { if (live) setError(errMsg(e)) })
    const off = onUpdateChanged((i) => {
      setInfo(i)
      const l = last.current
      const now = Date.now()
      if (i.state !== 'downloading') {
        l.at = 0
        l.received = 0
        l.speed = 0
        setSpeed(0)
        return
      }
      if (l.at > 0 && now > l.at && i.received >= l.received) {
        const instant = (i.received - l.received) * 1000 / (now - l.at)
        l.speed = l.speed > 0 ? l.speed * 0.7 + instant * 0.3 : instant
        setSpeed(l.speed)
      }
      l.at, l.received = now, i.received
    })
    return () => { live = false; off() }
  }, [])

  const notes = useMemo(
    () => renderMarkdown((info?.notes ?? '').trim() || t('本次发布没有写明更新内容。')),
    [info?.notes],
  )

  const start = () => {
    setError('')
    void Service.StartUpdate().catch((e) => setError(errMsg(e)))
  }
  const cancel = () => void Service.CancelUpdate().catch((e) => setError(errMsg(e)))
  const hide = () => void Service.HideUpdate()
  const skip = () => void Service.SkipUpdate()
  const page = () => {
    setError('')
    void Service.OpenUpdatePage().catch((e) => setError(errMsg(e)))
  }
  // A release note links to the web; clicking one must not navigate the window
  // away from the download it is showing.
  const followLinks = (e: ReactMouseEvent<HTMLDivElement>) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    e.preventDefault()
    openURL(anchor.getAttribute('href') ?? '')
  }

  const state = info?.state ?? 'idle'
  const downloading = state === 'downloading'
  const installing = state === 'installing'
  const failed = error || (state === 'error' ? info?.message ?? '' : '')
  const total = info?.total ?? 0
  const percent = Math.max(0, Math.min(100, info?.percent ?? 0))

  return (
    <div className="shell window-aux updatewin">
      <div className="dragbar">
        <div className="drag">
          <span className="title">{t('软件更新')}</span>
        </div>
        <WindowControls onClose={hide} />
      </div>

      <div className="updatebody">
        <div className="updatehead">
          <div className="updatekicker">Transdict</div>
          <div className="updateversion">
            {info?.version ? t('新版本 {v}', { v: info.version }) : t('软件更新')}
          </div>
          <div className="updatecurrent">{t('当前版本 {v}', { v: info?.current ?? '' })}</div>
        </div>

        <div className="updatesection">
          <span>{t('更新内容')}</span>
          <button className="ghost tiny" onClick={page}>{t('打开发布页')}</button>
        </div>
        <div
          className="updatenotes prose"
          onClick={followLinks}
          dangerouslySetInnerHTML={{ __html: notes }}
        />
      </div>

      <div className="updatefoot">
        {(downloading || installing) && (
          <>
            <div
              className={`updatebar${!installing && total <= 0 ? ' indeterminate' : ''}`}
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={total > 0 ? percent : undefined}
            >
              <span style={{ width: installing ? '100%' : total > 0 ? `${percent}%` : undefined }} />
            </div>
            <div className="updatemeta">
              <span>
                {installing
                  ? t('正在安装新版本…')
                  : total > 0
                    ? t('正在下载更新… {n}%', { n: percent })
                    : t('正在下载更新…')}
              </span>
              {downloading && (
                <span>
                  {total > 0 ? `${mb(info?.received ?? 0)} / ${mb(total)}` : mb(info?.received ?? 0)}
                  {speed > 0 ? ` · ${mb(speed)}/s` : ''}
                </span>
              )}
            </div>
            {downloading && (
              <div className="updateactions">
                <button className="tiny" onClick={cancel}>{t('取消下载')}</button>
              </div>
            )}
          </>
        )}

        {!downloading && !installing && (
          <>
            {failed && <div className="updateerror">{failed}</div>}
            {!failed && state === 'idle' && info?.message && (
              <div className="updatenote">{info.message}</div>
            )}
            <div className="updateactions">
              {state === 'error' ? (
                <>
                  <button className="ghost" onClick={hide}>{t('稍后')}</button>
                  <button className="primary" onClick={start}>{t('重试')}</button>
                </>
              ) : (
                <>
                  <button className="ghost" onClick={hide}>{t('稍后')}</button>
                  <button className="ghost" onClick={skip}>{t('忽略此版本')}</button>
                  <button className="primary" onClick={start}>{t('立即更新')}</button>
                </>
              )}
            </div>
          </>
        )}

        {installing && (
          <p className="noteline">{t('安装完成后会自动重新打开 Transdict。')}</p>
        )}
        {isMac && !downloading && !installing && (
          <p className="noteline">{t('自动更新会替换当前应用；未签名版本可能需要重新授予录屏与辅助功能权限。')}</p>
        )}
      </div>
    </div>
  )
}