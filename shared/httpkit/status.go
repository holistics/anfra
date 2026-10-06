package httpkit

import (
	"context"
	"net/http"
	"slices"

	"github.com/holistics/anfra/shared/apperr"
)

// Codes is what a server answers its clients with: the public codes of its own
// namespaces, besides apperr's generic ones, which every server serves, and the
// HTTP status of each. A code carries no status: only served codes reach the
// wire, and HTTP is the one transport that has a status to give them.
type Codes struct {
	// Namespaces are the server's own catalogs.
	Namespaces []apperr.Namespace
	// Imported are a library's public codes the server serves as they are, one
	// by one: a library whose contract is the server's for the operations it
	// serves, as the engine's is a hosted server's for the engine's ops. The library's other
	// codes still fail closed.
	Imported []apperr.Code
	// Status is the status of each of their public codes. The generic codes'
	// statuses are known here.
	Status map[apperr.Code]int
}

// genericStatus is the status of each of apperr's generic codes.
// TestEveryGenericCodeHasAStatus holds it to apperr.
var genericStatus = map[apperr.Code]int{
	apperr.InternalServerError:     http.StatusInternalServerError,
	apperr.InvalidRequest.Code():   http.StatusBadRequest,
	apperr.NotFound:                http.StatusNotFound,
	apperr.ValidationFailed.Code(): http.StatusUnprocessableEntity,
}

// Served is the public code a client is answered with for c: its public code,
// when that is generic, in one of the server's namespaces or imported, and
// internal_server_error otherwise. A library's code nobody translated or
// imported therefore fails closed.
func (cs Codes) Served(c apperr.Code) apperr.Code {
	pub := c.Public()
	if ns := pub.Namespace(); ns == apperr.Generic || slices.Contains(cs.Namespaces, ns) || slices.Contains(cs.Imported, pub) {
		return pub
	}
	return apperr.InternalServerError
}

// StatusOf is the HTTP status c is answered with: its served code's. A served
// code missing from Status is answered with 500; a server's test should keep that
// from happening.
func (cs Codes) StatusOf(c apperr.Code) int {
	pub := cs.Served(c)
	if s, ok := genericStatus[pub]; ok {
		return s
	}
	if s, ok := cs.Status[pub]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// Render is err's wire form: the response of its served code, with that code's
// status. The status is HTTP's, repeated in the body so the body is the same over
// every transport.
func (cs Codes) Render(err error, requestID string) apperr.Response {
	e := apperr.From(err)
	if pub := e.Code.Public(); cs.Served(pub) != pub {
		// The library's code is not ours to show. Kept as the inner error, for
		// the log; the steps still say what was being done, which stays true.
		f := apperr.From(apperr.Encapsulate(err, apperr.InternalServerError, "unserved code "+pub.Qualified()))
		f.Steps = e.Steps
		e = f
	}
	resp := e.Response(requestID)
	resp.Status = cs.StatusOf(e.Code)
	return resp
}

type codesKey struct{}

// codesOf is the Codes the request is served with, as Wrap put them in its
// context; outside Wrap, the generic codes alone.
func codesOf(ctx context.Context) Codes {
	cs, _ := ctx.Value(codesKey{}).(Codes)
	return cs
}
