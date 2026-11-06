//go:build darwin

package autostart

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework ServiceManagement -framework Foundation

#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>

// SMAppService is macOS 13 or newer, and a login item only has meaning once the
// process runs from an app bundle. Both cases answer through one status code so
// the Go side never deals with availability.
enum {
	kTransdictStatusUnknown = 0,
	kTransdictStatusEnabled = 1,
	kTransdictStatusDisabled = 2,
};

static int SMStatus(void) {
	if (@available(macOS 13, *)) {
		switch (SMAppService.mainAppService.status) {
			case SMAppServiceStatusEnabled: return kTransdictStatusEnabled;
			// RequiresApproval means the app asked and the user has not allowed
			// it yet; the login item will not run, so it is not enabled.
			case SMAppServiceStatusRequiresApproval:
			case SMAppServiceStatusNotRegistered:
			case SMAppServiceStatusNotFound: return kTransdictStatusDisabled;
		}
	}
	return kTransdictStatusUnknown;
}

// SMSet registers or removes the login item and returns a message written for
// the user, or NULL when the request took. Apps must be code signed for this to
// work, which is why a development build gets an explanation rather than a
// silent failure.
static char *SMSet(bool enabled) {
	if (@available(macOS 13, *)) {
		SMAppService *service = SMAppService.mainAppService;
		NSError *error = nil;
		BOOL ok = enabled ? [service registerAndReturnError:&error]
						  : [service unregisterAndReturnError:&error];
		if (ok) return NULL;
		if (enabled && service.status == SMAppServiceStatusRequiresApproval) {
			return strdup("已登记，请在 系统设置 → 通用 → 登录项 中允许 Transdict");
		}
		NSString *text = error.localizedDescription;
		if (text == nil) text = @"登录项设置失败";
		return strdup(text.UTF8String);
	}
	return strdup("开机自启需要 macOS 13 或更高版本");
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func Set(enabled bool) error {
	raw := C.SMSet(C.bool(enabled))
	if raw == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(raw))
	message := C.GoString(raw)
	if message == "" {
		return errors.New("开机自启设置失败")
	}
	return errors.New(message)
}

func Status() State {
	switch C.SMStatus() {
	case C.kTransdictStatusEnabled:
		return Enabled
	case C.kTransdictStatusDisabled:
		return Disabled
	}
	return Unknown
}
