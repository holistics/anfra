# Query cache

> Status: proposal, 2026-11-11. Not built. This is anfra core's side: the cache anfra runs itself, and the contracts a platform builds on. A platform's own caching, its store and what it does through the extension points, is the platform's own design. Where a question is not settled, it says so in place (`> Open:`).

A query cache keeps a query's result, so that the same query, run again soon, is answered from the cache instead of the warehouse. This doc says what the cache promises callers, how anfra builds it, and the contracts a platform embedding the engine builds on: the store it may provide, and the extension points in the query path.

## What it is for

- **A Data App reloaded, or opened again.** Its queries are the same each time, and a warehouse query can take seconds and cost money (BigQuery, Snowflake).
- **An agent iterating.** It runs the same query again while it works on what comes after it.
- **Several callers asking the same thing**, such as a Data App shared on a server, within a short time.

Not for:

- **Making a query faster the first time.** A miss runs on the warehouse as it does today.
- **Answering a query from another query's result**: a different page, sort or column order, a result filtered down or re-aggregated. That is a requery, which anfra never does itself. A platform may, through the [extension points](#extension-points).
- **Results across repos or machines.** The cache is a repo's own, on the machine that ran the query.

## What callers see

The cache changes how fresh an answer may be, never its shape: a query answers the same columns, types and rows from the cache as from the warehouse.

### Turning it on

In the repo's settings, `.anfra/config.yml`:

```yaml
cache:
  query:
    default:
      duration: 1h   # how old a cached result a query accepts, unless it says otherwise
```

Without it, a query is cached only if it asks to be (its own `cache.duration`). A repo whose queries never ask costs nothing: no process starts and no file is written.

`ANFRA_QUERY_CACHE=off` turns the cache off for a whole process, whatever the repo says: for a run that must see live data, or to rule the cache out while debugging.

### A query's input

`query` (and the HTTP op and MCP tool, which share its input) takes a new, optional input, `cache`:

```yaml
cache:
  duration: 1h        # how old an entry this query accepts, and how long one it writes is kept
  refresh: true       # don't read an entry: run the query, and cache its result
  read_only: true     # use an entry if there is one, but don't cache this query's result
  cached_only: true   # use an entry, or fail: never run the query on the warehouse
```

