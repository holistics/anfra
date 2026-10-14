// Package remoteapi is the publish protocol: how the anfra CLI publishes a local
// repo to a server that hosts it, as `anfra publish` does. The CLI is its client
// (Client); a server implements it by serving these ops with these types, as
// anfra-cloud does, so the two cannot drift.
//
// Its contract is api/remote.yaml, generated from this package (Spec). Its
// clients are any the server does not ship with (the CLI, scripts, agents), which
// it cannot reload, so it changes additively (new ops, new optional inputs, new
// outputs, new codes), and a client ignores what it does not know. A breaking
// change is avoided; when one is unavoidable, it is declared, as for the core API,
// and CI refuses one that is not. Before anfra is stable, the CLI and the server
// are meant to run matching releases; a compatibility number, which the server
// checks to refuse a client it cannot serve, comes with stability
// (anfra-cloud's PUBLISH.md §Compatibility).
//
// A repo publishes to an org on an anfra remote server, named by the org's
// address there, https://bi.example.com/o/acme (Remote); the server serves the
// org's ops at <origin>/api/o/<org>/<op>. The address is what `anfra publish
// --set-remote` records (remote.url in a repo's .anfra/config.yml). The anfra
// remote server is not a git remote, nor the external git repo a server may
// connect an org's repo to. A publish is:
//
//  1. repos.status: what is published, and the server's limits;
//  2. repos.list_missing_blobs: which of the files' blob ids (BlobID) the server
//     lacks;
//  3. repos.create_blobs: upload those, in batches within the limits;
//  4. repos.publish: publish every file, by path and blob id, as the whole repo.
//
// Before it, `anfra publish` may add the data sources the repo uses and the org
// lacks (data_sources.list, data_sources.create), when the person agrees: a
// connection is never sent unasked.
package remoteapi

import (
	"time"

	"github.com/holistics/anfra/shared/apperr"
)

// NS is the protocol's namespace, for its codes.
var NS = apperr.DefineNamespace("anfra_remote")

// The protocol's codes, beyond apperr's generic ones (validation_failed for
// files the server will not take, one violation per file) and a server's own
// (unauthenticated, forbidden, unavailable).
var (
	// RepoInvalid: a publish whose files fail the engine's validation; nothing is
	// published. Its details are the engine's validation, as `anfra validate`
	// answers.
	RepoInvalid = apperr.DefinePublicCodeWith[RepoProblems](NS, "repo_invalid", apperr.User, "The repo has errors. Fix them, then publish again.")
	// BlobsMissing: a publish naming content the server does not have; the client
	// uploads it and publishes again.
	BlobsMissing = apperr.DefinePublicCodeWith[MissingBlobs](NS, "blobs_missing", apperr.Client, "Some files' content has not been uploaded.")
)

// RepoProblems are the details of repo_invalid.
type RepoProblems struct {
	Validation any `json:"validation" doc:"The engine's validation of the files published, as core.validate answers: its diagnostics and reports."`
}

// MissingBlobs are the details of blobs_missing.
type MissingBlobs struct {
	OIDs []string `json:"oids" doc:"The blob ids to upload with repos.create_blobs."`
}

// Group is a group of the protocol's ops: the first segment of their names, and
// its summary. A server serving the ops declares their groups with these, so the
// names cannot drift from the ops'.
type Group struct {
	Name, Summary string
}

// The protocol's groups.
var (
	ReposGroup = Group{
		Name:    "repos",
		Summary: "Your org's repo: what is published, and publishing a new version of it, as `anfra publish` does.",
	}
	DataSourcesGroup = Group{
		Name:    "data_sources",
		Summary: "Your org's data sources: the warehouses its models query, and their connections.",
	}
)

// Groups are the protocol's groups; every op is in one.
var Groups = []Group{ReposGroup, DataSourcesGroup}

// Op is one of the protocol's ops, as it defines it: what a server serves it
// with, beside its types.
type Op struct {
	Name, Summary, Doc string
	// Errors are the protocol's codes the op answers with; a server adds its own.
	Errors     []apperr.AnyCode
	ReadOnly   bool
	Idempotent bool
}

