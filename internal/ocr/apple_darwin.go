//go:build darwin

package ocr

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework Vision -framework CoreGraphics -framework ImageIO -framework CoreText
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <math.h>

#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>
#import <Vision/Vision.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreText/CoreText.h>
#import <ImageIO/ImageIO.h>
#import <objc/message.h>
#import <objc/runtime.h>

// Two tiny helpers keep the JSON encoding in one place. cJsonFromString
// mallocs a NUL-terminated UTF-8 copy the caller must free(); on any failure
// it falls back to "[]" so the Go side never sees a NULL on the happy path.
static char *cJsonFromString(NSString *s) {
    if (s == nil) return strdup("[]");
    NSData *d = [s dataUsingEncoding:NSUTF8StringEncoding];
    if (d == nil) return strdup("[]");
    char *buf = (char *)malloc((size_t)d.length + 1);
    if (buf == NULL) return strdup("[]");
    memcpy(buf, d.bytes, (size_t)d.length);
    buf[d.length] = '\0';
    return buf;
}

static char *cErrorJson(NSString *msg) {
    if (msg == nil) msg = @"Vision 未知错误";
    NSDictionary *wrap = @{@"error": msg};
    NSData *j = [NSJSONSerialization dataWithJSONObject:wrap options:0 error:nil];
    if (j == nil) {
        NSString *fallback = [NSString stringWithFormat:@"{\"error\":\"%@\"}", msg];
        return cJsonFromString(fallback);
    }
    return cJsonFromString([[NSString alloc] initWithData:j encoding:NSUTF8StringEncoding]);
}

// Emit one Block dict from a normalized Vision boundingBox (origin at the
// image's bottom-left) plus its confidence, translating to pixel top-left
// coordinates that Block uses.
static NSDictionary *cBoxFromRect(CGRect bb, NSString *text, double score,
                                  size_t pxW, size_t pxH) {
    int x = (int)llround(bb.origin.x * (double)pxW);
    int y = (int)llround((1.0 - bb.origin.y - bb.size.height) * (double)pxH);
    int w = (int)llround(bb.size.width  * (double)pxW);
    int h = (int)llround(bb.size.height * (double)pxH);
    return @{
        @"text": text ?: @"",
        @"x": @(x), @"y": @(y), @"width": @(w), @"height": @(h),
        @"score": @(score),
    };
}

