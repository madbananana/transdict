// The read-aloud player is one element shared by every window. EasyDict's
// EZAudioButton and ⌘S (togglePlayQueryText) are both play/stop toggles, so a
// request for the clip already playing has to stop it instead of restarting it.
// The decision is kept here, away from the browser audio element, so the cases
// are testable.

/** speechKey names one request: the text, the voice language, and the speed. */
export function speechKey(text: string, lang: string, slow: boolean): string {
  return `${slow ? 'slow' : 'normal'}|${lang}|${text}`
}

/** stopsPlayback decides whether a press should stop rather than play. A clip
 * that finished (or was stopped) no longer counts, so the next press plays. */
export function stopsPlayback(
  playing: boolean,
  playingKey: string,
  text: string,
  lang: string,
  slow: boolean,
): boolean {
  return playing && playingKey === speechKey(text, lang, slow)
}
