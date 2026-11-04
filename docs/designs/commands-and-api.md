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
| CLI | `cmd/anfra/commands.go` | `anfra <group> <name>`: flags from the args (an object arg as JSON, or its fields as flags by their path, `--format-options.header`), output as YAML, JSON or pretty ([CLI output](#cli-output)). When the repo has a running `anfra serve`, the CLI sends the call there instead of starting its own sidecars, unless the command is CLI-local ([cli-vs-warm-server.md](cli-vs-warm-server.md)). |
| HTTP | `shared/apikit` `HTTP` | `POST /api/core.<name>`, a JSON body in, the answer or an error envelope out. |
| MCP | `shared/apikit` `MCP` | One tool per op, named like it, at `/mcp`. |
| In-process | `engine.Dispatch` | For a platform embedding anfra ([engine.md](engine.md)). |
| Discovery | `shared/apikit` `Discovery` | `GET /api/ops`, `/api/ops?group=…`, `/api/ops/<name>`; and over MCP, the `list_operations` and `describe_operation` tools. |

Discovery serves the same contract the operations do, filtered to what the caller may call. An agent can find out what anfra can do, and how to call it, without leaving the API. Each operation's usage names its args, which of them are required, their allowed values, and the codes it can fail with.

### CLI output

Every command writes structured output, except the few that write an artifact (below). (In docs, say *structured output* for what a command writes, as against prose; say *document* only beside YAML or JSON, where the format is the point.) Structured output comes in three formats:

- **YAML** (`--yaml`): a core command's default. Read by a person or an agent, and by YAML tools. Indented by 2, as most YAML is: a deep answer costs its reader fewer characters.
- **JSON** (`--json`): for tools and languages that read only JSON.
- **Pretty**: the pretty format, laid out for a person, with tables and sections ([cli-pretty.md](cli-pretty.md)). A CLI-only command's default, as people run them more than programs do; no flag asks for it yet. Not for a program: it can truncate, and its layout may change.

`--yaml` or `--json` works on every command, so an agent or a script passes one and never depends on a command's default. Whatever the format:

- **`--schema` describes a command instead of running it:** `command`, then its `input` and `output`, each a JSON Schema (2020-12), inlined as discovery inlines an op's usage (`/api/ops/<name>`, MCP's `describe_operation`), with a type that recurs kept once in `$defs`. A core command's come from its op; a CLI-only command's from its flags and its result type. Each input field is keyed by its name, snake_case, and is a flag, `--<field>`, in kebab or snake case (`--data-source`, `--data_source`; an object's field as its path, `--format-options.header`), unless its `x-cli` says otherwise, as the op's `cli` tag does: `positional`, and `stdin` when it's read from stdin unless given (`query`'s `query`). `--help` stays the first look, for a person or an agent; `--schema` is the contract when exactness matters, and costs more to read.
- **`--jq <expr>` filters a structured answer before it is written,** with jq built in (gojq, as `gh --jq`): what it gives is written as YAML, or JSON with `--json`, and when every result is a scalar, a plain line each, as `jq -r` writes them, for a shell. It applies to `--schema` too: `--schema --jq '.output | del(.. | .description?)'` is the output alone, without descriptions. A filtered object's keys come out sorted (gojq's objects are maps). An expression that doesn't parse is refused before the command runs; an artifact refuses it, as it refuses `--json`.
- **A command's `--help` says what its result means,** in a sentence ("Its result is `anfra version`'s answer, checked after the update."), and its fields are `--schema`'s: their descriptions live once, on the result's type (`doc` tags), as an op's do.
- **A field's description is one clause on what it means:** what the name and type don't say ("every number a string, with every digit"), not how it's made. Descriptions are a schema's largest part after its structure, read by every agent that asks; the reasoning behind a field belongs in a design doc.

