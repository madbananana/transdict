import ReactDOM from 'react-dom/client'
import App from './App'
import { isMac } from './api'

// Windows keeps no native title bar (the three windows draw their own), so the
// header does not reserve the traffic-light gutter or repeat the system frame.
if (!isMac) {
  document.documentElement.classList.add('not-mac')
}

// WebKit reports a clicked button as :focus-visible too, so its ring lingers on
// whatever was just pressed. The flag below is set only while the keyboard is
// the last thing used, which is when the ring is actually useful.
const keyboard = () => document.documentElement.classList.add('kbd-nav')
const pointer = () => document.documentElement.classList.remove('kbd-nav')
window.addEventListener('keydown', keyboard, true)
window.addEventListener('pointerdown', pointer, true)

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(<App />)
