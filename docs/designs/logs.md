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

- **`msg`**: what happened, as a sentence for people. Capitalized, no full stop: `Update available`. It may carry values. It can be reworded in any release, so nothing matches on it.
- **`event`**: what happened, as a name for programs: `<area>.<what happened>` in snake_case (`serve.update_available`, `routine.failed`, `rpc.request`). Alerts, dashboards, searches and tests match on it. Every record has one.
- **Attributes**: the facts that vary, with snake_case keys (`routine`, `status`, `duration`). An alert reads them with the event: `http.request` with a `status` of 500 or more.
- **`level`**: `DEBUG`, `INFO`, `WARN` or `ERROR`, in every service. For a failure, its error's scope decides it (`user` info, `client` warn, `server` error; see [errors](errors.md)), so operators alert on what anfra must fix. Otherwise, warn for what the operator should act on, info for what changed, debug for detail.
- **`trace_id`, `span_id`**: the trace and span the record was written in, as lowercase hex, OpenTelemetry's names for trace context in JSON logs. They join one request's records across services, and a tracing backend links each record to its span. A record written outside any request (a server starting) has none. Tracing is always on for its ids, exported or not: with no OTLP endpoint, spans are not sampled and nothing is exported, but each gets its ids and passes them on (`traceparent`).
- **What each service adds to every record**: `ts`, `service`, `pid`, and `repo` when the record is about one.

## Each service logs what it saw itself

Whether to log is decided with what the service itself knows, never with a guess about what another service will log. A service logs:

- **Each request it answered, with how**: one record, with its outcome, its error's code if it failed, and its duration. anfra's `http.request` and `command.failed`, anfra-node's `rpc.request`.
- **What happened to the service itself**: starting, ready, shutting down, an update available.
- **What it absorbed that no caller will see**: a retry, a fallback, an item skipped, a routine's failed run.

It never leaves something out because another service may log it, and never logs on another's behalf. A failure that crosses services so has a record in each, each its own view: anfra-node's `rpc.request`, and anfra's `http.request`, each with the error's code. `trace_id` joins them; nothing removes either.

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
- **A rename or a removal is caught in review,** and the release notes mention it by hand, until the list of events does it ([not yet](#not-yet)).
- **A platform embedding the engine** logs `shared/`'s events (`http.request`, `routine.*`) under the same names, with its own beside them. `shared/logkit` is public API for that.
- **A log event is not a product-analytics event.** It is a named log record, what OpenTelemetry calls an event (a log record's `EventName`), for operators. Tracking what users do, for product analytics, is another contract: other readers, user identity and consent, other guarantees. It gets its own events and its own list, never `logkit`'s, so neither can break the other. Where one place in the code is both, it writes both, each defined on its own side.

## Writing a log line

- **Name the event for what happened,** not for the code that logs it: `routine.cut_short`, not `routine.log_cut_short`. The area is the part of the service it belongs to, as a reader would look for it: `serve`, `sidecar`, `routine`, `http`, `rpc`.
- **Prefer adding to changing.** A new attribute is free; renaming one breaks whatever reads it.
- **Log with a context wherever one is at hand** (`InfoContext(ctx, …)` and the like): it is what gives a record its trace. Which context, and whether to pass one down to get it, is [which trace a record joins](#which-trace-a-record-joins).
- **What is checked:** in anfra, sloglint (`.golangci.yml`) refuses a message that is not capitalized, and a key that is not snake_case; in both services, defining an event refuses one defined twice, misnamed or undocumented. Nothing checks that a record has an `event`, or that it logs the attributes its event lists: a reviewer does.

### Which trace a record joins

A record carries `trace_id` only if it is logged with a context that holds a span. Most records are logged where that context is already at hand. The question comes up when it is not: a function that logs takes no context today (a `Close()`), or the code that logs is a resource that outlives the request that created it.

**Ask: does the trace wait on the work the record tells of?** If the work is done for one request, and that request takes longer when the work does, the record belongs in its trace: it explains where the request's time went. If the work serves many requests, or none, it belongs to no request's trace, and a record that claimed one would mislead.

The same code can answer both ways, depending on who runs it. A sidecar started by a CLI command lives and dies inside that command: the command waits on its start, and on its stop, which can take 3s when it won't exit. Its records belong in the command's trace. The same sidecar started by `anfra serve` serves every request: its start and stop are the server's, and belong to none.

**Then pick how the context reaches the record.** Go programs do one of three things:

1. **Log without a context.** Right for a long-lived resource's own lifecycle, opened once and serving many: a connection pool, a server, `anfra serve`'s sidecars. It is what most libraries' `Close()` does. Wrong when the work is for one request: the request's trace then hides a cost it pays.
2. **Log at the caller,** which has the context, around a call that takes none: `logger.InfoContext(ctx, …); x.Close()`. Right when only one caller cares, and the record is about the caller's decision. Wrong when the event is the callee's own: each caller then repeats it, and a new caller forgets it.
3. **Pass the context down,** so the function logs with it. Right when the event is the function's own and its work is done for its caller. This is the usual answer once the first question says the trace waits on the work.

**A context passed down for its trace is not for cancellation,** unless the function says otherwise. A parameter `ctx` reads as "cancel this to stop waiting", as in `http.Server.Shutdown(ctx)`, so a function that takes one only to log with says so in its doc comment, as `slog.Handler.Handle` does ("must not be used for cancellation"). Whether the function should also honour cancellation is a separate decision about its behaviour, made on its own merits, not as a side effect of logging: `sidecar.Process.Close(ctx)` does not, so a sidecar closed after Ctrl-C still gets its 3s to exit.

**What the caller passes:**

- Its own context, when it has one for the work.
- `context.WithoutCancel(ctx)`, when the only context it has is canceled or may be, and the callee might be changed to honour cancellation later: the trace stays, the cancellation goes.
- Never a context kept in a struct to log with later: Go's rule for contexts, and the trace it holds may have ended. A resource that logs at its close takes the context at its close.

## Logs and the CLI

Logs are for operators and debugging, not for a program reading a command's stderr: that is a stream of documents with a contract of its own ([errors](errors.md#from-error-to-caller)), and log lines reach it only with `ANFRA_LOG_STDERR=1`.

## Not yet

- **canal-query** writes plain text lines (`starting Canal Query at 42183 (…)`), and its OpenTelemetry exporter's failures, to the same `anfra.log`: no JSON, no `service`, no `event`, no trace context. TODO: JSON records in the shape above.
- **A list of events, and release notes from it.** TODO: each service commits a list of its events, generated from its definitions (`logkit.Events()`, anfra-node's `listEvents()`) and checked by a test, so a change to an event shows in its pull request. `pnpm bump` compares the lists at the last release with the lists now, anfra-node's at the versions anfra pinned, writes a "Log events" section (what was removed, what is new), and refuses a patch version with anything removed. The lists are read from source, by version, rather than asked of a running binary: events are fixed at build time, and a release compares two versions.
