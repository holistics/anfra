// Package querycache is the engine's query cache, as a platform embedding the
// engine provides it: the cache it sets on an Invocation (Cache), the store
// that keeps its metadata (Store, checked by engine/querycachetest), and what
// they deal in. Each name is an alias of the engine's own, so a value passes
// between the two unchanged. See docs/designs/query-cache.md.
//
// A platform turns the cache off for an invocation by setting no Cache on it.
package querycache

import (
	"github.com/holistics/anfra/internal/command"
	cache "github.com/holistics/anfra/internal/querycache"
)

type (
	// Cache is an invocation's query cache, set on an Invocation as
	// QueryCache: its state for status, its settings, and its manager for a
	// query. Nil: every query runs uncached, and status says nothing of one.
	Cache = command.QueryCache
	// Settings are how the cache answers a query that doesn't say otherwise.
	Settings = command.QueryCacheSettings
	// Defaults are the cache options of a query that sets none.
	Defaults = command.QueryCacheDefaults

	// Summarizer reports the cache's state, as status does.
	Summarizer = cache.Summarizer
	// Summary is the cache's state, as status reports it.
	Summary = cache.Summary
	// State is one of the cache's states (Ready, …).
	State = cache.State

	// Manager is the cache's algorithm over a Store: lookups, writes,
	// cleaning and sweeping. Cache.Manager gives one, made by NewManager.
	Manager = cache.Manager
	// Config is a Manager's store, lake, namespace and timings.
	Config = cache.Config
	// Lake is what a Manager needs of canal-lake besides reading and writing
	// entries: dropping their files.
	Lake = cache.Lake
	// Write is a write of a key's result in progress.
	Write = cache.Write

	// Store keeps the cache's metadata. A platform may implement it on its own
	// database; engine/querycachetest is what it must pass.
	Store = cache.Store
	// Entry is one write of a query's result: a physical entry.
	Entry = cache.Entry
	// EntryID identifies an Entry, and orders writes.
	EntryID = cache.EntryID
	// Key is a query's key in the cache.
	Key = cache.Key
	// KeyInput is what a key is made of (NewKey).
	KeyInput = cache.KeyInput
	// Tag is what a clean selects entries by.
	Tag = cache.Tag
	// TagKind is what a Tag selects by.
	TagKind = cache.TagKind
	// Expired is what a clean expired.
	Expired = cache.Expired
)

// The cache's states.
const (
	Ready        = cache.StateReady
	Incompatible = cache.StateIncompatible
	Unavailable  = cache.StateUnavailable
	Disabled     = cache.StateDisabled
)

// MaxDuration is the longest a query may ask the cache to keep its result,
// and accept a cached one for.
const MaxDuration = cache.MaxDuration

var (
	// ErrNotPending is a Store's refusal to commit or abandon a write that
	// isn't pending.
	ErrNotPending = cache.ErrNotPending
	// ErrNoDataSourceIdentity is NewKey's refusal of a data source the host
	// gave no identity (DataSource.Identity): its queries run uncached.
	ErrNoDataSourceIdentity = cache.ErrNoDataSourceIdentity
)

// NewManager makes a Manager over c's store.
func NewManager(c Config) (*Manager, error) { return cache.NewManager(c) }

// NewKey is the key of the query in.
func NewKey(in KeyInput) (Key, error) { return cache.NewKey(in) }

// ValidNamespace reports whether ns can prefix a file's name in canal-lake.
func ValidNamespace(ns string) bool { return cache.ValidNamespace(ns) }
