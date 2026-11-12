package querycache

import "context"

// Summarizer reports the query cache's state, as anfra status shows it,
// without changing anything. anfra's host implements it over the repo's store;
// a platform, over its own.
type Summarizer interface {
	Summarize(ctx context.Context) Summary
}

// State is the query cache's state: one for each thing the reader does about it.
type State string

const (
	// StateReady: the cache can be used. Nothing to do.
	StateReady State = "ready"
	// StateIncompatible: a newer anfra changed the store in a way this one
	// can't read. Update anfra. Only a store that processes of different
	// versions share can be in it.
	StateIncompatible State = "incompatible"
	// StateUnavailable: the store can't be used, for a reason Problem gives.
	StateUnavailable State = "unavailable"
	// StateDisabled: the cache is turned off, for a reason Reason gives.
	StateDisabled State = "disabled"
)

// Summary is the query cache's state, read without changing anything.
type Summary struct {
	State State `json:"state" enum:"ready,incompatible,unavailable,disabled" doc:"\"ready\": the cache can be used, and needs nothing. \"incompatible\": a newer anfra changed the cache's store in a way this anfra can't read. \"unavailable\": the cache's store can't be used, as \"problem\" says. \"disabled\": the query cache is turned off, as \"reason\" says. Queries run without the cache unless it's \"ready\"."`
	// Store is where the store is, when a person can act on it: anfra's file.
	// Absent when the cache is disabled, or the host doesn't say.
	Store   string `json:"store,omitempty" doc:"the query cache's store, a file. Absent when the cache is disabled, or the server keeps its store elsewhere."`
	Problem string `json:"problem,omitempty" doc:"what's wrong with the cache, when it is incompatible or unavailable"`
	Reason  string `json:"reason,omitempty" doc:"what turns the cache off, when it is disabled"`
	// Hint is what to do about the problem, for status to add to its hints.
	Hint string `json:"-"`
}
