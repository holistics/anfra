package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/publish"
	"github.com/holistics/anfra/internal/repo"
)

// dataSourcesHeader opens a new repo's data_sources.yml and its committed
// example; one data source follows it.
const dataSourcesHeader = `# The data sources this repo's models query, by name: a model's or dataset's
# data_source_name is one of these names. Each connection is passed as-is to
# the database driver: its keys depend on the type.
#
# data_sources.yml holds credentials, so it is git-ignored; this repo commits
# data_sources.yml.example instead, without them.
#
# settings, optional, are how the query engine connects and queries: timeouts,
# requiring SSL and the like. Without them, the engine's defaults.
data_sources:
`

// warehouseSource is the data source of a repo without the sample: one with
// placeholders to replace.
const warehouseSource = `  warehouse:
    type: postgresql
    connection:
      host: localhost
      port: 5432
      user: analyst
      password: change-me
      dbname: analytics
    # settings:
    #   statement_timeout_ms: 300000
    #   require_ssl: true
`

// sampleSource is the sample's data source: its DuckDB file, read-only and
// with no access outside it. It has no credentials.
const sampleSource = `  ecommerce_duckdb:
    type: duckdb
    connection:
      path: data/ecommerce.duckdb?access_mode=read_only&enable_external_access=false
`

// contextSourcesTemplate points anfra ingest at the repo's own AML, for
// anfra search. The path is relative to the config directory.
const contextSourcesTemplate = `# What anfra ingest reads into the local catalog that anfra search searches.
sources:
  - name: aml
    type: aml
    path: ..
`

// initLayout is the folders a repo keeps its files in: AML models and
// datasets, and Data Apps. Each gets a .gitkeep, so Git keeps it while empty.
var initLayout = []string{"models", "datasets", "apps"}

// The sample project: the latest main of its public repo, so a fix to it
// reaches every anfra version without a release. Only these files are taken
// from its archive; anything else in it, links included, is ignored.
var (
	sampleRepo  = "holistics/anfra-sample-ecommerce"
	sampleURL   = "https://github.com/" + sampleRepo + "/archive/refs/heads/main.tar.gz"
	sampleFiles = []string{
		"models/ecommerce/*.model.aml",
		"datasets/ecommerce/*.dataset.aml",
		"data/ecommerce.duckdb",
		"wiki/ecommerce/*.md",
		"AGENTS.md",
	}
)

// sampleMaxBytes caps the sample's archive, and its files together: it is a
// few hundred KB, so more is not the sample.
const sampleMaxBytes = 32 << 20

const sampleQuestion = "Include the e-commerce sample project?"

// sampleOffer is what init says when nobody chose about the sample: no
// terminal to ask on, and neither --with-sample nor --yes. Whoever ran it, often
// an agent, is to ask its user.
const sampleOffer = `
Sample available: an e-commerce project (6 models, a dataset with 8 metrics,
a small DuckDB file, business definitions) to try anfra on right away.
Not added: nobody was asked. If you are an agent, ask the user whether to
include it. If they agree, run:
  anfra init%s --with-sample
`

// newInitCmd sets up an anfra repo. It is a CLI command only, never an op: it
// writes files on this machine, which a server must not do for its callers.
func newInitCmd() *cobra.Command {
	var withSample, yes bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Set up an anfra repo: its data source config, and folders for models, datasets and Data Apps",
		Long: "Set up an anfra repo in dir (the current directory by default, created if missing):\n\n" +
			"  .anfra/data_sources.yml          the data sources to query, with credentials (git-ignored)\n" +
			"  .anfra/data_sources.yml.example  the same, without credentials, to commit\n" +
			"  .anfra/context_sources.yml       what anfra ingest reads for anfra search\n" +
			"  models/, datasets/, apps/        AML models, AML datasets, Data Apps\n" +
			"  .gitignore                       + .anfra/data_sources.yml\n\n" +
			"It never changes an existing file, so it is safe to run in a repo that has some of these already.\n\n" +
			"Sample project: --with-sample also adds an e-commerce sample (6 models, a dataset with 8 metrics,\n" +
			"a small DuckDB file, business definitions in wiki/ecommerce/ and an AGENTS.md), downloaded from\n" +
			"github.com/" + sampleRepo + ", and points data_sources.yml at its DuckDB file.\n" +
			"In a terminal, init asks whether to add it (default no). Without one it never asks and never adds it.\n\n" +
			"Agents: do not pass --with-sample on your own. Ask the user in chat whether to include the sample,\n" +
			"and run `anfra init --with-sample` only if they agree.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			sample, chosen := withSample, withSample || yes
			if !chosen && cliui.CanInteract(cmd.InOrStdin(), cmd.ErrOrStderr()) {
				ask, _ := stdinQuestions(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr())
				sample, chosen = publish.Confirm(ask, sampleQuestion)
			}
			return initRepo(cmd.Context(), dir, cmd.OutOrStdout(), initChoice{sample: sample, offer: !chosen})
		},
	}
	cmd.Flags().BoolVar(&withSample, "with-sample", false, "add the e-commerce sample project, downloaded from GitHub (agents: ask the user first)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "take every default without asking: no sample")
	return cmd
}

