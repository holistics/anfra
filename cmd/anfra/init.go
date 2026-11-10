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

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/publish"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
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
      path: example-data/ecommerce.duckdb?access_mode=read_only&enable_external_access=false
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
		"apps/ecommerce/*.html",
		sampleData,
		"wiki/ecommerce/*.md",
	}
)

// sampleMaxBytes caps the sample's archive, and its files together: it is a
// few hundred KB, so more is not the sample.
const sampleMaxBytes = 32 << 20

const sampleQuestion = "Include the e-commerce sample project?"

// sampleData is the sample's DuckDB file, committed with the rest of the repo.
const sampleData = "example-data/ecommerce.duckdb"

// newInitCmd sets up an anfra repo. It is a CLI command only, never an op: it
// writes files on this machine, which a server must not do for its callers.
func newInitCmd() *cobra.Command {
	var dataSourcesOnly, example, yes bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Set up an anfra repo: its data source config, and folders for models, datasets and Data Apps",
		Long: "Set up an anfra repo in dir (the current directory by default, created if missing):\n\n" +
			"  .anfra/data_sources.yml          the data sources to query, with credentials (git-ignored)\n" +
			"  .anfra/data_sources.yml.example  the same, without credentials, to commit\n" +
			"  .anfra/context_sources.yml       what anfra ingest reads for anfra search\n" +
			"  models/, datasets/, apps/        AML models, AML datasets, Data Apps\n" +
			"  .gitignore                       + .anfra/data_sources.yml\n" +
			"  AGENTS.md                        + the anfra block, telling coding agents to use the anfra skills\n" +
			"                                     (and @AGENTS.md in an existing CLAUDE.md)\n\n" +
			"It changes no existing file beyond adding those, so it is safe to run in a repo that has some of these already.\n" +
			"data_sources.yml is made from data_sources.yml.example when the repo has one, as a clone of it does.\n" +
			"--data-sources sets up only the data source files: data_sources.yml, its example, and the .gitignore line.\n\n" +
			"Sample project: --example also adds an e-commerce sample from github.com/" + sampleRepo + ":\n" +
			"a Data App, its semantic layer, and a small DuckDB file.\n" +
			"In a terminal, init asks whether to add it (default no). Without one, it never asks.\n" +
			"Agents: ask the user in chat before passing --example.\n\n" +
			"Its result opens with message, the outcome; then dir, the repo's folder, and files, each with\n" +
			"whether init changed it (created it, or added to it); and hints, last, what to do next.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if dataSourcesOnly && example {
				return flagViolated("--example", "--example can't be combined with --data-sources, because the sample needs a whole repo. Drop one of them.")
			}
			opts := initOptions{dataSourcesOnly: dataSourcesOnly, sample: example}
			// The sample is a choice for a whole repo only. Nobody chose: ask in a
			// terminal, else offer it in the hints.
			if !dataSourcesOnly && !example && !yes {
				opts.offer = true
				if cliui.CanInteract(cmd.InOrStdin(), cmd.ErrOrStderr()) {
					ask, _ := stdinQuestions(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr())
					var chosen bool
					opts.sample, chosen = publish.Confirm(ask, sampleQuestion)
					opts.offer = !chosen
				}
			}
			res, err := initRepo(cmd.Context(), dir, opts)
			if err != nil {
				return apperr.TranslateWith(err, clierr.CommandFailed, any(res))
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().BoolVar(&dataSourcesOnly, "data-sources", false, "set up only the data source files: data_sources.yml, its example, and the .gitignore line")
	cmd.Flags().BoolVar(&example, "example", false, "add the e-commerce sample project, downloaded from GitHub (agents: ask the user first)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "take every default without asking: no sample")
	markPrettyOutput(cmd)
	resultIs[initResult](cmd)
	return cmd
}

// initResult is init's result: the repo it set up, and each of its files.
type initResult struct {
	Message string     `json:"message" doc:"the outcome, in a sentence"`
	Dir     string     `json:"dir" doc:"the repo's folder, absolute"`
	Files   []initFile `json:"files" doc:"the repo's files init sets up"`
	Hints   []string   `json:"hints,omitempty" doc:"what to do next, if any"`
}

// initFile is a file of the repo, by its path in it, and whether init changed
// it: created it, or added its line or block.
type initFile struct {
	Path    string `json:"path" doc:"the file's path in the repo"`
	Changed bool   `json:"changed" doc:"whether init created it, or added to it"`
}

// initOptions is what init sets up: the data source files only, or a whole
// repo, with the sample or, when nobody chose, offering it in the hints.
type initOptions struct {
	dataSourcesOnly, sample, offer bool
}

// initStep makes one file of a repo, at path relative to it, unless it is
// there already; created says whether it did.
type initStep struct {
	path string
	do   func(path string) (created bool, err error)
}

// initRepo creates what is missing of an anfra repo in dir, and says what it
// did to each file; on a failure, as far as it got. With the sample, it adds
// the sample's files after the repo's own, so a failed download leaves the
// repo usable, and running init again adds what is still missing.
func initRepo(ctx context.Context, dir string, opts initOptions) (initResult, error) {
	res := initResult{Files: []initFile{}}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return res, err
	}
	res.Dir = abs
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, clierr.Precondition("Can't create %s: %v.", dir, err)
	}
	r := repo.Resolve(dir)
	config, err := filepath.Rel(dir, r.ConfigDir)
	if err != nil {
		return res, err
	}
	sources := filepath.ToSlash(filepath.Join(config, datasource.FileName))
	template := dataSourcesHeader + warehouseSource
	if opts.sample {
		template = dataSourcesHeader + sampleSource
	}

	// A clone has the committed example but not data_sources.yml, which holds
	// credentials and is git-ignored: the example is the team's data sources.
	dataSources := []byte(template)
	if example, err := os.ReadFile(filepath.Join(dir, sources+".example")); err == nil { //nolint:gosec // G304: a path under the repo being set up
		dataSources = example
	}
	steps := []initStep{
		{sources, writeNew(dataSources, 0o600)},
		{sources + ".example", writeNew([]byte(template), 0o644)},
		{".gitignore", ignoreLine(sources)},
	}
	if !opts.dataSourcesOnly {
		steps = append(steps, initStep{filepath.ToSlash(filepath.Join(config, "context_sources.yml")), writeNew([]byte(contextSourcesTemplate), 0o644)})
		for _, d := range initLayout {
			steps = append(steps, initStep{d + "/.gitkeep", writeNew(nil, 0o644)})
		}
		// The block agent setup writes too: what tells agents to use the skills.
		steps = append(steps, initStep{"AGENTS.md", func(string) (bool, error) {
			wroteBlock, wroteImport, err := agents.WriteBlock(dir)
			return wroteBlock || wroteImport, err
		}})
	}
	if err := runSteps(dir, steps, &res); err != nil {
		return res, err
	}

	commit := ""
	if opts.sample {
		var files []sampleFile
		commit, files, err = fetchSample(ctx)
		if err != nil {
			res.Hints = []string{fmt.Sprintf("The repo is set up without the sample. To try again, run `anfra init %s--example`.", retryArg(dir))}
			return res, err
		}
		steps = steps[:0]
		for _, f := range files {
			steps = append(steps, initStep{f.path, writeNew(f.content, 0o644)})
		}
		if err := runSteps(dir, steps, &res); err != nil {
			return res, err
		}
	}

	changed := 0
	for _, f := range res.Files {
		if f.Changed {
			changed++
		}
	}
	// The folder as the user named it: dir says where it is.
	where := "this folder"
	if filepath.Clean(dir) != "." {
		where = dir
	}
	what := "an anfra repo"
	switch {
	case opts.dataSourcesOnly:
		what = "the data source files"
	case opts.sample:
		what = "an anfra repo with the e-commerce sample"
	}
	switch kept := len(res.Files) - changed; {
	case changed == 0 && opts.dataSourcesOnly:
		res.Message = fmt.Sprintf("Nothing changed: %s has its data source files already.", where)
	case changed == 0 && opts.sample:
		res.Message = fmt.Sprintf("Nothing changed: %s has an anfra repo and the e-commerce sample already.", where)
	case changed == 0:
		res.Message = fmt.Sprintf("Nothing changed: %s is an anfra repo already.", where)
	case kept == 0:
		res.Message = fmt.Sprintf("Set up %s in %s, with %d files.", what, where, changed)
	default:
		res.Message = fmt.Sprintf("Set up %s in %s: changed %d files, kept %d already there.", what, where, changed, kept)
	}
	switch {
	case opts.dataSourcesOnly:
		res.Hints = []string{fmt.Sprintf("Fill in your database's connection in %s.", sources)}
	case opts.sample:
		res.Message += fmt.Sprintf(" The sample is %s@%s.", sampleRepo, commit)
		res.Hints = sampleHints(dir, r.ConfigDir, sources)
	default:
		res.Hints = []string{fmt.Sprintf("Put your database in %s, write models and datasets, then run `anfra validate`.", sources)}
		if opts.offer {
			res.Hints = append(res.Hints, fmt.Sprintf("An e-commerce sample project is available, with 6 models, a dataset with 8 metrics, "+
				"a small DuckDB file, a Data App, and business definitions. It wasn't added, because nobody was asked. "+
				"If you're an agent, ask the user whether to add it. If they agree, run `anfra init %s--example`.", retryArg(dir)))
		}
	}
	return res, nil
}

