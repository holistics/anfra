package query

import (
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// QueryResult is the `query` result: the SQL that ran, the AQL it was compiled
// from, what each column is, and the rows.
type QueryResult struct {
	SQL     string                    `json:"sql"`
	AQL     string                    `json:"aql,omitempty" doc:"the AQL that ran: the query with its Query Input applied; absent for a SQL query"`
	Columns []anfranode.ExploreColumn `json:"columns" doc:"one per field of result, in order"`
	Result  QueryRows                 `json:"result"`
}

type QueryRows struct {
	Fields  []string         `json:"fields"`
	Records []canalquery.Row `json:"records"`
}

// CompiledQuery is the `query compile` result: the SQL the query compiles to.
type CompiledQuery struct {
	SQL     string                    `json:"sql"`
	AQL     string                    `json:"aql,omitempty" doc:"the AQL compiled: the query with its Query Input applied; absent for a SQL query"`
	Columns []anfranode.ExploreColumn `json:"columns,omitempty" doc:"what each column of an explore query's answer will be; absent for other queries"`
}

// describeFields is one column per field a query answered, in order: as
// anfra-node described it when it did, and adhoc when it did not (any query but
// an explore, or SQL), named by the field alone.
func describeFields(fields []string, described []anfranode.ExploreColumn) []anfranode.ExploreColumn {
	byName := make(map[string]anfranode.ExploreColumn, len(described))
	for _, c := range described {
		byName[c.Name] = c
	}
	out := make([]anfranode.ExploreColumn, 0, len(fields))
	for _, name := range fields {
		c, ok := byName[name]
		if !ok {
			c = anfranode.ExploreColumn{Name: name, FieldName: name, Label: name, Adhoc: true}
		}
		out = append(out, c)
	}
	return out
}
