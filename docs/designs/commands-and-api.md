# Commands and the core API

Every capability anfra exposes is a **command**, defined once. That one definition becomes the CLI command, the HTTP operation, the MCP tool, the OpenAPI entry and the types clients are generated from. Nothing about a command is declared twice, so the surfaces cannot disagree: a person at the terminal, a program over HTTP and an agent over MCP call the same thing, validated the same way, answered in the same shape, failing with the same codes.

## One definition

A command is a `command.Def` (`internal/command/command.go`), checked by `command.Define`:

- **`Name`**, dotted by group: `query.compile` is the CLI's `anfra query compile` and the op `core.query.compile`.
- **`In`**, a struct whose fields are the args. Their tags drive both the op's JSON Schema (read by huma) and the CLI's flags (`json`, `doc`, `enum`, `default`, `group`; and for the CLI alone `short`, `alias`, `cli:"positional"` or `cli:"stdin"`). The full list is on `command.Arg` (`internal/command/args.go`). An input with args returns `command.ArgsSchema` from its `TransformSchema`, so the schema also states what the tags alone cannot: required strings and "exactly one of" groups.
- **`Out`**, the answer, and **`Valid`**, for a command whose job is to judge something (a validator, a health check). An answer it judges invalid is still an answer, carrying its verdict, and reported as status `invalid`, not as an error. The CLI exits non-zero on it.
- **`Check`**, for rules between args the schema cannot state; it runs before anything starts.
- **`Needs`**, the sidecars this input requires, so the one-shot CLI spawns only those. Under `anfra serve` they are all warm anyway.
- **`Errors`**, the codes `Run` itself fails with. The codes every command can fail with (undecided permissions, invalid args, and `sidecar_unavailable` when it needs one) are added for it. See [errors.md](errors.md).
- **`ReadOnly`**, **`Idempotent`** and **`Timeout`**, published on the op.

`Define` parses the tags at startup and panics on a malformed definition, so a bad command never ships half-described.

Commands live in packages under `internal/command/` (one per group), and are registered in order in `internal/app/commands.go`. That list is the registry every surface reads.

## The surfaces

| Surface | Built by | What it is |
|---|---|---|
| CLI | `cmd/anfra/commands.go` | `anfra <group> <name>`: flags from the args, output as YAML. When the repo has a running `anfra serve`, the CLI sends the call there instead of starting its own sidecars ([architecture.md](architecture.md)). |
| HTTP | `shared/apikit` `HTTP` | `POST /api/core.<name>`, a JSON body in, the answer or an error envelope out. |
| MCP | `shared/apikit` `MCP` | One tool per op, named like it, at `/mcp`. |
| In-process | `engine.Dispatch` | For a platform embedding anfra ([engine.md](engine.md)). |
| Discovery | `shared/apikit` `Discovery` | `GET /api/ops`, `/api/ops?group=…`, `/api/ops/<name>`; and over MCP, the `list_operations` and `describe_operation` tools. |

Discovery serves the same contract the operations do, filtered to what the caller may call. An agent can find out what anfra can do, and how to call it, without leaving the API. Each operation's usage names its args, which of them are required, their allowed values, and the codes it can fail with.

## The contract: `api/openapi.yaml`

The OpenAPI document generated from the registry is committed, and is the core API's contract. It is what clients see, and what changes to it are reviewed against.

```sh
scripts/openapi.sh generate          # write api/openapi.yaml (from `anfra openapi`; no repo or sidecars needed)
scripts/openapi.sh diff              # fail if the committed file is stale
scripts/openapi.sh breaking <base>   # fail if it breaks the base document
```

Run `generate` after changing anything a command publishes: a command added, removed or renamed, its input or output types, its declared errors, or a code. CI (`.github/workflows/continuous_integration.yml`, the `api` job) fails on a stale file, and refuses a breaking change unless a commit in the pull request declares it (`feat!:`, or a `BREAKING CHANGE:` footer). A breaking change reaches every client at once: Data Apps, the SDK, agents and every platform serving these ops. It has to be a decision, not a side effect.

The document's version is the contract's, not the binary's: it does not change with every release.

## Clients from the contract

The SDK's client for the core API, `anfra-sdk/api`, is typed by `web/sdk/src/api/schema.d.ts`, generated from `api/openapi.yaml` (`pnpm --filter anfra-sdk generate`). CI fails when it is stale. A change to a command's input or output therefore reaches TypeScript callers as a type error, not at runtime. Any other client can be generated from the same document.

## Why it is shaped this way

- **One definition, every surface.** A flag that exists on the CLI but not over HTTP, or an error the API returns but the CLI cannot show, is impossible by construction.
- **One interface, one body of knowledge.** The CLI command, the HTTP op and the MCP tool share their name, arguments, answer and errors, so what an agent knows about anfra holds however it reaches it. A coding agent with a terminal runs `anfra query --dataset sales …`; a web-based agent with no shell calls the `core.query` tool over MCP, with the same fields; a program posts the same input to `/api/core.query`. Skills, docs and examples written for one apply to the others, translated mechanically rather than rewritten, so agents, local or web-based, with CLI access or without, share the same skills and knowledge.
- **Programs and agents are first-class callers.** Inputs are typed and validated before anything runs, and a refusal names each offending arg (its violations). Answers are structured, an invalid result is distinguished from a failure, and every operation describes itself. An agent can call, read the refusal, fix its input and retry in one short loop.
- **The contract is reviewed, not inferred.** Committing the generated document turns every change to the API into a visible diff, and makes breaking ones explicit.
- **Platforms serve the same API.** `engine.Ops()` hands a platform these same operations, so the core API is identical on `anfra serve`, anfra-cloud, or any other platform ([engine.md](engine.md)).
