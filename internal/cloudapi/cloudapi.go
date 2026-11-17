// Package cloudapi is the anfra CLI's client of anfra-cloud: the ops `anfra
// publish` calls, with types written here for them.
//
// The contract is the server's: anfra-cloud owns its API and publishes it as
// api/openapi.yaml. This package vendors that file (openapi.yaml, copied by hand
// for now with scripts/cloudapi.sh) and a test holds these types to it, so a
// change on either side that the CLI would not survive fails here. The CLI calls
// public ops only, which anfra-cloud changes additively; a client ignores
// members it does not know.
//
// A repo publishes to an org, named by its address on its server,
// https://bi.example.com/o/acme (Org); the server serves the org's ops at
// <origin>/api/o/<org>/<op>. The address is what `anfra publish --set-org`
// records (org.url in a repo's .anfra/config.yml). A publish is:
//
//  1. repos.status: what is published, and the server's limits;
//  2. repos.list_missing_blobs: which of the files' blob ids
//     (repoformat.BlobID) the server lacks;
//  3. repos.create_blobs: upload those, in batches within the limits;
//  4. repos.publish: publish every file, by path and blob id, as the whole repo.
//
// Before it, `anfra publish` may add the data sources the repo uses and the org
// lacks (data_sources.list, data_sources.test, data_sources.create), when the
// person agrees: a connection is never sent unasked. It also reads the origins the
// org's Data Apps may load from (app_origins.list), to say which of the repo's
// own the org does not allow.
package cloudapi

import "time"

// The ops the CLI calls, by name: the contract's operationIds.
const (
	OpStatus           = "repos.status"
	OpListMissingBlobs = "repos.list_missing_blobs"
	OpCreateBlobs      = "repos.create_blobs"
	OpPublish          = "repos.publish"
	OpListFiles        = "repos.list_files"
	OpGetBlobs         = "repos.get_blobs"
	OpListDataSources  = "data_sources.list"
	OpCreateDataSource = "data_sources.create"
	OpTestDataSource   = "data_sources.test"
	OpRepoConfig       = "repo_config.get"
)

// The error codes the CLI tells apart.
const (
	// CodeRepoInvalid: a publish whose files fail the engine's validation; nothing
	// is published.
	CodeRepoInvalid = "repo_invalid"
	// CodeBlobsMissing: a publish naming content the server does not have; its
	// details are MissingBlobs. The client uploads it and publishes again.
	CodeBlobsMissing = "blobs_missing"
	// CodeForbidden: the caller lacks the permission the op needs.
	CodeForbidden = "forbidden"
	// CodeInvalidRequest: an input the op's schema refuses; its details are
	// apperr.Violations.
	CodeInvalidRequest = "invalid_request"
	// CodeBaseOutdated: a publish whose base is no longer the published
	// revision; its details are the published revision (LiveRevision).
	CodeBaseOutdated = "base_outdated"
	// CodeSettingsChangeForbidden: a publish changing repo settings in
	// .anfra/config.yml the caller may not change; its details name them
	// (RefusedSettings). Nothing is published.
	CodeSettingsChangeForbidden = "settings_change_forbidden"
	// CodeNotFound: no such thing, or, for an op, an older server without it.
	CodeNotFound = "not_found"
	// CodeRateLimited: too many calls; its details say how long to wait
	// (RetryAfter).
	CodeRateLimited = "rate_limited"
	// CodeUnavailable: the server can't take the call now; its details say how
	// long to wait when the server knows (RetryAfter).
	CodeUnavailable = "unavailable"
)

// LiveRevision are the details of base_outdated: the revision published
// since the publish's base, to pull.
type LiveRevision struct {
	Revision    string    `json:"revision"`
	PublishedBy *Person   `json:"published_by,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// RetryAfter are the details of rate_limited and unavailable, when the server
// knows how long to wait.
type RetryAfter struct {
	Seconds int `json:"retry_after_seconds"`
}

// MissingBlobs are the details of blobs_missing.
type MissingBlobs struct {
	OIDs []string `json:"oids"`
}

// StatusOut is the org's published revision, and the server's limits.
type StatusOut struct {
	Revision    string    `json:"revision"`
	PublishedBy *Person   `json:"published_by,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	// Limits are read from the root: the server's deprecated publish.limits,
	// the same values, is for CLIs from before pull.
	Limits Limits `json:"limits"`
}

// Limits are the server's bounds on the repo ops, which the client batches by.
type Limits struct {
	Files       int `json:"files"`
	UploadBlobs int `json:"upload_blobs"`
	UploadBytes int `json:"upload_bytes"`
	// FileBytes is the largest file; a server that does not send it takes any
	// file that fits in one upload call.
	FileBytes int `json:"file_bytes"`
	// DownloadBlobs is the most blob ids one repos.get_blobs call names.
	DownloadBlobs int `json:"download_blobs"`
	// DownloadBytes is the most content one repos.get_blobs answer carries:
	// past it, the answer leaves the rest out, to ask again for.
	DownloadBytes int `json:"download_bytes"`
}

// MaxFileBytes is the largest file the server takes.
func (l Limits) MaxFileBytes() int {
	if l.FileBytes > 0 {
		return min(l.FileBytes, l.UploadBytes)
	}
	return l.UploadBytes
}

