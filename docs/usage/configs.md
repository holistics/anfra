# Configuring anfra

How to configure an anfra setup: the repo, `anfra serve`, and the environment.

## The repo: `.anfra/`

A repo's config is its `.anfra/` folder, committed with it (`anfra init` creates it). The name is fixed: it is part of the repo's format, so every clone finds it.

| File | What it is |
|---|---|
| `data_sources.yml` | The data sources the repo's models query, by name: a `type` and a `connection` passed to the database driver unchanged. It holds credentials, so it is git-ignored; `anfra init` also creates `data_sources.yml.example`, the same without them, to commit. Each type's connection keys are listed in the anfra skills' [data source reference](https://github.com/holistics/anfra-skills/blob/main/plugins/anfra/skills/setup-repo/references/data-sources.md). |
| `context_sources.yml` | What `anfra ingest` reads into the local catalog that `anfra search` searches; by default, the repo's own semantic layer. |

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
| `ANFRA_NODE_BIN`, `ANFRA_CANAL_QUERY_BIN` | The sidecar binaries, as absolute paths, for a build that embeds none (a build from source). Releases embed them, and ignore these. |

#### Running anfra-node (the sidecar, not anfra) yourself

A platform that runs **anfra-node** as a service, rather than letting anfra spawn it, configures it with these. They are the sidecar's own settings, read by anfra-node, not by anfra:

| Setting | Effect |
|---|---|
| `--port`, or `ANFRA_PORT` | The port to listen on. One of this or a socket (`--socket`, `ANFRA_SOCKET`) is required. |
| `ANFRA_HOST` | The address to listen on, with `--port` or `ANFRA_PORT`. Default: `0.0.0.0`. |
| `ANFRA_COMPILE_CACHE_PATH` | Where compiled programs are cached on disk. Default: `~/.anfra/compile-cache`. anfra sets it per repo for the anfra-node it spawns. |
| `ANFRA_PROGRAM_MEMORY_SIZE` | How many repos' compiled programs to keep in memory between requests: a whole number. Default: `0`, none. Size it to the repos one process serves and the memory it has; anfra sets `1` for the anfra-node it spawns, which serves one repo. |
| `LOG_LEVEL` | As below. |

### Logs

| Variable | Effect |
|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error`, case-insensitive. Default: `info`, also for any other value. anfra-node inherits it. |
| `ANFRA_LOG_STDERR` | `1`: also copy the log stream (anfra's records and the sidecars' stderr) to stderr. Default: only to `<ANFRA_HOME>/repos/<repo id>/logs/anfra.log`. |
| `ANFRA_SIDECAR_STDOUT` | `1`: pass the sidecars' stdout through. Default: discarded. |

### Errors

| Variable | Effect |
|---|---|
| `ANFRA_HIDE_ERROR_CAUSES` | `1`: hide errors' causes from clients, showing only their message. Default: causes shown, since anfra's user is usually its operator. Set it when `anfra serve` serves other people: a cause can carry SQL, database errors and paths. A command run at the terminal still prints the cause there. |

### Telemetry

anfra exports traces over OTLP/HTTP when an endpoint is set, through the standard OpenTelemetry variables (`internal/telemetry`). Default: no endpoint, nothing exported.

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | The collector's base URL, e.g. `http://localhost:4318`. anfra also points anfra-node at it. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | The traces endpoint alone, a full URL ending in `/v1/traces`, instead of the base URL. |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | Label the traces, in OpenTelemetry's format (`key=value,key=value` for the attributes). Default service name: `anfra`. |
| `OTEL_SDK_DISABLED` | `true` exports nothing, whatever else is set. |
| `H_OTEL_ENABLED`, `H_OTEL_INTERNAL_EXPORTER_URL` | anfra-node's own tracing settings (`1`; a base URL). anfra sets them from the endpoint above, unless they are set already. |

### Updates

See [designs/release.md](../designs/release.md#updates).

| Variable | Effect |
|---|---|
| `ANFRA_NO_UPDATE_NOTIFIER` | `1`: no update checks or notices. |
| `ANFRA_AUTO_UPDATE` | `1`: install a newer release in the background, instead of only noticing it. |
| `ANFRA_GITHUB_TOKEN` (or `GH_TOKEN`, `GITHUB_TOKEN`) | A GitHub token, used only when an anonymous request for releases is refused. The first one set is used. |
| `CI` | When set, anfra never starts a background update check. CI systems set it. |

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
