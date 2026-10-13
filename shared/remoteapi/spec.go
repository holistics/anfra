package remoteapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
)

// Statuses are the HTTP statuses of the protocol's codes, for a server's status
// table.
var Statuses = map[apperr.Code]int{
	RepoInvalid.Code(): http.StatusUnprocessableEntity,
	// The request is well formed; the server's state is not yet what it needs.
	BlobsMissing.Code(): http.StatusConflict,
}

// Spec is the protocol's contract, api/remote.yaml: its ops, at
// <origin>/api/o/{org}/<op>, with their types and codes. A server serving them
// publishes the same ops in its own contract; this one is what a client is built
// against, and what CI holds to additive change.
func Spec() ([]byte, error) {
	reg := apikit.NewRegistry(apikit.RegistryConfig[struct{}]{Namespace: NS})
	reg.Group(Group, GroupSummary)
	register[struct{}, StatusOut](reg, StatusOp)
	register[ListMissingBlobsIn, ListMissingBlobsOut](reg, ListMissingBlobsOp)
	register[CreateBlobsIn, CreateBlobsOut](reg, CreateBlobsOp)
	register[PublishIn, PublishOut](reg, PublishOp)
	h := apikit.HTTP[struct{}]{
		Codes:   httpkit.Codes{Namespaces: []apperr.Namespace{NS}, Status: Statuses},
		Title:   "anfra remote server",
		Version: "0", // the contract's, not a binary's
		Request: func(http.ResponseWriter, *http.Request) (struct{}, error) { return struct{}{}, nil },
		Base: func(apikit.Meta) *apikit.Base {
			return &apikit.Base{Path: "/o/{org}", Variables: map[string]*huma.ServerVariable{
				"org": {Default: "my-org", Description: "The org published to, by its slug: the end of its address, https://bi.example.com/o/<org>."},
			}}
		},
	}
	return h.Spec(apikit.NewRuntime(), reg)
}

// errNotServed is every op's answer here: this registry only describes them.
var errNotServed = errors.New("remoteapi: the protocol's ops are served by an anfra remote server, not here")

func register[In, Out any](reg *apikit.Registry[struct{}], o Op) {
	admit := apikit.Admission[struct{}, struct{}]{Admit: func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil }}
	apikit.Register(reg, admit, &apikit.Def[struct{}, In, Out]{
		Name: o.Name, Summary: o.Summary, Doc: o.Doc, Errors: o.Errors,
		ReadOnly: o.ReadOnly, Idempotent: o.Idempotent, HTTP: true,
		Handle: func(context.Context, struct{}, In) (Out, error) {
			var out Out
			return out, errNotServed
		},
	})
}
