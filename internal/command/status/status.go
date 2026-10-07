// Package status is the commands that report on anfra itself: version, and
// status of the warm server and its sidecars.
package status

import (
	"context"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/meta"
)

// Version prints the anfra version.
var Version = command.Define(command.Def[command.NoInput, VersionResult]{
	Name:     "version",
	Short:    "Print the anfra version",
	ReadOnly: true,
	// No Needs: pure metadata, spawns nothing.
	Run: func(context.Context, command.CommandContext, command.NoInput) (VersionResult, error) {
		return VersionResult{Version: meta.Version}, nil
	},
})

// Status reports whether a warm server is running and its sidecars are healthy.
var Status = command.Define(command.Def[command.NoInput, StatusResult]{
	Name:     "status",
	Short:    "Report whether a warm server is running and its sidecars are healthy",
	ReadOnly: true,
	// No Needs on purpose: status must NOT spawn sidecars. One-shot (no warm
	// server) then honestly reports not_running instead of starting the
	// sidecars just to declare them healthy.
	Run: func(ctx context.Context, cc command.CommandContext, _ command.NoInput) (StatusResult, error) {
		r := checkStatus(ctx, cc.Clients)
		r.Server = cc.Server
		return r, nil
	},
	Valid: func(r StatusResult) bool { return r.State == StateHealthy },
})

// VersionResult is the `version` result.
type VersionResult struct {
	Version string `json:"version"`
}

// StatusResult is the `status` result: the warm server's state and, when it is
// running, where it is and its sidecars' health.
type StatusResult struct {
	State    State               `json:"state" enum:"healthy,degraded,not_running"`
	Server   *command.ServerInfo `json:"server,omitempty"`
	Sidecars *SidecarHealth      `json:"sidecars,omitempty"`
}

// State is the warm server's state.
type State string

const (
	// StateHealthy: the server is running, and every sidecar answers.
	StateHealthy State = "healthy"
	// StateDegraded: the server is running, and a sidecar does not answer.
	StateDegraded State = "degraded"
	// StateNotRunning: no warm server.
	StateNotRunning State = "not_running"
)

// SidecarHealth is each sidecar's health: "ok" or the error message.
type SidecarHealth struct {
	Node       string `json:"node"`
	CanalQuery string `json:"canal-query"`
}

// checkStatus reports the warm server's health. status spawns no sidecars, so in
// one-shot mode (no warm server) both clients are nil → not_running; under a
// warm server the clients are live and get health-checked.
func checkStatus(ctx context.Context, c command.Clients) StatusResult {
	if c.Node == nil && c.CanalQuery == nil {
		return StatusResult{State: StateNotRunning}
	}
	sc := &SidecarHealth{Node: "ok", CanalQuery: "ok"}
	if c.Node == nil {
		sc.Node = "unavailable"
	} else if _, err := c.Node.Ping(ctx); err != nil {
		sc.Node = err.Error()
	}
	if c.CanalQuery == nil {
		sc.CanalQuery = "unavailable"
	} else if err := c.CanalQuery.Health(ctx); err != nil {
		sc.CanalQuery = err.Error()
	}
	state := StateHealthy
	if sc.Node != "ok" || sc.CanalQuery != "ok" {
		state = StateDegraded
	}
	return StatusResult{State: state, Sidecars: sc}
}
