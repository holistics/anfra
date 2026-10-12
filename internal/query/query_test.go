package query

import (
	"context"
	"testing"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestExtractLimit(t *testing.T) {
	braceAQL := "explore {\n  dimensions {\n    products.name\n  }\n}"

	tests := []struct {
		name    string
		in      string
		wantAQL string
		wantLim int
		wantErr bool
	}{
		{
			name:    "no directive",
			in:      "products | select(products.name)",
			wantAQL: "products | select(products.name)",
			wantLim: NoLimit,
		},
		{
			name:    "brace-form with limit on its own line",
			in:      braceAQL + "\nlimit: 5\n",
			wantAQL: braceAQL + "\n", // brace kept, directive gone
			wantLim: 5,
		},
		{
			name:    "limit at end of input, no trailing newline",
			in:      braceAQL + "\nlimit: 100",
			wantAQL: braceAQL,
			wantLim: 100,
		},
		{
			name:    "extra spaces around value",
			in:      braceAQL + "  limit:   42  \n",
			wantAQL: braceAQL + "\n", // trailing newline after the directive is preserved
			wantLim: 42,
		},
		{
			name:    "limit zero is allowed",
			in:      braceAQL + " limit: 0",
			wantAQL: braceAQL,
			wantLim: 0,
		},
		{
			name:    "non-integer limit errors",
			in:      braceAQL + " limit: ten",
			wantErr: true,
		},
		{
			name:    "empty limit errors",
			in:      braceAQL + " limit:\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAQL, gotLim, err := ExtractLimit(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (aql=%q lim=%d)", gotAQL, gotLim)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotAQL != tt.wantAQL {
				t.Errorf("aql:\n got  %q\n want %q", gotAQL, tt.wantAQL)
			}
			if gotLim != tt.wantLim {
				t.Errorf("limit: got %d, want %d", gotLim, tt.wantLim)
			}
		})
	}
}

// The sidecar keys its compile cache by RepoID. One sidecar can serve several
// repos — and, when shared, several tenants — so the ID has to travel on the
// request; without it the sidecar falls back to a process-wide identity and
// distinct repos collide on one cache entry. A local repo sends its directory;
// a managed repo its revision; never both.
func TestCompileRequestCarriesTheRepo(t *testing.T) {
	sources := datasource.Set{"demo": {Name: "demo", DBType: "postgresql"}}
	local := repo.Local{Dir: "/repos/acme/sales", ID: "sales-1a2b3c4d"}.Repo()
	revision, err := repo.Managed("0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b",
		"0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		r    repo.Repo
		want map[string]any
	}{
		"local": {local, map[string]any{"repoId": "sales-1a2b3c4d", "repoPath": "/repos/acme/sales"}},
		"revision": {revision, map[string]any{"repoId": "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b",
			"revision": "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567"}},
	} {
		req, err := CompileRequest(context.Background(), Source{Repo: tc.r, DataSources: sources}, "ecommerce", "products | select(products.name)")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, err := jsonkit.Marshal(req)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		var wire map[string]any
		if err := jsonkit.Unmarshal(raw, &wire); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
		for _, key := range []string{"repoId", "repoPath", "revision"} {
			if got, want := wire[key], tc.want[key]; got != want {
				t.Errorf("%s: wire %s = %v, want %v", name, key, got, want)
			}
		}
		if ds, ok := wire["dataSources"].(map[string]any)["demo"].(map[string]any); !ok || ds["dbtype"] != "postgresql" {
			t.Errorf("%s: wire dataSources = %v", name, wire["dataSources"])
		}
	}
}
