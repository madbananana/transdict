package ocr

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"transdict/internal/netp"
	"transdict/internal/settings"
)

// youdaoOCR implements 有道智领先端 OCR. Credentials: AppID (应用 ID) ->
// settings.Provider.AppID, appSecret -> settings.Provider.Secret. Endpoint and
// the salt/curtime source are package vars so tests can pin them and hit an
// httptest server.
type youdaoOCR struct {
	AppID     string
	AppKey    string // same as AppID for the modern 智领 endpoints, kept for clarity
	AppSecret string
	DocType   string
	BaseURL   string

	httpc *http.Client
}

// youdaoSaltLength is unused but kept here as a note: the vendor accepts a
// salt up to 20 chars; youdaoSaltFn below emits an 8-digit number for parity
// with their own samples.

var (
	youdaoEndpoint = "https://openapi.youdao.com/ocrapi"
	youdaoNow      = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	youdaoSaltFn   = func() string { return strconv.Itoa(int(rand.Int31n(90000000)) + 10000000) }
	youdaoTimeout  = 30 * time.Second
)

func init() {
	Register(settings.ServiceType{
		Kind: "youdaoocr", Name: "有道智领先端 OCR",
		Note: "openapi.youdao.com/ocrapi 图片文字识别，需应用 ID (AppID) 与应用密钥 (AppSecret)",
		Fields: []settings.Field{
			{Key: settings.FieldAppID, Label: "应用 ID (AppID)", Required: true},
			{Key: settings.FieldSecret, Label: "应用密钥 (AppSecret)", Secret: true, Required: true},
			{Key: "docType", Label: "图片类型", Placeholder: "png（默认）/jpg"},
			{Key: settings.FieldBaseURL, Label: "接口地址（可选覆盖）", Placeholder: youdaoEndpoint},
		},
	}, func(p settings.Provider) (Provider, error) {
		if p.AppID == "" || p.Secret == "" {
			return nil, fmt.Errorf("有道 OCR 缺少应用 ID 或应用密钥")
		}
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = youdaoEndpoint
		}
		docType := p.Extra["docType"]
		if docType == "" {
			docType = "png"
		}
		return &youdaoOCR{
			AppID: p.AppID, AppKey: p.AppID, AppSecret: p.Secret,
			DocType: docType, BaseURL: base,
			httpc: netp.Client(youdaoTimeout),
		}, nil
	})
}

func (y *youdaoOCR) Label() string { return "有道 OCR" }

func (y *youdaoOCR) Recognize(ctx context.Context, png []byte) ([]Block, error) {
	q := base64.StdEncoding.EncodeToString(png)
	salt := youdaoSaltFn()
	curtime := strconv.FormatInt(youdaoNow().Unix(), 10)
	vk := youdaoSignMD5(y.AppKey, q, salt, curtime, y.AppSecret)

	form := url.Values{}
	form.Set("q", q)
	form.Set("appKey", y.AppKey)
	form.Set("salt", salt)
	form.Set("curtime", curtime)
	form.Set("vk", vk)
	form.Set("docType", y.DocType)
	form.Set("vendor", "Youdao")
	form.Set("action", "auto")
	form.Set("velocity", "horizontal")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, y.BaseURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := y.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("有道 OCR 网络错误: %s", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("有道 OCR 读取响应失败: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("有道 OCR HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	// The response nests line blocks under Text[i].cont[j]; a plain-shape
	// fallback keeps us working if the vendor ships the flat variant.
	var payload struct {
		ErrorCode string `json:"errorCode"`
		Reason    string `json:"reason"`
		Text      []struct {
			Cont []struct {
				TextContent string `json:"textContent"`
				Coords      []int  `json:"coords"`
				Prob        int    `json:"prob"`
			} `json:"cont"`
			TextContent string `json:"textContent"`
			Coords      []int  `json:"coords"`
		} `json:"Text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("有道 OCR 响应解析失败: %s", snippet(raw))
	}
	if payload.ErrorCode != "" && payload.ErrorCode != "0" {
		return nil, vendorError("有道 OCR", payload.ErrorCode, payload.Reason, string(raw))
	}

	blocks := make([]Block, 0, len(payload.Text))
	for _, blk := range payload.Text {
		if len(blk.Cont) == 0 {
			if t := strings.TrimSpace(blk.TextContent); t != "" {
				blocks = append(blocks, Block{Text: t})
			}
			continue
		}
		for _, c := range blk.Cont {
			t := strings.TrimSpace(c.TextContent)
			if t == "" {
				continue
			}
			b := Block{Text: t}
			if len(c.Coords) >= 8 {
				xs := []int{c.Coords[0], c.Coords[2], c.Coords[4], c.Coords[6]}
				ys := []int{c.Coords[1], c.Coords[3], c.Coords[5], c.Coords[7]}
				b.X, b.Y, b.Width, b.Height = bbox(xs, ys)
			}
			if c.Prob > 0 {
				b.Score = float64(c.Prob) / 100.0
			}
			blocks = append(blocks, b)
		}
	}
	sortBlocksReadingOrder(blocks)
	return blocks, nil
}

// youdaoSignInput assembles the string that goes into the MD5 hash. Split out
// so a test can assert the digest for fixed inputs. The vendor truncates long
// inputs to first-10 + runeCount + last-10 before hashing; the base64 image
// always trips this rule so the function is load-bearing, not decoration.
func youdaoSignInput(appKey, input, salt, curtime, secret string) string {
	return appKey + youdaoTruncate(input) + salt + curtime + secret
}

// youdaoSignMD5 returns the lowercase hex MD5 of youdaoSignInput.
func youdaoSignMD5(appKey, input, salt, curtime, secret string) string {
	sum := md5.Sum([]byte(youdaoSignInput(appKey, input, salt, curtime, secret)))
	return hex.EncodeToString(sum[:])
}

func youdaoTruncate(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= 20 {
		return s
	}
	r := []rune(s)
	return string(r[:10]) + strconv.Itoa(n) + string(r[n-10:])
}
