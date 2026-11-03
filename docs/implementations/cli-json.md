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
| CLI-only commands' structured results | `openapi`, `update` and `agent setup`, with `message` and `hints`; `auth` with `--yaml` or `--json`, without `message` and `hints` yet; `init` and `publish` not started |
| `--yaml`, and a CLI-only command's default, the pretty format | Done: `--yaml`, the defaults, the annotations, the help |
| The prettifier's first views, `update` and `agent setup`, then `init` | `update` and `agent setup` built (`internal/pretty`), to review; `init` next |
| CLI-only commands' errors classified | Not started |
| `--schema` | Not started |

## Order

```
1 the flags, core commands, stderr ──> 2 artifacts marked, CLI-only commands structured, --yaml ──> 3 CLI-only errors classified ──> 4 --schema
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

- **Every command has an output kind** (`cmd/anfra/output.go`): `markStructuredOutput` for core commands, `auth` and `openapi`; `markArtifactOutput` for `serve`, `completion`, `help` and the internal update check, refused for good with what they write instead; `markNoStructuredOutputYet` for `init`, `publish`, `update` and `agent setup`, refused with "doesn't write structured output yet". Done; step 2 empties the last kind and removes it.
- **A test holds every command to one** (`TestEveryCommandHasAnOutputKind`): it walks every runnable command of the root, and fails on one without a kind. A new command can't print prose unnoticed. Done.

- **`--yaml`** joins `--json` on the root command: YAML, from any command. Both refused by an artifact.
- **The annotations** (`cmd/anfra/output.go`): `anfra.output`, the kind (structured, artifact, or no structured output yet); `anfra.artifact-desc`, what an artifact writes instead; `anfra.default-format`, `yaml` or `pretty`; `anfra.interactable`, a command that may ask when the run is interactive. They decide the refusals, the default format (`writeResult`) and each command's help line.
- **A CLI-only command's default** is the pretty format: through the prettifier for `update` and `agent setup`, then `init`, its first views ([cli-pretty.md](../designs/cli-pretty.md)), reviewed from real output before they ship; for `publish` and `auth`, their hand-written output, until the prettifier covers them. Either way stdout holds no advice that the result lacks.
- **`--help` says it.** The root's: every command writes YAML, except CLI-only commands like `agent setup`, `auth`, `init` and `publish`, which write the pretty format by default; `anfra --help` lists the two apart, as "Core commands" and "CLI-only commands" (cobra's command groups). Each command's `--help` ends with its output: its default format, and whether it asks on a terminal. All generated from the commands' annotations, so they can't drift.
- **`message` and `hints`** open and close every CLI-only command's result ([CLI output](../designs/commands-and-api.md#cli-output)). Advice that's prose on stderr today ("Next: put your database in …", "To update the skills in …") moves into `hints`.

Each CLI-only command's result has a shape the command defines (a Go type in `cmd/anfra`), which step 4's `--schema` shows ([CLI output](../designs/commands-and-api.md#cli-output)), between its `message` and its `hints`:

| Command | Its result |
|---|---|
| `update --check` | `anfra version`'s answer, checked now, and the changelog's link ([new state as output](../designs/commands-and-api.md#new-state-as-output)). |
| `agent setup --check` | Each agent, and the copies of the skills found, with their versions. |
| `publish --list` | The org (`url`; null when none is configured) and the files. The note on the folder's other files goes. |
| `publish` | The org (`url`, `name`), the revision published, whether anything changed, what it replaced, and the data sources it created. Diagnostics from validation go in the error's details. |
| `update` | `anfra version`'s answer after the update, the version it replaced, and the changelog's link. Servers still running another version stay warnings, and the skills needing attention becomes one. |
| `agent setup` | `agent setup --check`'s report after setup, and what setup did per agent: `installed`, `failed`, `cli_missing` or `instructions_only`, with a message. |
| `init` | The files and folders it created. |
| `auth status`, `login`, `logout` | Their results as today with `--json`, now also with `--yaml`. Without either, their hand-written output, as today. |

`publish`, `agent setup` and `init` print progress and notes for a person today (`internal/cliui`), some on stdout. With `--yaml` or `--json`, stdout holds the result alone; progress goes to stderr, on a terminal only, and advice into `hints`.

A command that fails partway, such as `agent setup` failing for one agent of two, writes no result: its error carries the result, as it stands, in its `details`, as `publish`'s carries validation's diagnostics. stdout holds an answer only on success.

What a program should know becomes a warning document, with a code; what exists only because a person is there to answer stays prose, on a terminal only: questions and confirmations, MotherDuck's sign-in and upload, and a new token's secret, shown once. Off a terminal, `publish` never makes a token (no `MotherDuck` without one), so its secret never reaches a program. `publish`'s warnings (`p.Warn` in `internal/publish`) become:

| Code | Today's text |
|---|---|
| `data_sources_not_checked` | "Not checking the org's data sources: …" |
| `data_source_type_differs` | "Data source X is postgresql here and bigquery on the org…" |
| `data_sources_missing` | "Queries on the models using X fail until the org has them." |
| `data_source_local_file` | "Data source X (duckdb) uses a local file that the org's server can't read…" |
| `data_source_settings_not_sent` | "The settings of data source X are not sent…" |
| `origin_not_allowed` | "The org does not allow X, which this repo's Data Apps may load from…" |
| `origins_not_checked` | "Not checking the org's allowed origins: …" |
| `last_published_elsewhere` | "This repo was last published by X on Y: publishing replaces that version." |
| `token_not_revoked` | "Could not revoke the unused MotherDuck token X…": a live credential left behind, though only the interactive flow makes one |

## 3. CLI-only commands' errors classified

A CLI-only command's failure is mostly a plain error (`fmt.Errorf`, `errors.New`), in `cmd/anfra` and in what it calls (`internal/update`, `internal/agents`, `internal/publish`), and so is cobra's own, an unknown flag or a wrong number of args. `apperr.From` reads an error with no code as ours: `internal_server_error`, scope `server`. A program reads "anfra is broken" where the user can fix it: `update` from a build from source, a destination with no credentials, a flag mistyped.

Each becomes a coded error, as a core command's is ([errors.md](../designs/errors.md)):

- **The caller's input**, cobra's included: `invalid_request` or `validation_failed`, with a violation naming the arg, so the CLI's `arg` and `help` apply.
- **A state the user can change**, such as a build from source, or no login for the destination: a code of anfra's own, scope `user`, saying what to do.
- **The outside world**, such as GitHub or the org's server unreachable: a code saying which, so a program can retry, not `internal_server_error`.

The CLI's codes are in their own namespace, `anfra_cli` (`cmd/anfra/errors.go`), out of the engine's catalog (`engine.ErrorCodes`), which hosts translate. A failure no program branches on yet is the generic `command_failed`, scope `user` (as `query_failed` is for a query), with the command's details, as `agent setup`'s is with its result as far as it got; one a program comes to act on gets a code of its own. What's left with no code is anfra's own failure, as for a core command. New codes are public: they're part of the contract, and `feat!:` when an existing failure's code changes.

## 4. `--schema`

`anfra <command> --schema` writes the JSON Schema of the command's result instead of running it: in YAML, or with `--json` in JSON, as any output. One schema describes both, since a result has the same shape in either. A core command's comes from its op, as in `api/openapi.yaml`; a CLI-only command's is generated from its Go result type by huma, as the ops' are. `--help` then says, in a line, that the command writes its result as YAML or JSON and that `--schema` shows its shape, rather than listing its fields.
