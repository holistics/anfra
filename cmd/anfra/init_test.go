package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/shared/apperr"
)

var initFiles = []string{
	".anfra/data_sources.yml", ".anfra/data_sources.yml.example", ".gitignore", ".anfra/context_sources.yml",
	"models/.gitkeep", "datasets/.gitkeep", "apps/.gitkeep", "AGENTS.md",
}

func TestInitSetsUpARepo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	res, err := initRepo(context.Background(), dir, initOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range initFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
		if i >= len(res.Files) || res.Files[i] != (initFile{Path: f, Changed: true}) {
			t.Errorf("not reported as created: %s\n%+v", f, res.Files)
		}
	}
	if res.Dir != dir || res.Message != "Set up an anfra repo in "+dir+", with 8 files." || len(res.Hints) != 2 {
		t.Errorf("result: %+v", res)
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

// In a clone, which has the committed example but not data_sources.yml,
// init makes data_sources.yml from the example: the team's data sources.
func TestInitCopiesTheExample(t *testing.T) {
	dir := t.TempDir()
	example := "data_sources:\n  shop:\n    type: duckdb\n"
	if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".anfra/data_sources.yml.example"), []byte(example), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := initRepo(context.Background(), dir, initOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".anfra/data_sources.yml")); string(got) != example {
		t.Errorf("data_sources.yml: %q, want the example", got)
	}
}

// --data-sources sets up the data source files alone.
func TestInitDataSourcesOnly(t *testing.T) {
	dir := t.TempDir()
	res, err := initRepo(context.Background(), dir, initOptions{dataSourcesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	if want := initFiles[:3]; strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("files: %v, want %v", paths, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Error("AGENTS.md was written")
	}
}

// A symlinked .gitignore is not read by Git, and its target may be outside
// the repo: init leaves it alone and says so.
func TestInitRefusesASymlinkedGitignore(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(outside, []byte("export X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	_, err := initRepo(context.Background(), dir, initOptions{})
	if err == nil || !strings.Contains(err.Error(), ".gitignore") {
		t.Fatalf("err = %v, want one about .gitignore", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "export X=1\n" {
		t.Errorf("the link's target changed: %q", got)
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

	res, err := initRepo(context.Background(), dir, initOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".anfra/data_sources.yml")); string(got) != mine {
		t.Errorf("data_sources.yml was changed: %q", got)
	}
	if res.Files[0] != (initFile{Path: ".anfra/data_sources.yml", Changed: false}) {
		t.Errorf("not reported as kept: %+v", res.Files[0])
	}
	// Another tool's lines stay, and anfra's is added after them, on a line of its own.
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.HasPrefix(string(ignore), "node_modules\n") || !strings.HasSuffix(string(ignore), "\n.anfra/data_sources.yml\n") {
		t.Errorf(".gitignore: %q", ignore)
	}

	// Again: nothing left to create, and .gitignore gets no second line.
	res, err = initRepo(context.Background(), dir, initOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if f.Changed {
			t.Errorf("a second run changed %s", f.Path)
		}
	}
	if !strings.HasPrefix(res.Message, "Nothing changed: ") {
		t.Errorf("message: %q", res.Message)
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
	add(&tar.Header{Name: top + "example-data/ecommerce.duckdb"}, "duckdb")
	add(&tar.Header{Name: top + "apps/ecommerce/overview.html"}, "<html></html>\n")
	add(&tar.Header{Name: top + "wiki/ecommerce/metrics.md"}, "# Definitions\n")
	add(&tar.Header{Name: top + "wiki/ecommerce/AGENTS.md"}, "# Maintaining the wiki\n")
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
	res, err := initRepo(context.Background(), dir, initOptions{sample: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"models/ecommerce/orders.model.aml", "datasets/ecommerce/ecommerce.dataset.aml", "example-data/ecommerce.duckdb", "apps/ecommerce/overview.html", "wiki/ecommerce/metrics.md", "wiki/ecommerce/AGENTS.md"} {
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
	if !strings.Contains(res.Message, "holistics/anfra-sample-ecommerce@f89940a") {
		t.Errorf("the commit is not reported: %q", res.Message)
	}
	// The repo's one data source is the sample's DuckDB file.
	sources, err := datasource.Load(filepath.Join(dir, ".anfra"))
	if err != nil {
		t.Fatal(err)
	}
	if ds, ok := sources["ecommerce_duckdb"]; !ok || ds.DBType != "duckdb" || len(sources) != 1 {
		t.Errorf("data sources: %+v", sources)
	}
	// A clone can query the sample: the DuckDB file is not git-ignored, and the
	// committed example has its data source.
	if ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); strings.Contains(string(ignore), "ecommerce.duckdb") {
		t.Errorf(".gitignore: %q", ignore)
	}
	if example, _ := os.ReadFile(filepath.Join(dir, ".anfra/data_sources.yml.example")); !strings.Contains(string(example), "path: example-data/ecommerce.duckdb?") {
		t.Errorf("data_sources.yml.example:\n%s", example)
	}
	if strings.Contains(strings.Join(res.Hints, "\n"), "nobody was asked") {
		t.Errorf("offered the sample it added: %q", res.Hints)
	}
	// The repo's AGENTS.md is init's own: the sample's root AGENTS.md is not taken.
	if md, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); strings.Contains(string(md), "Agent instructions") || !strings.Contains(string(md), "<!-- anfra:begin -->") {
		t.Errorf("AGENTS.md:\n%s", md)
	}
}

// A download that fails leaves the repo set up, and says how to retry. It is
// GitHub unavailable, in the download's public step, with no glued prefix.
func TestInitWithSampleFailedDownload(t *testing.T) {
	serveSample(t, http.StatusNotFound, nil)
	dir := filepath.Join(t.TempDir(), "shop")
	res, err := initRepo(context.Background(), dir, initOptions{sample: true})
	if !errors.Is(err, clierr.Unavailable) {
		t.Fatalf("got %v, want unavailable", err)
	}
	r := apperr.From(err).Response("")
	if len(r.Context) != 1 || r.Context[0].Step != "anfra_cli.download_sample" || !strings.Contains(r.Message, "404") {
		t.Errorf("context %+v, message %q", r.Context, r.Message)
	}
	if !strings.Contains(strings.Join(res.Hints, "\n"), "anfra init "+dir+" --example") {
		t.Errorf("no retry command in the hints: %q", res.Hints)
	}
	for _, f := range initFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// Nobody chose, as when an agent runs init without a terminal: the hints
// offer the sample, for the agent to ask its user; a choice made leaves it out.
func TestInitOffersTheSample(t *testing.T) {
	for _, offer := range []bool{true, false} {
		res, err := initRepo(context.Background(), t.TempDir(), initOptions{offer: offer})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(strings.Join(res.Hints, "\n"), "ask the user whether to add it"); got != offer {
			t.Errorf("offer %v, hints: %q", offer, res.Hints)
		}
	}
}
