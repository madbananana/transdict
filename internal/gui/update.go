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

// notesLimit is what the update window will carry. A release body written by a
// human fits; a pasted commit log of a thousand commits is not a changelog and
// is not worth sending through an event on every progress tick.
const notesLimit = 20000

// The update window's states. It draws a progress bar while downloading, a
// finished bar while the installer runs, and the retry button after an error.
const (
	updateIdle        = "idle"
	updateDownloading = "downloading"
	updateInstalling  = "installing"
	updateError       = "error"
)

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

// UpdateInfo is the whole state of the update window, emitted on every change
// and polled once when the window mounts. The window never has to join two
// events to know whether a download is still running.
type UpdateInfo struct {
	Current  string `json:"current"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	Notes    string `json:"notes"`
	State    string `json:"state"`
	Percent  int    `json:"percent"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Message  string `json:"message"`
}

// updateNotice is the release rememberRelease keeps for the window. The whole
// release travels with it because the download needs its asset list, not just
// the version and notes the window draws.
type updateNotice struct {
	version string
	url     string
	notes   string
	release *update.Release
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
// says, including 没有更新, and remembers a newer release for the update window.
func (s *Service) CheckUpdate() (UpdateReport, error) {
	cfg := s.cfg.Get()
	report, rel, err := s.fetchReport(updateSource(cfg.UpdateURL), cfg.IncludeBeta)
	if err != nil {
		return UpdateReport{}, err
	}
	if rel != nil {
		s.noteRelease(rel)
	} else {
		s.forgetRelease()
	}
	return report, nil
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

// UpdateInfo answers the update window. It is also emitted as
// EventUpdateChanged after every change.
func (s *Service) UpdateInfo() UpdateInfo {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateInfoLocked()
}

// OpenUpdateWindow raises the window that shows the remembered release. The
// settings page calls it after its own check found something newer.
func (s *Service) OpenUpdateWindow() {
	s.updateMu.Lock()
	known := s.notice != nil
	s.updateMu.Unlock()
	if !known {
		return
	}
	s.show(WindowUpdate, true)
}

// HideUpdate is 稍后 and the window's close button: the notice survives, so the
// next periodic check can raise it again without another download.
func (s *Service) HideUpdate() {
	if w := s.window(WindowUpdate); w != nil {
		w.Hide()
	}
}

// SkipUpdate is 忽略此版本: the version is written where the automatic check
// reads it, and the window goes away. A manual check still reports it, so the
// choice can be changed.
func (s *Service) SkipUpdate() {
	s.updateMu.Lock()
	version := ""
	if s.notice != nil {
		version = s.notice.version
	}
	s.updateMu.Unlock()
	if version != "" {
		s.skipVersion(version)
	}
	s.HideUpdate()
}

// CancelUpdate stops a download in progress. The partial file is removed by
// Download; the window returns to its 立即更新 state with a short notice.
func (s *Service) CancelUpdate() {
	s.updateMu.Lock()
	cancel := s.cancelUpdate
	s.updateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StartUpdate downloads and installs the remembered release, or fetches the
// newest one first when the window was opened without a check. It answers as
// soon as the download starts; the window follows EventUpdateChanged.
func (s *Service) StartUpdate() error {
	s.updateMu.Lock()
	if s.updating {
		s.updateMu.Unlock()
		return errors.New("更新正在下载，请稍候")
	}
	notice := s.notice
	s.updateMu.Unlock()

	if notice == nil {
		cfg := s.cfg.Get()
		rel, err := s.latest(updateSource(cfg.UpdateURL), cfg.IncludeBeta, checkBudget)
		if err != nil {
			return err
		}
		if rel == nil {
			return errors.New("已是最新版本")
		}
		s.noteRelease(rel)
		s.updateMu.Lock()
		notice = s.notice
		s.updateMu.Unlock()
	}
	if notice == nil {
		return errors.New("没有可安装的版本")
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadBudget)
	s.updateMu.Lock()
	if s.updating {
		s.updateMu.Unlock()
		cancel()
		return errors.New("更新正在下载，请稍候")
	}
	s.updating = true
	s.updateState = updateDownloading
	s.updateErr = ""
	s.updateDone, s.updateTotal = 0, 0
	s.cancelUpdate = cancel
	s.updateMu.Unlock()

	s.emitUpdate()
	s.show(WindowUpdate, true)
	go s.downloadAndInstall(ctx, cancel, notice)
	return nil
}

// downloadAndInstall waits for nothing but the bytes: the installer cannot
// replace an executable that is still running, so update.Launch hands the file
// to a helper that waits for this process to exit.
func (s *Service) downloadAndInstall(ctx context.Context, cancel context.CancelFunc, notice *updateNotice) {
	defer cancel()
	defer func() {
		s.updateMu.Lock()
		s.updating = false
		s.cancelUpdate = nil
		s.updateMu.Unlock()
	}()

	asset := update.ChooseAsset(notice.release, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		log.Printf("transdict: 发布 %s 中没有 %s/%s 的安装包", notice.version, runtime.GOOS, runtime.GOARCH)
		s.setUpdateState(updateError, l10n.T("发布中没有适用于当前系统的安装包"))
		return
	}
	dir, err := update.SaveDir()
	if err != nil {
		s.setUpdateState(updateError, err.Error())
		return
	}
	path := filepath.Join(dir, filepath.Base(asset.Name))
	log.Printf("transdict: 下载 %s 到 %s", notice.version, path)

	err = update.Download(ctx, *asset, path, func(done, total int64) {
		s.updateMu.Lock()
		s.updateDone, s.updateTotal = done, total
		s.updateMu.Unlock()
		s.emitUpdate()
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("transdict: 更新下载已取消")
			s.setUpdateState(updateIdle, l10n.T("已取消下载"))
			return
		}
		log.Printf("transdict: 下载更新失败: %v", err)
		s.setUpdateState(updateError, err.Error())
		return
	}

	s.setUpdateState(updateInstalling, "")
	quit, message, err := update.Launch(path)
	if err != nil {
		log.Printf("transdict: 启动更新程序失败: %v", err)
		s.setUpdateState(updateError, err.Error())
		return
	}
	log.Printf("transdict: 更新程序已启动（%s）", message)
	// The installed build is now at least as new as the version that was
	// skipped, so that note in the settings page is stale.
	s.clearSkipped()
	if !quit {
		// Linux hands the package to the desktop and macOS falls back to the
		// mounted image when it cannot swap the bundle; both stay open, so the
		// window has to say what happened instead of waiting forever.
		s.setUpdateState(updateIdle, l10n.T(message))
		return
	}
	// The JS call has to answer before the window disappears; three seconds
	// is enough for the status line to paint and for the installer to show
	// its first page.
	go func() {
		time.Sleep(3 * time.Second)
		s.wails.Quit()
	}()
}

// OpenUpdatePage opens the release page of the newest published build. It goes
// through the same source as 检查更新 so a custom feed keeps working, and uses
// the remembered release when there is one.
func (s *Service) OpenUpdatePage() error {
	s.updateMu.Lock()
	if s.notice != nil && s.notice.url != "" {
		url := s.notice.url
		s.updateMu.Unlock()
		return s.wails.Browser.OpenURL(url)
	}
	s.updateMu.Unlock()

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

// fetchReport is the one network answer each check turns into a report and,
// when there is something newer, a remembered release.
func (s *Service) fetchReport(source string, includeBeta bool) (UpdateReport, *update.Release, error) {
	rel, err := s.latest(source, includeBeta, checkBudget)
	if err != nil {
		return UpdateReport{}, nil, err
	}
	report := UpdateReport{Current: s.version}
	if rel == nil {
		return report, nil, nil
	}
	return UpdateReport{
		Current: s.version,
		Version: rel.Version,
		URL:     rel.URL,
		Notes:   clipNotes(rel.Notes),
	}, rel, nil
}

func (s *Service) latest(source string, includeBeta bool, budget time.Duration) (*update.Release, error) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return update.Check(ctx, source, s.version, includeBeta)
}

// noteRelease makes rel the release the update window draws. An in-flight
// download keeps its own state: a periodic check must not reset the bar it is
// filling.
func (s *Service) noteRelease(rel *update.Release) {
	s.updateMu.Lock()
	if s.updating {
		s.updateMu.Unlock()
		return
	}
	s.notice = &updateNotice{
		version: rel.Version,
		url:     rel.URL,
		notes:   clipNotes(rel.Notes),
		release: rel,
	}
	s.updateState = updateIdle
	s.updateErr = ""
	s.updateDone, s.updateTotal = 0, 0
	s.updateMu.Unlock()
	s.emitUpdate()
}

// forgetRelease drops a notice the source no longer reports, so a stale window
// cannot offer a version that has been withdrawn.
func (s *Service) forgetRelease() {
	s.updateMu.Lock()
	if s.updating || s.notice == nil {
		s.updateMu.Unlock()
		return
	}
	s.notice = nil
	s.updateState = updateIdle
	s.updateErr = ""
	s.updateDone, s.updateTotal = 0, 0
	s.updateMu.Unlock()
	s.emitUpdate()
}

func (s *Service) updateInfoLocked() UpdateInfo {
	info := UpdateInfo{
		Current:  s.version,
		State:    s.updateState,
		Message:  s.updateErr,
		Received: s.updateDone,
		Total:    s.updateTotal,
	}
	if info.State == "" {
		info.State = updateIdle
	}
	if s.notice != nil {
		info.Version = s.notice.version
		info.URL = s.notice.url
		info.Notes = s.notice.notes
	}
	if info.Total > 0 {
		info.Percent = int(info.Received * 100 / info.Total)
		if info.Percent > 100 {
			info.Percent = 100
		}
	}
	return info
}

func (s *Service) setUpdateState(state, message string) {
	s.updateMu.Lock()
	s.updateState = state
	s.updateErr = message
	s.updateMu.Unlock()
	s.emitUpdate()
}

// emitUpdate publishes a snapshot outside the lock: UpdateInfo takes it again,
// and an event handler that calls back into the service must not deadlock.
func (s *Service) emitUpdate() {
	s.wails.Event.Emit(EventUpdateChanged, s.UpdateInfo())
}

func (s *Service) checkUpdate(manual bool) {
	title := l10n.T("检查更新")
	cfg := s.cfg.Get()
	report, rel, err := s.fetchReport(updateSource(cfg.UpdateURL), cfg.IncludeBeta)
	if err != nil {
		// A silent failure is right at launch and wrong when the user asked.
		log.Printf("transdict: 检查更新失败: %v", err)
		if manual {
			s.inform(title, err.Error())
		}
		return
	}
	if rel == nil {
		s.forgetRelease()
		s.clearSkipped()
		if manual {
			s.inform(title, l10n.T("已是最新版本")+"："+report.Current)
		}
		return
	}
	// 忽略此版本 only quiets the automatic check; a manual one still reports it
	// so the user can change their mind.
	if !manual && report.Version == cfg.SkippedUpdate {
		log.Printf("transdict: 自动更新跳过已忽略的版本 %s", report.Version)
		return
	}
	log.Printf("transdict: 发现新版本 %s（当前 %s，手动 %v）", report.Version, report.Current, manual)
	s.noteRelease(rel)
	s.show(WindowUpdate, true)
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

// clipNotes keeps a release body inside the window. The whole body scrolls
// there, so the cut is only a guard against a pathological payload.
func clipNotes(text string) string {
	r := []rune(text)
	if len(r) <= notesLimit {
		return text
	}
	return string(r[:notesLimit]) + "…"
}
