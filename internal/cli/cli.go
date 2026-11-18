// Package cli assembles the application: it wires the subsystems together and
// hands the result to Wails. Everything below main.go is deliberately here so
// the binary entry point stays three lines.
package cli

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"transdict/internal/autostart"
	"transdict/internal/capture"
	"transdict/internal/dock"
	"transdict/internal/gui"
	"transdict/internal/history"
	"transdict/internal/instance"
	"transdict/internal/l10n"
	"transdict/internal/logging"
	"transdict/internal/netp"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/selection"
	"transdict/internal/settings"
	"transdict/internal/shortcut"
)

// autoUpdateEvery is how often a running tray app asks again. Six hours is
// frequent enough to catch a release the same day without turning the release
// feed into a heartbeat.
const autoUpdateEvery = 6 * time.Hour

// Main assembles and runs the app. Embedded assets arrive from package main
// because //go:embed paths are relative to the declaring package.
//
// trayIcon is an optional monochrome glyph: macOS menu bars want a template
// image the system tints for light and dark, not the coloured app icon. The
// square variant is the same mark framed by 菜单栏图标. The Windows tray has its
// own icon: the notification area asks for 16-24px, too small for the app
// icon's two glyphs.
func Main(version string, assets embed.FS, appIcon, trayIcon, squareTrayIcon, windowsTrayIcon []byte) {
	if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println("transdict " + version)
		return
	}

	// Taking the lock before anything reads or writes the config is the point:
	// two copies editing the same file is how settings start disagreeing with
	// what is on screen, and both would claim the same global shortcuts.
	single, err := instance.Acquire()
	if err != nil {
		if errors.Is(err, instance.ErrBusy) {
			if nerr := instance.Notify(wakeCommand(os.Args)); nerr != nil {
				log.Printf("transdict: %v，且无法唤醒正在运行的实例: %v", err, nerr)
			}
			return
		}
		// A lock we could not read is not a reason to refuse to start.
		log.Printf("transdict: 单实例检查不可用: %v", err)
	}
	if single != nil {
		defer single.Close()
	}

	// 日志 belongs to the one process that owns the app: a wake-up launch exits
	// right after Notify, and letting it rotate the file would move the log out
	// from under the instance still writing to it.
	if _, err := logging.Setup(); err != nil {
		log.Printf("transdict: 日志文件不可用: %v", err)
	} else {
		log.Printf("transdict %s starting", version)
	}

	cfg, err := settings.NewStore()
	if err != nil {
		log.Fatalf("transdict: settings: %v", err)
	}
	// A config outlives the services it names: it moves between platforms and
	// survives a build that dropped a kind. A row nothing can run is worse than
	// no row — 服务设置 would offer a switch that never turns on — so the dead
	// ones are removed at startup. The snapshot only answers whether the file
	// needs rewriting, which a first run on a healthy config should not do.
	if snap := cfg.Get(); query.Prune(&snap) > 0 {
		if _, err := cfg.Update(func(c *settings.Config) { query.Prune(c) }); err != nil {
			log.Printf("transdict: 无法清理本机不支持的服务: %v", err)
		}
	}
	shots := capture.NewStore()
	hist, err := history.NewStore()
	if err != nil {
		log.Printf("transdict: 历史不可用: %v", err)
	}

	// A login item is the operating system's fact, not ours. Adopting what it
	// reports keeps 通用 honest after a change made in 系统设置 or the registry.
	if next, changed := autostart.Reconcile(cfg.Get().LoginItem, autostart.Status()); changed {
		if _, err := cfg.Update(func(c *settings.Config) { c.LoginItem = next }); err != nil {
			log.Printf("transdict: 开机自启状态写入失败: %v", err)
		}
	}

	svc := gui.New(cfg, shots, hist, version)
	settingsSvc := gui.NewSettings(cfg, version)

	application.RegisterEvent[query.Response](gui.EventResult)
	application.RegisterEvent[gui.Busy](gui.EventBusy)
	application.RegisterEvent[string](gui.EventCommand)
	application.RegisterEvent[string](gui.EventUILang)
	application.RegisterEvent[string](gui.EventTheme)

	// The interface language is read before the app exists because Wails takes the
	// about-panel wording from Options at construction. Nothing is registered to
	// OnChange yet, so this first Set cannot redraw anything that is not there.
	l10n.Set(cfg.Get().UILang)

	opts := application.Options{
		Name:        "Transdict",
		Description: l10n.T("跨平台词典与翻译工具"),
		Services: []application.Service{
			application.NewService(svc),
			application.NewService(settingsSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			// Wails applies this when the app finishes launching, so the stored
			// choice — not a later toggle — decides the first frame.
			ActivationPolicy: activationPolicy(cfg.Get().ShowInDock),
		},
	}
	// The packaged Windows build takes its icon from the resource compiler; a
	// dev run has no .syso, so this keeps the taskbar button branded there. It
	// is Windows-only because macOS takes the bundle icon.
	if runtime.GOOS == "windows" {
		opts.Icon = appIcon
	}
	app := application.New(opts)

	svc.Attach(app)

	// transdict:// links are how another program — Alfred, Raycast, a shell
	// script — hands the app a sentence. Wails reports the URL as an
	// application event on every platform; answering it in the floating panel
	// keeps the main window out of the way.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		if raw := e.Context().URL(); raw != "" {
			if err := svc.OpenLink(raw); err != nil {
				log.Printf("transdict: 链接 %s: %v", raw, err)
			}
		}
	})

	// EasyDict's menu bar is 编辑 / 显示 / 快捷键 / 窗口 / 帮助: there is no File
	// menu, Close lives in 窗口, and 快捷键 sits before 窗口 (MainMenuShortcutCommand).
	// The standard menus stay role-based — the roles are what wire AppKit's
	// selectors and the window list — so they are built here and translated
	// below rather than through DefaultApplicationMenu.
	appMenu := application.NewMenu()
	appMenu.AddRole(application.AppMenu)
	appMenu.AddRole(application.EditMenu)
	appMenu.AddRole(application.ViewMenu)
	// A language switch retitles the existing items and reattaches the same
	// menu, so the role-based structure AppKit keeps (the window list, the
	// Services submenu) survives. Each label remembers its source text.
	menuLabels := map[*application.MenuItem]string{}
	setLabel := func(item *application.MenuItem, text string) {
		if item == nil {
			return
		}
		item.SetLabel(l10n.T(text))
		menuLabels[item] = text
	}
	addItem := func(menu *application.Menu, text string) *application.MenuItem {
		item := menu.Add(l10n.T(text))
		menuLabels[item] = text
		return item
	}
	shortcuts := appMenu.AddSubmenu(l10n.T("快捷键"))
	// AddSubmenu returns the submenu, but the menu bar draws the item that
	// owns it; retitling the submenu object alone leaves "Shortcuts" behind.
	setLabel(appMenu.FindByLabel(l10n.T("快捷键")), "快捷键")
	appShortcutItems := map[string]*application.MenuItem{}
	shortcut := func(text, action string) {
		item := addItem(shortcuts, text)
		item.SetAccelerator(appAccel(cfg.Get(), action))
		item.OnClick(func(*application.Context) { svc.RunMenuAction(action) })
		appShortcutItems[action] = item
	}
	shortcut("清空查询内容", gui.ActClearInput)
	shortcut("清空所有", gui.ActClearAll)
	shortcut("复制文本", gui.ActCopy)
	shortcut("复制第一个翻译结果", gui.ActCopyFirst)
	shortcut("聚焦", gui.ActFocus)
	shortcut("播放单词发音", gui.ActPlay)
	shortcut("重试", gui.ActRetry)
	shortcut("交换翻译语言", gui.ActToggle)
	// EasyDict groups the window keys apart from the page keys.
	shortcuts.AddSeparator()
	shortcut("钉住窗口", gui.ActPin)
	shortcut("隐藏", gui.ActHide)
	shortcut("增大字体", gui.ActFontIncrease)
	shortcut("减小字体", gui.ActFontDecrease)
	shortcuts.AddSeparator()
	shortcut("通过 Google 搜索", gui.ActGoogle)
	shortcut("在欧路词典中打开", gui.ActEudic)
	shortcut("在苹果词典中打开", gui.ActAppleDic)
	appMenu.AddRole(application.WindowMenu)
	appMenu.AddRole(application.HelpMenu)
	app.Menu.SetApplicationMenu(appMenu)

	// 快捷键 in 快捷键 is an app-internal table now, so a rebind in the settings
	// page has to reach the menu items already on screen. The items themselves
	// stay: only their key equivalents change.
	boundApp := cfg.Get().AppShortcuts
	cfg.Watch(func(next settings.Config) {
		if sameAppShortcuts(boundApp, next.AppShortcuts) {
			return
		}
		boundApp = next.AppShortcuts
		application.InvokeAsync(func() {
			for action, item := range appShortcutItems {
				item.SetAccelerator(appAccel(next, action))
			}
		})
	})

	// macOS keeps Preferences in the application menu, and that is where people
	// look for it (⌘,). Wails' default app menu has About and Quit but no
	// Preferences, so it is rebuilt with 设置… between them. About opens the
	// app's own 关于 page and 检查更新 sits beside it, as EasyDict's menu does.
	if top := appMenu.ItemAt(0); top != nil && top.GetSubmenu() != nil {
		sub := top.GetSubmenu()
		sub.Clear()
		about := addItem(sub, "关于 Transdict")
		about.OnClick(func(*application.Context) { svc.OpenAbout() })
		updates := addItem(sub, "检查更新")
		updates.OnClick(func(*application.Context) { go svc.CheckUpdateNow() })
		sub.AddSeparator()
		prefs := addItem(sub, "设置…")
		prefs.SetAccelerator("cmd+,")
		prefs.OnClick(func(*application.Context) { svc.OpenSettings() })
		sub.AddSeparator()
		sub.AddRole(application.ServicesMenu)
		sub.AddSeparator()
		sub.AddRole(application.Hide)
		sub.AddRole(application.HideOthers)
		sub.AddRole(application.UnHide)
		sub.AddSeparator()
		sub.AddRole(application.Quit)
		// The roles arrive with English titles; the rest of the app is in the
		// user's language, and the app menu is the first thing a macOS user
		// reads.
		for english, label := range map[string]string{
			"Services":       "服务",
			"Hide Transdict": "隐藏 Transdict",
			"Hide Others":    "隐藏其他",
			"Show All":       "全部显示",
			"Quit Transdict": "退出 Transdict",
		} {
			setLabel(sub.FindByLabel(english), label)
		}
	}

	// The standard menus are role-based but titled in English. EasyDict's bar
	// reads 编辑 / 显示 / 窗口 / 帮助 in a Chinese UI, so each title and the
	// title-bearing items Wails inserts are translated in place; the entries
	// AppKit injects itself (AutoFill, dictation, window tiling) follow the
	// bundle localizations declared in Info.plist.
	if top := appMenu.FindByLabel("Edit"); top != nil {
		setLabel(top, "编辑")
		if sub := top.GetSubmenu(); sub != nil {
			sub.Clear()
			// EasyDict's Edit menu has no Paste and Match Style and no Speech
			// submenu: it is the stock text-editing set.
			add := func(role application.Role, english, label string) {
				sub.AddRole(role)
				setLabel(sub.FindByLabel(english), label)
			}
			add(application.Undo, "Undo", "撤销")
			add(application.Redo, "Redo", "重做")
			sub.AddSeparator()
			add(application.Cut, "Cut", "剪切")
			add(application.Copy, "Copy", "拷贝")
			add(application.Paste, "Paste", "粘贴")
			add(application.Delete, "Delete", "删除")
			add(application.SelectAll, "Select All", "全选")
		}
	}

	// The stock View menu is a browser's: Reload takes ⌘R before the webview
	// can see it, which would shadow the window's own 重试, and its zoom would
	// fight the Aa sizes. A translation window only needs full screen.
	if top := appMenu.FindByLabel("View"); top != nil {
		setLabel(top, "显示")
		if sub := top.GetSubmenu(); sub != nil {
			sub.Clear()
			sub.AddRole(application.ToggleFullscreen)
			setLabel(sub.ItemAt(0), "进入全屏幕")
		}
	}

	if top := appMenu.FindByLabel("Window"); top != nil {
		setLabel(top, "窗口")
		if sub := top.GetSubmenu(); sub != nil {
			// Close belongs to the window menu on macOS (EasyDict's 窗口 starts
			// with 关闭), and the role is what makes AppKit add 全部关闭 beside
			// it and keep the live window list at the bottom.
			sub.Clear()
			add := func(role application.Role, english, label string) {
				sub.AddRole(role)
				setLabel(sub.FindByLabel(english), label)
			}
			add(application.CloseWindow, "Close", "关闭")
			add(application.Minimise, "Minimize", "最小化")
			add(application.Zoom, "Zoom", "缩放")
			sub.AddSeparator()
			add(application.Front, "Bring All to Front", "前置全部窗口")
		}
	}

	// EasyDict's Help menu is one 反馈 entry: a translation app has no manual to
	// read, and the issue tracker is where a wrong answer is reported.
	if top := appMenu.FindByLabel("Help"); top != nil {
		setLabel(top, "帮助")
		if sub := top.GetSubmenu(); sub != nil {
			sub.Clear()
			feedback := addItem(sub, "反馈")
			feedback.OnClick(func(*application.Context) {
				if err := svc.OpenFeedbackPage(); err != nil {
					log.Printf("transdict: 反馈: %v", err)
				}
			})
		}
	}

	l10n.OnChange(func() {
		application.InvokeAsync(func() {
			for item, text := range menuLabels {
				item.SetLabel(l10n.T(text))
			}
			// AppKit takes a top-level menu's NSMenu title when the menu is
			// attached; retitling the items alone left 编辑/显示 above English
			// commands. Reattaching rebuilds the NSMenus from the model.
			app.Menu.SetApplicationMenu(appMenu)
		})
	})

	// The on-device recognizer loads its text model on the first request, which
	// costs ~12s on a cold macOS 26 process; without this the first 截图翻译 sits
	// there while every later one is instant. The delay leaves the launch alone
	// and the work is done on a throwaway image, never the screen.
	go func() {
		time.Sleep(2 * time.Second)
		ocr.Warm()
	}()

	// Re-launching while a copy is already up is how people try to bring the
	// window back, so the lock's request channel has to land in the same
	// places the tray menu offers.
	if single != nil {
		go func() {
			for cmd := range single.Commands() {
				switch cmd {
				case "input":
					svc.OpenInput()
				case "settings":
					svc.OpenSettings()
				case "history":
					svc.OpenHistory()
				case "clipboard":
					if err := svc.ClipboardQuery(); err != nil {
						log.Printf("transdict: 剪贴板: %v", err)
					}
				case "select":
					if err := svc.CatchSelection(); err != nil {
						log.Printf("transdict: 划词: %v", err)
					}
				case "odict":
					if err := svc.WordUnderCursor(); err != nil {
						log.Printf("transdict: 取词: %v", err)
					}
				case "replace":
					if err := svc.ReplaceSelection(); err != nil {
						log.Printf("transdict: 替换: %v", err)
					}
				case "polish":
					if err := svc.PolishReplaceSelection(); err != nil {
						log.Printf("transdict: 润色并替换: %v", err)
					}
				case "silent":
					if err := svc.StartSilent(); err != nil {
						log.Printf("transdict: 静音 OCR: %v", err)
					}
				case "ocr":
					if err := svc.ClipboardOCR(); err != nil {
						log.Printf("transdict: 剪贴板 OCR: %v", err)
					}
				case "ocrwindow":
					if err := svc.OpenLastOCR(); err != nil {
						log.Printf("transdict: OCR 窗口: %v", err)
					}
				case "logs":
					svc.OpenLogs()
				case "capture":
					if err := svc.Start(); err != nil {
						log.Printf("transdict: 截图: %v", err)
					}
				case "update":
					go svc.CheckUpdateNow()
				default:
					svc.OpenMain()
				}
			}
		}()
	}

	// Outbound routing and the wait each service gets are process-wide
	// decisions, so they are installed before any service is asked for anything
	// and reinstalled when the settings page changes them. A bad value cannot
	// reach here from that page, which validates; only a hand-edited config can
	// send it a proxy nobody typed into the field.
	outbound := cfg.Get()
	applyOutbound(outbound)
	cfg.Watch(func(next settings.Config) {
		if next.Proxy == outbound.Proxy && next.Timeout == outbound.Timeout {
			return
		}
		outbound = next
		applyOutbound(outbound)
	})

	// 启用强制取词, 强制取词类型 and 优先使用 AppleScript API are read inside the
	// selection layer itself, so they are installed next to the outbound switches
	// and refreshed whenever the settings page changes them.
	sel := cfg.Get()
	selection.Configure(sel.ForceCopy, sel.ForceCopyKind, sel.PreferAppleScriptAPI)
	cfg.Watch(func(next settings.Config) {
		if next.ForceCopy == sel.ForceCopy && next.ForceCopyKind == sel.ForceCopyKind &&
			next.PreferAppleScriptAPI == sel.PreferAppleScriptAPI {
			return
		}
		sel = next
		selection.Configure(sel.ForceCopy, sel.ForceCopyKind, sel.PreferAppleScriptAPI)
	})

	gui.NewMainWindow(app, cfg)
	bindShortcuts(app, cfg, svc)
	// Binding starts by unregistering every accelerator, so an unrelated edit —
	// a new target language, a pin, a theme — must not run it: the hotkeys would
	// blink out and back on every such change, and a failure in that gap would
	// leave them dead until the next settings write.
	boundShortcuts := cfg.Get().Shortcuts
	cfg.Watch(func(next settings.Config) {
		if sameShortcuts(boundShortcuts, next.Shortcuts) {
			return
		}
		boundShortcuts = next.Shortcuts
		bindShortcuts(app, cfg, svc)
	})

	// The tray is native chrome drawn once, so switching language means redrawing
	// the menu; the webview reloads its own tables instead.
	tray := newTray(app, cfg, svc, appIcon, trayIcon, squareTrayIcon, windowsTrayIcon)

	// 隐藏菜单栏图标 is chrome the user can turn off without a relaunch. The
	// status item can only be ordered out once the application loop is running,
	// so the stored choice is applied on ApplicationStarted — InvokeAsync here
	// would dispatch before there is a main thread and panic.
	hiddenTray := cfg.Get().HideMenuBarIcon
	if hiddenTray {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted,
			func(*application.ApplicationEvent) { tray.Hide() })
	}
	cfg.Watch(func(next settings.Config) {
		if next.HideMenuBarIcon == hiddenTray {
			return
		}
		hiddenTray = next.HideMenuBarIcon
		application.InvokeAsync(func() {
			if hiddenTray {
				tray.Hide()
			} else {
				tray.Show()
			}
		})
	})

	cfg.Watch(func(next settings.Config) {
		if l10n.Set(next.UILang) {
			app.Event.Emit(gui.EventUILang, next.UILang)
		}
	})

	// The theme is one attribute on the document element, so a window can follow
	// the change in place — unlike the language, whose tables are built when the
	// bundle loads and which therefore needs a reload.
	themed := cfg.Get().Theme
	cfg.Watch(func(next settings.Config) {
		if next.Theme == themed {
			return
		}
		themed = next.Theme
		app.Event.Emit(gui.EventTheme, themed)
	})

	// The Dock tile is a property of the running process, so unlike the taskbar
	// button it can follow the toggle without a relaunch. AppKit wants the main
	// thread.
	docked := cfg.Get().ShowInDock
	cfg.Watch(func(next settings.Config) {
		if next.ShowInDock == docked {
			return
		}
		docked = next.ShowInDock
		application.InvokeAsync(func() { dock.Set(docked) })
	})

	// The automatic check waits on somebody else's server, so it gets its own
	// goroutine: a release host that is down must not delay a window. The alert
	// it may show is native chrome, so the question has to wait for the loop —
	// asking before ApplicationStarted has no main thread to draw it on. A tray
	// app can stay open for weeks, so the same check repeats instead of trusting
	// that the next launch is soon.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted,
		func(*application.ApplicationEvent) {
			go svc.AutoCheck()
			go func() {
				ticker := time.NewTicker(autoUpdateEvery)
				defer ticker.Stop()
				for range ticker.C {
					svc.AutoCheck()
				}
			}()
		})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// applyOutbound moves the two network switches that belong to the process