// The ops, in the order a publish calls them.
var (
	// StatusOp: struct{} in, StatusOut out.
	StatusOp = Op{
		Name:     "repos.status",
		Summary:  "The published revision of the org's repo, who published it and when, and the limits for publishing.",
		Doc:      "Read before publishing, to show what a publish will replace, and to batch uploads within the limits.",
		ReadOnly: true,
	}
	// ListMissingBlobsOp: ListMissingBlobsIn in, ListMissingBlobsOut out.
	ListMissingBlobsOp = Op{
		Name:    "repos.list_missing_blobs",
		Summary: "Which of the given blob ids the org's repo does not have yet: the content to upload before publishing.",
		Doc: "Compute each file's git blob id locally and name them all; upload only what is listed, with repos.create_blobs. " +
			"A repeat publish of a small edit uploads one blob.",
		Errors:   []apperr.AnyCode{apperr.ValidationFailed},
		ReadOnly: true,
	}
	// CreateBlobsOp: CreateBlobsIn in, CreateBlobsOut out.
	CreateBlobsOp = Op{
		Name:       "repos.create_blobs",
		Summary:    "Upload file contents to the org's repo, for a publish to name by blob id.",
		Doc:        "Upload what repos.list_missing_blobs listed, in calls within limits.upload_blobs and limits.upload_bytes. Uploading content the repo has is harmless.",
		Errors:     []apperr.AnyCode{apperr.ValidationFailed},
		Idempotent: true,
	}
	// PublishOp: PublishIn in, PublishOut out.
	PublishOp = Op{
		Name:    "repos.publish",
		Summary: "Publish files as the whole of the org's repo, once they pass validation.",
		Doc: "List every file, as path and blob id: a file not listed is deleted. Upload the content first; blobs_missing names any that is not. " +
			"Files that fail validation are refused with repo_invalid, carrying the engine's problems, and nothing is published. " +
			"The last publish wins: the answer says what it replaced.",
		Errors:     []apperr.AnyCode{apperr.ValidationFailed, RepoInvalid, BlobsMissing},
		Idempotent: true,
	}
)

// The data source ops `anfra publish` uses, to add what the repo uses and the org
// lacks. A server's own data source ops (changing, removing) are its own.
var (
	// ListDataSourcesOp: struct{} in, ListDataSourcesOut out.
	ListDataSourcesOp = Op{
		Name:     "data_sources.list",
		Summary:  "The org's data sources, by name and type.",
		Doc:      "Connections are never returned, by this op or any other.",
		ReadOnly: true,
	}
	// CreateDataSourceOp: CreateDataSourceIn in, DataSource out.
	CreateDataSourceOp = Op{
		Name:    "data_sources.create",
		Summary: "Add a data source to the org: a name its models refer to, and the connection to the warehouse.",
		Doc:     "The connection is kept sealed, and never returned. A name the org already has is refused.",
		Errors:  []apperr.AnyCode{apperr.ValidationFailed},
	}
	// TestDataSourceOp: TestDataSourceIn in, TestDataSourceOut out.
	TestDataSourceOp = Op{
		Name:    "data_sources.test",
		Summary: "Check that the server can connect to a data source: one the org has, by name, or one not yet added, by type and connection.",
		Doc: "Runs a trivial statement on the warehouse. A connection that fails is an answer, ok false with the warehouse's reason, " +
			"not an error: the check ran. Test a connection before adding it, or before replacing one, as well as after.",
		Errors:   []apperr.AnyCode{apperr.ValidationFailed},
		ReadOnly: true,
	}
)

// Ops are the protocol's ops.
var Ops = []Op{StatusOp, ListMissingBlobsOp, CreateBlobsOp, PublishOp, ListDataSourcesOp, CreateDataSourceOp, TestDataSourceOp}

