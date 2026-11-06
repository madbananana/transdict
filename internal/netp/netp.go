// Package netp decides where the app's outbound HTTP goes and how long we wait
// for it.
//
// Every provider builds a plain http.Client, so they all share
// http.DefaultTransport. Replacing that one value at init — and never again —
// reaches two dozen services without touching a call site, while the proxy
// itself stays hot-swappable behind an atomic pointer.
//
// A proxy matters here more than in most apps: Google, Bing and DeepL are
// unreachable from the networks most users of a Chinese dictionary sit on, and
// Easydict answers that with a switch rather than a bug report.
//
// The deadline belongs here for the same reason. Each provider has always set
// its own http.Client timeout, so a 服务超时 setting that only moved the
// context deadline would still lose to a 15-second client; Client() is the one
// place the two are reconciled.
package netp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// chosen holds the explicit proxy in force, or nothing when the caller asked
// for 自动 and the operating system has none.
var chosen atomic.Pointer[url.URL]

// limit is the user's per-service wait in seconds. Zero leaves every provider
// on the timeout it was written with, which is why the default is zero rather
// than the smallest or largest number in the table.
var limit atomic.Int64

func init() {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	cloned := t.Clone()
	cloned.Proxy = route
	http.DefaultTransport = cloned
}

// route is the transport hook installed once. It reads the atomic pointer at
// request time, so a settings change applies to the next query rather than to
// a transport nobody is allowed to swap mid-flight.
func route(req *http.Request) (*url.URL, error) {
	if u := chosen.Load(); u != nil {
		return u, nil
	}
	return http.ProxyFromEnvironment(req)
}

// Apply points every provider at raw: a proxy URL, a bare "host:port", or an
// empty string for 自动, which asks the operating system and then the
// HTTP_PROXY family. The old value stays in force when raw cannot be used —
// otherwise a typo would take down the proxy that was working.
func Apply(raw string) error {
	u, err := resolve(raw)
	if err != nil {
		return err
	}
	chosen.Store(u)
	return nil
}

// Check validates a proxy string without installing it, which is what the
// settings page does before writing.
func Check(raw string) error {
	_, err := resolve(raw)
	return err
}

// SetTimeout installs the per-service wait in seconds; zero restores each
// provider's own. It is called on every config change, so the next query feels
// it without a restart.
func SetTimeout(seconds int) {
	if seconds < 0 {
		seconds = 0
	}
	limit.Store(int64(seconds))
}

// Timeout returns def unless the user named a number of seconds to wait.
func Timeout(def time.Duration) time.Duration {
	if s := limit.Load(); s > 0 {
		return time.Duration(s) * time.Second
	}
	return def
}

// Client is the http.Client every provider should build. Leaving Transport nil
// keeps the shared one — and with it the proxy — while Timeout carries the
// setting, resolved when the provider is constructed rather than captured.
func Client(def time.Duration) *http.Client {
	return &http.Client{Timeout: Timeout(def)}
}

// Reason names an error the way a result row shows it. Giving up because of the
// deadline this package installed is the one failure the user caused, so it
// should read as a timeout rather than as the runtime's sentence about a
// context — and a fixed phrase is what both string tables can translate, which a
// formatted "waited 45 s" is not.
func Reason(err error) string {
	if err == nil {
		return ""
	}
	if deadlineHit(err) {
		return "服务响应超时"
	}
	return err.Error()
}

// deadlineHit covers both walls that can expire: the pipeline's context
// deadline and the provider's own client timeout, which since Go 1.18 also
// unwraps to DeadlineExceeded. net.Error catches what the socket itself gave up
// on, such as a DNS query that never answered.
func deadlineHit(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func resolve(raw string) (*url.URL, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		target = systemProxy()
		if target == "" {
			return nil, nil
		}
	}
	return parse(target)
}

// parse accepts "http://127.0.0.1:7890" and "socks5://127.0.0.1:7891" as well
// as the bare "127.0.0.1:7890" people copy out of a proxy client, which is an
// HTTP proxy rather than a syntax error.
func parse(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("代理地址无法解析")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("代理只支持 http、https 与 socks5")
	}
	if u.Host == "" {
		return nil, errors.New("代理缺少地址")
	}
	return u, nil
}