// runSteps runs each step that is not done yet, and adds its file to res.
func runSteps(dir string, steps []initStep, res *initResult) error {
	for _, s := range steps {
		created, err := s.do(filepath.Join(dir, filepath.FromSlash(s.path)))
		if err != nil {
			return clierr.Precondition("Can't set up %s: %v.", s.path, err)
		}
		res.Files = append(res.Files, initFile{Path: s.path, Changed: created})
	}
	return nil
}

// sampleHints are the next steps after adding the sample. anfra reads the
// repo from the current directory, so they run from dir.
func sampleHints(dir, configDir, sources string) []string {
	var hints []string
	if have, err := datasource.Load(configDir); err != nil || have["ecommerce_duckdb"].DBType == "" {
		hints = append(hints, fmt.Sprintf("%s was there already, so it has no ecommerce_duckdb data source, which the sample queries. Add this under data_sources:\n%s", sources, sampleSource))
	}
	if filepath.Clean(dir) != "." {
		hints = append(hints, fmt.Sprintf("Run the next commands from the repo's folder: `cd %s`.", dir))
	}
	return append(hints,
		"Check the sample with `anfra validate`, then see its dataset with `anfra show ecommerce`.",
		"Query it: `anfra query -d ecommerce 'products | group(products.category) | select(products.category, revenue)'`.",
		"Open the sample's Data App with `anfra serve`.",
		"Read wiki/ecommerce/ before writing queries. It holds the sample's business rules.",
	)
}

