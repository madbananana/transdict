// A destructive button that asks in place. The Wails webview does not run
// window.confirm, so a modal would never appear; arming the button keeps the
// question in the row the action belongs to. The second click runs it, and
// looking away for four seconds disarms it again.
import { useEffect, useState, type ReactNode } from 'react'

export function ConfirmButton({ label, question, title, className = '', disabled, onConfirm }: {
  label: ReactNode
  question: string
  title?: string
  className?: string
  disabled?: boolean
  onConfirm: () => void
}) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const timer = setTimeout(() => setArmed(false), 4000)
    return () => clearTimeout(timer)
  }, [armed])
  return (
    <button
      className={`${className}${armed ? ' armed' : ''}`}
      title={title}
      disabled={disabled}
      onClick={() => {
        if (!armed) {
          setArmed(true)
          return
        }
        setArmed(false)
        onConfirm()
      }}
    >
      {armed ? question : label}
    </button>
  )
}
