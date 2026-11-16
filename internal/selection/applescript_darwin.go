//go:build darwin

package selection

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// scriptTimeout bounds the browser's scripting interface. A missing Automation
// grant or a disabled JavaScript-from-Apple-Events setting fails at once; the
// timeout is for a browser that stalls under a modal sheet, and it has to leave
// room for the copy routes below it.
var scriptTimeout = 3 * time.Second

// browserScripts maps the browsers whose AppleScript dictionary can hand back
// the front window's selection. Safari has no accessibility route at all, and
// Chromium browsers only answer through scripting when the user allowed
// JavaScript from Apple Events.
var browserScripts = map[string]string{
	"com.apple.Safari":                  safariSelection,
	"com.apple.SafariTechnologyPreview": safariSelection,
	"com.google.Chrome":                 chromiumSelection,
	"com.google.Chrome.canary":          chromiumSelection,
	"com.google.Chrome.beta":            chromiumSelection,
	"com.microsoft.edgemac":             chromiumSelection,
	"com.microsoft.edgemac.Beta":        chromiumSelection,
	"com.microsoft.edgemac.Dev":         chromiumSelection,
	"com.brave.Browser":                 chromiumSelection,
	"com.brave.Browser.beta":            chromiumSelection,
	"com.vivaldi.Vivaldi":               chromiumSelection,
	"com.operasoftware.Opera":           chromiumSelection,
	"org.chromium.Chromium":             chromiumSelection,
}

const safariSelection = `tell application id "%s" to do JavaScript "window.getSelection().toString()" in front document`

const chromiumSelection = `tell application id "%s" to execute front window's active tab javascript "window.getSelection().toString()"`

// browserSelectionScript names the AppleScript that reads a selection from the
// given application, or "" when it is not a browser we know how to ask.
func browserSelectionScript(bundle string) string {
	template, ok := browserScripts[bundle]
	if !ok {
		return ""
	}
	return strings.Replace(template, "%s", bundle, 1)
}

// BrowserSelectedText reads the frontmost browser's selection through its own
// scripting interface. ok=false means this route could not answer — the
// application is not a browser, the user has not allowed Automation, or the
// browser refuses page scripts — and the caller should try 强制取词.
func BrowserSelectedText() (string, bool) {
	script := browserSelectionScript(FrontmostBundleID())
	if script == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
