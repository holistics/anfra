package validate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppReports(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"apps/sales.html":         `<script src="chart.js"></script>`,
		"apps/team/overview.html": `<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/a@1/a.css">`,
		"apps/clean.html":         `<title>Clean</title>`,
		"apps/logo.png":           "png",
		"apps/.hidden/x.html":     `<img src="x.png">`,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files := func(globs ...string) map[string]int {
		got := map[string]int{}
		for _, r := range appReports(dir, globs) {
			if r.Severity != "warning" || r.Validator != "data_app" {
				t.Errorf("report %+v: want a data_app warning", r)
			}
			got[r.FilePath]++
		}
		return got
	}
	if got := files(); len(got) != 2 || got["apps/sales.html"] != 1 || got["apps/team/overview.html"] != 1 {
		t.Errorf("every app: %v", got)
	}
	if got := files("apps/team"); len(got) != 1 || got["apps/team/overview.html"] != 1 {
		t.Errorf("a folder: %v", got)
	}
	if got := files("apps/*.html"); len(got) != 1 || got["apps/sales.html"] != 1 {
		t.Errorf("a glob: %v", got)
	}
	if got := files("models/*.aml"); len(got) != 0 {
		t.Errorf("no app selected: %v", got)
	}
}