- **stdout holds the result alone.** A core command's is its op's answer, as HTTP returns it.
- **A CLI-only command's result is structured too.** It has no op, so the command defines the result's shape itself (a Go type in `cmd/anfra` or the package it calls), which `--schema` shows. It is in no published schema and CI checks no change to it, but scripts rely on it, so it changes as a CLI-only command's flags do: with a deprecation period ([deprecation before removal](#deprecation-before-removal)).
- **A CLI-only command's result has `message` and `hints`.** `message`, its first field, says in a sentence where things stand, so a person or an agent grasps the outcome before the details: "Updated anfra 0.16.0 → 0.17.0." It's best built from the result's own fields, so it agrees with them. `hints`, its last, is what to act on next, one a line, absent when nothing is, as `anfra version`'s are: "Put your database in .anfra/data_sources.yml, …". Advice goes in `hints`, never in prose on stderr, which an agent doesn't see off a terminal.
- **stderr holds the rest, as documents:** each warning, then at most one error, last ([errors.md](errors.md#from-error-to-caller)), each with one top-level key saying what it is: `warning` or `error`. They're YAML documents whatever stdout's format, pretty included, and JSONL with `--json`, so a program reads stderr the same way whichever command ran. Progress for a person ("Downloading…") is the one exception: on a terminal only.
- **The exit code** is non-zero for a failure and for an invalid verdict, whatever the format.
- **Questions, and notices for a person,** come only on a terminal, and not with `--non-interactive`: a command that would ask refuses instead, as it does without a terminal. `--no-interactive` is accepted as the same flag. Whether a run asks is the run's, not the command's: a command that asks on a terminal still writes structured output, and the format has no say in it (`--json` on a terminal still asks).

| | YAML (`--yaml`) | JSON (`--json`) | Pretty |
|---|---|---|---|
| Default for | core commands | none | CLI-only commands |
| For | a person or an agent reading it, and YAML tools (`yq`) | tools and languages that read only JSON (`jq`) | a person |
| stdout | the result as a YAML document | the result as one JSON document | the result laid out for reading: `message`, sections, tables, `hints` |
| stderr | multiple YAML documents, each after `---` | JSONL: one JSON document a line (indented, in [`anfra console`](console.md)) | as YAML |
| `openapi` | the OpenAPI document as YAML | the OpenAPI document as JSON | as YAML |

**Artifacts are the exception.** A few commands write something other than a result to stdout, and refuse `--yaml` and `--json` for good: `serve`, which runs until stopped and writes a log; `completion`, a shell script; `help`, its text; and `query export` without `--link`, the file. Every other command writes structured output, and a test holds every command to that (`cmd/anfra`): a new command either has a structured result or is marked an artifact.

## The contract: `api/openapi.yaml`

The OpenAPI document generated from the registry is committed, and is the core API's contract. It is what clients see, and what changes to it are reviewed against.

```sh
scripts/openapi.sh generate          # write api/openapi.yaml (from `anfra openapi`; no repo or sidecars needed)
scripts/openapi.sh diff              # fail if the committed file is stale
scripts/openapi.sh breaking <base>   # fail if it breaks the base document
```

Run `generate` after changing anything a command publishes: a command added, removed or renamed, its input or output types, its declared errors, or a code. CI (`.github/workflows/continuous_integration.yml`, the `api` job) fails on a stale file, and refuses a breaking change unless a commit in the pull request declares it (`feat!:`, or a `BREAKING CHANGE:` footer). A breaking change reaches every client at once: Data Apps, the SDK, agents and every platform serving these ops. It has to be a decision, not a side effect.

The document's version is the contract's, not the binary's: it does not change with every release.

### Deprecation before removal

A command or flag keeps working, with a deprecation warning, for at least one minor release before it is removed or changed. That is the default for every command, an op or not: people and their scripts call the CLI, and so do the anfra skills, whose copies on a machine can be a release or two behind the binary ([agent setup](agent-setup.md#deprecation)).

- **An op** still needs its breaking change declared, through the contract check above. The warning period comes before it.
- **A CLI-only command,** such as `update`, `init` or `agent setup`, is in no contract, so this rule is its only protection. `anfra skills install`, now `anfra agent setup`, is kept as an alias that says so.

### Asking without acting

A command that changes something offers ways to ask about it without changing it. Each answers a different question, so it takes a different form:

- **`status` describes the state of a thing.** Read-only, cheap, safe to run any time: "what is the situation?" It exits 0 whatever the state, unless judging the state is its job (`anfra status` exits non-zero when the server isn't healthy). `anfra status`, `anfra auth status`.
- **`<action> check` is the action's preflight.** It does everything the action would up to its first change, then reports what the action would do: "what would happen if I ran this?" It exits non-zero when the action would fail. `anfra update check`, `anfra agent setup check`, `anfra publish check`. A preflight that does only part of that so far, as `publish check`'s, offline, is still the action's check: the rest is added to it, rather than a command for each part, until real use asks for one (`publish check --offline`, say).

A preflight is a subcommand, not a flag, by the rule below, and `anfra <action> --help` lists it. A check belongs to its action, not to the action's group: `agent setup check`, not `agent check`, so that a second action in the group (`agent remove`, say) gets its own check without either becoming ambiguous.

anfra has no `--dry-run`. Other tools use the name for both an offline look and an online preflight, so it says nothing about how far a command goes.

### One command, one result

A command answers one shape of result, whatever its flags. A flag may narrow or tune what a command does (`--agent`, `--set-org`, `--page-size`), never change what its result is. A variant that answers something else is a command of its own, often a subcommand, as the preflights are (`publish check`, not `publish --list`).

The reason is that a command is an API, and is held to what an API's operation is ([philosophy.md](philosophy.md#make-interfaces-work-for-agents-and-programs-not-only-people)). An op has one input and one output type, the same over HTTP, MCP and the CLI ([define once, serve everywhere](philosophy.md#define-once-serve-everywhere)); a CLI-only command keeps to the same, so a program or an agent reads every command the same way, and is never surprised by what comes back. In practice:

- **`--schema` describes the command** with one schema, and a program or a typed client maps one command to one type: no branching on which flags were passed to know what it got.
- **`--help` lists the variant** among the commands, where a flag would hide it among the options.
- **The name says what it answers**, rather than leaving a reader to guess what a flag does to the output.

Precedents: Terraform's `plan` and `apply`, Git's `status` and `commit`. The counter-pattern, a flag that swaps the output (`cargo package --list`, `npm pack --dry-run`), is common in single-purpose tools, but leaves the command with two results to document and type.

### New state as output

*A pattern we follow where it fits, not a rule yet.*

A command that changes something can answer with the state it leaves, in the shape of the command that reads that state, plus what it changed. The caller learns where things stand now, which is what it acts on next, in fields it already knows from the read.

Precedents:

- **HTTP:** a `PUT` or `PATCH` that answers with the resource as it now is.
- **`kubectl apply -o yaml`:** the object as the cluster now holds it.
- **`anfra update`** (planned, [cli-json.md](../implementations/cli-json.md)): `anfra version`'s answer as it is after the update, plus `previous`, the version it replaced. `update check` is the same answer without acting.
- **`anfra agent setup`** (planned, same doc): `agent setup check`'s report as it is after setup, plus what setup did for each agent.

A command with no read of its state answers with what it changed alone: `anfra init`, the files it created.

### Public and internal ops

*Decided 2026-10-14, not built; anfra-cloud's `framework/api_framework.md` has the design.*

Every op a host serves is reachable from the internet, so every op meets the same bar:
authenticated where the host authenticates, authorized, validated, failing with formal codes. What
a host's documentation says is how formal and stable an op is: `apikit` will let a host mark an op,
or one field of its schema, internal (`x-anfra-internal: true`), which leaves it out of the
published spec, for ops serving the host's own frontend alone. Clients a host does not ship with
use public ops only.

**anfra core marks nothing internal.** Every core op is a command, so also a CLI command, an MCP
tool and part of the core API that Data Apps and the SDK are written against: public by
construction. The flag is for a host's own ops (anfra-cloud's web app's); when it lands, a test
here holds every core op public.

## Publishing: anfra-cloud's contract, vendored

`anfra publish` calls the server of the org a repo publishes to, an anfra-cloud: `repos.status`, `repos.list_missing_blobs`, `repos.create_blobs`, `repos.publish`, and `data_sources.list`, `.test`, `.create` for the data sources the repo uses. Those ops are not the core API, and `anfra serve` does not serve them: they are anfra-cloud's, and its `api/openapi.yaml` is their contract. anfra core vendors that file (`internal/cloudapi/openapi.yaml`, copied with `scripts/cloudapi.sh sync`; by hand for now, a proper sync process later) and writes its own types for the ops the CLI calls (`internal/cloudapi`: types, codes, a client). A test holds those types to the vendored contract: every member sent is one the server takes, every member it requires is sent, and every member read is one it answers, each of the same type.

The CLI calls public ops only, which anfra-cloud changes additively, so an older CLI keeps working; it ignores what it does not know. What a repo is (which files a publish carries, how content is named) stays core's: `shared/repoformat`, which anfra-cloud imports.

## Clients from the contract

The SDK's client for the core API, `@holistics/anfra-sdk/api`, is typed by `web/sdk/src/api/schema.d.ts`, generated from `api/openapi.yaml` (`pnpm --filter @holistics/anfra-sdk generate`). CI fails when it is stale. A change to a command's input or output therefore reaches TypeScript callers as a type error, not at runtime. Any other client can be generated from the same document.

## Why it is shaped this way

- **One definition, every surface.** A flag that exists on the CLI but not over HTTP, or an error the API returns but the CLI cannot show, is impossible by construction.
- **One interface, one body of knowledge.** The CLI command, the HTTP op and the MCP tool share their name, arguments, answer and errors, so what an agent knows about anfra holds however it reaches it. A coding agent with a terminal runs `anfra query --dataset sales …`; a web-based agent with no shell calls the `core.query` tool over MCP, with the same fields; a program posts the same input to `/api/core.query`. Skills, docs and examples written for one apply to the others, translated mechanically rather than rewritten, so agents, local or web-based, with CLI access or without, share the same skills and knowledge.
- **Programs and agents are first-class callers.** Inputs are typed and validated before anything runs, and a refusal names each offending arg (its violations). Answers are structured, an invalid result is distinguished from a failure, and every operation describes itself. An agent can call, read the refusal, fix its input and retry in one short loop.
- **The contract is reviewed, not inferred.** Committing the generated document turns every change to the API into a visible diff, and makes breaking ones explicit.
- **Platforms serve the same API.** `engine.Ops()` hands a platform these same operations, so the core API is identical on `anfra serve`, anfra-cloud, or any other platform ([engine.md](engine.md)).
