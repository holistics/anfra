// Package requestid identifies one request: the id an error body carries, and
// that leads to the request's log line and its trace.
//
// It is ours and separate from the trace id, so each can be managed on its own
// terms; see error_handling.md §The request id. The HTTP middleware mints one
// per request and stores it in the context; every transport reads it from
// there. An MCP tool call arrives in its own POST, so it carries that POST's id.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"strings"
)

type key struct{}

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// New returns a fresh request id: "req_" and 128 random bits in lowercase
// base32.
func New() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	return "req_" + strings.ToLower(encoding.EncodeToString(b[:]))
}

// With returns ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the request id ctx carries, or "" when it carries none.
func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}
