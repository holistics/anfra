package queries

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/holistics/anfra/internal/apps/dispatch"
)

// LineageRequest is the SDK's BackendLineageRequest. Targets are already in the catalog's shape
// ({dataset_name, metric_name}, ...), resolved by the SDK against the Dataset descriptors.
type LineageRequest struct {
	Targets []json.RawMessage `json:"targets"`
	Limit   *int              `json:"limit,omitempty"`
	Offset  *int              `json:"offset,omitempty"`
}

// Lineage runs the lineage of a Data App's targets on anfra and answers the SDK's
// BackendLineageResult: one result per target, a failed one carrying its own error.
func Lineage(ctx context.Context, a dispatch.Caller, req LineageRequest) (json.RawMessage, error) {
	args := map[string]any{"targets": req.Targets}
	if req.Limit != nil {
		args["limit"] = *req.Limit
	}
	if req.Offset != nil {
		args["offset"] = *req.Offset
	}
	// An "invalid" status only says some target failed; its error travels in its own result.
	_, raw, err := a.Call(ctx, "lineage", args)
	if err != nil {
		var callErr *dispatch.CallError
		if errors.As(err, &callErr) {
			return nil, &Failure{Message: callErr.Message}
		}
		return nil, err
	}
	return raw, nil
}
