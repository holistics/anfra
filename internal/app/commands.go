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
		Short: "Report whether a warm server is running and its sidecars are healthy",
		// No Needs on purpose: status must NOT spawn sidecars. One-shot (no warm
		// server) then honestly reports "not running" instead of starting the
		// sidecars just to declare them healthy.
		Run: func(ctx context.Context, c Clients, _ repo.Repo, _ map[string]any) (any, error) {
			res := checkStatus(ctx, c)
			st := StatusOK
			if res.Server != "running" || res.Sidecars == nil || res.Sidecars.Node != "ok" || res.Sidecars.CanalQuery != "ok" {
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
			{Name: "generate", Type: ArgBool, Usage: "output the generated SQL instead of running the query"},
			{Name: "validate", Type: ArgBool, Usage: "type-check the query and report diagnostics instead of running it (same as `anfra validate --aql`)"},
		},
		StdinArg: "aql",
		Needs: func(args map[string]any) Sidecars {
			// canal-query is only needed to actually run — not to generate SQL or validate.
			return Sidecars{Node: true, CanalQuery: !IsTruthy(args["generate"]) && !IsTruthy(args["validate"])}
		},
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			if IsTruthy(args["validate"]) {
				return validateAQLResponse(ctx, c, repo, argString(args, "dataset"), argString(args, "aql"))
			}
			return RunQuery(ctx, c, repo, args)
		},
	},
	{
		Name:  "validate",
		Short: "Validate the AML repo, or a single AQL query with --aql",
		Args: []Arg{
			{Name: "aql", Type: ArgString, Usage: "validate a single AQL query instead of repo files; if omitted, read from stdin"},
			{Name: "dataset", Type: ArgString, Usage: "dataset to validate --aql against (required with --aql)"},
		},
		Positional: &Positional{Name: "globs", Usage: "optional file globs; report only diagnostics for matching files (repo mode)"},
		StdinArg:   "aql",
		Needs:      func(map[string]any) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			// --aql (or piped stdin) selects single-query mode; otherwise validate repo files.
			if aql := argString(args, "aql"); aql != "" {
				return validateAQLResponse(ctx, c, repo, argString(args, "dataset"), aql)
			}
			res, err := validate.Repo(ctx, c.Node, repo, argStrings(args, "globs"))
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

// validateAQLResponse type-checks a single AQL query and wraps the result in a
// status envelope. Shared by `validate --aql` and `query --validate` so the two
// behave identically.
func validateAQLResponse(ctx context.Context, c Clients, r repo.Repo, dataset, aql string) (Response, error) {
	res, err := validate.AQL(ctx, c.Node, r, dataset, aql)
	if err != nil {
		return Response{}, err
	}
	st := StatusOK
	if res.Invalid() {
		st = StatusInvalid
	}
	return Response{Status: st, Data: res}, nil
}

// statusResult is the `status` result: whether the warm server is running and,
// when it is, its sidecars' health nested under `sidecars`.
type statusResult struct {
	Server   string         `json:"server"` // "running" | "not running"
	Sidecars *sidecarHealth `json:"sidecars,omitempty"`
}

// sidecarHealth is each sidecar's health: "ok" or the error message.
type sidecarHealth struct {
	Node       string `json:"node"`
	CanalQuery string `json:"canal-query"`
}

// checkStatus reports the warm server's health. status spawns no sidecars, so in
// one-shot mode (no warm server) both clients are nil → "not running"; under a
// warm server the clients are live and get health-checked.
func checkStatus(ctx context.Context, c Clients) statusResult {
	if c.Node == nil && c.CanalQuery == nil {
		return statusResult{Server: "not running"}
	}
	sc := &sidecarHealth{Node: "ok", CanalQuery: "ok"}
	if c.Node == nil {
		sc.Node = "unavailable"
	} else if _, err := c.Node.Ping(ctx); err != nil {
		sc.Node = err.Error()
	}
	if c.CanalQuery == nil {
		sc.CanalQuery = "unavailable"
	} else if err := c.CanalQuery.Health(ctx); err != nil {
		sc.CanalQuery = err.Error()
	}
	return statusResult{Server: "running", Sidecars: sc}
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

	if clients.CanalQuery == nil {
		return QueryResult{}, fmt.Errorf("query execution requires the canal-query sidecar")
	}
	r, err := query.Run(ctx, clients.Node, clients.CanalQuery, repo, dataset, aql)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{SQL: r.SQL, Result: &QueryRows{Fields: r.Fields, Records: r.Records}}, nil
}