// initChoice is what init was told about the sample: add it, or, when nobody
// chose, offer it in the output.
type initChoice struct {
	sample, offer bool
}

// initStep makes one file of a repo, at path relative to it, unless it is
// there already; created says whether it did.
type initStep struct {
	path string
	do   func(path string) (created bool, err error)
}

// initRepo creates what is missing of an anfra repo in dir, and reports each
// item as created or kept. With the sample, it then adds the sample's files:
// the repo is set up first, so a failed download leaves it usable, and running
// init again adds what is still missing.
func initRepo(ctx context.Context, dir string, out io.Writer, choice initChoice) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	r := repo.Resolve(dir)
	config, err := filepath.Rel(dir, r.ConfigDir)
	if err != nil {
		return err
	}
	sources := filepath.ToSlash(filepath.Join(config, datasource.FileName))
	template := dataSourcesHeader + warehouseSource
	if choice.sample {
		template = dataSourcesHeader + sampleSource
	}

	steps := []initStep{
		{sources, writeNew([]byte(template), 0o600)},
		{sources + ".example", writeNew([]byte(template), 0o644)},
		{filepath.ToSlash(filepath.Join(config, "context_sources.yml")), writeNew([]byte(contextSourcesTemplate), 0o644)},
	}
	for _, d := range initLayout {
		steps = append(steps, initStep{d + "/.gitkeep", writeNew(nil, 0o644)})
	}
	steps = append(steps, initStep{".gitignore", ignoreLine(sources)})
	if err := runSteps(dir, out, steps); err != nil {
		return err
	}

	if !choice.sample {
		fmt.Fprintf(out, "\nNext: put your database in %s, write models and datasets, then run anfra validate.\n", sources)
		if choice.offer {
			arg := ""
			if filepath.Clean(dir) != "." {
				arg = " " + dir
			}
			fmt.Fprintf(out, sampleOffer, arg)
		}
		return nil
	}

	commit, files, err := fetchSample(ctx)
	if err != nil {
		return fmt.Errorf("add the sample project: %w\nThe repo is set up without it. To retry, run: anfra init %s--with-sample", err, retryArg(dir))
	}
	fmt.Fprintf(out, "\nSample project, from %s@%s:\n", sampleRepo, commit)
	steps = steps[:0]
	for _, f := range files {
		steps = append(steps, initStep{f.path, writeNew(f.content, 0o644)})
	}
	if err := runSteps(dir, out, steps); err != nil {
		return err
	}
	if have, err := datasource.Load(r.ConfigDir); err != nil || have["ecommerce_duckdb"].DBType == "" {
		fmt.Fprintf(out, "\n%s has no ecommerce_duckdb data source, which the sample queries. Add it under data_sources:\n%s", sources, sampleSource)
	}
	// anfra reads the repo from the current directory, so these run from dir.
	fmt.Fprintln(out, "\nNext, try the sample:")
	if filepath.Clean(dir) != "." {
		fmt.Fprintf(out, "  cd %s\n", dir)
	}
	fmt.Fprint(out, "  anfra validate\n"+
		"  anfra show ecommerce\n"+
		"  anfra query -d ecommerce 'products | group(products.category) | select(products.category, revenue)'\n")
	for _, f := range files {
		if strings.HasPrefix(f.path, "wiki/ecommerce/") {
			fmt.Fprintln(out, "wiki/ecommerce/ has the sample's business rules: read it before writing queries.")
			break
		}
	}
	return nil
}

