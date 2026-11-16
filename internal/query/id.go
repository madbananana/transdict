package query

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// DataURL inlines an image for the webview. Passing bytes through the binding
// layer rather than an HTTP route keeps dev mode (Vite serves the frontend) and
// production (embedded assets) identical. The media type follows the bytes: the
// screenshot overlay is handed a JPEG preview, and a PNG label on JPEG data is
// only tolerated by sniffing.
func DataURL(image []byte) string {
	if len(image) == 0 {
		return ""
	}
	kind := "image/png"
	if len(image) >= 3 && image[0] == 0xFF && image[1] == 0xD8 && image[2] == 0xFF {
		kind = "image/jpeg"
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(image)
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", len(b))
	}
	return hex.EncodeToString(b[:])
}
