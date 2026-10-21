# Sidecars

anfra is a Go program with two helper processes, its sidecars. anfra owns the CLI, the server, the repo's config and credentials; the sidecars do what is best done in another runtime.

| Sidecar | What it is | Talks over |
|---|---|---|
| anfra-node | anfra's core Node.js functions: today, compiling and validating AML and AQL, showing a repo's objects, and ingesting, searching and tracing lineage through the catalog. Built from `holistics-core/apps/anfra-node`. | A Unix socket (JSON-RPC over HTTP) |
| canal-query | The query engine: runs SQL against the warehouse, with a connector per database type. Built from `holistics/canal` (`app/query`). | HTTP on a free loopback port |

Each has its own package under `internal/sidecar/` (its supervisor, client and wire types), on the shared framework in `internal/sidecar/` itself: the `Config` they are given, the lifecycle of a spawned one (`process.go`), how its binary is found (`binary.go`), and how one that does not answer is classified (`unreachable.go`). See [architecture.md](architecture.md) for where they sit in a command's path.

## Where the binaries come from

- **Releases embed them.** A build with the `embed_sidecar` tag embeds the binaries dropped in `internal/sidecar/<sidecar>/assets/` (`embed_release.go`); the release workflow puts the pinned ones there for each target ([release.md](release.md)). At startup, anfra writes each one to `<ANFRA_HOME>/sidecars/<name>-<hash>` (`internal/sidecar/binary.go`, `internal/home`), keyed by content hash: written only when missing, so concurrent runs converge on one file, and older versions of the same sidecar are pruned.
- **Builds from source embed nothing** (`embed_dev.go`), and take paths from `ANFRA_NODE_BIN` and `ANFRA_CANAL_QUERY_BIN`. `make build` is the exception: it embeds them as a release does, so `bin/anfra` runs on its own. `make dev` and `make build` default them to the pinned releases, downloaded into `.sidecars/` (`scripts/sidecars.sh`): anfra-node's unminified build, which each anfra-node release publishes beside the minified one releases embed, so a trace names the source's functions.
- **Or none is spawned:** `ANFRA_NODE_URL` and `ANFRA_CANAL_QUERY_URL` point anfra at sidecars something else runs (a compose or Kubernetes deployment, or a long-lived one while developing). anfra then only dials them, and never stops them.

Which sidecar releases a release embeds is pinned in `manifest.yml`, alongside anfra's own version. The sidecars' wire contracts are therefore versioned with anfra: a change to one is a pin bump here.

## Lifecycle

**Spawn** (`internal/sidecar/process.go`). Each sidecar runs in its own process group, so anfra can signal it with everything it started. On Linux the kernel also sends it SIGTERM if anfra dies, even by SIGKILL (`proc_linux.go`, Pdeathsig). Its stderr goes into anfra's log stream and its stdout is discarded, unless configured otherwise (see Logs below).

**Ready.** anfra polls each sidecar's health endpoint until it answers or a deadline passes (each client's `WaitReady`), and fails the command or the server's start with the sidecar's name if it does not.

**Shutdown.** `Close` sends SIGTERM, waits a few seconds (`process.go`), then SIGKILLs the process group. anfra-node uses the time to write out its compile cache.

**Orphans.** A sidecar must not outlive anfra: it would hold its socket and memory for nobody. Besides the process group and Pdeathsig, anfra keeps a pipe open to anfra-node's stdin; when anfra dies however it dies, the pipe closes and anfra-node shuts down (its watchdog, `apps/anfra-node/src/rpc/watchdog.ts`). The watchdog also treats a *change* of its parent PID as anfra having died (it was reparented). It used to treat a parent PID of 1 as that too, which fails wherever anfra is itself PID 1, as in a container: anfra-node shut down seconds after start. anfra-node's source now compares only against the parent PID it started with; an anfra-node release from before that fix still has the old check, which is one reason anfra's Docker image runs tini as PID 1 ([release.md](release.md)). canal-query relies on the process group and Pdeathsig, and has no stdin watchdog.

**Known gap:** a sidecar that exits is not restarted. Every later call that needs it fails with `sidecar_unavailable` until anfra restarts.

## Environment

A spawned sidecar runs with anfra's environment, less the names anfra owns, plus what its supervisor sets (`internal/sidecar/env.go`). The variables a sidecar reads fall into four kinds:

| Kind | Examples | How it gets them |
|---|---|---|
| **What it needs to run** on the machine | `HOME`, `PATH`, `TMPDIR`, the dynamic loader's (`LD_LIBRARY_PATH`, `NIX_LD`) | Inherited. |
| **Its own settings,** which a runner sets for one process | `PPROF_ADDR`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `GOMEMLIMIT`, `GOMAXPROCS`, `GOGC`, `NODE_OPTIONS` | Not inherited: from the sidecar's prefix, `ANFRA_CANAL_QUERY_ENV_<NAME>` or `ANFRA_NODE_ENV_<NAME>`, as `<NAME>`. |
| **Shared with anfra** | `OTEL_EXPORTER_OTLP_*`, `LOG_LEVEL`, `TZ`, `LANG`, proxies, `SSL_CERT_FILE` | Inherited. |
| **A driver's conveniences,** not anfra's to support | `AWS_*`, `GOOGLE_APPLICATION_CREDENTIALS`, `KRB5_CONFIG`, `DATABRICKS_*` | Inherited, until a data source's settings or a setting of the second kind replaces them. |

