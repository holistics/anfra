# Logs

A log record has two readers: a person reading what happened, and a program (an alert rule, a dashboard, a search) that matches on it. Each gets a part of the record of its own, so neither stands in the other's way. The same rules hold in every service that writes to anfra's log:

| Service | Its logging |
|---|---|
| anfra-host (`anfra`) | `internal/logging`: where records go; `shared/logkit`: the events, defined once |
| anfra-node (`holistics-core/apps/anfra-node`) | `src/logger.ts`, and its events, defined the same way |
| canal-query | not yet: plain text lines ([not yet](#not-yet)) |

## Where records go

anfra writes JSON records to the repo's `anfra.log` (`<ANFRA_HOME>/repos/<repo id>/logs/`, which `anfra status` prints), and forwards each sidecar's stderr into the same file, so one file tells the story of all three processes. A command's terminal stays clean for its answer, its warnings and its error; `anfra serve` also copies the log to stderr, as a server's operator watches it there. `ANFRA_LOG_STDERR` changes either default (`0` or `1`). `LOG_LEVEL` sets the level.

## What a record carries, and who reads it

```json
{"ts":"…","level":"INFO","msg":"Update available","service":"anfra-host","repo":"…","pid":4242,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","event":"serve.update_available","latest":"1.4.0","current":"1.3.2"}
```

- **`msg`**: what happened, as a sentence for people. Constant, capitalized, no full stop: `Update available`, never one built from values. It can be reworded in any release, so nothing matches on it.
- **`event`**: what happened, as a name for programs: `<area>.<what happened>` in snake_case (`serve.update_available`, `routine.failed`, `rpc.failed`). Alerts, dashboards, searches and tests match on it. Every record has one.
- **Attributes**: the facts that vary, with snake_case keys (`routine`, `status`, `duration`). An alert reads them with the event: `http.request` with a `status` of 500 or more.
- **`level`**: `DEBUG`, `INFO`, `WARN` or `ERROR`, in every service. For a failure, its error's scope decides it (`user` info, `client` warn, `server` error; see [errors](errors.md)), so operators alert on what anfra must fix. Otherwise, warn for what the operator should act on, info for what changed, debug for detail.
- **`trace_id`, `span_id`**: the trace and span the record was written in, as lowercase hex, OpenTelemetry's names for trace context in JSON logs. They join one request's records across services, and a tracing backend links each record to its span. A record written outside any request (a server starting) has none. Tracing is always on for its ids, exported or not: with no OTLP endpoint, spans are not sampled and nothing is exported, but each gets its ids and passes them on (`traceparent`).
- **What each service adds to every record**: `ts`, `service`, `pid`, and `repo` when the record is about one.

## Each service logs what it saw itself

Whether to log is decided with what the service itself knows, never with a guess about what another service will log. A service logs:

- **Each request it answered, with how**: one record, with its outcome, its error's code if it failed, and its duration. anfra's `http.request` and `command.failed`, anfra-node's `rpc.*`.
- **What happened to the service itself**: starting, ready, shutting down, an update available.
- **What it absorbed that no caller will see**: a retry, a fallback, an item skipped, a routine's failed run.

It never leaves something out because another service may log it, and never logs on another's behalf. A failure that crosses services so has a record in each, each its own view: anfra-node's `rpc.failed`, and anfra's `http.request` with the error's code. `trace_id` joins them; nothing removes either.

Within a service, a failure is logged once, in the request's record or where it is absorbed, not at each function it passes through: the steps it passed through are on the error itself ([steps](errors.md#steps-context-written-once)).

## Events are a soft contract

Alert rules and dashboards depend on events and their attributes, so anfra keeps them compatible, as best it can: an event or an attribute is added freely, and renamed or removed only when it is worth it, in a minor release, never a patch, with the release notes saying so. It is not the core API's contract: no change to it is a `feat!:`, and nothing refuses one. `msg` is outside it.

- **An event is defined once**, at package level, with its name, what it means, and its attributes. Nothing invents one at a call site, as nothing invents an error code. In anfra, with `logkit.DefineEvent`:

  ```go
  var eventFailed = logkit.DefineEvent("routine.failed", "A routine's run returned an error.",
  	"routine", "outcome", "error")

  g.logger.ErrorContext(ctx, "Routine failed", logkit.EventKey, eventFailed,
  	"routine", name, "outcome", outcome, "error", err.Error())
  ```

  anfra-node defines its own the same way, with `defineEvent`.
- **Every event is listed** in `api/log-events.json`: anfra's, generated from every package it links, and anfra-node's, from the version anfra pins. `make check` fails on a stale list (`scripts/log-events.sh diff`), so a change to an event is a change to the list, in the pull request that makes it, or that bumps anfra-node, where a reviewer sees it.
- **The release notes say what changed.** `pnpm bump` compares the list at the last release with the list now and writes a "Log events" section: the events and attributes removed, and those new. A rename is a removal and an addition; the pull request says why. `pnpm bump` refuses a patch version with anything removed.
- **A platform embedding the engine** logs `shared/`'s events (`http.request`, `routine.*`) under the same names, with its own beside them.

## Writing a log line

- **Name the event for what happened,** not for the code that logs it: `routine.cut_short`, not `routine.log_cut_short`. The area is the part of the service it belongs to, as a reader would look for it: `serve`, `sidecar`, `routine`, `http`, `rpc`.
- **Prefer adding to changing.** A new attribute is free; renaming one breaks whatever reads it.
- **What is checked:** in anfra, sloglint (`.golangci.yml`) refuses a message that is not constant or not capitalized, and a key that is not snake_case; in anfra-node, a test refuses an event defined twice or misnamed; the list refuses to go stale. Nothing checks that a record has an `event`, or that it logs the attributes its event lists: a reviewer does.

## Logs and the CLI

Logs are for operators and debugging, not for a program reading a command's stderr: that is a stream of documents with a contract of its own ([errors](errors.md#from-error-to-caller)), and log lines reach it only with `ANFRA_LOG_STDERR=1`.

## Not yet

- **canal-query** writes plain text lines (`starting Canal Query at 42183 (…)`), and its OpenTelemetry exporter's failures, to the same `anfra.log`: no JSON, no `service`, no `event`, no trace context. TODO: JSON records in the shape above, its events in the list.
