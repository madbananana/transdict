package netp

import (
	"net/http"
	"net/url"
	"testing"
)

// restore leaves the shared transport as the next test found it, because the
// pointer is process-wide by design.
func restore(t *testing.T) {
	t.Helper()
	before := chosen.Load()
	t.Cleanup(func() { chosen.Store(before) })
}

func proxyFor(t *testing.T, target string) *url.URL {
	t.Helper()
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("默认传输被换成了 %T", http.DefaultTransport)
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestApplyRoutesEveryProvider(t *testing.T) {
	restore(t)
	// A bare host:port is what a proxy client shows, so it has to work as
	// typed rather than only after the user adds a scheme by hand.
	if err := Apply("127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	got := proxyFor(t, "https://translate.googleapis.com/language/translate/v2")
	if got == nil || got.String() != "http://127.0.0.1:7890" {
		t.Fatalf("代理未生效: %v", got)
	}
}

func TestApplyKeepsSocksFive(t *testing.T) {
	restore(t)
	if err := Apply("socks5://127.0.0.1:1080"); err != nil {
		t.Fatal(err)
	}
	if got := proxyFor(t, "http://dict.youdao.com/x"); got == nil || got.Scheme != "socks5" {
		t.Fatalf("socks5 代理丢失: %v", got)
	}
}

func TestApplyRejectsWhatCannotBeUsed(t *testing.T) {
	restore(t)
	for _, bad := range []string{"ftp://127.0.0.1:21", "http://", "://127.0.0.1:7890"} {
		if err := Apply(bad); err == nil {
			t.Errorf("%q 被接受为代理", bad)
		}
	}
	// Rejecting has to leave the routing where it was: a hand-edited file that
	// says 自动 with nothing behind it means direct, not the old proxy.
	if err := Apply("http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	if err := Apply("ftp://127.0.0.1:21"); err == nil {
		t.Fatal("ftp 被接受为代理")
	}
	if got := proxyFor(t, "https://www.bing.com/x"); got == nil || got.Port() != "7890" {
		t.Fatalf("被拒绝的地址改动了正在使用的代理: %v", got)
	}
}

func TestCheckValidatesWithoutMovingTheTraffic(t *testing.T) {
	restore(t)
	if err := Apply("http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	if err := Check("ftp://127.0.0.1:21"); err == nil {
		t.Fatal("Check 放过了一个不能用的地址")
	}
	if got := proxyFor(t, "https://www.bing.com/x"); got == nil || got.Port() != "7890" {
		t.Fatalf("Check 改动了正在使用的代理: %v", got)
	}
	if err := Check(""); err != nil {
		t.Errorf("空值应当是「自动」: %v", err)
	}
}
