package app

import (
	"context"
	"fmt"

	"github.com/anfra-ai/anfra/internal/query"
	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/validate"
)

// Commands is the registry — the single source for the CLI and /call. Add a
// command here and it appears on both surfaces (and in help).
var Commands = []Command{
	{
		Name:  "status",
		Short: "Report the health of anfra's sidecars (node + canal-query)",
		Needs: func(map[string]any) Sidecars { return Sidecars{Node: true, Canal: true} },
		Run: func(ctx context.Context, c Clients, _ repo.Repo, _ map[string]any) (any, error) {
			res := checkSidecars(ctx, c)
			st := StatusOK
			if res.Node != "ok" || res.Canal != "ok" {
				st = StatusInvalid
			}
			return Response{Status: st, Data: res}, nil
		},
	},
	{
		Name:  "query",
		Short: "Compile and run an AQL query against a dataset",
		Args: []Arg{
			{Name: "dataset", Type: ArgString, Usage: "unique name of the dataset to compile against (required)"},
			{Name: "aql", Type: ArgString, Usage: "the AQL query; if omitted, read from stdin"},
			{Name: "generate", Type: ArgBool, Usage: "output the generated SQL instead of running the query", Aliases: []Alias{{Name: "validate", Usage: "validate the query without running it"}}},
		},
		StdinArg: "aql",
		Needs: func(args map[string]any) Sidecars {
			return Sidecars{Node: true, Canal: !IsTruthy(args["generate"])}
		},
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			return RunQuery(ctx, c, repo, args)
		},
	},
	{
		Name:       "validate",
		Short:      "Type-check the AML repo and report diagnostics",
		Positional: &Positional{Name: "globs", Usage: "optional file globs; report only diagnostics for matching files"},
		Needs:      func(map[string]any) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			res, err := validate.Run(ctx, c.Node, repo, argStrings(args, "globs"))
			if err != nil {
				return nil, err
			}
			st := StatusOK
			if res.Invalid() {
				st = StatusInvalid
			}
			return Response{Status: st, Data: res}, nil
		},
	},
}

// sidecarStatus is the `status` result: per-sidecar health, "ok" or the error.
type sidecarStatus struct {
	Node  string `json:"node"`
	Canal string `json:"canal"`
}

// checkSidecars health-checks each sidecar the invocation can reach. Under a warm
// server these are the long-lived sidecars; one-shot spawns fresh ones (a smoke
// test that they come up).
func checkSidecars(ctx context.Context, c Clients) sidecarStatus {
	res := sidecarStatus{Node: "ok", Canal: "ok"}
	switch {
	case c.Node == nil:
		res.Node = "unavailable"
	default:
		if _, err := c.Node.Ping(ctx); err != nil {
			res.Node = err.Error()
		}
	}
	switch {
	case c.Canal == nil:
		res.Canal = "unavailable"
	default:
		if err := c.Canal.Health(ctx); err != nil {
			res.Canal = err.Error()
		}
	}
	return res
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
