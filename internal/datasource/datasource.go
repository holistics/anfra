// Package datasource reads a repo's data_sources.yml. The host owns this
// (it holds connection config / credentials and runs queries); only the
// name->dbtype mapping is ever passed to the sidecar for SQL-dialect selection,
// so secrets never cross the IPC boundary.
package datasource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the data source manifest, inside the repo's config dir
// (repo.Repo.ConfigDir()).
const FileName = "data_sources.yml"

// DataSource describes one data source. Name/DBType are what the sidecar needs
// to compile SQL (dialect). Connection holds the host-side connection config
// (passed to canal-query as dbconfig) and is never serialized to the sidecar.
// Settings are how canal-query connects and queries (timeouts, SSL, the
// type's own options), passed to it as dbsetting; none leaves each at canal's
// default.
type DataSource struct {
	Name       string         `json:"name"`
	DBType     string         `json:"dbtype"`
	Connection map[string]any `json:"-"`
	Settings   map[string]any `json:"-"`
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
//	    settings:
//	      statement_timeout_ms: 60000
//	      require_ssl: true
type fileSchema struct {
	DataSources map[string]struct {
		Type       string         `yaml:"type"`
		Connection map[string]any `yaml:"connection"`
		Settings   map[string]any `yaml:"settings"`
	} `yaml:"data_sources"`
}

// Dialect is a data source as compiling needs it: its name and its type (the SQL
// dialect). No connection.
type Dialect struct {
	Name   string
	DBType string
}

// DataSources is how the engine reaches a repo's data sources, asking for no more
// than a command needs: every data source's dialect, to compile (the engine
// cannot know which one a dataset uses until it is compiled); one connection, to
// run a query on the data source it targets. validate asks for neither.
//
// A host serving a managed repo implements it over its own store: Dialects is
// metadata, and Connection is where one data source's connection is retrieved.
// A local repo's is its data_sources.yml (Load).
type DataSources interface {
	// Dialects is the data sources the caller may compile against, by name and
	// type.
	Dialects(ctx context.Context) ([]Dialect, error)
	// Connection is the data source by name, connection included, for SQL that
	// reads the data as mode says. A host may refuse a mode it did not grant the
	// call on that data source: a second gate, behind its authorization and the
	// invocation's DataPerms, which must be right on their own. A name the repo
	// has no data source by is ErrUnknown; any other error is the host's, and
	// reaches the caller as it is.
	Connection(ctx context.Context, name string, mode DataPermMode) (DataSource, error)
}

// DataPermMode is how the SQL a connection carries reads the data: a fact about
// the statement, which the engine states and a host may check, never a
// decision. Every statement the engine runs is one of the two (internal/query).
// The zero value is no mode, which a host refuses.
type DataPermMode int

const (
	// DataPermModeGoverned: SQL the engine compiled from a dataset, with the
	// invocation's DataPerms in it.
	DataPermModeGoverned DataPermMode = iota + 1
	// DataPermModeUngoverned: SQL as written, which nothing applies DataPerms
	// to; run only for an invocation whose DataPerms are Ungoverned.
	DataPermModeUngoverned
)

// String is the mode as logs name it.
func (m DataPermMode) String() string {
	switch m {
	case DataPermModeGoverned:
		return "governed"
	case DataPermModeUngoverned:
		return "ungoverned"
	}
	return fmt.Sprintf("DataPermMode(%d)", int(m))
}

// ErrUnknown is Connection's answer for a name the repo has no data source by.
var ErrUnknown = errors.New("no such data source")

// Set is data sources held in memory, by name: what data_sources.yml loads into,
// or what a host that already holds them all passes. It is a DataSources.
type Set map[string]DataSource

// Dialects is every data source in s, sorted by name.
func (s Set) Dialects(context.Context) ([]Dialect, error) {
	out := make([]Dialect, 0, len(s))
	for _, ds := range s {
		out = append(out, Dialect{Name: ds.Name, DBType: ds.DBType})
	}
	slices.SortFunc(out, func(a, b Dialect) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Connection is the data source named name in s, whatever the mode: a Set
// holds no grants.
func (s Set) Connection(_ context.Context, name string, _ DataPermMode) (DataSource, error) {
	ds, ok := s[name]
	if !ok {
		return DataSource{}, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
	return ds, nil
}

// Load reads <configDir>/data_sources.yml into a Set.
func Load(configDir string) (Set, error) {
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

	out := make(Set, len(parsed.DataSources))
	for name, ds := range parsed.DataSources {
		if ds.Type == "" {
			return nil, fmt.Errorf("data source %q in %s is missing `type`", name, path)
		}
		out[name] = DataSource{Name: name, DBType: ds.Type, Connection: ds.Connection, Settings: ds.Settings}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no data sources", path)
	}
	return out, nil
}
