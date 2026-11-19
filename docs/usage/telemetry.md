# Telemetry

> **Experimental.** The `anfra experimental telemetry` command, its `ANFRA_EXPERIMENTAL_TRACE_*` settings, the `telemetry` section of `anfra status`, and what a trace records may change in a minor release. The changelog says when they do.

anfra can record what each command does, so you or your agent can look back at it: each command's steps and how long each took, what it asked for and got back, and each query's SQL and row count ([what a trace holds](#what-a-trace-holds)). The records go where you send them. Nothing is sent to Holistics.

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
- your data, unless you [turn it off](#what-a-trace-holds): what the command asked for, the SQL it ran, what came back, and an error's message.

Query rows are never kept.

**For how long.** Three days. Then ClickHouse deletes them.

**Where.** In a Docker volume of the repo's stack, `anfra-telemetry-<repo id>_clickhouse-data`, not in `~/.anfra`. Deleting `~/.anfra` leaves it behind; the compose file's comments say how to remove it.

## Turn telemetry on

Three steps, in the repo's folder: write the repo's local telemetry stack's compose file, start the stack, and point the repo at it.

```sh
anfra experimental telemetry init
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

A trace holds what anfra knows, such as each step's name and timing, whether it failed and its error code, and counts such as the rows read. By default it also holds your data, so you or your agent can see what each command did:

- what each command asked for: its arguments (`anfra.command.input`) and its op's input (`anfra.op.input`), and the names of the dataset, data source and files it ran on;
- the SQL anfra sends to the warehouse (`db.query.text`);
- what came back: an op's answer (`anfra.op.output`) and a result's columns;
- an error's message and stack.

To leave your data out, in the environment or the repo's `.env`:

```sh
ANFRA_EXPERIMENTAL_TRACE_DATA=0
```

- **A query's rows are never recorded,** whatever you set.
- **Your data can include personal data,** such as an email in a filter, a SQL literal or an error message. anfra doesn't mask it.
- **With data off, a failed step keeps its error code,** not its message. The full message is always in `anfra.log`.
- **Each input, answer and query is cut at 16 KiB,** or at `ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT` bytes, and marked when cut (`anfra.payload.truncated`).
- **`anfra.telemetry.dropped_keys` lists attributes anfra left out of a span,** because it doesn't know whether they hold your data. Their values never leave your machine. A span with it is a bug in anfra, so please report the names it lists.

**When traces go to another machine,** anfra warns on every command while data is on by default, so you choose: `0` keeps your data out, `1` sends it and stops the warning. It also warns when the repo's `.env` sets it to `1`, since an agent can write that file too, and when traces go over plain `http://` to a machine outside your own network. This machine, a private address, and a name without a dot, such as the stack's collector on its Docker network, count as your own network. `anfra status` says so too, if the repo's `.env` set the endpoint, or if data is on.

For finer rules, such as masking emails or keeping only the attributes you list, use your collector's `redaction` processor. The local telemetry stack's compose file has a commented-out example, in its collector's configuration.

To keep a setting for the repo, put it in its `.env`. With `anfra serve` running, commands run on the server, so its settings apply: restart it after you change the file.

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

anfra's Docker image can't run Docker, so start the repo's stack on the host. With anfra installed there, run `anfra experimental telemetry init` in the repo, and start the file its hint names. The compose file's `name:` is the stack's network, `anfra-telemetry-<repo id>`.

Then join anfra's container to that network, and point it at the collector there. Inside a container, `localhost` is the container itself. Set the endpoint in the container's environment: the image has no Git, so in a Git repo it can't tell whether Git tracks `.env`, and doesn't read it.

```sh
docker run --network anfra-telemetry-<repo id> -e OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
  -p 7878:7878 -v "$PWD:/repo" ghcr.io/holistics/anfra
```
