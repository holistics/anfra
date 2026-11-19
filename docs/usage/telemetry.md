# Telemetry

> **Experimental.** The `anfra telemetry` command, the telemetry settings, the `telemetry` section of `anfra status`, and what a trace records may change in a minor release. The changelog says when they do.

anfra can record what each command does, so you or your agent can look back at it: each command's steps, how long each took, and each query's row count, and, if you [turn inputs on](#what-a-trace-holds), its dataset and data source. The records go where you send them. Nothing is sent to Holistics.

Today anfra records traces. Metrics and logs are planned, sent to the same place. anfra has no dashboards or alerts of its own: the records are OpenTelemetry traces, so any tool that reads OpenTelemetry can show them. If you don't have one, anfra gives you a local telemetry stack to run with Docker.

## The local telemetry stack

The local telemetry stack is two containers that receive a repo's traces and keep them. Each repo has its own, so the stacks of several repos run side by side, and one repo's traces never mix with another's.

| Container | What it does |
|---|---|
| An OpenTelemetry collector | Receives traces from anfra and the processes it starts, over OTLP/HTTP (usually port 4318) or OTLP/gRPC (usually 4317), and writes them to ClickHouse. |
| ClickHouse | A database that keeps the traces, in the table `otel_traces` of the database `anfra_traces`. |

**What it keeps.** Each command's trace is a tree of spans: the command, the op it ran, each call to anfra's sidecars, and the steps of a query. Each span has:

- its name, when it started and how long it took;
- whether it failed, and the error's code;
- what anfra knows about it: the op's name, and for a query its warehouse type and row count;
- a query's SQL, with its values masked.

It keeps nothing you typed, such as filter values, and no answers, unless you [turn them on](#what-a-trace-holds). Query rows are never kept.

**For how long.** Three days. Then ClickHouse deletes them.

**Where.** In a Docker volume of the repo's stack, `anfra-telemetry-<repo id>_clickhouse-data`, not in `~/.anfra`. Deleting `~/.anfra` leaves it behind; the compose file's comments say how to remove it.

## Turn telemetry on

Three steps, in the repo's folder: write the repo's local telemetry stack's compose file, start the stack, and point the repo at it.

```sh
anfra telemetry init
```

This writes the stack's compose file to the repo's folder in anfra's own, `~/.anfra/repos/<repo id>/telemetry/compose.yaml`, for this anfra version. Nothing is written in the repo. It starts nothing and changes no setting. Its hints give the file's path and what to do next.

```sh
docker compose -f ~/.anfra/repos/<repo id>/telemetry/compose.yaml up -d
```

This starts the stack. The first time, Docker downloads its images, which takes a minute. Docker starts the stack again whenever Docker itself starts, for example after a restart.

Then point the repo at it, in the `.env` at the repo's root (create it if it isn't there), with the endpoint line from init's hints:

```sh
# .env
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Every anfra command in the repo from then on is recorded, including the commands an agent runs: anfra reads the file on every command, so there's nothing to set in a shell or an agent's config. The file is yours: `anfra init` git-ignores it, and anfra never reads one that Git tracks ([configs](configs.md#your-settings-env)). A running `anfra serve` reads it when it starts, so restart it.

To check where anfra records, run `anfra status`. Its `telemetry` section shows the state (`off`, `on`, or `failing` when the endpoint doesn't accept connections), the endpoint, and what set it, and [what a trace holds](#what-a-trace-holds), each setting with what set it.

### Ports

The stack listens on this machine only. The first time init runs in a repo, it picks the usual ports, 4318 and 4317 for the collector and 8123 and 9000 for ClickHouse, or the next free ones when another program or another repo's stack holds them. They are written in the compose file, and init keeps them when it runs again, so the endpoint stays the same after an upgrade. Init's hint gives the endpoint with the collector's port.

## What a trace holds

By default, a trace holds what anfra knows (each step's name and timing, whether it failed and its error code, counts such as the rows read), and each query's SQL and error message with their values masked. It holds nothing you typed, no answers and no rows.

Each kind is set by a variable, in the environment or the repo's `.env`:

| Variable | What it records | Values | Default |
|---|---|---|---|
| `ANFRA_TRACE_INPUTS` | What a command asked for: its arguments (`anfra.command.input`), its op's input (`anfra.op.input`), and the names of the dataset, data source and files it ran on | `off`, `raw` | `off` |
| `ANFRA_TRACE_OUTPUTS` | What came back: an op's answer (`anfra.op.output`) without a query's rows, and a result's columns | `off`, `raw` | `off` |
| `ANFRA_TRACE_SQL` | The SQL anfra sends to the warehouse (`db.query.text`). `masked` replaces each literal with `?`. | `off`, `masked`, `raw` | `masked` |
| `ANFRA_TRACE_ERRORS` | An error's message and stack. `masked` replaces quoted strings and numbers with `?`, and leaves out the stack. | `off`, `masked`, `raw` | `masked` |

`ANFRA_TRACE_DATA=off` keeps them all off, whatever each says.

To see what each command asked for, and group traces by dataset and data source, turn inputs on:

```sh
ANFRA_TRACE_INPUTS=raw
```

To also see answers and the SQL as sent, for debugging:

```sh
ANFRA_TRACE_INPUTS=raw ANFRA_TRACE_OUTPUTS=raw ANFRA_TRACE_SQL=raw anfra query …
```

- **A query's rows are never recorded,** whatever you set.
- **SQL you write is an input,** such as a query run with `--lang sql`, so `ANFRA_TRACE_INPUTS` sets it. The SQL in an answer, such as `query`'s and `compile`'s, follows `ANFRA_TRACE_SQL`: the answer is recorded without it unless SQL is `raw`.
- **Masked errors are best effort.** A warehouse can word a value so the masker misses it, such as a table path without quotes. The full message is always in `anfra.log`.
- **Each input, answer and query is cut at 16 KiB,** or at `ANFRA_TRACE_PAYLOAD_LIMIT` bytes, and marked when cut (`anfra.payload.truncated`).
- **`anfra.telemetry.dropped_keys` lists attributes anfra left out of a span,** because it doesn't know whether they hold your data. Their values never leave your machine. A span with it is a bug in anfra, so please report the names it lists.

Inputs, answers and raw SQL can hold your data, such as the values in a filter. Turn them on only when you run the endpoint yourself, as you do the local telemetry stack. anfra warns on every command when the repo's `.env` sets one to `raw` and the endpoint is on another machine, since an agent can write that file too. It also warns when traces go over plain `http://` to a machine outside your own network. This machine, a private address, and a name without a dot, such as the stack's collector on its Docker network, count as your own network. When traces go to another machine, `anfra status` says so if the repo's `.env` set the endpoint, or if any kind of data is recorded `raw`, however it was set. Check that you meant both.

For finer rules, such as masking emails or keeping only the attributes you list, use your collector's `redaction` processor. The local telemetry stack's compose file has a commented-out example, in its collector's configuration.

To keep a setting for the repo, put it in its `.env`. With `anfra serve` running, commands run on the server, so its settings apply: restart it after you change the file.

## Names you can rely on

These span names and attributes keep their names and meanings across releases, so queries, dashboards and alerts built on them keep working. Any other span or attribute may change in any release, noted in the changelog. While telemetry is experimental, these may change too, in a minor release, with the old and new names in the changelog.

| Span | Stable attributes |
|---|---|
| `anfra <command path>`, each command, such as `anfra query` | `anfra.repo.id` |
| `anfra.op.<op>`, each op a command runs, such as `anfra.op.core.query` | `anfra.op`, `error.type`, `anfra.repo.id` |
| `anfra.op.core.query`, in addition | `anfra.query.id`, `anfra.query.kind`, `anfra.dataset`, `anfra.data_source`, `db.system.name`, `db.query.text` |

- **Group spans by `anfra.op`,** not by parsing the span's name.
- **An attribute is there only when it applies:** `anfra.repo.id` in a repo, `error.type` when the op failed, `anfra.dataset` for an AQL query.
- **An attribute a [setting](#what-a-trace-holds) records,** such as `anfra.dataset` or `db.query.text`, is there only when the setting records it.

## Record to a tool you already use

Skip the local telemetry stack, and set the endpoint to an OpenTelemetry collector or tool you already have (Jaeger, Langfuse, Honeycomb), in the repo's `.env`:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=https://otel.example.com:4318
```

Give the base URL of its OTLP/HTTP endpoint. anfra adds `/v1/traces` itself.

The variables work in the environment too, which wins over the file ([configs](configs.md#telemetry)). Use it where an environment is already the way to configure things, such as a container or a service.

anfra sends traces over HTTP only. To reach a backend that takes only gRPC, run a collector between the two: it receives HTTP from anfra and exports gRPC.

### A backend that needs a key

Most hosted backends need an API key or token. Set it in `OTEL_EXPORTER_OTLP_HEADERS`, as `key=value` pairs separated by commas, in the repo's `.env` beside the endpoint:

```sh
OTEL_EXPORTER_OTLP_HEADERS=x-honeycomb-team=YOUR_KEY
```

anfra sends these headers with every export, and passes them to the processes it starts. Your backend's docs give the header's name.

The file is git-ignored, as `.anfra/data_sources.yml` is, which holds your warehouse's credentials. It is plain text that your agents can read. To keep the key out of the repo, set the variable in the environment of whatever starts anfra instead: for an agent, export it before you start the agent. For `anfra serve`, restart the server after you set either.

## In a container

anfra's Docker image can't run Docker, so start the repo's stack on the host. With anfra installed there, run `anfra telemetry init` in the repo, and start the file its hint names. The compose file's `name:` is the stack's network, `anfra-telemetry-<repo id>`.

Then join anfra's container to that network, and point it at the collector there. Inside a container, `localhost` is the container itself. Set the endpoint in the container's environment: the image has no Git, so in a Git repo it can't tell whether Git tracks `.env`, and doesn't read it.

```sh
docker run --network anfra-telemetry-<repo id> -e OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
  -p 7878:7878 -v "$PWD:/repo" ghcr.io/holistics/anfra
```
