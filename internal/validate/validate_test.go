package validate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// anfra-node's aml.validate requires the repo's compile-cache identity.
func TestRepoSendsRepoID(t *testing.T) {
	var got struct {
		Method string                     `json:"method"`
		Params sidecar.ValidateAMLRequest `json:"params"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"compileErrors":[],"reports":[]}}`))
	}))
	defer srv.Close()

	r := repo.Resolve(t.TempDir())
	if _, err := Repo(context.Background(), sidecar.NewAnfraNodeClientHTTP(srv.URL), r, nil); err != nil {
		t.Fatalf("Repo: %v", err)
	}
	if got.Method != "aml.validate" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Params.RepoID == "" || got.Params.RepoID != r.ID {
		t.Errorf("repoId = %q, want %q", got.Params.RepoID, r.ID)
	}
}
