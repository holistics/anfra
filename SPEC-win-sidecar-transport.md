# Spec: win-sidecar-transport

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-sidecar-transport`). Depends on: nothing. Every other Windows module depends on it.

## Objective

anfra talks to its sidecars over **Unix sockets** (AF_UNIX): a socket file on disk that only processes on the same machine can reach (`internal/sidecar/socket.go`). This module answers one question, and fixes what it finds:

> **On Windows, can the three processes still talk over Unix sockets, with no change to how they talk?**

The three links:

| From → to | Today | Code |
|---|---|---|
| anfra (Go) → anfra-node (Bun) | HTTP JSON-RPC over a Unix socket. anfra passes `--socket=<path>`, anfra-node runs `http.createServer().listen(path)`. | anfra `internal/sidecar/anfranode/client.go` `NewClientUnix`; holistics-core `apps/anfra-node/src/rpc/server.ts` `startServer` |
| anfra (Go) → canal-query (Go) | HTTP over a Unix socket. anfra sets `SOCKET=<path>`. | anfra `internal/sidecar/canalquery/canalquery.go`; canal `internal/shared/utils/listen.go` `Listen` |
| canal-query (Go) → canal-lake (Go) | Arrow Flight (gRPC) over a Unix socket. canal-query dials `"unix:" + CANAL_LAKE_SOCKET`. | canal `internal/query/lakeclient/lakeclient.go` `Location`; canal `internal/shared/utils/listen.go` `Listen` |

**What we already know** ([findings doc, Background: Bun's source](docs/research/windows-sidecar-transport.md#background-buns-source)): Bun's source shows that on Windows, `Bun.listen({ unix: path })` and `node:net` create a real AF_UNIX socket for a normal file path, and a named pipe only for `\\.\pipe\…` names. Go supports AF_UNIX on Windows. This was read from source code; nothing has run on Windows yet.

**What is still unknown:**

1. anfra-node listens with **`node:http`**, not `node:net`. In Bun, `node:http` runs on `Bun.serve`, a different code path from `Bun.listen`. Does `Bun.serve` with a file path also make an AF_UNIX socket on Windows?
2. Does grpc-go accept `unix:C:\Users\…\canal-lake.sock`? Its address parser may read `C:` as part of a URL.
3. Does canal's stale-socket check (`fi.Mode()&os.ModeSocket != 0` in `listen.go`) see a leftover socket file on Windows? If not, the next start fails with "address in use".
4. Is the socket path short enough? AF_UNIX caps it at about 108 bytes. `SocketPath` puts it in the temp folder, on Windows `C:\Users\<name>\AppData\Local\Temp\anfra-<random>\canal-query.sock`, which is about 70 bytes plus the user name.
5. Does anfra-node's stdin-EOF watchdog (`src/rpc/watchdog.ts` `fdIsPipe`) turn on when stdin is a Windows anonymous pipe? This is not transport, but it's checked in the same run because it decides whether anfra-node can be stopped cleanly on Windows (needed by `win-process-lifecycle` and `win-anfra-node`).

**Users:** the engineers building the other Windows modules. The output decides whether they build on Unix sockets (expected) or a fallback transport: named pipes, or loopback TCP + token, [still open](WINDOWS-PLAN.md#decisions-needed). Its cost is in the plan's [Estimate](WINDOWS-PLAN.md#estimate).

**Out of scope:** building canal or anfra-node for release (`win-canal`, `win-anfra-node`); process start/stop (`win-process-lifecycle`); TCP mode (`--port`, `ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL`), which is for services and unchanged.

## Tech Stack

- anfra: Go 1.27 (`go.mod`), `net.Dial("unix", …)`.
- canal: Go 1.26, `google.golang.org/grpc` v1.83.2, Arrow Flight client (`flight.NewClientWithMiddleware`).
- anfra-node: Bun 1.2.10 (pinned in `holistics-core/.github/workflows/anfra_node_build_release.yml`), `node:http`.
- Windows: Windows 11 x64 (AF_UNIX needs Windows 10 1803 or later).

## Commands

The check programs are cross-built on Linux/WSL and run on Windows.

```sh
# anfra-node for Windows (in holistics-core/apps/anfra-node)
BUN_COMPILE_TARGET=bun-windows-x64 bash scripts/build-sidecar.sh /tmp/wincheck/anfra-node.exe

# Go check programs for Windows (pure Go, no cgo)
GOOS=windows GOARCH=amd64 go build -o /tmp/wincheck/ ./...     # in the check folder

