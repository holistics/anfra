# Telemetry

Proposal: what anfra's traces, metrics and logs are for, what is built, and what to build next. Open questions are marked in place (`> Open:`).

Telemetry here means the traces, metrics and logs anfra sends to a collector its user runs. Nothing goes to Holistics. Anonymous usage stats, which anfra may send to Holistics, get a doc of their own: who receives the data, who controls it and what it may contain all differ.

## 1. Use cases

### Agent authors, and agents themselves

Most of anfra's callers are coding agents ([philosophy](philosophy.md#make-interfaces-work-for-agents-and-programs-not-only-people)). An agent answers one question with many calls: `anfra show`, `query validate`, `query run`, a fix, another run. Each call is its own trace, so without a session nothing ties them to the question.

- **Replay a session:** list every call made for one question, in order, with each call's input, answer and error code.
- **Check a session ad hoc:** an agent unsure of its answer looks up what it already ran instead of running it again.
- **Measure an agent:** across sessions, how many calls a question takes, how many queries fail validation, how many warehouse queries were wasted. This compares skills, prompts and models.
- **Follow the agent's reasoning:** its intent, findings and changes of approach, beside its calls.

### Local app authors

- **Find why a dashboard or Data App is slow:** split a query's time between compiling (anfra-node), building the SQL (canal-query) and the warehouse, with the dataset, the row count and whether the cache answered.
- **Find which Data App call failed,** and follow it from the browser into the server.

### People running anfra for others

Self-hosters on Docker, and platforms that embed the engine.

- **Watch the server's health:** error rates, latency per op, sidecar restarts, background routines.
- **Place anfra inside their own traces,** each span attributed to their tenant, user or session.

### anfra's developers

- **Debug a failure across processes,** from the CLI through `anfra serve`, anfra-node, canal-query and canal-lake, with one trace id.

## 2. Goals

