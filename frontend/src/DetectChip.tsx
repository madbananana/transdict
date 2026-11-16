// 识别为, EasyDict's EZDetectLanguageButton: a small grey pill whose language
// name sits in blue (#007AFF) and opens the whole language table. Picking the
// language that is already shown does nothing — selectedLanguageBlock only
// retries when the choice differs from detectedLanguage — so retry behaviour
// stays in the caller and the chip only reports the pick.
import { useState } from 'react'
import { langOption, pickableLangs, type Lang } from './api'
import { t } from './i18n'

export function DetectChip({ label, current, disabled, langs, onPick }: {
  label: string
  current: string
  disabled?: boolean
  langs: Lang[]
  onPick: (code: string) => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="qwrap">
      <button
        type="button"
        className="lang-chip"
        title={t('识别为 {lang}', { lang: label })}
        aria-haspopup="menu"
        aria-expanded={open}
        // A source language picked by hand makes the correction moot, so the
        // chip stays readable but inert, like EasyDict's disabled button.
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="chip-lead">{t('识别为')}</span> <span className="chip-lang">{label}</span>
      </button>
      {open && (
        <>
          <div className="qshade" onClick={() => setOpen(false)} />
          <div className="qmenu langmenu" role="menu">
            {pickableLangs(langs).map((l) => (
              <button
                key={l.code}
                type="button"
                role="menuitem"
                className={current === l.code ? 'on' : ''}
                onClick={() => {
                  setOpen(false)
                  onPick(l.code)
                }}
              >
                {current === l.code ? '✓ ' : ''}{langOption(l.code, l.name)}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
