// The updater is one question asked of somebody else's server, so every way of
// asking it — the settings page, the menu bar, the quiet check at launch — comes
// through this file.
package gui

import (
	"context"
	"time"

	"transdict/internal/l10n"
	"transdict/internal/update"
)

// checkBudget bounds one question. A release host is not a translation service,
// so 服务超时 must not be able to hold a launch open for minutes.
const checkBudget = 15 * time.Second

// notesLimit is what an alert can show before the panel stops being readable.
const notesLimit = 400

// UpdateReport answers 检查更新: the running build, and what to install when the
// source is ahead of it. An empty Version means the build is current, which is a
// result rather than a failure.
type UpdateReport struct {
	Current string `json:"current"`
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

func (r UpdateReport) Newer() bool { return r.Version != "" }

// CheckUpdate is the button on the settings page. It answers whatever the source
// says, including 没有更新.
func (s *Service) CheckUpdate() (UpdateReport, error) {
	return s.updateReport(s.cfg.Get().UpdateURL)
}

// CheckUpdateNow is the menu bar item. A manual check that stays silent is
// indistinguishable from a broken one, so it reports even when nothing changed.
//
//wails:ignore
func (s *Service) CheckUpdateNow() {
	s.checkUpdate(true)
}

// AutoCheckOnLaunch asks once and only speaks when there is something to say.
//
//wails:ignore
func (s *Service) AutoCheckOnLaunch() {
	s.checkUpdate(false)
}

func (s *Service) updateReport(source string) (UpdateReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkBudget)
	defer cancel()
	rel, err := update.Check(ctx, source, s.version)
	if err != nil {
		return UpdateReport{}, err
	}
	report := UpdateReport{Current: s.version}
	if rel != nil {
		report.Version = rel.Version
		report.URL = rel.URL
		report.Notes = shorten(rel.Notes)
	}
	return report, nil
}

func (s *Service) checkUpdate(manual bool) {
	title := l10n.T("检查更新")
	cfg := s.cfg.Get()
	if cfg.UpdateURL == "" {
		if manual {
			s.inform(title, l10n.T("还没有设置更新源，请在「通用」里填写发布文件地址。"))
		}
		return
	}
	report, err := s.updateReport(cfg.UpdateURL)
	if err != nil {
		// A silent failure is right at launch and wrong when the user asked.
		if manual {
			s.inform(title, err.Error())
		}
		return
	}
	if !report.Newer() {
		if manual {
			s.inform(title, l10n.T("已是最新版本")+"："+report.Current)
		}
		return
	}
	body := l10n.T("发现新版本") + " " + report.Version + "（" + l10n.T("当前") + " " + report.Current + "）"
	if report.Notes != "" {
		body += "\n\n" + report.Notes
	}
	s.ask(title, body, report.URL)
}

// inform shows one panel with one way to close it.
func (s *Service) inform(title, body string) {
	dialog := s.wails.Dialog.Info().SetTitle(title).SetMessage(body)
	dialog.AddButton(l10n.T("好"))
	dialog.Show()
}

// ask offers the release page when there is one to offer. The link comes from
// the manifest, so nothing here has to know where the builds live.
func (s *Service) ask(title, body, link string) {
	dialog := s.wails.Dialog.Info().SetTitle(title).SetMessage(body)
	if link != "" {
		dialog.AddButton(l10n.T("打开发布页")).OnClick(func() {
			_ = s.wails.Browser.OpenURL(link)
		})
	}
	dialog.AddButton(l10n.T("好"))
	dialog.Show()
}

// shorten keeps a changelog inside an alert. Release notes written by a human
// fit; a pasted commit log does not, and the panel is not where it gets read.
func shorten(text string) string {
	r := []rune(text)
	if len(r) <= notesLimit {
		return text
	}
	return string(r[:notesLimit]) + "…"
}
