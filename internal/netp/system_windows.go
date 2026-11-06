//go:build windows

package netp

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxy reads the Internet Settings keys that the browser and WinHTTP
// answer to. Go's transport knows only environment variables, so without this
// a Windows machine already behind a corporate proxy would keep reaching
// Google directly — and keep failing.
func systemProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return ""
	}
	raw, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	// Two shapes: one endpoint for every scheme ("127.0.0.1:7890") or a
	// per-scheme list ("http=127.0.0.1:7890;https=127.0.0.1:7890").
	if !strings.Contains(raw, "=") {
		return "http://" + strings.TrimSpace(raw)
	}
	for _, want := range []struct{ prefix, scheme string }{
		{"https=", "http://"}, {"http=", "http://"}, {"socks=", "socks5://"}} {
		for _, part := range strings.Split(raw, ";") {
			part = strings.TrimSpace(part)
			if rest, ok := strings.CutPrefix(part, want.prefix); ok && rest != "" {
				return want.scheme + rest
			}
		}
	}
	return ""
}
