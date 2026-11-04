# Implementation: `--json` on every CLI command

Implements [CLI output](../designs/commands-and-api.md#cli-output): every command writes structured output, `--yaml` and `--json` write it as YAML or JSON from any command, and `--non-interactive` asks nothing. A core command defaults to YAML ([philosophy.md](../designs/philosophy.md#make-interfaces-work-for-agents-and-programs-not-only-people)); a CLI-only command to the pretty format ([cli-pretty.md](../designs/cli-pretty.md)).

## Where it stands

| Piece | State |
|---|---|
| `--json` and `--non-interactive` on the root command, replacing `auth`'s | Done (`cmd/anfra/output.go`) |
| Core commands' answers | Done |
| Errors and warnings on stderr | Done, deprecation notes included |
| No questions, no notices | Done |
| Every command's output kind, and the test holding every command to one | Done |
| CLI-only commands' structured results | Done: `openapi`, `update`, `agent setup`, `init`, `publish` and `auth`, with `message` and `hints` |
| `--yaml`, and a CLI-only command's default, the pretty format | Done: `--yaml`, the defaults, the annotations, the help |
| The prettifier's first views: `update`, `agent setup` and `init` | Built (`internal/pretty`) |
| CLI-only commands' errors classified | Done (`internal/clierr`) |
| `--schema` | Done (`cmd/anfra/schema.go`) |
| `--jq` | Done (`cmd/anfra/jq.go`) |

## Order

```
1 the flags, core commands, stderr ──> 2 artifacts marked, CLI-only commands structured, --yaml ──> 3 CLI-only errors classified ──> 4 --schema ──> 5 --jq
```

Step 1 is one change in the shared rendering code, and leaves every CLI-only command refusing `--json` except `auth` and `openapi`. Step 2 turns the rule around, so every command writes structured output unless it is marked an artifact, and gives the remaining CLI-only commands their structured results, all at once: the test that holds every command to the rule passes only then.

## 1. The flags, core commands and stderr

- **The flags.** Persistent `--json` and `--non-interactive` on the root command (`newRootCmd`, `cmd/anfra/main.go`). `auth`'s own `--json` and `--no-interactive` (`cmd/anfra/auth.go`) go: it reads the root's, with the same meanings, so `anfra auth status --json` keeps working. `--no-interactive` stays accepted for good, as `--non-interactive` under another name (pflag's `SetNormalizeFunc`), shown nowhere. The two are independent: `--json` sets the format only. So `auth --json` on a terminal now asks, as `auth` alone does, where it used to ask nothing; a script passes `--non-interactive` too.
- **Answers.** `renderTo` (`cmd/anfra/commands.go`) writes the body as it is, one JSON document and a newline, instead of converting it to YAML.
- **Errors.** `printError` (`cmd/anfra/errors.go`) writes the envelope as one JSON line, `{"error":{…}}`, with the CLI's `arg` and `help` as in YAML.
- **Warnings.** `printWarning` writes `{"warning":{…}}`, one a line. Cobra's flag deprecation notes, plain text today (`publish --dry-run`), become warnings with a code, so they don't break the stream.
- **No questions, no notices.** `--non-interactive` counts as no terminal wherever the CLI decides whether to ask or to print a notice (`stderrIsInteractive` in `cmd/anfra/update.go`, `terminalFor` in `cmd/anfra/auth.go`, `publish`'s terminal).
- **Refusals.** A command not marked as writing structured output refuses `--json`, as an invalid argument naming `--json`, before it runs. In step 1 the marked ones are every core command, `auth` and `openapi`. `serve` and `completion` stay refused for good.
- **`openapi`** writes the OpenAPI document as JSON.
- **`query export`** refuses `--json` without `--link`.

Tests: for a core command, `--json`'s stdout parses as JSON and equals the op's answer; an error and a warning are JSON lines; a refused command fails before running; `auth --json` and `auth --no-interactive` behave as before.

## 2. Artifacts marked, CLI-only commands structured, `--yaml`

- **Every command has an output kind** (`cmd/anfra/output.go`): `markStructuredOutput` for core commands, `auth` and `openapi`; `markArtifactOutput` for `serve`, `completion`, `help` and the internal update check, refused for good with what they write instead; a third kind, no structured output yet, for the CLI-only commands until each had its result. Done; that kind is gone, now that every CLI-only command has one.
- **A test holds every command to one** (`TestEveryCommandHasAnOutputKind`): it walks every runnable command of the root, and fails on one without a kind. A new command can't print prose unnoticed. Done.

- **`--yaml`** joins `--json` on the root command: YAML, from any command. Both refused by an artifact.
- **The annotations** (`cmd/anfra/output.go`): `anfra.output`, the kind (structured, artifact, or no structured output yet); `anfra.artifact-desc`, what an artifact writes instead; `anfra.default-format`, `yaml` or `pretty`; `anfra.interactable`, a command that may ask when the run is interactive. They decide the refusals, the default format (`writeResult`) and each command's help line.
- **A CLI-only command's default** is the pretty format, through the prettifier ([cli-pretty.md](../designs/cli-pretty.md)), each view reviewed from real output: `update`, `agent setup`, `init`, `publish` and `auth`. Done. stdout holds no advice that the result lacks.
- **`--help` says it.** The root's: every command writes YAML, except CLI-only commands like `agent setup`, `auth`, `init` and `publish`, which write the pretty format by default; `anfra --help` lists the two apart, as "Core commands" and "CLI-only commands" (cobra's command groups). Each command's `--help` ends with its output: its default format, and whether it asks on a terminal. All generated from the commands' annotations, so they can't drift.
- **`message` and `hints`** open and close every CLI-only command's result ([CLI output](../designs/commands-and-api.md#cli-output)). Advice that's prose on stderr today ("Next: put your database in …", "To update the skills in …") moves into `hints`.

Each CLI-only command's result has a shape the command defines (a Go type in `cmd/anfra`), which step 4's `--schema` shows ([CLI output](../designs/commands-and-api.md#cli-output)), between its `message` and its `hints`:

| Command | Its result |
|---|---|
| `update check` | `anfra version`'s answer, checked now, and the changelog's link. It was `update --check`, which warns now ([new state as output](../designs/commands-and-api.md#new-state-as-output)). |
| `agent setup check` | Each agent, and the copies of the skills found, with their versions. It was `agent setup --check`, which warns now. |
| `publish check` | The org (`url`; null when none is configured) and the files, a tree in the pretty format. It was `publish --list`, and before it `--dry-run`, which warn now. The note on the folder's other files goes. |
| `publish` | The org (`url`, `name`, from the login), the revision published, whether anything changed, what it replaced, how many files, and the data sources it created. A repo that fails validation fails with `repo_invalid`, anfra-cloud's code for the same, with its details: `{validation: <validate's answer>}`. |
| `update` | `anfra version`'s answer after the update, the version it replaced, and the changelog's link. Servers still running another version stay warnings, and the skills needing attention becomes one. |
| `agent setup` | `agent setup check`'s report after setup, and what setup did per agent: `installed`, `failed`, `cli_missing` or `instructions_only`, with a message. |
| `init` | Its folder, and each file with whether init changed it (created it, or added to it). A failure partway carries the result as far as it got. |
| `auth status`, `login`, `logout` | `status`: each org with a saved login, and the current repo's without one: `name`, `url`, `saved_login` (`valid`, `invalid`, `missing`), `current_repo`. `login`: `org_url`. `logout`: `org_url`, `local_removed`, `remote_revocation_confirmed`; a failed revocation after local removal is `command_failed`, the result its details. `remote_checked`, `repo_org_url`, `repo_destination_changed` and `repo_active` go: their guarantees are in `--help` and the usage doc. |

`publish`, `agent setup` and `init` print progress and notes for a person today (`internal/cliui`), some on stdout. With `--yaml` or `--json`, stdout holds the result alone; progress goes to stderr, on a terminal only, and advice into `hints`.

A command that fails partway, such as `agent setup` failing for one agent of two, writes no result: its error carries the result, as it stands, in its `details`, as `publish`'s carries validation's diagnostics. stdout holds an answer only on success.

What a program should know becomes a warning document, with a code; what exists only because a person is there to answer stays prose, on a terminal only: questions and confirmations, MotherDuck's sign-in and upload, and a new token's secret, shown once. Off a terminal, `publish` never makes a token (no `MotherDuck` without one), so its secret never reaches a program. `publish`'s warnings (`p.Warn` in `internal/publish`) become:

| Code | Today's text |
|---|---|
| `data_sources_not_checked` | "Not checking the org's data sources: …" |
| `data_source_type_differs` | "Data source X is postgresql here and bigquery on the org…" |
| `data_sources_missing` | "Queries on the models using X fail until the org has them." |
| `data_source_local_file` | "Data source X (duckdb) uses a local file that the org's server can't read…" |
| `origin_not_allowed` | "The org does not allow X, which this repo's Data Apps may load from…" |
| `origins_not_checked` | "Not checking the org's allowed origins: …" |
| `last_published_elsewhere` | "This repo was last published by X on Y: publishing replaces that version." |
| `token_not_revoked` | "Could not revoke the unused MotherDuck token X…": a live credential left behind, though only the interactive flow makes one |
| `data_source_unreachable` | "Data source X is not reachable from the org's server, so it wasn't created": without a terminal to choose to create it anyway |
| `data_source_not_created` | "Data source X was not created: …": the server refused it |
| `data_app_not_portable` | validate's Data App warnings: what in a Data App won't work in a host |

Advice that was prose (`p.Note`: ask an admin, pass `--create-data-sources`, load a DuckDB file's data into a warehouse) is in the result's `hints`.

## 3. CLI-only commands' errors classified

A CLI-only command's failure was mostly a plain error (`fmt.Errorf`, `errors.New`), in `cmd/anfra` and in what it calls, and so was cobra's own. `apperr.From` reads an error with no code as ours: `internal_server_error`, scope `server`, telling a program "anfra is broken" where the user can fix it. And a coded error wrapped in a plain one (`fmt.Errorf("…: %w")`) reads the same, its code lost. So a path that may carry a code returns it as it is, and adds context, where it matters, with a public step ([errors.md](../designs/errors.md#steps-context-written-once)), never by rewording the message: a failed sign-in carries `anfra_cli.sign_in`, "sign in to {origin}", in its `context`.

The CLI's codes are in `internal/clierr`, in their own namespace, `anfra_cli`, out of the engine's catalog (`engine.ErrorCodes`), which hosts translate. Every failure is in one of three buckets.

**Kept `internal_server_error`: our own failures.** Encoding or decoding our own data; `crypto/rand`; `cliauth`'s broken invariants; "anfra runs no program"; the org's server hashing uploads differently; an update whose rollback failed too; setting up logging or the working directory.

**Existing codes:**

| Failure | Code | From |
|---|---|---|
| cobra: an unknown flag, a flag missing its value or with one it can't parse; the wrong args | `invalid_request`, a violation on the flag (or `args`) | apperr |
| cobra: an unknown command | `unknown_command` | the engine |
| a JSON flag that isn't one; no org URL without a terminal; an invalid org address; `--scope project`; an invalid `ANFRA_CLOUD_URL` or `ANFRA_SITE_URL` | `validation_failed`, a violation on the arg, flag or variable | apperr |
| no saved login to sign out of | `not_found` | apperr |
| a sidecar that won't start | `sidecar_unavailable` | the engine |
| a command failing partway, its result in details | `command_failed` | `anfra_cli` |
| the org's server's own errors (`cloudapi.Error`) | its code, passed through as its envelope, as `anfra serve`'s | anfra-cloud |

**New codes**, each where a program does something of its own, or mirroring anfra-cloud's, so a client reads one code whichever side found the failure:

| Code | Scope | Failures | What a program does |
|---|---|---|---|
| `unauthenticated` | user | no usable login saved for the org; anfra-cloud's code, for the same as its server finds it | asks the user to sign in (`anfra auth login`), which takes a browser; in CI, fails saying the job needs a login |
| `org_not_set` | user | the repo has no org to publish to, and no terminal to choose one | passes `--set-org` if it knows the org, else asks the user which |
| `confirmation_required` | user | publishing would replace the org's version, with no one to confirm | asks the user, and passes `--yes` only on a yes: never on its own |
| `interrupted` | user | Ctrl-C, whatever failed as it stopped; a no to the question; access denied in the browser. The CLI exits 130, as a shell does | neither retries nor reports a failure: the user stopped it. Not anfra-cloud's `cancelled`, an operation someone else stopped |
| `precondition_failed` | user | built from source (`update`); no coding agent; no files to publish, too many or too large; a symlink anfra won't write through or publish; `serve` already running, or the address taken; can't listen for the browser, or open it; a sign-in timed out; an unreadable config or `org.url`; the credentials file's permissions or state; MotherDuck's names taken, or another account signed in; a file changed while publishing; no permission to replace the binary | shows the message: the user changes something |
| `unavailable` | server | GitHub (the release check, the download, the changelog, the skills' manifest); the org's server unreachable, or not answering as anfra; its authentication server; MotherDuck's CLI or API; the warm `anfra serve` not answering. anfra-cloud's code, details `{service, retry_after_seconds}` | retries later; `service` (`github`, `motherduck`, `org`, `auth`, `anfra_serve`) says what was down, anfra's or a third party's |
| `repo_invalid` | user | the repo fails validation before a publish; anfra-cloud's code, with its details, `{validation: <validate's answer>}` | shows the diagnostics: the user fixes the files |

`internal_server_error` stays the one catch-all, for CLI-only commands too: one code for "anfra failed" whichever surface, as HTTP's 500 and gRPC's `INTERNAL` are.

Tests: cobra's refusals coded; an interrupted run; the org's server's errors passed through; `repo_invalid`'s details as anfra-cloud's; a failed sign-in's step in its context.

## 4. `--schema`

`anfra <command> --schema` writes the command's input and output instead of running it ([CLI output](../designs/commands-and-api.md#cli-output)): `command`, `input`, `output`, in YAML or with `--json` in JSON. Done:

- **A core command's** come from its op, as discovery's usage does (`rt.Inline`). **A CLI-only command's** input from its flags and positionals, its output from its result type, recorded with `resultIs`; a test holds every structured command to having one.
- **Each input field is a flag, `--<field>`,** in kebab or snake case: pflag takes a flag by its field's name too (`--data_source`). Only a field that isn't one says so, `x-cli: [positional]` or `[positional, stdin]`, as the op's `cli` tag. No short forms: `--help` has them, and an agent is better off with the long one.
- **In order:** a schema's keywords (`type`, `description`, `x-cli`, `required`, `properties`, …) and its properties in the result's own field order, as the result is written.
- **Descriptions** from `doc` tags, now on the CLI-only result types too, one clause each; the "Its result opens with…" paragraphs in `--help` went, for a sentence on the result's meaning.
- **An artifact refuses it**, as it refuses `--json`.
- **Two fixes to apikit's inlining**, which discovery shares: a struct field's description, beside its `$ref`, was dropped; and a type that refers to itself (lineage's `DiagramSection`) failed, so `/api/ops/lineage` did. It is now kept once, in `$defs`.

The summary and doc stay out: `--help` is the first look, `--schema` the contract, and repeating them would cost every reader. The hint for reading the output alone, without descriptions, came with `--jq`, in the AGENTS.md block.

## 5. `--jq`

`anfra <command> --jq <expr>` filters a structured answer before it's written, with jq built in (gojq v0.12.19, as `gh --jq` embeds it: no `jq` to install). Done:

- **Applied in `renderTo`**, so to every structured answer: core commands', CLI-only commands' (it skips the pretty format, as `--yaml` does), and `--schema`'s.
- **Written in the chosen format:** each result a JSON line with `--json`; when every result is a scalar, a plain line each, a string unquoted, as `jq -r`, for a shell (`for url in $(anfra auth status --jq '.orgs[].url')`); else YAML, each result a document of its own when there are several. A filtered object's keys come out sorted: gojq's objects are maps.
- **Numbers as written:** the answer is decoded with each integer an `int`, or a `*big.Int` past its range, so a filter keeps every digit.
- **Refused early:** an expression that doesn't parse is an invalid `--jq` before the command runs, a filter that fails on the answer one after; an artifact refuses it, as it refuses `--json`.
- **jq, not yq.** jq is the language most people and agents know. mikefarah's yq, the common one, speaks a dialect that looks alike and behaves differently, so it isn't offered beside it; `--yaml` piped into yq still works.
- **A filter, not field selection.** `gh --json fields` selects what `gh` asks its API for. An op here computes its whole answer, so selecting fields would only cut it afterwards, which the filter does, and more. An op whose answer is costly can take its own argument for less (a `fields` on `query`, say), later and per op.
