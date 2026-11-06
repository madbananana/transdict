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
	"transdict/internal/netp"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/shortcut"
)

// Main assembles and runs the app. Embedded assets arrive from package main
// because //go:embed paths are relative to the declaring package.
//
// trayIcon is an optional monochrome glyph: macOS menu bars want a template
// image the system tints for light and dark, not the coloured app icon.
func Main(version string, assets embed.FS, appIcon, trayIcon []byte) {
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

	app := application.New(application.Options{
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
	})

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

	// macOS keeps Preferences in the application menu, and that is where people
	// look for it (⌘,). Wails' default app menu has About and Quit but no
	// Preferences, so it is rebuilt with 设置… between them.
	appMenu := application.DefaultApplicationMenu()
	if top := appMenu.ItemAt(0); top != nil && top.GetSubmenu() != nil {
		sub := top.GetSubmenu()
		sub.Clear()
		sub.AddRole(application.About)
		sub.AddSeparator()
		prefs := sub.Add(l10n.T("设置…"))
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
		app.Menu.SetApplicationMenu(appMenu)
	}

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
				case "mini":
					if err := svc.ClipboardMini(); err != nil {
						log.Printf("transdict: 迷你窗: %v", err)
					}
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
				case "silent":
					if err := svc.StartSilent(); err != nil {
						log.Printf("transdict: 静音 OCR: %v", err)
					}
				case "capture":
					if err := svc.Start(); err != nil {
						log.Printf("transdict: 截图: %v", err)
					}
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
	newTray(app, cfg, svc, appIcon, trayIcon)

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

	// The launch check waits on somebody else's server, so it gets its own
	// goroutine: a release host that is down must not delay a window.
	if cfg.Get().AutoUpdate {
		go svc.AutoCheckOnLaunch()
	}

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
// "come back". Naming a window explicitly is how a script reaches 迷你窗 without
// a global hotkey; select, odict and capture are the same idea for the actions
// that otherwise only a shortcut or the tray can start.
func wakeCommand(args []string) string {
	if len(args) > 1 {
		switch args[1] {
		case "main", "input", "settings", "history", "mini", "clipboard", "select", "odict", "replace", "silent", "capture":
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

func bindShortcuts(app *application.App, cfg *settings.Store, svc *gui.Service) {
	actions := shortcut.Actions{
		Capture:   func() { _ = svc.Start() },
		Input:     svc.OpenInput,
		Settings:  svc.OpenSettings,
		Clipboard: func() { _ = svc.ClipboardQuery() },
		Mini:      func() { _ = svc.ClipboardMini() },
		SilentOCR: func() { _ = svc.StartSilent() },
		Selection: func() { _ = svc.CatchSelection() },
		Replace:   func() { _ = svc.ReplaceSelection() },
		ODict:     func() { _ = svc.WordUnderCursor() },
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
	quit      func()
	capture   func()
	silentOCR func()
	input     func()
	selection func()
	clipboard func()
	mini      func()
	replace   func()
	odict     func()
	history   func()
	settings  func()
	main      func()
	pin       func()
	update    func()
	set       func(apply func(*settings.Config))
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
	set := func(name string, apply func(*settings.Config)) *application.MenuItem {
		return application.NewMenuItem(l10n.T(name)).OnClick(func(*application.Context) { act.set(apply) })
	}
	pin := application.NewMenuItemCheckbox(l10n.T("置顶译窗"), cur.Pinned)
	pin.OnClick(func(*application.Context) { act.pin() })

	return application.NewMenuFromItems(
		keyed(item("截图翻译", act.capture), settings.SlotCapture),
		keyed(item("静音 OCR", act.silentOCR), settings.SlotSilentOCR),
		keyed(item("输入翻译", act.input), settings.SlotInput),
		keyed(item("划词翻译", act.selection), settings.SlotSelection),
		keyed(item("剪贴板翻译", act.clipboard), settings.SlotClipboard),
		keyed(item("迷你窗", act.mini), settings.SlotMini),
		keyed(item("翻译并替换选区", act.replace), settings.SlotReplace),
		keyed(item("鼠标取词", act.odict), settings.SlotODict),
		application.NewMenuItemSeparator(),
		pin,
		item("翻译历史记录", act.history),
		keyed(item("服务设置", act.settings), settings.SlotSettings),
		item("打开主窗口", act.main),
		application.NewSubmenu(l10n.T("界面语言"), application.NewMenuFromItems(
			set("简体中文", func(c *settings.Config) { c.UILang = "zh-CN" }),
			set("English", func(c *settings.Config) { c.UILang = "en" }),
		)),
		application.NewSubmenu(l10n.T("主题"), application.NewMenuFromItems(
			set("跟随系统", func(c *settings.Config) { c.Theme = "auto" }),
			set("浅色", func(c *settings.Config) { c.Theme = "light" }),
			set("深色", func(c *settings.Config) { c.Theme = "dark" }),
		)),
		item("检查更新", act.update),
		application.NewMenuItemSeparator(),
		item("退出 Transdict", act.quit),
	)
}

func newTray(app *application.App, cfg *settings.Store, svc *gui.Service, icon, template []byte) {
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" && len(template) > 0 {
		tray.SetTemplateIcon(template)
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
		silentOCR: func() {
			if err := svc.StartSilent(); err != nil {
				log.Printf("transdict: 静音 OCR: %v", err)
			}
		},
		input: svc.OpenInput,
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
		mini: func() {
			if err := svc.ClipboardMini(); err != nil {
				log.Printf("transdict: 迷你窗: %v", err)
			}
		},
		replace: func() {
			if err := svc.ReplaceSelection(); err != nil {
				log.Printf("transdict: 替换: %v", err)
			}
		},
		odict: func() {
			if err := svc.WordUnderCursor(); err != nil {
				log.Printf("transdict: 取词: %v", err)
			}
		},
		history:  svc.OpenHistory,
		settings: svc.OpenSettings,
		main:     svc.OpenMain,
		pin:      func() { svc.TogglePin() },
		// The answer waits on a server and the panel waits on Show(), which has
		// to leave the main thread to appear; a menu callback is on it.
		update: func() { go svc.CheckUpdateNow() },
		set: func(apply func(*settings.Config)) {
			if _, err := cfg.Update(func(c *settings.Config) { apply(c) }); err != nil {
				log.Printf("transdict: 菜单写入设置失败: %v", err)
			}
		},
	}
	tray.SetMenu(trayMenu(cfg.Get(), actions))
	l10n.OnChange(func() {
		tray.SetMenu(trayMenu(cfg.Get(), actions))
		// The settings and history pages redraw themselves in the new language;
		// the native title each window reports was written when it was created.
		svc.SyncWindowTitles()
	})
	// The menu draws the current shortcuts and the 置顶 tick, so a change made
	// in 服务设置 has to redraw it rather than wait for the next launch.
	cfg.Watch(func(next settings.Config) { tray.SetMenu(trayMenu(next, actions)) })
}
