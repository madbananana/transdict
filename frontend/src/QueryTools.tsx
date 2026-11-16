// The two icons EasyDict's EZQueryView keeps at the bottom of every query
// view, beside the detect chip.
export function SpeakGlyph({ stop }: { stop?: boolean }) {
  if (stop) {
    return (
      <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <rect x="6.5" y="6" width="4" height="12" rx="1.2" />
        <rect x="13.5" y="6" width="4" height="12" rx="1.2" />
      </svg>
    )
  }
  return (
    <svg
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M11 5 6.5 9H3v6h3.5L11 19z" />
      <path d="M15.5 9.5a3.5 3.5 0 0 1 0 5" />
      <path d="M18 7a7 7 0 0 1 0 10" />
    </svg>
  )
}

export function CopyGlyph() {
  return (
    <svg
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="9" y="9" width="10" height="10" rx="2" />
      <path d="M5 15V6a1 1 0 0 1 1-1h9" />
    </svg>
  )
}

// EasyDict draws its 快捷动作 trigger with SF Symbol switch.2; WKWebView has no
// SF Symbols, so this is the same two toggles with their knobs at opposite ends.
export function QuickActionGlyph() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 18 18"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      aria-hidden="true"
    >
      <rect x="1.6" y="2.7" width="14.8" height="5.6" rx="2.8" />
      <circle cx="4.6" cy="5.5" r="1.4" fill="currentColor" stroke="none" />
      <rect x="1.6" y="9.7" width="14.8" height="5.6" rx="2.8" />
      <circle cx="13.4" cy="12.5" r="1.4" fill="currentColor" stroke="none" />
    </svg>
  )
}

// SwapIcon is EasyDict's transform image: two arrows leaving in opposite
// directions, drawn crisp instead of the text glyph's thin ⇄.
export function SwapIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 8h14" />
      <path d="m14.5 4.5 3.5 3.5-3.5 3.5" />
      <path d="M20 16H6" />
      <path d="M9.5 12.5 6 16l3.5 3.5" />
    </svg>
  )
}
