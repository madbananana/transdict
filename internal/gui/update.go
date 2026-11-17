// The updater is one question asked of somebody else's server, so every way of
// asking it — the settings page, the menu bar, the quiet check at launch — comes
// through this file.
package gui

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"transdict/internal/l10n"
	"transdict/internal/settings"
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

// AutoCheck is the quiet check at launch and on the periodic tick. It reads the
// setting now rather than when the timer was installed, so switching automatic
// checks off stops both without a relaunch.
//
//wails:ignore
func (s *Service) AutoCheck() {
	if !s.cfg.Get().AutoUpdate {
		return
	}
	s.checkUpdate(false)
}

// InstallUpdate is 下载并安装 on the settings page. It fetches the release
// again, downloads the one asset this OS installs, and hands it over. Windows
// replaces the executable in place and macOS swaps the app bundle, so both
// answer first and quit a moment later; Linux gives the package to the desktop.
func (s *Service) InstallUpdate() (string, error) {
	s.updateMu.Lock()
	if s.updating {
		s.updateMu.Unlock()
		return "", errors.New("更新正在下载，请稍候")
	}
	s.updating = true
	s.updateMu.Unlock()
	defer func() {
		s.updateMu.Lock()
		s.updating = false
		s.updateMu.Unlock()
	}()

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
	quit, message, err := update.Launch(path)
	if err != nil {
		return "", err
	}
	// The installed build is now at least as new as the version that was
	// skipped, so that note in the settings page is stale.
	s.clearSkipped()
	if quit {
		// The JS call has to answer before the window disappears; three seconds
		// is enough for the status line to paint and for the installer to show
		// its first page.
		go func() {
			time.Sleep(3 * time.Second)
			s.wails.Quit()
		}()
	}
	return l10n.T(message), nil
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
		s.clearSkipped()
		if manual {
			s.inform(title, l10n.T("已是最新版本")+"："+report.Current)
		}
		return
	}
	// 忽略此版本 only quiets the automatic check; a manual one still reports it
	// so the user can change their mind.
	if !manual && report.Version == cfg.SkippedUpdate {
		return
	}
	s.promptUpdate(report)
}

// promptUpdate is the three-way answer to a discovered release: install now,
// later, or never for this version. The skip is stored by version, so the next
// release asks again.
func (s *Service) promptUpdate(report UpdateReport) {
	title := l10n.T("发现新版本")
	body := l10n.T("发现新版本") + " " + report.Version + "（" + l10n.T("当前") + " " + report.Current + "）"
	if report.Notes != "" {
		body += "\n\n" + report.Notes
	}
	dialog := s.wails.Dialog.Info().SetTitle(title).SetMessage(body)
	dialog.AddButton(l10n.T("立即更新")).OnClick(func() { go s.installFromPrompt() })
	dialog.AddButton(l10n.T("稍后"))
	dialog.AddButton(l10n.T("忽略此版本")).OnClick(func() { s.skipVersion(report.Version) })
	dialog.Show()
}

// installFromPrompt runs the same download the settings page starts. The
// progress events only reach a window that is listening, so a failure has to be
// reported here; success on Windows and macOS ends with the app restarting.
func (s *Service) installFromPrompt() {
	if _, err := s.InstallUpdate(); err != nil {
		s.inform(l10n.T("更新失败"), err.Error())
	}
}

func (s *Service) skipVersion(version string) {
	if _, err := s.cfg.Update(func(c *settings.Config) { c.SkippedUpdate = version }); err != nil {
		log.Printf("transdict: 记录忽略的更新版本: %v", err)
	}
}

func (s *Service) clearSkipped() {
	if s.cfg.Get().SkippedUpdate == "" {
		return
	}
	if _, err := s.cfg.Update(func(c *settings.Config) { c.SkippedUpdate = "" }); err != nil {
		log.Printf("transdict: 清除忽略的更新版本: %v", err)
	}
}

// inform shows one panel with one way to close it.
func (s *Service) inform(title, body string) {
	dialog := s.wails.Dialog.Info().SetTitle(title).SetMessage(body)
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
