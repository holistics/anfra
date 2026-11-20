package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

func columnRow(table, column string) Row {
	return Row{"database_name": "analytics", "schema_name": "public", "table_name": table, "column_name": column, "data_type": "numeric(20,4)"}
}

func TestReadMetadata(t *testing.T) {
	foreign := columnRow("orders", "customer_id")
	foreign["constraint_name"], foreign["ref_schema_name"], foreign["ref_table_name"], foreign["ref_column_name"] = "customer_fk", "crm", "customers", "id"
	table := columnRow("orders", "id")
	table["description"] = "Order facts"
	rows := map[string][]Row{
		tablesSQL:      {table},
		columnsSQL:     {columnRow("orders", "id"), columnRow("orders", "region"), columnRow("orders", "customer_id")},
		primaryKeysSQL: {columnRow("orders", "id"), columnRow("orders", "region")},
		foreignKeysSQL: {foreign},
	}
	var calls []string
	snapshot, err := Read(context.Background(), func(_ context.Context, sql string) ([]Row, error) {
		calls = append(calls, sql)
		return rows[sql], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{tablesSQL, columnsSQL, primaryKeysSQL, foreignKeysSQL}) {
		t.Fatalf("queries = %v", calls)
	}
	if len(snapshot.Tables) != 1 || snapshot.Tables[0].RelationName != `"analytics"."public"."orders"` || snapshot.Tables[0].Description != "Order facts" {
		t.Fatalf("tables = %+v", snapshot.Tables)
	}
	if len(snapshot.Columns) != 3 || !snapshot.Columns[0].IsPrimaryKey || !snapshot.Columns[1].IsPrimaryKey || snapshot.Columns[2].IsPrimaryKey || snapshot.Columns[0].DataType != "numeric(20,4)" {
		t.Fatalf("columns = %+v", snapshot.Columns)
	}
	fk := snapshot.ForeignKeys[0]
	if fk.From.ColumnName != "customer_id" || fk.To.SchemaName != "crm" || fk.To.TableName != "customers" || fk.To.ColumnName != "id" || fk.To.DatabaseName != "analytics" {
		t.Fatalf("foreign key = %+v", fk)
	}
	encoded, err := jsonkit.Marshal(snapshot)
	if err != nil || !strings.Contains(string(encoded), `"foreignKeys"`) || !strings.Contains(string(encoded), `"database_name":"analytics"`) {
		t.Fatalf("wire = %s, %v", encoded, err)
	}
}

func TestParseSkipsMalformedRows(t *testing.T) {
	column := columnRow("orders", "id")
	column["data_type"] = nil
	snapshot := parse([]Row{{"database_name": "analytics", "schema_name": "public"}}, []Row{column}, []Row{{}}, []Row{{}})
	if len(snapshot.Tables)+len(snapshot.Columns)+len(snapshot.ForeignKeys) != 0 || len(snapshot.Warnings) == 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	for _, warning := range snapshot.Warnings {
		if !strings.Contains(warning, "Skipped") {
			t.Errorf("warning = %q", warning)
		}
	}
}

func TestReadDiscardsPartialSnapshot(t *testing.T) {
	want := errors.New("query failed")
	calls := 0
	snapshot, err := Read(context.Background(), func(_ context.Context, _ string) ([]Row, error) {
		calls++
		if calls == 2 {
			return nil, want
		}
		return []Row{columnRow("orders", "id")}, nil
	})
	if !errors.Is(err, want) || calls != 2 || len(snapshot.Tables) != 0 {
		t.Fatalf("snapshot = %+v, calls = %d, err = %v", snapshot, calls, err)
	}
}
