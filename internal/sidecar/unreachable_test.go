package sidecar

import (
	"context"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
)

// A sidecar that does not answer is an outage, classified; a caller that gave
// up first is not, and its own error comes back.
func TestUnreachable(t *testing.T) {
	refused := errors.New("connection refused")
	if err := Unreachable(context.Background(), "anfra-node", refused); !errors.Is(err, errcode.SidecarUnavailable) || !errors.Is(err, refused) {
		t.Errorf("down: %v, want sidecar_unavailable wrapping the cause", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Unreachable(ctx, "anfra-node", refused); err != refused {
		t.Errorf("canceled: %v, want the error as it is", err)
	}
}