Inheriting by default is deliberate. The first kind cannot be listed reliably: it is read by the sidecars' libraries and the C code they load, differs by host, and changes with each sidecar release, so a list that misses one breaks the sidecar on the hosts that need it, and no test on anfra's CI hosts would show it. The names anfra owns, it can list: its own (`ANFRA_*`, which also keeps its secrets, such as `ANFRA_GITHUB_TOKEN`, out of the sidecars) and the second kind, which are the ones that collide. Set for anfra, they would also land on a sidecar: two pprof servers on one address, three processes' traces under one service name, anfra's memory limit on each process. A sidecar's supervisor sets the variables it must control (canal-query's `PORT`, `ANFRA_REPO_ID`) over both, and its defaults (anfra-node's `ANFRA_PROGRAM_MEMORY_SIZE`) under the prefix's.

A sidecar setting that is a generic name, read by the sidecar's own code, belongs to the second kind: a new one, in a sidecar release, is added to `stripped` with the pin bump.

A sidecar anfra does not spawn (`ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL`) gets its environment from its own deployment.

> Open: canal-query loads a `.env` from its working directory, which is anfra's, usually the repo's. Values already set win, but a repo's `.env` can add others. Running it from a folder of its own needs anfra to resolve the paths in a `connection` (a DuckDB file's) first, which are relative to that directory today.

## Sockets and ports

anfra-node listens on a Unix socket in the temp dir, `anfra-<pid>.sock` (`internal/sidecar/anfranode/anfranode.go`): per anfra process, so several repos' servers do not collide, and in the temp dir because a Unix socket path is limited to about a hundred bytes, which a deep home folder can exceed. canal-query listens on a free loopback port chosen at spawn (`internal/sidecar/canalquery/canalquery.go`).

## One-shot and warm

A CLI command spawns only the sidecars it declares it needs (`Needs`), and stops them when it is done. `anfra serve` starts both and keeps them for its lifetime; CLI commands in that repo are forwarded to it rather than spawning their own ([architecture.md](architecture.md)). A warm canal-query gets connection pooling (`Config.EnablePooling`, set by `serve` and by the engine's `Connect`): database connections are reused across requests, which matters most for a remote warehouse, where opening a connection can cost more than the query. An in-memory DuckDB (no `path`, or `:memory:`) is never pooled (`poolFor` in `internal/sidecar/canalquery/client.go`): each of its connections is a database of its own, so a pooled one would carry one query's tables into the next, and canal-query refuses it.

## anfra-node's compile cache

Compiling a repo's AML is anfra-node's largest fixed cost, so it caches the compiled program, by the repo id each request carries (a process may serve many repos):

- **On disk:** the serialized program, under `ANFRA_COMPILE_CACHE_PATH`, which anfra sets to the repo's own cache folder (`<ANFRA_HOME>/repos/<id>/cache/compile-cache`, `cmd/anfra/host.go`). A request starts from it and re-interprets only the files that changed (`apps/anfra-node/src/amql-shell/aml/program.ts`).
- **In memory,** between requests: off unless `ANFRA_PROGRAM_MEMORY_SIZE` says how many repos' programs to keep, because what is safe depends on how many repos one process serves, which only its platform knows. anfra sets it to 1 for the anfra-node it spawns, which serves one repo, unless `ANFRA_NODE_ENV_ANFRA_PROGRAM_MEMORY_SIZE` says otherwise (`ProcSpec.DefaultEnv`). With it on, the disk cache is written shortly after a program's last use rather than on every request, and flushed on shutdown (`program-cache.ts`).

Clearing the cache (`aml.clear_cache`) clears both.

## What canal-query is given

anfra reads `.anfra/data_sources.yml` itself (`internal/datasource`). For each query it sends canal-query the data source's `type` as `dbtype`, its `connection` map, unchanged, as `dbconfig`, and its `settings` map, unchanged, as `dbsetting` (`internal/sidecar/canalquery/client.go`). Credentials never reach anfra-node, which only gets each source's name and type, to pick the SQL dialect.

`dbsetting` holds the options canal-query keeps apart from the connection: timeouts (`connect_timeout_ms`, `statement_timeout_ms`), requiring and verifying SSL (`require_ssl`, `verify_ssl`), and each type's own (BigQuery's `maximum_bytes_billed`, Snowflake's `oauth_passthrough`, Presto's `use_trino_driver`, MySQL's `sql_modes`, ...). A data source without `settings` sends it empty, which leaves each at canal-query's default. anfra checks none of it: canal-query ignores a key it does not know, and refuses a value of the wrong type when the query runs.

## Telemetry and logs

**Traces.** When anfra exports traces ([configs](../usage/configs.md)), it also points anfra-node at the same collector, through the `H_OTEL_*` variables its tracing library reads (`internal/telemetry`), unless they are set already. Calls to the sidecars carry the trace (`traceparent`) through `Config.WrapTransport`, which the CLI and `serve` set to an otelhttp transport (`cmd/anfra/telemetry.go`); the engine's `WithTransport` lets an embedding platform do the same ([engine.md](engine.md)). So one trace runs from the command or request into anfra-node's own spans.

**Logs.** Both sidecars' stderr goes into the repo's `anfra.log`, with anfra's own records (`internal/logging`). `ANFRA_LOG_STDERR` also copies that stream to anfra's stderr; `ANFRA_SIDECAR_STDOUT` passes the sidecars' stdout through instead of discarding it.
