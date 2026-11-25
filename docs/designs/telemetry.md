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

> Status: work in progress, 2026-11-25. Sessions are built: the agent's id read from its environment, `ANFRA_SESSION_ID`, and `gen_ai.conversation.id` in the caller's baggage. Tasks are built. Requiring a session is still to build.

An agent's calls are grouped into its session, and nest in the agent's own trace.

### Design

#### Sessions

A **session** is the calls an agent, a person or a program makes for one piece of work. For an agent, that's usually one agent session, such as one Claude Code session. In prose, say "agent session", so it isn't confused with an MCP session, an `anfra console` session, or anfra Cloud's sign-in session.

A session is identified by an id. Whoever owns the session creates the id, which is usually the agent. anfra finds the id on each command and puts it on what it records. It doesn't store sessions or know when one ends, and never creates an id itself ([considerations](#considerations-1)).

Within a session, the agent can mark its work on each of the user's messages as a [task](#tasks).

##### The life of a session id

1. **Created**, outside anfra in most cases: by the agent when its session starts, by whoever launches the agent, or by the caller of one command.
2. **Found again on every command.** Nothing is stored between commands. Each command looks for the id in what it receives ([where the id comes from](#where-the-id-comes-from)), and checks it.
3. **Recorded** on every span and log record of the command ([how the id reaches spans and logs](#how-the-id-reaches-spans-and-logs)).
4. **Carried** in the command's baggage, as `gen_ai.conversation.id`, to `serve` when the CLI forwards a command, and to the sidecars.
5. **Never ended.** anfra stops seeing the id when the agent's session ends, after `/clear`, or when a launcher unsets its variable. Each command is its own trace, so nothing is left open.
6. **Read back** from `anfra.log`, from the collector's store, or from the agent's own tracing backend.
7. **Kept** only as long as the store keeps it: 72 hours in the [local stack](#the-local-telemetry-stack), and per `anfra.log`'s rotation.

##### The interface

**Most users set nothing.** anfra detects the session of the agent that runs it, from what the agent already gives the commands it runs. Asking an agent to set an id itself wouldn't work. Claude Code, Codex and Gemini CLI run each command in a new shell, so an `export` in one command doesn't reach the next ([research](#what-coding-agents-expose)). The model could pass an id on every call instead, but one forgotten call drops out of the session without anyone noticing, and after `/clear` or a compaction it may lose the id. The agent's program, unlike its model, never forgets, so anfra reads what the program sets.

anfra keeps a table of the agents it knows (`internal/agentsession`). Each entry says where that agent puts its session id, and what name to record for the agent:

| Agent | Session id, in a command's environment | Session id, in an MCP tool call's `_meta` | Agent name recorded |
|---|---|---|---|
| Claude Code | `CLAUDE_CODE_SESSION_ID` | None | `claude-code` |
| Codex CLI | `CODEX_SESSION_ID` (the root session, shared by its subagents) | `sessionId` | `codex` |
| OpenCode | `OPENCODE_SESSION_ID` | None | `opencode` |
| Goose | `AGENT_SESSION_ID` | `agent-session-id` | `goose` |

The table is data in one place, so adding an agent is one entry. Goose's `AGENT_SESSION_ID` sounds generic, but only Goose sets it, so it is Goose's entry and not a name anfra reads for every agent. Copilot CLI's `COPILOT_AGENT_SESSION_ID` and pi's `PI_SESSION_ID` are candidates, to add once checked in a command they run.

Gemini CLI, Cursor, Cline, Windsurf, Kiro and Continue give their session id only to their own hooks, which can't pass it on to later commands. VS Code's agent mode, Zed, Aider and Amp expose none. Their calls are grouped when whoever launches them sets `ANFRA_SESSION_ID`.

**Who sets what:**

| Situation | What the user does | Where the session id comes from |
|---|---|---|
| A known agent runs `anfra`, or calls its MCP server | Nothing | The agent's own variable or `_meta` key, from the table |
| Another agent | Launches it with `ANFRA_SESSION_ID` set, such as `ANFRA_SESSION_ID=$(uuidgen) amp` | Inherited by everything the agent runs |
| A person at a terminal or in `anfra console` | Nothing | None, so each command stands alone |
| An HTTP API or MCP caller | Sends `gen_ai.conversation.id` in its baggage: the `baggage` header, or `_meta.baggage` on a tool call | The baggage, as the allowlist lets it in ([the caller's baggage](#the-callers-baggage)) |
| A platform embedding the engine | Sets `Attribution["gen_ai.conversation.id"]` | The platform |

##### Where the id comes from

A CLI command runs once per process, so its environment belongs to the caller. It takes the first id it finds:

1. A known agent's variable, in the table's order, with the agent's name.
2. `ANFRA_SESSION_ID`, with no agent's name.
3. `gen_ai.conversation.id` in the caller's `BAGGAGE`, when the allowlist lets it in.
4. None. The command is recorded alone.

A known agent's id comes first because it matches the agent's own telemetry. `ANFRA_SESSION_ID` is the fallback for agents that expose none, so it never hides an agent's id. An eval harness that wants its own run id beside the agent's session names it in the caller's baggage, such as `run.id`, with the allowlist including it.

An agent run by another inherits the outer agent's variable too, so the first match in the table isn't always the innermost agent.

`anfra serve` and its MCP server take an id only from each call: a known agent's own `_meta` key on a tool call, with the agent's name, then `gen_ai.conversation.id` in the call's baggage. They never read their own environment. A server is started once and serves many callers. If it was started inside an agent session, its environment would carry that session's id long after the session ended, and put it on every caller's requests.

The command puts the id it found in its own baggage, as `gen_ai.conversation.id`, when the allowlist lets it in, so a command forwarded to `serve`, and the sidecars it calls, record it too.

The MCP session id (`Mcp-Session-Id`) isn't a source. MCP's spec removed it in 2026 (SEP-2567), and it never matched an agent session anyway: it was one client connection.

Each agent defines its own session, and anfra records it as it is. Claude Code starts a new id on `/clear`. Codex shares one id between a session and its subagents. Some agents keep one id across a resume, and some don't. anfra doesn't smooth these over, because the point is that its id matches the agent's.

**An id is checked when it arrives.** anfra treats it as an opaque string, but refuses one longer than 128 bytes or containing control characters, because the id goes into log records and span attributes. A refused id counts as missing. Langfuse has a similar rule, discarding ids of 200 characters or more.

The id is attribution, so it never changes what a command does. `internal/attribution` and its depguard rule already enforce that. [Requiring a session](#requiring-a-session) is a check the host makes before the command runs, like an access check, so the engine still never reads the id.

**Without a collector.** Each log record carries `gen_ai_conversation_id`, and `anfra.log` already records each request, so a session can be replayed from the local log alone.

> Open: a command that reads a session back from `anfra.log`, such as `anfra log --session <id>`. It reads only the local log, so core still stores no traces. But it is a new command, and it sees only what that machine logged.

##### Requiring a session

By default, a command without a session id runs and is recorded alone. That suits a person at a terminal. An eval harness or a CI job needs the opposite: a call without an id is lost from the run, and its scores are wrong without anyone noticing. So it can require one:

```sh
ANFRA_EXPERIMENTAL_REQUIRE_SESSION=1
```

With it set, a command without a session id fails before it runs:

```text
code: session_required
scope: client
message: This command needs a session id, because ANFRA_EXPERIMENTAL_REQUIRE_SESSION is set.
  Set ANFRA_SESSION_ID, send gen_ai.conversation.id in the call's baggage, or run it
  from an agent anfra detects (Claude Code, Codex, OpenCode, Goose).
```

- **What counts.** A detected agent's id, `ANFRA_SESSION_ID`, or `gen_ai.conversation.id` in the call's baggage.
- **Where it's checked.** The host checks it at its edge, before the command runs. The CLI checks once per command, and commands that run no op, such as `--help` and `anfra version`, are exempt. `anfra serve` checks each request. The engine never sees the setting, and a platform embedding it makes its own choice.
- **The scope is `client`**, because the calling program has to fix it, and its exit code is non-zero like any failure.
- **Data App pages are exempt.** Browser requests carry no session, so requiring one would break every Data App on that server.
- **A task is never required.** A session whose commands carry no [task](#tasks) is scored as a whole, and says so, so requiring a task would only make the agent's cooperation a condition of running.

The setting works the same with or without a collector, because `anfra.log` records the id either way.

##### How the id reaches spans and logs

This part is plumbing. Nobody sets it by hand, and it can change.

- **Recording.** The resolved session goes into the invocation's `Attribution`, and phase 0's span processor and log handler copy it onto every span and log record ([the caller on every span and log record](#the-caller-on-every-span-and-log-record)).
- **Names.** The session id is the `Attribution` key `gen_ai.conversation.id`, the GenAI conventions' name for a conversation ("session, thread"). Following phase 0's [naming rule](#the-caller-on-every-span-and-log-record), it is that span attribute on every span anfra makes, and the field `gen_ai_conversation_id` on each log record. Every span also carries it as `session.id`, OpenTelemetry's general name for a session. Langfuse and Claude Code's own telemetry group by `session.id` alone, so without it they wouldn't find anfra's spans, and joining them would take a collector rule the user writes ([research](#agent-tracing-platforms)). It is the one field anfra writes under two names. Log records keep `gen_ai_conversation_id` alone, and a caller still names its session as `gen_ai.conversation.id` in its baggage. The detected agent becomes `gen_ai.agent.name`, the GenAI conventions' name for an agent, and `gen_ai_agent_name`.
- **Many traces, one session.** Each command stays its own trace, and the session id links them. One trace per session would need a root span that lives as long as the session, which nobody ends and which arrives at the collector last, or never. Most trace stores handle that badly. The traces stay small, and a session is a query.
- **Nesting in the agent's own trace.** When anfra receives the agent's trace context, its spans start as children of the agent's span. A CLI command reads `TRACEPARENT` from its environment, which OpenTelemetry is standardising as "environment variables as context carriers" (a release candidate in 2026). Claude Code sets it on Bash commands. The MCP server reads `traceparent` from a tool call's `_meta`, which MCP's 2026 spec defines (SEP-414), and which Codex sends. An agent that traces itself then sees each anfra command under the tool call that ran it. The trace context decides only where a span sits in a tree, never which session it belongs to. It is an adapter that users never set, so the user docs mention it only as "works with agents that trace themselves".

**`TRACEPARENT` from the environment is always trusted,** whether or not telemetry exports. The environment is the user's own, unlike an HTTP header from a browser. Each log record carries its trace's id, exported or not ([logs](logs.md)), so a command's records in `anfra.log` carry the agent's trace id, and find the tool call that ran it in the agent's own trace, with no collector. A stale `TRACEPARENT` left in a shell puts unrelated commands under one trace, which changes where their spans sit, never their session.

##### The caller's baggage

A caller can name its own ids, such as its session or its eval run, in W3C `baggage`, beside the trace context. anfra copies the members it allows onto every span and log record of the call, as it copies the host's `Attribution`, so a caller that doesn't embed anfra can still find everything one of its runs did.

**Where anfra reads it.** The CLI reads `BAGGAGE` from its environment. `serve` reads the `Baggage` header, and an MCP tool call's `_meta.baggage`, from any caller, loopback or not. A platform embedding the engine passes it in the context it calls `Dispatch` with.

**What gets in.** One allowlist decides, wherever baggage enters: `ANFRA_EXPERIMENTAL_TRACE_BAGGAGE_INCLUDE`, a comma-separated list of 1 to 16 key patterns, where `*` and `?` are wildcards. It is `gen_ai.conversation.id` unless set, and `none` lets no baggage in. A platform sets it in code (`Settings.WithBaggageInclude`). OpenTelemetry's Java agent offers the same allowlist, as an experimental setting with wildcards. Each entry point keeps only the members it allows (`telemetry.AllowedBaggage`): `httpkit`'s middleware rewrites the header before its tracing starts, the MCP handler filters `_meta.baggage`, and the CLI filters `BAGGAGE`. What gets in is what anfra records, and what it sends on to its sidecars with each call. Of the allowed members, it keeps:

- values of at most 128 bytes, with no control characters;
- none of anfra's own keys: a key starting with `anfra.`, or one anfra declares, other than `gen_ai.conversation.id`. A caller can't forge a repo or a command id. A [task's](#tasks) id, `anfra.task.id`, is the exception: it gets in whatever the allowlist says, `none` included, but only in a task id's format, `t_` and 12 lowercase hex digits, so it holds nothing of the user's.

The host's `Attribution` wins over baggage on the same key. A bad allowlist is refused at start, as the data settings are.

**Trust.** Trust decides the trace, not the baggage: an exposed `serve` links a remote caller's trace rather than continuing it, but takes its allowed members as a loopback caller's. So a remote caller can name any `gen_ai.conversation.id`, or any member the allowlist adds, on its own calls. W3C's baggage spec asks that baggage not cross a trust boundary; the allowlist is that boundary, so a server exposed to callers it doesn't trust keeps it to ids it accepts any value of. To let no baggage in at all, set it to `none`; empty is unset, so the default. A platform names its caller from its own authenticated state, in `Attribution`, which wins.

**What it doesn't do.** anfra passes its trace and allowed baggage on per call, in each request's headers, never by changing its environment, which every goroutine shares. The processes it starts that outlive the caller's call, the sidecars and a console's `serve`, get no `TRACEPARENT`, `TRACESTATE` or `BAGGAGE` at all.

##### Parallel calls and subagents

An agent can run several anfra commands at once: parallel tool calls, and the subagents it starts. A subagent's commands carry its caller's session id. Claude Code gives a subagent's commands the same environment as the main agent's, with nothing that names the subagent, and Codex shares `CODEX_SESSION_ID` with its subagents ([research](#subagents)). So one session's commands can come from several agents and overlap in time, and their order by time is no single agent's order.

**Eval counts, so order doesn't matter.** Each rule is a count over what anfra records on its own: a session's commands, or an eval case's, which the harness names in the caller's baggage. It needs nothing from the agent:

| Rule | What is counted |
|---|---|
| Stays in the semantic layer | Raw SQL commands |
| Fixes what failed | Datasets with a failed query and no successful one |
| Doesn't repeat itself | Failed queries run again with the same text |
| Asks for what it needs | Queries that scan or return more than a set limit |
| Takes little effort | Commands |

A count is the same whichever agent ran each command, and in whatever order, so parallel calls and subagents change no score. A count doesn't tell the order, such as that an agent's query failed and it then wrote SQL. A person reads that from the trace.

**The lane** is which agent in a session ran a command: the main agent, or one of its subagents. The lane is the [task](#tasks): a subagent starts its own task under its caller's, so its commands carry its own id. When the agent marks tasks, each rule is also counted per task, which names the subagent that wrote SQL, and effort per user message. A missing mark makes a score coarser, never wrong.

The agent's program names its subagents too, for a person reading the trace, not for eval. Claude Code's `claude_code.tool` spans carry `agent_id`, and anfra's command span sits under the tool call that ran it ([how the id reaches spans and logs](#how-the-id-reaches-spans-and-logs)), when Claude Code's tracing is on and exports to the same store. Codex gives `CODEX_THREAD_ID` in a command's environment, and `_meta.threadId` on a tool call, which anfra records as `agent.thread.id`.

##### Tasks

A **task** is the agent's work on one of the user's messages that asks for data, or on a part of it. The agent marks it, so eval reads tasks when they are there, and scores the session as a whole when they aren't. The commands are experimental, under `anfra experimental telemetry task`. For a message asking for last month's revenue and this year's best product:

```sh
anfra experimental telemetry task start "What was revenue last month, and which product sold best this year?"
# prints t_7f3a91c0d2e4, a root task
anfra experimental telemetry task start "best product this year" --parent t_7f3a91c0d2e4
# prints t_c204e8a1b9f3, a subtask, such as a subagent's
anfra query --task t_c204e8a1b9f3 …
anfra experimental telemetry task note --task t_7f3a91c0d2e4 --kind summary "Revenue last month was $1.2M, and the best product was …"
```

- **`task start <goal> [--parent <id>]`** prints a new id: `t_` and 12 lowercase hex digits, which anfra makes. A root task's goal is the user's message. A subagent starts its own task under the id its caller passes it in its prompt. A parent that isn't a task's id is refused.
- **`--task <id>`** goes on every command the agent runs for the task: a flag on every CLI command, and an optional `task` argument on every MCP tool (`apikit.MCP.CallArgs`), taken out before the op reads its input. A `--task` that isn't a task's id fails the command, so a typo shows at once. It is a flag, not a variable: a variable before the command (`ANFRA_TASK=… anfra query`) stops Claude Code's allow rules from matching. It is an argument, not `_meta`, which the agent's program writes, not the model.
- **`task note --task <id> --kind finding|pivot|progress|summary <text>`** records what the agent found, a change of approach, where it is, or its answer, for a person to read. Its task is the command's own `--task`, as for any command.

With telemetry off, `task start` still prints an id, and both commands say that nothing was recorded and that the agent can skip tasks for the rest of its session.

The rules that keep a missing or wrong mark from scoring a command in the wrong task:

- **A root task is one user message that asks for data.** A message with two questions is one root task, with a subtask per question. Effort is scored per root task, over it and every task under it, so how the agent splits its work below a root doesn't change the score.
- **A command belongs to a task only by its id,** never by time, so overlapping work and subagents never mix.
- **A session is scored per root task only when each of its commands belongs to one.** A command without `--task`, or with an id that no `task start` in the session printed, is unassigned. A session with an unassigned command is scored as a whole, and its result says how many commands were unassigned.
- **A task has no end and no outcome.** A task is the commands that carry its id, so a task is never left open, and the agent never grades itself.
- **Eval never reads notes.** Its rules read task ids and parents only.

**What is recorded.** `task start`'s op span carries `anfra.task.id`, `anfra.task.parent_id`, and the goal as `anfra.task.goal`, which is data. Every span and log record of a command run with `--task`, or an MCP tool call with `task`, carries `anfra.task.id`. A note's op span carries `anfra.task.note.kind`, and the text as `anfra.task.note.text`, which is data. An id's format is checked, so it holds no user data, and is metadata. anfra stores no tasks. A command the CLI forwards to `anfra serve` passes its task in its baggage, as it passes its session, so its spans on `serve` carry the task too ([the caller's baggage](#the-callers-baggage)).

**How the agent learns it.** The anfra block of the repo's `AGENTS.md`, which `anfra agent setup` writes and imports into `CLAUDE.md`, tells the agent to mark tasks. An agent loads it at the start of every session, whether or not it loads a skill, and an agent that loads no skill never sees what the skills say ([research](#subagents)). The block's lines are in [agent setup](agent-setup.md).

The main agent passes the instruction on in a subagent's prompt, so a subagent follows it whether or not it loads `AGENTS.md` itself. anfra's own `aql-writer` subagent carries the rule in its definition too. A repo whose block predates it is told that its block is out of date, as for any change to the block.

### Considerations

**A session id from a caller.** Anyone can send any session id. That's harmless, because the id only labels records, and its length and characters are checked when it arrives. But a platform shouldn't trust the header as an identity, and sets `Attribution` from its own sessions instead. `anfra serve` on loopback takes the header as it is.

**anfra keeps no session between commands, for now.** anfra could keep the current session itself, so that agents exposing nothing would still have their calls grouped. These ways were considered:

| Way | How it works | Why not, for now |
|---|---|---|
| A state file with an idle timeout | The first command writes an id to a file under `~/.anfra`. Later commands reuse it until, say, 30 minutes pass with no command. This is how web analytics define a session ([prior art](#prior-art)). | Two agents running at once share the file, so their sessions merge. A person typing between an agent's calls joins its session, and a long pause splits one session in two. |
| A state file per project | The same, with one file per repo | The same problems, within one project |
| A state file per agent process | Walk up the parent processes to the agent's, and keep one id per agent PID | Process trees differ between Linux, macOS, Windows and containers. Claude Code's `/clear` starts a new session in the same process. |
| `anfra session start` writes the id, and later commands read it | The agent starts a session explicitly | The agent must remember to start and end it, which is the same problem as [the model passing an id](#the-interface). Ending it is easy to forget, so the next unrelated work joins it. |

All of these have the same three costs:

- **The boundaries are guesses.** anfra sees one command at a time, so it can't know where an agent's session starts or ends. The agent knows both.
- **The id matches nothing outside anfra.** An id anfra keeps doesn't join the agent's own traces, its transcript, or what Langfuse or Phoenix record for it.
- **State the user has to know about.** A file that changes what every later command records is hidden behaviour, and has to be reset, cleaned up and explained.

So anfra reads the caller's id on each command and stores nothing. For agents that expose nothing, whoever launches them sets `ANFRA_SESSION_ID`.

**anfra creates no id of its own,** not for `anfra console`, nor for a Data App page. A person's console work and a page's queries are found by time and trace, and an id anfra made up would match nothing outside it. A command doesn't get one either: it would cover exactly one trace, which the trace id already does.

**Detecting the agent's session can't be turned off.** A setting for it was considered, for three cases: keeping an agent's session id out of a shared collector, though the id is random and says nothing of the user; detection picking the wrong agent, which hasn't been seen; and an eval harness that wants only its own id, which instead names its run in the caller's baggage, beside the agent's session ([which id wins](#sessions)). It waits for a real case, as `ANFRA_EXPERIMENTAL_DETECT_SESSION=0`.

**No session header, flag or `_meta` key of anfra's own.** A caller names its session as `gen_ai.conversation.id` in W3C baggage, which every entry point already takes, so `Anfra-Session-Id`, `--session` and `_meta["anfra/session"]` would each duplicate it, and each slip past the allowlist that decides what of a caller's ids gets in.

**Agent tasks were considered, and removed.** In that design, a coding agent recorded each task it did for the user with three commands: `task start --intent … --acceptance …`, `task note --kind pivot|finding`, and `task end --outcome met|partial|failed`. The task's id was its trace's id, its start the trace's root, and the host kept the session's open task, so every command the agent ran until the end carried the task's id. It was built as a proof of concept, and removed:

- **The lifecycle was the agent's.** Agents forgot to end tasks, so tasks stayed open, and every later command fell in them. Fixes without a hook (timeouts, hints, closing on idle) only limit it. A hook works in one agent only.
- **A task had no set size.** One agent opened a task per question, another one per session, so tasks couldn't be compared across agents or runs.
- **The outcome was self-graded.** `met` is the agent's claim, and checking it takes the answer against a reference anyway.
- **A command found its task by session.** Without a session id, no task; subagents of one session shared one open task.

**Call context was considered, and removed.** In that design, each call took an optional context, the why of it: `intent`, what the agent wanted from the call, and `goal`, the user's request that led to it. MCP tools took it as a `context` argument, the HTTP API as the `Anfra-Call-Intent` and `Anfra-Call-Goal` headers, and the CLI as `--intent` and `--goal`. The op's span recorded both as data, and the calls that shared a goal were one piece of work. A setting in `.env`, `ANFRA_EXPERIMENTAL_TRACE_CALL_CONTEXT`, offered it, off by default. It was built as a proof of concept, and removed:

- **It cost every call.** The agent wrote two sentences on each call, and the goal repeated across one request's calls.
- **Agents left it out.** A local agent that read `--help` through `head -30` never saw the flags, so the help had to open by asking for them. Optional fields are skipped by an agent that doesn't read why they matter.
- **The why was the agent's own words.** It says why the agent made a call, not whether the answer was right, which takes the answer checked against a reference.

An eval runner that defines its cases in its own code names each run in the caller's baggage ([the caller's baggage](#the-callers-baggage)).

**Marking the user's messages: notes were tried, and tasks chosen** ([tasks](#tasks)). Both were built as proofs of concept:

- **Notes, found by time.** `note --kind goal|progress|summary` recorded the user's question, what the agent found, and its answer. A question's commands were those between its goal note and the next. That breaks when work overlaps: a subagent's goal note ends its caller's question, parallel subagents' notes interleave, and a subagent still running when the user asks the next question has its later commands counted in that one.
- **Tasks, passed on every command.** The proof of concept kept a main agent's and a subagent's commands apart, even when they started in the same millisecond.

Tasks fix what removed the earlier agent tasks: anfra keeps no open task, so none is left open; a root task's size is the user's message; there is no outcome to self-grade; and a command finds its task by id, not by session. Unlike call context, a call costs only an id, not two sentences.

**A root task is a user message, not a question.** If the agent chose a task's size, the same seven commands could be one task of seven, two of three and four, or seven of one, and effort would score how the agent split its work. A question has no fixed edge either: "and by region?" may be one or two. The user's message is the one edge that is the same for every agent given the same input, so eval runs and setups compared on the same work compare fairly. A message with two questions scores about twice the effort, which the goal shows.

**Lanes from outside anfra were considered, and rejected** ([parallel calls and subagents](#parallel-calls-and-subagents)):

- **A Claude Code hook.** `PreToolUse` sees a subagent's `agent_id`, but can give it to the command only by rewriting the command, and Claude Code then checks the user's permission rules against the rewritten command, which an allow rule such as `Bash(anfra query:*)` no longer matches. Recording `agent_id` beside the command instead means matching the two by text and time, in one agent only.
- **The agent's transcript.** Claude Code keeps each subagent's transcript apart, but reading it means anfra reads another tool's files, in a format that tool doesn't promise.
- **Ordering by time, or by content.** Read by time, overlapping commands look like one agent's sequence. Pairing a failed query with the later command that looks like its fix is a guess.

Counts need no lane, so none of these is needed.

**Eval needs no task.** Tasks need the agent to follow an instruction, and a missing one is never noticed by the agent. In a real Claude Code session, the main agent started its subagents without loading any skill ([research](#subagents)), so a step in a skill would never have reached it. So every rule works over the session, and tasks only refine effort.

**Standards carry the trace.** Each piece of carrying a trace has one, though none covers why an agent made a call. anfra follows them, so every agent or harness that adopts them joins anfra's traces with no work on anfra's side:

| Piece | Standard | anfra |
|---|---|---|
| A trace into a CLI | `TRACEPARENT`, OpenTelemetry's environment carriers (release candidate) | Reads it at startup, and passes its own on to the processes it starts |
| A trace into an MCP call | `_meta.traceparent`, MCP SEP-414 (final) | Reads it on each tool call |
| A trace into an HTTP request | W3C `traceparent` | Reads it from loopback callers |
| Grouping by conversation | `gen_ai.conversation.id`, OpenTelemetry's GenAI conventions (development), and `session.id`, its general session conventions | Sets both on spans |
| A CLI's span | `process.exit.code`, and `error.type` and an error status when it isn't 0, OpenTelemetry's CLI conventions (development) | Sets all three on the command's span |
| Why an agent made a call | None. A `gen_ai.task.*` proposal is a stalled draft. | Not recorded |

A carrier of anfra's own, such as `ANFRA_TRACE`, was ruled out: agents that don't pass the standard one wouldn't pass anfra's either.

### Implementation details

**Decide first**

- [x] Whether `TRACEPARENT` from the environment is always trusted: always, exported or not ([how the id reaches spans and logs](#how-the-id-reaches-spans-and-logs)).
- [x] How much eval depends on the agent's cooperation: none, with the agent's marks only refining effort ([parallel calls and subagents](#parallel-calls-and-subagents)).
- [x] How the agent marks its work on a user's message: [tasks](#tasks), found by id.
- [x] How the instruction to mark tasks reaches the agent: the anfra block of `AGENTS.md` ([tasks](#tasks)).

**Sessions**

- [x] **The resolver** (`internal/agentsession`): the known-agents table as data, with a test per entry; the CLI's order (a known agent, then `ANFRA_SESSION_ID`); the id check, at most 128 bytes, no control characters, a refused id counted as missing.
- [x] **The CLI.** Resolved once per command, where its root span starts (`sessionAttribution` in `traceCommands`), so CLI-only commands and the spans before the op are labelled too: `gen_ai.conversation.id`, `gen_ai.agent.name`, `gen_ai_conversation_id` and `gen_ai_agent_name`.
- [x] **The CLI to `serve`, and to the sidecars.** The command puts its session in its baggage as `gen_ai.conversation.id` (`telemetry.WithCallerBaggage`), when the allowlist lets it in, so a forwarded command's spans on `serve`, and the sidecars' spans, carry it. No header: `serve` takes baggage from any caller. The cloud client sends no baggage.
- [x] **`serve`.** `gen_ai.conversation.id` from each call's baggage, the header or `_meta.baggage`, as the allowlist lets it in.
- [x] **MCP, the agents' own keys.** A tool call's `_meta.sessionId` (Codex) or `_meta["agent-session-id"]` (Goose) names its session and agent (`agentsession.FromMeta`, through `apikit.MCP.MetaCaller`), over its baggage, and put in the op's baggage as the CLI's is (`telemetry.WithCallerBaggage`), so the sidecars' spans carry it too.
- [ ] **Requiring a session.** `ANFRA_EXPERIMENTAL_REQUIRE_SESSION=1`, checked by the CLI before an op runs and by `serve` per request, with Data App pages exempt. The error code `session_required` (scope `client`) is defined in `internal/errcode`. A new public code changes the API contract, so regenerate it and the SDK's types (`scripts/openapi.sh generate`, `pnpm --filter @holistics/anfra-sdk generate`).
- [x] **`ANFRA_SESSION_ID`** (`agentsession.ExplicitVar`), for the CLI: the fallback when no known agent gives its id.
- [x] **`session.id`.** The span processor writes the session's id as `session.id` beside `gen_ai.conversation.id`, on every span (`spanAttrs` in `shared/telemetry/attributes.go`), with the key declared as metadata in `shared/telemetry/keys.go`, so a caller's baggage can't set it. Log records and the baggage key stay `gen_ai.conversation.id` alone. Checked against a live stack.

**Nesting in the agent's trace**

- [x] **The CLI.** Its root span (`traceCommands` in `cmd/anfra/telemetry.go`) starts under `TRACEPARENT` from the environment, read once. anfra never writes it back: its trace reaches the sidecars and a forwarded `serve` in each call's headers, and the sidecars and a console's `serve` are started without the caller's trace context (`stripped` in `internal/sidecar/env.go`, `console_server.go`).
- [x] **The CLI's exit code.** The command's span records `process.exit.code` (`exitCode` in `cmd/anfra/main.go`), and, when it isn't 0, an error status and `error.type`, whoever's fault it was, as OpenTelemetry's CLI conventions define an error: the error's code, or the exit code, such as `1`, for an answer that says it failed, such as validate's problems. An op's span keeps the server's rule, an error status only when the failure is anfra's, so a query on op spans' status finds what anfra got wrong, and one on command spans finds every command that failed.
- [ ] **Checked live with an agent:** a headless `claude -p` with tracing on, running anfra, shows anfra under `claude_code.tool.execution`. For a command a subagent ran, the `claude_code.tool` span above it carries the subagent's `agent_id`. Checked with `TRACEPARENT` set by hand only.
- [x] **`serve`.** An MCP tool call's span starts under `_meta.traceparent` from a trusted caller (`apikit.MCP.TrustMeta`, on loopback), over HTTP.
- [x] **The engine.** An op `Dispatch`ed in a context with the host's span starts under it, and carries the host's `Attribution` (`engine/tracing_test.go`).
- [x] **The caller's baggage** ([the caller's baggage](#the-callers-baggage)): the allowlist in `shared/telemetry`, the CLI's `BAGGAGE`, `serve`'s header and `_meta.baggage` from any caller, each filtered by the allowlist where it enters (`telemetry.AllowedBaggage`), so what is recorded and what goes on to the sidecars are the same.
- [x] **The caller's baggage in `anfra status`,** as `telemetry.baggage`: the keys let in and what set them, with the API contract and the SDK's types regenerated.

**Commands and sampling**

- [x] **Command spans.** Marked `anfra.command`, and every span of a command carrying `anfra.command.id`, so a command's spans are found together when its trace holds other commands too. The reference's models group by `command_id`, checked against a live stack: `is_command` keeps a fallback for spans recorded before the marker.
- [x] **Telemetry off samples nothing,** even under a sampled caller's span: the provider that exports nothing uses `NeverSample`, not `ParentBased`. Nothing relied on it: each `IsRecording` check only skips work. `TestACallersSampledTraceRecordsNothingWithoutAnEndpoint` checks it.
- [x] **Exporting follows a trusted caller's sampling.** The exporting provider is `ParentBased(AlwaysSample)`: a command with no caller's trace is always recorded, and one under a loopback caller's trace is recorded when the caller's is. W3C Trace Context asks that a component making a recording decision "SHOULD respect the `sampled` flag", so a trace is whole or absent. An untrusted caller's trace is only linked, so its flag decides nothing.
- [ ] **Codex's threads.** `CODEX_THREAD_ID`, and a tool call's `_meta.threadId`, as `agent.thread.id`, so a person reading the trace tells Codex's subagents apart ([parallel calls and subagents](#parallel-calls-and-subagents)).

**Tasks** ([tasks](#tasks))

- [x] **`task start` and `task note`,** core commands in the registry (`internal/command/task`), as `experimental.telemetry.task.*`, so the CLI, `serve` and MCP all have them. The CLI puts a core command whose group is a CLI-only command under it (`appCommands`). API contract and SDK types regenerated.
- [x] **`--task`** on every CLI command (`addTaskFlag`, checked in `checkTaskFlag`), passed to a forwarded `anfra serve` in the command's baggage (`formattedKeys` in `shared/telemetry/baggage.go`), and a `task` argument on every MCP tool (`apikit.MCP.CallArgs`, `mcpTask`), with the id's format checked. Checked against a live stack: a root task, a subtask under it, a command and a note each carry their task.
- [x] **The keys** in `shared/telemetry/keys.go`: `anfra.task.id`, `anfra.task.parent_id` and `anfra.task.note.kind` as metadata, `anfra.task.goal` and `anfra.task.note.text` as data.
- [x] **The instruction,** in the anfra block of `AGENTS.md` (`internal/agents/agentsmd.go`, copied in [agent setup](agent-setup.md)), and the subagent's rule in `skills/plugins/anfra/agents/aql-writer.md`.
- [ ] **Checked live with Claude Code:** a main agent that starts two subagents in parallel, with no skill loaded, marks a root task, and each subagent its own under it.

**Docs**

- [x] **The usage page** ([agent tracing](../usage/observability-and-eval/agent-tracing.md)): finding a session's id, what a trace holds, tagging commands, nesting under the agent's trace. `ANFRA_SESSION_ID` and the baggage allowlist in [configs](../usage/configs.md#telemetry).
- [ ] **[Logs](logs.md)** gains `gen_ai_conversation_id` and `gen_ai_agent_name`, which log records already carry.
- [ ] **With requiring a session:** `ANFRA_EXPERIMENTAL_REQUIRE_SESSION` in configs, and `session_required` on the usage page.
- [x] **With tasks:** a section on the usage page, saying what the anfra block tells the agent, and the `anfra.task.*` attributes in what a trace holds.

### Other research notes

These notes are pointers to check before building, not decisions. They were gathered in November 2026, and these products change quickly.

#### Agent tracing platforms

All the platforms below accept OpenTelemetry. They differ in how they group spans into a session, and in which attribute they read for it.

| Platform | Accepts | Groups a session by | Self-hosted | Notes for anfra |
|---|---|---|---|---|
| **Langfuse** | OTLP over HTTP only, at `/api/public/otel` | `session.id` or `langfuse.session.id`. The docs disagree on whether it must be on the root span or on every span. | Yes, open source. Postgres, ClickHouse, Redis and object storage. | Closest to the proposed stack, since it stores traces in ClickHouse too. It refuses gRPC, which canal-query used to export only. |
| **Arize Phoenix** (OpenInference conventions) | OTLP over HTTP (port 6006) and gRPC (4317) | `session.id`. For spans in OpenTelemetry's GenAI conventions, `gen_ai.conversation.id`. | Yes, open source. | Converts GenAI-convention spans to its own, so either attribute works. |
| **LangSmith** | OpenTelemetry ingestion | Threads. How OTLP spans map to them wasn't confirmed. | Enterprise contract only. | Tied closely to LangChain. |
| **Braintrust, Opik (Comet), MLflow, Laminar** | OpenTelemetry ingestion, by their own accounts | Not checked. | Opik and MLflow are open source. | Mostly evaluation-first. Details came from vendor articles only. |
| **General backends** (Honeycomb, Dash0, SigNoz, Grafana) | OTLP | Any attribute the user queries by. Honeycomb's agent docs follow `gen_ai.conversation.id`. | Varies. | Treat agent spans as ordinary spans. |

What this means for the design:

- **The session is both `gen_ai.conversation.id` and `session.id`.** Phoenix and Honeycomb read the first, the GenAI conventions' name. Langfuse and Claude Code's own telemetry read the second, so writing both joins anfra's spans with theirs without a collector rule. Put it on every span, not only the root, because the platforms disagree on which they read.
- **Export over HTTP everywhere.** At least one major platform refuses gRPC. That is one reason canal-query gains OTLP/HTTP ([one trace across processes](#one-trace-across-processes)).
- **ClickHouse is a common choice.** Langfuse, ClickStack and the collector's exporter all store traces in ClickHouse. The proposed compose stack doesn't lock anyone in, because every platform above reads the same OTLP.

#### Coding agents that trace themselves

Claude Code exports traces, behind `CLAUDE_CODE_ENABLE_TELEMETRY=1` and `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`. Its spans form this tree:

```text
claude_code.interaction        one per user prompt
├── claude_code.llm_request
└── claude_code.tool           one per tool call
    ├── claude_code.tool.blocked_on_user
    └── claude_code.tool.execution
```

- Every span carries `session.id`.
- **Bash commands inherit `TRACEPARENT`,** the W3C trace context of the tool call's span. A program that reads it can put its spans under the tool call. This is what [joining the agent's own trace](#sessions) relies on. Claude Code sends it only when it talks to Anthropic's API directly, unless `CLAUDE_CODE_PROPAGATE_TRACEPARENT=1` is set.
- **Prompts, tool inputs and tool output are redacted by default.** `OTEL_LOG_USER_PROMPTS`, `OTEL_LOG_TOOL_DETAILS` and `OTEL_LOG_TOOL_CONTENT` turn them on. That is the same shape as anfra's [data settings](#data-in-spans): what the user typed is off by default, one switch per kind of content.
- **Claude Code passes its session id to the commands it runs:** `CLAUDE_CODE_SESSION_ID`, in Bash and PowerShell commands, hooks and stdio MCP servers. It matches the `session_id` its hooks receive, and changes on `/clear`. `CLAUDECODE=1` marks any process Claude Code started. Exports don't persist between Bash calls ([env vars](https://code.claude.com/docs/en/env-vars), [tools reference](https://code.claude.com/docs/en/tools-reference)).

#### What coding agents expose

Checked in November 2026: in the source of the open-source agents (their main branches), and in the official docs of the others. **Unverified** marks what came only from forums, third-party pages or tests. These change with every release, so the table in [the interface](#the-interface) needs re-checking from time to time.

| Agent | Session id in a command's environment | Agent marker | On an MCP call | Hooks get the session id | OpenTelemetry session attribute | `export` persists |
|---|---|---|---|---|---|---|
| Claude Code | `CLAUDE_CODE_SESSION_ID` | `CLAUDECODE=1`, `AI_AGENT` | Same variable, for stdio servers. `traceparent` on HTTP MCP requests | `session_id` | `session.id` | No |
| Codex CLI | `CODEX_SESSION_ID` (root session), `CODEX_THREAD_ID`, `CODEX_TOOL_CALL_ID` | `CODEX_VERSION` | `_meta.sessionId`, `threadId`, `callId`, `traceparent` | `session_id` | `conversation.id` | Probably not |
| Gemini CLI | None | `GEMINI_CLI=1` | None | `session_id`, `GEMINI_SESSION_ID` | `session.id`, `gen_ai.conversation.id` | No |
| Copilot CLI (1.0.63) | `COPILOT_AGENT_SESSION_ID`, found in code, not in docs | `COPILOT_CLI=1` | Unknown | `sessionId` | `gen_ai.conversation.id` | Probably yes |
| VS Code agent mode | None | `AI_AGENT=github_copilot_vscode_agent`, `COPILOT_AGENT=1` | Not documented | Not documented | `gen_ai.conversation.id` | Probably yes |
| Cursor | Unverified: `CURSOR_AGENT=1`. A `CURSOR_CONVERSATION_ID` is claimed by one third party and denied in a forum thread | Unverified | Not documented | `conversation_id` | No trace export | Unknown |
| OpenCode | `OPENCODE_SESSION_ID` | `OPENCODE=1`, `AGENT=1`, `AI_AGENT=opencode` | None | Plugins can set a command's env | `session.id` | Not checked |
| Goose | `AGENT_SESSION_ID` | `AGENT=goose`, on some commands only | `_meta["agent-session-id"]`, and `AGENT_SESSION_ID` for stdio servers | `session_id` | `session.id` | Not checked |
| Kiro | Unverified: `KIRO_SESSION_ID` | Unverified: `TERM_PROGRAM=kiro` | Not documented | `session_id` | None found | Not checked |
| Cline | None | `CLINE_ACTIVE=true` | None | `taskId` | A `ulid` property | Not checked |
| Windsurf | None documented | None documented | Not documented | `trajectory_id`, `execution_id` | None found | Not checked |
| Continue | None | None | None | `session_id` | Metrics only | Not checked |
| Amp | None documented. Commits get an `Amp-Thread-ID` trailer | None documented | Not documented | `thread.id`, in plugin events | None documented | Not checked |
| Zed | None | `ZED_TERM=true`, set in every Zed terminal | None | No hooks | None | Not checked |
| Aider | None | None | No MCP | No hooks | None | Not checked |

What this shows:

- **No standard name exists for an agent's session id.** Each agent uses its own variable, so anfra needs a table. Goose's `AGENT_SESSION_ID` sounds generic, but no other agent sets it.
- **`AI_AGENT` is the one marker that is spreading.** VS Code calls it the cross-vendor standard, and Claude Code, OpenCode, glab, GitLab Duo, the Slack CLI and Vercel's `detect-agent` set or read it.
- **Only Claude Code lets a hook set a variable for later commands.** Its `SessionStart` hook can write exports to `CLAUDE_ENV_FILE`, a script Claude Code runs before each Bash command ([env vars](https://code.claude.com/docs/en/env-vars)). Claude Code already exposes its session id, so anfra doesn't need it. Cursor's `sessionStart` hook can return `env`, but that reaches only later hooks. So a hook can't fill the gap for the agents that expose nothing.
- **MCP standardised trace context, not sessions.** SEP-414 puts `traceparent`, `tracestate` and `baggage` in `_meta`. No proposal covers a session or conversation id, and SEP-2567 removed `Mcp-Session-Id`. Codex's `_meta.sessionId` and Goose's `_meta["agent-session-id"]` are each their own.
- **Few agents pass trace context to their children.** OpenTelemetry's spec for `TRACEPARENT` in the environment is a release candidate, and only Claude Code is known to set it on its commands. Codex reads it from its own environment, as a child would.

Sources: [Codex](https://github.com/openai/codex) (`codex-rs/protocol/src/shell_environment.rs`, `core/src/exec_env.rs`, `core/src/mcp_tool_call.rs`), [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`packages/core/src/services/shellExecutionService.ts`, `hooks/hookRunner.ts`), [Copilot CLI hooks](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-hooks-reference), [Copilot OpenTelemetry](https://docs.github.com/en/copilot/reference/enterprise-administrators/opentelemetry-reference), [VS Code `aiAgentEnv.ts`](https://github.com/microsoft/vscode/blob/main/src/vs/platform/chat/common/aiAgentEnv.ts), [VS Code issue 311734](https://github.com/microsoft/vscode/issues/311734), [Cursor hooks](https://cursor.com/docs/agent/hooks), [Cursor forum: conversation id](https://forum.cursor.com/t/cursor-conversation-id-through-environment-variables/160346), [OpenCode](https://github.com/sst/opencode), [Goose](https://github.com/block/goose) (`crates/goose/src/agents/platform_extensions/developer/shell.rs`, `mcp_client.rs`), [Kiro hooks](https://kiro.dev/docs/cli/hooks/), [Cline](https://github.com/cline/cline), [Windsurf hooks](https://docs.devin.ai/desktop/cascade/hooks), [Continue](https://github.com/continuedev/continue), [Amp plugin API](https://ampcode.com/manual/plugin-api), [Zed](https://github.com/zed-industries/zed), [Aider](https://github.com/Aider-AI/aider), [MCP SEP-414](https://modelcontextprotocol.io/seps/414-request-meta.md), [MCP SEP-2567](https://modelcontextprotocol.io/seps/2567), [OpenTelemetry environment carriers](https://opentelemetry.io/docs/specs/otel/context/env-carriers/).

#### Agents that pass their trace on

Checked in November 2026, in the docs and the source of each agent. Only Claude Code was run.

| Agent | Reads `TRACEPARENT` when started | Sets `TRACEPARENT` on the commands it runs | Sends `traceparent` to MCP servers |
|---|---|---|---|
| Claude Code, its CLI and its VS Code and JetBrains extensions | Headless and SDK sessions only | Yes, the tool execution span's, when its tracing is on | HTTP MCP only |
| Codex, its CLI and its extension | Yes (`codex-rs/otel/src/trace_context.rs`) | Not found in its source | Yes, in `_meta` (`codex-rs/rmcp-client/src/trace_context.rs`) |
| VS Code Copilot agent mode | Not documented | Not documented. Copilot CLI sessions in its terminal are separate traces. | Not documented |
| Gemini CLI | Not found | Not found | Not found |
| Goose, OpenCode | — | No | No, with open requests for SEP-414 |
| Cursor, Windsurf, Cline, Roo Code, Continue, JetBrains AI Assistant and Junie | No built-in tracing found. Cursor and Windsurf are traced through third-party hooks. | No | No |

- **Claude Code, checked live.** `claude -p` with `CLAUDE_CODE_ENABLE_TELEMETRY=1`, `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` and an OTLP exporter gave its Bash command `TRACEPARENT=00-<trace>-<span>-01`, where the span was `claude_code.tool.execution`, under `claude_code.tool`. No `claude_code.interaction` root was exported for that run.
- **VS Code Copilot traces itself** when `github.copilot.chat.otel.enabled` is on or `OTEL_EXPORTER_OTLP_ENDPOINT` is set, by default to `localhost:4318`: `invoke_agent`, then `chat <model>`, `execute_tool <name>` and `execute_hook`, with `gen_ai.conversation.id`. Its default port is the local stack's.
- **Hooks fill the gap for some.** loongsuite-pilot's PreToolUse hook puts `TRACEPARENT=…` before each Bash command, off by default, for Claude Code, Codex, Cursor and others.
- **The standards aren't settled.** OpenTelemetry's CLI conventions leave propagation as a TODO (semantic-conventions issue 1612, open since 2024), pointing to the environment carriers spec. That spec says the child reads the variables at startup, the parent injects into a copy of the environment for each child, and the values aren't for secrets. It says nothing on nested processes.

Sources: [Claude Code monitoring](https://code.claude.com/docs/en/monitoring-usage), [Codex](https://github.com/openai/codex), [VS Code: monitor agents](https://code.visualstudio.com/docs/agents/guides/monitoring-agents), [Copilot SDK OpenTelemetry](https://docs.github.com/en/copilot/how-tos/copilot-sdk/observability/opentelemetry), [Gemini CLI telemetry](https://geminicli.com/docs/cli/telemetry/), [Goose issue 12598](https://github.com/aaif-goose/goose/issues/12598), [OpenCode issue 46856](https://github.com/anomalyco/opencode/issues/46856), [loongsuite-pilot](https://github.com/alibaba/loongsuite-pilot), [opentelemetry-hooks](https://github.com/o11y-dev/opentelemetry-hooks), [CLI conventions](https://opentelemetry.io/docs/specs/semconv/cli/cli-spans/), [issue 1612](https://github.com/open-telemetry/semantic-conventions/issues/1612), [environment carriers](https://opentelemetry.io/docs/specs/otel/context/env-carriers/).

#### Subagents

Checked in November 2026.

- **A Claude Code subagent's commands get the main agent's environment.** In Claude Code 2.1.330, a subagent's Bash command had the same variables as the main agent's, the same `CLAUDE_CODE_SESSION_ID` included, and none that named the subagent.
- **Claude Code's own trace names the subagent.** Its `claude_code.tool` and `claude_code.llm_request` spans carry `agent_id` and `parent_agent_id`, both absent for the main agent, and a subagent's spans nest under its caller's `claude_code.tool` span for the Agent tool. Bash commands get `TRACEPARENT` for the `claude_code.tool.execution` span, which sits under `claude_code.tool`. The docs don't say outright that a subagent's Bash commands get it too. All of it is under "Traces (beta)", and the docs give v2.1.310 as the first version with `agent_id` on events.
- **Claude Code's hooks name the subagent too.** `PreToolUse` input carries `agent_id` and `agent_type` for a subagent's tool call. A hook can rewrite the command (`updatedInput`), but Claude Code checks permission rules against the rewritten command, and an allow rule doesn't match past an assignment of a variable it doesn't know. `SubagentStart` and `SubagentStop` give each subagent's `agent_id` and `agent_type`.
- **Claude Code's transcripts keep each subagent apart.** Each subagent's transcript is its own file under the session's `subagents/` folder, with `agentId` on each entry, and a `.meta.json` beside it with the subagent's type and the tool call that started it.
- **In a real session, the main agent loaded no skill before delegating.** Asked for two queries through subagents, Claude Code started two `aql-writer` subagents at once, without loading a skill.
- **No tool found records which of its caller's subagents called it.** OpenTelemetry's GenAI conventions nest a subagent as an `invoke_agent` span, with no attribute for its parent agent. agent-eval-mcp scores coding agents' sessions from their transcripts, subagents' included. A hook-based tracer that kept shared state put concurrent subagents' spans in the wrong traces (claude-pace-maker issue 158).

Sources: [Claude Code monitoring](https://code.claude.com/docs/en/monitoring-usage), [Claude Code hooks](https://code.claude.com/docs/en/hooks), [Claude Code permissions](https://code.claude.com/docs/en/permissions), [GenAI agent spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/), [agent-eval-mcp](https://github.com/mrd5591/agent-eval-mcp), [claude-pace-maker issue 158](https://github.com/LightspeedDMS/claude-pace-maker/issues/158).

#### Tasks and evals elsewhere

No tool found asks the agent calling it to declare its task, and puts the tool's own steps under it. Each piece has precedent:

- **A reason on each call.** PostHog's MCP analytics adds a `context` argument to every tool and records it as `$mcp_intent`. AWS adds a user-goal parameter to its MCP tools. Both cost tokens on every call.
- **A task as a span.** An OpenLLMetry RFC (traceloop/openllmetry issue 3460, November 2025) proposes `gen_ai.task.*`: create, execute and delegate, with a status. Its author called it a draft with no timeline in March 2026. Its tasks are set by the developer's code, and OpenLLMetry's `@task` decorator marks a function, not a goal.
- **An outcome at the end.** AgentOps' `end_session` takes `Success`, `Fail` or `Indeterminate`, and a reason, set by the developer's code.
- **The agent reporting on itself.** Copilot CLI has a `report_intent` tool, undocumented. Microsoft's control plane for MCP proposes that agents declare their intended actions for policy checks, on its roadmap.
- **Grouping without a why.** Claude Code's `prompt.id` and `claude_code.interaction` cover one prompt, and OpenTelemetry's `invoke_agent` one agent run. None records what the agent meant to do or whether it got there.

Sources: [OpenLLMetry RFC](https://github.com/traceloop/openllmetry/issues/3460), [AgentOps sessions](https://docs.agentops.ai/v1/concepts/sessions), [MCP servers telling agents what to do next](https://thenewstack.io/aws-mcp-agent-observability/), [Microsoft: securing MCP](https://developer.microsoft.com/blog/securing-mcp-a-control-plane-for-agent-tool-execution/), [Copilot CLI issue 556](https://github.com/github/copilot-cli/issues/556), [Sentry MCP monitoring](https://docs.sentry.io/ai/monitoring/mcp).

#### Prior art

No tool found does the whole of [sessions](#sessions): a tool that agents call, putting the agent's session id on its own spans. Each piece has precedent.

- **Detecting the calling agent from its environment.** Vercel's [`@vercel/detect-agent`](https://cdn.jsdelivr.net/npm/@vercel/detect-agent@1.2.3/README.md) checks `AI_AGENT` first, then each agent's own marker (`CLAUDECODE`, `CURSOR_AGENT`…). Hugging Face Hub has the same in `_detect_agent.py`. CI detection, as in `ci-info`, did this earlier for `CI` and `GITHUB_ACTIONS`.
- **Reading the agent's session id and trace context.** The Rust crate [`is-ai-agent`](https://docs.rs/crate/is-ai-agent/latest/source/README.md) is closest to this design. It reads `AI_AGENT`, then a table of markers, and returns each agent's session id (`CLAUDE_CODE_SESSION_ID`, `CODEX_THREAD_ID`, `QWEN_CODE_SESSION_ID`, `CLINE_TASK_ID`…) and its `TRACEPARENT`, for a tool to attach to its own requests. It covers more agents than anfra's table, so it is worth checking when the table is updated. It picks Codex's thread id, while this design picks `CODEX_SESSION_ID`, so that a Codex session and its subagents stay one session. Codex sets both (`codex-rs/protocol/src/shell_environment.rs`).
- **`TRACEPARENT` in the environment.** The Jenkins OpenTelemetry plugin sets it in shell steps, and otel-cli, Thoth and Run with Telemetry set or read it ([OpenTelemetry blog](https://opentelemetry.io/blog/2026/environment-variable-context-propagation/)).
- **Grouping traces by a caller's id.** Langfuse groups traces by a `sessionId` the application chooses, and builds replay, scores and evals per session on it ([sessions](https://langfuse.com/docs/observability/features/sessions)). Its Flowise integration maps Flowise's own `chatId` to the session id, the same idea as reading an agent's id.
- **A platform that creates the id.** [PostHog](https://posthog.com/docs/data/sessions) and [Amplitude](https://amplitude.com/docs/data/sources/instrument-track-sessions) create session ids in their browser and mobile SDKs, ended by 30 minutes without activity. PostHog lets every tab of one browser share a session, and tells them apart with a second id, `$window_id`. Their server SDKs and HTTP APIs leave the id to the caller, because a server can't tell its callers' sessions apart. anfra, called by many agents, is in the same position as their servers. That is why it reads the caller's id first, and creates one only where it owns the session.
- **SDKs used like libraries take the caller's id and keep nothing.** The OpenAI Agents SDK links traces through a `group_id` the application passes, such as a chat thread id, and generates only the trace id ([tracing](https://openai.github.io/openai-agents-python/tracing/)). The Vercel AI SDK has no session at all: its ids cover one call, and an application passes its own conversation id through `runtimeContext` ([telemetry](https://ai-sdk.dev/docs/ai-sdk-core/telemetry)). Langfuse's SDK takes the `session_id` it is given, and OpenTelemetry itself has no session, only the `session.id` attribute.
- **Server SDKs make a session one request.** In server mode, Sentry starts a session when a request arrives and ends it when the response is sent, because a backend has no single user to group by. In app mode it keeps one per app launch or page load, like PostHog ([release health](https://docs.sentry.io/product/releases/health/)).
- **Command-line tools give each run an id, and leave grouping to whoever launches them.** dbt generates an `invocation_id` for each command ([docs](https://docs.getdbt.com/reference/dbt-jinja-functions/invocation_id)) and keeps nothing across commands. dbt Cloud, which runs the commands, adds its own run and job ids as `DBT_CLOUD_*` variables. anfra splits it the same way: a trace id per command, and `ANFRA_SESSION_ID` from the launcher.

#### Conventions

- **OpenTelemetry's GenAI conventions** define `gen_ai.conversation.id`, and span names such as `invoke_agent {agent.name}` and `execute_tool {tool.name}`, with `gen_ai.tool.name` and `gen_ai.tool.call.id`. The agent and tool parts are still marked Development, and changed in 2026. They moved into a repository of their own, `semantic-conventions-genai`. anfra's ops are tools from an agent's side, but anfra shouldn't name its own spans `execute_tool`, because the agent's span already is one. anfra adds only the conversation id and the agent's name, `gen_ai.agent.name` ("Human-readable name of the GenAI agent… MUST have low cardinality").

#### Sources

- Langfuse: [OpenTelemetry integration](https://langfuse.com/integrations/native/opentelemetry), [migration to v4](https://langfuse.com/integrations/native/opentelemetry/migration-to-v4)
- Arize Phoenix: [sessions](https://docs.arize.com/phoenix/tracing/how-to-tracing/setup-tracing/setup-sessions), [Claude Managed Agents tracing](https://arize.com/docs/phoenix/integrations/platforms/claude-managed-agents/claude-managed-agents-tracing)
- Claude Code: [monitoring](https://code.claude.com/docs/en/monitoring-usage), [Agent SDK observability](https://code.claude.com/docs/en/agent-sdk/observability)
- GenAI conventions: [Honeycomb, agents](https://docs.honeycomb.io/send-data/agents), [Dash0](https://www.dash0.com/knowledge/opentelemetry-genai-semantic-conventions-explained)
- Comparisons, mostly written by vendors: [Laminar](https://laminar.sh/article/2026-04-23-top-6-agent-observability-platforms), [Comet](https://www.comet.com/site/blog/ai-observability-tools/), [MLflow](https://www.mlflow.org/top-5-agent-observability-tools/)

#### From the first draft

- The idea "force every op to take `--agenttrace=<id>` in debug mode" became `ANFRA_SESSION_ID`, so no command's input changes.

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