func retryArg(dir string) string {
	if filepath.Clean(dir) == "." {
		return ""
	}
	return dir + " "
}

// sampleFile is one file of the sample, at its path in a repo.
type sampleFile struct {
	path    string
	content []byte
}

// downloadSampleStep is downloading the sample: public, so that a failure in
// it says so in its context, and its message is the failure's own.
var downloadSampleStep = appstep.Define(clierr.NS, "download_sample", appstep.Public("download the sample project"))

// fetchSample downloads the sample's archive and returns its files, all read
// before any is written, so a broken download writes nothing; commit is the
// commit GitHub archived, or "main" when the archive does not say. A failure is
// GitHub's, unavailable.
func fetchSample(ctx context.Context) (commit string, files []sampleFile, err error) {
	ctx, span := apptracing.Start(ctx, downloadSampleStep)
	defer span.End(&err)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sampleURL, nil)
	if err != nil {
		return "", nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, clierr.Unreachablef(clierr.GitHub, err, "GitHub didn't answer: %v.", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", nil, clierr.Unreachablef(clierr.GitHub, nil, "GitHub returned %s for %s.", res.Status, sampleURL)
	}
	gz, err := gzip.NewReader(io.LimitReader(res.Body, sampleMaxBytes))
	if err != nil {
		return "", nil, unreadableSample(err)
	}
	defer func() { _ = gz.Close() }()
	commit, left := "main", int64(sampleMaxBytes)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, unreadableSample(err)
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
			return "", nil, unreadableSample(err)
		}
		if left -= int64(len(content)); left < 0 {
			return "", nil, clierr.Unreachablef(clierr.GitHub, nil, "The sample's archive from GitHub holds more than %d MB of files, far more than the sample, so anfra stopped reading it.", sampleMaxBytes>>20)
		}
		files = append(files, sampleFile{name, content})
	}
	if !hasSampleData(files) {
		return "", nil, clierr.Unreachablef(clierr.GitHub, nil, "The sample's archive from GitHub has no %s.", sampleData)
	}
	return commit, files, nil
}

// unreadableSample is the sample's archive failing to read: cut off, or not a
// gzipped tar.
func unreadableSample(err error) error {
	return clierr.Unreachablef(clierr.GitHub, err, "The sample's archive from GitHub can't be read: %v.", err)
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
		if f.path == sampleData {
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
// is that pattern. Reported as created when it adds the line. A .gitignore
// that is not a regular file is refused: Git does not follow a symlinked
// one, so the line would not ignore anything, and writing through the link
// would change a file outside the repo.
func ignoreLine(pattern string) func(path string) (bool, error) {
	return func(path string) (bool, error) {
		if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
			return false, fmt.Errorf("it's a symlink, which Git doesn't follow. Add %s to the repo's ignore rules yourself", pattern)
		}
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
