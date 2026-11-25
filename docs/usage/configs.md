# Configuring anfra

How to configure an anfra setup: the repo, your own settings for it, where a setting goes, `anfra serve`, telemetry, and the environment.

## The repo: `.anfra/`

A repo's config is its `.anfra/` folder, committed with it (`anfra init` creates it). The name is fixed: it is part of the repo's format, so every clone finds it.

| File | What it is |
|---|---|
| `data_sources.yml` | The data sources the repo's models query, by name: a `type`, a `connection` passed to the database driver unchanged, and optional `settings` for the query engine (timeouts such as `statement_timeout_ms`, `require_ssl` and the like; see [sidecars](../designs/sidecars.md#what-canal-query-is-given)). It holds credentials, so it is git-ignored; `anfra init` also creates `data_sources.yml.example`, the same without them, to commit. Each type's connection keys are listed in the anfra skills' [data source reference](https://github.com/holistics/anfra-skills/blob/main/plugins/anfra/skills/setup-repo/references/data-sources.md). |
| `context_sources.yml` | What `anfra ingest` reads into the local catalog that `anfra search` searches and `anfra lineage` traces through; by default, the repo's own semantic layer. |
| `config.yml` | The repo's settings, written by hand and committed ([below](#configyml)). `anfra init` creates it with every setting at its default. `anfra publish` publishes it with the repo. |
| `config.local.yml` | Your own settings on this machine, laid over `config.yml` ([below](#configlocalyml)). Never committed or published: `anfra init` adds it to `.gitignore`. |

anfra reads them on every command, so an edit needs no restart, except `config.yml`'s `query` settings, which `anfra serve` reads when it starts.

### `config.yml`

The repo's settings, written by hand and committed. It never holds a credential. anfra refuses a key it doesn't know, naming its line and the key meant, rather than ignoring it, and a setting with no value (`enabled:` or `enabled: ~`), since a setting is unset by leaving it out.

`anfra publish` publishes `config.yml` with the repo's other files, so the org runs with the same settings, and `anfra pull` merges it as any other file. `org.url` only says where this folder publishes, so the org ignores it. On the org:

- **Changing a setting is an Admin's.** A publish that changes one, as anything but an Admin, is refused with `settings_change_forbidden`, and nothing is published. Comments and formatting aren't settings, so anyone who can publish can change them.
- **An Admin can override a setting on the org,** in its settings, without publishing. The override stays across publishes, and `anfra publish` warns when it changes a setting the org overrides, since the change takes no effect there. Overrides don't apply on your machine.

| Setting | Effect |
|---|---|
| `org.url` | The org `anfra publish` publishes the repo to, by its address on its server (`https://bi.example.com/o/acme`), so everyone publishes to the same org. `anfra publish --set-org` sets it, changing nothing else in the file. |
| `apps.allow_origins` | Origins the repo's Data Apps may load scripts, styles and fonts from, and send requests to, beyond the SDK's own CDNs, as `anfra serve` runs them, each `https://<host>[:<port>]`. It's never published, since an org keeps its own list, and `anfra publish` says which of them the org doesn't allow. |
| `query.default.input.timezone` | The IANA time zone an AQL query uses for relative dates and date truncation, when it sets no `input.timezone` of its own, such as `Asia/Ho_Chi_Minh`. Default: `Etc/UTC`. |
| `query.default.input.weekStartDay` | The day weeks start on, for an AQL query that sets no `input.weekStartDay` (keys under `query.default.input` are camelCase, as a query's `input` names them): for grouping by week, relative dates such as `this week`, and the day of the week's number. One of `monday` to `sunday`, in lowercase. Default: `monday`, on every database. |
| `query.locked_defaults` | The defaults above that a query can't change, by their path under `query.default`: `input.timezone`, `input.weekStartDay` or `cache.duration`. A query may leave a locked input out, or send the same value; any other value is refused. Each must be set under `query.default`. Default: none. |
| `query.default.cache.duration` | How old a cached result a query accepts, and how long a result it caches is kept, for every query that sets no `cache.duration` of its own: a duration such as `5m` or `1h`, at most `168h` (7 days). It applies to every caller, `anfra query` and MCP included, so an agent asking a question again gets the cached answer. To cache only a Data App's queries, set the app's own default instead, in its `createApp` (`query: { default: { cache: { duration: '5m' } } }`), which takes precedence for its queries. Default: unset, so a query uses the cache only when it, or its Data App, asks. |
| `query.cache.enabled` | `false` turns the query cache off for the repo: anfra keeps no store and starts no canal-lake. A query that asks for the cache runs on the warehouse, and its answer says why (`cache.warning`, `cache_disabled`); `anfra cache clean` fails. Default: `true`. `ANFRA_QUERY_CACHE_ENABLED` overrides it for one process ([below](#query-cache)). |
| `query.cache.max_duration` | The longest a query's result may be cached, from `1s` to `168h` (7 days). A query that asks for longer, by its own `cache.duration`, is refused with the cap to use, rather than cached for less than it asked. A `query.default.cache.duration` over it is an error in the file. Default: `168h`. |
| `aql.compiler.version` | The AQL compiler that queries and `anfra validate` use: `v1` (default) or `v2` (experimental). anfra reads it on every command. |

`anfra serve` reads the `query` settings when it starts: restart it after changing them. The same goes for `config.local.yml`.

If `config.yml` holds a key or a value anfra doesn't take, every query fails with `repo_settings_invalid` and the file's error, rather than run with defaults the repo may not mean.

For example:

```yaml
org:
  url: https://bi.example.com/o/acme
apps:
  allow_origins:
    - https://cdn.acme.com
query:
  default:
    input:
      timezone: Asia/Ho_Chi_Minh
      weekStartDay: sunday
    cache:
      duration: 5m
  locked_defaults:
    - input.weekStartDay
  cache:
    max_duration: 24h
```

### `config.local.yml`

Your own settings, on this machine only, with the same keys as `config.yml`, read as strictly. Each setting it sets replaces `config.yml`'s, and a list replaces the whole list. Use it to try a setting before the team takes it, such as the v2 compiler or a longer cache, or to publish one folder to another org (`org.url`).

It's never committed or published, so it never changes what others run with:

- `anfra status` lists what it sets.
- `anfra publish` warns about each setting it gives a value the org doesn't use. The org validates with its own settings, so a repo that passes here with your settings can still be refused there.

```yaml
aql:
  compiler:
    version: v2
```

## Your settings: `.env`

Your own settings for a repo go in `.env` at its root, beside `.anfra/`. It is optional. You write it, by hand or through your agent; anfra never does. Each line is a variable, as you would set it in the environment:

```sh
# .env
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318   # where this repo's traces go (observability-and-eval/telemetry.md)
ANFRA_EXPERIMENTAL_TRACE_DATA=0
```

- **Only its `ANFRA_*` and `OTEL_*` names are read,** as if they were in the environment. Other names belong to your other tools, such as an app that shares the folder, and anfra leaves them alone. The format is dotenv's: `NAME=value` lines, `#` comments, optional quotes, nothing expanded.
- **The environment wins,** name by name.
- **It is yours.** `anfra init` adds `.env` to `.gitignore`, so your settings never reach a teammate, and `anfra publish` never sends it.
- **A `.env` that Git tracks is never read,** since anyone who clones the repo would get its settings, including where its traces go. anfra warns when such a file sets an `ANFRA_*` or `OTEL_*` name.
- **The sidecars get its `OTEL_*` names as they get the environment's.** The sidecar prefixes, `ANFRA_NODE_ENV_*`, `ANFRA_CANAL_QUERY_ENV_*` and `ANFRA_CANAL_LAKE_ENV_*` ([below](#sidecars)), aren't read from it, so set them in the environment.

anfra reads it when a command starts, so an edit applies to the next command. A running `anfra serve` reads it when it starts: restart it after an edit. A bad value for one of anfra's switches is refused, naming the line.

## Where a setting goes

A setting goes where the person who decides it can set it:

| Where | What goes there | Who sets it |
|---|---|---|
| The repo's `.anfra/` | What describes the repo, and is the same for everyone who clones it | The repo's authors, through Git |
| The repo's `.anfra/config.local.yml` | Your own values for the repo's settings, on this machine | You, or your agent, by editing the file |
| The repo's `.env` | Your choices for this repo, which have to reach your agents too | You, or your agent, by editing the file |
| The environment | Choices for one process or deployment: a container, a service, CI, or `anfra serve` run for others | Whoever starts the process |

When a setting can come from more than one place, the first one set wins: the environment, then the repo's `.env`, then `.anfra/config.local.yml`, then `.anfra/config.yml`. `anfra status` shows where the telemetry endpoint came from.

Your `.env` wins over the committed `.anfra/config.yml`. A repo is often someone else's work, and anfra doesn't ask whether you trust it. So a committed file can only make a security setting stricter, never looser. Telemetry's settings never come from it: a repo can never choose where your traces go, or what they record.

A setting belongs in `.env` when it has to reach agents: an agent can edit a file, but it can't change the environment its own commands run in.

## `anfra serve`

`anfra serve` serves everything unless turned off (`anfra serve --help`, `cmd/anfra/serve.go`):

| Flag | Effect |
|---|---|
| `--addr host:port` | Where to listen. Default: a loopback address, or a free port when another repo's server holds it; `anfra status` says where. Anything but loopback is reachable by others, and `serve` has no authentication. |
| `--no-mcp` | Without the MCP endpoint (`/mcp`). |
| `--no-apps` | Without the Data Apps (`/`, `/apps/…`). |
| `--no-watch` | The Data Apps without live reload. |

## Telemetry

anfra can record each command's trace and send it, over OTLP/HTTP, to an OpenTelemetry endpoint you choose ([telemetry](observability-and-eval/telemetry.md)). Nothing is recorded until an endpoint is set, and nothing is ever sent to Holistics. Telemetry is experimental, as the `EXPERIMENTAL` in its settings says, so they may change in a minor release.

Each variable below can be set in the environment or in the repo's [`.env`](#your-settings-env). **On your machine,** set the endpoint in the repo's `.env`. Every anfra command in the repo reads it, including the ones your agents run:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

**In a container, a service or CI,** set it in the environment instead, which wins over the file.

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/HTTP: the collector's base URL, e.g. `http://localhost:4318`. anfra points anfra-node and canal-query at it too. Default: none, so nothing is exported. |
| `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` | anfra sends over HTTP only, and warns when this is `grpc`. Point the endpoint at an HTTP receiver (usually port 4318). |
| `ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT` | Another endpoint for canal-query's traces, instead of anfra's. It exports over gRPC then, unless `ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`. Default: anfra's endpoint, over HTTP. |
| `OTEL_EXPORTER_OTLP_TIMEOUT`, `OTEL_EXPORTER_OTLP_TRACES_TIMEOUT`, `OTEL_BSP_EXPORT_TIMEOUT` | How long one export may take, in milliseconds. Default: 1500, so a collector that is down delays a command by no more than that. |
| `ANFRA_EXPERIMENTAL_TRACE_DATA` | Whether traces record your data: what a command asked for, the SQL it ran, what came back (never a query's rows), and an error's message and stack. These can hold your filter values ([what a trace holds](observability-and-eval/telemetry.md#what-a-trace-holds)). `0` leaves it out. Default: `1`, with a warning on every command when traces go to another machine. |
| `ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT` | The bytes kept of each input, answer and query; a longer one is cut and marked. A whole number. Default: `16384`. |
| `ANFRA_SESSION_ID` | The session of the agent that runs anfra's commands, for an agent anfra doesn't detect. Set it where the agent starts, and every command the agent runs inherits it. A detected agent's own id wins over it ([agent tracing](observability-and-eval/agent-tracing.md)). Default: none. |
| `ANFRA_EXPERIMENTAL_TRACE_BAGGAGE_INCLUDE` | The keys of a caller's W3C baggage that anfra records, on every span of the call: from `BAGGAGE` for the CLI, or the `baggage` header or MCP's `_meta.baggage` for `anfra serve`. A comma-separated list of keys, where `*` and `?` are wildcards, such as `gen_ai.conversation.id,run.id`. `none` lets no baggage in. A key starting with `anfra.` is never let in, except a task's id, `anfra.task.id`, which always is, in a task id's format only. Default: `gen_ai.conversation.id`. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | The traces endpoint alone, a full URL ending in `/v1/traces`, instead of the base URL. |
| `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | Headers sent with each export, such as a hosted backend's API key, as `key=value,key=value`. In the repo's `.env`, they reach your agents' commands too ([telemetry](observability-and-eval/telemetry.md#a-backend-that-needs-a-key)). Default: none, which is what the telemetry stack needs. |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | Label anfra's traces, in OpenTelemetry's format (`key=value,key=value` for the attributes). Default service name: `anfra`. The sidecars don't inherit them; set theirs with `ANFRA_CANAL_QUERY_ENV_` or `ANFRA_NODE_ENV_` (above). |
| `OTEL_SDK_DISABLED` | `true` exports nothing, whatever else is set. |

## Environment

Set these in the environment anfra runs in (the shell, a container's environment, a service's unit file), or in the repo's [`.env`](#your-settings-env). With `anfra serve` running, CLI commands in that repo run on the server, so it's the server's environment that counts for them.

Telemetry's variables are listed in [Telemetry](#telemetry).

anfra's on/off switches take `1` (on) or `0` (off); unset means off. Any other value is refused when anfra starts, naming the variable, so a mistyped switch never silently does the opposite. Standard variables (`CI`, the `OTEL_*` ones) keep their own conventions.

### Paths

| Variable | Effect |
|---|---|
| `ANFRA_HOME` | anfra's own folder: the installed binary, the unpacked sidecars, each repo's logs and caches (`internal/home`). A repo's query cache is in `repos/<repo id>/cache/query/`: its store, `store.sqlite`, and the results of the canal-lake anfra starts, `lake/`. In a container, put it on a lasting volume to keep the cache across restarts. A path. Default: `~/.anfra`. The installer honours it too. |

### Query cache

See [designs/query-cache.md](../designs/query-cache.md).

| Variable | Effect |
|---|---|
| `ANFRA_QUERY_CACHE_ENABLED` | `0` turns the query cache off for this process, whatever the repo's `query.cache.enabled`; `1` turns it on over a repo's `false`. For a run that must see live data, such as CI, or to rule the cache out while debugging. Default: unset, so the repo's setting applies. |
| `ANFRA_CACHE_QUERY_NAMESPACE` | The folder in canal-lake's storage that anfra writes its results to, for several anfras sharing one storage ([below](#running-canal-query-and-canal-lake-yourself)). Default: none. |

### Sidecars

See [designs/sidecars.md](../designs/sidecars.md).

| Variable | Effect |
|---|---|
| `ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL` | Use sidecars something else runs (a compose or Kubernetes deployment) instead of spawning them: their base URLs, e.g. `http://anfra-node:8080`. Default: unset, and anfra spawns its own. |
| `ANFRA_NODE_BIN`, `ANFRA_CANAL_BIN` | The sidecar binaries, as absolute paths, for a build that embeds none (`go build`, `make dev`): anfra-node, and anfra-canal, which runs canal-query and canal-lake. `ANFRA_CANAL_QUERY_BIN` and `ANFRA_CANAL_LAKE_BIN` name a canal-query or canal-lake built alone, run in place of anfra-canal's. Releases and `make build` embed them, and ignore these. |
| `ANFRA_CANAL_QUERY_ENV_<NAME>`, `ANFRA_CANAL_LAKE_ENV_<NAME>`, `ANFRA_NODE_ENV_<NAME>` | Set `<NAME>` in the canal-query, canal-lake or anfra-node that anfra spawns, and in no other process: `ANFRA_CANAL_QUERY_ENV_PPROF_ADDR=localhost:6061` serves canal-query's pprof there. The sidecars inherit anfra's environment otherwise, except anfra's own `ANFRA_*` and the settings each process has its own of, which they get only this way: `PPROF_ADDR`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `GOMEMLIMIT`, `GOMAXPROCS`, `GOGC`, `NODE_OPTIONS` ([designs/sidecars.md](../designs/sidecars.md#environment)). Values anfra sets for a sidecar, such as canal-query's `SOCKET`, cannot be changed, except anfra-node's `ANFRA_PROGRAM_MEMORY_SIZE` (below). |

#### Running anfra-node (the sidecar, not anfra) yourself

A platform that runs **anfra-node** as a service, rather than letting anfra spawn it, configures it with these. They are the sidecar's own settings, read by anfra-node, not by anfra:

| Setting | Effect |
|---|---|
| `--port`, or `ANFRA_PORT` | The port to listen on. One of this or a socket (`--socket`, `ANFRA_SOCKET`) is required. |
| `ANFRA_HOST` | The address to listen on, with `--port` or `ANFRA_PORT`. Default: `0.0.0.0`. |
| `ANFRA_COMPILE_CACHE_PATH` | Where compiled programs are cached on disk. Default: `~/.anfra/compile-cache`. anfra sets it per repo for the anfra-node it spawns. |
| `ANFRA_PROGRAM_MEMORY_SIZE` | How many repos' compiled programs to keep in memory between requests: a whole number. Default: `0`, none. Size it to the repos one process serves and the memory it has; anfra sets `1` for the anfra-node it spawns, which serves one repo; `ANFRA_NODE_ENV_ANFRA_PROGRAM_MEMORY_SIZE=0` turns that off. |
| `LOG_LEVEL` | As below. |

#### Running canal-query and canal-lake yourself

A platform that runs **canal-query** as a service (`ANFRA_CANAL_QUERY_URL`) also runs its **canal-lake**, which keeps the query cache's results. anfra then starts no canal-lake, and never calls one. canal-query writes results to canal-lake, and reads and drops them, at the address its settings give. Run both from anfra-canal (`anfra-canal query`, `anfra-canal lake`), or from canal's own images at the releases `manifest.yml` records in `anfra_canal_services`. Configure them with the settings below. canal-query and canal-lake read these, not anfra, and anfra sets the same ones for the canal-query and canal-lake it spawns.

| Setting | On | Effect |
|---|---|---|
| `SOCKET`, or `PORT` | both | Where it listens: a Unix socket's path, or a port. Default: port 1323 for canal-query, 1324 for canal-lake. |
| `SKIP_HOLISTICS_DB=1` | canal-query | Runs canal-query without the Holistics app's database, which anfra doesn't have. |
| `ENABLE_DUCKDB_CONNECTOR=1` | canal-query | Lets it query DuckDB data sources. |
| `EXPOSE_ERROR_CAUSES=1` | canal-query | Sends anfra the cause of each failure, apart from its message, so anfra can log it ([errors](../designs/errors.md#sidecar-failures)). |
| `CANAL_LAKE_SOCKET`, or `CANAL_LAKE_HOST` and `CANAL_LAKE_PORT` | canal-query | Where its canal-lake listens. Default: `localhost:1324`. |
| `STORAGE_ROOT` | canal-lake | The folder canal-lake keeps its result files in, such as `/data/canal-lake/`. End the path with `/`, because canal-lake appends `duckdb.tmp/` to it for DuckDB's temporary files. Default: `/tmp/canal_lake/`. |
| `LAKE_READ_TIMEOUT_SECONDS` | canal-lake | The longest a read of a cached result may take, from its start to its last row sent: a `query export` from the cache must finish writing its file within it. Default: 60. anfra leaves the default for the canal-lake it spawns; raise it there with `ANFRA_CANAL_LAKE_ENV_LAKE_READ_TIMEOUT_SECONDS`. |
| `LAKE_STREAMING_READS=1` | canal-lake | Sends a cached result as DuckDB reads it, rather than reading all of it into memory first. With it, the read limit also stops a read mid-fetch. Default: off. anfra sets it for the canal-lake it spawns, so set it on yours too. |
| `DUCKDB_MAX_CONNECTIONS` | canal-lake | The most reads canal-lake serves at once, since each read holds one DuckDB connection until it ends. Default: 64. |
| `DUCKDB_MEMORY_LIMIT` | canal-lake | DuckDB's memory limit, such as `4GB`. Set it below the container's memory, so that a read too big fails on its own, rather than the kernel stopping canal-lake and every read with it. A read's result is outside this limit: without streaming reads, leave room above it for the whole results of `DUCKDB_MAX_CONNECTIONS` reads. Default: a share of the memory DuckDB detects. |
| `SHUTDOWN_GRACE_SECONDS` | canal-lake | How long canal-lake waits for its calls to finish when stopped. Default: 1200 (20 minutes). anfra sets 2 for the canal-lake it spawns. |

If several anfras write to one canal-lake storage, replicas included, give each its own namespace with `ANFRA_CACHE_QUERY_NAMESPACE`, such as `prod` or `prod/eu`. A namespace is letters, digits and `_`, with `/` between names. anfra then writes its files in that subfolder of the storage. Without a namespace, anfra writes them at the storage's top level, named by the entry numbers in its own store (`1234.parquet`), so two anfras writing there overwrite each other's results. anfra keeps the store, which lists the results, in its own folder, at `<ANFRA_HOME>/repos/<repo id>/cache/query/store.sqlite`, wherever canal-lake keeps the results. Keep the storage for as long as the store, by putting both on lasting volumes, or by deleting them together. If anfra creates a new store while the storage still holds files, it warns with their size, and leaves deleting them to you ([query-cache.md](../designs/query-cache.md#file-names)).

### `anfra serve`

| Variable | Effect |
|---|---|
| `ANFRA_SITE_URL` | The URL callers reach the server at, when it is not the one their requests name: behind a proxy that terminates TLS or serves it under a path, say `https://data.example.com/anfra`. Links the server makes (an export's) start with it. Default: unset, and a link starts with the scheme and host the caller used. |
| `TMPDIR` | Where exports' files go until their links expire (`anfra-exports-<uid>/` in it). Default: the system's temp directory, `/tmp` on Linux. Where `/tmp` is held in memory (`tmpfs`), point it at a disk for large exports. |

### Logs

`anfra logs` shows the repo's log, and `anfra logs --help` how to narrow it.

| Variable | Effect |
|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error`, case-insensitive. Default: `info`, also for any other value. anfra-node inherits it. |
| `ANFRA_LOG_STDERR` | `1`: also copy the log stream (anfra's records and the sidecars' stderr) to stderr; `0`: don't. Default: on for `anfra serve`; for any other command, only to `<ANFRA_HOME>/repos/<repo id>/logs/anfra.log`, which `anfra status` prints. On a command, it is for debugging: a program that reads a command's stderr as YAML leaves it off. |
| `ANFRA_SIDECAR_STDOUT` | `1`: pass the sidecars' stdout through. Default: discarded. |

### Errors

| Variable | Effect |
|---|---|
| `ANFRA_HIDE_ERROR_CAUSES` | `1`: hide errors' causes from clients, showing only their message. Default: causes shown, since anfra's user is usually its operator. Set it when `anfra serve` serves other people: a cause can carry SQL, database errors and paths. It hides them at the terminal too; every cause is still in `anfra.log`. |

### Profiling

| Variable | Effect |
|---|---|
| `ANFRA_PPROF_ADDR` | Under `anfra serve`, serve Go's profiling endpoints for anfra (`/debug/pprof/`, for `go tool pprof`) at this address, e.g. `localhost:6060`, apart from the server. An address it cannot listen on fails the start. Keep it on loopback: they show anfra's memory, with no authentication. Default: unset, not served. canal-query has its own: `ANFRA_CANAL_QUERY_ENV_PPROF_ADDR`, at another address. |

### Updates

See [designs/release.md](../designs/release.md#updates).

| Variable | Effect |
|---|---|
| `ANFRA_NO_UPDATE_NOTIFIER` | `1`: no update checks or notices. |
| `ANFRA_NO_APP_LOGS` | `1`: `anfra serve` writes nothing its Data Apps report from the browser (their runtime errors, console lines, which can hold their data, and mounts) to `anfra.log`. The inspect panel still lists them. |
| `ANFRA_AUTO_UPDATE` | `1`: install a newer release in the background, instead of only noticing it. |
| `CI` | When set, anfra never starts a background update check. CI systems set it. |

### Publishing and login

See [Publishing and login](publishing.md) for signing in, publishing a repo, and managing
stored credentials. The repo's publish destination is `org.url` in `.anfra/config.yml` (above).

| Variable | Effect |
|---|---|
| `ANFRA_CLOUD_URL` | Overrides the cloud servers the build offers for `anfra auth login`. An explicitly empty value disables them. One server's URL, or a `;`-separated list of `Label=URL` entries, such as `United States=https://us.example.com;Europe`, where a label alone is a server that isn't available yet. Each URL is an HTTPS origin, or HTTP on literal `127.0.0.1` or `[::1]` for local testing. See [DEVELOPMENT.md](../../DEVELOPMENT.md#cloud-login-url) for build-time configuration and URL requirements. |

### Data Apps

| Variable | Effect |
|---|---|
| `TZ` | The time zone a Data App's reader has, as an IANA name such as `Asia/Singapore` (`internal/appserve/reader.go`). Default: the machine's. |

## Development

For working on anfra itself; see [DEVELOPMENT.md](../../DEVELOPMENT.md). `make` reads these from `.env.local` (from `.env.local.example`, which `make setup` copies) and passes them to everything it runs.

**`ANFRA_DEV_REPO`:** the repo `make dev` runs `anfra serve` in, as a path, relative to the anfra checkout or absolute. Required by `make dev`; no default.

**`ANFRA_NODE_BIN`, `ANFRA_CANAL_BIN`:** the sidecars for a build from source, as above. Absolute paths, since `anfra serve` runs from inside `ANFRA_DEV_REPO`. Or set `ANFRA_NODE_URL` and `ANFRA_CANAL_QUERY_URL` to dial sidecars already running, so a rebuild doesn't respawn them.

**`ANFRA_APPSERVE_DEV_URL`:** where `make dev`'s Vite dev server serves the Data App frontend from source; `anfra serve`'s pages redirect there. A base URL. Default under `make dev`: `http://127.0.0.1:5173` (the `Makefile`); without it, `anfra serve` serves its built frontend and redirects nothing.

**`ANFRA_SERVE_URL`:** the `anfra serve` that Vite's dev server proxies `/api` and `/appserve` to. A base URL. Default: `http://127.0.0.1:7878` (`web/appserve/vite.config.ts`).

**`ANFRA_BASE_VERSION`:** the release a build from source reports itself after, as `<this>+<commit>`. Read from `manifest.yml` by the `Makefile`, which takes precedence over the environment; not set by hand.
