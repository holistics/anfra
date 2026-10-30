package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
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
	if err := initRepo(context.Background(), dir, &out, initChoice{}); err != nil {
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
	if err := initRepo(context.Background(), dir, &out, initChoice{}); err != nil {
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
	if err := initRepo(context.Background(), dir, &out, initChoice{}); err != nil {
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

// sampleArchive is an archive like GitHub's: one top folder, and a global
// header naming the commit. Besides the sample's files it holds what init must
// not write: a file outside the sample's folders, a path out of the repo, and a
// link.
func sampleArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(h *tar.Header, body string) {
		h.Size = int64(len(body))
		if h.Typeflag == 0 {
			h.Typeflag, h.Mode = tar.TypeReg, 0o644
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "f89940a37e6cdd313b76037cbeb7718fd65134ea"}}, "")
	top := "anfra-sample-ecommerce-main/"
	add(&tar.Header{Name: top + "models/ecommerce/orders.model.aml"}, "Model orders {}\n")
	add(&tar.Header{Name: top + "datasets/ecommerce/ecommerce.dataset.aml"}, "Dataset ecommerce {}\n")
	add(&tar.Header{Name: top + "data/ecommerce.duckdb"}, "duckdb")
	add(&tar.Header{Name: top + "wiki/ecommerce/metrics.md"}, "# Definitions\n")
	add(&tar.Header{Name: top + "AGENTS.md"}, "# Agent instructions\n")
	add(&tar.Header{Name: top + "README.md"}, "# Sample\n")
	add(&tar.Header{Name: top + ".anfra/data_sources.yml"}, "data_sources: {}\n")
	add(&tar.Header{Name: top + "models/ecommerce/../../escape.model.aml"}, "x")
	add(&tar.Header{Name: top + "models/ecommerce/link.model.aml", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, "")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serveSample(t *testing.T, status int, body []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	old := sampleURL
	sampleURL = srv.URL
	t.Cleanup(func() { sampleURL = old })
}

func TestInitWithSample(t *testing.T) {
	serveSample(t, http.StatusOK, sampleArchive(t))
	dir := filepath.Join(t.TempDir(), "shop")
	var out bytes.Buffer
	if err := initRepo(context.Background(), dir, &out, initChoice{sample: true}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"models/ecommerce/orders.model.aml", "datasets/ecommerce/ecommerce.dataset.aml", "data/ecommerce.duckdb", "wiki/ecommerce/metrics.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	// Only the sample's files: not its config, nothing out of the repo, no link.
	for _, f := range []string{"../escape.model.aml", "escape.model.aml", "models/ecommerce/link.model.aml", "README.md"} {
		if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s was written", f)
		}
	}
	if !strings.Contains(out.String(), "holistics/anfra-sample-ecommerce@f89940a") {
		t.Errorf("the commit is not reported:\n%s", out.String())
	}
	// The repo's one data source is the sample's DuckDB file.
	sources, err := datasource.Load(filepath.Join(dir, ".anfra"))
	if err != nil {
		t.Fatal(err)
	}
	if ds, ok := sources["ecommerce_duckdb"]; !ok || ds.DBType != "duckdb" || len(sources) != 1 {
		t.Errorf("data sources: %+v", sources)
	}
	if strings.Contains(out.String(), "Sample available") {
		t.Errorf("offered the sample it added:\n%s", out.String())
	}
}

// A download that fails leaves the repo set up, and says how to retry.
func TestInitWithSampleFailedDownload(t *testing.T) {
	serveSample(t, http.StatusNotFound, nil)
	dir := filepath.Join(t.TempDir(), "shop")
	var out bytes.Buffer
	err := initRepo(context.Background(), dir, &out, initChoice{sample: true})
	if err == nil || !strings.Contains(err.Error(), "anfra init "+dir+" --with-sample") {
		t.Fatalf("error: %v", err)
	}
	for _, f := range initFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// Nobody chose, as when an agent runs init without a terminal: the output
// offers the sample, for the agent to ask its user; a choice made leaves it out.
func TestInitOffersTheSample(t *testing.T) {
	for _, offer := range []bool{true, false} {
		var out bytes.Buffer
		if err := initRepo(context.Background(), t.TempDir(), &out, initChoice{offer: offer}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(out.String(), "ask the user whether to\ninclude it"); got != offer {
			t.Errorf("offer %v, output:\n%s", offer, out.String())
		}
	}
}
