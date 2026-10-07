package sidecar

import (
	"context"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// Unreachable classifies a failure to reach a sidecar at all — the request
// never got a response — as sidecar_unavailable. A caller that gave up first
// is not an outage: its cancellation or deadline is returned as it is, for the
// host to treat as its own.
func Unreachable(ctx context.Context, sidecar string, err error) error {
	if ctx.Err() != nil {
		return err
	}
	return apperr.Encapsulate(err, errcode.SidecarUnavailable, sidecar+" is not responding.")
}
