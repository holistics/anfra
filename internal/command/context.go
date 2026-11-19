package command

import (
	"context"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/telemetry"
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
	// TraceData says what of the user's data the invocation's spans record, per
	// category: an op's input and answer, a query's SQL, an error's message
	// (docs/designs/telemetry.md). Zero: each category at its default. A query's
	// rows are never recorded.
	TraceData telemetry.Settings
	// Server is the warm server the command runs in, for status to report: nil
	// one-shot, and in a host that embeds the engine.
	Server *ServerInfo
	// Update reports the newest anfra release the host knows of, for version to
	// report: nil when the host does not check for updates (one that embeds the
	// engine, or a user who opted out), and a nil result when it knows none yet.
	Update func() *UpdateInfo
	// Telemetry reports where the host sends its traces, for status to report: nil when the host doesn't say (one that embeds the engine).
	Telemetry func() *TelemetryInfo
	// Skills reports the latest anfra skills version the host knows of, for
	// version to compare the skills in use with: nil when the host does not
	// check, and a nil result when it knows none yet.
	Skills func() *SkillsInfo
	// QueryCache is the repo's query cache: its state, for status, and its
	// use, for a query. anfra's host's is over the repo's store, a platform's
	// over its own. Nil: the host keeps no query cache, every query runs
	// uncached, and status says nothing of one.
	QueryCache QueryCache
	// QueryDefaults are what a query gets when it sets none, and which of them
	// it can't change (docs/designs/query-settings.md). Zero: anfra's own
	// defaults, none locked. Check them when they're set.
	QueryDefaults QueryDefaults
	// AQLCompiler is the AQL compiler the repo's AQL compiles and type-checks
	// with, "v1" or "v2", as a platform sets it from the repo's settings
	// (aql.compiler.version). Empty: a local repo's settings file says, and a
	// managed repo, which has none here, compiles with v1.
	AQLCompiler string
}

// QueryCache is the invocation's query cache, as commands use it.
type QueryCache interface {
	querycache.Summarizer
	// Settings are the cache's rules for the repo.
	Settings() QueryCacheSettings
	// Manager is the cache's manager: its store open, its files under its
	// namespace. An error is why the cache can't be used now: a query runs
	// without it.
	Manager(ctx context.Context) (*querycache.Manager, error)
	// StartLake starts the canal-lake a write needs, unless it runs already
	// or something else runs it.
	StartLake(ctx context.Context) error
	// Contents are what the cache holds, and where, for cache.status: read
	// without creating, changing or migrating anything.
	Contents(ctx context.Context) (QueryCacheContents, error)
	// Report tells whoever runs anfra why a query ran without the cache: an
	// error of Manager's, of a write, or a key that couldn't be made. The
	// query's answer only says that it did.
	Report(ctx context.Context, err error)
}

// QueryCacheSettings are the query cache's rules for a repo (query.cache in
// .anfra/config.yml). A query's default cache.duration is one of the repo's
// QueryDefaults.
type QueryCacheSettings struct {
	// Disabled says why the cache is off for the repo; empty, it is on.
	Disabled string
	// MaxDuration is the longest a query's result may be cached, from 1s to
	// querycache.MaxDuration: zero, querycache.MaxDuration. A query that asks
	// for longer is refused. Check it, and the repo's default duration against
	// it (CheckDefault), when it's set.
	MaxDuration time.Duration
	// Namespace is the subfolder of canal-lake's storage the cache's files
	// go in, as whoever runs anfra set it (ANFRA_CACHE_QUERY_NAMESPACE):
	// empty, the storage's top level.
	Namespace string
}

// QueryCacheContents are what a query cache holds, and where.
type QueryCacheContents struct {
	// Folder is the cache's folder on this machine: its store, and the
	// results of the canal-lake anfra starts. Empty when the host keeps its
	// cache elsewhere.
	Folder string
	querycache.Contents
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

// TelemetryInfo is where the host records what its commands do: the user's own
// telemetry, never data sent to Holistics (docs/designs/telemetry.md).
type TelemetryInfo struct {
	State    TelemetryState `json:"state" enum:"off,on,failing" doc:"\"off\": no endpoint is set, so nothing is sent. \"on\": traces are sent to the endpoint, which accepts connections. That doesn't prove it stores them. \"failing\": an endpoint is set but doesn't accept connections, so traces are lost."`
	Endpoint string         `json:"endpoint,omitempty" doc:"the OpenTelemetry endpoint traces are sent to. Absent when off."`
	SetBy    string         `json:"set_by,omitempty" doc:"what set the endpoint: an OTEL_* variable, followed by 'in .env' when the repo's .env set it. When off, OTEL_SDK_DISABLED if that turned telemetry off, otherwise absent."`
	Data     *TelemetryData `json:"data,omitempty" doc:"what of your data reaches a span, per category, after every rule is applied."`
	// PayloadLimit is how many bytes of each payload a span records.
	PayloadLimit *TelemetryLimit `json:"payload_limit,omitempty" doc:"how many bytes of each input, result and query a span records, at most. A longer one is cut, and marked as cut."`
	// Hints are what the host says to check about where traces go, which
	// status adds to its own hints. Not part of the answer: status has said
	// them by then.
	Hints []string `json:"-"`
}

// TelemetryData is how each category of the user's data is recorded: one
// field per category of shared/telemetry's registry, which a test checks.
type TelemetryData struct {
	Inputs  TelemetryLevel `json:"inputs" doc:"what a command asked for: its arguments and input, and the names of the dataset, data source and files it ran on."`
	Outputs TelemetryLevel `json:"outputs" doc:"what came back: a command's result, without a query's rows."`
	SQL     TelemetryLevel `json:"sql" doc:"the SQL anfra sends to the warehouse."`
	Errors  TelemetryLevel `json:"errors" doc:"an error's message and stack."`
}

// TelemetryLevel is one category's level, and what set it.
type TelemetryLevel struct {
	Value string `json:"value" enum:"off,masked,raw" doc:"\"off\": not recorded. \"masked\": recorded with what you wrote replaced with ?. \"raw\": recorded as it is."`
	SetBy string `json:"set_by" doc:"what set it: its ANFRA_TRACE_* variable (followed by \"in .env\" when the repo's .env set it), ANFRA_TRACE_DATA when that held it off, or \"default\"."`
}

// TelemetryLimit is the payload limit, and what set it.
type TelemetryLimit struct {
	Value int    `json:"value" doc:"the limit, in bytes."`
	SetBy string `json:"set_by" doc:"what set it: ANFRA_TRACE_PAYLOAD_LIMIT (followed by \"in .env\" when the repo's .env set it), or \"default\"."`
}

// TransformSchema describes TelemetryInfo wherever it appears, so what it is and
// that it is experimental are said once, on the type.
func (TelemetryInfo) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "Where anfra records each command's trace: the user's own telemetry, to their own endpoint, never data sent to Holistics. " +
		"Under anfra serve, the server's settings, since its commands run there. Absent when the host doesn't say. " +
		"Experimental: its fields may change in a minor release."
	return s
}

// TelemetryState is what happens to the host's traces.
type TelemetryState string

const (
	// TelemetryOff: no endpoint is set, so nothing is sent.
	TelemetryOff TelemetryState = "off"
	// TelemetryOn: traces are sent to an endpoint that accepts connections.
	TelemetryOn TelemetryState = "on"
	// TelemetryFailing: an endpoint is set but doesn't accept connections, so
	// traces are lost: the telemetry stack has stopped, after a restart say.
	TelemetryFailing TelemetryState = "failing"
)

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
