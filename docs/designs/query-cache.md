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

The query cache is on in every repo, and a query uses it when it asks to, with its own `cache.duration`, or when the repo sets a default for every query. In the repo's settings, `.anfra/config.yml`:

```yaml
cache:
  query:
    enabled: true    # the default. false: the query cache is off for this repo
    default:
      duration: 1h   # how old a cached result a query accepts, unless it says otherwise
```

- **Off for a repo** (`cache.query.enabled: false`): no store, no canal-lake, no checks and no warnings, ever. A query that asks for the cache anyway runs without it, and its answer says so (`cache.warning`, `cache_disabled`). `cache.cached_only` and `cache.clean` fail with `cache_disabled`, scope `user`: the repo's own setting refuses them.
- **Off for a process** (`ANFRA_CACHE_QUERY_ENABLED=0`, the setting's path as a variable, `1` or `0` as anfra's other switches): whatever the repo says, and `1` turns it on over a repo's `false`. For a run that must see live data, such as CI, or to rule the cache out while debugging.
- **What it costs a repo whose queries never ask:** one small file, the store, which `anfra serve` creates at startup to check the cache works ([When the cache isn't working](#when-the-cache-isnt-working)). canal-lake, a process, starts only when a query is cached.

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

On a miss whose result was cached, `hit` is `false` and `cached_at` is when this query ran. When nothing was cached (`cache.read_only`, or the cache couldn't be used), `cached_at` and `expires_at` are absent. A query the cache doesn't apply to (no `cache.duration` of its own or the repo's, or `0s`) has no `cache`. In a stream, it is in the summary.

When a query would be cached but the cache isn't working, or the repo turned it off, the query still runs, on the warehouse, and its `cache` says why, as a `warning`: a code and a message, nested under the part it concerns, as anfra reports a problem that doesn't fail a request ([errors](errors.md)):

```yaml
cache:
  key: 9f1c4e...
  hit: false
  warning:
    code: cache_unavailable
    message: The query ran without the cache, because the cache isn't working. The server's log says why.
```

| Code | Means |
|---|---|
| `cache_unavailable` | The cache isn't working: its store or canal-lake failed. The cause is in the log, for whoever runs anfra, not in the answer |
| `cache_disabled` | The repo turned the query cache off (`cache.query.enabled: false`) |

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

It selects entries by their [tags](#tags), the fields combined with AND, and returns how many entries it removed, and their size.

**After a clean, no result read before it is answered from the cache.** That is the clean's goal: someone cleaning after fixing a table's data must be able to trust that what the cache answers from then on was read after the fix. So a clean reaches queries already running too: a write it selects, still in progress, ends as if abandoned, and its result is never used, though the query's own caller still gets its rows. Most caches' purges leave such writes to land, and a purge can be undone by a request that began before it; anfra's doesn't ([the lease](#a-writes-lease)). `query` leaves room for anfra's other caches (anfra-node's compile cache) under the same op. At the CLI, the fields are flags: `anfra cache clean --query.data-source warehouse --query.model orders`.

## How it works

### The pieces

| Piece | Does |
|---|---|
| **anfra-node** | Compiles the query, and its **cache key**: a hash of the query's structure ([Keys](#keys)). |
| **The cache manager** (`internal/querycache`) | The algorithm: looks a key up, begins, commits and abandons writes, applies the age rule, plans sweeps. |
| **The store** | Where the manager keeps the cache's metadata: SQLite for anfra, a platform's own for a platform ([The store](#the-store)). |
| **canal-query and canal-lake** | Executors only. canal-query runs a query on the warehouse, and writes its result to canal-lake as it streams it back. canal-lake stores each result as a Parquet file under a key, and reads it back. Neither knows what a key means, nor any expiry. |

The cache keeps two kinds of entry: a **logical entry** for each key, pointing at the **physical entry** that answers it now, and a physical entry for each result written, with its own file in canal-lake. A key's result written again is a new physical entry, which its logical entry moves to once the write is complete.

### Keys

anfra answers a query only from an entry of exactly that query. Its key is computed by anfra-node, from the query's structure as the compiler sees it:

- **Everything that shapes what runs and what it answers is in it:** the dataset's models, fields and joins, filters and conditions (data restrictions arrive as filters), parameters, the time zone, `distinct`, and the result's presentation: its columns in their order, `order_by`, `limit` and `offset`, the page.
- **Whatever doesn't change the result is left out:** comments, labels, descriptions, tags, and the order filters and conditions are written in.
- **The time reaches it only through the seeds:** relative dates resolve against the reference time, `last 7 days` to two timestamps. So a `last 7 days` query keeps its key for a day, and gets a new one with the next, and a query compiled again with the seeds its response returned gets the same key. A key is a function of what the query was sent and what it read, and of nothing else.

anfra adds what the compiler doesn't see, and hashes the two together, on the same principle: **what shapes what runs and what it answers is in the key, and whatever doesn't change the result is left out.** Leaving out what doesn't matter is not tidiness: a key holding more than it needs costs misses, and a miss fails nothing, so no one notices the cache hitting less.

What the key promises comes in two strengths, as a missed input costs differently:

| Left out of the key | Costs | So the key is |
|---|---|---|
| Something that decides **access** | Another caller's rows: a leak | **A guarantee.** When in doubt, it is in; tested |
| Something that changes **the rows**, the access the same | Rows other than a run now would give, for as long as the query's `cache.duration` accepts the entry | **Best effort.** What anfra knows changes the rows is in; it starts small |
| Something that changes **neither** | Nothing: it is left out | |

**Access, guaranteed.** An entry answers only a query with:

- **The same compiled query:** anfra-node's key, with data restrictions and user attributes resolved into the structure it hashes.
- **The same tenant** (the invocation's `TenantID`), so tenants never share an entry, as they never share a warehouse connection. A platform's store keeps tenants apart too ([The store](#the-store)); the key holds even if a store were bound wrongly.
- **The same data source identity:** who the warehouse sees. The host gives it with the data source's connection (`DataSource.Identity`), and must change it whenever access changes: another warehouse user, role, project, or the viewer under OAuth passthrough. anfra's own, for a local repo, gives none, and the identity is the connection hashed, credentials included: a changed password or project is another key. Credentials are only in the hash, never stored. A platform gives one stable across what doesn't change access, such as a rotated secret or a refreshed token, which hashing the connection would turn into a new key each time.

A restriction that is none of these, if one is ever added, must become one of them before it ships. A test holds it: another restriction, user attribute, tenant or identity is another key.

**The rows, best effort.** The key also holds what anfra knows changes the rows under the same access:

- **The data source's type**, the SQL dialect.
- **anfra-node's version**, as a compiler change changes what runs. An upgrade starts the keys afresh: one miss for each query.
- **The row truncation**, since canal-query stores the rows it streamed.

Nothing promises a hit equals running the query now: the warehouse's data, a view redefined, or a setting on the warehouse, change rows without changing any key. An entry is never older than its query's `cache.duration` accepts, and `cache.clean` and skipping the cache are the remedies; that bound, not the key, is what keeps data fresh. A data source's own settings are left out to start, though a few change rows: MySQL's `sql_modes` and `collation`, Databricks' native complex types. They are the first to add, should stale rows from them matter.

**Left out:** the query's comments and labels (anfra-node's key leaves them out already; the SQL text is never hashed, so a comment a platform adds for its warehouse's logs can't split the cache), timeouts, SSL, pooling, and the result's maximums. A maximum, in rows or bytes, fails a query rather than changes its rows, so a hit checks it instead: an entry over the reader's maximum answers the same error a miss would.

A query written in SQL (`--lang sql`) has no structure to hash, so its key holds its text in place of anfra-node's key and version. A comment changes it: SQL can't be normalized safely, and a miss is the right failure.

So each page is its own entry, run on the warehouse with its sort and limit. A change to a model, a dataset or the query is a new key, and needs no invalidation. There is one implementation of each part, anfra-node's and anfra's, so every path in a deployment finds the entries the others wrote.

### Tags

An entry's **tags** are what a clean, and later attribution, select it by: each a kind and a value, written here as `model:orders`, and kept and selected as the two (`cache.clean`'s input has a field per kind). An entry has many, and many entries share one, as cache tags do in CDNs and web frameworks (Cloudflare's `Cache-Tag`, Next.js's `revalidateTag`).

| Tag | From | Trusted |
|---|---|---|
| `data_source`, `dataset` | anfra, from the query | Yes |
| `model` | anfra, from the compile: every model the query read | Yes |
| `app`, `block`, an origin (`cli`, `agent`, `mcp`) | The caller, later ([Implementation plan](#implementation-plan)) | No: a label the caller asserts |

Tags are not the key's dependencies. A key is a function of what decides access and what anfra knows changes the result, the query's structure and seeds and the caller's user attributes among them ([Keys](#keys)); tags are a few of those, and labels besides, kept only to select entries by. Two rules keep them apart:

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
2. **Begin a write.** The store records a pending physical entry, with a canal-lake key of its own (`<namespace><entry id>`, [file names](#file-names)) and when it started. Every write has its own canal-lake key, since canal-lake can't write one key twice safely ([Concurrency](#concurrency-and-failure)).
3. **Run** the query on canal-query with that canal-lake key: it streams the result back as today, and writes it to canal-lake as it goes.
4. **Commit** once canal-query's trailer says the write finished, in one transaction: the physical entry gets its row count, size, columns, `cached_at` and `expires_at` (`cached_at` plus the query's `cache.duration`). `cached_at` is when the write began, since the warehouse reads a query's rows as it starts: an entry never claims to be fresher than its data. If the write's lease hasn't ended, the key's logical entry points at it, unless a newer write has committed (the greater id wins); the key's other physical entries expire. A failed or cancelled query commits nothing: canal-lake removes its half-written file, and the pending entry is swept.

The caller gets its rows as they stream, as without a cache: caching adds nothing to the time to first row.

### A hit

1. **Look up** the key: the logical entry's physical entry, if the age rule accepts it. An entry over the reader's maximums, in rows or bytes, answers the error a miss over them would; the maximums are not in the key ([Keys](#keys)).
2. **Read** it whole: canal-query reads the entry from canal-lake and streams it in the format of a warehouse result. One implementation encodes every result, cached or not, with the same streaming, limits, progress and cancellation, so a hit cannot answer differently from a miss.

anfra reaches canal through an executor: run a query on the warehouse, into a canal-lake key; read an entry. A query run over an entry by a platform names the entry for the executor to resolve, never its file's path, so the read path can change without changing any stored entry.

### Expiry and cleaning

An entry expires at its `expires_at`, or when a newer one replaces it. Expired, no lookup returns it, and a sweep removes it:

- **Expired entries**, once expired for longer than the longest read: a read that started before expiry finishes first.
- **Pending entries**, once their lease has run out ([below](#a-writes-lease)): a write still running is never removed.
- **Over the size limit** (after the first version): the oldest entries first, while the repo's cache is over `cache.query.max_bytes` (or `ANFRA_CACHE_QUERY_MAX_BYTES`).

A sweep drops an entry's file from canal-lake, then its rows from the store. In that order, a store row may name a file already gone, but no file is left that no row names.

#### A write's lease

A pending entry's `expires_at` is its lease: the latest its write may commit and be used. It is set when the write begins, from the run's own deadline (its command's timeout, 30 minutes for `query export`) plus a margin, so a write can't outlive it. Committing checks it, and ending it early is how a write is stopped from ever being used:

| Case | The lease | When the write commits |
|---|---|---|
| The write finishes | Still running | The entry becomes current, unless a newer one is |
| The query fails or is cancelled (abandoned) | Ended at that moment | Its entry, if it commits at all, is expired at once |
| A clean selects its key | Ended at the clean | Expired at once: what was read before the clean is never used |
| anfra crashes | Runs out on its own | Never commits; swept like any expired entry |

So one check, at commit, covers each case: a write is used only if its lease hasn't ended.

The sweep is a [background routine](background-routines.md): in `anfra serve`, every ten minutes; at the CLI, when a command first writes to the cache, as export files are swept ([export stores](export-store.md)).

### Concurrency and failure

The cache manager makes the cache safe under concurrency, not canal-lake. canal-lake promises two things only: a write that fails removes its file, and a complete file can be read. It has no locks or atomic writes: two writes to one key corrupt each other, and a read of a key being written fails. The manager never writes a canal-lake key twice, never reads one before its write commits, and never sweeps a write in progress or a file being read.

| Case | What happens |
|---|---|
| Two callers miss on the same key at once | Both run the query, each writing its own canal-lake key. The newer write's entry wins, whichever commits first. |
| A write fails or is cancelled | canal-lake removes the file; the pending entry is swept. Nothing was committed. |
| anfra or canal-lake crashes mid-write | A partial file may remain. Its pending entry is swept after the cutoff, and drops it. |
| canal-lake can't start or isn't reachable | The query runs uncached, and says so ([When the cache isn't working](#when-the-cache-isnt-working)). |
| The store fails | As above. |

#### When the cache isn't working

**When the cache isn't working, a request that doesn't require it is answered without it, and one that does fails. Either way, the failure is reported where the people concerned look: the answer's `warning`, the CLI's and `anfra serve`'s output, and the log.**

- **Not required:** a query cached because of its own `cache.duration` or the repo's default. Caching is a side effect of answering it, and the data is on the warehouse, so a cache failure doesn't fail it: refusing the query because its copy can't be kept would turn a cache problem into an outage. It runs uncached, with a warning.
- **RequiredByInstalled:** `cache.cached_only`, which asks for a cached result or nothing, and `cache.clean`. Without the cache there is nothing to do, so a cache failure fails them, with `cache_unavailable`, scope `server`, or `cache_disabled`, scope `user`, when the repo turned it off. A `cached_only` query that finds no entry fails with `cache_miss`, scope `user`.

A failure that doesn't fail the request is still reported, because a cache that silently stops working keeps someone paying the warehouse for every query, believing they're cached.

| Who | What they get |
|---|---|
| **The caller** | On its answer, `cache.warning`, `cache_unavailable` ([A query's answer](#a-querys-answer)): its rows weren't cached, and the log says why |
| **A person at the CLI** | A `warning` document on stderr, with the cause, as the CLI reports other problems that don't stop a command |
| **The person running `anfra serve`** | **At startup**, unless the repo turned the cache off, the server opens the store, creating or migrating it, and a failure is a warning in its startup output, before it serves. While it runs, a failure is in the log, with each query it affected |
| **Whoever investigates** | The log and the trace, at error level, as every server-scope failure |

**`anfra status`** reports the cache as one of four states, one for each thing to do about it. It only reads the store's header, so it never creates or changes the store.

| `state` | Means | What to do |
|---|---|---|
| `ready` | The store can be used: at this anfra's version, older (brought up to date when next opened), newer but compatible, or not made yet (made at the first cached query, or when `anfra serve` starts) | Nothing |
| `incompatible` | A newer anfra changed the store in a way this one can't read. Its `problem` gives the versions | `anfra update` |
| `unavailable` | The store isn't a file anfra made, or can't be read | Move the file aside, or read the error |
| `disabled` | The repo's settings, or `ANFRA_CACHE_QUERY_ENABLED`, turn the cache off. `reason` says which | Nothing, or turn it on |

How a store's version is kept is the cache's own affair: no state, command or message mentions migrating.

> Consideration: backing off after a failure. Without it, while the store can't open or canal-lake can't start, each query that would be cached tries again, and fails again, before running uncached. A failure to open or start is quick, so that costs little; if it proves costly, the cache can wait before trying again, and say so on the answer.

> Open: running the same query once for concurrent callers (single flight). Not in a first version: identical queries at the same moment are rare on one repo, and a stream's first rows shouldn't wait on another caller.

### Where it lives

Under the repo's folder in anfra's home, never in the repo, beside its compile cache:

```
<ANFRA_HOME>/repos/<repo id>/cache/query/
  store.sqlite   the store
  lake/          canal-lake's storage: the Parquet files, and DuckDB's temporary files
```

anfra's host sets the path, as it does the compile cache's (`repo.CacheDir()`, then `query`), and opens the store and starts canal-lake there. A platform embedding the engine supplies its own store and canal-lake instead.

### canal-lake as a sidecar

anfra runs canal-lake as it runs canal-query: from anfra-canal (`anfra-canal lake`), or, in a build from source, `ANFRA_CANAL_LAKE_BIN` for a standalone canal-lake ([sidecars](sidecars.md)). Its own supervisor, `internal/sidecar/canallake`, gives it a socket in a folder of its own (`SOCKET`). Differences:

- **It starts only when a cached query needs it**, so a repo that caches nothing never runs it. When the cache is on, anfra picks canal-lake's socket path before it starts canal-query, and gives canal-query that path (`CANAL_LAKE_SOCKET`) whether canal-lake runs yet or not: canal-query connects for each query.
- **Its storage is the repo's** (`STORAGE_ROOT`, the `lake/` folder, ending in `/`: canal-lake appends `duckdb.tmp/` to it for DuckDB's temporary files), not canal-lake's default, `/tmp/canal_lake/`.
- **It runs in that folder.** canal-lake loads a `.env` from its working directory, which would otherwise be anfra's, usually the repo's.
- **It stops quickly.** anfra sets `SHUTDOWN_GRACE_SECONDS` low: canal-lake's default waits 20 minutes for its calls to finish, and anfra kills a sidecar that hasn't stopped after 3 seconds anyway.
- **It is ready when its socket accepts a connection.** canal-lake answers gRPC, health included, but anfra never calls it itself: canal-query relays writes, reads and drops. A dial avoids a gRPC client for one health check.
- **Its settings prefix is `ANFRA_CANAL_LAKE_ENV_`**, as canal-query's is. anfra sets no `PPROF_ADDR`, so it serves no pprof unless asked to.
- **A deployment's canal-query comes with its canal-lake.** With canal-query at a URL (`ANFRA_CANAL_QUERY_URL`), anfra starts no canal-lake: canal-query reaches the deployment's, by its own settings, and anfra never calls canal-lake itself, so it needs no address for one (`ANFRA_CANAL_LAKE_URL`). [The settings](../usage/configs.md#running-canal-query-and-canal-lake-yourself) say what to configure, and a namespace for each anfra that shares one canal-lake storage ([file names](#file-names)).

## The code

| Package | Holds |
|---|---|
| `internal/querycache` | The data structures (logical and physical entries, `CacheMetadata`), the manager (lookup with the age rule, begin, commit, abandon, sweep planning), the cache's state as status reports it, and the `Store`, `Lake`, `Summarizer`, `Planner` and `Recorder` interfaces. A platform reaches what it implements through `engine`'s aliases (`engine.QueryCacheStore`, …), as it does `engine.ExportStore`: the cache is an extension point of the engine, not a framework a platform builds its own on, so it isn't in `shared/`. |
| `engine/querycachetest` | The conformance suite every store must pass: the commit rules, concurrent commits, the age rule, pending and expired sweeps. Public, so a platform runs it on its own store, as `net/http/httptest` is public beside `net/http`. |
| `internal/querycache/local` | A local repo's query cache, as anfra's process holds it, imported as `localquerycache`: the repo's folder, the store opened at first use, and the cache's state for status. A platform serving managed repos supplies its own store and canal-lake instead. |
| `internal/querycache/store/sqlite` | anfra's store, on SQLite (`modernc.org/sqlite`, pure Go, as anfra builds without cgo). Its queries are generated by sqlc from `queries.sql`, checked against its migrations; sqlc's SQLite engine is Beta, so a query it can't handle is written by hand beside them. Imported as `sqlitestore`, so a call site says it is the store. |

`Lake` is the little the manager needs of canal-lake besides reading and writing, which go through the executor: dropping files, which canal-query relays (`POST /do_lake_action`).

The engine takes a `Store` on each invocation, as it takes an `ExportStore`, and the extensions below. Without a store, nothing is cached.

### The store

The `Store` interface is defined on the data structures, not on tables: each method is one step of the algorithm, and the transaction it needs is the store's to keep.

**A store is one cache, and names no tenant.** anfra's is one per repo, which is already its tenant. A platform serving several tenants binds its store to one tenant for each invocation, as it binds the engine's other stores: it keeps the tenant in its own rows and reads only that tenant's, and a user, if it attributes entries to one, the same way. So the interface, the manager and `querycachetest` have no tenant, and a platform's isolation is its own to test: two stores bound to two tenants over one database never see each other's entries.

| Method | Does |
|---|---|
| `Lookup(key, now)` | The key's current physical entry, unexpired, or none. The manager applies the age rule. |
| `Get(id, now)` | A physical entry by id, as `Lookup` would return it: for a platform's planner, which finds its entry by other means. |
| `Begin(entry, tags)` | Records a pending physical entry, with its namespace and its lease, and its key's tags. |
| `Commit(entry, now)` | In one transaction: fills the physical entry; if its lease hasn't ended by now, makes it current unless a newer one is, and expires the key's others; otherwise commits it expired. |
| `Abandon(id, now)` | Ends a pending entry's lease. |
| `Sweepable(cutoff, limit)` | Entries a sweep may remove: expired before the cutoff, which is the longest read ago. |
| `Delete(ids)` | Removes entries; a key's logical entry and tags with its last. |
| `Expire(tags, now)` | Expires every entry of the keys with all the given tags, or of every key when none are given, and ends their writes' leases: `cache.clean`. Returns the committed entries it expired. |

The interface is `internal/querycache`, which a platform reaches as `engine.QueryCacheStore`, and its suite is `engine/querycachetest`; anfra's store is `internal/querycache/store/sqlite`.

An entry carries the query's structure, as anfra-node compiled it, as data. anfra's store doesn't keep it. A platform's store may, for its own use.

**A reference schema**, which anfra's SQLite store follows and a platform's may:

```sql
query_cache_logical_entries (
  key          text primary key,
  physical_id  integer                   -- its current physical entry
)
query_cache_physical_entries (
  id           integer primary key,      -- increasing: the greater id is the newer write
  namespace    text,                     -- its file's prefix in canal-lake: empty in anfra
  key          text,                     -- the logical entry it is for
  cached_at    timestamp,                -- when its write began: its rows' time. Null while pending
  expires_at   timestamp,                -- while pending, its write's lease
  row_count    integer, size_bytes integer,
  columns      json,                     -- names, database and Arrow types: the result's header
  aql          text,                     -- the query that ran, as AQL
  sql          text                      -- and as SQL
)
query_cache_tags (
  key          text,                     -- the logical entry it tags
  kind         text,                     -- data_source | dataset | model, and later app | block | origin
  value        text,                     -- such as orders
  primary key (key, kind, value)
)
query_cache_meta (
  min_store_version integer             -- one row: the schema version an anfra must be at, at least, to use it
)
-- indexes: query_cache_physical_entries (key), (expires_at);
--          query_cache_tags (kind, value)
```

The tables are the query cache's alone, named for it. A generic cache, one store for results of every kind, would push the query's columns into an untyped one, and tell kinds apart by a key's prefix, with the over-matching that brings. A second kind of cached result, if one comes, can generalize it then: anfra's store is disposable, and a platform's migrates.

The schema is a reference, not a contract: a store may lay its tables out as it likes. **The contract is `querycachetest`**, which pins down what each method does whatever the tables are.

#### File names

An entry's file in canal-lake is named by its namespace and its id: `<namespace><entry id>`. anfra's namespace is empty by default, since the canal-lake it starts is the repo's, so its files are named `1234`. Several anfras writing to one canal-lake storage, as a deployment's replicas may, need a namespace each, which `ANFRA_CACHE_QUERY_NAMESPACE` sets (`prod`, so `prod/1234`). anfra can't tell whether a deployment's storage is shared, so the deployer sets it: a random default would make the common case, one anfra per storage, harder to read, and a copied value can collide as easily as none ([settings](../usage/configs.md#running-canal-query-and-canal-lake-yourself)). A value canal-lake can't take leaves the cache unavailable, and `anfra status` says how to fix it. A platform sharing one canal-lake between tenants gives each its own namespace, such as `3f9a1c0e/`, so a tenant's files are grouped, as `3f9a1c0e/1234`. The ids are the store's, increasing, so they also order writes, the newer winning.

The namespace is set by whoever makes the manager, for each invocation as its store is (`Config.Namespace`), and is stored on each physical entry from its write's start. So every row names its file, pending or not: a write killed before it commits leaves a row whose lease runs out, and its sweep drops the file. A sweep covers every namespace, as each row names its own file, and a file leads back to its row by its id. The name is derived (`Entry.LakeKey()`), and never stored whole: should the naming ever change, it is a migration.

Ids rewind only when a store is lost, replaced (an older anfra's newer store, [below](#its-schema-migrations)) or, in Cloud, restored from a backup. Then a new write may reuse the name of a file no row names any more. Overwriting it is harmless, since no lookup could find it. In Cloud, a restore during writes in flight could put two writes on one file: the result is a file that fails to read, which reads as a miss.

What a rewind does cost is disk: the files no new write reuses stay, named by no row, so no sweep removes them (orphans). **anfra never deletes them on its own.** When it creates a store and canal-lake's folder isn't empty, it warns with their size and the folder to delete, as it reports other cache problems ([When the cache isn't working](#when-the-cache-isnt-working)): "The query cache's folder holds 1.2 GB of files that the new cache doesn't use, probably left by an earlier one. To reclaim the space, delete `<folder>` while no anfra is running for this repo." The folder is measured inside the transaction that creates the store, under its write lock: another process opening the store waits for it, so none has written an entry, and no entry names a file found then. Where the files came from is a guess, hence "probably": most often a store lost or deleted while its files stayed. Deleting a folder's contents is the most destructive thing the cache could do: done on its own, a wrong path would delete files that aren't the cache's, and a process still using the old store would lose its files mid-use. The person decides, and the space is all it costs until they do.

Orphans have other causes too, all outside the cache's own flows (a file left by hand, canal-lake dropping less than it reported), so a later sweep removes files no row names ([Implementation plan](#implementation-plan)).

#### Its schema: migrations

anfra's store migrates as its schema changes, so an upgrade keeps the cache. Its schema is a folder of numbered SQL migrations, embedded in anfra, and a store is created by applying them all, so the schema can't drift from them; sqlc reads them as the schema it checks the queries against.

Three schema versions decide whether an anfra can use a store:

- **The anfra's current version** is the number of its last migration (`CurrentVersion`).
- **The store's installed version** is the number of the last migration applied to it. SQLite keeps it in the file's header, as `user_version`.
- **The store's required version** is the lowest current version an anfra must have to use the store. The store keeps it in `query_cache_meta`, as `min_store_version`.

An anfra uses a store when its current version is at least the store's required version. Only a breaking migration changes the required version. A migration is breaking when it drops or renames, or empties the tables because existing rows can't follow it, and it says so on its first line (`-- breaking`).

| Applied | Installed | Required | Which anfra can use the store |
|---|---|---|---|
| Migrations 1 and 2, neither breaking | 2 | 1 | One at version 1 or later |
| Migration 3, which adds a column | 3 | 1 | Still one at version 1 or later: an older anfra doesn't read the new column |
| Migration 4, which drops a column | 4 | 4 | Only one at version 4 or later: an older anfra would query the dropped column |

**When:** a process migrates a store when it opens it: at its first use of the cache, or for `anfra serve`, at startup. Every migration it needs runs in one write transaction, which reads the version first, so two processes opening at once migrate it once.

| The store is | At open |
|---|---|
| Missing, or unreadable | Create it from every migration. If canal-lake's folder holds files, warn with their size ([File names](#file-names)) |
| Older than this anfra | Migrate it to this anfra's last version |
| This anfra's version | Use it |
| Newer, but its required version is at most this anfra's current version | Use it as it is. A store is never migrated down |
| Newer, and not compatible | The cache is off for this process, with a warning naming the versions. A store this anfra doesn't understand is never emptied or replaced: a newer anfra may be using it |

**Several processes, several versions.** An `anfra serve` started last week and a CLI upgraded today share the repo's store, and nothing tells one process the other runs, so a migration can't wait for the others to stop. Instead, every store call reads the store's version first (the database header, microseconds). An older process meeting a newer, still compatible version carries on, as SQLite prepares its statements again for the changed schema; meeting an incompatible one, it stops using the cache at once and says so. A read that races the migration in the instant between the check and its query fails like any store error, and runs uncached; a write checks inside its transaction, after its lock, so it can't race at all.

**When a migration fails** (a full disk, a lock held too long, a crash), the transaction rolls back and the store is as it was: the query runs uncached and reports it, and the next query that would be cached opens the store again, so migrates again. A migration that can't apply at all, a bug, is prevented, not handled: a test applies the migrations from every earlier version, with rows, and runs `querycachetest` on the result. anfra never deletes a store because a migration failed.

**In the code:** `sqlitestore.Open` creates or migrates a store, and refuses one it can't use with `ErrOtherVersion` (a `*VersionError`, naming the versions) or `ErrNotStore`; every call after checks the version as above. `sqlitestore.Peek` reads a store's versions without creating or changing it, which is all `anfra status` needs: a status never migrates. The cache's state, as `anfra status` reports it, is the engine's contract (`engine.QueryCacheSummarizer`, `QueryCacheSummary` and its four states), so a platform reports its own cache's state through the same op. anfra's `internal/querycache/local` implements it: it holds the process's store, opened at first use and tried again after a failure, and turns the store's versions into a state (`Summarize`, and `Explain` for a warning). Only anfra's host imports it. Later, `anfra cache status --query` opens the store read-only for its details: entries, size, the oldest and newest.

A platform's store migrates as its database does.

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
| anfra-node | Compile returns the query's cache key, computed from its query structure by the rules above, and takes and returns the query's seeds. Done (0.14.0). | A key from the structure, not the SQL, that a query compiled again with its seeds reproduces. |
| anfra-node | Pages a pivot in its SQL. It refuses a paged pivot today. | anfra core pages on the warehouse, one entry per page. Paging over a cached whole result is a requery, a platform's. |
| canal-lake | Keeps decimals as decimals when asked. Its reads cast decimal columns to strings today. | A cached decimal column must keep its Arrow type, or a hit's header differs from a miss's. |
| canal | anfra-canal: canal-query and canal-lake in one binary, released as a line of its own (canal's [design](https://github.com/holistics/canal/blob/master/docs/design/merged_binary.md)). Done, not released yet. | Embedding canal-lake beside canal-query as a binary of its own would double the sidecars' share of anfra's download. |
| canal-lake | Its pprof address is a setting (`PPROF_ADDR`, as canal-query's), off when unset. Done (canal 3a65a44), not released yet. | Two repos' servers each run a canal-lake: with a fixed address, the second can't take the port. |
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

Status as of 2026-11-13, on the `cache/query-cache` branch.

1. **anfra-node:** the structural cache key, and the seeds, done (0.14.0). To do: paging a pivot in its SQL. Until then, a paged pivot is refused, cached or not.
2. **canal:** anfra-canal and pprof's address, done but not released (`anfra-canal-v1.0.0`, which anfra's manifest already pins). To do: decimals kept, canal-query reading an entry from canal-lake, a read's limit as a setting; the parity test.
3. **The canal-lake sidecar.**
   1. canal-query run from anfra-canal (`internal/sidecar/anfracanal`, the manifest's `anfra_canal` and `anfra_canal_services`, `scripts/sidecars.sh`, `make build`, the release workflow, `anfra publish` recognising anfra-canal, anfra-cloud's `scripts/bump-anfra.sh`). Done.
   2. canal-lake's supervisor ([canal-lake as a sidecar](#canal-lake-as-a-sidecar)), started on demand (`localquerycache`'s `StartLake`); `scripts/sidecars.sh` checking canal-lake's version too. Proved by a test against the real binary (`internal/sidecar/canallake`): a DuckDB query with a canal-lake key writes its Parquet file under the folder anfra gave canal-lake. Done.
4. **The cache.**
   - Done: `input.seeds`, and `pagination` and `timezone` moved into `input` ([a query's input](query-input.md)); `internal/querycache` with `engine/querycachetest`; the SQLite store with its migrations and their compatibility rule; `cache.query.enabled`; where it lives; its state in `anfra status`, and `anfra serve`'s startup check.
   - To do: anfra's key (anfra-node's key and version, the tenant, the data source's type and identity, the row truncation) with its access test; the miss and hit paths in `query` and `query export`, the maximums checked on a hit, and a hit whose file is gone read as a miss (a file deleted by hand, or a store pointed at another canal-lake storage); the `cache` input, with `cache_miss`, and `cache.query.default.duration`, which a query without a `cache.duration` uses; and the answer's `cache` (a change to the core API's contract: the OpenAPI document and the SDK's types); reporting the cache's failures on a query (`cache.warning`, the CLI's warning); uncached when canal-lake fails; the sweep for expired and pending entries; the tags anfra derives; `cache.clean`.
5. **The SDK:** a query's result keeps the `input` it ran with, and replays it for the next page and an export; `refresh` leaves the seeds out. `inspect` showing the answer's `cache`, the `refresh` plugin.
6. **The engine:** the `Store`, `Planner` and `Recorder` on each invocation, with core's implementations; `DataSource.Identity`, which anfra Cloud sets for each data source, and for each viewer under OAuth passthrough.
7. **After:** removing orphaned files (in canal-lake with no row); a command to throw a repo's cache away by hand (`anfra cache clean --query --reset`); the caller's tags (`app`, `block`, an origin, through a `cache.origin` input) and attribution by tag (hits, misses and size per tag, for a `cache.stats` op and `inspect`); the size limit; canal-query streaming on when its canal-lake write fails; the data source settings that change rows, in the key; canal-lake in `anfra status`.
