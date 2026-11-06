// The two macOS grants this app can be missing are dead ends twice over: the
// system prompt only appears once, and the checkbox it points at only takes
// effect after the app restarts. So the first press asks the operating system
// and the presses after it get a dialog with the two buttons that finish the job.
package gui

import (
	"sync"

	"transdict/internal/capture"
	"transdict/internal/l10n"
	"transdict/internal/selection"
)

// permissionAsk is one missing grant and the two ways of answering it.
type permissionAsk struct {
	title  string
	body   string
	pane   string
	prompt func()

	mu          sync.Mutex
	prompted    bool
	dialogShown bool
}

// demand answers one refusal. A grant is asked for again and again while it is
// missing, so the dialog is offered once per run: a modal on every press is
// noise rather than help.
func (p *permissionAsk) demand(s *Service) {
	prompt, dialog := p.step()
	switch {
	case prompt:
		p.prompt()
	case dialog:
		go s.offerPermission(l10n.T(p.title), l10n.T(p.body), p.pane)
	}
}

// step is the whole policy, kept apart from the acting so it can be read and
// tested on its own. It is atomic because two presses in a row must not stack
// two system prompts.
func (p *permissionAsk) step() (prompt, dialog bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.prompted {
		p.prompted = true
		return true, false
	}
	if p.dialogShown {
		return false, false
	}
	p.dialogShown = true
	return false, true
}

var (
	askScreen = permissionAsk{
		title: "屏幕录制权限",
		// A rebuild changes the signature, and a checkbox that still reads
		// "on" then refuses the app it names — so the switch has to be moved,
		// not just confirmed.
		body:   "Transdict 读不到其他窗口的画面，截屏只会得到桌面。请在 系统设置 → 隐私与安全性 → 录屏与系统录音 中打开 Transdict 的开关；如果开关已经打开却仍然无效，请用列表左下角的「−」移除 Transdict，再点「+」重新添加。改完重启一次 Transdict 才会生效。",
		pane:   screenPane,
		prompt: func() { capture.RequestScreenAccess() },
	}
	askTrust = permissionAsk{
		title:  "辅助功能权限",
		body:   "Transdict 读不到其他应用里选中的文字。请在 系统设置 → 隐私与安全性 → 辅助功能 中勾选 Transdict；如果已经勾选，请关掉再打开一次。改完重启一次 Transdict 才会生效。",
		pane:   accessibilityPane,
		prompt: func() { selection.RequestTrust() },
	}
)