func retryArg(dir string) string {
	if filepath.Clean(dir) == "." {
		return ""
	}
	return dir + " "
}

// runSteps runs each step that is not done yet, and reports it as created or kept.
func runSteps(dir string, out io.Writer, steps []initStep) error {
	for _, s := range steps {
		created, err := s.do(filepath.Join(dir, filepath.FromSlash(s.path)))
		if err != nil {
			return fmt.Errorf("init %s: %w", s.path, err)
		}
		status := "kept   "
		if created {
			status = "created"
		}
		fmt.Fprintf(out, "  %s %s\n", status, s.path)
	}
	return nil
}

// sampleFile is one file of the sample, at its path in a repo.
type sampleFile struct {
	path    string
	content []byte
}

// fetchSample downloads the sample's archive and returns its files, all read
// before any is written, so a broken download writes nothing; commit is the
// commit GitHub archived, or "main" when the archive does not say.
func fetchSample(ctx context.Context) (commit string, files []sampleFile, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sampleURL, nil)
	if err != nil {
		return "", nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download %s: %s", sampleURL, res.Status)
	}
	gz, err := gzip.NewReader(io.LimitReader(res.Body, sampleMaxBytes))
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", sampleURL, err)
	}
	commit, left := "main", int64(sampleMaxBytes)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", sampleURL, err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			if c := h.PAXRecords["comment"]; len(c) >= 7 {
				commit = c[:7]
			}
			continue
		}
		// GitHub puts every file under one top folder, <repo>-<branch>/.
		_, name, ok := strings.Cut(h.Name, "/")
		if !ok || h.Typeflag != tar.TypeReg || !isSampleFile(name) {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(tr, left+1))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", sampleURL, err)
		}
		if left -= int64(len(content)); left < 0 {
			return "", nil, fmt.Errorf("%s is larger than %d MB, which the sample is not", sampleURL, sampleMaxBytes>>20)
		}
		files = append(files, sampleFile{name, content})
	}
	if !hasSampleData(files) {
		return "", nil, fmt.Errorf("%s has no data/ecommerce.duckdb", sampleURL)
	}
	return commit, files, nil
}

// isSampleFile says whether name, a path in the archive, is one the sample
// adds. The patterns' * matches no "/", so nothing outside their folders does.
func isSampleFile(name string) bool {
	if path.Clean(name) != name {
		return false
	}
	for _, pattern := range sampleFiles {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func hasSampleData(files []sampleFile) bool {
	for _, f := range files {
		if f.path == "data/ecommerce.duckdb" {
			return true
		}
	}
	return false
}

// writeNew writes content to a file that does not exist yet, creating its
// folder; an existing file is left as it is.
func writeNew(content []byte, perm fs.FileMode) func(path string) (bool, error) {
	return func(path string) (bool, error) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) //nolint:gosec // G304: a path under the repo being set up
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if _, err := f.Write(content); err != nil {
			_ = f.Close()
			return false, err
		}
		return true, f.Close()
	}
}

// ignoreLine adds pattern to a .gitignore, creating it, unless a line already
// is that pattern. Reported as created when it adds the line.
func ignoreLine(pattern string) func(path string) (bool, error) {
	return func(path string) (bool, error) {
		existing, err := os.ReadFile(path) //nolint:gosec // G304: the .gitignore of the repo being set up
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		for line := range strings.SplitSeq(string(existing), "\n") {
			if strings.TrimSpace(line) == pattern || strings.TrimSpace(line) == "/"+pattern {
				return false, nil
			}
		}
		var add bytes.Buffer
		if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
			add.WriteString("\n")
		}
		add.WriteString("# anfra: data source credentials\n")
		add.WriteString(pattern)
		add.WriteString("\n")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644) //nolint:gosec // G304: as above
		if err != nil {
			return false, err
		}
		if _, err := f.Write(add.Bytes()); err != nil {
			_ = f.Close()
			return false, err
		}
		return true, f.Close()
	}
}
