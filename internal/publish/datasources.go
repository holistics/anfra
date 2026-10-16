package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
)

// offerDataSourcesToOrg adds to the org the data sources this repo uses here
// (.anfra/data_sources.yml) and the org lacks, one at a time, each once the
// person agrees: asked at a terminal, or --create-data-sources without one. A
// connection, credentials included, is never sent unasked, not even to check it.
// Each is checked before it is created; one the server cannot reach is created
// only when the person chooses to. Nothing here stops the publish: a data source
// the org lacks fails only the queries on the models that use it.
//
// Never changed: a data source the org has, though it differs from this one;
// separate connections per environment are the point of keeping them out of the
// repo. A type that differs is said, since the two compile to different SQL.
//
// Never offered: a DuckDB one (localOnly), whose database is a file on this
// machine; it is said instead, with what to do.
func offerDataSourcesToOrg(ctx context.Context, c *cloudapi.Client, dir string, create bool, errOut io.Writer, ask Prompt) error {
	local, err := localDataSources(dir)
	if err != nil {
		fmt.Fprintf(errOut, "Not checking the org's data sources: %v\n", err)
		return nil
	}
	if len(local) == 0 {
		return nil
	}
	have, err := c.ListDataSources(ctx)
	if cloudapi.IsForbidden(err) {
		fmt.Fprintf(errOut, "Not checking the org's data sources: listing them needs permission to query data sources. "+
			"An org admin can check it has %s.\n", strings.Join(slices.Sorted(maps.Keys(local)), ", "))
		return nil
	}
	if err != nil {
		return err
	}
	byName := make(map[string]cloudapi.DataSource, len(have))
	for _, d := range have {
		byName[d.Name] = d
	}

	var missing []datasource.DataSource
	var skipped []string
	for _, name := range slices.Sorted(maps.Keys(local)) {
		l := local[name]
		if r, ok := byName[name]; ok {
			if r.Type != l.DBType {
				fmt.Fprintf(errOut, "Data source %s is %s here and %s on the org: the org's models compile to %s's SQL.\n", name, l.DBType, r.Type, r.Type)
			}
			continue
		}
		if localOnly[l.DBType] {
			fmt.Fprintf(errOut, "Warning: data source %s is %s, a file on this machine, which the org's server cannot read: not creating it there. "+
				"Load its data into a warehouse the server reaches, and add that to the org as %s.\n", name, l.DBType, name)
			skipped = append(skipped, name)
			continue
		}
		missing = append(missing, l)
	}

	for i, d := range missing {
		if !create {
			yes, asked := Confirm(ask, fmt.Sprintf("Data source %s is not available on the org yet. Would you like to create it? "+
				"This would send its connection, credentials included, to the server.", d.Name))
			if !asked {
				for _, rest := range missing[i:] {
					skipped = append(skipped, rest.Name)
				}
				fmt.Fprintln(errOut, "Not creating data sources without a terminal to ask: --create-data-sources does.")
				break
			}
			if !yes {
				skipped = append(skipped, d.Name)
				continue
			}
		}
		created, err := createDataSource(ctx, c, d, errOut, ask)
		if cloudapi.IsForbidden(err) {
			names := make([]string, 0, len(missing)-i)
			for _, rest := range missing[i:] {
				names = append(names, rest.Name)
			}
			fmt.Fprintf(errOut, "Only an org admin can add data sources: ask one to add %s.\n", strings.Join(names, ", "))
			return nil
		}
		if err != nil {
			return err
		}
		if !created {
			skipped = append(skipped, d.Name)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(errOut, "Queries on the models using %s fail until the org has them.\n", strings.Join(skipped, ", "))
	}
	return nil
}

// localOnly is the types whose database is on this machine, which an org's
// server never reads: anfra-cloud refuses them, as reading files on its own disk.
var localOnly = map[string]bool{"duckdb": true}

// createDataSource checks the server can reach d, then creates it, or, when it
// cannot, asks whether to create it anyway; created is false for one skipped. An
// error is the server's (forbidden, for one who may not add data sources). A
// server too old to take data sources' settings gets d without them, saying so.
func createDataSource(ctx context.Context, c *cloudapi.Client, d datasource.DataSource, errOut io.Writer, ask Prompt) (created bool, err error) {
	check, err := c.TestDataSource(ctx, cloudapi.TestDataSourceIn{Name: d.Name, Type: d.DBType, Connection: d.Connection, Settings: d.Settings})
	if len(d.Settings) > 0 && cloudapi.Unknown(err, "settings") {
		fmt.Fprintf(errOut, "Data source %s's settings are not sent: the org's server takes none, so it runs with the defaults.\n", d.Name)
		d.Settings = nil
		check, err = c.TestDataSource(ctx, cloudapi.TestDataSourceIn{Name: d.Name, Type: d.DBType, Connection: d.Connection})
	}
	if err != nil {
		return false, err
	}
	if !check.OK {
		fmt.Fprintf(errOut, "Data source %s is not reachable from the org's server: %s\n", d.Name, check.Message)
		choice, asked := ask("What would you like to do with it?", "create anyway", "skip")
		if !asked {
			fmt.Fprintf(errOut, "Skipping it: there is no terminal to choose.\n")
			return false, nil
		}
		if choice != 0 {
			return false, nil
		}
	}
	if _, err := c.CreateDataSource(ctx, cloudapi.CreateDataSourceIn{Name: d.Name, Type: d.DBType, Connection: d.Connection, Settings: d.Settings}); err != nil {
		if cloudapi.IsForbidden(err) {
			return false, err
		}
		fmt.Fprintf(errOut, "Data source %s was not created: %v\n", d.Name, err)
		return false, nil
	}
	fmt.Fprintf(errOut, "Created data source %s (%s) on the org.\n", d.Name, d.DBType)
	return true, nil
}

// localDataSources is the repo's data_sources.yml here, by name; none when it
// has no such file.
func localDataSources(dir string) (datasource.Set, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	configDir := repo.Resolve(abs).ConfigDir
	if _, err := os.Stat(filepath.Join(configDir, datasource.FileName)); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return datasource.Load(configDir)
}
