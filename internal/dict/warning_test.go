package dict

import (
	"context"
	"errors"
	"testing"
)

type failingProvider struct{ err error }

func (f failingProvider) Label() string { return "stub" }
func (f failingProvider) Explain(context.Context, string, string, string) ([]Entry, error) {
	return nil, f.err
}

type emptyProvider struct{}

func (emptyProvider) Label() string { return "stub" }
func (emptyProvider) Explain(context.Context, string, string, string) ([]Entry, error) {
	return nil, nil
}

// A dictionary saying "not in me" folds to a warning row, while an HTTP fault
// stays a red error 重试 can fix — EasyDict's noResult versus a real failure.
func TestNotFoundIsAWarningButAFaultIsNot(t *testing.T) {
	miss := RunQuery(context.Background(), "stub", "Stub",
		failingProvider{NotFound("苹果词典未收录该词：zzz")}, "zzz", "en", "zh-CN")
	if !miss.Warning {
		t.Errorf("未收录应标记为 warning: %+v", miss)
	}
	if miss.Error != "苹果词典未收录该词：zzz" {
		t.Errorf("错误文案 = %q, 期望保留服务原文", miss.Error)
	}

	fault := RunQuery(context.Background(), "stub", "Stub",
		failingProvider{errors.New("苹果词典 HTTP 500：boom")}, "zzz", "en", "zh-CN")
	if fault.Warning {
		t.Errorf("网络故障不应是 warning: %+v", fault)
	}
	if fault.Error != "苹果词典 HTTP 500：boom" {
		t.Errorf("错误文案 = %q", fault.Error)
	}

	empty := RunQuery(context.Background(), "stub", "Stub", emptyProvider{}, "zzz", "en", "zh-CN")
	if empty.Warning || empty.Error != "" {
		t.Errorf("空词条回答既不是 warning 也不是错误: %+v", empty)
	}
}

// A dictionary refusing the query type keeps 重试 in EasyDict: ⊘ without the
// fold, because asking again with a word could work.
func TestUnsupportedQueryIsNotAWarning(t *testing.T) {
	res := RunQuery(context.Background(), "stub", "Stub",
		failingProvider{UnsupportedQuery("苹果词典：仅支持单词查询")}, "a whole sentence", "en", "zh-CN")
	if !res.Unsupported || res.Warning {
		t.Errorf("查询类型不支持应只标记 unsupported: %+v", res)
	}
	if res.Error != "苹果词典：仅支持单词查询" {
		t.Errorf("错误文案 = %q", res.Error)
	}
}
