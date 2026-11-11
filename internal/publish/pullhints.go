package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/internal/repo"
)

// dataSourceName finds the data source a model or dataset names, in its AML.
// A text scan, not a compile: pull runs no sidecar, and a hint needs no more.
var dataSourceName = regexp.MustCompile(`data_source_name\s*:\s*['"]([^'"]+)['"]`)

// missingDataSources is a hint naming the data sources the repo's AML uses
// that .anfra/data_sources.yml doesn't have, each with its type on the
// org when the org lets this member list them; "" when none is missing.
func missingDataSources(ctx context.Context, dir string, client *cloudapi.Client) string {
	used := map[string]bool{}
	files, err := workdir.Files(dir)
	if err != nil {
		return ""
	}
	for _, f := range files {
		if !strings.EqualFold(filepath.Ext(f), ".aml") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		for _, m := range dataSourceName.FindAllSubmatch(content, -1) {
			used[string(m[1])] = true
		}
	}
	configDir := filepath.Join(dir, repo.ConfigDirName)
	var have datasource.Set
	if _, err := os.Stat(filepath.Join(configDir, datasource.FileName)); err == nil {
		if have, err = datasource.Load(configDir); err != nil {
			return "" // a file that doesn't load says so on every command
		}
	}
	var missing []string
	for name := range used {
		if _, ok := have[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	slices.Sort(missing)
	types := map[string]string{}
	if list, err := client.ListDataSources(ctx); err == nil {
		for _, ds := range list {
			types[ds.Name] = ds.Type
		}
	}
	named := make([]string, len(missing))
	for i, name := range missing {
		named[i] = name
		if t := types[name]; t != "" {
			named[i] += " (" + t + ")"
		}
	}
	n := len(missing)
	return fmt.Sprintf("This repo uses %s that %s in %s yet: %s. Add %s, with %s.",
		pick(n, "a data source", "data sources"), pick(n, "isn't", "aren't"),
		filepath.ToSlash(filepath.Join(repo.ConfigDirName, datasource.FileName)), joinPaths(named),
		itThem(n), pick(n, "its connection", "their connections"))
}

// uncommittedHint is a hint for a pull into a git working tree that had
// uncommitted changes: its changes are mixed with them in `git diff`.
func uncommittedHint() string {
	return "This folder had uncommitted changes, so `git diff` shows them mixed with the pull's. To keep them apart, run `anfra pull --abort`, commit, then pull again."
}
