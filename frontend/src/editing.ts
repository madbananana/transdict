// Clipboard keys for the query boxes. WKWebView sends ⌘C/⌘X through the Edit
// menu, so a query box whose window never becomes key would never reach the
// pasteboard. The handlers run inside the keydown, which counts as a user
// gesture.

function selectedQuery(): HTMLTextAreaElement | HTMLInputElement | null {
  const el = document.activeElement
  if (!(el instanceof HTMLTextAreaElement || el instanceof HTMLInputElement)) return null
  return el.selectionStart === el.selectionEnd ? null : el
}

/** copySelection copies the focused query's selection, or any selected answer
 * text. Returns false when there is nothing to copy, so the caller leaves the
 * key alone. */
export function copySelection(): boolean {
  if (!selectedQuery()) {
    const sel = window.getSelection()
    if (!sel || sel.isCollapsed || !sel.toString()) return false
  }
  return document.execCommand('copy')
}

/** cutSelection cuts the focused query's selection; the input event it fires
 * goes through the box's onChange like typed text. */
export function cutSelection(): boolean {
  return selectedQuery() ? document.execCommand('cut') : false
}
