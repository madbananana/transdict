//go:build darwin

package netp

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework SystemConfiguration -framework CoreFoundation

#import <CoreFoundation/CoreFoundation.h>
#import <SystemConfiguration/SystemConfiguration.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// The enable flags are CFBoolean and the ports CFNumber, so one reader per
// shape covers the whole dictionary.
static long long numberOf(CFDictionaryRef d, CFStringRef key) {
	CFTypeRef v = CFDictionaryGetValue(d, key);
	if (!v) return 0;
	if (CFGetTypeID(v) == CFBooleanGetTypeID()) return CFBooleanGetValue((CFBooleanRef)v) ? 1 : 0;
	if (CFGetTypeID(v) == CFNumberGetTypeID()) {
		long long n = 0;
		if (CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, &n)) return n;
	}
	return 0;
}

static char *valueOf(CFDictionaryRef d, CFStringRef key) {
	CFTypeRef v = CFDictionaryGetValue(d, key);
	if (!v || CFGetTypeID(v) != CFStringGetTypeID()) return NULL;
	char buf[256];
	if (!CFStringGetCString((CFStringRef)v, buf, sizeof(buf), kCFStringEncodingUTF8)) return NULL;
	return strdup(buf);
}

// The system proxy as a URL, or NULL when nothing is switched on. Plain HTTP
// comes first: a proxy client that configures both serves the same endpoint,
// and an HTTP proxy carries https traffic too through CONNECT.
static char *systemProxyURL(void) {
	CFDictionaryRef d = SCDynamicStoreCopyProxies(NULL);
	if (!d) return NULL;

	CFStringRef enables[3] = {kSCPropNetProxiesHTTPEnable, kSCPropNetProxiesHTTPSEnable,
		kSCPropNetProxiesSOCKSEnable};
	CFStringRef hosts[3] = {kSCPropNetProxiesHTTPProxy, kSCPropNetProxiesHTTPSProxy,
		kSCPropNetProxiesSOCKSProxy};
	CFStringRef ports[3] = {kSCPropNetProxiesHTTPPort, kSCPropNetProxiesHTTPSPort,
		kSCPropNetProxiesSOCKSPort};
	const char *schemes[3] = {"http", "https", "socks5"};

	char *out = NULL;
	for (int i = 0; i < 3 && out == NULL; i++) {
		if (numberOf(d, enables[i]) == 0) continue;
		char *host = valueOf(d, hosts[i]);
		long long port = numberOf(d, ports[i]);
		if (host && port > 0) {
			char buf[320];
			snprintf(buf, sizeof(buf), "%s://%s:%lld", schemes[i], host, port);
			out = strdup(buf);
		}
		free(host);
	}
	CFRelease(d);
	return out;
}
*/
import "C"
import "unsafe"

func systemProxy() string {
	v := C.systemProxyURL()
	if v == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(v))
	return C.GoString(v)
}
