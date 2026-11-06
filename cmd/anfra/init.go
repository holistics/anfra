package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
)

// dataSourcesTemplate is a new repo's data_sources.yml, and its committed
// example: one data source, with placeholders to replace.
const dataSourcesTemplate = `# The data sources this repo's models query, by name: a model's or dataset's
# data_source_name is one of these names. Each connection is passed as-is to
# the database driver: its keys depend on the type.
#
# data_sources.yml holds credentials, so it is git-ignored; this repo commits
# data_sources.yml.example instead, without them.
#
# settings, optional, are how the query engine connects and queries: timeouts,
# requiring SSL and the like. Without them, the engine's defaults.
data_sources:
  warehouse:
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

// newInitCmd sets up an anfra repo. It is a CLI command only, never an op: it
// writes files on this machine, which a server must not do for its callers.
func newInitCmd() *cobra.Command {
	var dataSourcesOnly bool
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
			"Its result opens with message, the outcome; then dir, the repo's folder, and files, each with\n" +
			"whether init changed it (created it, or added to it); and hints, last, what to do next.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			res, err := initRepo(dir, dataSourcesOnly)
			if err != nil {
				return commandFailedWith(err.Error(), res)
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().BoolVar(&dataSourcesOnly, "data-sources", false, "set up only the data source files: data_sources.yml, its example, and the .gitignore line")
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

// initStep makes one file of a repo, at path relative to it, unless it is
// there already; created says whether it did.
type initStep struct {
	path string
	do   func(path string) (created bool, err error)
}

// initRepo creates what is missing of an anfra repo in dir, and says what it
// did to each file; on a failure, as far as it got. dataSourcesOnly limits it
// to the data source files.
func initRepo(dir string, dataSourcesOnly bool) (initResult, error) {
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

	// A clone has the committed example but not data_sources.yml, which holds
	// credentials and is git-ignored: the example is the team's data sources.
	dataSources := []byte(dataSourcesTemplate)
	if example, err := os.ReadFile(filepath.Join(dir, sources+".example")); err == nil { //nolint:gosec // G304: a path under the repo being set up
		dataSources = example
	}
	steps := []initStep{
		{sources, writeNew(dataSources, 0o600)},
		{sources + ".example", writeNew([]byte(dataSourcesTemplate), 0o644)},
		{".gitignore", ignoreLine(sources)},
	}
	if !dataSourcesOnly {
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

	changed := 0
	for _, s := range steps {
		created, err := s.do(filepath.Join(dir, s.path))
		if err != nil {
			return res, clierr.Precondition("Can't set up %s: %v.", s.path, err)
		}
		res.Files = append(res.Files, initFile{Path: s.path, Changed: created})
		if created {
			changed++
		}
	}
	// The folder as the user named it: dir says where it is.
	where := "this folder"
	if filepath.Clean(dir) != "." {
		where = dir
	}
	what := "an anfra repo"
	if dataSourcesOnly {
		what = "the data source files"
	}
	switch kept := len(res.Files) - changed; {
	case changed == 0 && dataSourcesOnly:
		res.Message = fmt.Sprintf("Nothing changed: %s has its data source files already.", where)
	case changed == 0:
		res.Message = fmt.Sprintf("Nothing changed: %s is an anfra repo already.", where)
	case kept == 0:
		res.Message = fmt.Sprintf("Set up %s in %s, with %d files.", what, where, changed)
	default:
		res.Message = fmt.Sprintf("Set up %s in %s: changed %d files, kept %d already there.", what, where, changed, kept)
	}
	res.Hints = []string{fmt.Sprintf("Put your database in %s, write models and datasets, then run `anfra validate`.", sources)}
	if dataSourcesOnly {
		res.Hints = []string{fmt.Sprintf("Fill in your database's connection in %s.", sources)}
	}
	return res, nil
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
