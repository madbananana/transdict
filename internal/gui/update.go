// The updater is one question asked of somebody else's server, so every way of
// asking it — the settings page, the menu bar, the quiet check at launch — comes
// through this file.
package gui

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"transdict/internal/l10n"
	"transdict/internal/update"
)

// checkBudget bounds one question. A release host is not a translation service,
// so 服务超时 must not be able to hold a launch open for minutes.
const checkBudget = 15 * time.Second

// downloadBudget is the other half: an installer is tens of megabytes and the
// whole transfer is bounded here rather than by the per-service timeout.
const downloadBudget = 30 * time.Minute

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

// UpdateProgress is one download step. Percent is zero when the server did not
// advertise a length.
type UpdateProgress struct {
	Percent  int   `json:"percent"`
	Received int64 `json:"received"`
	Total    int64 `json:"total"`
}

// updateSource resolves 更新源地址: an empty field means the official release
// feed, because an app that ships with a release pipeline can name its own
// feed. A custom address still wins, which is what a fork needs.
func updateSource(raw string) string {
	if source := strings.TrimSpace(raw); source != "" {
		return source
	}
	return update.DefaultSource
}

// CheckUpdate is the button on the settings page. It answers whatever the source
// says, including 没有更新.
func (s *Service) CheckUpdate() (UpdateReport, error) {
	cfg := s.cfg.Get()
	return s.updateReport(updateSource(cfg.UpdateURL), cfg.IncludeBeta)
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

// InstallUpdate is 下载并安装 on the settings page. It fetches the release
// again, downloads the one asset this OS installs, and hands it to the system.
// On Windows the installer replaces the running executable, so the call answers
// first and the app quits a moment later; on macOS and Linux the package opens
// beside the running app.
func (s *Service) InstallUpdate() (string, error) {
	cfg := s.cfg.Get()
	rel, err := s.latest(updateSource(cfg.UpdateURL), cfg.IncludeBeta, checkBudget)
	if err != nil {
		return "", err
	}
	if rel == nil {
		return "", errors.New("已是最新版本")
	}
	asset := update.ChooseAsset(rel, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return "", errors.New("发布中没有适用于当前系统的安装包")
	}
	dir, err := update.SaveDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, filepath.Base(asset.Name))

	ctx, cancel := context.WithTimeout(context.Background(), downloadBudget)
	defer cancel()
	err = update.Download(ctx, *asset, path, func(done, total int64) {
		progress := UpdateProgress{Received: done, Total: total}
		if total > 0 {
			progress.Percent = int(done * 100 / total)
		}
		s.wails.Event.Emit(EventUpdateProgress, progress)
	})
	if err != nil {
		return "", err
	}
	quit, err := update.Launch(path)
	if err != nil {
		return "", err
	}
	if quit {
		// The JS call has to answer before the window disappears; three seconds
		// is enough for the status line to paint and for the installer to show
		// its first page.
		go func() {
			time.Sleep(3 * time.Second)
			s.wails.Quit()
		}()
	}
	return l10n.T(update.LaunchMessage()), nil
}

// OpenUpdatePage opens the release page of the newest published build. It goes
// through the same source as 检查更新 so a custom feed keeps working.
func (s *Service) OpenUpdatePage() error {
	cfg := s.cfg.Get()
	rel, err := s.latest(updateSource(cfg.UpdateURL), cfg.IncludeBeta, checkBudget)
	if err != nil {
		return err
	}
	if rel == nil || rel.URL == "" {
		return errors.New("没有可用的发布页")
	}
	return s.wails.Browser.OpenURL(rel.URL)
}

func (s *Service) updateReport(source string, includeBeta bool) (UpdateReport, error) {
	rel, err := s.latest(source, includeBeta, checkBudget)
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

func (s *Service) latest(source string, includeBeta bool, budget time.Duration) (*update.Release, error) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return update.Check(ctx, source, s.version, includeBeta)
}

func (s *Service) checkUpdate(manual bool) {
	title := l10n.T("检查更新")
	cfg := s.cfg.Get()
	report, err := s.updateReport(updateSource(cfg.UpdateURL), cfg.IncludeBeta)
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
