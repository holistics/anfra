// Package postgres reads PostgreSQL schema metadata for the semantic catalog.
package postgres

import (
	"context"
	"fmt"
	"strings"
)

// TableIdentity identifies a table within a PostgreSQL database.
type TableIdentity struct {
	DatabaseName string `json:"database_name"`
	SchemaName   string `json:"schema_name"`
	TableName    string `json:"table_name"`
}

// ColumnIdentity identifies a column within a table.
type ColumnIdentity struct {
	TableIdentity
	ColumnName string `json:"column_name"`
}

// Table is a table's source metadata.
type Table struct {
	TableIdentity
	RelationName string `json:"relation_name"`
	Description  string `json:"description,omitempty"`
}

// Column keeps the source's type and primary-key membership, without AML mapping.
type Column struct {
	ColumnIdentity
	DataType     string `json:"data_type"`
	Description  string `json:"description,omitempty"`
	IsPrimaryKey bool   `json:"is_primary_key"`
}

// ForeignKey is one referencing/referenced column pair of a constraint.
type ForeignKey struct {
	ConstraintName string         `json:"constraint_name,omitempty"`
	From           ColumnIdentity `json:"from"`
	To             ColumnIdentity `json:"to"`
}

// SchemaSnapshot is the parsed metadata of one PostgreSQL database. Its wire
// form is the Postgres plugin's LoadedPostgresMetadata.
type SchemaSnapshot struct {
	Tables      []Table      `json:"tables"`
	Columns     []Column     `json:"columns"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
	Warnings    []string     `json:"warnings,omitempty"`
}

// Row is a catalog-query row, keyed by column name.
type Row map[string]any

// Read fetches all four catalog queries through query and parses their rows.
// A query failure discards the snapshot. Malformed rows are skipped with warnings.
func Read(ctx context.Context, query func(context.Context, string) ([]Row, error)) (SchemaSnapshot, error) {
	var rows [4][]Row
	for i, sql := range []string{tablesSQL, columnsSQL, primaryKeysSQL, foreignKeysSQL} {
		var err error
		rows[i], err = query(ctx, sql)
		if err != nil {
			return SchemaSnapshot{}, err
		}
	}

	return parse(rows[0], rows[1], rows[2], rows[3]), nil
}

func parse(tables, columns, primaryKeys, foreignKeys []Row) SchemaSnapshot {
	var snapshot SchemaSnapshot
	keys := make(map[ColumnIdentity]bool, len(primaryKeys))
	for _, row := range primaryKeys {
		if valid(row, "primary key", &snapshot.Warnings, "database_name", "schema_name", "table_name", "column_name") {
			keys[columnIdentity(row)] = true
		}
	}

	for _, row := range tables {
		if !valid(row, "table", &snapshot.Warnings, "database_name", "schema_name", "table_name") {
			continue
		}

		id := tableIdentity(row)
		snapshot.Tables = append(snapshot.Tables, Table{
			TableIdentity: id, Description: text(row, "description"),
			RelationName: quote(id.DatabaseName) + "." + quote(id.SchemaName) + "." + quote(id.TableName),
		})
	}

	for _, row := range columns {
		if !valid(row, "column", &snapshot.Warnings, "database_name", "schema_name", "table_name", "column_name", "data_type") {
			continue
		}

		id := columnIdentity(row)
		snapshot.Columns = append(snapshot.Columns, Column{
			ColumnIdentity: id, DataType: text(row, "data_type"),
			Description: text(row, "description"), IsPrimaryKey: keys[id],
		})
	}

	for _, row := range foreignKeys {
		if !valid(row, "foreign key", &snapshot.Warnings, "database_name", "schema_name", "table_name", "column_name", "ref_schema_name", "ref_table_name", "ref_column_name") {
			continue
		}

		from := columnIdentity(row)
		snapshot.ForeignKeys = append(snapshot.ForeignKeys, ForeignKey{
			ConstraintName: text(row, "constraint_name"), From: from,
			To: ColumnIdentity{
				TableIdentity: TableIdentity{DatabaseName: from.DatabaseName, SchemaName: text(row, "ref_schema_name"), TableName: text(row, "ref_table_name")},
				ColumnName:    text(row, "ref_column_name"),
			},
		})
	}

	return snapshot
}

func tableIdentity(row Row) TableIdentity {
	return TableIdentity{DatabaseName: text(row, "database_name"), SchemaName: text(row, "schema_name"), TableName: text(row, "table_name")}
}

func columnIdentity(row Row) ColumnIdentity {
	return ColumnIdentity{TableIdentity: tableIdentity(row), ColumnName: text(row, "column_name")}
}

func text(row Row, key string) string {
	value, _ := row[key].(string)
	return value
}

func quote(identifier string) string { return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"` }

func valid(row Row, kind string, warnings *[]string, fields ...string) bool {
	ok := true
	for _, field := range fields {
		if text(row, field) != "" {
			continue
		}

		parts := []string{}
		for _, key := range []string{"schema_name", "table_name", "column_name"} {
			if value := text(row, key); value != "" {
				parts = append(parts, value)
			}
		}

		*warnings = append(*warnings, fmt.Sprintf("Skipped %s row %q because %q must be a non-empty string.", kind, strings.Join(parts, "."), field))
		ok = false
	}

	return ok
}
