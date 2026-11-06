// Package l10n localizes the text Go puts in front of the user: the tray menu
// and the labels behind the settings tables. The webview keeps its own table in
// frontend/src/i18n.ts. Both are keyed by the Chinese source text, so a phrase
// without an English entry still renders rather than turning into a blank menu
// item.
package l10n

import "sync"

// zhCN is the source language, so an unrecognised setting falls back to it
// instead of leaving the app half-translated.
const zhCN = "zh-CN"

var (
	mu      sync.RWMutex
	lang    = zhCN
	changed []func()
)

// Set adopts the interface language and reports whether it changed. Menus built
// at startup do not re-read config, so anything drawn outside the webview has to
// be rebuilt from an OnChange callback.
func Set(next string) bool {
	if next != "en" {
		next = zhCN
	}
	mu.Lock()
	was := lang != next
	lang = next
	handlers := make([]func(), len(changed))
	copy(handlers, changed)
	mu.Unlock()
	if was {
		for _, fn := range handlers {
			fn()
		}
	}
	return was
}

// Lang reports the interface language in use.
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// T returns text in the interface language. Chinese is what the source says, so
// it is what comes back whenever there is no English entry.
func T(text string) string {
	mu.RLock()
	english := lang == "en"
	mu.RUnlock()
	if !english {
		return text
	}
	if got, ok := en[text]; ok {
		return got
	}
	return text
}

// OnChange registers a redraw for surfaces that were built once, such as the
// tray menu. Callbacks run on the goroutine that changed the language.
func OnChange(fn func()) {
	mu.Lock()
	changed = append(changed, fn)
	mu.Unlock()
}

var (
	zhOnce sync.Once
	toZh   map[string]string
)

// Zh is T backwards: it returns the Chinese source of an English string the
// table produced, or "" when the text was never ours. A settings save comes back
// worded for the language the window was showing, and stored data keeps the
// source wording so the next language switch can translate it again.
func Zh(text string) string {
	zhOnce.Do(func() {
		toZh = make(map[string]string, len(en))
		for zh, english := range en {
			if _, clash := toZh[english]; !clash {
				toZh[english] = zh
			}
		}
	})
	mu.RLock()
	english := lang == "en"
	mu.RUnlock()
	if !english {
		return ""
	}
	return toZh[text]
}
