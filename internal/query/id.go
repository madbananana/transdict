package query

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// DataURL inlines an image for the webview. Passing bytes through the binding
// layer rather than an HTTP route keeps dev mode (Vite serves the frontend) and
// production (embedded assets) identical.
func DataURL(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", len(b))
	}
	return hex.EncodeToString(b[:])
}
