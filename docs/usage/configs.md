# Configuring anfra

How to configure an anfra setup: the repo, `anfra serve`, and the environment.

## The repo: `.anfra/`

A repo's config is its `.anfra/` folder, committed with it (`anfra init` creates it). The name is fixed: it is part of the repo's format, so every clone finds it.

| File | What it is |
|---|---|
| `data_sources.yml` | The data sources the repo's models query, by name: a `type`, a `connection` passed to the database driver unchanged, and optional `settings` for the query engine (timeouts such as `statement_timeout_ms`, `require_ssl` and the like; see [sidecars](../designs/sidecars.md#what-canal-query-is-given)). It holds credentials, so it is git-ignored; `anfra init` also creates `data_sources.yml.example`, the same without them, to commit. Each type's connection keys are listed in the anfra skills' [data source reference](https://github.com/holistics/anfra-skills/blob/main/plugins/anfra/skills/setup-repo/references/data-sources.md). |
| `context_sources.yml` | What `anfra ingest` reads into the local catalog that `anfra search` searches and `anfra lineage` traces through; by default, the repo's own semantic layer. |
| `config.yml` | The repo's settings, written by hand. `org.url`: the org `anfra publish` publishes the repo to, by its address on its server (`https://bi.example.com/o/acme`), committed so everyone publishes to the same org. `anfra publish --set-org` sets it, changing nothing else in the file. `apps.allow_origins`: origins the repo's Data Apps may load scripts, styles and fonts from, and send requests to, beyond the SDK's own CDNs, as `anfra serve` runs them, each `https://<host>[:<port>]` (`[https://cdn.acme.com]`); never published, since an org keeps its own list, and `anfra publish` says which of them the org does not allow. It never holds a credential. |

anfra reads them on every command, so an edit needs no restart.

## `anfra serve`

`anfra serve` serves everything unless turned off (`anfra serve --help`, `cmd/anfra/serve.go`):

| Flag | Effect |
|---|---|
| `--addr host:port` | Where to listen. Default: a loopback address, or a free port when another repo's server holds it; `anfra status` says where. Anything but loopback is reachable by others, and `serve` has no authentication. |
| `--no-mcp` | Without the MCP endpoint (`/mcp`). |
| `--no-apps` | Without the Data Apps (`/`, `/apps/…`). |
| `--no-watch` | The Data Apps without live reload. |

## Environment

Set these in the environment anfra runs in: the shell, a container's environment, a service's unit file. With `anfra serve` running, CLI commands in that repo run on the server, so it's the server's environment that counts for them.

anfra's on/off switches take `1` (on) or `0` (off); unset means off. Any other value is refused when anfra starts, naming the variable, so a mistyped switch never silently does the opposite. Standard variables (`CI`, the `OTEL_*` ones) keep their own conventions.

### Paths

| Variable | Effect |
|---|---|
| `ANFRA_HOME` | anfra's own folder: the installed binary, the unpacked sidecars, each repo's logs and caches (`internal/home`). A path. Default: `~/.anfra`. The installer honours it too. |

### Sidecars

See [designs/sidecars.md](../designs/sidecars.md).

| Variable | Effect |
|---|---|
| `ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL` | Use sidecars something else runs (a compose or Kubernetes deployment) instead of spawning them: their base URLs, e.g. `http://anfra-node:8080`. Default: unset, and anfra spawns its own. |
| `ANFRA_NODE_BIN`, `ANFRA_CANAL_QUERY_BIN` | The sidecar binaries, as absolute paths, for a build that embeds none (`go build`, `make dev`). Releases and `make build` embed them, and ignore these. |
| `ANFRA_CANAL_QUERY_ENV_<NAME>`, `ANFRA_NODE_ENV_<NAME>` | Set `<NAME>` in the canal-query or anfra-node that anfra spawns, and in no other process: `ANFRA_CANAL_QUERY_ENV_PPROF_ADDR=localhost:6061` serves canal-query's pprof there. The sidecars inherit anfra's environment otherwise, except anfra's own `ANFRA_*` and the settings each process has its own of, which they get only this way: `PPROF_ADDR`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `GOMEMLIMIT`, `GOMAXPROCS`, `GOGC`, `NODE_OPTIONS` ([designs/sidecars.md](../designs/sidecars.md#environment)). Values anfra sets for a sidecar, such as canal-query's `SOCKET`, cannot be changed, except anfra-node's `ANFRA_PROGRAM_MEMORY_SIZE` (below). |

#### Running anfra-node (the sidecar, not anfra) yourself

A platform that runs **anfra-node** as a service, rather than letting anfra spawn it, configures it with these. They are the sidecar's own settings, read by anfra-node, not by anfra:

| Setting | Effect |
|---|---|
| `--port`, or `ANFRA_PORT` | The port to listen on. One of this or a socket (`--socket`, `ANFRA_SOCKET`) is required. |
| `ANFRA_HOST` | The address to listen on, with `--port` or `ANFRA_PORT`. Default: `0.0.0.0`. |
| `ANFRA_COMPILE_CACHE_PATH` | Where compiled programs are cached on disk. Default: `~/.anfra/compile-cache`. anfra sets it per repo for the anfra-node it spawns. |
| `ANFRA_PROGRAM_MEMORY_SIZE` | How many repos' compiled programs to keep in memory between requests: a whole number. Default: `0`, none. Size it to the repos one process serves and the memory it has; anfra sets `1` for the anfra-node it spawns, which serves one repo; `ANFRA_NODE_ENV_ANFRA_PROGRAM_MEMORY_SIZE=0` turns that off. |
| `LOG_LEVEL` | As below. |

### `anfra serve`

