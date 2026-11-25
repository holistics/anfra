# Spec: win-process-lifecycle

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-process-lifecycle`). Depends on: `win-sidecar-transport` (done). Uses `win-canal`'s results (done). Repo: **anfra**. Milestone **M1**: anfra compiles and runs on Windows.

## Objective

anfra doesn't compile for Windows today, and the parts that start, stop and unpack its sidecars are Unix-only. This module makes anfra build for Windows, and run its sidecars there as safely as on Linux and macOS: started, stopped cleanly, never left running, never popping up windows, and unpacked so two anfra versions can run side by side.

**What fails today** (`GOOS=windows go build ./...`, checked 2026-11-25, with `internal/sidecar` stubbed to reveal the rest):

| Error | Where |
|---|---|
| `unknown field Setpgid in struct literal of type syscall.SysProcAttr` | `internal/sidecar/proc_other.go:14` |
| `undefined: syscall.Kill` | `internal/sidecar/process.go:133` |
| `undefined: restartConsole` | `cmd/anfra/console.go:279` (only `console_restart_unix.go` defines it) |
| `undefined: detachProcess` | `cmd/anfra/console_server.go:99`, `cmd/anfra/update.go:332` (only `detach_unix.go` defines it) |

Nothing else fails: `engine` and every other `internal` package compile for Windows once these do.

**What else this module changes, though it compiles** (from the plan's `win-process-lifecycle` rows and `win-canal`'s hand-off):

1. **Sidecars die with anfra.** On Linux, `Pdeathsig` (`proc_linux.go`) kills a sidecar when anfra dies; Windows has none. A **Job Object** with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` does it there: Windows kills every process in the job when anfra's handle to it closes, crash and `taskkill /F` included.
2. **Clean stop.** `Process.Close` sends `SIGTERM`, then kills the process group after 3 s (`process.go:128-133`). On Windows: close stdin first (anfra-node stops on stdin EOF, `win-sidecar-transport` check 5), send **Ctrl-Break** (`GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`; canal takes its normal shutdown and removes its socket, `win-canal` T5), then terminate the job after 3 s.
3. **Ctrl-C reaches anfra, not its sidecars.** Sidecars start in their own process group (`CREATE_NEW_PROCESS_GROUP`), as `Setpgid` does on Unix, so Ctrl-C in `anfra console` cancels a query without killing them.
4. **No console windows.** A sidecar started by an anfra that has no console (started by an agent or an editor) must not open a window.
5. **`.exe` and `duckdb.dll`, one folder per version (Windows only, decided with the user).** `extractRuntime` (`binary.go`) unpacks each sidecar as one file named by its content hash, `sidecars/<name>-<hash>`, and deletes older ones at once. On Windows:
   - the file needs `.exe` to run;
   - canal comes with DuckDB's official `duckdb.dll` (the `win-canal` decision, for MotherDuck), which must sit beside `anfra-canal.exe`;
   - a running program and its DLL are locked, so an older version still used by a running `anfra serve` can't be replaced or deleted.
   So Windows unpacks each version into its own folder, `sidecars\<name>-<hash>\<name>.exe` (plus `duckdb.dll` for canal), and pruning skips a folder still in use. **Linux and macOS keep today's single-file layout unchanged.**
