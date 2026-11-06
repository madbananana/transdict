// Package update asks a release source whether this build is behind.
//
// There is no updater here: the app publishes a release and the user installs
// it. What the code can do is notice the difference and hand over the link,
// which is the whole of what a desktop user needs from 检查更新.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"transdict/internal/netp"
)

// checkTimeout stays short on purpose. A launch must not wait on a host the
// network cannot reach, and nobody reads release notes at ten seconds.
const checkTimeout = 10 * time.Second

// bodyLimit covers a changelog written by a human. Beyond it the response is
// not a manifest but a mistake, and reading it whole would cost memory for
// nothing.
const bodyLimit = 1 << 20

// Release is what a user needs in order to act: which version exists, where to
// get it, and what changed.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

// manifest accepts the two shapes a release can be published in: the flat
// document this app would serve itself, and a GitHub Releases API entry, whose
// tag_name/html_url/body say the same three things in other words.
type manifest struct {
	Version string `json:"version"`
	Tag     string `json:"tag_name"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
	Notes   string `json:"notes"`
	Body    string `json:"body"`
}

// Fetch reads the manifest at url. Anything that does not name a version is an
// error rather than a quiet 已是最新, because a wrong url would otherwise look
// like an up-to-date app forever.
func Fetch(ctx context.Context, url string) (*Release, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("未设置更新源")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(url), nil)
	if err != nil {
		return nil, errors.New("更新源地址无效")
	}
	req.Header.Set("User-Agent", "transdict-update-check")
	// The client comes from netp so the proxy and 服务超时 settings apply here as
	// well: a release host unreachable from the user's network is the same
	// problem Google is.
	resp, err := netp.Client(checkTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接更新源: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新源返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("读取更新源失败: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.New("更新源返回的不是发布文件")
	}
	rel := &Release{
		Version: firstOf(m.Version, m.Tag),
		URL:     firstOf(m.URL, m.HTMLURL),
		Notes:   firstOf(m.Notes, m.Body),
	}
	if rel.Version == "" {
		return nil, errors.New("发布文件没有版本号")
	}
	return rel, nil
}

// Check is the one call the app makes: it reports the release to install when
// the source is ahead of the running version, and nothing when it is not. The
// caller decides whether 没有新版本 is a sentence or silence.
func Check(ctx context.Context, url, current string) (*Release, error) {
	rel, err := Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	if !Newer(current, rel.Version) {
		return nil, nil
	}
	return rel, nil
}

// Newer reports whether latest supersedes current.
func Newer(current, latest string) bool {
	return compare(current, latest) < 0
}

func compare(a, b string) int {
	as, bs := segments(a), segments(b)
	for i := range max(len(as), len(bs)) {
		if d := compareSegment(segmentAt(as, i), segmentAt(bs, i)); d != 0 {
			return d
		}
	}
	return compareSuffix(a, b)
}

// segments splits a tag into the parts that can be ordered. A leading v is how
// tags are written, and a suffix (rc1, beta.2) is not a number, so the release
// line is what gets compared.
func segments(v string) []string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil
	}
	return strings.Split(v, ".")
}

func segmentAt(parts []string, i int) string {
	if i >= len(parts) {
		return ""
	}
	return parts[i]
}

// compareSegment orders numbers as numbers — 1.10 beats 1.9 — and treats what is
// not a number as behind, so a development build always hears about a release.
// A missing segment is zero, which makes 1.2 and 1.2.0 the same version.
func compareSegment(a, b string) int {
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return compareInt(an, bn)
	case aerr == nil:
		return 1
	case berr == nil:
		return -1
	}
	return strings.Compare(a, b)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// suffix is the pre-release part of a tag — the rc1 in 1.2.3-rc1. A +build
// marker is metadata and says nothing about which build is newer, so it goes.
func suffix(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	i := strings.Index(v, "-")
	if i < 0 {
		return ""
	}
	if j := strings.IndexByte(v[i+1:], '+'); j >= 0 {
		return v[i+1 : i+1+j]
	}
	return v[i+1:]
}

// compareSuffix orders two builds of the same release line: the one without a
// pre-release marker is the published one and comes last.
func compareSuffix(a, b string) int {
	as, bs := suffix(a), suffix(b)
	switch {
	case as == bs:
		return 0
	case as == "":
		return 1
	case bs == "":
		return -1
	}
	return strings.Compare(as, bs)
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