| Variable | Effect |
|---|---|
| `ANFRA_SITE_URL` | The URL callers reach the server at, when it is not the one their requests name: behind a proxy that terminates TLS or serves it under a path, say `https://data.example.com/anfra`. Links the server makes (an export's) start with it. Default: unset, and a link starts with the scheme and host the caller used. |
| `TMPDIR` | Where exports' files go until their links expire (`anfra-exports-<uid>/` in it). Default: the system's temp directory, `/tmp` on Linux. Where `/tmp` is held in memory (`tmpfs`), point it at a disk for large exports. |

### Logs

| Variable | Effect |
|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error`, case-insensitive. Default: `info`, also for any other value. anfra-node inherits it. |
| `ANFRA_LOG_STDERR` | `1`: also copy the log stream (anfra's records and the sidecars' stderr) to stderr; `0`: don't. Default: on for `anfra serve`; for any other command, only to `<ANFRA_HOME>/repos/<repo id>/logs/anfra.log`. On a command, it is for debugging: a program that reads a command's stderr as YAML leaves it off. |
| `ANFRA_SIDECAR_STDOUT` | `1`: pass the sidecars' stdout through. Default: discarded. |

### Errors

| Variable | Effect |
|---|---|
| `ANFRA_HIDE_ERROR_CAUSES` | `1`: hide errors' causes from clients, showing only their message. Default: causes shown, since anfra's user is usually its operator. Set it when `anfra serve` serves other people: a cause can carry SQL, database errors and paths. It hides them at the terminal too; every cause is still in `anfra.log`. |

### Telemetry

anfra exports traces over OTLP/HTTP when an endpoint is set, through the standard OpenTelemetry variables (`internal/telemetry`). Default: no endpoint, nothing exported.

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/HTTP, for anfra and anfra-node: the collector's base URL, e.g. `http://localhost:4318`. anfra points anfra-node at it. |
| `ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/gRPC, for canal-query: the collector's gRPC receiver, e.g. `http://localhost:4317`. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | The traces endpoint alone, a full URL ending in `/v1/traces`, instead of the base URL. |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | Label anfra's traces, in OpenTelemetry's format (`key=value,key=value` for the attributes). Default service name: `anfra`. The sidecars don't inherit them; set theirs with `ANFRA_CANAL_QUERY_ENV_` or `ANFRA_NODE_ENV_` (above). |
| `OTEL_SDK_DISABLED` | `true` exports nothing, whatever else is set. |
| `H_OTEL_ENABLED`, `H_OTEL_INTERNAL_EXPORTER_URL` | anfra-node's own tracing settings (`1`; a base URL). anfra sets them from the endpoint above, unless they are set already. |

### Profiling

| Variable | Effect |
|---|---|
| `ANFRA_PPROF_ADDR` | Under `anfra serve`, serve Go's profiling endpoints for anfra (`/debug/pprof/`, for `go tool pprof`) at this address, e.g. `localhost:6060`, apart from the server. An address it cannot listen on fails the start. Keep it on loopback: they show anfra's memory, with no authentication. Default: unset, not served. canal-query has its own: `ANFRA_CANAL_QUERY_ENV_PPROF_ADDR`, at another address. |

### Updates

See [designs/release.md](../designs/release.md#updates).

| Variable | Effect |
|---|---|
| `ANFRA_NO_UPDATE_NOTIFIER` | `1`: no update checks or notices. |
| `ANFRA_AUTO_UPDATE` | `1`: install a newer release in the background, instead of only noticing it. |
| `CI` | When set, anfra never starts a background update check. CI systems set it. |

### Publishing and login

See [Publishing and login](publishing.md) for signing in, publishing a repo, and managing
stored credentials. The repo's publish destination is `org.url` in `.anfra/config.yml` (above).

| Variable | Effect |
|---|---|
| `ANFRA_CLOUD_URL` | Overrides the build's default cloud URL for `anfra auth login`. An explicitly empty value disables that default. An HTTPS origin, or HTTP on literal `127.0.0.1` or `[::1]` for local testing. See [DEVELOPMENT.md](../../DEVELOPMENT.md#cloud-login-url) for build-time configuration and URL requirements. |

### Data Apps

| Variable | Effect |
|---|---|
| `TZ` | The time zone a Data App's reader has, as an IANA name such as `Asia/Singapore` (`internal/appserve/reader.go`). Default: the machine's. |

## Development

For working on anfra itself; see [DEVELOPMENT.md](../../DEVELOPMENT.md). `make` reads these from `.env.local` (from `.env.local.example`, which `make setup` copies) and passes them to everything it runs.

**`ANFRA_DEV_REPO`:** the repo `make dev` runs `anfra serve` in, as a path, relative to the anfra checkout or absolute. Required by `make dev`; no default.

**`ANFRA_NODE_BIN`, `ANFRA_CANAL_QUERY_BIN`:** the sidecars for a build from source, as above. Absolute paths, since `anfra serve` runs from inside `ANFRA_DEV_REPO`. Or set `ANFRA_NODE_URL` and `ANFRA_CANAL_QUERY_URL` to dial sidecars already running, so a rebuild doesn't respawn them.

**`ANFRA_APPSERVE_DEV_URL`:** where `make dev`'s Vite dev server serves the Data App frontend from source; `anfra serve`'s pages redirect there. A base URL. Default under `make dev`: `http://127.0.0.1:5173` (the `Makefile`); without it, `anfra serve` serves its built frontend and redirects nothing.

**`ANFRA_SERVE_URL`:** the `anfra serve` that Vite's dev server proxies `/api` and `/appserve` to. A base URL. Default: `http://127.0.0.1:7878` (`web/appserve/vite.config.ts`).

**`ANFRA_BASE_VERSION`:** the release a build from source reports itself after, as `<this>+<commit>`. Read from `manifest.yml` by the `Makefile`, which takes precedence over the environment; not set by hand.
