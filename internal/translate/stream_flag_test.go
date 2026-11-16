package translate

import (
	"context"
	"testing"
)

type plainProvider struct{}

func (plainProvider) Label() string { return "普通" }

func (plainProvider) Translate(context.Context, string, string, string) (string, string, error) {
	return "你好", "", nil
}

type streamProvider struct{ plainProvider }

func (streamProvider) TranslateStream(ctx context.Context, text, src, dst string,
	onText func(string)) (string, string, error) {
	return streamProvider{}.Translate(ctx, text, src, dst)
}

// EasyDict only reads an empty answer as noResult for services that cannot
// stream, so the pipeline has to know which kind answered even when the caller
// passed no progress callback.
func TestResultRecordsWhetherTheServiceStreams(t *testing.T) {
	plain := RunQuery(context.Background(), "tr-plain", "普通", plainProvider{}, "hello", "en", "zh-CN")
	if plain.Stream {
		t.Error("普通服务被标记成流式")
	}
	stream := RunQueryStream(context.Background(), "tr-ai", "AI", streamProvider{}, "hello", "en", "zh-CN", nil)
	if !stream.Stream {
		t.Error("流式服务没有标记")
	}
}

// A service whose 启用流式传输 switch is off takes the one-shot path: the answer
// is not a stream, so an endpoint that buffers or refuses `stream` is reported
// as an ordinary failure at worst, never as an empty stream.
type toggledStreamProvider struct {
	streamProvider
	on bool
}

func (t toggledStreamProvider) StreamingEnabled() bool { return t.on }

func TestStreamToggleTakesTheOneShotPath(t *testing.T) {
	off := RunQueryStream(context.Background(), "tr-ai", "AI", toggledStreamProvider{on: false},
		"hello", "en", "zh-CN", nil)
	if off.Stream {
		t.Error("关闭流式传输后仍被标记成流式")
	}
	on := RunQueryStream(context.Background(), "tr-ai", "AI", toggledStreamProvider{on: true},
		"hello", "en", "zh-CN", nil)
	if !on.Stream {
		t.Error("开启流式传输后没有标记流式")
	}
}
