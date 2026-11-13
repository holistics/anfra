// Package local is a local repo's query cache, as anfra's own process holds
// it: the repo's store, opened at first use, its canal-lake, started at first
// use, and the cache's state as anfra status reports it (a
// querycache.Summarizer). A platform serving managed repos
// supplies its own store and canal-lake instead. The cache's algorithm and
// contract are internal/querycache; anfra's store is store/sqlite.
//
// Import it as localquerycache, so a call site says what is local:
//
//	import localquerycache "github.com/holistics/anfra/internal/querycache/local"
//
//	cache := localquerycache.New(dir, localquerycache.Options{Lake: lake})
//
// See docs/designs/query-cache.md.
package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/querycache"
	sqlitestore "github.com/holistics/anfra/internal/querycache/store/sqlite"
)

// Cache is a repo's query cache in this process: its folder, whether it is
// on, its store once opened, and its canal-lake. A process makes one, and
// closes it as it ends.
type Cache struct {
	dir string
	// disabled says why the cache is off; empty when it is on.
	disabled  string
	lake      Lake
	namespace string
	// unusable is why the cache can't be used though it is on: its settings
	// are wrong. nil when they're right.
	unusable error
	settings command.QueryCacheSettings
	report   func(ctx context.Context, err error)
	notice   func(ctx context.Context, n Notice)

	mu    sync.Mutex
	store *sqlitestore.Store
}

// Lake is canal-lake as the cache runs it: started when first needed, and
// again if it exited (canallake.Sidecar).
type Lake interface {
	Start(ctx context.Context) error
	Close(ctx context.Context)
}

// Options are how a process runs the repo's query cache.
type Options struct {
	// Disabled says why the cache is off, as anfra status reports it; empty,
	// it is on.
	Disabled string
	// Lake is its canal-lake; nil when the cache is off.
	Lake Lake
	// Namespace is the subfolder of canal-lake's storage its files go in, as
	// the person running anfra gives it: word segments joined by "/", such as
	// prod or prod/eu; empty for the storage's top level
	// (docs/designs/query-cache.md#file-names).
	Namespace string
	// Default is the repo's cache.query.default: the cache options of a query
	// that sets none.
	Default command.QueryCacheDefaults
	// Report and Notice are how whoever runs anfra is told why a query ran
	// without the cache, and of what doesn't stop it (Open's notices). Nil
	// tells no one.
	Report func(ctx context.Context, err error)
	Notice func(ctx context.Context, n Notice)
}

// NamespaceEnv sets Options.Namespace.
const NamespaceEnv = "ANFRA_CACHE_QUERY_NAMESPACE"

// New is the query cache in dir, the repo's folder for it. A namespace
// canal-lake can't take leaves the cache unusable, which Open and Summarize
// report, rather than failing here: a cache problem never fails a command.
func New(dir string, o Options) *Cache {
	c := &Cache{dir: dir, disabled: o.Disabled, lake: o.Lake, report: o.Report, notice: o.Notice,
		settings: command.QueryCacheSettings{Disabled: o.Disabled, Default: o.Default}}
	if o.Namespace != "" {
		c.namespace = strings.TrimSuffix(o.Namespace, "/") + "/"
		if !querycache.ValidNamespace(c.namespace) {
			c.unusable = fmt.Errorf("%w: %s is %q", errBadNamespace, NamespaceEnv, o.Namespace)
		}
	}
	return c
}

var errBadNamespace = errors.New("the query cache's namespace isn't one canal-lake can take")

// Namespace prefixes the names of the cache's files in canal-lake: empty, or
// segments each ending in "/" (querycache.Config.Namespace).
func (c *Cache) Namespace() string { return c.namespace }

// StorePath is the store's file.
func (c *Cache) StorePath() string { return filepath.Join(c.dir, "store.sqlite") }

// LakeDir is canal-lake's storage: its files, and DuckDB's temporary ones.
func (c *Cache) LakeDir() string { return LakeDir(c.dir) }

// LakeDir is canal-lake's storage for the query cache in dir, for starting
// the canal-lake the cache is then made with (New).
func LakeDir(dir string) string { return filepath.Join(dir, "lake") }

// Disabled says why the cache is off, and whether it is.
func (c *Cache) Disabled() (string, bool) { return c.disabled, c.disabled != "" }

// Notice is something to tell the person running anfra that doesn't stop the
// cache, with its code.
type Notice struct {
	Code    string
	Message string
}

// NoticeOrphanedFiles is a new store's lake folder already holding files, which
// no entry names.
const NoticeOrphanedFiles = "query_cache_orphaned_files"

// ErrDisabled is Open's refusal when the cache is off.
var ErrDisabled = errors.New("the query cache is disabled")

