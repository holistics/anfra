package telemetry

import (
	"context"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
	"unicode"

	"go.opentelemetry.io/otel/baggage"
)

// A caller can name itself in W3C baggage (docs/designs/telemetry.md): its
// session, its tenant, its user. Baggage is the caller's, not anfra's, so a
// host lets through, wherever baggage enters it, only the members its
// settings name (BaggageIncludeVar, AllowedBaggage), never one in anfra's own
// keys, and only short, plain values, whoever the caller. What gets through is
// what a span records, and what anfra sends on to its sidecars. It never
// sends baggage out of its deployment.
//
// What a host names its caller by in code (WithAttributes, an invocation's
// Attribution) is its own choice, so is recorded whatever its keys.

// maxBaggageValue bounds how long a caller's baggage value a span records may
// be.
const maxBaggageValue = 128

// baggageKeys are the keys anfra declares that a caller may still set through
// baggage: the ids it names a caller by. Every other declared key is anfra's.
var baggageKeys = map[string]bool{"gen_ai.conversation.id": true}

// AllowedBaggage is the members of a caller's baggage b that s lets into
// anfra: matching an include pattern (BaggageIncludeVar), not anfra's, and
// short. A host applies it wherever baggage
// enters, from any caller: an HTTP request's header (httpkit), an MCP call's
// _meta, the CLI's BAGGAGE.
func AllowedBaggage(b baggage.Baggage, s Settings) baggage.Baggage {
	kept := safeMembers(b, func(key string) bool { return includes(s.BaggageInclude(), key) })
	out, err := baggage.New(kept...)
	if err != nil {
		return baggage.Baggage{}
	}
	return out
}

// WithAllowedBaggage is ctx with only the members of its baggage that s lets
// into anfra (AllowedBaggage).
func WithAllowedBaggage(ctx context.Context, s Settings) context.Context {
	b := baggage.FromContext(ctx)
	if b.Len() == 0 {
		return ctx
	}
	return baggage.ContextWithBaggage(ctx, AllowedBaggage(b, s))
}

// WithCallerBaggage is ctx with fields, the ids a host names its caller by
// (WithAttributes), also in its baggage, as far as s lets them in
// (AllowedBaggage): sent with each call made in ctx, so the spans of the
// processes it calls, a forwarded anfra serve or a sidecar, carry them too. Of
// anfra's own keys, only gen_ai.conversation.id gets through. Empty values are left out.
func WithCallerBaggage(ctx context.Context, fields map[string]string, s Settings) context.Context {
	b := baggage.FromContext(ctx)
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		if fields[k] == "" {
			continue // as WithAttributes leaves it out, so a member the caller sent stays
		}
		m, err := baggage.NewMemberRaw(k, fields[k])
		if err != nil {
			continue
		}
		if next, err := b.SetMember(m); err == nil {
			b = next
		}
	}
	if b.Len() == 0 {
		return ctx
	}
	return baggage.ContextWithBaggage(ctx, AllowedBaggage(b, s))
}

// safeMembers is b's members that can name a caller and that keep says get
// in: not anfra's, short and plain.
func safeMembers(b baggage.Baggage, keep func(key string) bool) []baggage.Member {
	var out []baggage.Member
	for _, m := range b.Members() {
		if keep(m.Key()) && !anfras(m.Key()) && plain(m.Value()) {
			out = append(out, m)
		}
	}
	return out
}

// callerBaggage is ctx's baggage, as fields: what got into anfra
// (AllowedBaggage), so the members a span records are those its host let in,
// checked again for safety only.
func callerBaggage(ctx context.Context) map[string]string {
	members := safeMembers(baggage.FromContext(ctx), func(string) bool { return true })
	if len(members) == 0 {
		return nil
	}
	out := make(map[string]string, len(members))
	for _, m := range members {
		out[m.Key()] = m.Value()
	}
	return out
}

// includes reports whether a pattern names key.
func includes(patterns []string, key string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, key); ok {
			return true
		}
	}
	return false
}

// anfras reports whether key is one of anfra's own, which a caller's baggage
// never sets: in its namespace, or declared, other than a caller's id.
func anfras(key string) bool {
	if strings.HasPrefix(key, "anfra.") {
		return true
	}
	return Declared(key) && !baggageKeys[key]
}

// plain reports whether v can name a caller: short, with no control character.
func plain(v string) bool {
	if v == "" || len(v) > maxBaggageValue {
		return false
	}
	return !strings.ContainsFunc(v, unicode.IsControl)
}

// callerFields is everything ctx names its caller by: its baggage, as its
// host let it in, and what its host put there (WithAttributes), which wins on
// the same key. Each key is a caller's key from then on (callerKey).
func callerFields(ctx context.Context) map[string]string {
	fromBaggage := callerBaggage(ctx)
	host := attributesOf(ctx)
	if len(fromBaggage) == 0 {
		return host
	}
	fields := maps.Clone(fromBaggage)
	maps.Copy(fields, host)
	for k := range fromBaggage {
		markCallerKey(k)
	}
	return fields
}

// callerKeys are the keys a host or an allowed caller's baggage has named a
// caller by in this process, which Filter records as metadata though anfra
// doesn't declare them: a host names its caller knowingly.
var callerKeys sync.Map // string → struct{}

// markCallerKey makes key a caller's key.
func markCallerKey(key string) {
	callerKeys.Store(key, struct{}{})
}

// callerKey reports whether key names a caller (markCallerKey).
func callerKey(key string) bool {
	_, ok := callerKeys.Load(key)
	return ok
}
