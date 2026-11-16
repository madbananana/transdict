//go:build darwin

package gui

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include <math.h>
#include <dispatch/dispatch.h>
#import <AppKit/AppKit.h>

// The toast is EasyDict's EZToast: a small dark pill that names what a
// keyboard-only action just did (开关划词翻译) or reports a card action
// (添加到 Anki). Without it the global shortcut changes behaviour with nothing
// on screen to confirm it.
//
// Measurements are its defaults: labelMargin 20, minHeight 35, maxWidth 800,
// cornerRadius 10, 15pt text on #313131, dismiss after 2s, 0.5s fades. A toast
// with no anchor sits 55pt from the top and 20pt from the right; CTPositionMouse
// puts its top-left corner on the cursor instead.
//
// EZToast's success variant is a small ✓ at the pointer: labelMargin 5,
// min 25x25, cornerRadius 5, 20pt black glyph, #D6D6D6/#454545 in light mode
// and #404040/#C2C2C2 in dark. It is the only feedback 静音 OCR gives, so the
// panel is reused and restyled on every show.
static NSPanel *guiToastPanel = nil;
static NSView *guiToastBox = nil;
static NSTextField *guiToastLabel = nil;
// guiToastGen invalidates the pending dismissal when a newer toast replaces it.
static NSInteger guiToastGen = 0;

static void guiToastBuild(void) {
	guiToastPanel = [[NSPanel alloc]
		initWithContentRect:NSMakeRect(0, 0, 10, 35)
			  styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
				backing:NSBackingStoreBuffered
				  defer:NO];
	guiToastPanel.backgroundColor = [NSColor clearColor];
	guiToastPanel.opaque = NO;
	guiToastPanel.hasShadow = YES;
	guiToastPanel.level = NSPopUpMenuWindowLevel;
	// A confirmation must never eat the click that follows it.
	guiToastPanel.ignoresMouseEvents = YES;
	guiToastPanel.hidesOnDeactivate = NO;
	guiToastPanel.animationBehavior = NSWindowAnimationBehaviorNone;
	guiToastPanel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
		NSWindowCollectionBehaviorFullScreenAuxiliary |
		NSWindowCollectionBehaviorStationary;

	NSView *box = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 10, 35)];
	box.wantsLayer = YES;
	box.layer.backgroundColor = [NSColor colorWithCalibratedRed:49.0 / 255.0
														 green:49.0 / 255.0
														  blue:49.0 / 255.0
														 alpha:1.0].CGColor;
	box.layer.cornerRadius = 10;
	guiToastPanel.contentView = box;
	guiToastBox = box;

	guiToastLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(20, 0, 10, 18)];
	guiToastLabel.bezeled = NO;
	guiToastLabel.drawsBackground = NO;
	guiToastLabel.editable = NO;
	guiToastLabel.selectable = NO;
	guiToastLabel.font = [NSFont systemFontOfSize:15];
	guiToastLabel.textColor = [NSColor whiteColor];
	guiToastLabel.lineBreakMode = NSLineBreakByTruncatingTail;
	[box addSubview:guiToastLabel];
}

// guiToastScreen names the display the pointer is on, which is the one the user
// is looking at — the same rule EZToast follows rather than the main display.
static NSScreen *guiToastScreen(void) {
	NSPoint mouse = [NSEvent mouseLocation];
	for (NSScreen *screen in [NSScreen screens]) {
		if (NSPointInRect(mouse, screen.frame)) return screen;
	}
	return [NSScreen mainScreen];
}

// guiToastAppearanceIsDark follows EasyDict's NSObject.isDarkMode: the app's
// effective appearance decides, not the display or the page.
static BOOL guiToastAppearanceIsDark(void) {
	NSAppearanceName name = [[NSApp effectiveAppearance]
		bestMatchFromAppearancesWithNames:@[NSAppearanceNameDarkAqua, NSAppearanceNameAqua]];
	return [name isEqualToString:NSAppearanceNameDarkAqua];
}

static NSColor *guiToastColor(int red, int green, int blue) {
	return [NSColor colorWithSRGBRed:red / 255.0 green:green / 255.0 blue:blue / 255.0 alpha:1.0];
}