// rather than to one window: where requests leave from, and how long a service
// may take answering. Providers construct their HTTP client per query and read
// the same numbers, so the next query after a save is enough — no restart.
func applyOutbound(cfg settings.Config) {
	if err := netp.Apply(cfg.Proxy); err != nil {
		log.Printf("transdict: %v", err)
	}
	netp.SetTimeout(settings.ClampTimeout(cfg.Timeout))
}

// activationPolicy is the macOS half of 在 Dock 中显示图标: an accessory app has
// no tile and no application menu, which is the default for a tool driven from
// the menu bar and global hotkeys.
// wakeCommand is what a second launch asks the running copy to show or do. A
// name we do not recognise falls back to the panel: Finder, `open -a` and macOS
// itself pass arguments the user never typed (-psn_…), and all of them mean
// "come back". Naming a window explicitly is how a script reaches it without a
// global hotkey; select, odict and capture are the same idea for the actions
// that otherwise only a shortcut or the tray can start.
func wakeCommand(args []string) string {
	if len(args) > 1 {
		switch args[1] {
		case "main", "input", "settings", "history", "clipboard", "select", "odict", "replace", "polish", "silent", "capture", "ocr", "ocrwindow", "logs", "update":
			return args[1]
		}
	}
	return "main"
}

func activationPolicy(showInDock bool) application.ActivationPolicy {
	if showInDock {
		return application.ActivationPolicyRegular
	}
	return application.ActivationPolicyAccessory
}

