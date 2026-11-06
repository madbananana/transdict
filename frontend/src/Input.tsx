// 输入翻译: type or paste text and ask every service of the main context. The
// AI chips reuse the same providers with a different instruction, which is how
// 润色 and 总结 avoid a second pipeline.
import { useEffect, useRef, useState } from 'react'
import { Service, errMsg, type Lang } from './api'
import { t } from './i18n'

const instructions: { label: string; text: string }[] = [
  { label: t('润色'), text: t('在不改变原意的前提下润色下面的文字，使其更地道、通顺：') },
  { label: t('总结'), text: t('用三句以内的中文总结下面的文字：') },
  { label: t('更正式'), text: t('把下面的文字改写得更正式、书面化：') },
  { label: t('更口语'), text: t('把下面的文字改写得更口语、自然：') },
  { label: t('纠错'), text: t('纠正下面文字中的拼写、语法与标点错误，只输出修改后的文字：') },
  { label: t('扩写'), text: t('在保持原意的前提下扩写下面的文字：') },
]

export default function InputView({ langs, onBusy }: { langs: Lang[]; onBusy: (text: string) => void }) {
  const [text, setText] = useState('')
  const [source, setSource] = useState('auto')
  const [target, setTarget] = useState('zh-CN')
  const [dictionaryOnly, setDictionaryOnly] = useState(false)
  const box = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    box.current?.focus()
  }, [])

  const run = (instruction?: string) => {
    const value = text.trim()
    if (!value) {
      onBusy(t('请输入文字'))
      return
    }
    if (instruction) {
      void Service.Prompt(value, instruction).catch((e) => onBusy(errMsg(e)))
      return
    }
    onBusy(dictionaryOnly ? t('查词典中…') : t('查询中…'))
    void Service.Ask({
      context: 'main', mode: 'input', text: value,
      sourceLang: source, targetLang: target, dictionaryOnly,
    }).catch((e) => onBusy(errMsg(e)))
  }

  return (
    <div className="content single">
      <div className="card">
        <h3>
          {t('输入')}
          <span className="spacer" />
          <span className="byline">{t('{n} 字', { n: Array.from(text).length })}</span>
        </h3>
        <textarea
          ref={box}
          className="input-area"
          value={text}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          placeholder={t('输入或粘贴要翻译的文字，⌘/Ctrl + 回车 查询')}
          onChange={(e) => setText(e.target.value)}
          // ⌘/Ctrl+↵ runs; a plain ↵ stays a newline, which multi-line text needs.
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) run()
          }}
        />
        <div className="row picks">
          <label className="pick">
            {t('源语言')}
            <select value={source} onChange={(e) => setSource(e.target.value)}>
              <option value="auto">{t('自动检测')}</option>
              {langs.filter((l) => l.code !== 'auto').map((l) => (
                <option key={l.code} value={l.code}>{l.name}</option>
              ))}
            </select>
          </label>
          <label className="pick">
            {t('目标语言')}
            <select value={target} onChange={(e) => setTarget(e.target.value)}>
              {langs.filter((l) => l.code !== 'auto').map((l) => (
                <option key={l.code} value={l.code}>{l.name}</option>
              ))}
            </select>
          </label>
          <label className="check">
            <input type="checkbox" checked={dictionaryOnly} onChange={(e) => setDictionaryOnly(e.target.checked)} />
            {t('仅查词典')}
          </label>
          <div className="spacer" />
          <button className="primary" onClick={() => run()}>{t('查询')}</button>
        </div>
      </div>

      <div className="card">
        <h3>
          {t('AI 指令')}
          <span className="spacer" />
          <span className="byline">{t('需要启用一个 AI 翻译服务')}</span>
        </h3>
        <div className="chips">
          {instructions.map((i) => (
            <button key={i.label} className="chip" disabled={!text.trim()} onClick={() => run(i.text)}>
              {i.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
