package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/validate"
	"github.com/holistics/anfra/shared/apperr"
)

// decodeOnly decodes args without running the command: for tests that hold
// the specs to the decoder.
func (c *command[In, Out]) decodeOnly(args map[string]any) error {
	_, err := decode[In](c.def.Name, c.args, args)
	return err
}

// Every command, in every mode, against real sidecars: what it answers, and
// with which status, or how it fails. The answer's type is the compiler's to
// hold; this holds the behaviour. Every command is covered, and every command
// that can answer invalid does so in some case.
//
// It needs the sidecars' binaries and the dev warehouse:
//
//	docker compose -f docker-compose.dev.yml up -d postgres
//	ANFRA_NODE_BIN=… ANFRA_CANAL_QUERY_BIN=… go test ./internal/app -run TestCommandsAgainstRealSidecars
//
// Without the binaries it is skipped.
func TestCommandsAgainstRealSidecars(t *testing.T) {
	if os.Getenv("ANFRA_NODE_BIN") == "" || os.Getenv("ANFRA_CANAL_QUERY_BIN") == "" {
		t.Skip("set ANFRA_NODE_BIN and ANFRA_CANAL_QUERY_BIN to run commands against real sidecars")
	}
	t.Setenv("HOME", t.TempDir()) // the repo's state, the catalog among it, goes under HOME
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	valid := repo.Resolve(fixture)
	broken := repo.Resolve(brokenCopy(t, fixture))
	clients := startSidecars(ctx, t, valid)

	const ok, invalid = StatusOK, StatusInvalid
	q := func(text string) map[string]any { return map[string]any{"dataset": "ecommerce", "query": text} }
	cases := []struct {
		name    string
		repo    repo.Repo
		clients Clients
		req     Request
		status  Status         // the answer's, when it answers
		fails   apperr.AnyCode // the code it fails with, when it fails
	}{
		{"version", valid, clients, Request{Command: "version"}, ok, nil},
		{"status, healthy", valid, clients, Request{Command: "status"}, ok, nil},
		{"status, not running", valid, Clients{}, Request{Command: "status"}, invalid, nil},
		{"query", valid, clients, Request{Command: "query", Args: q("products | select(products.id, products.name)")}, ok, nil},
		{"query, invalid", valid, clients, Request{Command: "query", Args: q("nosuch | select(x.y)")}, "", validate.QueryInvalid},
		{"query compile", valid, clients, Request{Command: "query.compile", Args: q("products | select(products.id)")}, ok, nil},
		{"query compile, invalid", valid, clients, Request{Command: "query.compile", Args: q("nosuch | select(x.y)")}, "", validate.QueryInvalid},
		{"query validate, valid", valid, clients, Request{Command: "query.validate", Args: q("products | select(products.id)")}, ok, nil},
		{"query validate, invalid", valid, clients, Request{Command: "query.validate", Args: q("nosuch | select(x.y)")}, invalid, nil},
		{"ingest", valid, clients, Request{Command: "ingest"}, ok, nil},
		{"search, after ingest", valid, clients, Request{Command: "search", Args: map[string]any{"query": []any{"products"}}}, ok, nil},
		{"validate, valid", valid, clients, Request{Command: "validate"}, ok, nil},
		{"validate, invalid", broken, clients, Request{Command: "validate"}, invalid, nil},
	}

	ran := map[string]bool{}
	answered := map[string]map[Status]bool{}
	for _, tc := range cases {
		ran[tc.req.Command] = true
		t.Run(tc.name, func(t *testing.T) {
			res, err := Dispatch(ctx, CommandContext{Clients: tc.clients, Repo: tc.repo, DataPerms: dataperm.Unrestricted()}, tc.req)
			if tc.fails != nil {
				if !errors.Is(err, tc.fails) {
					t.Fatalf("got %v, want %s", err, tc.fails.Code().Qualified())
				}
				if diags, ok := apperr.DetailsOf(err, validate.QueryInvalid); ok && len(diags.Diagnostics) == 0 {
					t.Error("query_invalid carries no diagnostics")
				}
				return
			}
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if res.Status != tc.status {
				t.Errorf("status %s, want %s", res.Status, tc.status)
			}
			if answered[tc.req.Command] == nil {
				answered[tc.req.Command] = map[Status]bool{}
			}
			answered[tc.req.Command][res.Status] = true
		})
	}
	for _, c := range Commands {
		if !ran[c.Name()] {
			t.Errorf("no case runs %s", c.Name())
		}
		if c.CanBeInvalid() && !answered[c.Name()][StatusInvalid] {
			t.Errorf("%s can answer invalid, but no case does", c.Name())
		}
	}
}

// startSidecars spawns anfra-node and canal-query from their binaries, as the
// one-shot CLI does, for the whole test.
func startSidecars(ctx context.Context, t *testing.T, r repo.Repo) Clients {
	t.Helper()
	cfg := sidecar.Config{RepoID: r.ID, CompileCachePath: filepath.Join(r.CacheDir(), "compile-cache")}
	node := sidecar.NewAnfraNode(cfg)
	if err := node.Start(ctx); err != nil {
		t.Fatalf("start anfra-node: %v", err)
	}
	t.Cleanup(node.Close)
	canal := sidecar.NewCanalQuery(cfg)
	if err := canal.Start(ctx); err != nil {
		t.Fatalf("start canal-query: %v", err)
	}
	t.Cleanup(canal.Close)
	return Clients{Node: node.Client(), CanalQuery: canal.Client()}
}

// brokenCopy is the fixture plus a model that does not parse.
func brokenCopy(t *testing.T, fixture string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "broken")
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.model.aml"), []byte("Model broken { type: \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
