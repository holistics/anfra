package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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
data_sources:
  warehouse:
    type: postgresql
    connection:
      host: localhost
      port: 5432
      user: analyst
      password: change-me
      dbname: analytics
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
	return &cobra.Command{
		Use:   "init [dir]",
		Short: "Set up an anfra repo: its data source config, and folders for models, datasets and Data Apps",
		Long: "Set up an anfra repo in dir (the current directory by default, created if missing):\n\n" +
			"  .anfra/data_sources.yml          the data sources to query, with credentials (git-ignored)\n" +
			"  .anfra/data_sources.yml.example  the same, without credentials, to commit\n" +
			"  .anfra/context_sources.yml       what anfra ingest reads for anfra search\n" +
			"  models/, datasets/, apps/        AML models, AML datasets, Data Apps\n" +
			"  .gitignore                       + .anfra/data_sources.yml\n\n" +
			"It never changes an existing file, so it is safe to run in a repo that has some of these already.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return initRepo(dir, cmd.OutOrStdout())
		},
	}
}

// initStep makes one file of a repo, at path relative to it, unless it is
// there already; created says whether it did.
type initStep struct {
	path string
	do   func(path string) (created bool, err error)
}

// initRepo creates what is missing of an anfra repo in dir, and reports each
// item as created or kept.
func initRepo(dir string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	r := repo.Resolve(dir)
	config, err := filepath.Rel(dir, r.ConfigDir)
	if err != nil {
		return err
	}
	sources := filepath.ToSlash(filepath.Join(config, datasource.FileName))

	steps := []initStep{
		{sources, writeNew([]byte(dataSourcesTemplate), 0o600)},
		{sources + ".example", writeNew([]byte(dataSourcesTemplate), 0o644)},
		{filepath.ToSlash(filepath.Join(config, "context_sources.yml")), writeNew([]byte(contextSourcesTemplate), 0o644)},
	}
	for _, d := range initLayout {
		steps = append(steps, initStep{d + "/.gitkeep", writeNew(nil, 0o644)})
	}
	steps = append(steps, initStep{".gitignore", ignoreLine(sources)})

	for _, s := range steps {
		created, err := s.do(filepath.Join(dir, s.path))
		if err != nil {
			return fmt.Errorf("init %s: %w", s.path, err)
		}
		status := "kept   "
		if created {
			status = "created"
		}
		fmt.Fprintf(out, "  %s %s\n", status, s.path)
	}
	fmt.Fprintf(out, "\nNext: put your database in %s, write models and datasets, then run anfra validate.\n", sources)
	return nil
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