// DataSource is a data source as a response shows it: never its connection.
type DataSource struct {
	Name      string    `json:"name" doc:"What AML's data_source_name refers to."`
	Type      string    `json:"type" doc:"The database type, which is also the SQL dialect: postgresql, bigquery, snowflake."`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListDataSourcesOut is the org's data sources.
type ListDataSourcesOut struct {
	DataSources []DataSource `json:"data_sources"`
}

// TestDataSourceIn is a data source to check: one the org has, by Name alone, or
// one it has not, by Type and Connection (and a Name, to say in the answer).
type TestDataSourceIn struct {
	Name       string         `json:"name,omitempty" doc:"A data source the org has; alone, it is the one checked."`
	Type       string         `json:"type,omitempty" doc:"With connection: the database type of a data source not yet added."`
	Connection map[string]any `json:"connection,omitempty" doc:"With type: the connection to check, not kept. Write-only."`
}

// TestDataSourceOut is a check's outcome.
type TestDataSourceOut struct {
	OK      bool   `json:"ok" doc:"Whether the server connected and ran a trivial statement."`
	Message string `json:"message,omitempty" doc:"Why not, in the warehouse's words, when not ok."`
}

// CreateDataSourceIn is a new data source.
type CreateDataSourceIn struct {
	Name       string         `json:"name" doc:"What AML's data_source_name refers to: a letter, then letters, digits and underscores, at most 63."`
	Type       string         `json:"type" doc:"The database type: postgresql, bigquery, snowflake."`
	Connection map[string]any `json:"connection" doc:"The database driver's settings (host, user, password and so on), passed to it unchanged. Write-only: no response ever includes it."`
}

// StatusOut is the org's published revision, and the server's publishing limits.
type StatusOut struct {
	Revision    string          `json:"revision" doc:"The published revision. Opaque: show it, do not parse it."`
	PublishedBy *Person         `json:"published_by,omitempty" doc:"Who published it; absent for the repo's starting files, which the org's creation put there."`
	PublishedAt time.Time       `json:"published_at"`
	Publish     PublishSettings `json:"publish"`
}

// PublishSettings are how to publish to this server.
type PublishSettings struct {
	Limits Limits `json:"limits"`
}

// Limits are a server's bounds on publishing. A client batches by them, not by
// numbers compiled into it, so a server can raise them without a client release.
type Limits struct {
	Files       int `json:"files" doc:"The most files one publish may have, and the most blob ids one repos.list_missing_blobs call may name."`
	UploadBlobs int `json:"upload_blobs" doc:"The most blobs one repos.create_blobs call may carry."`
	UploadBytes int `json:"upload_bytes" doc:"The most content bytes one repos.create_blobs call may carry, before base64: also the largest file."`
}

// Person is who published a revision. Email is empty once the account is gone,
// or has left the org.
type Person struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// ListMissingBlobsIn is blob ids a client computed from its files.
type ListMissingBlobsIn struct {
	OIDs []string `json:"oids" doc:"Git blob ids of the files to publish: sha1 of \"blob <size>\\0\" and the content. At most limits.files."`
}

// ListMissingBlobsOut is those the server lacks, in the order given.
type ListMissingBlobsOut struct {
	OIDs []string `json:"oids" doc:"The blob ids to upload with repos.create_blobs before publishing."`
}

// CreateBlobsIn is content to store.
type CreateBlobsIn struct {
	Blobs [][]byte `json:"blobs" doc:"File contents, base64. At most limits.upload_blobs, of at most limits.upload_bytes in all."`
}

// CreateBlobsOut is the stored content's blob ids, in order: the client's own
// BlobID of each, or the two hash differently.
type CreateBlobsOut struct {
	OIDs []string `json:"oids"`
}

// PublishIn is the files to publish, as the whole repo.
type PublishIn struct {
	Files   []File `json:"files" doc:"Every file of the repo, as it is to be published: a file not listed is deleted. At most limits.files."`
	Message string `json:"message,omitempty" doc:"What the publish is, for the repo's history."`
}

// File is one file to publish.
type File struct {
	Path string `json:"path" doc:"Slash-separated, relative to the repo root."`
	OID  string `json:"oid" doc:"Its content's git blob id, uploaded with repos.create_blobs."`
}

// PublishOut is a publish's outcome.
type PublishOut struct {
	Revision string    `json:"revision" doc:"The revision now published."`
	Changed  bool      `json:"changed" doc:"False when the files were already the published ones: nothing changed."`
	Replaced *Replaced `json:"replaced,omitempty" doc:"The published revision this publish replaced; absent when nothing changed."`
}

// Replaced is a published revision a publish replaced.
type Replaced struct {
	Revision    string    `json:"revision"`
	PublishedBy *Person   `json:"published_by,omitempty" doc:"Who published it; absent for the repo's starting files."`
	PublishedAt time.Time `json:"published_at"`
}
