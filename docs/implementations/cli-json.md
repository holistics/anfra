# Implementation: `--json` on every CLI command

Implements [CLI output](../designs/commands-and-api.md#cli-output): every command writes structured output, `--json` writes it as JSON instead of YAML, and `--non-interactive` asks nothing. YAML stays the default ([philosophy.md](../designs/philosophy.md#make-interfaces-work-for-agents-and-programs-not-only-people)).

## Where it stands

| Piece | State |
|---|---|
| `--json` and `--non-interactive` on the root command, replacing `auth`'s | Done (`cmd/anfra/output.go`) |
| Core commands' answers | Done |
| Errors and warnings on stderr | Done, deprecation notes included |
| No questions, no notices | Done |
| Every command's output kind, and the test holding every command to one | Done |
| CLI-only commands' structured results | `auth` and `openapi`; the rest not started |

## Order

```
1 the flags, core commands, stderr ──> 2 artifacts marked, CLI-only commands structured
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

## 2. Artifacts marked, CLI-only commands structured

- **Every command has an output kind** (`cmd/anfra/output.go`): `markStructuredOutput` for core commands, `auth` and `openapi`; `markArtifactOutput` for `serve`, `completion`, `help` and the internal update check, refused for good with what they write instead; `markNoStructuredOutputYet` for `init`, `publish`, `update` and `agent setup`, refused with "doesn't write structured output yet". Done; step 2 empties the last kind and removes it.
- **A test holds every command to one** (`TestEveryCommandHasAnOutputKind`): it walks every runnable command of the root, and fails on one without a kind. A new command can't print prose unnoticed. Done.

Each CLI-only command's result has a shape the command defines (a Go type in `cmd/anfra`) and documents in its `--help` ([CLI output](../designs/commands-and-api.md#cli-output)):

| Command | Its result |
|---|---|
| `update --check` | The running version, the latest, whether an update is available. |
| `agent setup --check` | Each agent, and the copies of the skills found, with their versions. |
| `publish --list` | The files, the destination, and the files left out. |
| `publish` | The revision published, whether anything changed, and what it replaced. Diagnostics from validation go in the error's details. |
| `update` | The version installed, and each server still running another. |
| `agent setup` | What it installed or updated, per agent. |
| `init` | The files and folders it created. |

`publish`, `agent setup` and `init` print progress and notes for a person today (`internal/cliui`), some on stdout. That goes to stderr, on a terminal only, and stdout holds the result alone, in YAML or JSON.