// toggleSelect flips 划词翻译 (automatic selection). The tray checkbox, its
// accelerator and the 通用 switch all read the same field, so one write keeps
// them in step.
func toggleSelect(cfg *settings.Store) {
	next, err := cfg.Update(func(c *settings.Config) { c.MouseSelect = !c.MouseSelect })
	if err != nil {
		log.Printf("transdict: 切换自动划词: %v", err)
		return
	}
	// A global shortcut can fire with nothing on screen, so the new state is
	// announced the way EasyDict announces it: 已开启划词翻译/已关闭划词翻译.
	if next.MouseSelect {
		gui.ShowToast(l10n.T("已开启划词翻译"))
	} else {
		gui.ShowToast(l10n.T("已关闭划词翻译"))
	}
}

func bindShortcuts(app *application.App, cfg *settings.Store, svc *gui.Service) {
	actions := shortcut.Actions{
		Capture:      func() { _ = svc.Start() },
		Input:        svc.OpenInput,
		Settings:     svc.OpenSettings,
		Clipboard:    func() { _ = svc.ClipboardQuery() },
		SilentOCR:    func() { _ = svc.StartSilent() },
		OCRShot:      func() { _ = svc.StartOCR() },
		OCRClipboard: func() { _ = svc.ClipboardOCR() },
		OCRWindow:    func() { _ = svc.OpenLastOCR() },
		Selection:    func() { _ = svc.CatchSelection() },
		ToggleSelect: func() { toggleSelect(cfg) },
		Replace:      func() { _ = svc.ReplaceSelection() },
		Polish:       func() { _ = svc.PolishReplaceSelection() },
		ODict:        func() { _ = svc.WordUnderCursor() },
	}
	problems := shortcut.Bind(app, cfg.Get(), actions)
	for _, p := range problems {
		if p.Accel == "" {
			log.Printf("transdict: 清除旧快捷键失败: %s", p.Err)
			continue
		}
		log.Printf("transdict: %s 注册失败: %s", shortcut.Display(p.Accel), p.Err)
	}
	// The refusal is kept for the 快捷键 page: at launch there is no window to
	// tell, and a key owned by another app simply never fires.
	svc.SetShortcutProblems(problems)
}

