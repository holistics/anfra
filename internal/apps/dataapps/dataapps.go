// Package dataapps serves a Data App's HTML with the Anfra SDK provisioned ahead of its code.
package dataapps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/holistics/anfra/internal/apps/descriptors"
)

// User is who every Data App runs as: Data App serving is local, with one reader.
type User struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Email       string      `json:"email"`
	Role        string      `json:"role"`
	Timezone    string      `json:"timezone"`
	Permissions Permissions `json:"permissions"`
}

type Permissions struct {
	CanViewGeneratedSQL bool `json:"canViewGeneratedSql"`
	CanExportData       bool `json:"canExportData"`
}

// localTimezone is the machine's IANA zone: $TZ, else what /etc/localtime links to, else UTC.
func localTimezone() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		return tz
	}
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(target, "zoneinfo/"); ok {
			return zone
		}
	}
	return "UTC"
}

// LocalUser is the fixed local reader.
func LocalUser() User {
	return User{
		ID: 1, Name: "Local Developer", Email: "developer@localhost", Role: "admin",
		Timezone:    localTimezone(),
		Permissions: Permissions{CanViewGeneratedSQL: true, CanExportData: true},
	}
}

// scriptJSON is JSON safe inside a <script>: no `</script>` or `<!--` can end it early.
// encoding/json escapes <, > and & as \u003c, \u003e and \u0026 by default (HTML-safe output).
func scriptJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

var closeScript = regexp.MustCompile(`(?i)</script`)

// scriptCode keeps inlined code from closing its own <script> tag.
func scriptCode(code string) string {
	return closeScript.ReplaceAllString(code, `<\/script`)
}

var (
	charsetRe = regexp.MustCompile(`(?i)<meta[^>]*charset[^>]*>`)
	headRe    = regexp.MustCompile(`(?i)<head[^>]*>`)
)

// Options is what a Data App is provisioned with.
type Options struct {
	SDKBundle   string
	Bootstrap   string
	Datasets    map[string]descriptors.Dataset
	ShellOrigin string
}

// Provision inlines the provisioning data, the SDK bundle and the frame bootstrap at the top of the
// Data App's <head>, so `Anfra` exists before the author's first script. The author's file never
// mentions the SDK.
func Provision(html string, opts Options) string {
	injected := strings.Join([]string{
		`<script type="application/json" id="anfra-provision">` + scriptJSON(map[string]any{
			"datasets":    opts.Datasets,
			"user":        LocalUser(),
			"shellOrigin": opts.ShellOrigin,
		}) + `</script>`,
		`<script>` + scriptCode(opts.SDKBundle) + `</script>`,
		`<script>` + scriptCode(opts.Bootstrap) + `</script>`,
	}, "\n")

	// After <meta charset> when there is one, so the encoding is still declared first.
	for _, re := range []*regexp.Regexp{charsetRe, headRe} {
		if loc := re.FindStringIndex(html); loc != nil {
			return html[:loc[1]] + "\n" + injected + "\n" + html[loc[1]:]
		}
	}
	return injected + "\n" + html
}
