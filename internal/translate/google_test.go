package translate

import "testing"

// With 自动检测 the endpoint answers rows of [译文, 检测语种]; with an explicit
// source language it answers a flat array of strings. Both have to parse, or
// 中文→英文 falls through to the endpoint that rate-limits first.
func TestGoogleClients5ReadsBothShapes(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantText     string
		wantDetected string
	}{
		{"auto source", `[["你好世界","en"]]`, "你好世界", "en"},
		{"explicit source", `["Hello World"]`, "Hello World", ""},
		{"segments in rows", `[["Hello ","en"],["World","en"]]`, "Hello World", "en"},
		{"segments flat", `["Hello ","World"]`, "Hello World", ""},
	}
	for _, c := range cases {
		text, detected, err := parseClients5([]byte(c.body))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if text != c.wantText || detected != c.wantDetected {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, text, detected, c.wantText, c.wantDetected)
		}
	}
}

func TestGoogleClients5RejectsEmpty(t *testing.T) {
	if _, _, err := parseClients5([]byte(`[]`)); err == nil {
		t.Error("空数组应报错，不能当作空译文返回")
	}
	if _, _, err := parseClients5([]byte(`<html>`)); err == nil {
		t.Error("非 JSON 应报错")
	}
}