// appAccel is the key a 快捷键 item carries: the stored one when the user
// rebound it, otherwise the EasyDict default. A stored empty string is a
// deliberate unbinding, so the item keeps its place in the menu with no key.
func appAccel(cur settings.Config, action string) string {
	if v, ok := cur.AppShortcuts[action]; ok {
		return v
	}
	return settings.DefaultAppShortcuts()[action]
}

// sameAppShortcuts reports whether a rebind would leave every menu item's key
// unchanged, so unrelated saves do not walk the menu on every settings write.
func sameAppShortcuts(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for action, accel := range a {
		if b[action] != accel {
			return false
		}
	}
	return true
}

// sameShortcuts reports whether two slot tables would bind the same keys.
// Order inside a slot matters: it is the order readAloud-style fallbacks and the
// tray show, and reordering should reach the native registration.
func sameShortcuts(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for slot, keys := range a {
		other, ok := b[slot]
		if !ok || len(keys) != len(other) {
			return false
		}
		for i := range keys {
			if keys[i] != other[i] {
				return false
			}
		}
	}
	return true
}

// trayActions are what a menu entry does. They are values so the menu can be
// built — and its labels, keys and tick checked — without a running
// application; newTray is the only place that knows about the real services.
type trayActions struct {
	quit         func()
	capture      func()
	captureOCR   func()
	clipboardOCR func()
	ocrWindow    func()
	silentOCR    func()
	input        func()
	selection    func()
	toggleSelect func()
	clipboard    func()
	replace      func()
	polish       func()
	odict        func()
	history      func()
	settings     func()
	about        func()
	feedback     func()
	main         func()
	pin          func()
	update       func()
	set          func(apply func(*settings.Config))
}

