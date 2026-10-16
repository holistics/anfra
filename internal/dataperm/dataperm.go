// Package dataperm carries the data-level permissions one invocation runs
// under: whether its queries are governed, and the caller attributes that row
// and column rules are written against.
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

// Set is the data permissions for one invocation: whether its queries are
// governed, and the attributes the repo's rules are evaluated against.
//
// A governed query reaches the data only through the semantic layer: SQL the
// engine compiled from a dataset, with the rules in it. An ungoverned one may
// also read it as written (raw SQL), which nothing can apply a rule to. The
// names are the access path's, not the caller's: Governed with no attributes is
// governed, with no rule filtering anything for this caller.
//
// Its fields are unexported, so a Set cannot be produced by a struct literal.
// That is the point: "ungoverned" is an answer someone has to give
// (Ungoverned), not a state reached by leaving a field blank. An undecided
// Set is refused rather than treated as permissive.
//
// It carries attributes and not rules, deliberately. A rule is expected to be
// declared in the repo's AML, against attribute names, with the request
// supplying the values — so attributes are the durable half of this and belong
// here now. Who skips a rule (an admin, say) is the rules' to say, through an
// attribute; it is not a third state of Set. Whether a host also needs to pass
// rules of its own per request is undesigned and waits on the upstream work in
// T3; adding a field to Set later costs nothing, whereas exporting a
// placeholder through the engine facade and then changing it does.
type Set struct {
	decided  bool
	governed bool
	// attributes are what the rules are evaluated against.
	attributes Attributes
}

// Ungoverned states that the caller's queries may read the data as written:
// raw SQL runs, and no rule applies. It is the CLI's answer, whose user owns
// the repo they are pointed at, and a platform's for a caller it granted a
// data source itself. It is a decision rather than a default.
func Ungoverned() Set { return Set{decided: true} }

// Governed states that the caller's queries reach the data only through the
// semantic layer, with the repo's rules evaluated against attrs: raw SQL is
// refused, even with no attributes.
func Governed(attrs Attributes) Set {
	return Set{decided: true, governed: true, attributes: attrs}
}

// Decided reports whether anyone has stated what applies. A zero Set has not,
// and Dispatch refuses to run a data-reading command with one.
func (s Set) Decided() bool { return s.decided }

// Governed reports whether the caller's queries are governed — Governed, even
// with no attributes — rather than stated Ungoverned. A query nothing can
// govern (raw SQL) runs only when it is false.
func (s Set) Governed() bool { return s.governed }

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
