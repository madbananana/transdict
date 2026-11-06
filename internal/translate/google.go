package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// Google uses two unofficial endpoints. `clients5.google.com` (what Chrome's
// dictionary bubble calls) still answers on networks where the `gtx` widget
// endpoint returns a block page, so it is tried first.
type google struct {
	httpc *http.Client
	// Empty means the real hosts; tests point these at a local server.
	clients5URL string
	gtxURL      string
}

func init() {
	Register(settings.ServiceType{
		Kind: "google", Name: "Google", Note: "免密钥，国内网络通常需代理", Free: true,
		Site:    "https://translate.google.com/?sl={source}&tl={target}&text={text}&op=translate",
		Default: settings.Provider{Enabled: true},
	}, func(settings.Provider) (Provider, error) {
		return &google{httpc: netp.Client(15 * time.Second)}, nil
	})
}

func (g *google) Label() string { return "Google" }

func (g *google) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	if sourceLang == "" {
		sourceLang = "auto"
	}
	// Both hosts take the text in a GET query, so a paragraph long enough to
	// overflow the URL is cut into pieces first; without this Google answered
	// 400 for a few thousand 汉字.
	join := trJoinFor(targetLang)
	out, detected, err := trFanoutURL(ctx, text, trURLBudget, join,
		func(chunk string) (string, string, error) {
			return g.viaClients5(ctx, chunk, sourceLang, targetLang)
		})
	if err == nil {
		return out, detected, nil
	}
	fallback, fbDetected, fbErr := trFanoutURL(ctx, text, trURLBudget, join,
		func(chunk string) (string, string, error) {
			return g.viaGtx(ctx, chunk, sourceLang, targetLang)
		})
	if fbErr != nil {
		// Both errors are wrapped, not printed: a deadline that expired on both
		// hosts still has to read as one timeout in the panel.
		return "", "", fmt.Errorf("%w（备用端点同样失败: %w）", err, fbErr)
	}
	return fallback, fbDetected, nil
}

func (g *google) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := g.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// viaClients5 parses [[ "译文", "检测语种" ], ...]. With an explicit source
// language the endpoint drops the per-row wrapper and answers ["译文"].
func (g *google) viaClients5(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	q := url.Values{}
	q.Set("client", "dict-chrome-ex")
	q.Set("sl", sourceLang)
	q.Set("tl", targetLang)
	q.Set("q", text)

	endpoint := g.clients5URL
	if endpoint == "" {
		endpoint = "https://clients5.google.com/translate_a/t"
	}
	body, err := g.get(ctx, endpoint+"?"+q.Encode())
	if err != nil {
		return "", "", err
	}
	return parseClients5(body)
}

// parseClients5 reads either shape: an array of rows, or an array of strings.
// Refusing the flat one left 中文→英文 failures on the fallback endpoint, which
// then reported a 429.
func parseClients5(body []byte) (string, string, error) {
	if !strings.HasPrefix(strings.TrimLeft(string(body), " \n\t"), "[") {
		return "", "", fmt.Errorf("Google 端点被拦截（返回非 JSON）")
	}

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return "", "", fmt.Errorf("Google 响应格式异常: %s", truncate(string(body)))
	}

	var sb strings.Builder
	detected := ""
	for _, raw := range items {
		var row []string
		if err := json.Unmarshal(raw, &row); err == nil {
			if len(row) == 0 {
				continue
			}
			sb.WriteString(row[0])
			if len(row) > 1 && detected == "" {
				detected = row[1]
			}
			continue
		}
		var chunk string
		if err := json.Unmarshal(raw, &chunk); err == nil {
			sb.WriteString(chunk)
		}
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", "", fmt.Errorf("Google 返回空译文")
	}
	return out, detected, nil
}

// viaGtx parses [[["译文","原文",null,null,10]],null,"en",...].
func (g *google) viaGtx(ctx context.Context, text, sourceLang, targetLang string) (string, string, error) {
	q := url.Values{}
	q.Set("client", "gtx")
	q.Set("sl", sourceLang)
	q.Set("tl", targetLang)
	q.Set("dt", "t")
	q.Set("q", text)

	endpoint := g.gtxURL
	if endpoint == "" {
		endpoint = "https://translate.googleapis.com/translate_a/single"
	}
	body, err := g.get(ctx, endpoint+"?"+q.Encode())
	if err != nil {
		return "", "", err
	}

	var outer []json.RawMessage
	if err := json.Unmarshal(body, &outer); err != nil || len(outer) < 3 {
		return "", "", fmt.Errorf("Google gtx 端点被拦截或响应格式异常")
	}

	var segments [][]json.RawMessage
	if err := json.Unmarshal(outer[0], &segments); err != nil {
		return "", "", fmt.Errorf("Google 分段解析失败")
	}

	var sb strings.Builder
	for _, seg := range segments {
		if len(seg) == 0 {
			continue
		}
		var chunk string
		if err := json.Unmarshal(seg[0], &chunk); err == nil {
			sb.WriteString(chunk)
		}
	}

	var detected string
	_ = json.Unmarshal(outer[2], &detected)

	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", "", fmt.Errorf("Google 返回空译文")
	}
	return out, detected, nil
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
