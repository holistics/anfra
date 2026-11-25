//go:build windows

package workdir_test

import (
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/publish/workdir"
)

// On Windows, pull refuses names Windows can't hold as written: they would
// reach a device, a file's hidden stream, or another file.
func TestCheckPathWindowsNames(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"models/a.aml", "models/console.aml", "models/con2.aml", "models/com10.aml", "apps/x/index.html"} {
		if err := workdir.CheckPath(dir, p); err != nil {
			t.Errorf("%s: %v, want accepted", p, err)
		}
	}
	for _, p := range []string{
		"models/CON.aml", "models/aux", "models/nul.model.aml", "con/a.aml", "models/Com1.aml", "models/lpt9.aml",
		"models/a:b.aml", "models/a<b.aml", "models/a>b.aml", `models/a"b.aml`, "models/a|b.aml", "models/a?.aml", "models/a*.aml",
		"models/a\x01.aml",
		"models/a.aml.", "models/a.aml ", "models./a.aml", "models /a.aml",
	} {
		var unsafe *workdir.UnsafePathError
		if err := workdir.CheckPath(dir, p); !errors.As(err, &unsafe) {
			t.Errorf("%q: %v, want refused", p, err)
		} else {
			t.Logf("%q: %v", p, err)
		}
	}
}