static void guiToastShowOnMain(NSString *message, int atMouse, int success) {
	if (guiToastPanel == nil) guiToastBuild();
	guiToastGen++;
	NSInteger gen = guiToastGen;

	CGFloat labelMargin;
	CGFloat minWidth;
	CGFloat minHeight;
	if (success) {
		message = @"✓";
		BOOL dark = guiToastAppearanceIsDark();
		guiToastLabel.font = [NSFont systemFontOfSize:20 weight:NSFontWeightBlack];
		guiToastLabel.alignment = NSTextAlignmentCenter;
		guiToastLabel.textColor = dark ? guiToastColor(0xC2, 0xC2, 0xC2) : guiToastColor(0x45, 0x45, 0x45);
		guiToastBox.layer.backgroundColor =
			(dark ? guiToastColor(0x40, 0x40, 0x40) : guiToastColor(0xD6, 0xD6, 0xD6)).CGColor;
		guiToastBox.layer.cornerRadius = 5;
		labelMargin = 5;
		minWidth = 25;
		minHeight = 25;
	} else {
		guiToastLabel.font = [NSFont systemFontOfSize:15];
		guiToastLabel.alignment = NSTextAlignmentLeft;
		guiToastLabel.textColor = [NSColor whiteColor];
		guiToastBox.layer.backgroundColor = guiToastColor(49, 49, 49).CGColor;
		guiToastBox.layer.cornerRadius = 10;
		labelMargin = 20;
		minWidth = 50;
		minHeight = 35;
	}

	guiToastLabel.stringValue = message;
	[guiToastLabel sizeToFit];
	CGFloat textWidth = ceil(guiToastLabel.frame.size.width);
	CGFloat textHeight = ceil(guiToastLabel.frame.size.height);

	CGFloat width = textWidth + labelMargin * 2;
	if (width < minWidth) width = minWidth;
	if (width > 800) width = 800;
	CGFloat height = success ? minHeight : textHeight + 16;
	if (height < minHeight) height = minHeight;

	NSRect visible = guiToastScreen().visibleFrame;
	NSRect frame;
	if (atMouse) {
		// CTPositionMouse: the pill's top-left corner sits on the pointer, and it
		// flips to the other side of the pointer rather than leaving the screen.
		NSPoint mouse = [NSEvent mouseLocation];
		CGFloat x = mouse.x;
		CGFloat y = mouse.y - height;
		if (x + width > NSMaxX(visible)) x = mouse.x - width;
		if (x < NSMinX(visible)) x = NSMinX(visible);
		if (y < NSMinY(visible)) y = mouse.y;
		if (y + height > NSMaxY(visible)) y = NSMaxY(visible) - height;
		if (y < NSMinY(visible)) y = NSMinY(visible);
		frame = NSMakeRect(x, y, width, height);
	} else {
		frame = NSMakeRect(NSMaxX(visible) - 20 - width,
						   NSMaxY(visible) - 55 - height, width, height);
	}
	[guiToastPanel setFrame:frame display:YES];
	[guiToastLabel setFrame:NSMakeRect(labelMargin, (height - textHeight) / 2, width - labelMargin * 2, textHeight)];

	[guiToastPanel setAlphaValue:0];
	[guiToastPanel orderFrontRegardless];
	[NSAnimationContext runAnimationGroup:^(NSAnimationContext *context) {
		context.duration = 0.5;
		guiToastPanel.animator.alphaValue = 1;
	} completionHandler:nil];

	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(2.0 * NSEC_PER_SEC)),
				   dispatch_get_main_queue(), ^{
		if (gen != guiToastGen) return;
		[NSAnimationContext runAnimationGroup:^(NSAnimationContext *context) {
			context.duration = 0.5;
			guiToastPanel.animator.alphaValue = 0;
		} completionHandler:^{
			if (gen == guiToastGen) [guiToastPanel orderOut:nil];
		}];
	});
}

// guiToastShow is safe from any goroutine; AppKit only takes it on the main
// queue, where the panel and its animations live.
static void guiToastShow(const char *text, int atMouse) {
	NSString *message = [NSString stringWithUTF8String:text];
	dispatch_async(dispatch_get_main_queue(), ^{
		guiToastShowOnMain(message, atMouse, 0);
	});
}

// guiToastSuccess is EZToast.showSuccessToast: the ✓ that confirms 静音 OCR
// copied the recognized text, at the pointer, with no window involved.
static void guiToastSuccess(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		guiToastShowOnMain(@"✓", 1, 1);
	});
}
*/
import "C"

import "unsafe"

// ShowToast puts a short confirmation in the top-right corner. It is the
// feedback EasyDict gives a keyboard-only action, so the app answers even when
// no window is open.
func ShowToast(text string) {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	C.guiToastShow(c, 0)
}

// ShowToastAtMouse puts the confirmation at the pointer instead, the way
// EasyDict reports 添加到 Anki: right where the click was aimed.
func ShowToastAtMouse(text string) {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	C.guiToastShow(c, 1)
}

// ShowSuccessToast is the ✓ EasyDict shows when 静音 OCR has copied the
// recognized text: no window opens, so the toast is the only confirmation.
func ShowSuccessToast() {
	C.guiToastSuccess()
}
