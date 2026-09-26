// Package dataperm carries the data-level permissions one invocation runs
// under: the caller attributes that row and column restrictions are written
// against.
//
// This is the engine's entire share of authorization, and it is deliberately
// small. Deciding *whether* a caller may run a command — roles, grants,
// memberships — belongs to whatever is hosting the engine, because only the
// host has a user model. What cannot be done anywhere else is injecting
// predicates while AML/AQL is compiled, so that is what lives here.
//
// Out of scope, by design:
//
//   - Roles and permissions. The engine has no notion of either.
//   - Tenancy. The engine does not know what a tenant is; see
//     internal/attribution for the identity that travels for logging only.
//   - Filtering rows after the warehouse returns them. That is slow and it
//     leaks the moment a code path forgets; restrictions are compiled in.
package dataperm

// Attributes are the caller attributes that restrictions reference, such as a
// region, a department or an email. They are decision-bearing: the compiler
// reads them, and changing one changes which rows come back. That is the whole
// difference between this package and internal/attribution.
//
// The engine does not know or care where an attribute came from — a role, a
// group, a user record. The host resolves all of that before calling.
type Attributes map[string]any

// Set is the data permissions for one invocation.
//
// Its fields are unexported, so a Set cannot be produced by a struct literal.
// That is the point: "no restrictions apply" is an answer someone has to give
// (Unrestricted), not a state reached by leaving a field blank. An undecided
// Set is refused rather than treated as permissive.
//
// It carries attributes and not rules, deliberately. A restriction is expected
// to be declared in the repo's AML, against attribute names, with the request
// supplying the values — so attributes are the durable half of this and belong
// here now. Whether a host also needs to pass rules of its own per request is
// undesigned and waits on the upstream work in T3; adding a field to Set later
// costs nothing, whereas exporting a placeholder through the engine facade and
// then changing it does.
type Set struct {
	decided    bool
	attributes Attributes
}

// Unrestricted states that no data restrictions apply. It is the CLI's answer,
// whose user owns the repo they are pointed at, and it is a decision rather
// than a default.
func Unrestricted() Set { return Set{decided: true} }

// Restricted carries the attributes of a caller whose access is conditional on
// them.
func Restricted(attrs Attributes) Set {
	return Set{decided: true, attributes: attrs}
}

// Decided reports whether anyone has stated what applies. A zero Set has not,
// and Dispatch refuses to run a data-reading command with one.
func (s Set) Decided() bool { return s.decided }

// Attributes returns the attributes restrictions are evaluated against. The map
// is copied, so a caller cannot reach back into the Set and change what applies
// after it was constructed.
func (s Set) Attributes() Attributes {
	if len(s.attributes) == 0 {
		return nil
	}
	out := make(Attributes, len(s.attributes))
	for k, v := range s.attributes {
		out[k] = v
	}
	return out
}
