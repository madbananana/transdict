//go:build windows

package settings

// addPlatformServices adds the engine Windows ships. Windows.Media.Ocr reads the
// picture offline through PowerShell, so a fresh install translates screenshots
// without a key or a network round trip, the way Apple's recognizer does on
// macOS.
func addPlatformServices(c *Config) {
	addFirst(&c.OCR, Provider{ID: "ocr-windows", Category: CatOCR, Kind: "windows",
		Label: "Windows 系统 OCR", Enabled: true})
	selectFirst(c, ContextScreenshot, "ocr-windows")
	selectFirst(c, ContextSilentOCR, "ocr-windows")
}
