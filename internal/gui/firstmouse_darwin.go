//go:build darwin

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#import <objc/runtime.h>

static BOOL guiAlwaysAcceptFirstMouse(id self, SEL _cmd, NSEvent *event) {
	return YES;
}

// guiPatchClass teaches one class to accept the click that arrives while the
// app is not frontmost. A class that defines the method already gets its own
// implementation replaced; one that only inherits it gets an override added, so
// patching NSView does not drag every AppKit control along with it.
static void guiPatchClass(const char *name) {
	Class cls = objc_getClass(name);
	if (cls == Nil) {
		return;
	}
	SEL sel = sel_registerName("acceptsFirstMouse:");
	unsigned int count = 0;
	Method *methods = class_copyMethodList(cls, &count);
	Method own = NULL;
	for (unsigned int i = 0; i < count; i++) {
		if (method_getName(methods[i]) == sel) {
			own = methods[i];
			break;
		}
	}
	if (methods != NULL) {
		free(methods);
	}
	if (own != NULL) {
		method_setImplementation(own, (IMP)guiAlwaysAcceptFirstMouse);
		return;
	}
	Method inherited = class_getInstanceMethod(cls, sel);
	if (inherited != NULL) {
		class_addMethod(cls, sel, (IMP)guiAlwaysAcceptFirstMouse, method_getTypeEncoding(inherited));
	}
}

// The 划词查询图标 is a non-activating panel, so the click that lands on it
// while another app is frontmost is a "first mouse" event: macOS asks the view
// under the pointer whether it wants it, and WebKit's web view says no. The
// click was swallowed, which is why the button needed two clicks.
void guiAcceptFirstMouseInViews(void) {
	guiPatchClass("NSView");
	guiPatchClass("WKWebView");
	guiPatchClass("WKContentView");
	guiPatchClass("WKFlippedView");
	guiPatchClass("WKScrollView");
}
*/
import "C"

func acceptFirstMouseInViews() { C.guiAcceptFirstMouseInViews() }
