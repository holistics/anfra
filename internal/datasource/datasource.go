// Package datasource reads a project's data_sources.yml. The host owns this
// (it holds connection config / credentials and runs queries); only the
// name->dbtype mapping is ever passed to the sidecar for SQL-dialect selection,
// so secrets never cross the IPC boundary.
package datasource

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the data source manifest, inside the project's config dir
// (project.Project.ConfigDir()).
const FileName = "data_sources.yml"

// DataSource describes one data source. Name/DBType are what the sidecar needs
// to compile SQL (dialect). Connection holds the host-side connection config
// (passed to canal-query as dbconfig) and is never serialized to the sidecar.
type DataSource struct {
	Name       string         `json:"name"`
	DBType     string         `json:"dbtype"`
	Connection map[string]any `json:"-"`
}

// first-cut schema (connection keys map straight to canal's dbconfig):
//
//	data_sources:
//	  demo:
//	    type: postgresql
//	    connection:
//	      host: localhost
//	      port: 5432
//	      user: anfra
//	      password: anfra
//	      dbname: anfra
type fileSchema struct {
	DataSources map[string]struct {
		Type       string         `yaml:"type"`
		Connection map[string]any `yaml:"connection"`
	} `yaml:"data_sources"`
}

// Load reads <configDir>/data_sources.yml into a name->DataSource map.
func Load(configDir string) (map[string]DataSource, error) {
	path := filepath.Join(configDir, FileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no data source manifest at %s — define your data sources, e.g.\n\ndata_sources:\n  <name>:\n    type: postgresql", path)
		}
		return nil, err
	}

	var parsed fileSchema
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	out := make(map[string]DataSource, len(parsed.DataSources))
	for name, ds := range parsed.DataSources {
		if ds.Type == "" {
			return nil, fmt.Errorf("data source %q in %s is missing `type`", name, path)
		}
		out[name] = DataSource{Name: name, DBType: ds.Type, Connection: ds.Connection}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no data sources", path)
	}
	return out, nil
}
