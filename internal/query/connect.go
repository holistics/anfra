package query

import (
	"context"

	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
)

// Every statement the engine runs on a data source is one of two, and this file
// holds the one place either meets a connection (connect):
//
//   - governed SQL, compiled from a dataset with the invocation's DataPerms in it:
//     a Compile result, run by Stream;
//   - ungoverned SQL, as written, which nothing applies DataPerms to: an
//     UngovernedSQL, which only ConnectUngoverned makes, and only for an
//     invocation whose DataPerms are Ungoverned.
//
// Each asks for its connection in its own mode (datasource.DataPermMode), so a
// host can check the mode against what it granted: the mode is the statement's,
// never a call site's choice.

// CheckUngoverned refuses SQL as written for a governed caller: nothing can
// govern it.
func CheckUngoverned(perms dataperm.Set) error {
	if perms.Governed() {
		return apperr.New(errcode.DataPermsUnenforceable,
			"a SQL query runs only for a caller with ungoverned data permissions: no rule can be applied to it")
	}
	return nil
}

// UngovernedSQL is SQL as written, with the data source it runs on, connection
// included. Only ConnectUngoverned makes one.
type UngovernedSQL struct {
	ds  datasource.DataSource
	sql string
}

// ConnectUngoverned is sql, as written, on the data source named name: refused
// for a governed caller (CheckUngoverned), and connected in the ungoverned mode.
// A name the repo has no data source by is datasource.ErrUnknown.
func ConnectUngoverned(ctx context.Context, perms dataperm.Set, sources datasource.DataSources, name, sql string) (UngovernedSQL, error) {
	if err := CheckUngoverned(perms); err != nil {
		return UngovernedSQL{}, err
	}
	ds, err := connect(ctx, sources, name, datasource.DataPermModeUngoverned)
	if err != nil {
		return UngovernedSQL{}, err
	}
	return UngovernedSQL{ds: ds, sql: sql}, nil
}

// Stream runs s as it is: written in its data source's dialect, with its own
// LIMIT. bounds are as for the package's Stream.
func (s UngovernedSQL) Stream(ctx context.Context, canal *canalquery.Client, bounds ResultBounds, h canalquery.Handler) error {
	return stream(ctx, canal, s.ds, canalquery.Statement{SQL: s.sql, DBType: s.ds.DBType}, bounds, h)
}

// connect is the data source named name, connection included, asked for in the
// mode of the statement that will run on it: the engine's one call of
// DataSources.Connection.
func connect(ctx context.Context, sources datasource.DataSources, name string, mode datasource.DataPermMode) (datasource.DataSource, error) {
	return sources.Connection(ctx, name, mode)
}
