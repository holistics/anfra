# Errors

An anfra error is not a string. It is a classified failure that a caller can act on: a person reading a CLI message, a script branching on a code, an agent fixing the one argument it got wrong, an operator reading a log line or a trace. The framework that does this lives in `shared/`, so the engine and every platform that embeds it (see [engine.md](engine.md)) speak the same error language:

| Package | Owns |
|---|---|
| `shared/apperr` | what an error means: its code, scope, message, details, the steps it passed through, and its wire form |
| `shared/appstep` | what a step is: a named operation, public or private, that both traces and contextualises errors |
| `shared/apptracing` | running steps: a span per step, and the step added to any error that leaves it |
| `shared/httpkit` | how an error becomes an HTTP response and a request log line |
| `shared/apikit` | how an op's errors reach HTTP and MCP callers, and which codes each op documents |
| `internal/errcode` | the engine's own namespace, and the codes no single domain owns |

Each package's doc comment is the detailed reference; this document is the shape of the whole.

## What an error carries, and who reads it

One failure has several readers, and most parts of an error are read by more than one of them, each for its own purpose. A formal error (`apperr.Error`) has:

- **A code**: which kind of failure this is (`query_invalid`, `validation_failed`). Programs and Go callers decide what to do on it (`errors.Is(err, code)` for "caused by", `errors.As` for the outermost formal error), and so do agents. Codes are defined once, at package level, with `apperr.DefinePublicCode`; nothing can invent one at a call site. A code is itself an error: `return errcode.SidecarUnavailable` is a formal error with the code's default message.
- **A scope**: which side must act. `user` (the request was well-formed, and the person's input or the state they act on refuses it), `client` (the calling program is wrong), or `server` (anfra is wrong, or something it depends on is down). Every caller reads it to know whether to fix its input, report a bug or wait; operators, to know what to alert on. It belongs to the code, so it cannot be paired with the wrong one at a call site, and it drives the log level and whether a span is marked as errored.
- **A message**: why it failed, as a sentence safe to show. For people and agents; never parsed, since it may change. Short, about the caller's situation, not anfra's internals.
- **Typed details**: facts to act on, such as the fields at fault (violations) or a query's diagnostics with their locations. Programs read them to point at what failed (highlight a field, mark a line), Go callers with `apperr.DetailsOf`, agents to fix their next call. A code defined with `DefinePublicCodeWith[D]` only accepts details of type `D`, and that type is published in the OpenAPI document, so a generated client knows what `details` holds for each code.
- **Steps**: the operations the error passed through, outermost first, added by `apptracing` (see [steps](#steps-context-written-once)). Public steps reach callers as `context`, what was being done when it failed; private ones stay in the log and the trace.

Two more parts come from where the error is served, not from the error itself:

- **A status**: the failure's conventional category, assigned per public code by the transport (an HTTP status) and repeated in the body for transports without one. Standard handling works on it (retry when unavailable, sign in again when unauthenticated, don't retry a refused input), and it adds to what the code says. A code knows nothing of any transport.
- **A request id**: which request this was, for support and operators to find its log line and trace ([one request, one trail](#principles)).

What stays out of every answer is the cause beneath an encapsulated error, and its private steps: they are for whoever debugs it, in the log and the trace, with sensitive parameters redacted.

An error that isn't a formal one is *unclassified*: a plain error, a `fmt.Errorf` wrap, another module's error, an `errors.Join`. `apperr.From` looks only at the outermost value, so an unclassified error renders as `internal_server_error`. That is deliberate: a failure nobody classified is ours until someone says otherwise, and its text (a diagnosis that may name internals) stays in the log.

Agents are among the main callers of anfra, and an agent reads `message` to correct its next call, so a message's quality is part of how the system works, not polish.

## Principles

**Know who must act.** Every error says which side is responsible: the person's input or state (`user`), the calling program (`client`), or anfra and what it depends on (`server`). That decides the wording, the status, whether the trace is marked as failed and whether anyone is alerted: a refused input is the system working, not an incident. Scope belongs to the code, and is decided again at every boundary, never copied: the same failure can be a client's mistake to the engine and a server bug to the platform that built the call ([at a platform's boundary](#at-a-platforms-boundary)).

**Write for each reader.** Give each reader its form rather than one string for all: a code to branch on, a message to show, typed details to act on, and the cause and steps for whoever debugs it.

**Capture context first; present it later.** Every formal step records itself on any error leaving it, so the error says what was being done, layer by layer, without anyone writing it at the failure ([steps](#steps-context-written-once)). *What* was being done (`context`, short fragments made to be chained) is kept apart from *why* it failed (`message`, a sentence), so each client presents them its own way: one line, a heading, a breadcrumb.

**A failure is told apart from an answer.** A failure is output too, as structured as an answer, but never mixed into one: no result with an error string inside it, no partial answer followed by prose. A caller knows which it got before reading either: from the HTTP status, or MCP's error flag. At the CLI, the answer is alone on stdout and the failure goes to stderr, so a pipe never carries a failure as data. The CLI's exit code says only whether the command succeeded: non-zero for a failure, and for an invalid verdict, so `anfra validate` can stop a CI job. What went wrong is in the documents, not in the exit code. Then the failure itself is a document in one form everywhere (code, scope, message, details), so a program branches on the code, never on text. The CLI renders it as it renders an answer: the envelope as YAML, with no format of its own for errors. Showing causes changes what a person can read, never what a program can rely on: the same failure has the same code and details whether causes are shown or hidden.

**Record every failure where it happens.** Every failure that is ours (server scope, or unclassified) is logged and traced by the process it happened in, whatever its caller is shown: anfra's own and each sidecar's. A sidecar logs its failure once, with its stack, where it answers it, and records it on its span ([sidecar failures](#sidecar-failures)); anfra logs it again where it ends, as the request's or the command's failure, with the whole chain. The trace id joins the two. Hiding a cause is about callers, never about the log: an operator who hides causes must still find them.

**Guard what isn't the reader's.** Causes can carry SQL, connection details, file paths and data. A formal error always shows its message and public steps; what it encapsulates is shown only where allowed. Each process decides for its own caller, by who that caller is, and each default can be changed:

- **The `anfra` binary** (the CLI and `anfra serve`) shows causes to its clients by default: on the user's own machine, the reader is the operator, who can fix it. An `anfra serve` serving others hides them with `ANFRA_HIDE_ERROR_CAUSES=1`.
- **The engine** hides them from its platform's clients by default, which is safe for a platform serving other people. A platform whose callers are its operators may show them, with `apperr.DisableErrorEncapsulation`.
- **A sidecar's** caller is its host, not the host's clients. Whether it sends its causes to its host is a setting of the sidecar's, set by whoever runs it ([sidecar failures](#sidecar-failures)). anfra is a host that can be trusted with them, so it turns the setting on for the sidecars it spawns, and a deployment running them for anfra does too. Whether a client then sees a sidecar's cause is anfra's decision, by the settings above, never the sidecar's: a sidecar knows neither its host's clients nor its host's settings.

Where causes are shown, they are shown in `message`, so the envelope has the same fields either way. An error encapsulated without a message of its own shows its cause as its message. One with its own message keeps it: its author judged it complete for the reader, so its cause stays in the log, shown or hidden.

Parameters a step declares sensitive are redacted even in the log ([security.md](security.md)). And every message is written to be safe for callers, whatever its code's visibility, since a translation at a boundary can carry it to one.

**Point to the fix when you can.** A refused input names each argument at fault, as a violation on that field, so a caller can fix exactly that and retry ([violations](#violations-errors-an-agent-can-fix)). Being actionable is welcome, not required: a plain, accurate message beats a guessed remedy.

**A code is a promise.** Codes are part of the contract: never repurpose one; add another. Mint a public code only when its callers would act on it differently; otherwise define an internal code that maps to an existing public one. A module's codes stay its own, translated once where another module calls it.

**Unknown means the server's fault.** An error nobody classified (a plain error, a `fmt.Errorf` wrap, another module's error) renders as `internal_server_error`, never as something milder. Unclassified fails closed.

**A chain means causation.** Wrap only what caused the failure. An error that was handled (a fallback taken, a retry that succeeded) is not chained into the next failure: it didn't cause it.

**A verdict isn't a failure.** "The query is invalid" is an answer; "the check could not run" is an error. A command that judges something answers its verdict as data (status `invalid`), so a caller can tell the two apart.

**Fail early, near the cause.** Inputs are checked against the schema, then the command's own rules, before anything starts, and a query is diagnosed before it reaches the warehouse, so errors are cheap and specific ([commands-and-api.md](commands-and-api.md)).

**Fail only what failed.** When parts are independent, finish the rest and report the broken part beside the result. A dataset that can't be shown is left out of `core.show`'s answer and reported in its diagnostics; the others still load.

**One request, one trail.** Every request served over HTTP or MCP gets an id, returned in the `Request-Id` header and in the error, logged on the request's one log line, and set on its root span as `h.request.id` (`shared/httpkit`). A user's report leads straight to what happened.

## Codes: public and internal, by namespace

- **Public codes** are a module's contract with its clients. Mint one only when a client should act differently from how it acts on every existing code; otherwise reuse a generic one.
- **Internal codes** (`DefineInternalCode`) let callers inside a module handle a finer case, and render as the public code they map to, carrying their message and details over.
- **Namespaces**: every code belongs to one, one per module (`apperr.DefineNamespace`), so modules name codes independently. A client sees the bare name (`not_found`); the log sees the qualified one (`apperr.not_found`, `anfra.sidecar_unavailable`).
- **The generic codes**, in `apperr`'s own namespace, are the few any platform serves: `internal_server_error`, `invalid_request` (the request's structure is wrong), `validation_failed` (well-formed, but its meaning is refused) and `not_found`. No other namespace may reuse their names.
- **The engine's codes** live in `internal/errcode` when no domain owns them (an unknown command, missing data permissions, an unavailable sidecar, a query the data source failed to run), and with their domain otherwise: `query_invalid` and its `QueryValidation` details are `internal/query`'s. The engine package re-exports them all, and `engine.ErrorCodes` lists them.

Codes carry no HTTP status. A status is a transport's concern, assigned per served code (below).

## Making an error

| Situation | Use |
|---|---|
| A failure that starts here | `apperr.New(code, msg)`, or `NewWith` for typed details. An empty `msg` uses the code's default. |
| An error from below, classified here | `apperr.Encapsulate(err, code, msg)` / `EncapsulateWith`. The inner error stays reachable to `errors.Is`/`As` and to the log, but is hidden from clients. |
| Another module's error, re-coded at a boundary | `apperr.Translate(err, code)` / `TranslateWith`. The same failure under the platform's code: its message and context stay visible, its code and details do not. |
| A bare failure with the default message | `return code` |

**Encapsulation is the default, and the safe one.** A client never sees what an error encapsulates, and an `internal_server_error` shows only its generic message. A local entry point whose user is its own operator calls `apperr.DisableErrorEncapsulation()` once at startup, so causes show where the error shows. The `anfra` binary does (`cmd/anfra/main.go`): the person looking at a failed chart is the one who can fix it. A platform serving other people must never call it.

## Violations: errors an agent can fix

Field-level problems are `apperr.Violations` details: one entry per field the caller got wrong, each with the field's name *as the caller sent it*, a code from a small generic set (`required`, `invalid`, `unknown`, `unsupported`, `too_short`, `too_long`, `taken`) and a message. Two codes carry them:

- `invalid_request`, scope `client`, for structure. An op's input is decoded against the schema of its type (`shared/apikit/internal/decode`), and every schema failure becomes a violation naming its field.
- `validation_failed`, scope `user`, for meaning: the value is well-formed but refused. A command raises one with `command.InvalidArg(field, code, msg)` (`internal/command/admission.go`).

Naming the field is what makes a failure fixable in one step, by a person or by an agent, rather than by guessing. The CLI goes one step further: `printError` (`cmd/anfra/errors.go`) maps each violation's field back to the flag or positional argument the user typed, and points to `--help`.

### Worked example: `anfra show`

anfra-node refuses a param it cannot use as a public error: JSON-RPC's `-32602`, with `data: {code, path?, details?}`, `path` naming the param ([anfra-node's errors design](https://github.com/holistics/holistics-core/blob/master/apps/anfra-node/docs/designs/errors.md)). Anything else it answers is its own failure, and stays unclassified. The show command turns a refused fqn into a violation of the argument at fault (`internal/command/show/show.go`):

```go
res, err := aml.Show(ctx, cc.Clients.Node, cc.Repo, typ, fqn)
if v, ok := refused(err); ok {
	return anfranode.ShowResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
}
```

where `refused` reads the public error's path (`fqn` or `type`) into an `apperr.Violation{Field: path, Code: "invalid", Message: …}`. So the sidecar's refusal reaches every caller as the same `validation_failed` with a violation on `fqn`:

```json
{"error": {"code": "validation_failed", "scope": "user", "status": 422,
  "message": "There's no dataset \"nosuch\" in the repo.",
  "details": {"violations": [{"field": "fqn", "code": "invalid", "message": "There's no dataset \"nosuch\" in the repo."}]},
  "request_id": "req_…"}}
```

and the CLI prints the same envelope as YAML on stderr, with the violation's `arg` (`fqn`, the positional) and a `help` that points to `anfra show --help`.

The query commands do the same with `anfranode.Violation` and a map of anfra-node's params to their args (`datasetFqn` to `dataset`, `input.…` to `input.…`, `internal/command/query/compile.go`): `anfra query -d nosuch …` fails with a violation on `dataset`, whose `arg` at the CLI is `--dataset`. A param they do not map is one the engine built, so its refusal is our failure. A query that does not compile is refused with its diagnostics (`query_invalid`), which `query.Compile` answers as anfra's `query_invalid`. Any other sidecar failure is left as it came, so it renders as unclassified, our failure.

## Steps: context written once

A step (`appstep.Define(ns, name)`) is a formal operation. Running it with `apptracing.Start` does two things at once: it opens a span in the trace, and, through `span.End(&err)`, adds the step to any error leaving it (`apperr.WithStep`), without changing the error's meaning. There is no ad hoc context: to add context, formalise the step.

A **public** step carries user-facing text and appears in the error's `context` on the wire ("importing dashboard X: …"); a **private** one appears only in the log and the trace. Every op is a step (`shared/apikit/registry.go`), so every op's failures say which op they came from.

`apptracing` marks a span as errored only for a **server**-scope failure. A user's invalid input or a client's missing argument is the system working, and marking it would bury the failures that need someone. Every failing span still carries `error.type`, the code, so the others stay findable.

## From error to caller

One error, rendered for each surface by the same pieces:

- **HTTP** (`anfra serve`, any platform using `apikit`): `httpkit.WriteError` renders `{"error": {code, scope, status, context, message, details, request_id}}` (`apperr.Envelope`), with the status of its *served* code. A server declares what it serves (`httpkit.Codes`): the generic codes, its own namespaces, and any library codes it imports one by one, each with a status. `anfra serve`'s list is `codes` in `cmd/anfra/serve.go`. A code the server does not serve fails closed: it renders as `internal_server_error`, with the original kept for the log.
- **MCP**: a failed tool call returns the same envelope as an error result (`shared/apikit/mcp.go`), so an agent reads the same code and violations over either transport.
- **CLI**: the envelope as a document on stderr, as an answer is a document on stdout, and a non-zero exit: YAML, or JSON with `--json` ([CLI output](commands-and-api.md#cli-output)). What helps a person or an agent fix the call stays in the document as fields: each violation also names the flag or positional argument the user typed (`arg: --dataset`), and `help` points to the command's `--help`. `help` is one string, one piece of advice a line: after a run of failures, an agent without the anfra skills also gets a line pointing to `anfra agent setup` ([agent setup](agent-setup.md#after-repeated-failures)). Otherwise it is the envelope as HTTP and MCP serve it, in its fields' order: the CLI adds to it and never removes or reorders a field (`request_id` stays, empty for a command run here), so a program that reads the wire's envelope reads the CLI's. A shown cause is in `message`, as it is there, not in a field of the CLI's own. A call forwarded to a running server prints that server's envelope the same way, as its body carried it.

  **stderr is a stream of documents,** multiple YAML documents, or with `--json` JSONL documents (one JSON document a line), so a program reads it with any YAML library (`yaml.safe_load_all`, `yq`) or JSONL reader (`jq`) rather than by cutting text. Everything anfra writes there is a document whose one top-level key says what it is: `warning` (a problem with the operator's own setup that did not stop the command, such as telemetry that could not start, or a running `anfra serve` of another version than the CLI's), each with a code and a message, and at most one `error`, the last. A program takes the `error` and can skip the warnings without knowing them. Two things stay out of the stream:
  - **What is for a person:** notices, such as an available update; a command's progress ("Downloading…", "Publishing…"); and its questions. They are printed only when stderr is a terminal, where no program reads it, and not with `--non-interactive`. Advice, what to do next, is in the result's `hints`, never here.
  - **Log lines** (`ANFRA_LOG_STDERR=1`, JSON lines) are for debugging, not part of the contract: a program reading stderr leaves the setting off, which is the default for every command but `anfra serve`, whose errors are its HTTP answers, so nothing parses its stderr. Every record is in `anfra.log` either way.

  ```yaml
  error:
    code: validation_failed
    scope: user
    message: There's no dataset "nosuch" in the repo.
    details:
      violations:
        - field: dataset
          arg: --dataset
          code: invalid
          message: There's no dataset "nosuch" in the repo.
    help: Run `anfra query --help` for usage.
  ```

  With `--json`, the same error is one line: `{"error":{"code":"validation_failed","scope":"user",…}}`.
- **The log**: `httpkit` logs one line per request with the code, the scope and the whole chain (`err.Error()`, sensitive step parameters redacted). The level follows the scope: `user` is info, `client` warn, `server` error. A sidecar logs its own failures as well ([sidecar failures](#sidecar-failures)).
- **The trace**: the span of each step it left, as above.
- **The OpenAPI document**: each op lists the codes it can fail with, and the details type of each, so the contract is published, not only described.

## In a Data App

A Data App is a fourth surface, reached through the SDK rather than one of anfra's doors: its definition calls the core API through the host page, over the bridge ([data-apps.md](data-apps.md), [sdk-plugins.md](sdk-plugins.md)). The same principles hold, with the browser in place of a process.

**Its readers.** The definition's own code, which decides what the person viewing the app sees; that person, who sees only what the code shows; and the agent or author building the app, who reads `query.error` to fix the next try, often with no console to look at.

**One shape, the API's** (`common/errors.ts`). A Data App's error is the document the API answers, not a class picked for it: every failure of a call is a `DataAppError` with the envelope's fields, `scope` and `message` always, `code`, and `status`, `details`, `context` and `requestId` where the backend has them, plus the SDK's own `entity`, the query or control it concerns. An app branches as any other caller does: on `scope` to know who must act, on `code` to know what happened, and reads `details` to point at it (a diagnostic's line, a row limit). Nothing re-maps it, so a code a host adds reaches the app as it is, by its scope. A query's error is stored on the query, never thrown: one failed query must not blank the rest.

The one other class is `ValidationError`, for what the definition declares wrong. It is thrown synchronously, at the line that declared it, never comes from a backend, and has the same shape (`declaration_invalid`, scope `client`), so code that reads `scope` and `code` treats every error alike.

**Generic codes, for every backend.** A backend that is not anfra's (a host that runs the SDK over its own API, with an HTTP status and a message but no envelope) cannot speak anfra's codes. So the SDK names the few cases an app must tell apart on any backend, and every backend uses them where they apply, as `apperr`'s generic codes serve every platform. Each reuses an anfra or Cloud code where one exists, so those backends pass their own errors through:

| Code | Scope | Means |
|---|---|---|
| `query_invalid` | user | The query was refused before it ran. |
| `query_failed` | user | It ran, and its data source failed it. |
| `forbidden` | user | This reader may not see it or do it. |
| `unauthenticated` | user | The reader's session is gone: sign in again. |
| `invalid_request` | client | The app's call was malformed: a plugin refused it. |
| `declaration_invalid` | client | The definition declared something wrong (`ValidationError`). |
| `connection_failed` | server | The server could not be reached, or its answer was cut or garbled. |
| `call_failed` | server | The host failed the call, for a reason the frame is not shown. |
| `internal_server_error` | server | The server failed. |

Any other code (anfra's `result_too_large` and `page_too_large`, Cloud's own) is the backend's, with its scope. An app that branches on `scope` and the generic codes runs on any host; one that reads a backend's own codes runs best on that host. The SDK's types name the generic and anfra's codes, each with its `details` type, generated from the API's schema.

**A backend answers in the shape** (`Backend` in `common/types.ts`). anfra's passes the API's envelope through (`api/errors.ts`). One without an envelope maps what it has, by a table the SDK publishes and a helper that applies it (`fromHttpStatus`): 401 is `unauthenticated`, 403 `forbidden`, any other 4xx the refusal of what was called (the caller says which: `query_invalid` for a query, `invalid_request` by default), 5xx `internal_server_error`, no answer `connection_failed`. A backend that rejects with anything else has failed in a way nobody classified: it is `call_failed`, scope `server`, since unknown means the server's fault.

**Decorated, not rewritten.** The SDK keeps the document and adds to it. Where the facts are in `details` (a refused query's diagnostics, a refused input's fields) and the message is only its code's sentence ("The query is invalid."), the SDK writes them into `message` (`line 1:9: No field x.`), so a message read alone still says what is wrong; `details` stays as it came.

**One request, one trail, into the frame.** Every error the server answered carries its `requestId`, kept when it crosses the bridge. With it, the person or agent who sees the error finds its line in the server's log, where the cause is whether or not it was shown. It is the one identifier allowed into the frame: it names a request, not anything in it, the server already sends it to every caller, and only someone who can read the log can turn it into anything.

**What crosses the bridge.** The frame runs code anyone may write, so only the SDK's own errors cross with their fields, and only the fields the shape names (`common/endpoint.ts`): `context` as each step's `text`, never its params, which can name a dataset or a path; `details` only for a code the SDK knows the type of; a `scope` the SDK does not know as `server`. Anything else (a bug in the host, a middleware's error, a library's) crosses as `call_failed`, since its message may hold a URL, a resource's id or a server's error. Two rules keep that rare and findable:

- **The SDK classifies its own failures.** A stream the server cut, or a line in it that is not an event (a proxy's error page), is `connection_failed` saying so (`api/stream.ts`, `api/backend.ts`), not an unclassified error. What remains `call_failed` is a real bug in the host.
- **A hidden error is recorded where it was hidden.** The host reports the original as a runtime error before answering `call_failed` (below), and the frame logs one it hides to its own console: *every failure leaves a record*, in the page where it happened.

**Fail only what failed.** A `subscribe()` listener is the definition's own code drawing what changed. An exception it throws is logged to the frame's console and stops nothing else: the other listeners still run, and the query whose change it was told of keeps its result (`app/observable.ts`). An author's bug can break its own drawing, never the app's data.

**Every runtime error reaches the host.** What goes wrong while the app runs is reported to its host whether or not anyone inspects it (`app/reporting.ts`, `common/runtimeErrors.ts`), as one event, `anfra.error`, of four kinds:

| Kind | What | Reported by |
|---|---|---|
| `query` | A query's run ended in `error` (`execute()`). | the frame |
| `uncaught` | The definition's uncaught error or unhandled rejection, a `ValidationError` from a declaration no one caught included; and what the SDK catches to keep the app running: a menu item's action, a plugin's setup or drawing, a frame call that failed. | the frame |
| `listener` | A `subscribe()` listener's exception. | the frame |
| `hidden` | A call the host failed and showed the frame only as `call_failed`, with the original's message and stack. | the host |

Each carries the error's shape (`code` and `scope` when it is one of the SDK's, `message`, `requestId`, `entity`), the time and the mount, and for `uncaught`, `listener` and `hidden` a truncated stack; never rows or a request's params. An error the app handled itself is not reported: a `fetchMore()` or an export rejects to its caller, and only reaches the host if no one catches it. The host checks each event, since the frame's word is not trusted: a frame cannot claim a `hidden` error, and each string is capped. The frame sends at most 120 errors a minute, and 120 console lines apart from them, so a render loop that throws or warns on every frame cannot flood its host, and its warnings cannot crowd out its errors; how many it did not send it says in its own console when the minute is over. The host holds each mount to the same, since a frame can post events without the SDK. The SDK's own console notes (`anfra: …`) are not sent as console lines, so each failure that writes one is also reported as an error.

A host receives them in one callback, `createHost({ onError })`, which defaults to the page's console, and per mount as `mounted.on('anfra.error')`. A console line is not an error: the app's `console.error` and `console.warn` lines are `anfra.console`, sent only when the host captures them (`createHost({ captureConsole })`), since they can hold the app's data. Unset, a development build of the host's page captures them and a production build does not (`process.env.NODE_ENV`).

Where they go is each host's choice. On `anfra serve`:

- **The panel** lists them among its problems, with the static ones (markup and wiring found without running the app), the errors first; one naming a query or control links to it, any other opens to its stack. Console lines are in a section of their own.
- **anfra.log**: the appserve page captures console lines and posts both to `POST /appserve/log`, which writes each with `"service":"app"` and the app's path ([data-apps.md](data-apps.md#appserve-what-anfra-serve-adds)). An agent reads them without a browser: `anfra status` names the log (`repo.logs`), and a `"service":"app"` line's `request_id` finds the server's line with the cause.

## At a platform's boundary

A library's codes are its own contract, not its platform's. Where a platform calls the engine, it decides what each engine error means *to its own clients*: it translates the engine's codes to its own (`apperr.Translate`, which re-decides the scope), serves some of them as they are (`httpkit.Codes.Imported`), and lets the rest fail closed. The engine never decides a platform's status codes or messages. See [engine.md](engine.md).

## Sidecar failures

A sidecar is part of anfra's server side, so its failures are anfra's to record and to classify ([sidecars.md](sidecars.md)). It answers one of two kinds:

- **A refusal it classified**: anfra-node's public errors (`-32602` with `data.code`), and what canal-query scopes `User` (the data source refused the query, or could not be reached). The command or domain function translates it into anfra's code, as in [the worked example](#worked-example-anfra-show).
- **Its own failure**: anything else. The sidecar logs it with its stack, records it on its span, and answers its host the failure's message and its cause chain, never its stack. anfra leaves it unclassified, so it is an `internal_server_error` whose text is the sidecar's cause: shown or hidden by anfra's setting, and in anfra's log either way.

The cause travels apart from any text a host may make public. canal-query's `message` becomes `query_failed`'s message (`asQueryFailed` in `internal/command/query/run.go`), so its cause comes in a field of its own, `cause`, which `canalquery.Error` keeps out of `Message`. canal-query sends it only when `EXPOSE_ERROR_CAUSES=1`, because its other caller, the Holistics app, shows canal's messages to end users and is not set up to keep a cause from them. anfra sets it on the canal-query it spawns; a deployment that runs canal-query for anfra sets it too. anfra-node has one kind of caller, a host like anfra, so it needs no setting: it always answers its own failure as `-32603` with the failure's message.

## Adding an error

1. **Reuse first.** A refused argument is `validation_failed` with a violation, through `command.InvalidArg` or `EncapsulateWith`. A missing thing is `not_found`. Our own failure needs no code at all: leave it unclassified, or `Encapsulate` it as `internal_server_error` with a diagnosis for the log.
2. **Mint a public code** only when a client should act differently. Put it with its domain, or in `internal/errcode` if it is the engine's as a whole; give it the scope that is honestly responsible and a default message about the user's situation; give it typed details if a client needs data to act on, rather than text to parse.
3. **Give it a status** in every server that serves it (`codes` in `cmd/anfra/serve.go` for `anfra serve`). An unserved code still answers, as `internal_server_error`.
4. **Name the operation** it happens in as a step, if a caller would want to know where it failed.

## Known gaps

Where the code does not follow these rules yet:

- **A Data App's errors say who must act, not whether to retry.** Scope `server` covers both a server that will be back and a bug; nothing yet tells an app to retry.
- **A backend's own message reaches the frame as it is.** The SDK trusts what a backend puts in `message`: anfra's are written to be safe for callers, but another backend's may not be (a job's error log can hold SQL).
- **An error loses its `entity` crossing the bridge.** The frame names it again where it can (a query's error, an export's), not for a plugin's own call.
- **OpenTelemetry's own messages are text lines on stderr.** Its SDK logs a problem it meets (an endpoint it cannot parse) through its own logger, before the repo's log exists, so a YAML or JSON parser refuses the stream there.

Related: [commands-and-api.md](commands-and-api.md) (how ops are defined and published), [engine.md](engine.md) (the boundary platforms translate at), [security.md](security.md) (why causes are hidden by default).