// trayMenu draws the status-bar menu for one snapshot of the settings. Labels
// are read here, so a language or shortcut change has to rebuild it; SetMenu
// replaces the item list even while the menu is open.
func trayMenu(cur settings.Config, act trayActions) *application.Menu {
	item := func(name string, onClick func()) *application.MenuItem {
		return application.NewMenuItem(l10n.T(name)).OnClick(func(*application.Context) { onClick() })
	}
	// A tray action shows the key that does the same thing. The accelerator is
	// native decoration — the Carbon/RegisterHotKey binding is what fires while
	// another app is in front — but the menu is where a user looks to learn it.
	keyed := func(m *application.MenuItem, slot string) *application.MenuItem {
		if keys := cur.Shortcuts[slot]; len(keys) > 0 && keys[0] != "" {
			m.SetAccelerator(keys[0])
		}
		return m
	}
	// A preference item only writes the store. The watchers installed in Main
	// are what repaint the windows and redraw this menu, so no item has to do
	// both — and a half-redrawn app is worse than a one-frame delay.
	// 菜单栏图标 is a checkmark pair rather than a select: both styles stay
	// visible and the tick says which one is drawing the status item.
	style := func(name, value string) *application.MenuItem {
		m := application.NewMenuItemCheckbox(l10n.T(name), cur.MenuBarIconStyle == value)
		m.OnClick(func(*application.Context) {
			act.set(func(c *settings.Config) { c.MenuBarIconStyle = value })
		})
		return m
	}
	// 界面语言 and 主题 are checkmark groups for the same reason: the menu is the
	// only place these can be seen, so the tick has to name the live value.
	pick := func(name string, active bool, apply func(*settings.Config)) *application.MenuItem {
		m := application.NewMenuItemCheckbox(l10n.T(name), active)
		m.OnClick(func(*application.Context) { act.set(apply) })
		return m
	}
	pin := application.NewMenuItemCheckbox(l10n.T("置顶译窗"), cur.Pinned)
	pin.OnClick(func(*application.Context) { act.pin() })

	// The OCR entries are a preference because they double the length of a menu
	// that is otherwise one shortcut per line; 显示 OCR 菜单项 is where Easydict
	// puts them too. Every platform here recognizes both a screenshot and a
	// picture on the pasteboard, so the trio is not macOS-only.
	items := []*application.MenuItem{
		keyed(item("截图翻译", act.capture), settings.SlotCapture),
		keyed(item("静默截图 OCR", act.silentOCR), settings.SlotSilentOCR),
	}
	if cur.ShowOCRMenu {
		items = append(items,
			keyed(item("截图 OCR", act.captureOCR), settings.SlotOCRShot),
			keyed(item("剪贴板 OCR", act.clipboardOCR), settings.SlotOCRClipboard),
			keyed(item("显示 OCR 窗口", act.ocrWindow), settings.SlotOCRWindow),
		)
	}
	// 开关划词翻译 is the one menu row that changes state instead of opening a
	// window, so it is a checkbox: the tick is the same stored switch the 通用
	// page shows, and its global shortcut flips it from any app.
	autoSelect := application.NewMenuItemCheckbox(l10n.T("开关划词翻译"), cur.MouseSelect)
	autoSelect.OnClick(func(*application.Context) { act.toggleSelect() })
	items = append(items,
		keyed(item("输入翻译", act.input), settings.SlotInput),
		keyed(item("划词翻译", act.selection), settings.SlotSelection),
	)
	// Auto selection reads the accessibility API, so the entry only exists on
	// the platform where the switch does anything.
	if runtime.GOOS == "darwin" {
		items = append(items, keyed(autoSelect, settings.SlotToggleSelect))
	}
	items = append(items,
		keyed(item("剪贴板翻译", act.clipboard), settings.SlotClipboard),
		keyed(item("翻译并替换", act.replace), settings.SlotReplace),
		keyed(item("润色并替换", act.polish), settings.SlotPolish),
		keyed(item("鼠标取词", act.odict), settings.SlotODict),
		application.NewMenuItemSeparator(),
		pin,
		item("翻译历史记录", act.history),
		keyed(item("服务设置", act.settings), settings.SlotSettings),
		item("打开主窗口", act.main),
		application.NewSubmenu(l10n.T("界面语言"), application.NewMenuFromItems(
			pick("简体中文", cur.UILang == "zh-CN", func(c *settings.Config) { c.UILang = "zh-CN" }),
			pick("English", cur.UILang == "en", func(c *settings.Config) { c.UILang = "en" }),
		)),
		application.NewSubmenu(l10n.T("主题"), application.NewMenuFromItems(
			pick("跟随系统", cur.Theme == "auto", func(c *settings.Config) { c.Theme = "auto" }),
			pick("浅色", cur.Theme == "light", func(c *settings.Config) { c.Theme = "light" }),
			pick("深色", cur.Theme == "dark", func(c *settings.Config) { c.Theme = "dark" }),
		)),
	)
	// A menu bar icon only exists on macOS; elsewhere the tray is a window
	// taskbar icon that the style does not reach.
	if runtime.GOOS == "darwin" {
		items = append(items, application.NewSubmenu(l10n.T("菜单栏图标"), application.NewMenuFromItems(
			style("圆角", settings.MenuBarIconRounded),
			style("方形", settings.MenuBarIconSquare),
		)))
	}
	items = append(items,
		item("检查更新", act.update),
		// 关于 and 反馈 are the two entries Easydict keeps beside 检查更新: the
		// version and credits, and the place to report what is wrong.
		item("关于 Transdict", act.about),
		item("反馈", act.feedback),
		application.NewMenuItemSeparator(),
		item("退出 Transdict", act.quit),
	)
	return application.NewMenuFromItems(items[0], items[1:]...)
}