// CRecognizePNG decodes the PNG bytes in `data`, runs Vision's text pass, and
// writes a UTF-8 JSON string to *outJson (malloc'd; caller frees).
//
//   includeBarcode  — also queue a VNDetectBarcodesRequest; its payload
//                     strings are emitted ONLY when the text pass returned
//                     nothing (Easydict's 截图取码 behaviour).
//   langHint        — BCP-47-ish code ("zh-Hans", "en", "ja", ...) or NULL;
//                     forwarded to VNRecognizeTextRequest.recognitionLanguages.
//
// On success *outJson is a JSON array of {text,x,y,width,height,score}. On
// failure it is a {"error":"..."} object so the Go wrapper can surface the
// reason without leaking any CF/ObjC references across the boundary.
void CRecognizePNG(const uint8_t *data, int len, char **outJson,
                   int includeBarcode, const char *langHint)
{
    if (outJson == NULL) return;
    *outJson = NULL;
    if (data == NULL || len <= 0) { *outJson = strdup("[]"); return; }

    @autoreleasepool {
        NSData *imgData = [NSData dataWithBytes:(const void *)data length:(NSUInteger)len];
        if (imgData == nil) { *outJson = cErrorJson(@"无法载入 PNG 字节"); return; }

        CGImageSourceRef src = CGImageSourceCreateWithData((__bridge CFDataRef)imgData, NULL);
        if (src == NULL) { *outJson = cErrorJson(@"ImageIO 无法解析图像数据"); return; }
        CGImageRef img = CGImageSourceCreateImageAtIndex(src, 0, NULL);
        CFRelease(src);
        if (img == NULL) { *outJson = cErrorJson(@"PNG 中未找到位图"); return; }

        size_t pxW = CGImageGetWidth(img);
        size_t pxH = CGImageGetHeight(img);
        if (pxW == 0 || pxH == 0) {
            CGImageRelease(img);
            *outJson = strdup("[]");
            return;
        }

        VNImageRequestHandler *handler =
            [[VNImageRequestHandler alloc] initWithCGImage:img options:@{}];

        VNRecognizeTextRequest *textReq = [[VNRecognizeTextRequest alloc] init];
        if ([textReq respondsToSelector:@selector(setRecognitionLevel:)]) {
            textReq.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
        }
        if ([textReq respondsToSelector:@selector(setUsesLanguageCorrection:)]) {
            textReq.usesLanguageCorrection = YES;
        }
        // Prefer the highest revision the OS exposes without tripping an
        // availability check at compile time: `currentRevision` is only
        // visible from the macOS 15 SDK, but the runtime lookup keeps the
        // binary loadable on older hosts as well.
        Class textReqClass = object_getClass((id)textReq);
        SEL curRevSel = NSSelectorFromString(@"currentRevision");
        SEL setRevSel = NSSelectorFromString(@"setRevision:");
        if (textReqClass != Nil && curRevSel != NULL && setRevSel != NULL &&
            [textReqClass respondsToSelector:curRevSel] &&
            [textReq respondsToSelector:setRevSel]) {
            typedef NSInteger (*RevGetter)(id, SEL);
            RevGetter getRev = (RevGetter)objc_msgSend;
            NSInteger rev = getRev((id)textReqClass, curRevSel);
            if (rev > 0) {
                typedef void (*RevSetter)(id, SEL, NSInteger);
                RevSetter setRev = (RevSetter)objc_msgSend;
                setRev((id)textReq, setRevSel, rev);
            }
        }
        // Vision's default recognition set is English-only: a Chinese or
        // Japanese screenshot then yields no observations at all. Widen the
        // default to every language this macOS build supports, and let an
        // explicit hint (the source language) replace it. Unknown tags are
        // filtered instead of sent, because one bad tag makes the whole
        // assignment raise.
        NSMutableArray<NSString *> *wanted = [NSMutableArray array];
        if (langHint != NULL && langHint[0] != '\0') {
            NSString *code = [NSString stringWithUTF8String:langHint];
            if (code.length > 0) [wanted addObject:code];
        } else {
            [wanted addObjectsFromArray:@[
                @"zh-Hans", @"zh-Hant", @"en-US", @"ja-JP", @"ko-KR",
                @"fr-FR", @"de-DE", @"es-ES", @"it-IT", @"pt-BR", @"ru-RU",
            ]];
        }
        if (wanted.count > 0) {
            NSArray<NSString *> *supported = nil;
            if (@available(macOS 11.0, *)) {
                supported = [textReq supportedRecognitionLanguagesAndReturnError:nil];
            }
            if (supported != nil) {
                NSMutableArray<NSString *> *usable = [NSMutableArray array];
                for (NSString *code in wanted) {
                    if ([supported containsObject:code]) [usable addObject:code];
                }
                if (usable.count > 0) {
                    @try {
                        textReq.recognitionLanguages = usable;
                    } @catch (NSException *e) {
                        // Keep Vision's default list rather than fail the
                        // whole screenshot over a language tag.
                    }
                }
            }
        }

        NSMutableArray<VNRequest *> *requests = [NSMutableArray arrayWithObject:textReq];
        VNDetectBarcodesRequest *barcodeReq = nil;
        if (includeBarcode) {
            barcodeReq = [[VNDetectBarcodesRequest alloc] init];
            [requests addObject:barcodeReq];
        }

        NSError *err = nil;
        BOOL ok = [handler performRequests:requests error:&err];

        NSMutableArray *items = [NSMutableArray array];
        if (ok) {
            for (VNRecognizedTextObservation *obs in textReq.results) {
                VNRecognizedText *top = nil;
                if ([obs respondsToSelector:@selector(topCandidates:)]) {
                    NSArray *cands = [obs topCandidates:1];
                    if (cands.count > 0) top = [cands firstObject];
                }
                if (top == nil) continue;
                NSString *txt = top.string;
                if (txt.length == 0) continue;
                [items addObject:cBoxFromRect(obs.boundingBox, txt, top.confidence, pxW, pxH)];
            }
            if (items.count == 0 && barcodeReq != nil) {
                for (VNBarcodeObservation *obs in barcodeReq.results) {
                    NSString *payload = obs.payloadStringValue;
                    if (payload.length == 0) continue;
                    [items addObject:cBoxFromRect(obs.boundingBox, payload, 1.0, pxW, pxH)];
                }
            }
        }

        CGImageRelease(img);

        if (!ok) {
            NSString *msg = err.localizedDescription ?: @"Vision 请求失败";
            *outJson = cErrorJson(msg);
            return;
        }
        NSData *json = [NSJSONSerialization dataWithJSONObject:items options:0 error:nil];
        if (json == nil) { *outJson = strdup("[]"); return; }
        NSString *s = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
        *outJson = cJsonFromString(s);
    }
}

