// transdict — 跨平台词典与翻译工具。装配在 internal/cli，这里只嵌入资源。
package main

import (
	"embed"

	"transdict/internal/cli"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

//go:embed build/trayicon.png
var trayIcon []byte

// version is set at build time: -ldflags "-X main.version=…"
var version = "dev"

func main() { cli.Main(version, assets, appIcon, trayIcon) }
