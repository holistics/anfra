package command

import (
	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// Clients are the sidecar clients a command runs against. A client is nil when
// the command doesn't need that sidecar (see Def.Needs).
type Clients struct {
	Node       *anfranode.Client
	CanalQuery *canalquery.Client
}

// CommandContext is everything a command runs against, other than its args: the
// sidecars to call, the repo to act on, the data restrictions to apply, and who
// the invocation is for. It is host-constructed and never deserialized —
// Request is the caller-supplied half of an invocation, this is the trusted
// half, and that boundary is why the two are not one struct.
//
// Note what is absent: there is no principal, role or policy. The engine makes
// no access decisions. A host decides whether a caller may run a command before
// it builds one of these; what reaches the engine is the consequence of that
// decision, not its inputs.
type CommandContext struct {
	Clients Clients
	Repo    repo.Repo
	// DataPerms are the restrictions to compile into queries. Required for
	// EVERY invocation, not only the ones that read data — see app.Dispatch.
	DataPerms dataperm.Set
	// Attribution names the caller for logs and audit. Never read by a command,
	// and never by the query layer — see internal/attribution.
	Attribution attribution.Fields
	// Server is the warm server the command runs in, for status to report: nil
	// one-shot, and in a host that embeds the engine.
	Server *ServerInfo
}

// ServerInfo is a running `anfra serve`, as status reports it.
type ServerInfo struct {
	URL        string `json:"url"`
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
}

// Sidecars declares which sidecars a command needs (so the one-shot CLI knows
// what to spawn; under `serve` they're all warm regardless).
type Sidecars struct {
	Node       bool
	CanalQuery bool
}
