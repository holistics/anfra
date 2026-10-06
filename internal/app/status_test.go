package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/internal/sidecar"
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
		clients Clients
		want    State
	}{
		{"no warm server", Clients{}, StateNotRunning},
		{"a sidecar not connected", Clients{CanalQuery: sidecar.NewCanalQueryClient(canal.URL, false)}, StateDegraded},
		{"a sidecar down", Clients{
			Node:       sidecar.NewAnfraNodeClientHTTP(down.URL),
			CanalQuery: sidecar.NewCanalQueryClient(canal.URL, false),
		}, StateDegraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkStatus(context.Background(), tc.clients)
			if got.State != tc.want {
				t.Errorf("state = %s, want %s (%+v)", got.State, tc.want, got.Sidecars)
			}
		})
	}
	if c, _ := Find("status"); c.(*command[NoInput, StatusResult]).def.Valid(StatusResult{State: StateDegraded}) {
		t.Error("degraded is valid")
	}
	if c, _ := Find("status"); !c.(*command[NoInput, StatusResult]).def.Valid(StatusResult{State: StateHealthy}) {
		t.Error("healthy is invalid")
	}
}
