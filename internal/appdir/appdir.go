// Package appdir resolves the per-user directories Transdict reads and writes.
package appdir

import (
	"os"
	"path/filepath"
	"sync"
)

const appName = "Transdict"

var (
	once sync.Once
	root string
	err  error
)

// Root is the application config directory, created on first use.
func Root() (string, error) {
	once.Do(func() {
		base, e := os.UserConfigDir()
		if e != nil {
			err = e
			return
		}
		root = filepath.Join(base, appName)
		err = os.MkdirAll(root, 0o700)
	})
	return root, err
}

// Sub returns a created subdirectory such as dicts or cache.
func Sub(name ...string) (string, error) {
	base, err := Root()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(append([]string{base}, name...)...)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
