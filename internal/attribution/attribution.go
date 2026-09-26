// Package attribution carries who an invocation is for, so that a log line, a
// span or an audit record can name them.
//
// It is tracing baggage and nothing else. No command reads it, the query layer
// does not read it, and it never reaches the compiler — a depguard rule in
// .golangci.yml enforces that rather than trusting this comment.
//
// The distinction from internal/dataperm is the whole reason this is a separate
// package, because the two look alike and must not merge. An attribution field
// named "user_id" and a dataperm attribute of the same name may hold the same
// string with entirely different standing: the first is for a human reading
// logs, the second changes which rows a query returns. A host that wants the
// caller's email to drive a row rule puts it in dataperm.Attributes explicitly.
package attribution

// Fields names the caller of one invocation, as key/value pairs destined for a
// log or span.
//
// It is a map rather than a struct because the engine genuinely does not
// interpret any of it. A `TenantID` field would have put tenancy into the
// engine's type system — the very thing the engine is supposed to have no
// notion of — and would have needed an engine change every time a host wanted
// to carry one more identifier. Hosts put in whatever they can attribute by:
// a tenant, a user, a session, a request id.
//
// Values are strings because the destination is a log field. Rendering is the
// host's job, which keeps log output predictable and stops a live object being
// carried across the boundary by accident.
type Fields map[string]string
