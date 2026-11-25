package exports

import (
	"net/url"
	"path/filepath"
	"strings"
)

// FileLink is the file:// link to the file at path, in the standard form a
// browser reads: file:///tmp/x on Unix, file:///C:/Users/x on Windows.
func FileLink(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // a drive (C:/…) goes after the third slash
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// FileLinkPath is the path the file:// link u names.
func FileLinkPath(u *url.URL) string {
	p := u.Path
	if strings.HasPrefix(p, "/") && filepath.VolumeName(p[1:]) != "" {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
