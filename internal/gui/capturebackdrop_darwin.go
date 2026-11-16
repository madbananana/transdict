//go:build darwin && !ios

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework WebKit

#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>

// guiPaintWebviewBackdrop puts black behind the page instead of the WKWebView's
// default white. Wails colours the NSWindow, but the webview is an opaque sheet
// on top of it, and that white sheet is the flash before a screenshot overlay
// paints.
static void guiPaintWebviewBackdrop(void *nsWindow) {
	NSWindow *window = (NSWindow *)nsWindow;
	if (window == nil) {
		return;
	}
	NSColor *black = [NSColor blackColor];
	NSMutableArray *pending = [NSMutableArray arrayWithObject:[window contentView]];
	while (pending.count > 0) {
		NSView *view = pending.lastObject;
		[pending removeLastObject];
		if ([view isKindOfClass:[WKWebView class]]) {
			WKWebView *webview = (WKWebView *)view;
			if (@available(macOS 12.0, *)) {
				webview.underPageBackgroundColor = black;
			}
			webview.wantsLayer = YES;
			webview.layer.backgroundColor = black.CGColor;
		}
		[pending addObjectsFromArray:view.subviews];
	}
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func paintOverlayBackdrop(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	if handle := w.NativeWindow(); handle != nil {
		C.guiPaintWebviewBackdrop(handle)
	}
}
