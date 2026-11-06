// Package dock decides how the app sits in the desktop shell.
//
// Transdict is driven from the menu bar and global shortcuts, so the Dock tile
// is the opt-in rather than the default — the same way Easydict hides itself
// until asked. Only macOS has a notion of a background app; Windows shows a
// taskbar button while a window is open and nothing otherwise, so there is
// nothing to switch.
//
// Giving up the tile has a second consequence: the process stops being one the
// system activates for you, so raising a window leaves the keyboard with
// whatever app was frontmost. Activate is the answer for the windows the user
// asked for.
package dock
