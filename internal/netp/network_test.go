package netp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// timeoutErr is what a socket gives up with — DNS, a dead peer. net.Error is the
// only shape that says so, and Reason has to catch it as well as the deadline.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "dial tcp: i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestSetTimeoutMovesEveryClient(t *testing.T) {
	defer SetTimeout(0)
	// 内置默认: each provider keeps the number it was written with, because a
	// dictionary round trip and a vision model do not take the same time.
	SetTimeout(0)
	if got := Client(15 * time.Second).Timeout; got != 15*time.Second {
		t.Fatalf("默认超时 = %v，期望服务自带的 15s", got)
	}
	SetTimeout(90)
	if got := Client(15 * time.Second).Timeout; got != 90*time.Second {
		t.Fatalf("设置后超时 = %v，期望 90s", got)
	}
	// A client built after the change is what picks it up, which is why the
	// pipeline constructs providers per query rather than caching them.
	if got := Timeout(time.Minute); got != 90*time.Second {
		t.Fatalf("Timeout = %v，期望 90s", got)
	}
	SetTimeout(-5)
	if got := Timeout(time.Minute); got != time.Minute {
		t.Fatalf("负数应回到默认，得到 %v", got)
	}
}

func TestReasonNamesTheDeadline(t *testing.T) {
	defer SetTimeout(0)
	if got := Reason(nil); got != "" {
		t.Errorf("nil = %q，期望空", got)
	}
	if got := Reason(context.DeadlineExceeded); got != "服务响应超时" {
		t.Errorf("上下文超时 = %q", got)
	}
	if got := Reason(timeoutErr{}); got != "服务响应超时" {
		t.Errorf("socket 超时 = %q", got)
	}
	// A service that answered, badly, is not a timeout: its own message is what
	// the user needs to see.
	denied := errors.New("HTTP 403：密钥无效")
	if got := Reason(denied); got != "HTTP 403：密钥无效" {
		t.Errorf("非超时错误被改写 = %q", got)
	}
}

// The client timeout is Go's own deadline error wrapped in http's message, so
// this is the test that says Reason works for the wall providers hit rather
// than only the one the pipeline sets.
func TestReasonNamesAClientTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() {
		close(block)
		srv.Close()
	}()

	_, err := Client(20 * time.Millisecond).Get(srv.URL)
	if err == nil {
		t.Fatal("慢响应未被客户端超时打断")
	}
	if got := Reason(err); got != "服务响应超时" {
		t.Fatalf("Reason(%v) = %q，期望超时文案", err, got)
	}
}