// trayGlyph picks the template image 菜单栏图标 names, falling back to the bare
// mark when the square variant was not embedded.
func trayGlyph(style string, bare, square []byte) []byte {
	if style == settings.MenuBarIconSquare && len(square) > 0 {
		return square
	}
	return bare
}

func newTray(app *application.App, cfg *settings.Store, svc *gui.Service, icon, template, square, windowsIcon []byte) *application.SystemTray {
	tray := app.SystemTray.New()
	glyph := func(style string) []byte { return trayGlyph(style, template, square) }
	if runtime.GOOS == "darwin" && len(template) > 0 {
		tray.SetTemplateIcon(glyph(cfg.Get().MenuBarIconStyle))
	} else if runtime.GOOS == "windows" && len(windowsIcon) > 0 {
		tray.SetIcon(windowsIcon)
	} else {
		tray.SetIcon(icon)
	}
	tray.SetTooltip("Transdict")

	actions := trayActions{
		quit: app.Quit,
		capture: func() {
			if err := svc.Start(); err != nil {
				log.Printf("transdict: 截图: %v", err)
			}
		},
		captureOCR: func() {
			if err := svc.StartOCR(); err != nil {
				log.Printf("transdict: 截图 OCR: %v", err)
			}
		},
		clipboardOCR: func() {
			if err := svc.ClipboardOCR(); err != nil {
				log.Printf("transdict: 剪贴板 OCR: %v", err)
			}
		},
		ocrWindow: func() {
			if err := svc.OpenLastOCR(); err != nil {
				log.Printf("transdict: OCR 窗口: %v", err)
			}
		},
		silentOCR: func() {
			if err := svc.StartSilent(); err != nil {
				log.Printf("transdict: 静音 OCR: %v", err)
			}
		},
		input:        svc.OpenInput,
		toggleSelect: func() { toggleSelect(cfg) },
		selection: func() {
			if err := svc.CatchSelection(); err != nil {
				log.Printf("transdict: 划词: %v", err)
			}
		},
		clipboard: func() {
			if err := svc.ClipboardQuery(); err != nil {
				log.Printf("transdict: 剪贴板: %v", err)
			}
		},
		replace: func() {
			if err := svc.ReplaceSelection(); err != nil {
				log.Printf("transdict: 替换: %v", err)
			}
		},
		polish: func() {
			if err := svc.PolishReplaceSelection(); err != nil {
				log.Printf("transdict: 润色并替换: %v", err)
			}
		},
		odict: func() {
			if err := svc.WordUnderCursor(); err != nil {
				log.Printf("transdict: 取词: %v", err)
			}
		},
		history:  svc.OpenHistory,
		settings: svc.OpenSettings,
		about:    svc.OpenAbout,
		feedback: func() {
			if err := svc.OpenFeedbackPage(); err != nil {
				log.Printf("transdict: 反馈: %v", err)
			}
		},
		main: svc.OpenMain,
		pin:  func() { svc.TogglePin() },
		// The answer waits on a server and the panel waits on Show(), which has
		// to leave the main thread to appear; a menu callback is on it.
		update: func() { go svc.CheckUpdateNow() },
		set: func(apply func(*settings.Config)) {
			if _, err := cfg.Update(func(c *settings.Config) { apply(c) }); err != nil {
				log.Printf("transdict: 菜单写入设置失败: %v", err)
			}
		},
	}

	// Windows opens the menu on right-click, so the left button is free. A
	// single click opens the translation window; a double click opens 设置.
	// Wails 3.0.0 does not deliver the left click itself, so the platform file
	// recovers it from the tray's window procedure.
	installTrayClick(tray, svc)

	tray.SetMenu(trayMenu(cfg.Get(), actions))
	l10n.OnChange(func() {
		tray.SetMenu(trayMenu(cfg.Get(), actions))
		// The settings and history pages redraw themselves in the new language;
		// the native title each window reports was written when it was created.
		svc.SyncWindowTitles()
	})
	// The menu draws the current shortcuts and the 置顶 tick, so a change made
	// in 服务设置 has to redraw it rather than wait for the next launch.
	style := cfg.Get().MenuBarIconStyle
	cfg.Watch(func(next settings.Config) {
		tray.SetMenu(trayMenu(next, actions))
		if runtime.GOOS != "darwin" || next.MenuBarIconStyle == style {
			return
		}
		style = next.MenuBarIconStyle
		application.InvokeAsync(func() { tray.SetTemplateIcon(glyph(style)) })
	})
	return tray
}
