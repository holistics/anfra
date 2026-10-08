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

**Guard what isn't the reader's.** Causes can carry SQL, connection details, file paths and data. A formal error shows its message and public steps, and an encapsulated cause stays in the log; parameters a step declares sensitive are redacted even there. `anfra` shows causes by default, because on the user's own machine its user is the operator; an `anfra serve` serving others hides them with `ANFRA_HIDE_ERROR_CAUSES`, and a platform embedding the engine hides them by default ([security.md](security.md)). And every message is written to be safe for callers, whatever its code's visibility, since a translation at a boundary can carry it to one.

**Point to the fix when you can.** A refused input names each argument at fault, as a violation on that field, so a caller can fix exactly that and retry ([violations](#violations-errors-an-agent-can-fix)). Being actionable is welcome, not required: a plain, accurate message beats a guessed remedy.

**A code is a promise.** Codes are part of the contract: never repurpose one; add another. Mint a public code only when its callers would act on it differently; otherwise define an internal code that maps to an existing public one. A module's codes stay its own, translated once where another module calls it.

**Unknown means the server's fault.** An error nobody classified (a plain error, a `fmt.Errorf` wrap, another module's error) renders as `internal_server_error`, never as something milder. Unclassified fails closed.

**A chain means causation.** Wrap only what caused the failure. An error that was handled (a fallback taken, a retry that succeeded) is not chained into the next failure: it didn't cause it.

**A verdict isn't a failure.** "The query is invalid" is an answer; "the check could not run" is an error. A command that judges something answers its verdict as data (status `invalid`), so a caller can tell the two apart.

**Fail early, near the cause.** Inputs are checked against the schema, then the command's own rules, before anything starts, and a query is diagnosed before it reaches the warehouse, so errors are cheap and specific ([commands-and-api.md](commands-and-api.md)).

**Fail only what failed.** When parts are independent, finish the rest and report the broken part beside the result. A dataset that can't be shown is left out of `core.show`'s answer and reported in its diagnostics; the others still load.

**One request, one trail.** Every request served over HTTP or MCP gets an id, returned in the `Request-Id` header and in the error, logged on the request's one log line, and set on its root span as `request.id` (`shared/httpkit`). A user's report leads straight to what happened.

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

anfra-node refuses an fqn it found nothing at as an RPC error with a path. The show command turns that into a violation of the argument at fault (`internal/command/show/show.go`):

```go
res, err := aml.Show(ctx, cc.Clients.Node, cc.Repo, typ, fqn)
if v, ok := refused(err); ok {
	return anfranode.ShowResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
}
```

where `refused` reads the RPC error's path (`fqn` or `type`) into an `apperr.Violation{Field: path, Code: "invalid", Message: …}`. So the sidecar's refusal reaches every caller as the same `validation_failed` with a violation on `fqn`:

```json
{"error": {"code": "validation_failed", "scope": "user", "status": 422,
  "message": "No dataset \"nosuch\" in the repo.",
  "details": {"violations": [{"field": "fqn", "code": "invalid", "message": "No dataset \"nosuch\" in the repo."}]},
  "request_id": "req_…"}}
```

and the CLI prints it as `Error: fqn: no dataset "nosuch" in the repo`, then ``Run `anfra show --help` for usage.`` Any other sidecar failure is left as it came, so it renders as unclassified, our failure.

## Steps: context written once

A step (`appstep.Define(ns, name)`) is a formal operation. Running it with `apptracing.Start` does two things at once: it opens a span in the trace, and, through `span.End(&err)`, adds the step to any error leaving it (`apperr.WithStep`), without changing the error's meaning. There is no ad hoc context: to add context, formalise the step.

A **public** step carries user-facing text and appears in the error's `context` on the wire ("importing dashboard X: …"); a **private** one appears only in the log and the trace. Every op is a step (`shared/apikit/registry.go`), so every op's failures say which op they came from.

`apptracing` marks a span as errored only for a **server**-scope failure. A user's invalid input or a client's missing argument is the system working, and marking it would bury the failures that need someone. Every failing span still carries `error.type`, the code, so the others stay findable.

## From error to caller

One error, rendered for each surface by the same pieces:

- **HTTP** (`anfra serve`, any platform using `apikit`): `httpkit.WriteError` renders `{"error": {code, scope, status, context, message, details, request_id}}` (`apperr.Envelope`), with the status of its *served* code. A server declares what it serves (`httpkit.Codes`): the generic codes, its own namespaces, and any library codes it imports one by one, each with a status. `anfra serve`'s list is `codes` in `cmd/anfra/serve.go`. A code the server does not serve fails closed: it renders as `internal_server_error`, with the original kept for the log.
- **MCP**: a failed tool call returns the same envelope as an error result (`shared/apikit/mcp.go`), so an agent reads the same code and violations over either transport.
- **CLI**: `printError` (`cmd/anfra/errors.go`) prints the context and message on one line, the violations by argument, other details as YAML, then the cause. A call forwarded to a running server prints that server's envelope the same way.
- **The log**: `httpkit` logs one line per request with the code, the scope and the whole chain (`err.Error()`, sensitive step parameters redacted). The level follows the scope: `user` is info, `client` warn, `server` error.
- **The trace**: the span of each step it left, as above.
- **The OpenAPI document**: each op lists the codes it can fail with, and the details type of each, so the contract is published, not only described.

## At a platform's boundary

A library's codes are its own contract, not its platform's. Where a platform calls the engine, it decides what each engine error means *to its own clients*: it translates the engine's codes to its own (`apperr.Translate`, which re-decides the scope), serves some of them as they are (`httpkit.Codes.Imported`), and lets the rest fail closed. The engine never decides a platform's status codes or messages. See [engine.md](engine.md).

## Adding an error

1. **Reuse first.** A refused argument is `validation_failed` with a violation, through `command.InvalidArg` or `EncapsulateWith`. A missing thing is `not_found`. Our own failure needs no code at all: leave it unclassified, or `Encapsulate` it as `internal_server_error` with a diagnosis for the log.
2. **Mint a public code** only when a client should act differently. Put it with its domain, or in `internal/errcode` if it is the engine's as a whole; give it the scope that is honestly responsible and a default message about the user's situation; give it typed details if a client needs data to act on, rather than text to parse.
3. **Give it a status** in every server that serves it (`codes` in `cmd/anfra/serve.go` for `anfra serve`). An unserved code still answers, as `internal_server_error`.
4. **Name the operation** it happens in as a step, if a caller would want to know where it failed.

Related: [commands-and-api.md](commands-and-api.md) (how ops are defined and published), [engine.md](engine.md) (the boundary platforms translate at), [security.md](security.md) (why causes are hidden by default).