| Priority | Goal | Done when | Design |
|---|---|---|---|
| **P0** | **An agent's work can be traced.** | Anyone can find every call an agent made for one question, in order, with what each did and how it ended. | [Phase 1](#6-phase-1-agent-tracing), [op spans](#what-an-ops-span-records) |
| **P0** | **A slow or failed query can be explained.** | A trace shows where a query's time went and where it failed. | [Op spans](#what-an-ops-span-records), [one trace across processes](#one-trace-across-processes) |
| **P1** | **Anyone can start tracing quickly.** | A user with Docker gets their traces into a store they can query. | [The local stack](#the-local-telemetry-stack) |
| **P1** | **Every process reports to one place.** | anfra and its sidecars send traces, metrics and logs to one collector, joined by one trace id. | [One endpoint](#one-endpoint-for-every-process), [one trace across processes](#one-trace-across-processes), [metrics and logs](#metrics-and-logs-p1) |
| **P1** | **Agents can explain their reasoning.** | An agent's intent, findings and changes of approach sit beside its calls. | [Phase 1](#6-phase-1-agent-tracing) |
| **P1** | **Platforms see anfra in their own observability.** | A platform's traces include anfra's work, labelled with its own tenants, users and sessions. | [Attribution](#attribution-on-spans-p1) |
| **P2** | **Data Apps are traced from the page.** | A Data App's query can be followed from the browser to the warehouse. | [Data Apps](#data-apps-p2) |

Every goal must also keep anfra safe and cheap for users who don't use telemetry: nothing is exported until the user sets it up, and telemetry never slows or stops a command ([considerations](#considerations)). A part of the design can have a lower priority than its goal: the goal is met by the part's minimum, and the rest comes later.

### Non-goals

- **Storing traces in core.** anfra exports to a collector. It doesn't keep or serve traces.
- **A trace viewer.** Users bring their own: ClickHouse queries, a trace viewer, or an agent-tracing tool that reads OTLP.
- **Anonymous usage stats.** A separate pipeline, with its own design.

## 3. Background

### What is built

Phase 0 is built in anfra, as its [checklist](#implementation-details) marks; the sidecars' parts wait on their releases. Details are in [architecture](architecture.md#logs-and-telemetry), [sidecars](sidecars.md#telemetry-and-logs), [logs](logs.md), [errors](errors.md#steps-context-written-once), [configs](../usage/configs.md#telemetry), [Docker](../usage/docker.md#telemetry) and the [usage page](../usage/observability-and-eval/telemetry.md).

#### How the parts fit

Where the settings come from:

```
.env + environment
        │
        ▼
cmd/anfra/telemetry.go ── reads the endpoint and the data switch
        │
        ▼
internal/telemetry.Setup ── builds the exporter
        │
        └──► sets env for the sidecars (anfra-node, canal-query)
```

The spans one command makes:

```
anfra query                      ← root span  (cmd/anfra/telemetry.go)
└── anfra.op.core.query          ← op span    (shared/apikit)
    ├── query attributes, SQL    ← query observer (internal/telemetry/queries.go)
    └── anfra-node's own spans   ← sidecar, exported by itself
```

`publish` and `pull` have a span per call to the org's server under their root instead, such as `org repos.publish` (`internal/cloudapi`).

Where a span goes:

```
span ──► Spans{} ──► Filter ──► OTLP/HTTP ──► collector ──► ClickHouse
         adds the    drops       (anfra's      (the repo's local stack,
         repo id     undeclared   exporter)     started with Docker)
                     keys, and data
                     when it's off
```

`shared/telemetry` holds the rules the filter applies: the declared keys, each data or metadata.

- **Setup.** `internal/telemetry` installs a tracer provider configured by the standard `OTEL_*` variables, from the environment or the repo's `.env` ([settings](#settings)). Tracing is always on, so log records carry span ids, but spans are exported only when an endpoint is set. A setup that fails prints a warning, and the command runs untraced.
- **Spans.**
  - Every CLI command is the root of its trace, `anfra <command path>`, a refused one included (`traceCommands` in `cmd/anfra/telemetry.go`). It records `process.exit.code`, and, when that isn't 0, `error.type` and an error status, as OpenTelemetry's CLI conventions define an error. `serve`, `console` and shell completion have none: each request, and each command a console runs, is its own trace, and completion runs on every Tab.
  - Each op is a span, `anfra.op.<op>`, such as `anfra.op.core.query` (`shared/apikit/invoke.go`), carrying [what the op did](#what-an-ops-span-records).
  - Under `serve`, each HTTP request has a span carrying `h.request.id`, named by its method and route once routed, as OpenTelemetry's HTTP conventions name it: `POST /api/core.query`, `GET /api/ops`, `POST /mcp`.
  - Each MCP request has a span under its HTTP request's, as OpenTelemetry's MCP conventions name it: `tools/call core.query`, `tools/list`, `initialize`, with `mcp.method.name`, `mcp.session.id` and `mcp.protocol.version`, and on a tool call `gen_ai.tool.name` and `gen_ai.operation.name=execute_tool`. A tool whose result says it failed is `error.type=tool_error`. The op's span goes under it, so a CLI call reads `anfra query` → `anfra.op.core.query`, an API call `POST /api/core.query` → `anfra.op.core.query`, and an MCP call `POST /mcp` → `tools/call core.query` → `anfra.op.core.query`.
  - Each call to a sidecar has a span, and carries the trace into the sidecar.
  - Each call to an org's server, from `publish` and `pull`, has a span such as `org repos.publish`. It doesn't carry the trace, which is the user's.
  - A few named steps: sign-in, the update check, a sidecar's start, the sample's download.
- **The caller.** Every span and log record of a command in a repo carries its repo id ([the caller](#the-caller-on-every-span-and-log-record)).
- **User data.** The user's data is recorded only when its switch is on ([data in spans](#data-in-spans)), and `anfra status` shows whether it is.
- **Across processes.** A CLI call forwarded to `serve` carries its trace, which the server continues on loopback. anfra points anfra-node at the same collector, and canal-query and canal-lake at none unless one's own endpoint is set (`ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT`, `ANFRA_CANAL_LAKE_ENV_OTEL_EXPORTER_OTLP_ENDPOINT`), since the release anfra ships exports over gRPC only, and tells every sidecar it spawns what of the user's data to record. canal-query carries the trace into canal-lake.
- **Logs.** Every record logged with a context carries `trace_id` and `span_id` (`shared/logkit/trace.go`).
- **Errors.** Every failing span carries `error.type`, the error's code, but only a server-scope failure marks it failed. A step's sensitive parameters are redacted from span and log (`shared/appstep`).
- **Query observers.** The engine reports each warehouse query to its host through `Observers.Queries` (`internal/command/observers.go`): its id, data source, dataset, AQL and SQL, the warehouse's query id, then rows read, bytes scanned, a timeline of steps, and the error. The CLI and `serve` write it onto the op's span.
- **The local stack.** `anfra experimental telemetry init` writes a repo's [telemetry stack](#the-local-telemetry-stack) for Docker to run.

### What is missing

1. **Metrics are recorded but never exported.** `shared/routine` records `anfra.routine.*`, but no meter provider is installed.
2. **Logs go only to the local `anfra.log`.**
3. **canal-query's and canal-lake's spans don't reach the collector.** The `anfra-canal` release `manifest.yml` pins exports over gRPC only. canal-query flushes with a cancelled context under the CLI. Their log lines have no trace id ([logs](logs.md#not-yet)).
4. **The sidecars don't apply the data settings.** anfra passes them `H_OTEL_DATA`, but anfra-node records each failed call's message and stack, canal-query records raw errors, stacks, a result's column names and types, and, for MySQL, the full SQL, and canal-lake records raw errors and stacks. Once pointed at the user's collector, all of it goes there.
5. **Nothing ties one agent's calls together.** No session id exists.
6. **The SDK sends no `traceparent`.** A Data App's query starts a new trace at `serve`.

## 4. Phases

| Phase | What it delivers | Goals it meets | Done when |
|---|---|---|---|
| **0. Generic tracing** | Each command's trace, from the CLI or `serve` through anfra-node, canal-query and canal-lake, saying what each op and query did, with payloads when asked for, a local stack, and a sample that reads its traces with anfra. Nothing agent-specific. | A slow or failed query can be explained (P0). Anyone can start tracing quickly (P1). | A user runs `anfra experimental telemetry init`, then a query, and finds its trace in ClickHouse with its warehouse, rows and step times, and, with data on, its dataset. |
| **1. Agent tracing** | Every call an agent makes, grouped by the agent's session, nested in the agent's own trace. | An agent's work can be traced (P0). Agents can explain their reasoning (P1). | A coding agent runs several commands with nothing set, and one query by `gen_ai.conversation.id` returns all of them, in order. |

Phase 1 needs phase 0: a session id is worth adding only once spans say something, and it reaches them through phase 0's span processor. The rest follows by priority. A **Decide** item blocks the steps after it. Paths are where a change is expected, not a commitment.

## 5. Phase 0: generic tracing

Every command's trace says what it did, with or without an agent.

### Design

Six questions: what the feature is called, what a trace records, what user data may leave the machine, where the settings live, how a user gets a stack to send traces to, and how they read the traces in it.

#### Choosing the term

The aim is traces, metrics and logs in one place, so the command, setting, status section and stack are named for all three. "Tracing" names one signal. "Monitoring" suggests dashboards and alerts anfra doesn't have. "Observability" names a property, and "otel" is clear only to those who know OpenTelemetry. In a CLI, telemetry can also mean data sent to the maker, so every user-facing line about it says nothing goes to Holistics. `ANFRA_EXPERIMENTAL_TRACE_*` keeps "trace", since it decides what goes into a span.

#### What a trace records

##### What an op's span records

`apikit` starts a span around every op (`shared/apikit/invoke.go`), so none skips tracing. It sets what every op has. An op with more to say adds it from its own code with `apptracing.SetAttributes`, which skips empty values, as OpenTelemetry's guidance does: the framework sets what it knows, and the handler adds the rest.

An attribute is short, from a fixed set where it can be, and holds nothing from the warehouse or the user's filters. User data goes only in attributes a [data setting](#data-in-spans) covers. Rows are never recorded.

| Op | Attribute | Value | Set by |
|---|---|---|---|
| Every op | `anfra.op` | The op's name, such as `core.query` | `apikit` |
| | `error.type` | The error code, on failure | `apikit` |
| | [The caller](#the-caller-on-every-span-and-log-record) | Such as `anfra.repo.id`, or phase 1's `gen_ai.conversation.id` | The span processor |
| `query.*` | `anfra.query.id` | The query run's id, a UUIDv7 | The query observer |
| | `anfra.query.kind` | `aql` or `sql` | The query observer |
| | `anfra.dataset` | The AQL dataset, unset for SQL | The query observer |
| | `anfra.data_source` | The data source's name | The query observer |
| | `db.system.name` | The warehouse's type, such as `postgresql` | The query observer |
| | `anfra.warehouse.query_id` | The warehouse's own query id, if any | The query observer |
| | `db.response.returned_rows` | Rows read from the warehouse | The query observer |
| | `anfra.query.bytes_scanned` | Where the warehouse reports it | The query observer |
| `show`, `validate` | `anfra.target` | The dataset, or files, asked about | The op |
| | `anfra.problem_count` | For `validate`, the problems found | The op |

`anfra.dataset`, `anfra.data_source` and `anfra.target` say what the command asked for, so they are inputs, recorded only when inputs are ([data in spans](#data-in-spans)).

CLI-only commands such as `publish` aren't ops: they have the root span, their org calls, and the op of any core command they run. Column count and cache hits come with a result cache.

The CLI and `serve` set a query observer that writes what it hears onto the current span: the `query.*` attributes as they become known, a failure's code as `error.type`, and each `Timings` step as a span event, so compiling, canal-query's steps and the warehouse show in order. A platform that sets its own observer gets these only if it chains core's.

Naming rules, so a new attribute needs no discussion:

- **OpenTelemetry's name where one fits:** `db.system.name`, `db.response.returned_rows`, `db.query.text` from its [database conventions](https://opentelemetry.io/docs/specs/semconv/database/database-spans/), so database views work without setup.
- **`anfra.` otherwise,** then thing, then field, lowercase, `_` inside a word: `anfra.query.bytes_scanned`.
- **No units.** Counts are integers. Times aren't attributes: they are span durations and the gaps between events.

> Open: whether the query observer allows more than one listener, so a platform can keep core's and add its own.

##### The caller on every span and log record

Spans and logs that say only what a command did, not where or for whom (which repo, which agent session, which platform tenant and user), can't tell two repos' traces apart in one backend, or find everything one caller did.

The host puts the invocation's `Attribution` in the command's context once. A span processor and a log handler copy its fields onto every span and log record made in that context, so no code has to remember them.

- **Every span is labelled, whoever starts it,** anfra or a library. A span already running when the caller becomes known, such as the root, is labelled then.
- **Every log record is labelled** with the same fields.
- **It is core's,** so a platform embedding the engine gets the same with its own fields.

With `Attribution` `{"anfra.repo.id": "shop-1a2b3c4d", "gen_ai.conversation.id": "s-1"}`:

| What | Labelled with |
|---|---|
| Every span | `anfra.repo.id=shop-1a2b3c4d`, `gen_ai.conversation.id=s-1`, `session.id=s-1` |
| Every log record | `anfra_repo_id=shop-1a2b3c4d`, `gen_ai_conversation_id=s-1` |

Keys are span attribute names, dotted as OpenTelemetry's are, since agent-tracing platforms match them exactly. Log keys are snake_case ([logs](logs.md)). Empty values are left out.

**Who fills it.** The CLI and `serve` fill `anfra.repo.id`, the [repo id](architecture.md#state-and-config) anfra names the repo's state folder by: its folder name and a hash of its path, such as `shop-1a2b3c4d`. So traces from several repos in one backend can be told apart, two checkouts of the same repo included, and a span matches the repo's logs and state folder. The folder name is recorded by default, as anfra's own name for the repo, not as an input.

The path itself isn't recorded, since it often holds the user's name, but the hash doesn't hide it from someone who guesses paths. Containers that mount their repo at the same path share a repo id, so a deployment names itself with `OTEL_SERVICE_NAME` or `OTEL_RESOURCE_ATTRIBUTES`. A command outside a repo, such as `anfra version`, has none. Phase 1 adds the agent's session, and a platform its tenant, user and request id ([attribution on spans](#attribution-on-spans-p1)).

**Where.** The CLI fills it when the command starts, before the root span is labelled, so a CLI-only command and the spans before the op carry it too. `serve` fills it per request, since it learns the caller from each call. The sidecars' spans don't carry it: they come from other processes, and join the command's trace by its trace id.

##### One trace across processes

A command's trace runs through every process it reaches. anfra carries the W3C `traceparent` into each sidecar call, canal-query carries it into its calls to canal-lake, and the CLI carries it to `serve` when it forwards a command, which `serve` continues on loopback. Every process exports to the same collector over OTLP/HTTP: every backend accepts it, some agent-tracing tools accept nothing else, and it is OpenTelemetry's default.

canal-query exports over HTTP when `OTEL_EXPORTER_OTLP_PROTOCOL` (or a signal's own) is `http/protobuf`, and over gRPC otherwise. anfra sets its endpoint and protocol through canal-query's prefix (`ANFRA_CANAL_QUERY_ENV_`), so anfra's own `OTEL_*` variables still say only what the user set. canal-query exports traces only, as anfra does, and an endpoint the user set with the prefix wins. It flushes on the way out within 2 seconds, under the 3 seconds anfra waits for it to stop. An older canal-query exports nothing under the CLI, silently, so anfra needs no version check.

canal-lake is canal's too, built from the same tracing code (`h_otel`) and released with canal-query in `anfra-canal`, so it exports the same way. anfra sets its endpoint and protocol through its own prefix (`ANFRA_CANAL_LAKE_ENV_`), as it does canal-query's. canal-lake runs only while the query cache needs it ([query cache](query-cache.md#canal-lake-as-a-sidecar)).

**A backend that takes only gRPC** is reached through a collector, which receives HTTP and exports gRPC. The local stack is one, and the usage page shows the config. With `OTEL_EXPORTER_OTLP_PROTOCOL=grpc`, anfra warns once and exports over HTTP. gRPC from anfra waits ([later](#implementation-details-1)).

#### What user data may leave the machine

##### Payloads

Payloads are the user's data a command carries, recorded only when data is on ([data in spans](#data-in-spans)). Each is cut at `ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT` bytes (default 16 KiB), with the cut marked.

**What the command asked for:**

- **A command's arguments:** the span event `anfra.command.input` on the root span, each flag and argument as given. This covers CLI-only commands and runs refused before they started, such as a mistyped flag, often what an agent got wrong. Stdin and prompt answers aren't recorded, only that stdin was used. Pass no secret as a flag: its value is recorded as given.
- **An op's input:** `anfra.op.input` on the op's span, as JSON. It repeats the arguments in the op's shape, which is simpler than a rule for skipping one. No input holds a credential ([security](security.md)).
- **The SQL anfra sends to the warehouse:** `db.query.text` on the query's span. It is built from the command's input and holds its values as literals. It is the first thing anyone debugging a query asks for, and it matches the trace to the warehouse's query history.

**What came back:**

- **An op's answer:** `anfra.op.output` on the op's span. A CLI-only command's answer isn't recorded, nor is a streamed one.
- **Rows are never recorded,** only the columns and row count.

**Each process decides by its own settings which payloads it records.** The CLI records the arguments, and the process that runs the op records the rest. So if `serve` runs the op, `serve`'s settings apply, not the CLI's.

##### Data in spans

Every attribute that can hold user data is declared **data**, and one switch, `ANFRA_EXPERIMENTAL_TRACE_DATA`, says whether spans record it, in anfra and each sidecar alike: `1`, the default, records it as it is, and `0` leaves it out. Data is what a command asked for (its arguments, its op's input, the datasets, data sources and files it ran on), the SQL anfra sends to the warehouse, what came back (an op's answer without its rows, a result's columns), and a failure's message and stack. Everything else is anfra's own and always recorded: span names, counts, sizes, timings, whether each call succeeded, its error code and step, and ids such as the repo id (`anfra.repo.id`). The sidecars' span names name steps (`acquire_pool`, `rpc.compile`), not data, so they are shown, but they aren't an interface and can change without notice.

- **On by default.** Most use cases need it: replaying an agent's session needs each call's input and answer, and explaining a slow or failed query needs its dataset, SQL and message. With data off, a trace still shows which step was slow, which failed and with what error code, and how many rows a query read. The risk is data leaving the machine, so the CLI warns on every command while data is on by default and the endpoint isn't loopback; setting the switch either way is the user's choice, and stops it. A host that embeds the engine and passes no settings gets data off.
- **One switch, not one per kind of data.** Each use case wants all of it or none: replaying an agent's session needs its inputs and answers, and explaining a failure needs the input, the SQL and the error, whose message often repeats the input's values. Finer switches can be added beside this one later without breaking anyone.
- **No masked level.** Masking SQL's literals or an error's quoted values keeps a trace's shape, but it is best effort: each warehouse words its errors its own way, and a value the masker misses leaks. Two values are simpler to set, check and trust.
- **Baggage is the caller's own.** anfra-node copies each W3C baggage entry onto its spans as given, and the switch doesn't change that: the caller chose to put it in its trace.
- **Whatever the switch says, the full error message stays in `anfra.log`,** in a record carrying the span's `trace_id`. The log is the operator's local record; the trace is what leaves the machine.

**Is a new attribute data?** It is when it's text from the user, their repo, their warehouse, their org or their machine. Numbers, anfra's own fixed values and opaque ids aren't.

**Mistakes that would leak data, and what stops each.** Read this before adding an attribute, an event, a span or a sidecar's tracing. Each guard fails the safe way where it can, so a slip loses some detail from a trace rather than sending the user's data.

- **Declaring a key metadata when it can hold data.** Metadata is recorded whatever the settings, so nothing downstream catches this one. `server.address` and `error.message` were metadata once: one names the org, the other holds an error's text. Declare a key `meta` only when no value it can hold comes from the user's data, repo, warehouse, org or machine. When unsure, declare it data. A reviewer checks every new `meta(...)` against that rule.
- **Not declaring a key.** The span processor (`telemetry.Filter`) leaves an undeclared attribute or event out of the span, and names it in `anfra.telemetry.dropped_keys` so the trace shows it went missing. The registry test fails on it too, but only on the paths it runs. Don't rely on the test alone: a library upgrade can add a key on a path no test runs.
- **Text that is never filtered.** The filter doesn't change a span's name or the resource. Name a span for its step or route, never for a value: not a dataset, a file or a URL's path. The resource describes the process and machine, as the "Always recorded" list in [implementation details](#implementation-details) says. A span's error status keeps its text only when that text is a public error code (`apperr.IsPublicName`) or `panic`. Any other status text is a message, kept only when errors are on.
- **A host that doesn't pass the settings.** An op's spans follow `CommandContext.TraceData`. A host that leaves it empty gets data off: the CLI and `serve` resolve the switch, an embedding platform chooses its own.
- **A sidecar's spans.** The sidecars export straight to the collector, so anfra's filter never sees their spans. Each applies `H_OTEL_DATA` itself. `TestNoDataLeavesInTraces` (`internal/app`, run by `make test-sidecars`) runs queries through anfra and the real sidecars with a canary value in a filter, a SQL literal, a result and a warehouse error. It fails if the canary reaches any exported string with data off, and checks that it does reach one with data on. Run it after changing what anfra records, and before pinning a new sidecar release.

**Why `EXPERIMENTAL` in the names.** The settings may change while telemetry is experimental, and the name says so where they are set, in a `.env` or a deployment's environment, as `anfra experimental telemetry` does for the command. Once telemetry is stable, they lose `EXPERIMENTAL`, and the old names are read for a release.

**How the sidecars get it.** anfra resolves the switch and sets the result, `1` or `0`, in its own environment as `H_OTEL_DATA`, as it does the sidecars' endpoint, so each sidecar it spawns inherits it and applies it as given, without repeating anfra's precedence. A sidecar without it records as it always has, so a platform's sidecars are unchanged, and a deployment that runs the sidecars itself (`ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL`, which brings its own canal-lake) sets it there. No sidecar records SQL: canal-query turns off `otelsql`'s statement text, so the SQL is in a trace once, on anfra's query span.

**If users want finer rules than one switch, they can set them in their own collector.** anfra has no setting that drops an attribute by name or masks values matching a pattern, such as emails. Attribute names change with each sidecar release, so a list of names to drop would stop matching without saying so, and a renamed attribute would leak. The OpenTelemetry Collector's `redaction` processor fails the safe way: it keeps only the keys on its list, so a new one is dropped, and it masks values matching a pattern. The collector config in the local stack's compose file carries a commented-out example.

#### Settings

##### Setting storages

Telemetry's settings live in two layers, and the first one set wins: the environment, then the repo's `.env`. Both take the same names: OpenTelemetry's `OTEL_*` variables and anfra's `ANFRA_EXPERIMENTAL_TRACE_*`, so there is one set of names to learn and document.

| | Environment variables | `.env` |
|---|---|---|
| **Who it reaches** | The process and its children. Not a new shell, nor an agent started from an IDE or desktop app. | Every anfra command run in the repo, agents included, and the repo's `anfra serve`. |
| **Under `anfra serve`** | Only the server's environment counts: a variable in front of a forwarded command does nothing. | Read when the server starts. A change needs a restart. |
| **In a container, service or CI** | The usual way: `docker run -e`, a unit file, a manifest. | Comes with the mounted repo, if one is written there. |
| **Who can change it** | Whoever starts the process. An agent can't change its next commands' environment. | Whoever can write the repo, agents included. |
| **Seeing what is set** | Spread over shells, profiles and unit files. | One file per repo. |

**The file is for a person's own machine; variables are for a deployment.** On a laptop, an agent runs anfra in its own shell. An endpoint the user exports in their terminal doesn't reach that shell, unless they exported it before starting the agent. Every command run in the repo reads `.env`, so the agent's commands are [traced](#agent-authors-and-agents-themselves) too. A container or service sets variables, as usual. A platform that embeds the engine sets up its own tracing and reads neither.

**Per repo, not per machine.** Each repo has its own settings and its own [stack](#the-local-telemetry-stack), so payloads can be on in the sample repo and off in a repo on a production warehouse, and one repo's traces don't mix with another's. The cost is setting up each repo, which `anfra experimental telemetry init`'s hints walk through.

**`.env`, not `.anfra/config.local.yml`.** Both are a person's own and git-ignored, but `config.local.yml` holds a person's values for the repo's settings, and telemetry's settings belong to the person and their processes, not to the repo:

- **OpenTelemetry is set by variables.** anfra's exporter and each sidecar's read the `OTEL_*` variables themselves, and the sidecars inherit anfra's environment. A line in `.env` is that variable, for every process. In YAML, anfra would need a key for each variable, and code to turn it back into the variable for each process, and a variable it didn't map couldn't be set.
- **`config.local.yml` takes `config.yml`'s keys.** Telemetry keys there would either be valid in the committed `config.yml`, so a cloned repo could choose where its traces go, or make the local file take keys the committed one refuses.
- **The endpoint's headers are credentials,** such as a hosted backend's key, and `config.yml`'s rules keep credentials out of it.
- **Telemetry starts before the command runs,** so its settings are read first. `.env` is loaded when anfra starts, before the repo's settings files.

The cost is a second personal file, checked less strictly than YAML: a value of anfra's own is checked, but a mistyped name is ignored.

**The file:**

- **The repo's root `.env`,** beside `.anfra/`: the place users and agents look first. The folder may also be an app's, whose tools keep their own settings in the same file, often credentials.
- **Git-ignored and each person's.** `anfra init` adds `.env` to `.gitignore`, as most templates already do, so a teammate's endpoint never reaches someone who doesn't run that stack. `publish` never sends it, since it sends only AML and Data Apps.
- **A tracked file is never read.** Otherwise cloning a repo would let its author choose where its traces, and their payloads, go. Nor is one in a Git work tree when Git can't say, as in anfra's Docker image, which has no Git. anfra warns when a tracked `.env` sets an `ANFRA_*` or `OTEL_*` name, and says nothing about one that sets only other tools' names.
- **dotenv's format:** `NAME=value` lines, `#` comments, optional quotes, no expansion.
- **Only `ANFRA_*` and `OTEL_*` names are read,** but for the sidecar prefixes `ANFRA_NODE_ENV_*`, `ANFRA_CANAL_QUERY_ENV_*` and `ANFRA_CANAL_LAKE_ENV_*`. The other names belong to the folder's other tools: read, an app's `PGPASSWORD` or `AWS_ACCESS_KEY_ID` would reach canal-query, which inherits anfra's environment, and change the account it queries the warehouse as, and `PATH` would choose the `git` anfra runs. canal-query also loads the `.env` of its working directory itself, which is usually the repo, so the file's other names still reach it until it runs from a folder of its own ([sidecars](sidecars.md#environment)). The sidecar prefixes set any name in a sidecar, `NODE_OPTIONS` included, so they would let whoever writes the file, such as an agent allowed to edit the repo but not run commands, run code in it; they stay in the environment. anfra loads the file when it starts, before it reads any setting, so a name read applies as if it were in the environment. The repo is the current directory, so it is known that early. A bad value for one of anfra's switches is refused, naming the file and line.
- **Data sent to a remote endpoint is warned about.** Agents write in the repo without asking, so one misled by a doc or a query result could edit the file to send the user's data elsewhere. Every command warns when data is on by default or from `.env` and the endpoint isn't loopback, and when the endpoint is `http://` to a host outside the user's own network. Loopback, a private address, and a name without a dot, such as the stack's collector on its Docker network, count as the user's own network. `anfra status` also hints when traces go to another machine and either `.env` set the endpoint, since even the defaults send each step's name and timing there, or data is on, whatever set it. These are hints, not warnings on every command, because a hosted backend is a setup this design recommends, and a deployment that sets the switch chose it.
- **The environment wins name by name:** `ANFRA_EXPERIMENTAL_TRACE_DATA=1` in a server's environment leaves the file's `ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT` as it is.

The stack's ports are in its compose file.

**`anfra status` shows what applies,** the server's settings under `serve`:

```yaml
telemetry:
  state: on                   # off | on | failing
  endpoint: http://localhost:4318
  set_by: OTEL_EXPORTER_OTLP_ENDPOINT in .env
  data: {on: true, set_by: ANFRA_EXPERIMENTAL_TRACE_DATA in .env}
  payload_limit: {value: 16384, set_by: default}
```

- **`state`** names the outcome: `off`, `on`, or `failing` when the endpoint refuses connections, with a hint and the log file.
- **`set_by`** names the winning layer, or the rule that forced the value. It also names `OTEL_SDK_DISABLED`, whose `off` would otherwise look like no endpoint.
- The section is experimental, as telemetry is.

##### One endpoint for every process

The user sets the endpoint once per repo, and anfra passes it to every process it starts, so every process's spans reach one collector. The first one set wins:

1. **The standard `OTEL_*` variables,** for a container, a service or a one-off run.
2. **`OTEL_EXPORTER_OTLP_ENDPOINT` in the repo's `.env`,** read on every command in the repo, so an edit applies to the next.

Without either, nothing is exported. A command outside a repo reads only the environment. `anfra status` shows the endpoint and its source. `anfra version` doesn't, since agents run it to check their skills. A running `serve` needs a restart to pick up a change.

##### Authentication to the user's endpoint

The local stack needs no credentials: its ports listen on 127.0.0.1. A hosted backend or a team's collector usually does, as request headers.

**The user sets them in `OTEL_EXPORTER_OTLP_HEADERS`,** in the environment or the repo's `.env`. It reaches every process: anfra's exporter reads it, and canal-query and anfra-node inherit it (`internal/sidecar/env.go`), whose exporters read it too. In the environment it doesn't reach an agent started without it; in `.env` it does. That file is git-ignored, like `data_sources.yml`, which already holds the warehouse's credentials.

#### The local telemetry stack

A collector and a store, nothing more, one per repo: anfra ships it and the user runs it. Users know it as the **telemetry stack**. Viewing traces is a [non-goal](#non-goals): ClickHouse is a data source like any other, and the [trace sample](#querying-traces-through-anfra) gives a model and AQL questions over its spans, so no separate viewer is needed ([philosophy](philosophy.md)).

**Three steps, from the [usage page](../usage/observability-and-eval/telemetry.md)** or the hints `anfra experimental telemetry init` answers with:

```sh
anfra experimental telemetry init    # write this repo's telemetry stack's compose file, for this anfra version
docker compose -f ~/.anfra/repos/<repo id>/telemetry/compose.yaml up -d    # start it: init's hint gives the path
```

```sh
# .env, at the repo's root: where this repo's traces go (init's hint gives the port)
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

```sh
anfra status    # telemetry: {state: on, endpoint: http://localhost:4318, set_by: OTEL_EXPORTER_OTLP_ENDPOINT in .env}
```

- **The compose file is embedded in the binary,** so it matches the anfra that writes it, works offline, and nothing lands in the repo ([philosophy](philosophy.md#dont-bloat-the-users-machine-or-repo)).
- **`anfra experimental telemetry init` writes it for the current repo,** into the repo's state folder, `<ANFRA_HOME>/repos/<repo id>/telemetry/`, so nothing generated lands in the repo. Outside a repo it refuses, as other repo commands do. It starts nothing and saves no endpoint. Its result is structured ([CLI output](commands-and-api.md#cli-output)): a `message`, the file as `created`, `updated` or `unchanged`, and `hints` with the next steps. A `compose.yaml` there that anfra didn't write is refused. Printing the file would make it an artifact command, which anfra keeps to a few.
- **One stack per repo.** Its compose project and network are `anfra-telemetry-<repo id>`, with a volume of its own, so stacks of several repos run side by side. `init` picks free ports the first time and keeps them when run again, so the endpoint and a data source on the store survive an upgrade. The ports are written in the compose file, and the hint gives the endpoint line with them.
- **It doesn't set the endpoint:** set before the stack runs, it would make every command wait out the export timeout.
- **Docker runs the stack's life.** `restart: unless-stopped` brings it back with Docker. The file's comments say how to stop it (`stop`), delete it and its traces (`down -v`, also before deleting the repo or `~/.anfra`, since the volume outlives both), and upgrade it (`anfra experimental telemetry init` again, then `up -d`).
- **Each stack costs memory,** a collector and a ClickHouse. A user with many repos runs the stacks of the repos being traced, and stops the rest. Why anfra doesn't manage this: [managing the stack](#who-runs-the-stack).
- **Users with a tracing tool already** skip the stack and set the endpoint to it.

| Container | What it does |
|---|---|
| OpenTelemetry Collector (contrib) | Receives OTLP over HTTP (4318) and gRPC (4317), and writes to ClickHouse with the `clickhouse` exporter, which creates `otel_traces` and the other tables. |
| ClickHouse | Keeps spans for 72 hours, and answers anfra's queries as the data source `anfra_traces`. Its system logs are off, since nothing bounds them. |

Every port listens on 127.0.0.1. The collector stays because ClickHouse doesn't receive OTLP, and because it is where a gRPC-only backend is bridged and, in P1, where `filelog` reads `anfra.log`. Its config is a base with an exporter config over it (`clickhouse.yaml`, or `debug.yaml` to print what arrives), each inline in the compose file, so that one file is the whole stack. Each stack's containers share its own Docker network.

| Where anfra runs | What the user does |
|---|---|
| The CLI, or an agent, on the user's machine | Start the repo's stack, and set the endpoint in the repo's `.env` |
| `anfra serve` on the user's machine | The same, then restart the server |
| anfra's Docker image | Run `telemetry init` in the image with the repo and an `ANFRA_HOME` mounted, start the compose file its hint names, and join the stack's network: `--network anfra-telemetry-<repo id> -e OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318`, since `localhost` is the container itself. |
| A tracing tool the user already has | Its address as the endpoint |
| A platform embedding the engine | Not this: it installs its own tracer provider. |

Other stacks: an all-in-one image runs more services and queries each signal in its own language, which anfra can't model. A trace viewer's image only shows traces. A collector built with `ocb` would be smaller than contrib, but anfra would have to publish it.

##### Who runs the stack

**The user runs it with Docker, and anfra only writes it,** to keep the effort small: one command, `anfra experimental telemetry init`, and a compose file. The cost is two steps for the user, and a stack that upgrades only when they run `init` again.

**If anfra manages the stack later,** with `start`, `stop` and `open` beside `init`, it has to answer:

- **Starting:** whether a command that finds the stack down starts it.
- **Stopping:** stopping keeps the traces, and only a command that says "delete" removes the volume.
- **Upgrading:** an old collector keeps running after an anfra upgrade, and the exporter never changes tables that exist.
- **Ports:** a stack anfra starts must find its ports still free each time, and with a remote `DOCKER_HOST` they open on another machine.
- **Storage:** traces live in a Docker volume, bounded by time, not size.
- **The endpoint:** if anfra saves it, a stopped stack slows every command by the export timeout.

It is worth it if users give up between the steps, the store needs anfra to run (a table migration), or phase 1 makes the stack the default way to read a session.

#### Querying traces through anfra

A data source on the stack's ClickHouse and AML models over `otel_traces` let an agent measure its own sessions with the semantic layer. They ship as a sample, as the e-commerce one does ([init sample](init-sample.md)), needing nothing from core.

- **A sample repo,** `holistics/anfra-sample-traces`: the `anfra_traces` data source, the `anfra_spans` model and dataset, a Data App (recent commands, one trace as a timeline, slow queries by dataset, errors by code), and a wiki for agents on the fields.
- **`anfra init <dir> --example traces`** adds it, through the e-commerce sample's download and allow-list. `anfra experimental telemetry init`'s last hint names it.
- **A sample, not a Data App each agent builds:** each agent would otherwise re-derive the attribute names from the raw table and meet the traps a tested model avoids (`@(last 1 day)` is yesterday; an error code is on the op's span, not the command's).
- **Its CI** starts the store and records a few commands before checking the app's queries.

The sample's first content, checked against a real store (the model validates, and each question returns rows), kept here until the sample ships:

<details>
<summary>The data source, model, dataset and questions, as the usage page had them</summary>

Read them the way you read any data with anfra: add the store as a data source, model its spans in AML, and ask with AQL, in whichever repo you choose. An agent can do each step.

##### Add the store as a data source

In `.anfra/data_sources.yml`, under `data_sources:`, with the ClickHouse port from the comments in the stack's compose file:

```yaml
  anfra_traces:
    type: clickhouse
    connection:
      host: localhost
      port: 9000    # this stack's ClickHouse port
      dbname: anfra_traces
      user: anfra
      password: anfra
```

`anfra publish` never sends `anfra_traces` to an org, because the store runs on this machine only. The model and dataset below are published like any others.

##### Model the spans

Every span is a row of the store's `otel_traces` table. This model makes each one a row with fields: what the span is, which command it belongs to, how long it took, and what anfra recorded on it. Save it as `models/anfra_traces/anfra_spans.model.aml`:

```aml
// Each span anfra and its sidecars recorded: one row of otel_traces.
Model anfra_spans {
  type: 'table'
  label: 'anfra spans'
  description: 'Each step anfra and its sidecars recorded: a command, an op, a call to a sidecar, a query.'
  data_source_name: 'anfra_traces'
  table_name: 'otel_traces'

  dimension span_id {
    label: 'Span Id'
    type: 'text'
    primary_key: true
    definition: @sql {{ #SOURCE.SpanId }};;
  }
  dimension trace_id {
    label: 'Trace Id'
    type: 'text'
    description: 'The command the span belongs to: every span of one command shares it.'
    definition: @sql {{ #SOURCE.TraceId }};;
  }
  dimension parent_span_id {
    label: 'Parent Span Id'
    type: 'text'
    hidden: true
    definition: @sql {{ #SOURCE.ParentSpanId }};;
  }
  dimension name {
    label: 'Name'
    type: 'text'
    description: 'What the span is: the command (anfra query), the op (anfra.op.core.query), a sidecar call.'
    definition: @sql {{ #SOURCE.SpanName }};;
  }
  dimension service {
    label: 'Service'
    type: 'text'
    description: 'The process that recorded it: anfra, anfra-node or canal-query.'
    definition: @sql {{ #SOURCE.ServiceName }};;
  }
  dimension started_at {
    label: 'Started At'
    type: 'datetime'
    definition: @sql {{ #SOURCE.Timestamp }};;
  }
  dimension start_time {
    label: 'Start Time'
    type: 'text'
    description: 'When the span started, to the microsecond, to order the spans of one command.'
    definition: @sql formatDateTime({{ #SOURCE.Timestamp }}, '%F %T.%f');;
  }
  dimension duration_ms {
    label: 'Duration (ms)'
    type: 'number'
    definition: @sql round({{ #SOURCE.Duration }} / 1e6, 1);;
  }
  dimension status {
    label: 'Status'
    type: 'text'
    description: 'Ok, Error, or Unset.'
    definition: @sql replaceOne({{ #SOURCE.StatusCode }}, 'STATUS_CODE_', '');;
  }
  dimension is_command {
    label: 'Is Command'
    type: 'truefalse'
    description: 'Whether the span is a whole anfra command, the root of its trace.'
    definition: @sql {{ #SOURCE.ParentSpanId }} = '' AND {{ #SOURCE.ServiceName }} = 'anfra';;
  }
  dimension op {
    label: 'Op'
    type: 'text'
    description: 'The op the span ran, such as core.query. Empty on spans that are not an op.'
    definition: @sql {{ #SOURCE.SpanAttributes }}['anfra.op'];;
  }
  dimension error_code {
    label: 'Error Code'
    type: 'text'
    description: 'The error code, when the span failed.'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['error.type'], '');;
  }
  dimension query_id {
    label: 'Query Id'
    type: 'text'
    description: 'The query run, on the span of an op that ran a query.'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['anfra.query.id'], '');;
  }
  dimension query_kind {
    label: 'Query Kind'
    type: 'text'
    description: 'aql or sql.'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['anfra.query.kind'], '');;
  }
  dimension dataset {
    label: 'Dataset'
    type: 'text'
    description: 'The AQL query dataset.'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['anfra.dataset'], '');;
  }
  dimension data_source {
    label: 'Data Source'
    type: 'text'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['anfra.data_source'], '');;
  }
  dimension warehouse {
    label: 'Warehouse'
    type: 'text'
    description: 'The data source type, such as postgresql or duckdb.'
    definition: @sql nullIf({{ #SOURCE.SpanAttributes }}['db.system.name'], '');;
  }
  dimension returned_rows {
    label: 'Returned Rows'
    type: 'number'
    definition: @sql toInt64OrNull({{ #SOURCE.SpanAttributes }}['db.response.returned_rows']);;
  }

  measure spans {
    label: 'Spans'
    type: 'number'
    definition: @aql count(anfra_spans.span_id);;
  }
  measure avg_ms {
    label: 'Average (ms)'
    type: 'number'
    definition: @aql avg(anfra_spans.duration_ms);;
  }
  measure p95_ms {
    label: '95th percentile (ms)'
    type: 'number'
    definition: @aql round(percentile_cont(anfra_spans.duration_ms, 0.95), 1);;
  }
  measure max_ms {
    label: 'Slowest (ms)'
    type: 'number'
    definition: @aql max(anfra_spans.duration_ms);;
  }
}
```

And a dataset to ask it through, as `datasets/anfra_traces.dataset.aml`:

```aml
Dataset anfra_traces {
  label: 'anfra traces'
  description: 'What each anfra command did, from the telemetry stack: its steps, their timings, the queries it ran and how they ended.'
  data_source_name: 'anfra_traces'
  models: [anfra_spans]
  relationships: []
}
```

Then check both with `anfra validate`.

Each command is one trace: every span of it shares a `trace_id`. Its root span is the command (`anfra query`), and the op's span (`anfra.op.core.query`) holds what the op did, as these attributes, which the model above makes fields:

| Attribute | What it holds |
|---|---|
| `anfra.op` | The op, such as `core.query` |
| `error.type` | The error code, when the op failed |
| `anfra.query.id` | The query run's id |
| `anfra.query.kind` | `aql` or `sql` |
| `anfra.dataset` | The AQL query's dataset |
| `anfra.data_source` | The data source the query ran on |
| `db.system.name` | The data source's type, such as `postgresql` or `duckdb` |
| `anfra.warehouse.query_id` | The warehouse's own id for the query, when it gives one |
| `db.response.returned_rows` | The rows the warehouse returned |
| `anfra.query.bytes_scanned` | The bytes the warehouse scanned, when it reports them |
| `anfra.target` | For `show` and `validate`: the dataset, or the files, asked about |
| `anfra.problem_count` | For `validate`: the problems found |

`anfra.dataset`, `anfra.data_source` and `anfra.target` are filled only when data is recorded, as it is by default.

A query's steps are span events on the op's span, each at the time it started: `compiling_query`, `connecting_to_dw`, `executing_query`, `fetching_query_result`. The model above has one row per span, so they aren't in it.

##### Questions to start from

Run each with `anfra query -d anfra_traces '<query>'`.

The commands of the last 24 hours, latest first:

```aql
explore {
  dimensions {
    started_at: anfra_spans.started_at,
    command: anfra_spans.name,
    trace_id: anfra_spans.trace_id,
    status: anfra_spans.status,
  }
  measures {
    ms: anfra_spans | max(anfra_spans.duration_ms),
  }
  filters {
    anfra_spans.is_command == true,
    anfra_spans.started_at > @(24 hours ago),
  }
  sorts {
    started_at DESC,
  }
}
```

One command's whole trace, step by step: put a `trace_id` from the query above in the filter.

```aql
explore {
  dimensions {
    start_time: anfra_spans.start_time,
    service: anfra_spans.service,
    name: anfra_spans.name,
    error_code: anfra_spans.error_code,
  }
  measures {
    ms: anfra_spans | max(anfra_spans.duration_ms),
  }
  filters {
    anfra_spans.trace_id == '<trace_id>',
  }
  sorts {
    start_time ASC,
  }
}
```

The slowest queries of the last 24 hours, by dataset (a SQL query has no dataset):

```aql
explore {
  dimensions {
    dataset: anfra_spans.dataset,
  }
  measures {
    queries: anfra_spans | count(anfra_spans.query_id),
    p95_ms: anfra_spans.p95_ms,
    max_ms: anfra_spans.max_ms,
  }
  filters {
    anfra_spans.query_id is not null,
    anfra_spans.started_at > @(24 hours ago),
  }
  sorts {
    p95_ms DESC,
  }
}
```

The errors of the last 24 hours, by op and code:

```aql
explore {
  dimensions {
    op: anfra_spans.op,
    error_code: anfra_spans.error_code,
  }
  measures {
    errors: anfra_spans | count(anfra_spans.span_id),
  }
  filters {
    anfra_spans.error_code is not null,
    anfra_spans.started_at > @(24 hours ago),
  }
  sorts {
    errors DESC,
  }
}
```

</details>

> Open: the e-commerce sample is always `main`, so a fix needs no anfra release. Trace attributes come from the binary, so an older anfra could get a `main` naming attributes it doesn't record. Acceptable while telemetry is experimental.

### Considerations

**Telemetry is experimental.** Its command is `anfra experimental telemetry`, and its settings `ANFRA_EXPERIMENTAL_TRACE_*`, so whoever runs or sets one sees it may change in a minor release, noted in the changelog, exempt from [deprecation before removal](commands-and-api.md#deprecation-before-removal) until declared stable. The usage page and configs say the same of what isn't a name: `anfra status`'s section, span names and attributes, and the stack's tables. Leaving the group is a rename: the old command and settings stay, the command hidden, for a release.

**Safe by default.** Nothing is exported until the user sets an endpoint, and data goes only to that collector. The [data switch](#data-in-spans) is on by default, for the use cases that need it, with a warning on every command while traces go to another machine and nobody set it. Rows never reach a span.

**Cost and volume.** A span per op is cheap; payloads aren't, and an agent session can run hundreds of ops. The payload limit bounds each, and `OTEL_TRACES_SAMPLER` works for anyone who needs less. Metric labels come from fixed sets, never a session or user id.

**Short CLI commands.** A command waits up to 2 seconds on exit to flush (`flushTimeout`), so a collector that is down could slow every command by that much. The export timeout stays under it, and a failed export logs once. A stack stopped while `.env` keeps its endpoint makes this likelier: `anfra status` says so, and the failure's log line names where the endpoint came from.

**No hidden state.** `.env` is written by the user, never by anfra, shown by `anfra status`, and loses to the environment. Phase 1 keeps no session between commands, since its boundaries would be guesses nobody sees.

**Core's boundary.** Core records what it does and labels it with what its caller says. It doesn't store, show or analyse traces: the stack and the sample sit outside the engine.

**Stable span names, later.** Users build queries, dashboards and alerts on span names and attributes, and the [trace sample](#querying-traces-through-anfra) models them. While telemetry is experimental every name may change, noted in the changelog. Before it is declared stable, anfra could list the spans users can rely on, each with its stable attributes, and a test could fail when one loses an attribute. The likely first list: each command's root (`anfra <command path>`, with `anfra.repo.id`), each op's span (`anfra.op.<op>`, with `anfra.op`, `error.type`, `anfra.repo.id`), and the query's (`anfra.op.core.query`, with `anfra.query.*`, `anfra.dataset`, `anfra.data_source`, `db.system.name`, `db.query.text`). Group by `anfra.op` rather than by parsing a span's name.

**Semantic conventions.** OpenTelemetry's names where one exists (`gen_ai.conversation.id`, `error.type`, `db.*`), `anfra.` otherwise. Where a GenAI convention covers a field, its name is the only one, with one exception: an agent's session is `gen_ai.conversation.id`, and also `session.id` on spans, because Langfuse and Claude Code's own telemetry group sessions by `session.id` alone. A log record carries it once, as `gen_ai_conversation_id`. The existing `h.step.*` and `h.request.id` stay.

### Implementation details

**Traces** ([what a trace records](#what-a-trace-records))

- [x] **Spans.** Every command is a trace's root (`traceCommands`), refused runs included. Each op sets its [attributes](#what-an-ops-span-records) with `apptracing.SetAttributes`. The query observer is set in `commandContext` (`cmd/anfra/host.go`). Org calls send no `traceparent`. Tests: one root per command, none sent to an org.
- [x] **[The caller](#the-caller-on-every-span-and-log-record).** The span processor and log handler (`shared/telemetry`) copy `Attribution` from their own context value. The CLI fills it per command, `serve` per request, the registry per op (`CallContext`). The processor runs before the batcher. Tests: every span and log record carries `anfra.repo.id`; none outside a repo; no path recorded.
- [x] **Names are data ([what an op's span records](#what-an-ops-span-records)).** `anfra.dataset`, `anfra.data_source` and `anfra.target` reach a span only with data on.
- [x] **Exporting never slows a command** ([short CLI commands](#considerations)). Tested against a collector that is down.
- [x] **A warning for gRPC,** and a collector example for gRPC-only backends ([one trace across processes](#one-trace-across-processes)).

**User data** ([what user data may leave the machine](#what-user-data-may-leave-the-machine))

- [x] **The [settings](#data-in-spans),** read in `internal/envflag` and shown in `anfra status`.
- [ ] **One registry of keys** (`shared/telemetry`), which the [guards against leaks](#data-in-spans) rely on:
  - [x] Each key is declared once, as data or metadata. `SetAttributes`, `RecordPayload` and `RecordError` apply it.
  - [x] `forbidigo` bans raw attribute calls outside `shared/telemetry` and `shared/apptracing`, so no code records a key around the registry.
  - [x] Tests: CLI runs' spans hold no undeclared key.
  - [ ] The registry test also records a query and a `serve` request (`make test-sidecars`).
- [x] **anfra's spans follow the settings:** payloads cut at the limit, the command's arguments ([payloads](#payloads)), the query observer, `apptracing`'s errors. Tests: data on and off; rows never recorded.
- [x] **A slip fails safe** ([mistakes that would leak data](#data-in-spans)), a test for each.
- [x] **The sidecars get `H_OTEL_DATA`** ([how](#data-in-spans)), resolved, even at its default. A sidecar prefix overrides it.
- [x] **`TestNoDataLeavesInTraces`:** no canary leaves with data off; one does with data on.
- [x] **A warning on every command** while data is on by default and the endpoint isn't loopback (`data_by_default`).
- [x] **`anfra status` hints** when data is on or `.env` sets the endpoint ([setting storages](#setting-storages)).
- [x] **A commented-out `redaction` processor** in the stack's collector config ([finer rules](#data-in-spans)).
- [ ] **Library and resource keys,** classified again whenever `manifest.yml` pins a new sidecar, since a sidecar's spans bypass anfra's filter ([mistakes](#data-in-spans)):
  - [x] canal-query `query-v2.24.0` and anfra-node 0.13.5:
    - **Always recorded:** `host.name`, `process.owner`, `process.executable.path`, `h.host_name`, `user_agent.original`, `h.step.sidecar`.
    - **Left out:** `process.command_args`, which `anfra.command.input` covers; `client.address` and `network.peer.address` on `serve`'s requests, a person's IP.
    - **Inputs:** `h.step.origin`, `h.step.org`, org calls' `url.full` and `server.address`; canal-query's `net.peer.name`, `db.user`, BigQuery's `url.full`; anfra-node's `h.fqn`.
    - **Errors:** `error.message`; canal-query's status description, `echo.error`, Athena's `api.error_message`; anfra-node's `h.aml_cache_persist_error`.
  - [ ] anfra-node 0.15.0: `h.git_repo` and `h.git_sha` among them.

**Settings** ([settings](#settings))

- [x] **The repo's `.env`** (`internal/repoenv`), read as [setting storages](#setting-storages) says, before any setting. A bad line of anfra's is refused with file and line. Tests: other names reach no process; `publish` never sends the file.
- [x] **[One endpoint](#one-endpoint-for-every-process):** canal-query gets anfra's endpoint and protocol.
- [x] **`anfra status`** shows `state`, `endpoint` and `set_by` ([setting storages](#setting-storages)).
- [x] **The prompt-injection guards** on `.env` ([setting storages](#setting-storages)).
- [x] **`~/.anfra/config.yml` retired,** with `internal/homeconfig`: `.env` replaces it, since settings are [per repo](#setting-storages).

**The local telemetry stack** ([the stack](#the-local-telemetry-stack))

- [x] **The compose file,** embedded in `internal/telemetry/stack/`.
- [x] **`anfra experimental telemetry init`,** one stack per repo, in `<ANFRA_HOME>/repos/<repo id>/telemetry/`. Ports are picked once and kept.
- [x] **A CI check** that a span reaches `otel_traces` (`TestTheStackRecordsASpan`), since only running the compose file shows that an image upgrade still works.
- [x] **The managed stack removed** ([who runs the stack](#who-runs-the-stack)). Never released.
- [x] **A setup skill** (`skills/plugins/anfra/skills/anfra-telemetry`), so an agent can run the [three steps](#the-local-telemetry-stack) for its user.

**Reading traces** ([the trace sample](#querying-traces-through-anfra))

- [ ] **The sample repo,** `holistics/anfra-sample-traces`:
  - [ ] The data source, model and dataset. The data source finds its stack's port: each repo's stack picks its own, and the sample's committed file can't assume 9000.
  - [ ] Its Data App: recent commands, a trace timeline, slow queries, errors by code.
  - [ ] Its wiki for agents on the fields.
  - [ ] Its CI: record a few commands, then check the app's queries.
- [ ] **Getting it:**
  - [ ] `anfra init <dir> --example traces`.
  - [ ] `anfra experimental telemetry init`'s last hint names it.
- [ ] **Decide** how an agent learns to read traces. Recommended: the setup skill points to the sample, whose wiki teaches the fields.

**Sidecars**

Until anfra-node's items ship, its spans carry raw errors, stacks and `h.fqn`.

- [ ] **canal**, postponed ([what is missing](#what-is-missing)). Ships in or before the `anfra-canal` release that exports over HTTP:
  - [ ] **Export over OTLP/HTTP** (`h_otel`), honour the `OTEL_*` protocol and exporter settings, flush on stop, pin the release.
  - [ ] **Apply the data settings** in `RecordError` and `SetError`. canal-query also gates `h.db_schema_fields`, drops its input keys, and sets `otelsql`'s `DisableQuery`, which otherwise records MySQL's full SQL.
  - [ ] **Pass anfra's endpoint to canal-lake** through `ANFRA_CANAL_LAKE_ENV_`.
  - [ ] **Classify canal-lake's keys,** `otelgrpc`'s on its server spans included.
  - [ ] **`TestNoDataLeavesInTraces` checks canal-query's spans.**
- [ ] **anfra-node** (holistics-core), postponed:
  - [ ] **Apply the data settings** in `h-otel`'s `HSpan`.
  - [ ] **Stop sending `err.stack` to anfra** (`jsonrpc.ts`), which [errors](errors.md#sidecar-failures) forbids.

**Docs**

- [x] **[configs](../usage/configs.md#telemetry),** **[sidecars](sidecars.md#telemetry-and-logs)** and **[Docker](../usage/docker.md#telemetry).**
- [ ] **The [usage page](../usage/observability-and-eval/telemetry.md):**
  - [x] The three steps and "What a trace holds".
  - [ ] A reading section pointing to the trace sample.
- [ ] **[architecture](architecture.md#logs-and-telemetry):**
  - [x] `.env` and the per-repo stack.
  - [ ] `credentials.json` and `agents-check.json` in its `<ANFRA_HOME>/` tree.
  - [ ] canal-query and canal-lake in its traces paragraph, after the canal release.

## 6. Phase 1: agent tracing

> Status: work in progress, 2026-11-17. Still being designed, and not built. Phase 0 comes first. Its design is added here once it settles.

An agent's calls are grouped by its session, so one query finds everything it did for a piece of work, nested in the agent's own trace when it traces itself. anfra finds the session id from what the agent already gives the commands it runs, and puts it in `Attribution`. Phase 0's [span processor and log handler](#the-caller-on-every-span-and-log-record) copy it onto every span and log record.

## 7. Later

The parts after phase 1, by priority, outside any phase.

### Design

#### Metrics and logs (P1)

- **Metrics.** A meter provider, exported only when an endpoint is set. `httpkit` and `apikit` add per-op request counts and latency histograms, labelled by op and error code, never by session.
- **Logs** stay in `anfra.log`, and a collector reads the file with `filelog`: exporting from `slog` would miss crashes, startup and the sidecars. The local stack can mount the log folder.

#### Attribution on spans (P1)

Phase 0's [span processor](#the-caller-on-every-span-and-log-record) copies every `Attribution` field, so a platform's tenant, user and request id reach anfra's spans with no engine change. Left: a test that they arrive. `anfra serve` sets only the session id, having one user.

#### Data Apps (P2)

The SDK creates a session id per page load and sends it as `Anfra-Session-Id`, so a page's queries form one session, and its API client sends a `traceparent` with each request. `anfra serve` trusts a `traceparent` only on loopback (`TrustTraceparent`), where appserve runs; an exposed server keeps it as a link. Page spans, such as a render, would need a browser exporter relayed through `serve`, which stamps the caller it admitted and drops any identity the browser claimed.

### Implementation details

**P1**

- [ ] **Metrics.**
  - **Decide** names and temporality. Recommended: deltas, so a rate is a sum anfra can model, and names after OpenTelemetry's `http.server.request.duration` family. `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` switches to cumulative for Prometheus.
  - A meter provider beside the tracer provider, so `anfra.routine.*` reaches the collector.
  - Request counts and latency histograms in `httpkit` and `apikit`.
- [ ] **Logs.**
  - **Decide** whether logs go through `filelog` only. Recommended: yes, though a remote collector then needs the file mounted.
  - The local stack's collector reads `anfra.log` and parses `trace_id` and `span_id`, so logs join their spans.
- [ ] **Platforms.** A test that a platform's `Attribution` fields reach spans and logs.
- [ ] **Query observer.** **Decide** whether it takes more than one listener, through a fan-out observer.

**P2**

- [ ] **Data Apps.** The SDK's session id and `traceparent`.
- [ ] **The console** creates one UUIDv7 session id at start, for every command it runs.
- [ ] **Page spans,** if needed.
- [ ] **Decide** whether to infer a session id for agents that expose nothing ([phase 1](#6-phase-1-agent-tracing)).

**P3**

- [ ] **gRPC export, if users ask.** anfra honours `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` with `otlptracegrpc`, cheap since `google.golang.org/grpc` is already a dependency, and the warning goes. anfra-node comes last, since `@grpc/grpc-js` adds weight to a sidecar every user downloads.
