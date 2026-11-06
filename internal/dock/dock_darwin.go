//go:build darwin

package dock

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// An accessory app keeps its windows and its global hotkeys but drops the Dock
// tile and the application menu, which is what a menu-bar tool wants. Answering
// through one int keeps the boolean switch on the Go side.
static void applyPolicy(int showInDock) {
	NSApplicationActivationPolicy policy = NSApplicationActivationPolicyAccessory;
	if (showInDock != 0) {
		policy = NSApplicationActivationPolicyRegular;
	}
	[[NSApplication sharedApplication] setActivationPolicy:policy];
}

// A background app has to ask for activation itself; makeKeyAndOrderFront only
// orders within our own app and leaves the previous one frontmost. Activation
// lands a runloop turn later, which is why the window is not yet key when this
// returns — AppKit finishes the handover before the user can type.
static void raiseApp(void) {
	NSApplication *app = [NSApplication sharedApplication];
	[app activateIgnoringOtherApps:YES];
	[[app mainWindow] makeKeyAndOrderFront:nil];
}
*/
import "C"

// Set moves the app between the menu bar and the Dock. AppKit expects this on
// the main thread, so callers go through application.InvokeAsync.
func Set(showInDock bool) {
	value := C.int(0)
	if showInDock {
		value = 1
	}
	C.applyPolicy(value)
}

// Activate brings the app forward so the window shown alongside it really does
// take the keyboard. Main thread only, for the same reason as Set.
func Activate() { C.raiseApp() }
