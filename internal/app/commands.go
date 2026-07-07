package app

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/validate"
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
			{Name: "dataset", Shorthand: "d", Type: ArgString, Usage: "unique name of the dataset to compile against (required)"},
			{Name: "aql", Shorthand: "a", Type: ArgString, Usage: "the AQL query; if omitted, read from stdin"},
			{Name: "generate", Shorthand: "g", Type: ArgBool, Usage: "output the generated SQL instead of running the query"},
			{Name: "validate", Shorthand: "c", Type: ArgBool, Usage: "type-check the query and report diagnostics instead of running it", Aliases: []Alias{{Name: "check"}}},
		},
		// --generate and --validate each pick a "don't run" mode, so they conflict.
		ExclusiveArgs: [][]string{{"generate", "validate"}},
		StdinArg:      "aql",
		Needs: func(args map[string]any) Sidecars {
			// canal-query is only needed to actually run — not to generate SQL or validate.
			return Sidecars{Node: true, CanalQuery: !IsTruthy(args["generate"]) && !IsTruthy(args["validate"])}
		},
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
			// The AQL engine doesn't support `limit:`; strip it here (see query.ExtractLimit)
			// and apply it at execution time via canal's truncate_rows.
			aql, limit, err := query.ExtractLimit(argString(args, "aql"))
			if err != nil {
				return nil, err
			}
			dataset := argString(args, "dataset")
			if IsTruthy(args["validate"]) {
				return validateAQLResponse(ctx, c, repo, dataset, aql)
			}
			return RunQuery(ctx, c, repo, dataset, aql, limit, IsTruthy(args["generate"]))
		},
	},
	{
		Name:       "validate",
		Short:      "Validate the AML repo, optionally scoped to file globs",
		Positional: &Positional{Name: "globs", Usage: "optional file globs; report only diagnostics for matching files"},
		Needs:      func(map[string]any) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, c Clients, repo repo.Repo, args map[string]any) (any, error) {
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
// status envelope (used by `query --validate`).
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

// RunQuery compiles an AQL query and, unless generate is set, executes it (with
// canal truncating to limit rows; query.NoLimit for all). On a compile failure it
// surfaces the same structured diagnostics as --validate with an "invalid" status
// (which the CLI maps to a non-zero exit), so callers see what's wrong without
// re-running the query.
func RunQuery(ctx context.Context, clients Clients, repo repo.Repo, dataset, aql string, limit int, generate bool) (any, error) {
	if dataset == "" {
		return nil, fmt.Errorf("dataset is required")
	}
	if aql == "" {
		return nil, fmt.Errorf("aql is required")
	}
	if !generate && clients.CanalQuery == nil {
		return nil, fmt.Errorf("query execution requires the canal-query sidecar")
	}

	compiled, err := query.Compile(ctx, clients.Node, repo, dataset, aql)
	if err != nil {
		// A parse/type error in the AQL: report the diagnostics (same as --validate)
		// rather than a bare message. StatusInvalid → the CLI exits non-zero.
		// Structural failures (unknown dataset/data source) aren't diagnostic-shaped,
		// so fall back to the original error.
		if diags, verr := validate.AQL(ctx, clients.Node, repo, dataset, aql); verr == nil && diags.Invalid() {
			return Response{Status: StatusInvalid, Data: diags}, nil
		}
		return nil, err
	}

	if generate {
		return QueryResult{SQL: compiled.SQL}, nil
	}
	r, err := query.Execute(ctx, clients.CanalQuery, repo, compiled, limit)
	if err != nil {
		return nil, err
	}
	return QueryResult{SQL: r.SQL, Result: &QueryRows{Fields: r.Fields, Records: r.Records}}, nil
}
