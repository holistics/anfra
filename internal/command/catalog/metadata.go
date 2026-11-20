package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/metadata/postgres"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/logkit"
)

var eventMetadataSkipped = logkit.DefineEvent("catalog.postgres_skipped", "Postgres metadata could not be loaded; ingestion continued with the other sources.", "error")

func loadMetadata(ctx context.Context, cc command.CommandContext) (*anfranode.CatalogMetadata, error) {
	sources, err := cc.ResolveDataSources()
	if err != nil {
		return nil, err
	}

	dialects, err := sources.Dialects(ctx)
	if err != nil {
		return nil, err
	}

	for _, dialect := range dialects {
		if dialect.DBType != "postgresql" {
			continue
		}

		// TODO: support more than the first Postgres source when the catalog can
		// bind a plugin to multiple sources. Keep the host's ordering here.
		snapshot, err := postgres.Read(ctx, func(ctx context.Context, sql string) ([]postgres.Row, error) {
			return readPostgresRows(ctx, cc, sources, dialect.Name, sql)
		})
		if err != nil {
			return nil, fmt.Errorf("read metadata for data source %q: %w", dialect.Name, err)
		}

		return &anfranode.CatalogMetadata{
			Postgres: &anfranode.PostgresMetadata{SourceKey: dialect.Name, Snapshot: snapshot},
		}, nil
	}

	return nil, nil
}

func readPostgresRows(ctx context.Context, cc command.CommandContext, sources datasource.DataSources, name, sql string) ([]postgres.Row, error) {
	q, err := query.InDataSourceUngoverned(ctx, cc.DataPerms, sources, name, sql)
	if err != nil {
		return nil, err
	}

	observer, id, started := cc.QueryObserver(), command.NewQueryID(), time.Now()
	observer.QueryCompiled(ctx, command.CompiledQuery{ID: id, DataSource: name, SQL: sql})

	var fields []string
	var rows []postgres.Row
	err = q.Stream(ctx, cc.Clients.CanalQuery.ForTenant(cc.TenantID), query.Bounded(query.NoLimit, cc.QueryLimits), canalquery.Handler{
		Header: func(header canalquery.Header) error {
			fields = header.Fields
			return nil
		},
		Row: func(row canalquery.Row) error {
			var cells []any
			if err := jsonkit.Unmarshal(row, &cells); err != nil {
				return err
			}
			if len(cells) != len(fields) {
				return fmt.Errorf("metadata row has %d cells for %d fields", len(cells), len(fields))
			}

			values := make(postgres.Row, len(fields))
			for i, field := range fields {
				values[field] = cells[i]
			}
			rows = append(rows, values)

			return nil
		},
		Progress: func(progress canalquery.Progress) error {
			if progress.Kind == "dw_query_id" {
				observer.QueryOnWarehouse(ctx, id, progress.DwQueryID)
			}

			return nil
		},
	})
	observer.QueryFinished(ctx, id, command.FinishedQuery{Rows: len(rows), Err: err, Timings: command.Timings{EndMs: time.Since(started).Milliseconds()}})

	return rows, err
}