| Input | Meaning |
|---|---|
| `cache.duration` | How old a cached result this query accepts, as a duration (`10m`, `1h`), and how long the result it writes stays cached. Without it, `cache.query.default.duration`. `0s` turns the cache off for the query: no entry is younger than it, so none is read, and one written with it would expire at once, so none is written. |
| `cache.refresh` | `true` runs the query on the warehouse whatever is cached, and caches its result. |
| `cache.read_only` | `true` uses a cached result if there is one, and otherwise runs the query without caching its result: for a one-off query that shouldn't take up the cache. |
| `cache.cached_only` | `true` uses a cached result, or fails with `cache_miss` without running the query: to find out whether a result is still there ([Re-reading a result](#re-reading-a-result)). |

Each flag changes one thing from the default, which is to use a cached result, or run the query and cache it. Two combinations are refused, since they can never do what they ask: `refresh` with `cached_only`, and `cached_only` with a `duration` of `0s`.

`cached_only`'s miss is a failure, not an empty result, so it can't be taken for a query that returns no rows. Its code is `cache_miss`, a caller's error, with the help: "The query's result isn't cached anymore. Run it without cached_only to get it again."

At the CLI, they are `--cache.duration`, `--cache.refresh`, `--cache.read-only` and `--cache.cached-only`, as anfra names an object's fields.

`cache` is not part of the query's `input`: it is how this run treats the cache, not part of the query a caller replays ([a query's input](query-input.md#what-it-holds)).

And one, in `input`, that isn't the cache's alone: `input.seeds.referenceTime`, the time the query runs as of. A seed is an input the server generates for each request when it's unset, and returns in the response, so a caller can run the same query the same way again ([a query's input](query-input.md#seeds)). The cache relies on it to find a result again ([Re-reading a result](#re-reading-a-result)).

### Which entry a query accepts

A query uses a cached entry only if the entry is both **unexpired** (its own `expires_at` is ahead) and **younger than the query's `cache.duration`**. A query that asks for fresher data than an entry was written for is never answered from data older than it accepts:

| Entry (written with, age) | Query's `cache.duration` | Used? |
|---|---|---|
| 1h, 50 min old | 10m | No: older than 10 minutes |
| 1h, 5 min old | 10m | Yes: 5 minutes old, and unexpired |
| 5m, 4 min old | 1h | Yes: unexpired |
| 5m, 6 min old | 1h | No: expired |

### A query's answer

Every answer of a query the cache applied to has `cache`, what the cache did:

```yaml
cache:
  key: 9f1c4e...                     # the cache key
  hit: true                          # answered from the cache
  cached_at: 2026-11-10T10:42:05Z    # when the rows were read from the warehouse
  expires_at: 2026-11-10T11:42:05Z   # when the entry stops being used
```

Every answer of an AQL query has `input`, as the query ran, its seeds filled in ([a query's input](query-input.md#the-response)):

```yaml
input:
  seeds:
    referenceTime: 2026-11-10T10:42:05Z    # generated: the request left it unset
```

On a miss whose result was cached, `hit` is `false` and `cached_at` is when this query ran. A query the cache did not apply to (`cache.duration: 0s`, the cache off) has no `cache`. In a stream, it is in the summary.

### Which commands

- **`query`, as a page or a stream:** cached.
- **`query export`:** reads a cached result if there is one, and caches what it ran otherwise. Its file is written as it is today.
- **`query compile` and `query validate`:** run nothing on the warehouse, so nothing to cache.

### In a Data App

Two of anfra's SDK plugins show the cache to the person using a Data App ([SDK plugins](sdk-plugins.md)):

- **`inspect`** shows each query's `cache` in its Queries tab: whether it was a hit, when its rows were read, when they expire, and its key. inspect's middleware already logs each query's answer, so it needs no new call.
- **`refresh`**, a new plugin, re-runs queries with `cache.refresh`, and without their seeds, so they run as of now:
  - **The whole app:** a command in the host's action bar ("Refresh data"), which re-runs every query of the mount.
  - **One block:** an item on the block's menu, in the frame's plugin layer as `view-underlying-data`'s is, which re-runs that block's queries.

  Its frame half asks the app model to re-run a block's queries, or all of them, with `cache.refresh` set: a small addition to the kernel's `Query`.

### Cleaning

`cache.clean`, a core op (and `anfra cache clean` at the CLI), removes entries from the repo's query cache: every entry it selects stops being used at once, and their files are removed as a sweep removes them ([Expiry and cleaning](#expiry-and-cleaning)). It is an op, so the server that owns the cache cleans it (a running `anfra serve`, when there is one), and a platform decides who may call it ([CLI and warm server](cli-vs-warm-server.md)).

```yaml
# cache.clean's input
query:                  # the query cache's entries to remove. Empty: all of the repo's.
  data_source: warehouse
  model: orders         # every entry read from the model: after its data was fixed
```

It selects entries by their [tags](#tags), the fields combined with AND, and returns how many entries it removed, and their size. `query` leaves room for anfra's other caches (anfra-node's compile cache) under the same op. At the CLI, the fields are flags: `anfra cache clean --query.data-source warehouse --query.model orders`.

## How it works

### The pieces

| Piece | Does |
|---|---|
| **anfra-node** | Compiles the query, and its **cache key**: a hash of the query's structure ([Keys](#keys)). |
| **The cache manager** (`shared/querycache`) | The algorithm: looks a key up, begins, commits and abandons writes, applies the age rule, plans sweeps. |
| **The store** | Where the manager keeps the cache's metadata: SQLite for anfra, a platform's own for a platform ([The store](#the-store)). |
| **canal-query and canal-lake** | Executors only. canal-query runs a query on the warehouse, and writes its result to canal-lake as it streams it back. canal-lake stores each result as a Parquet file under a key, and reads it back. Neither knows what a key means, nor any expiry. |

The cache keeps two kinds of entry: a **logical entry** for each key, pointing at the **physical entry** that answers it now, and a physical entry for each result written, with its own file in canal-lake. A key's result written again is a new physical entry, which its logical entry moves to once the write is complete.

### Keys

anfra answers a query only from an entry of exactly that query. Its key is computed by anfra-node, from the query's structure as the compiler sees it:

- **Everything that shapes what runs and what it answers is in it:** the dataset's models, fields and joins, filters and conditions (data restrictions arrive as filters), parameters, the time zone, `distinct`, and the result's presentation: its columns in their order, `order_by`, `limit` and `offset`, the page.
- **Whatever doesn't change the result is left out:** comments, labels, descriptions, tags, and the order filters and conditions are written in.
- **The time reaches it only through the seeds:** relative dates resolve against the reference time, `last 7 days` to two timestamps. So a `last 7 days` query keeps its key for a day, and gets a new one with the next, and a query compiled again with the seeds its response returned gets the same key. A key is a function of what the query was sent and what it read, and of nothing else.

anfra adds what the compiler doesn't see, and hashes the two together:

- **The tenant** (the invocation's `TenantID`), so tenants never share an entry, as they never share a warehouse connection.
- **The data source's identity**: its type and connection settings, credentials included, hashed. A changed password or project is another key, so an entry never outlives the access it was read with. Credentials are only in the hash, never stored.
- **What bounds the result**: its row truncation and byte limit, since canal-query stores the rows it streamed.

A query written in SQL (`--lang sql`) has no structure to hash, so its key holds its text in place of the structure.

So each page is its own entry, run on the warehouse with its sort and limit. A change to a model, a dataset or the query is a new key, and needs no invalidation. There is one key implementation, in anfra-node, so every path in a deployment finds the entries the others wrote.

**What decides access is always in the key.** Data restrictions and user attributes are resolved into the query's structure, and the tenant and the data source's connection are added by anfra. A restriction that is neither, if one is ever added, must become one of them before it ships. A test holds it: another restriction, user attribute or credential is another key.

### Tags

An entry's **tags** are what a clean, and later attribution, select it by: each a kind and a value, written here as `model:orders`, and kept and selected as the two (`cache.clean`'s input has a field per kind). An entry has many, and many entries share one, as cache tags do in CDNs and web frameworks (Cloudflare's `Cache-Tag`, Next.js's `revalidateTag`).

| Tag | From | Trusted |
|---|---|---|
| `data_source`, `dataset` | anfra, from the query | Yes |
| `model` | anfra, from the compile: every model the query read | Yes |
| `app`, `block`, an origin (`cli`, `agent`, `mcp`) | The caller, later ([Implementation plan](#implementation-plan)) | No: a label the caller asserts |

Tags are not the key's dependencies. A key is a function of everything that changes its result, the query's text and seeds and the caller's user attributes among them ([Keys](#keys)); tags are a few of those, and labels besides, kept only to select entries by. Two rules keep them apart:

- **No lookup or access decision reads a tag.** The key alone finds an entry.
- **A tag the caller asserts only narrows a clean** the caller is already allowed: cleaning by block removes no entry the caller couldn't remove by cleaning everything.

A tag is the key's, not a write's: entries of one key have one set of tags, recorded when the key's first entry commits. A caller's tags, once there, add to it as other callers use the entry. Cleaning by one block's tag removes an entry another block shares: that block's next run goes to the warehouse, which costs a query, and nothing else.

### Re-reading a result

A caller that wants a result a query returned before sends the same query again, with the `input` its response returned ([a query's input](query-input.md#why-it-is-kept-together)): the next page, an export, an agent reading what a user saw. anfra compiles it as the caller, with the caller's own user attributes and restrictions, and looks its key up as for any query:

- **The same key** proves the caller may see the rows: its own compile produced the query that made them. Two callers with the same access share the entry.
- **Another key** (other restrictions, a changed model, or a permission changed since) is another query. The caller never gets a result its own compile didn't produce.

When the entry is gone (it expired, or was swept), what happens is the caller's choice:

- **Without `cached_only`,** the query runs again on the warehouse, for the same reference time, and its `cache` says so (`hit: false`). The rows are of the same query, read now.
- **With `cached_only`,** it fails with `cache_miss`, and runs nothing: for a caller that would rather tell the person the result they saw is gone, or offer to refresh it. An agent reading what a user saw would usually send it.

Two rules hold this up:

- **No one reads an entry by its key.** A key is something anfra computes, and never something a caller is trusted with. A key from a caller is at most a hint ([Extension points](#extension-points)).
- **What decides access never comes from the request.** User attributes and restrictions are filled in from who is asking, on every query. A platform that passes them to the engine takes them from its own identity, never from its client.

### A miss

1. **Look up** the key: no entry the [age rule](#which-entry-a-query-accepts) accepts, or `cache.refresh`.
2. **Begin a write.** The store records a pending physical entry, with a canal-lake key of its own (`<tenant hash>/<key>/<write id>`) and when it started. Every write has its own canal-lake key, since canal-lake can't write one key twice safely ([Concurrency](#concurrency-and-failure)).
3. **Run** the query on canal-query with that canal-lake key: it streams the result back as today, and writes it to canal-lake as it goes.
4. **Commit** once canal-query's trailer says the write finished, in one transaction: the physical entry gets its row count, size, columns, `cached_at` and `expires_at` (`cached_at` plus the query's `cache.duration`); the key's logical entry points at it, unless a newer write has committed (the greater id wins); the key's other physical entries expire. A failed or cancelled query commits nothing: canal-lake removes its half-written file, and the pending entry is swept.

The caller gets its rows as they stream, as without a cache: caching adds nothing to the time to first row.

### A hit

1. **Look up** the key: the logical entry's physical entry, if the age rule accepts it.
2. **Read** it whole: canal-query reads the entry from canal-lake and streams it in the format of a warehouse result. One implementation encodes every result, cached or not, with the same streaming, limits, progress and cancellation, so a hit cannot answer differently from a miss.

anfra reaches canal through an executor: run a query on the warehouse, into a canal-lake key; read an entry. A query run over an entry by a platform names the entry for the executor to resolve, never its file's path, so the read path can change without changing any stored entry.

### Expiry and cleaning

An entry expires at its `expires_at`, or when a newer one replaces it. Expired, no lookup returns it, and a sweep removes it:

- **Expired entries**, once expired for longer than the longest read: a read that started before expiry finishes first.
- **Pending entries**, once older than the longest a cached query can run (`query export`'s timeout, 30 minutes) plus a margin, so a write still running is never removed. The cutoff is derived from the timeout, and a test holds it above it.
- **Over the size limit** (after the first version): the oldest entries first, while the repo's cache is over `ANFRA_QUERY_CACHE_MAX_BYTES`.

A sweep drops an entry's file from canal-lake, then its rows from the store. In that order, a store row may name a file already gone, but no file is left that no row names.

The sweep is a [background routine](background-routines.md): in `anfra serve`, every ten minutes; at the CLI, when a command first writes to the cache, as export files are swept ([export stores](export-store.md)).

### Concurrency and failure

The cache manager makes the cache safe under concurrency, not canal-lake. canal-lake promises two things only: a write that fails removes its file, and a complete file can be read. It has no locks or atomic writes: two writes to one key corrupt each other, and a read of a key being written fails. The manager never writes a canal-lake key twice, never reads one before its write commits, and never sweeps a write in progress or a file being read.

| Case | What happens |
|---|---|
| Two callers miss on the same key at once | Both run the query, each writing its own canal-lake key. The newer write's entry wins, whichever commits first. |
| A write fails or is cancelled | canal-lake removes the file; the pending entry is swept. Nothing was committed. |
| anfra or canal-lake crashes mid-write | A partial file may remain. Its pending entry is swept after the cutoff, and drops it. |
| canal-lake can't start or isn't reachable | The query runs uncached, and the failure is logged and traced. A cache problem never fails a query. |
| The store fails | As above: uncached, logged. |

> Open: running the same query once for concurrent callers (single flight). Not in a first version: identical queries at the same moment are rare on one repo, and a stream's first rows shouldn't wait on another caller.

### Where it lives

Under the repo's folder in anfra's home, never in the repo:

```
<ANFRA_HOME>/repos/<repo id>/query-cache/
  cache.sqlite   the store
  lake/          canal-lake's storage: the Parquet files, and DuckDB's temporary files
```

### canal-lake as a sidecar

anfra runs canal-lake as it runs canal-query: from anfra-canal (`anfra-canal lake`), `ANFRA_CANAL_LAKE_BIN` for a standalone build, or `ANFRA_CANAL_LAKE_URL` for one something else runs ([sidecars](sidecars.md)). Differences:

- **It starts only when a cached query needs it**, so a repo that caches nothing never runs it. anfra gives canal-query canal-lake's socket path (`CANAL_LAKE_SOCKET`) when it starts canal-query, whether canal-lake runs yet or not: canal-query connects for each query.
- **Its storage is the repo's** (`STORAGE_ROOT`), not canal-lake's default, `/tmp/canal_lake/`.
- **Its settings prefix is `ANFRA_CANAL_LAKE_ENV_`**, as canal-query's is.

## The code

| Package | Holds |
|---|---|
| `shared/querycache` | The data structures (logical and physical entries, `CacheMetadata`), the manager (lookup with the age rule, begin, commit, abandon, sweep planning), and the `Store`, `Lake`, `Planner` and `Recorder` interfaces. In `shared/` because a platform embedding the engine imports it, as it does `shared/routine`. |
| `shared/querycache/storetest` | The conformance suite every store must pass: the commit rules, concurrent commits, the age rule, pending and expired sweeps. |
| `internal/querycache/sqlite` | anfra's store, on SQLite (`modernc.org/sqlite`, pure Go, as anfra builds without cgo). |

`Lake` is the little the manager needs of canal-lake besides reading and writing, which go through the executor: dropping files, which canal-query relays (`POST /do_lake_action`).

The engine takes a `Store` on each invocation, as it takes an `ExportStore`, and the extensions below. Without a store, nothing is cached.

### The store

The `Store` interface is defined on the data structures, not on tables: each method is one step of the algorithm, and the transaction it needs is the store's to keep.

| Method | Does |
|---|---|
| `Lookup(tenant, key, now)` | The key's committed physical entry, unexpired, or none. The manager applies the age rule. |
| `Get(id)` | A physical entry by id, as `Lookup` would return it: for a platform's planner, which finds its entry by other means. |
| `Begin(entry)` | Records a pending physical entry. |
| `Commit(entry)` | In one transaction: fills the physical entry, points the logical entry at it unless a newer one is there, expires the key's others. |
| `Abandon(entry)` | Expires a pending entry. |
| `Sweepable(now, cutoffs, limit)` | Entries a sweep may remove: expired past the read cutoff, pending past the write cutoff. |
| `Delete(entries)` | Removes entries, and logical entries that point at them. |
| `Tag(tenant, key, tags)` | Adds tags to a key. |
| `Expire(tenant, tags)` | Expires every entry with all the given tags, or every entry when none are given: `cache.clean`. Returns what it expired. |

An entry carries the query's structure, as anfra-node compiled it, as data. anfra's store doesn't keep it. A platform's store may, for its own use.

**A reference schema**, which anfra's SQLite store follows and a platform's may:

```sql
query_cache_logical_entries (
  tenant_id    text, key text,
  physical_id  integer,                  -- its current physical entry
  primary key (tenant_id, key)
)
query_cache_physical_entries (
  id           integer primary key,      -- increasing: the greater id is the newer write
  tenant_id    text, key text,           -- the logical entry it is for
  lake_key     text unique,              -- its file in canal-lake
  status       text,                     -- pending | committed
  started_at   timestamp,
  cached_at    timestamp, expires_at timestamp,
  row_count    integer, size_bytes integer,
  columns      json,                     -- names, database and Arrow types: the result's header
  aql          text,                     -- the query that ran, as AQL
  sql          text                      -- and as SQL
)
query_cache_tags (
  tenant_id    text, key text,           -- the logical entry it tags
  kind         text,                     -- data_source | dataset | model, and later app | block | origin
  value        text,                     -- such as orders
  primary key (tenant_id, key, kind, value)
)
-- indexes: query_cache_physical_entries (tenant_id, key), (expires_at), (status, started_at);
--          query_cache_tags (tenant_id, kind, value)
```

The tables are the query cache's alone, named for it. A generic cache, one store for results of every kind, would push the query's columns into an untyped one, and tell kinds apart by a key's prefix, as Holistics' does, with the over-matching that brings. A second kind of cached result, if one comes, can generalize it then: anfra's store is disposable, and a platform's migrates.

The schema is a reference, not a contract: a store may lay its tables out as it likes. **The contract is `storetest`**, which pins down what each method does whatever the tables are.

anfra's cache is disposable: a store whose schema version (SQLite's `user_version`) is not the one anfra expects is deleted with its canal-lake files, and started again. A platform's store migrates as its database does.

## Extension points

anfra caches exact queries. A platform embedding the engine may do more, through two extension points in the query path, each with a core implementation that does nothing:

| Extension | Called | Core's implementation |
|---|---|---|
| **`Planner`** | Before a query runs, with its compiled form, its structure, its key and its tenant | Answers the query as it is |
| **`Recorder`** | After a query is answered, with its structure, its entry and how it was answered | Nothing |

A `Planner` answers with one of:

1. **As it is.** The path above: an exact lookup, then a hit or a miss.
2. **Transformed.** Another query, which runs on the path above under its own key: a transformed query is cached like any other.
3. **Requeried.** A cached entry and a query over it: the executor runs the query on the entry, and the answer has the entry's `cache`. The entry must pass `Store.Get` and the age rule against the query's `cache.duration`, as a lookup's must.

A `Recorder` lets a platform keep what a later `Planner` needs, such as each entry's structure.

> Open: whether core's query input takes more for a `Planner`, such as keys of queries a Data App ran before (`cache.candidates`), which core would accept and ignore. If it does, a key from a caller is a hint and never trusted: a `Planner` looks it up within the caller's tenant only, and holds the entry's data restrictions to the query's.

## Changes in anfra-node and canal

| Where | Change | Why |
|---|---|---|
| anfra-node | Compile returns the query's cache key, computed from its query structure by the rules above, and takes and returns the query's seeds. | A key from the structure, not the SQL, that a query compiled again with its seeds reproduces. |
| canal-lake | Keeps decimals as decimals when asked. Its reads cast decimal columns to strings today. | A cached decimal column must keep its Arrow type, or a hit's header differs from a miss's. |
| canal-lake | Its pprof address is a setting (`PPROF_ADDR`, as canal-query's). It always listens on `localhost:6060` today. | Two repos' servers each run a canal-lake: the second can't take the port. |
| canal-query | Reads an entry from canal-lake, and streams it as a warehouse result. | The hit path. canal-query only writes to canal-lake today. |
| canal-lake | A read's limit (one minute today) becomes a setting. | A `query export` read from the cache can take longer. |
| canal-query | Streams on when its canal-lake write fails, and reports the failure in its trailer, instead of failing the query. | A cache problem should never fail a query. Today a write that fails mid-query fails it. Later. |

A check holds a hit to a miss: a test runs a set of queries (every type [result values](result-values.md) names) once cold and once cached, and compares the answers, all but `cache`, byte for byte.

## Alternatives considered

- **Key on the compiled SQL.** Holds everything that decides the rows by construction. But a changed comment is a new key, and SQL is no structure a platform's `Planner` can match a query against.
- **Reuse an entry for another column order**, by putting its columns in the query's order on read. Cheap, but a requery in miniature: it blurs where anfra stops and a platform's extensions start, transforms cached results outside any requery's code, and pays little in anfra, whose callers are agents and code that don't rearrange columns.
- **Cache the whole result and page it on the cache.** Reuse across pages, but every miss runs twice (the whole result, then the page), the first page waits for the whole result, and it is a requery.
- **The store in DuckDB, inside canal-lake.** anfra's Go side can't open DuckDB itself (it builds without cgo), so canal-lake would hold the store, behind new calls. But one process at a time can write a DuckDB file, so two one-shot CLI runs on a repo, each with its own canal-lake, would conflict, and the cache's metadata would be down whenever canal-lake is.
- **The store as one JSON file per key.** No dependency, and atomic by rename. But nothing a listing or a platform's lookup could query.
- **Cache in anfra-node or in anfra's memory.** A result can be hundreds of megabytes, a server's memory is the user's, and it would neither survive a restart nor be shared with the CLI.

## Implementation plan

1. **anfra-node:** the structural cache key, and the seeds.
2. **canal:** decimals kept, pprof's address, canal-query reading an entry from canal-lake; the parity test.
3. **The canal-lake sidecar:** its supervisor, started on demand, with its storage under the repo's folder.
4. **The cache:** `input.seeds`, and `pagination` and `timezone` moved into `input` ([a query's input](query-input.md)); `shared/querycache` with `storetest`, the SQLite store, the miss and hit paths in `query` and `query export`, the `cache` input, with `cache_miss`, and the answer's `cache` (a change to the core API's contract: the OpenAPI document and the SDK's types), uncached when canal-lake fails, the sweep for expired and pending entries, the tags anfra derives, `cache.clean`.
5. **The SDK:** a query's result keeps the `input` it ran with, and replays it for the next page and an export; `refresh` leaves the seeds out. `inspect` showing the answer's `cache`, the `refresh` plugin.
6. **The engine:** the `Store`, `Planner` and `Recorder` on each invocation, with core's implementations.
7. **After:** the caller's tags (`app`, `block`, an origin, through a `cache.origin` input) and attribution by tag (hits, misses and size per tag, for a `cache.stats` op and `inspect`); the size limit; canal-query streaming on when its canal-lake write fails.
