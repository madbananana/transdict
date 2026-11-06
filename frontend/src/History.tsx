// 历史记录: every query the app has answered, searchable and favoritable. The
// store is a JSON file in the app data directory, so this view is a thin client.
import { useCallback, useEffect, useState } from 'react'
import { Service, errMsg, langName, timeAgo, useLangNames, useWhere } from './api'
import type { Entry, ServiceCount } from '../bindings/transdict/internal/history/models.js'
import { ConfirmButton } from './Confirm'
import { t } from './i18n'

export default function HistoryView({ onBusy }: { onBusy: (text: string) => void }) {
  const [entries, setEntries] = useState<Entry[]>([])
  const [services, setServices] = useState<ServiceCount[]>([])
  const [needle, setNeedle] = useState('')
  const [favoritesOnly, setFavoritesOnly] = useState(false)
  const [service, setService] = useState('')
  const [format, setFormat] = useState('markdown')
  const [days, setDays] = useState(0)
  const where = useWhere()
  const langNames = useLangNames()

  const refresh = useCallback(() => {
    void Promise.all([
      Service.History(needle, favoritesOnly, service, 300),
      Service.HistoryServices(),
    ])
      .then(([list, svcs]) => {
        setEntries(list ?? [])
        setServices(svcs ?? [])
      })
      .catch((e) => onBusy(errMsg(e)))
  }, [needle, favoritesOnly, service, onBusy])

  useEffect(() => {
    refresh()
  }, [refresh])

  // 清理 ignores the search box and the service filter; it removes by age, so
  // the question names the range the select is showing rather than the number
  // of rows currently on screen.
  const scope = days === 0 ? t('全部（保留收藏）') : days === 1 ? t('一天前') : days === 7 ? t('一周前') : t('一月前')
  const clear = () => {
    void Service.ClearHistory(days)
      .then((removed) => {
        onBusy(t('已清理 {n} 条', { n: removed }))
        refresh()
      })
      .catch((e) => onBusy(errMsg(e)))
  }

  const save = () => {
    void Service.SaveHistoryFile(needle, favoritesOnly, service, format)
      .then((path) => {
        // An empty answer is the panel being closed, not a failed write.
        onBusy(path ? t('已导出到 {path}', { path }) : t('已取消导出'))
      })
      .catch((e) => onBusy(errMsg(e)))
  }

  const copy = () => {
    void Service.ExportHistory(needle, favoritesOnly, service, format)
      .then((text) => {
        if (!text || text.trim() === '[]') {
          onBusy(t('没有可导出的记录'))
          return
        }
        // The export lands on the pasteboard as well as in a file: pasting into a
        // note is what people actually do with a translation log.
        void Service.CopyText(text)
        onBusy(t('已复制 {n} 字，可粘贴到笔记', { n: text.length }))
      })
      .catch((e) => onBusy(errMsg(e)))
  }

  // A filter with nothing behind it is not an empty log: saying 还没有记录 there
  // reads as if the export or the cleanup had removed rows that are still
  // there, one cleared filter away.
  const filtering = needle.trim() !== '' || favoritesOnly || service !== ''

  return (
    <div className="content single">
      <div className="card">
        <h3>
          {t('历史')}
          <span className="spacer" />
          <span className="byline">{t('{n} 条', { n: entries.length })}</span>
        </h3>
        <div className="hbar">
          <input
            className="grow"
            value={needle}
            placeholder={t('搜索原文或译文')}
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
            autoFocus
            onChange={(e) => setNeedle(e.target.value)}
          />
          <select value={service} onChange={(e) => setService(e.target.value)} aria-label={t('按服务筛选')}>
            <option value="">{t('全部服务')}</option>
            {services.map((c) => (
              <option key={c.name} value={c.name}>
                {c.label} · {c.entries}
              </option>
            ))}
          </select>
          <label className="check">
            <input type="checkbox" checked={favoritesOnly} onChange={(e) => setFavoritesOnly(e.target.checked)} />
            {t('只看收藏')}
          </label>
        </div>
        <div className="hbar">
          <span className="hbar-label">{t('导出格式')}</span>
          <select value={format} onChange={(e) => setFormat(e.target.value)} aria-label={t('导出格式')}>
            <option value="markdown">Markdown</option>
            <option value="json">JSON</option>
            <option value="csv">CSV</option>
          </select>
          <button onClick={save}>{t('导出文件')}</button>
          <button onClick={copy}>{t('复制到剪贴板')}</button>
          <span className="spacer" />
          <select value={days} onChange={(e) => setDays(Number(e.target.value))} aria-label={t('清理范围')}>
            <option value={0}>{t('全部（保留收藏）')}</option>
            <option value={1}>{t('一天前')}</option>
            <option value={7}>{t('一周前')}</option>
            <option value={30}>{t('一月前')}</option>
          </select>
          <ConfirmButton
            className="danger"
            label={t('清理')}
            question={t('确认清理？')}
            title={t('清理「{scope}」的记录？此操作不可撤销。', { scope })}
            onConfirm={clear}
          />
        </div>
      </div>

      <div className="card grow">
        <div className="body plain history">
          {entries.length === 0 && (
            <div className="muted">{filtering ? t('没有匹配的记录') : t('还没有记录')}</div>
          )}
          {entries.map((e) => (
            <div className="hrow" key={e.id}>
              <button
                className={`star ${e.favorite ? 'on' : ''}`}
                title={e.favorite ? t('取消收藏') : t('收藏')}
                onClick={() => void Service.StarHistory(e.id).then(() => refresh())}
              >
                {e.favorite ? '★' : '☆'}
              </button>
              <div className="htext" onClick={() => void Service.QueryText(e.source, 'main')}>
                <div className="src">{e.source}</div>
                {e.target && <div className="tgt">{e.target}</div>}
                <div className="meta">
                  <span>{where[e.kind || e.context] ?? e.kind ?? e.context}</span>
                  {e.services?.length ? <span>· {e.services.join(' / ')}</span> : null}
                  {e.targetLang ? <span>· {langName(e.targetLang, langNames) || e.targetLang}</span> : null}
                  <time>· {timeAgo(e.createdAt)}</time>
                </div>
              </div>
              <button className="tiny" onClick={() => void Service.DeleteHistory(e.id).then(() => refresh())}>{t('删除')}</button>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
