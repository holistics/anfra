package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// status answers its verdict as state: not_running without a warm server,
// degraded when a sidecar does not answer, healthy when both do. Only healthy
// is valid.
func TestStatusState(t *testing.T) {
	canal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer canal.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()

	for _, tc := range []struct {
		name    string
		clients command.Clients
		want    State
	}{
		{"no warm server", command.Clients{}, StateNotRunning},
		{"a sidecar not connected", command.Clients{CanalQuery: canalquery.NewClient(canal.URL, false)}, StateDegraded},
		{"a sidecar down", command.Clients{
			Node:       anfranode.NewClientHTTP(down.URL),
			CanalQuery: canalquery.NewClient(canal.URL, false),
		}, StateDegraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkStatus(context.Background(), tc.clients)
			if got.State != tc.want {
				t.Errorf("state = %s, want %s (%+v)", got.State, tc.want, got.Sidecars)
			}
		})
	}
	if valid, _ := Status.Valid(StatusResult{State: StateDegraded}); valid {
		t.Error("degraded is valid")
	}
	if valid, _ := Status.Valid(StatusResult{State: StateHealthy}); !valid {
		t.Error("healthy is invalid")
	}
}
