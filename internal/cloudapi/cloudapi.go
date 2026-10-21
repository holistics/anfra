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
	OpListDataSources  = "data_sources.list"
	OpCreateDataSource = "data_sources.create"
	OpTestDataSource   = "data_sources.test"
	OpListAppOrigins   = "app_origins.list"
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
)

// MissingBlobs are the details of blobs_missing.
type MissingBlobs struct {
	OIDs []string `json:"oids"`
}

// StatusOut is the org's published revision, and the server's publishing limits.
type StatusOut struct {
	Revision    string          `json:"revision"`
	PublishedBy *Person         `json:"published_by,omitempty"`
	PublishedAt time.Time       `json:"published_at"`
	Publish     PublishSettings `json:"publish"`
}

// PublishSettings are how to publish to the server.
type PublishSettings struct {
	Limits Limits `json:"limits"`
}

// Limits are the server's bounds on publishing, which the client batches by.
type Limits struct {
	Files       int `json:"files"`
	UploadBlobs int `json:"upload_blobs"`
	UploadBytes int `json:"upload_bytes"`
}

// Person is who published a revision. Email is empty once the account is gone,
// or has left the org.
type Person struct {
	ID    string `json:"id"`
	Email string `json:"email"`
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
	Revision    string    `json:"revision"`
	PublishedBy *Person   `json:"published_by,omitempty"`
	PublishedAt time.Time `json:"published_at"`
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

// AppOriginsOut is the origins the org's Data Apps may load from beyond the SDK's
// own: each https://<host>[:<port>].
type AppOriginsOut struct {
	Origins []string `json:"origins"`
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
