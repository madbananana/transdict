package gui

import (
	"errors"
	"testing"

	"transdict/internal/selection"
	"transdict/internal/settings"
)

func TestNoSelectionRecognisesBothQuietRoutes(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{selection.ErrNothingSelected, true},
		{selection.ErrTimeout, true},
		{selection.ErrUnsupported, false},
		{errors.New("权限"), false},
		{nil, false},
	}
	for _, c := range cases {
		if got := noSelection(c.err); got != c.want {
			t.Errorf("noSelection(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// 自动选中所有文本 widens only an empty selection; a permission refusal must
// keep its own message, and the option being off leaves 翻译并替换 as it was.
func TestWidenOnlyWhenTheOptionIsOnAndNothingIsSelected(t *testing.T) {
	on := settings.Config{AutoSelectAllText: true}
	off := settings.Config{}
	if !shouldWiden(on, selection.ErrNothingSelected) || !shouldWiden(on, selection.ErrTimeout) {
		t.Error("开关打开时，未选中文本应触发全选")
	}
	if shouldWiden(off, selection.ErrNothingSelected) {
		t.Error("开关关闭时不该全选")
	}
	if shouldWiden(on, selection.ErrUnsupported) || shouldWiden(on, errTrust) {
		t.Error("权限或平台拒绝不该被当成未选中文本")
	}
	if shouldWiden(on, nil) {
		t.Error("已经读到选区时不该全选")
	}
}

func TestKeepLastNeedsAnAnswer(t *testing.T) {
	s := &Service{}
	if s.keepLast() {
		t.Error("空的历史不应被当成已保留的结果")
	}
}

// 兼容模式替换 writes the answer through the accessibility API, and a refusal
// falls back to pasting rather than leaving the field untouched.
func TestReplaceWritesBackByPastingOrTyping(t *testing.T) {
	origPaste, origType := pasteAnswer, typeAnswer
	defer func() { pasteAnswer, typeAnswer = origPaste, origType }()

	var pasted, typed string
	clipboard := ""
	pasteAnswer = func() error { pasted = "yes"; return nil }
	typeAnswer = func(text string) error { typed = text; return nil }
	setClipboard := func(text string) bool { clipboard = text; return true }

	if err := writeBack(settings.Config{}, setClipboard, "译文"); err != nil {
		t.Fatalf("默认应粘贴：%v", err)
	}
	if pasted == "" || typed != "" || clipboard != "译文" {
		t.Errorf("默认路径应为剪贴板+粘贴，got pasted=%q typed=%q clipboard=%q", pasted, typed, clipboard)
	}

	pasted, typed, clipboard = "", "", ""
	if err := writeBack(settings.Config{ReplaceCompatible: true}, setClipboard, "译文"); err != nil {
		t.Fatalf("兼容模式应直接写入：%v", err)
	}
	if typed != "译文" || pasted != "" || clipboard != "" {
		t.Errorf("兼容模式应只写入，got pasted=%q typed=%q clipboard=%q", pasted, typed, clipboard)
	}

	pasted, typed, clipboard = "", "", ""
	typeAnswer = func(string) error { return errors.New("refused") }
	if err := writeBack(settings.Config{ReplaceCompatible: true}, setClipboard, "译文"); err != nil {
		t.Fatalf("写入被拒时应回退粘贴：%v", err)
	}
	if pasted == "" || clipboard != "译文" {
		t.Errorf("写入被拒应回退到粘贴，got pasted=%q typed=%q clipboard=%q", pasted, typed, clipboard)
	}

	setClipboard = func(string) bool { return false }
	if err := writeBack(settings.Config{}, setClipboard, "译文"); err == nil {
		t.Error("剪贴板写入失败时应报错")
	}
}

// 润色并替换 asks the AI stage with an instruction, exactly like the 润色 chip;
// a dictionary-only request would skip the one stage that can follow it.
func TestPolishReplaceUsesThePromptPath(t *testing.T) {
	req := polishReplaceRequest("hello")
	if req.Context != settings.ContextTranslation || req.Mode != "prompt" {
		t.Errorf("润色请求落在 %s/%s，应落在 AI 译文阶段", req.Context, req.Mode)
	}
	if req.Prompt != polishInstruction {
		t.Errorf("润色指令 = %q, want %q", req.Prompt, polishInstruction)
	}
	if req.DictionaryOnly {
		t.Error("润色被标记为仅查词：译文阶段会被跳过")
	}
}

// 替换 on a result card needs an answer; an empty one would paste over the
// selection and leave nothing behind.
func TestReplaceWithTextRejectsEmpty(t *testing.T) {
	s := &Service{}
	if err := s.ReplaceWithText("   "); err == nil {
		t.Fatal("空译文应被拒绝")
	}
}
