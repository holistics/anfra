package query

import (
	"os"
	"path/filepath"
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
// distinct repos collide on one cache entry.
func TestCompileRequestCarriesRepoID(t *testing.T) {
	configDir := t.TempDir()
	manifest := filepath.Join(configDir, datasource.FileName)
	if err := os.WriteFile(manifest, []byte("data_sources:\n  demo:\n    type: postgresql\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	r := repo.Repo{Dir: "/repos/acme/sales", ID: "sales-1a2b3c4d", ConfigDir: configDir}

	req, err := CompileRequest(r, "ecommerce", "products | select(products.name)")
	if err != nil {
		t.Fatalf("CompileRequest: %v", err)
	}
	if req.RepoID != r.ID {
		t.Errorf("RepoID = %q, want %q", req.RepoID, r.ID)
	}
	if req.RepoPath != r.Dir {
		t.Errorf("RepoPath = %q, want %q", req.RepoPath, r.Dir)
	}

	// The wire contract: anfra-node reads `repoId`.
	raw, err := jsonkit.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := jsonkit.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := wire["repoId"]; got != r.ID {
		t.Errorf("wire repoId = %v, want %q", got, r.ID)
	}
}
