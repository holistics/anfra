//go:build !windows

package appserve

import (
	"path/filepath"
	"strings"
)

// systemTimezone is the IANA zone /etc/localtime links to, or "".
func systemTimezone() string {
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			return zone
		}
	}
	return ""
}