# Back in anfra, after any fix: the checks AGENTS.md asks for
GOOS=windows go vet ./internal/sidecar/...
go test ./internal/sidecar/...
make check
make test
```

```powershell
# On Windows: run each check, e.g.
.\anfra-node.exe --socket="$env:TEMP\anfra-x\anfra-node.sock"
.\dialnode.exe "$env:TEMP\anfra-x\anfra-node.sock"
```

## Project Structure

| What | Where | Committed? |
|---|---|---|
| Check programs (small Go and Bun programs, one per unknown) | a branch `spike/win-sidecar-transport` in anfra, folder `dev/wincheck/` | **No.** Kept on the branch for reference, never merged. |
| Findings: for each unknown, what ran, on which Windows build, the result, the decision | `docs/research/windows-sidecar-transport.md`, listed in `docs/README.md` under Research | Yes |
| Fixes, only for checks that fail | anfra `internal/sidecar/socket.go`; canal `internal/shared/utils/listen.go`, `internal/query/lakeclient/lakeclient.go`; holistics-core `apps/anfra-node/src/rpc/server.ts` | Yes, one PR per repo |

## What each check does, and the fix if it fails

| # | Check | Pass when | If it fails |
|---|---|---|---|
| 1 | Start `anfra-node.exe --socket=<file path>`. Dial it from Go with the same dialer as `NewClientUnix`, and call `GET /health` and one JSON-RPC method. | `/health` answers `{"status":"ok"}` and the RPC answers | Try `Bun.listen` / `node:net` in place of `node:http`. If none makes an AF_UNIX socket, stop: choose the fallback transport and re-review this spec. |
| 2 | A Go Flight/gRPC server on a Unix socket in `%TEMP%`. Dial it with `"unix:" + path`, the way `lakeclient.Location` does. | One Flight call succeeds | Change `Location` to build an address grpc-go reads on Windows (e.g. `unix:///C:/…` with forward slashes), behind a small function with a test. |
| 3 | Start a listener with canal's `Listen`, kill the process, start it again on the same path. | The second start succeeds | In `Listen`, also treat a file that fails to dial as stale when `ModeSocket` isn't reported (Windows), and remove it. |
| 4 | Print `SocketPath("canal-query")` for a user with a long name (20+ characters), and listen on it. | Listening works, and the path is under 108 bytes | On Windows, `SocketPath` makes its folder under a shorter base, e.g. `%LOCALAPPDATA%\anfra\s\<random>`. The folder must stay private to the user. |
| 5 | Start `anfra-node.exe` from Go with stdin as a pipe (`PipeStdin: true`), then close the pipe. | anfra-node logs `watchdog.parent_death` and exits | Record it for `win-anfra-node` to fix there; no fix in this module. |

## Code Style

Fixes follow each repo's own rules. For anfra, read [naming](docs/dev/naming.md) and [readable prose](docs/designs/readable-prose.md) before writing a name or an error message. Windows-only behaviour goes in a build-tagged file next to the Unix one, not in a `runtime.GOOS` branch in the middle of a function, as `internal/filelock/filelock_windows.go` does today:

```go
// internal/sidecar/socket_windows.go
//go:build windows

package sidecar

// socketBase is where SocketPath makes its folder. Windows: under
// LOCALAPPDATA, since TEMP under a long user name can pass the
// ~108-byte cap on a socket's path.
func socketBase() string { … }
```

Comments say only why, briefly, as the comment on `SocketPath` does.

## Testing Strategy

- **The checks themselves are run by hand on Windows 11 x64**, and their results recorded in the findings doc with the Windows build number and the Bun version.
- **Each fix gets a unit test in its repo**, in the package it changes (`internal/sidecar` in anfra; `internal/shared/utils` and `internal/query/lakeclient` in canal). Tests that need Windows use `//go:build windows`, so they run in the Windows CI job that `win-release` adds. Until then, run them by hand on Windows: `go test ./internal/sidecar/...`.
- On Linux, existing tests must still pass: `go test ./internal/sidecar/...`, then `make check` and `make test` (anfra); canal's and holistics-core's own CI.

## Boundaries

- **Always:**
  - Record every check's result, pass or fail, in the findings doc.
  - Keep Linux and macOS behaviour exactly as it is.
  - Keep socket folders private to the user.
  - Run Go checks one command at a time (AGENTS.md).
- **Ask first:**
  - Switching any link to named pipes or TCP.
  - Adding a dependency (e.g. `go-winio`).
  - Changing the address format canal-query sends to anfra-node (`BaseURL`, `unix:<path>`).
  - Bumping Bun.
- **Never:**
  - Merge the check programs.
  - Make a TCP listener bind anything but loopback.
  - Change the sidecar contract on Linux/macOS to make Windows easier.
  - Commit before you ask.

## Success Criteria

1. `docs/research/windows-sidecar-transport.md` exists, with a result for each of the 5 checks, the Windows build and Bun version they ran on, and one decision: **Unix sockets** or **a fallback transport**.
2. If the decision is Unix sockets: checks 1–4 pass on Windows 11 x64 (after fixes where needed), and each fix has a test.
3. If the decision is a fallback transport: this spec and WINDOWS-PLAN.md are updated with the new transport, and reviewed before any other Windows module starts.
4. Check 5's result is recorded and handed to `win-anfra-node`.
5. On Linux: anfra's `make check` and `make test` pass, and canal's and holistics-core's CI pass, with no change to how sidecars talk there.

## Open Questions

None. Any Windows 10 1803+ or Windows 11 x64 machine can run the checks; the findings doc records which one did.
