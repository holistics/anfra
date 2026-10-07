package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/datasource"
)

var initFiles = []string{
	".anfra/data_sources.yml", ".anfra/data_sources.yml.example", ".anfra/context_sources.yml",
	"models/.gitkeep", "datasets/.gitkeep", "apps/.gitkeep", ".gitignore",
}

func TestInitSetsUpARepo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	var out bytes.Buffer
	if err := initRepo(dir, &out); err != nil {
		t.Fatal(err)
	}
	for _, f := range initFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
		if !strings.Contains(out.String(), "created "+f) {
			t.Errorf("not reported as created: %s\n%s", f, out.String())
		}
	}

	// The template is a data source anfra reads, so a repo works once it is filled in.
	sources, err := datasource.Load(filepath.Join(dir, ".anfra"))
	if err != nil {
		t.Fatal(err)
	}
	if ds := sources["warehouse"]; ds.DBType != "postgresql" || ds.Connection["host"] != "localhost" {
		t.Errorf("the template: %+v", sources)
	}
	// It holds credentials: the owner's alone, and out of Git.
	if info, _ := os.Stat(filepath.Join(dir, ".anfra/data_sources.yml")); info.Mode().Perm() != 0o600 {
		t.Errorf("data_sources.yml is %v, want 0600", info.Mode().Perm())
	}
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(ignore), "\n.anfra/data_sources.yml\n") {
		t.Errorf(".gitignore: %q", ignore)
	}
}

func TestInitKeepsWhatIsThere(t *testing.T) {
	dir := t.TempDir()
	mine := "data_sources:\n  prod:\n    type: snowflake\n"
	if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".anfra/data_sources.yml"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := initRepo(dir, &out); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".anfra/data_sources.yml")); string(got) != mine {
		t.Errorf("data_sources.yml was changed: %q", got)
	}
	if !strings.Contains(out.String(), "kept    .anfra/data_sources.yml") {
		t.Errorf("not reported as kept:\n%s", out.String())
	}
	// Another tool's lines stay, and anfra's is added after them, on a line of its own.
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.HasPrefix(string(ignore), "node_modules\n") || !strings.HasSuffix(string(ignore), "\n.anfra/data_sources.yml\n") {
		t.Errorf(".gitignore: %q", ignore)
	}

	// Again: nothing left to create, and .gitignore gets no second line.
	out.Reset()
	if err := initRepo(dir, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "created") {
		t.Errorf("a second run created something:\n%s", out.String())
	}
	if again, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); !bytes.Equal(again, ignore) {
		t.Errorf(".gitignore changed on a second run: %q", again)
	}
}

// Setting up a repo writes files on this machine, so it stays a CLI command and
// is never registered as an op that a server would serve.
func TestInitIsNotAnOp(t *testing.T) {
	if _, ok := app.Find("init"); ok {
		t.Error("init is a registered command, so it is served as an op")
	}
}
