package command

import (
	"fmt"
	"time"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
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
	// Exports is where an export's file goes, and how it is reached (see
	// ExportStore). Nil: this server stores no exports, and an export refuses.
	Exports ExportStore
	Repo    repo.Repo
	// AllowLocalRepo lets a command run on a local repo (a directory). Off by
	// default, so a platform serving managed repos cannot run one on a directory
	// by mistake, reading one directory, and its data_sources.yml, for every
	// caller: a command on a local repo is refused (LocalRepoNotAllowed). anfra's
	// CLI and serve set it; a platform that serves a directory on purpose (a
	// self-hosted install) sets it from its own configuration.
	AllowLocalRepo bool
	// DataSources are how commands reach the repo's data sources, supplied by the
	// host: every one's dialect, to compile, and one connection, to run a query
	// (datasource.DataSources). A managed repo has no data_sources.yml to read, so
	// a host that runs one supplies them (a server keeps them in its own store,
	// never in the repo); for a local repo they override its file. Nil: a local
	// repo's file, and a managed repo has none (see ResolveDataSources).
	DataSources datasource.DataSources
	// TenantID is the tenant the invocation acts for, as the host identifies it:
	// its org, on a host that serves several. canal-query keys its warehouse
	// connection pools by it, so that tenants never share a connection, and
	// records it on its spans. Required to run a query: a host that serves one
	// tenant (anfra's CLI and serve) names it too, rather than a default that a
	// host serving several could fall into by omission.
	TenantID string
	// DataPerms are the restrictions to compile into queries. Required for
	// EVERY invocation, not only the ones that read data — see app.Dispatch.
	DataPerms dataperm.Set
	// QueryLimits bound what a query may cost the host: a result over them fails
	// with query.ResultTooLarge rather than being cut short. An export, which
	// streams to a file, is not bound by them. Zero: the defaults.
	QueryLimits query.Limits
	// Observers are told of what the command does as it does it — each query it
	// runs on a warehouse — for a host that records it. Zero: nobody is.
	Observers Observers
	// Attribution names the caller for logs and audit. Never read by a command,
	// and never by the query layer — see internal/attribution.
	Attribution attribution.Fields
	// Server is the warm server the command runs in, for status to report: nil
	// one-shot, and in a host that embeds the engine.
	Server *ServerInfo
	// Update reports the newest anfra release the host knows of, for version to
	// report: nil when the host does not check for updates (one that embeds the
	// engine, or a user who opted out), and a nil result when it knows none yet.
	Update func() *UpdateInfo
	// Skills reports the latest anfra skills version the host knows of, for
	// version to compare the skills in use with: nil when the host does not
	// check, and a nil result when it knows none yet.
	Skills func() *SkillsInfo
}

// SkillsInfo is the latest anfra skills version the host knows of, and when it
// checked.
type SkillsInfo struct {
	Latest    string
	CheckedAt time.Time
}

// ResolveDataSources is how the invocation reaches the repo's data sources: the
// host's (DataSources) when it supplies them, a local repo's data_sources.yml
// otherwise. A managed repo without the host's has none, which is refused here
// rather than read as an empty directory.
func (cc CommandContext) ResolveDataSources() (datasource.DataSources, error) {
	if cc.DataSources != nil {
		return cc.DataSources, nil
	}
	local, ok := cc.Repo.Local()
	if !ok {
		return nil, fmt.Errorf("repo %s keeps no data_sources.yml: its host supplies its data sources (Invocation.DataSources)", cc.Repo)
	}
	sources, err := datasource.Load(local.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("load data sources: %w", err)
	}
	return sources, nil
}

// ServerInfo is a running `anfra serve`, as status reports it.
type ServerInfo struct {
	// URL is where the server listens, for a CLI on this machine to reach it:
	// http://0.0.0.0:7878 in a container. Not where a caller elsewhere reaches
	// it, nor a base for links (see baseURLOf in cmd/anfra).
	URL        string `json:"url"`
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
}

// UpdateInfo is the newest anfra release the host knows of, as version reports it.
type UpdateInfo struct {
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

// Sidecars declares which sidecars a command needs (so the one-shot CLI knows
// what to spawn; under `serve` they're all warm regardless).
type Sidecars struct {
	Node       bool
	CanalQuery bool
}
