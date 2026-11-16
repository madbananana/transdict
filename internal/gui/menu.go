package gui

// The 快捷键 menu's actions, named the way EasyDict's ShortcutAction spells them.
// The menu and the ⌘ keys in the windows answer the same names, so an action
// added to one is never left out of the other.
const (
	ActClearInput   = "clearInput"
	ActClearAll     = "clearAll"
	ActCopy         = "copy"
	ActCopyFirst    = "copyFirstResult"
	ActFocus        = "focus"
	ActPlay         = "play"
	ActRetry        = "retry"
	ActToggle       = "toggle"
	ActPin          = "pin"
	ActHide         = "hide"
	ActFontIncrease = "increaseFontSize"
	ActFontDecrease = "decreaseFontSize"
	ActGoogle       = "google"
	ActEudic        = "eudic"
	ActAppleDic     = "appleDic"
)

// RunMenuAction forwards a 快捷键 item to the query window that holds the
// keyboard. The menu bar is the app's, not a webview's, so the page that can
// actually clear or copy is only known here.
//
//wails:ignore
func (s *Service) RunMenuAction(action string) {
	s.wails.Event.Emit(EventMenuAction, MenuAction{Target: WindowMain, Action: action})
}
