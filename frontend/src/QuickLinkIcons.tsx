// The titlebar 快捷链接 wear the colour of the app they open, the way
// EasyDict's quick links show the real app icons. All three are drawn on the
// same 16px box as the tool glyphs, with padding inside the viewBox where the
// artwork fills it, so the strip keeps one optical size.

// Google's four-colour G. The path fills its 48-unit viewBox, so the box is
// padded to 64: without that the solid G reads a third larger than the books.
export function GoogleMark() {
  return (
    <svg width="16" height="16" viewBox="-8 -8 64 64" aria-hidden="true">
      <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
      <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
      <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
    </svg>
  )
}

// Apple Dictionary's red cover, with the cream page block at its edge and the
// gold rules a dictionary wears. A closed book keeps it from reading as the
// same mark as Eudic's open one at titlebar size. The cover is as wide as the
// open book beside it so the two marks do not look like different sizes.
export function DictionaryMark() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
      <rect x="3.8" y="3.6" width="16.4" height="16.8" rx="1.8" fill="#f7e8d0" />
      <rect x="3.8" y="3.6" width="14.2" height="16.8" rx="1.5" fill="#d2453f" />
      <path stroke="#f3cf92" strokeWidth="1.2" strokeLinecap="round" d="M7.2 7.8h6.4M7.2 10.6h4.4" />
      <path stroke="#b93a35" strokeWidth="1" strokeLinecap="round" d="M3.8 6.6v11" />
    </svg>
  )
}

// Eudic's blue open dictionary. The two apps are both books, so the colour and
// the open/closed silhouette are what tell them apart in the strip.
export function EudicMark() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#2e9cf2" d="M12 5.7C10.4 4.4 8.2 3.8 5.6 3.8c-1 0-1.9.12-2.6.36v13.9c.7-.24 1.6-.36 2.6-.36 2.6 0 4.8.6 6.4 1.9 1.6-1.3 3.8-1.9 6.4-1.9 1 0 1.9.12 2.6.36V4.16c-.7-.24-1.6-.36-2.6-.36-2.6 0-4.8.6-6.4 1.9Z" />
      <path stroke="#d9efff" strokeWidth="1.1" strokeLinecap="round" d="M7.1 8.1c.9 0 1.7.12 2.4.36M7.1 10.7c.9 0 1.7.12 2.4.36M16.9 8.1c-.9 0-1.7.12-2.4.36M16.9 10.7c-.9 0-1.7.12-2.4.36" />
      <path stroke="#eaf7ff" strokeWidth="1.3" strokeLinecap="round" d="M12 5.9v13.5" />
    </svg>
  )
}