// CAppleVisionAvailable reports whether Vision.framework resolved enough
// symbols to run at all. A CI runner on an unsupported macOS release can then
// skip the smoke test rather than crash inside performRequests:.
int CAppleVisionAvailable(void) {
    Class text = NSClassFromString(@"VNRecognizeTextRequest");
    Class handler = NSClassFromString(@"VNImageRequestHandler");
    Class barcode = NSClassFromString(@"VNDetectBarcodesRequest");
    if (text == Nil || handler == Nil || barcode == Nil) return 0;
    return 1;
}

// CWarmVision runs one throwaway recognition pass over an image we draw
// ourselves, so the recognizer loads its model before the user's first
// screenshot needs it. A cold request costs ~12s on macOS 26 while a warm one
// costs ~50ms, and nothing else can pre-load that model.
int CWarmVision(void) {
    @autoreleasepool {
        const size_t w = 384, h = 96;
        CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
        CGContextRef ctx = CGBitmapContextCreate(NULL, w, h, 8, 0, cs,
                                                 (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
        CGColorSpaceRelease(cs);
        if (ctx == NULL) return 0;
        CGContextSetGrayFillColor(ctx, 1.0, 1.0);
        CGContextFillRect(ctx, CGRectMake(0, 0, (CGFloat)w, (CGFloat)h));
        CGContextSetGrayFillColor(ctx, 0.0, 1.0);

        NSAttributedString *text = [[NSAttributedString alloc] initWithString:@"Transdict"
            attributes:@{NSFontAttributeName: [NSFont systemFontOfSize:40]}];
        CTLineRef line = CTLineCreateWithAttributedString((__bridge CFAttributedStringRef)text);
        CGContextSetTextPosition(ctx, 16, 28);
        CTLineDraw(line, ctx);
        CFRelease(line);

        CGImageRef img = CGBitmapContextCreateImage(ctx);
        CGContextRelease(ctx);
        if (img == NULL) return 0;

        VNRecognizeTextRequest *req = [[VNRecognizeTextRequest alloc] init];
        if ([req respondsToSelector:@selector(setRecognitionLevel:)]) {
            req.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
        }
        VNImageRequestHandler *handler =
            [[VNImageRequestHandler alloc] initWithCGImage:img options:@{}];
        NSError *err = nil;
        BOOL ok = [handler performRequests:@[req] error:&err];
        CGImageRelease(img);
        return ok ? 1 : 0;
    }
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unsafe"

	"transdict/internal/settings"
)

// appleVision wraps the Objective-C recognizer registered under kind "apple".
// No credentials — it is Free by design so it can be the default OCR path on
// every Mac install.
type appleVision struct {
	IncludeBarcode bool
	LangHint       string
}

func init() {
	Register(settings.ServiceType{
		Kind: "apple", Name: "Apple Vision",
		Note: "系统自带 · 离线 · 无需密钥 · 附带二维码识别",
		Free: true,
		Fields: []settings.Field{
			{Key: "langHint", Label: "语言提示（可选）", Placeholder: "zh-Hans / en / ja / ko …"},
			{Key: "includeBarcode", Label: "无文字时提取二维码/条码", Placeholder: "留空=开启，0=关闭"},
		},
	}, func(p settings.Provider) (Provider, error) {
		include := true
		if v := strings.ToLower(strings.TrimSpace(p.Extra["includeBarcode"])); v == "0" || v == "false" || v == "off" {
			include = false
		}
		return &appleVision{
			IncludeBarcode: include,
			LangHint:       strings.TrimSpace(p.Extra["langHint"]),
		}, nil
	})
}

func (a *appleVision) Label() string { return "Apple Vision（离线）" }

// CAppleVisionAvailable is the Go-side thin wrapper around the C probe used by
// the darwin test suite so we can skip on hosts where Vision.framework refuses
// to load (CI with an unsupported macOS release, non-Xcode toolchain, etc.).
func CAppleVisionAvailable() int32 { return int32(C.CAppleVisionAvailable()) }

// Warm pays the recognizer's one-time model load on an image we draw ourselves.
// A cold request costs ~12s on macOS 26 and a warm one ~50ms, so without this
// the very first 截图翻译 looks stuck while every later one is instant.
func Warm() { C.CWarmVision() }

// visionLang translates our language codes into the tags Vision recognizes.
// The C side also filters unsupported tags, but zh-CN cannot match anything
// until it becomes zh-Hans.
func visionLang(code string) string {
	switch code {
	case "zh-CN":
		return "zh-Hans"
	case "zh-TW":
		return "zh-Hant"
	case "en":
		return "en-US"
	case "ja":
		return "ja-JP"
	case "ko":
		return "ko-KR"
	case "fr":
		return "fr-FR"
	case "de":
		return "de-DE"
	case "es":
		return "es-ES"
	case "it":
		return "it-IT"
	case "pt":
		return "pt-BR"
	case "ru":
		return "ru-RU"
	}
	return code
}

func (a *appleVision) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	if len(png) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hint := visionLang(a.LangHint)
	if h := LangFromContext(ctx); h != "" {
		hint = visionLang(h)
	}

	var cHint *C.char
	if hint != "" {
		cHint = C.CString(hint)
		defer C.free(unsafe.Pointer(cHint))
	}

	var out *C.char
	barcode := C.int(0)
	if a.IncludeBarcode {
		barcode = 1
	}

	// &png[0] is a Go pointer into Go memory; cgo's rules let us hand it over
	// for the duration of the call because the ObjC side copies the bytes
	// into an NSData before returning. &out is Go memory that will hold a
	// C-allocated pointer, which cgo explicitly allows.
	C.CRecognizePNG(
		(*C.uint8_t)(unsafe.Pointer(&png[0])),
		C.int(len(png)),
		&out,
		barcode,
		cHint,
	)
	if out == nil {
		return nil, fmt.Errorf("Apple Vision: 未返回结果")
	}
	raw := C.GoString(out)
	C.free(unsafe.Pointer(out))

	trimmed := strings.TrimLeft(raw, " \t\r\n")
	if strings.HasPrefix(trimmed, "{") {
		var e struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(raw), &e); err == nil && e.Error != "" {
			return nil, fmt.Errorf("Apple Vision: %s", e.Error)
		}
	}

	var blocks []Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return nil, fmt.Errorf("Apple Vision 响应解析失败: %s", snippet([]byte(raw)))
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}
