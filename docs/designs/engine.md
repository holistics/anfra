# The engine: anfra as a library

The `engine` package is anfra's public Go API. Any Go program can import it to run anfra in-process: a BI platform, an internal data portal, an agent runtime, a server of any kind. The importing program, the **platform**, brings what anfra deliberately leaves out: who its users are, what each may do, how calls reach it (HTTP, MCP, a queue, a UI of its own), and what it builds around the results. anfra-cloud is one such platform today. The engine is shaped for any of them, not for anfra-cloud in particular.

This doc is the contract between anfra core and its platforms: what the engine promises, what it refuses to do, and what a platform must bring. [usage/engine.md](../usage/engine.md) shows it in code.

## The surface

The public packages are `engine` and `shared/*` (`apperr`, `appstep`, `apptracing`, `apikit`, `httpkit`, `requestid`): the error and API frameworks the engine's operations are built on, which a platform builds its own on too. Everything under `internal/` is not part of the contract and may change in any release.

`engine` is small on purpose (its package comment, `engine/engine.go`, says why). Everything the engine can do is reachable through one call, and the rest of the package exists to build that call:

| | |
|---|---|
| `Connect(ctx, nodeURL, canalQueryURL, …)` | dial the sidecars, which run on their own ([sidecars.md](sidecars.md)) |
| `OpenRepo(dir)` | resolve a repo: its identity and where its state lives |
| `Unrestricted()`, `Restricted(attrs)` | state what the caller may see |
| `Dispatch(ctx, inv, req)` | run one command |
| `Ops()` | every command as an `apikit` op (`core.<command>`), to serve as it is |
| `Describe()`, `Commands()`, `ErrorCodes()` | what there is, for a platform to publish and to test against |

The types (`Request`, `Response`, `Invocation`, `Repo`, `DataPerms`, …) are aliases of the internal ones, not wrappers: nothing converts between them, so nothing can drift.

## What the engine promises

**The same semantics as the CLI.** The `anfra` CLI, `anfra serve` and every platform reach commands through the same registry (`internal/app`) and the same `Dispatch`. A query means the same thing, validates the same way and fails with the same codes wherever it runs. A platform never re-implements a command; it decides who may call it. See [commands-and-api.md](commands-and-api.md).

**A contract a platform can check after every upgrade.**
- `Commands()` lists every command, so a platform that maps commands to its own permissions can fail its build when an upgrade adds one it has not decided on, rather than silently exposing it. anfra-cloud does this (`TestEveryCommandIsDecided`).
- `ErrorCodes()` lists every code `Dispatch` can fail with, so a platform can test that it translates each one.
- `Ops()` carries each operation's name, input and output schemas, codes, and whether it is read-only. A platform that serves them as they are serves the same core API as `anfra serve`, so a client, a Data App or an agent meets one contract on every platform.

**Errors with meaning.** Every failure carries a code, a scope (whose fault it is) and typed details, under the engine's namespace (`engine.Namespace`). A platform translates them into its own codes where it calls the engine (`apperr.Translate`), or imports them as its own (`httpkit.Codes.Imported`). Anything unclassified is an internal error, never a leak. See [errors.md](errors.md).

**Sidecars at the platform's addresses.** `Connect` dials anfra-node and canal-query where the platform runs them, as containers with their own lifecycle, restarts and scaling. It never starts a process: that is how the one-shot CLI works, and the wrong shape for a server. `WithTransport` lets the platform wrap each sidecar's HTTP transport (with `otelhttp`, say), so sidecar calls appear in the platform's traces, without the engine taking a tracing dependency.

## What the engine refuses to do

**Make access decisions.** There is no principal, role, policy or tenant anywhere in the engine. The platform decides whether a caller may run a command before calling `Dispatch`. What crosses the boundary is the *consequence* of that decision:

- **`DataPerms`**: the data restrictions that apply to this caller. They are decision-bearing: the compiler reads the attributes, and changing one changes which rows come back. They cannot be left unset: `Dispatch` refuses an `Invocation` whose `DataPerms` nobody decided (`DataPermsMissing`), so "no restrictions" is something a platform states (`Unrestricted()`), never something it defaults into. A query that cannot apply restrictions (raw SQL) refuses `Restricted` ones (`DataPermsUnenforceable`).
- **`Attribution`**: who the caller is, for logs, spans and audit. It is baggage and nothing else: no command reads it and it never reaches the compiler. A value that must change the rows belongs in `Attributes`, and the engine keeps the two apart.

`Invocation` is the trusted half of a call: built server-side, never decoded from a request body, or a caller could assert their own permissions. `Request` (a command name and its args) is the untrusted half.

**Grow by accident.** Adding to `engine` is a deliberate act. If a platform needs something from `internal/`, the answer is an issue and a considered addition, not a fork or a reach past the boundary.

## What a platform brings

- **Identity and authorization**: who is calling, and whether they may run this command on this input. `apikit.Mount` gives each op a host-side `Admit`, `Authorize` and `Bind` around anfra's own validation and handler.
- **The invocation**: the repo the call acts on, the sidecar clients, the `DataPerms` derived from the caller, and `Attribution` for its logs.
- **The transport**: HTTP, MCP or its own, with its own middleware.
- **The sidecars**: running, reachable, and released with the versions this anfra pins (`manifest.yml`; see [release.md](release.md)).
- **Its own codes and statuses**: either translating the engine's or importing them.
- **Everything around the results**: users, sharing, hosting, history, a UI. That is the platform's product; see [philosophy.md](philosophy.md) for where core stops.

## Keeping the contract

Changes to anfra core are judged by their effect on platforms:

- An op's name, schemas and codes are the core API. A breaking change to them is declared and versioned ([commands-and-api.md](commands-and-api.md)).
- A new command, error code or `engine` symbol is an addition platforms will see through `Commands()`, `ErrorCodes()` and their builds; that is the intended way for them to notice.
- Nothing about who may do what moves into the engine. If a feature seems to need it, the engine takes the consequence (as `DataPerms` does) and the platform keeps the decision.
