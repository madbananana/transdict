package settings

import "testing"

// The panel only knows a service's web page through the URL it hands back, so a
// template must expand into a link that still points at the same words the user
// queried — including the characters a URL cannot carry raw.
func TestSiteURLFillsTheServicePage(t *testing.T) {
	cases := []struct {
		name           string
		site           string
		text, src, dst string
		want           string
	}{
		{"没有网页版", "", "word", "auto", "en", ""},
		{"词典只带词条", "https://www.iciba.com/word?w={text}", "a b&c", "", "",
			"https://www.iciba.com/word?w=a+b%26c"},
		{"翻译带语种", "https://translate.google.com/?sl={source}&tl={target}&text={text}",
			"你好", "auto", "en-US", "https://translate.google.com/?sl=auto&tl=en-US&text=%E4%BD%A0%E5%A5%BD"},
		{"重复占位符都替换", "https://e.com/?a={text}&b={text}", "x", "", "", "https://e.com/?a=x&b=x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ServiceType{Site: c.site}.SiteURL(c.text, c.src, c.dst)
			if got != c.want {
				t.Errorf("SiteURL() = %q, want %q", got, c.want)
			}
		})
	}
}

// A page is only worth offering when it can be asked for the thing on screen, so
// internal/query checks every registered template expands without leftovers.
func TestSiteURLIgnoresUnknownPlaceholders(t *testing.T) {
	got := ServiceType{Site: "https://e.com/?a={word}"}.SiteURL("测试", "auto", "en")
	if got != "https://e.com/?a={word}" {
		t.Errorf("SiteURL() = %q, want 原样保留（只有三个已知占位符）", got)
	}
}
