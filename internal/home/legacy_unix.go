//go:build !windows

package home

import (
	"os"
	"path/filepath"
)

// legacyDir is where anfra kept its files before Dir: anfra in the user's
// cache folder. "" when there is none.
func legacyDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "anfra")
}
