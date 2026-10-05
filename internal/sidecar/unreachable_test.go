package sidecar

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
)

// A sidecar that does not answer is an outage, classified; a caller that gave
// up first is not, and its own error comes back.
func TestUnreachableSidecar(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close() // nothing listens there now

	node := NewAnfraNodeClientHTTP(url)
	if _, err := node.Ping(context.Background()); !errors.Is(err, errcode.SidecarUnavailable) {
		t.Errorf("anfra-node down: %v, want sidecar_unavailable", err)
	}
	canal := NewCanalQueryClient(url, false)
	if _, err := canal.Execute(context.Background(), "postgres", nil, "select 1", 0); !errors.Is(err, errcode.SidecarUnavailable) {
		t.Errorf("canal-query down: %v, want sidecar_unavailable", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := node.Ping(ctx); errors.Is(err, errcode.SidecarUnavailable) || !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v, want the cancellation, unclassified", err)
	}
}
