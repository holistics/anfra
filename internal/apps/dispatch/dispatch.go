// Package dispatch is how Data App serving runs anfra commands: in-process, through the same
// registry `POST /call` serves, against the server's warm sidecars.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/repo"
)

// Caller runs one anfra command and returns its status and its data as JSON.
type Caller interface {
	Call(ctx context.Context, command string, args map[string]any) (status string, data json.RawMessage, err error)
	// Healthy reports whether the sidecars the commands run on answer.
	Healthy(ctx context.Context) bool
}

// CallError is a command that failed: a bad query, an unknown dataset, …
type CallError struct{ Message string }

func (e *CallError) Error() string { return e.Message }

// IsAbort reports whether err comes from a cancelled context (a client that went away).
func IsAbort(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// InProcess runs commands with app.Dispatch, on the server's sidecar clients.
type InProcess struct {
	Clients app.Clients
	Repo    repo.Repo
}

// Call dispatches the command. Its args go through JSON first, as a `/call` body's do, so a
// command reads the same types from either.
func (d InProcess) Call(ctx context.Context, command string, args map[string]any) (string, json.RawMessage, error) {
	decoded := map[string]any{}
	if len(args) > 0 {
		raw, err := json.Marshal(args)
		if err != nil {
			return "", nil, err
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", nil, err
		}
	}
	res, err := app.Dispatch(ctx, d.Clients, d.Repo, app.Request{Command: command, Args: decoded})
	if err != nil {
		if IsAbort(err) || ctx.Err() != nil {
			return "", nil, errors.Join(ctx.Err(), err)
		}
		return "", nil, &CallError{Message: err.Error()}
	}
	data, err := json.Marshal(res.Data)
	if err != nil {
		return "", nil, err
	}
	return string(res.Status), data, nil
}

// Healthy runs `status`, which pings both sidecars.
func (d InProcess) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	status, _, err := d.Call(ctx, "status", nil)
	return err == nil && status == string(app.StatusOK)
}
