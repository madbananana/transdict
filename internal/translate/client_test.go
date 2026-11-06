package translate

import (
	"testing"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// The pipeline's context deadline is not enough on its own: each provider also
// caps its own socket work with an http.Client timeout, and the user always
// meets the smaller of the two. This is what keeps 服务超时 an honest setting
// rather than one that Google's fifteen-second client quietly overrules.
func TestProviderClientFollowsTheSetting(t *testing.T) {
	t.Cleanup(func() { netp.SetTimeout(0) })

	p, err := New(settings.Provider{Kind: "google"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*google).httpc.Timeout; got != 15*time.Second {
		t.Fatalf("默认超时 = %v，期望服务自带的 15s", got)
	}

	netp.SetTimeout(60)
	slow, err := New(settings.Provider{Kind: "google"})
	if err != nil {
		t.Fatal(err)
	}
	if got := slow.(*google).httpc.Timeout; got != 60*time.Second {
		t.Fatalf("设置后超时 = %v，期望 60s", got)
	}
}