6. **Detached processes** (`detachProcess`: the background update check, auto-update, the console's background server) start in their own process group, with no console window, so closing the terminal doesn't kill them.
7. **Restarting the console after an update** (`restartConsole`): `syscall.Exec` doesn't exist on Windows, so start the new anfra with the same arguments and terminal, wait for it, and exit with its code.
8. **Stopping the console's background server** (`console_server.go:163`) sends `SIGTERM`, which Windows can't deliver: stop it another way (Ctrl-Break to its process group, or the server's own shutdown).
9. **`anfra serve`'s busy-port fallback** checks `syscall.EADDRINUSE` (`serve.go:292`); Windows reports `WSAEADDRINUSE`, so serve fails instead of moving to a free port.

**Users:** anyone running anfra on Windows; every later module (`win-files-auth`, `win-line-endings`, `win-terminal`, `win-release`, …) builds on this one compiling.

**Out of scope:** canal and anfra-node themselves (no change needed: `win-canal`, `win-sidecar-transport`); building the release binaries and embedding them from release assets (`win-release`); CI (deferred); file, auth and line-ending fixes (their own modules); `canallake.go`'s `STORAGE_ROOT` already ends in `os.PathSeparator`, so the plan's row for it needs no change.

## Tech Stack

- Go 1.27 (`go.mod`); `golang.org/x/sys` v0.49.0 (already a dependency) for `windows.CreateJobObject`, `SetInformationJobObject`, `AssignProcessToJobObject`, `TerminateJobObject`, `GenerateConsoleCtrlEvent`.
- Windows 10 1803+ / 11, x64.
- The sidecars from the spikes, for running anfra on Windows: `anfra-node.exe` (Bun build, `win-sidecar-transport`) and the DLL build of `anfra-canal.exe` with `duckdb.dll` (`win-canal`), in `C:\Users\nuita\wincheck`.

## Commands

```sh
# Compile for Windows (the module's first gate), one command at a time
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go vet ./...

# Linux: the packages this module touches, then what CI runs (run when the user asks)
go test ./internal/sidecar/... ./cmd/anfra/...
make check
make test

# Windows: build anfra and the touched packages' tests in WSL, run them on the Windows host
GOOS=windows GOARCH=amd64 go build -o /mnt/c/Users/nuita/wincheck/anfra.exe ./cmd/anfra
GOOS=windows GOARCH=amd64 go test -c -o /mnt/c/Users/nuita/wincheck/sidecar.test.exe ./internal/sidecar
```

```powershell
# On Windows: anfra with the spike's sidecars (dev build: no embedded sidecars)
$env:ANFRA_NODE_BIN  = "C:\Users\nuita\wincheck\anfra-node.exe"
$env:ANFRA_CANAL_BIN = "C:\Users\nuita\wincheck\dll\anfra-canal.exe"   # duckdb.dll beside it
cd <a copy of anfra-demo-data on C:>
..\anfra.exe validate; ..\anfra.exe show; ..\anfra.exe query --dataset ecommerce "<AQL>"
```

## Project Structure

Windows-only code goes in `_windows.go` files beside the Unix ones; Unix files gain a `!windows` build tag where needed. No `runtime.GOOS` branches inside shared functions.

| File | Change |
|---|---|
| `internal/sidecar/proc_other.go` | Build tag `!linux && !windows` |
| `internal/sidecar/proc_windows.go` (new) | Process group, no console window, Job Object assignment |
| `internal/sidecar/process.go` | The kill step moves into a per-OS function; Windows: stdin close, Ctrl-Break, then job terminate |
| `internal/sidecar/stop_unix.go`, `stop_windows.go` (new) | That per-OS function |
| `internal/sidecar/binary.go` (+ `unpack_windows.go`, `unpack_other.go`) | Windows: one folder per version, `.exe`, extra files (`duckdb.dll`), prune that skips locked folders. Linux/macOS: unchanged |
| `internal/sidecar/anfracanal/` (`embed_release_windows.go`, …) | Windows release builds also embed `duckdb.dll`, unpacked beside `anfra-canal.exe`; `win-release` fills the asset |
| `cmd/anfra/detach_windows.go`, `console_restart_windows.go` (new) | Windows versions of `detachProcess` and `restartConsole` |
| `cmd/anfra/console_server.go` | Stop without `SIGTERM` on Windows |
| `cmd/anfra/serve.go` (+ `addrinuse_windows.go`, `addrinuse_other.go`) | `isAddrInUse(err)` that matches `WSAEADDRINUSE` on Windows |
| Tests beside each file | `_windows_test.go` for Windows-only behaviour |
| `docs/designs/sidecars.md` | A Windows section: Job Object, the stop sequence, the folder layout |

## Code Style

anfra's conventions: read [naming](docs/dev/naming.md) before naming anything, and [readable prose](docs/designs/readable-prose.md) before writing any message a user sees. Comments say only why, briefly. The shape, for the per-OS stop:

```go
//go:build windows

package sidecar

// stop ends a sidecar the Windows way: no SIGTERM here. Closing stdin stops
// anfra-node; Ctrl-Break, which Go delivers as os.Interrupt, stops canal.
func (p *Process) stop(wait time.Duration) { … }
```

## Testing Strategy

- **Compile gate:** `GOOS=windows go build ./...` and `go vet` clean.
- **Linux/macOS unchanged:** existing tests in the touched packages pass on Linux; `make check` and `make test` before finishing (run when the user asks, per the user's rules). The unpack layout on Linux stays `sidecars/<name>-<hash>`; a test pins it.
- **Windows unit tests** (`_windows_test.go`, built with `go test -c` in WSL, run on the Windows host until the Windows CI job exists):
  - the Job Object: a started child dies when its parent is killed;
  - the stop sequence: a child that waits for Ctrl-Break exits cleanly within the timeout, and one that ignores it is terminated;
  - the unpack: a folder per hash with the `.exe` and the extra file; a second version beside it; pruning skips a folder whose `.exe` is running and removes it once it isn't.
- **M1 on Windows 11** (end to end, with the spike's sidecars):
  - `anfra version`, `validate`, `show`, and `query` on a copy of `anfra-demo-data` (with and without the query cache, so canal-lake runs too);
  - after each command, no `anfra-node` or `anfra-canal` process is left;
  - `taskkill /F` on anfra during a long query leaves no sidecar running;
  - `anfra serve` moves to a free port when the default one is taken;
  - in `anfra console`, Ctrl-C during a query cancels it without killing the sidecars; leaving the console stops its background server;
  - started without a console (e.g. `Start-Process -WindowStyle Hidden`), anfra opens no sidecar windows;
  - once, with an `embed_sidecar` build that embeds the spike's sidecars and `duckdb.dll`: the folder layout is unpacked and used.

## Boundaries

- **Always:**
  - Keep Linux and macOS behaviour exactly as it is, the unpack layout included.
  - Put Windows code in build-tagged files.
  - Run Go checks one command at a time (AGENTS.md); run the full test suite when the user asks.
  - Record each Windows check's result in the PR description or a short findings note.
- **Ask first:**
  - Changing anything Linux/macOS users would notice.
  - Adding a dependency.
  - Touching the Makefile or CI (e.g. adding `GOOS=windows go build ./...` to `make check`).
  - Changing canal or anfra-node.
- **Never:**
  - Commit before asking.
  - Disable or weaken a test to make a Windows run pass.
  - Leave a sidecar process running after a check.

## Success Criteria

1. `GOOS=windows GOARCH=amd64 go build ./...` and `go vet ./...` succeed.
2. On Linux: the touched packages' tests pass, and `make check` / `make test` pass; the unpack layout and process handling are unchanged.
3. On Windows 11: `anfra version`, `validate`, `show` and `query` (with and without the cache) work on a copy of `anfra-demo-data`.
4. On Windows: no sidecar outlives anfra, after a normal exit, a crash or `taskkill /F`; Ctrl-C in the console doesn't kill the sidecars; no console windows appear when anfra has no console.
5. On Windows: each sidecar version is unpacked into its own folder (canal with `duckdb.dll` beside it); two versions run side by side; an unused old folder is removed, and one in use is left until it's free.
6. On Windows: `anfra serve` falls back to a free port; the console restarts after an update; leaving the console stops its background server.
7. `docs/designs/sidecars.md` describes the Windows lifecycle and layout.

## Open Questions

None.

Risk to settle early in the plan, not a decision: **Ctrl-Break and "no console window" can conflict.** `GenerateConsoleCtrlEvent` reaches only processes attached to the caller's console, and a sidecar started with `CREATE_NO_WINDOW` gets a console of its own. `win-canal`'s check worked with `CREATE_NEW_PROCESS_GROUP` alone, from an anfra that had a console. The plan tests both cases (anfra with and without a console) and picks flags per case; if Ctrl-Break can't reach canal, closing its stdin can't either (canal has no stdin watchdog), so the fallback is terminating the job after the wait, which skips canal-lake's clean shutdown.