// Person is who published a revision. Email is empty once the account is gone,
// or has left the org.
type Person struct {
	ID    string `json:"id" doc:"the person's id"`
	Email string `json:"email" doc:"the person's email"`
}

// ListMissingBlobsIn is blob ids the client computed from its files.
type ListMissingBlobsIn struct {
	OIDs []string `json:"oids"`
}

// ListMissingBlobsOut is those the server lacks, in the order given.
type ListMissingBlobsOut struct {
	OIDs []string `json:"oids"`
}

// CreateBlobsIn is content to store.
type CreateBlobsIn struct {
	Blobs [][]byte `json:"blobs"`
}

// CreateBlobsOut is the stored content's blob ids, in order.
type CreateBlobsOut struct {
	OIDs []string `json:"oids"`
}

// PublishIn is the files to publish, as the whole repo.
type PublishIn struct {
	Files   []File `json:"files"`
	Message string `json:"message,omitempty"`
	// Base is the revision the files were built on: the one last pulled or
	// published. When it's no longer the published revision, the server
	// refuses with base_outdated. Empty, the publish replaces whatever is
	// published.
	Base string `json:"base,omitempty"`
	// WithConfig says Files hold the repo's settings, .anfra/config.yml: a
	// file not listed is deleted, the settings too. Without it, the server
	// refuses the settings file, and keeps the one published. Sent to a server
	// that has repo_config.get (Client.RepoConfig).
	WithConfig bool `json:"with_config,omitempty"`
}

// ListFilesIn names a revision of the org's repo.
type ListFilesIn struct {
	Revision string `json:"revision"`
	// WithConfig lists the repo's settings file with its other files.
	WithConfig bool `json:"with_config,omitempty"`
}

// ListFilesOut is a revision's files, as a publish carries them, by path.
type ListFilesOut struct {
	Files []File `json:"files"`
}

// GetBlobsIn is blob ids to read.
type GetBlobsIn struct {
	OIDs []string `json:"oids"`
}

// GetBlobsOut is the blobs' content, in the order named, and the ids that
// didn't fit in this answer, to ask again for.
type GetBlobsOut struct {
	Blobs   []BlobContent `json:"blobs"`
	Omitted []string      `json:"omitted"`
}

// BlobContent is one blob's content.
type BlobContent struct {
	OID     string `json:"oid"`
	Content []byte `json:"content"`
}

// File is one file to publish.
type File struct {
	Path string `json:"path"`
	OID  string `json:"oid"`
}

// PublishOut is a publish's outcome.
type PublishOut struct {
	Revision string    `json:"revision"`
	Changed  bool      `json:"changed"`
	Replaced *Replaced `json:"replaced,omitempty"`
}

// Replaced is a published revision a publish replaced.
type Replaced struct {
	Revision    string    `json:"revision" doc:"the revision replaced"`
	PublishedBy *Person   `json:"published_by,omitempty" doc:"who published it"`
	PublishedAt time.Time `json:"published_at" doc:"when it was published" pretty:",moment"`
}

// DataSource is a data source as the server shows it: never its connection.
type DataSource struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ListDataSourcesOut is the org's data sources.
type ListDataSourcesOut struct {
	DataSources []DataSource `json:"data_sources"`
}

// RepoConfigOut is the org's repo settings, as they apply on it: the
// published .anfra/config.yml's, and an Admin's overrides.
type RepoConfigOut struct {
	Settings []RepoSetting `json:"settings"`
	// Revision is the published revision the repo's values come from.
	Revision string `json:"revision,omitempty"`
	// Error is why the settings in effect can't be used, if they can't.
	Error string `json:"error,omitempty"`
}

// RepoSetting is one setting, as it applies on the org.
type RepoSetting struct {
	// Key is its path in .anfra/config.yml: query.cache.max_duration.
	Key string `json:"key"`
	// Value is the value in effect, absent at anfra's default.
	Value any `json:"value,omitempty"`
	// Source is where Value comes from: "repo", "override" or "default".
	Source string `json:"source"`
	// Repo is the published file's value, absent when it doesn't set it.
	Repo     any           `json:"repo,omitempty"`
	Override *RepoOverride `json:"override,omitempty"`
	// Editable says whether the caller may change it.
	Editable bool `json:"editable"`
}

// RepoOverride is an Admin's override of a setting on the org.
type RepoOverride struct {
	Value     any       `json:"value"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RefusedSettings are a settings_change_forbidden's details.
type RefusedSettings struct {
	Settings []RefusedSetting `json:"settings"`
}

// RefusedSetting is one setting a publish may not change.
type RefusedSetting struct {
	Key        string `json:"key"`
	Permission string `json:"permission"`
}

// CreateDataSourceIn is a new data source.
type CreateDataSourceIn struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Connection map[string]any `json:"connection"`
	Settings   map[string]any `json:"settings,omitempty"`
}

// TestDataSourceIn is a data source to check: one not yet added, by type and
// connection (and a name, to say in the answer).
type TestDataSourceIn struct {
	Name       string         `json:"name,omitempty"`
	Type       string         `json:"type,omitempty"`
	Connection map[string]any `json:"connection,omitempty"`
	Settings   map[string]any `json:"settings,omitempty"`
}

// TestDataSourceOut is a check's outcome.
type TestDataSourceOut struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
