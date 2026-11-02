# The CLI and the warm server

While `anfra serve` runs for a repo, the CLI in that repo is mostly its client: a command typed at the terminal is sent to the server and answered there. This doc says why, and which commands are not sent, or not sent as they are.

## The warm server

`anfra serve` is a long-running server for one repo, with both sidecars kept warm ([architecture.md](architecture.md#two-ways-to-run)):

- **anfra-node** keeps the compiled program in memory, so a query or a validate after the first skips most of the compile.
- **canal-query** pools warehouse connections, which matters most for a remote warehouse, where opening a connection can cost more than the query ([sidecars.md](sidecars.md)).

A one-shot CLI command starts the sidecars it needs (`Needs` in `internal/command/command.go`), runs, and stops them, paying their startup and a cold compile every time. With a server already running, that is waste, so the CLI uses the server instead. It also keeps one process serving the repo, rather than the server and CLI-spawned sidecars working on the same compile cache and catalog side by side.

## How a command reaches it

The server records itself in the repo's runtime file (`<ANFRA_HOME>/repos/<repo id>/runtime/serve.json`) once it is ready, and removes it when it stops. A core command at the CLI goes through `invoke` (`cmd/anfra/commands.go`):

1. **Find the server.** `findServer` (`cmd/anfra/runtime.go`) reads the runtime file and asks the server's `/health` who it is. Only a server that answers as this repo and this instance counts: a file a crash left behind, or another server that took over the port, is no server.
2. **Forward, or run.** With a server, the CLI posts the call to its op (`callServe`) and prints that server's answer, or its error envelope ([errors.md](errors.md)). Without one, it runs the command itself, with its own sidecars.
3. **Present.** Either way, the CLI shows the answer its own way: YAML, or for an export, the file downloaded from the answer's link (`deliver`). Presentation is the CLI's; the answer is the server's.

The server may run another anfra version than the CLI: see [Version mismatch](#version-mismatch).

`/health` is the CLI's identity check, kept cheap: the CLI calls it before every command, with a 500ms timeout. It checks no sidecar.

## Three kinds of CLI command

| Kind | Commands | A core op? | Whose data answers at the CLI | Where it runs while a server runs |
|---|---|---|---|---|
| **CLI-only** | `serve`, `init`, `update`, `agent`, `auth`, `publish`, `openapi` | No: no door serves it. | The CLI's. | The CLI: there is no op to forward. |
| **Forwarded** | Every core command not below: `query`, `validate`, `show`, `status`, … | Yes. | The server's. | The server. |
| **CLI-local** | `version` | Yes: the same op, input and answer as on every door. | The CLI's own. | The CLI, which may still call the server's op where relevant. |

### CLI-only commands

These are Cobra commands in `cmd/anfra`, not core commands: they are in no registry, and no door serves them ([philosophy.md](philosophy.md#what-core-is)). They act on this machine or its user (the binary, the repo's files, the agents' config, the user's credentials), which a server must never do for its callers.

A CLI-only command can still run a core command, through `invoke`, and that call is forwarded like any other: `publish` validates the repo first (`validateHere` in `cmd/anfra/publish.go`), on the warm server when there is one.

### Forwarded commands

The default. A core command answers the same whichever door it is reached through, so the CLI forwarding it gets the same answer an HTTP or MCP caller would, from the same warm sidecars. A new core command is forwarded unless it has a reason below not to be.

### CLI-local commands

**CLI-local data** is a fact about the anfra process the user ran, such as its version or its update check. A core command whose answer includes some is **CLI-local**.

Unlike a CLI-only command, it is a core command: the same op, input and answer on every door. Only whose data fills the answer differs. On every door it is the anfra the caller reached: the server, over HTTP and MCP; at the CLI, the CLI itself. Forwarded, it would answer with the server's data as if it were the CLI's. So the CLI runs it itself, and may still call the server's op where the answer needs the server's data.

- `version`: the CLI's own version, and whether an update is out for it.

The list is explicit, in `cmd/anfra` beside `invoke` (`cliLocal`), for the same reason `noNotifyCommands` is (`cmd/anfra/update.go`): where a command runs is the CLI's behaviour, not the command's.

## Guidelines

- **Decide first whether it is an op.** A command that acts on this machine or its user (the binary, the repo's files, the agents' config, credentials) is a Cobra command, CLI-only, and never served. Anything else is a core command, served on every door.
- **An op is forwarded; nothing to do.** Unless:
- **Its answer is about the anfra running it, not the repo.** If `Run` reads the process's own facts (`meta.Version`, `cc.Update`, `cc.Skills`), a forwarded call would report the server's facts as the CLI's. Add it to `cliLocal`.
- **Needing no sidecar is not a reason.** A command that needs none may still read the server's state. Where a command runs follows from what its answer is about, never from its `Needs`.
- **Ask the server through its ops.** A CLI-local command gets the server's facts from the server's own op, not from its runtime file beyond finding it, its sidecars' sockets or a new endpoint. `/health` stays an identity check.
- **Keep one answer.** Running in the CLI changes which anfra answers, never the answer's shape: the CLI adds no field an HTTP caller lacks. What the CLI adds on its own is presentation (YAML, a download), and warnings on stderr.

## Considerations

### Version mismatch

A server keeps the version it started with, through `anfra update`, until it is restarted. So the CLI and the repo's server can be different versions, most often right after an update. Then:

- A forwarded command runs the server's code: its answer is what that version answers, without the fixes or fields the CLI's version has, and an input only the CLI's version knows may be refused.
- A CLI-local command answers as the CLI. Right after `anfra update`, `anfra version` shows the new version while the repo's server still runs the old one.
- An HTTP or MCP client talks to the server alone, so its answers are consistently the server's version. It has no CLI to differ from.

Mitigations:

A `server_version_mismatch` warning, on stderr before the answer, wherever the CLI meets a server of another version (`warnIfOtherVersion` in `cmd/anfra/runtime.go`):

- A forwarded command: "This command runs on `anfra serve` 0.15.0, still running for this repo. Restart it to run 0.16.0."
- `anfra version`, though it answers itself: "`anfra serve` 0.15.0 is still running for this repo. Restart it to run 0.16.0."
- `anfra update`, at the end, for each running server of another version, whatever its repo: "`anfra serve` 0.15.0 is still running for /path/to/repo. Restart it to run 0.16.0."

`anfra status` is forwarded, so it gets the warning, and its answer has the server's version.

## Alternatives considered

- **Forward only commands that need a sidecar.** No list to keep, and it follows the server's purpose. But it gets `version` and `status` right by coincidence: a future command that needs no sidecar but reads the server's state would quietly run in the CLI, and adding or removing a `Needs` would move a command without anyone reviewing that.
- **Forward only a listed set of commands.** The CLI answers as itself unless told otherwise. But the list is nine commands and grows with nearly every new one, and a command left off it fails quietly: it still works, only cold, beside the server.
- **`status` as CLI-local**, taking the server's part from the server's `core.status`, to show the CLI's version beside the server's. Forwarded, it is simpler, and the warning says when the two differ.
- **Sidecar health in `/health`** (always, or behind a parameter), for a CLI-local `status` to read. It duplicates `core.status`, and makes the check the CLI runs before every command slower.
- **`/livez` and `/readyz`, as anfra Cloud's servers have** (`anfra-cloud/cmd/anfra-bi/probes.go`). These are an orchestrator's probes: whether to restart an instance, whether to send it traffic. They would suit a container healthcheck for `anfra serve`, which nothing asks for yet, and are a separate decision.
