package app

import (
	"context"
	"fmt"

	"github.com/anfra-ai/anfra/internal/query"
	"github.com/anfra-ai/anfra/internal/repo"
)

// Commands is the registry — the single source for the CLI and /call. Add a
// command here and it appears on both surfaces (and in help).
var Commands = []Command{
	{
		Name:  "ping",
		Short: "Round-trip a liveness ping to the anfra-node sidecar",
		Needs: func(map[string]any) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, c Clients, _ repo.Repo, _ map[string]any) (any, error) {
			return c.Node.Ping(ctx)
		},
	},
	{
		Name:  "query",
		Short: "Compile and run an AQL query against a dataset",
		Args: []Arg{
			{Name: "dataset", Type: ArgString, Usage: "unique name of the dataset to compile against (required)"},
			{Name: "aql", Type: ArgString, Usage: "the AQL query; if omitted, read from stdin"},
			{Name: "generate", Type: ArgBool, Usage: "output the generated SQL instead of running the query"},
		},
		StdinArg: "aql",
		Needs: func(args map[string]any) Sidecars {
			return Sidecars{Node: true, Canal: !IsTruthy(args["generate"])}
		},
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			return RunQuery(ctx, c, repo, args)
		},
	},
}

// QueryResult is the `query` result. Result is nil for --generate (compile only).
type QueryResult struct {
	SQL    string     `json:"sql"`
	Result *QueryRows `json:"result,omitempty"`
}

type QueryRows struct {
	Fields  []string `json:"fields"`
	Records [][]any  `json:"records"`
}

// RunQuery compiles an AQL query to SQL and, unless generate is set, executes it.
func RunQuery(ctx context.Context, clients Clients, repo repo.Repo, args map[string]any) (QueryResult, error) {
	dataset := argString(args, "dataset")
	aql := argString(args, "aql")
	if dataset == "" {
		return QueryResult{}, fmt.Errorf("dataset is required")
	}
	if aql == "" {
		return QueryResult{}, fmt.Errorf("aql is required")
	}

	if IsTruthy(args["generate"]) {
		sql, err := query.GenerateSQL(ctx, clients.Node, repo, dataset, aql)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{SQL: sql}, nil
	}

	if clients.Canal == nil {
		return QueryResult{}, fmt.Errorf("query execution requires the canal-query sidecar")
	}
	r, err := query.Run(ctx, clients.Node, clients.Canal, repo, dataset, aql)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{SQL: r.SQL, Result: &QueryRows{Fields: r.Fields, Records: r.Records}}, nil
}
