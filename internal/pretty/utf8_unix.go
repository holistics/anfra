//go:build !windows

package pretty

import (
	"os"
	"strings"
)

// utf8Terminal reports whether the locale is UTF-8, for tree's lines.
func utf8Terminal() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToUpper(v)
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "UTF8")
		}
	}
	return false
}