// Open is the store, opened (and created or brought up to date) the first time,
// and the same store after. A failure leaves it unopened, so the next Open
// tries again. Creating a store reports the files its lake folder already
// holds, which no entry names: anfra never deletes them itself
// (docs/designs/query-cache.md#file-names).
func (c *Cache) Open(ctx context.Context) (*sqlitestore.Store, []Notice, error) {
	if c.disabled != "" {
		return nil, nil, ErrDisabled
	}
	if c.unusable != nil {
		return nil, nil, c.unusable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.store != nil {
		return c.store, nil, nil
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create the query cache's folder: %w", err)
	}
	// The lake's files are measured as the store is created, under its write
	// lock: no other process can have written an entry of this store yet, so
	// no entry names them.
	var orphaned int64
	s, err := sqlitestore.Open(ctx, c.StorePath(), sqlitestore.Options{OnCreate: func() { orphaned = dirSize(c.LakeDir()) }})
	if err != nil {
		return nil, nil, err
	}
	c.store = s
	var notices []Notice
	if orphaned > 0 {
		notices = append(notices, Notice{Code: NoticeOrphanedFiles, Message: fmt.Sprintf(
			"The query cache's folder holds %s of files that the new cache doesn't use, probably left by an earlier one. To reclaim the space, delete %s while no anfra is running for this repo.",
			humanBytes(orphaned), c.LakeDir())})
	}
	return s, notices, nil
}

// StartLake starts canal-lake unless it runs already, for canal-query to write
// results to and read them from.
func (c *Cache) StartLake(ctx context.Context) error {
	if c.disabled != "" {
		return ErrDisabled
	}
	if c.unusable != nil {
		return c.unusable
	}
	if c.lake == nil {
		return errors.New("the query cache has no canal-lake")
	}
	return c.lake.Start(ctx)
}

// Close stops canal-lake and closes the store, those that were started.
//
// ctx carries the trace its log records join; it is not for cancellation.
func (c *Cache) Close(ctx context.Context) error {
	if c.lake != nil {
		c.lake.Close(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.store == nil {
		return nil
	}
	err := c.store.Close()
	c.store = nil
	return err
}

var _ command.QueryCache = (*Cache)(nil)

// Settings are the repo's cache settings, as a query uses them.
func (c *Cache) Settings() command.QueryCacheSettings { return c.settings }

// Manager is the cache ready for a query: the store open, created and its
// notices told the first time, and canal-lake started. An error is why it
// can't be used now; the next query tries again.
func (c *Cache) Manager(ctx context.Context) (*querycache.Manager, error) {
	store, notices, err := c.Open(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range notices {
		if c.notice != nil {
			c.notice(ctx, n)
		}
	}
	if err := c.StartLake(ctx); err != nil {
		return nil, fmt.Errorf("start canal-lake: %w", err)
	}
	// No Lake for the manager yet: only a sweep drops files, through
	// canal-query, which comes with the sweep.
	return querycache.NewManager(querycache.Config{Store: store, Namespace: c.namespace})
}

// Report tells whoever runs anfra why a query ran without the cache.
func (c *Cache) Report(ctx context.Context, err error) {
	if c.report != nil {
		c.report(ctx, err)
	}
}

// Summarize reads the cache's state: only the store's header, so it never
// creates, changes or migrates the store.
func (c *Cache) Summarize(ctx context.Context) querycache.Summary {
	if c.disabled != "" {
		return querycache.Summary{State: querycache.StateDisabled, Reason: c.disabled}
	}
	s := querycache.Summary{State: querycache.StateReady, Store: c.StorePath()}
	if c.unusable != nil {
		return c.explain(s, c.unusable)
	}
	v, err := sqlitestore.Peek(ctx, c.StorePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s
	case err == nil && v.Compatible():
		return s
	case err == nil:
		err = &sqlitestore.VersionError{Versions: v}
	}
	return c.explain(s, err)
}

// Explain is what a failure to open the store means to the person running
// anfra: a problem, and what to do about it.
func (c *Cache) Explain(err error) (problem, hint string) {
	s := c.explain(querycache.Summary{}, err)
	return s.Problem, s.Hint
}

func (c *Cache) explain(s querycache.Summary, err error) querycache.Summary {
	var ve *sqlitestore.VersionError
	switch {
	case errors.As(err, &ve):
		s.State = querycache.StateIncompatible
		s.Problem = fmt.Sprintf("The query cache's store is from a newer anfra, which changed it in a way this anfra can't read "+
			"(the store's schema is at version %d, and requires schema version %d or later, but this anfra's is version %d). Queries run without the cache.",
			ve.Installed, ve.RequiredByInstalled, sqlitestore.CurrentVersion)
		s.Hint = "Run `anfra update` to use the query cache again."
	case errors.Is(err, errBadNamespace):
		s.State = querycache.StateUnavailable
		s.Problem = fmt.Sprintf("%s isn't a namespace anfra can use. Queries run without the cache.", NamespaceEnv)
		s.Hint = fmt.Sprintf("Set %s to letters, digits and _ only, or several such names joined by /, such as prod or prod/eu.", NamespaceEnv)
	case errors.Is(err, sqlitestore.ErrNotStore):
		s.State = querycache.StateUnavailable
		s.Problem = "The query cache's store isn't a file anfra made. Queries run without the cache."
		s.Hint = fmt.Sprintf("Move or delete %s. anfra makes a new store at the next cached query.", c.StorePath())
	default:
		s.State = querycache.StateUnavailable
		s.Problem = fmt.Sprintf("anfra can't read the query cache's store: %v. Queries run without the cache.", err)
		s.Hint = "The repo's log file says more, and anfra tries the store again at the next cached query."
	}
	return s
}

// dirSize is the size of the files under dir: zero when there is none.
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a file we can't read is left out of the size
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
		}
		return nil
	})
	return size
}

// humanBytes is n as a person reads a size: 1.2 GB.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
