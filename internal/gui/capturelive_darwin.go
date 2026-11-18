//go:build darwin

package gui

// liveOverlay is EasyDict's flow: a transparent marquee over the running screen,
// with the pixels read only after the selection is released. Freezing first
// would put a full-display capture and encode in front of the shortcut.
const liveOverlay = true
