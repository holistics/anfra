# Tasks: win-process-lifecycle

Plan: [plan.md](plan.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Spec: [SPEC-win-process-lifecycle.md](../SPEC-win-process-lifecycle.md).

Code changes go on `feat/windows-installer` (anfra). Windows-only tests are built in WSL (`GOOS=windows go test -c`) and run on the Windows host from `C:\Users\nuita\wincheck`. Go checks run one command at a time (AGENTS.md); the full suite (`make check`, `make test`) when the user asks. Nothing is committed without asking.

---

## Phase 1: It compiles

### - [x] T1: `internal/sidecar` compiles for Windows: process group and the per-OS stop

**Description:** Replace the two Unix-only calls with per-OS code. Windows starts each sidecar in its own process group, and stops it with the spec's sequence: close stdin for a `PipeStdin` sidecar, Ctrl-Break otherwise, then terminate after the wait (by process for now; T4 makes it the Job Object).

**Acceptance criteria:**
- [x] `proc_other.go` is `//go:build !linux && !windows`; new `proc_windows.go` sets `CREATE_NEW_PROCESS_GROUP` (console flags settled in T3).
- [x] `Process.Close` calls a per-OS `stop`: `stop_unix.go` keeps today's SIGTERM → 3 s → `Kill(-pid)` exactly; `stop_windows.go` closes stdin or sends Ctrl-Break (`windows.GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`), waits 3 s, then terminates.
- [x] `GOOS=windows go build ./internal/...` succeeds (`go vet` clean for Windows and macOS too).

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/sidecar/...`.
- [x] Build: `GOOS=windows GOARCH=amd64 go build ./internal/...`.
- [x] Manual, on Windows: a `_windows_test.go` starts a small helper process that waits for Ctrl-Break, and checks `Close` stops it within 3 s; and one with `PipeStdin` that exits on stdin EOF. Both pass on Windows 11 (`stop_windows_test.go`: each exits cleanly, code 0, in ~0.5 s).

**Dependencies:** None

**Files likely touched:** `internal/sidecar/proc_other.go`, `proc_windows.go`, `process.go`, `stop_unix.go`, `stop_windows.go` (+ `stop_windows_test.go`)

**Estimated scope:** M

### - [x] T2: `cmd/anfra` compiles for Windows: `detachProcess` and `restartConsole`

**Description:** Add the two Windows versions `cmd/anfra` is missing.

**Acceptance criteria:**
- [x] `detach_windows.go`: `detachProcess` starts the child in its own process group, detached from anfra's console (`CREATE_NEW_PROCESS_GROUP`, and `DETACHED_PROCESS` or `CREATE_NO_WINDOW`, per T3's finding for windows), so closing the terminal doesn't end it. `CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS` for now; T3 may change it.
- [x] `console_restart_windows.go`: `restartConsole` starts the anfra at `exe` with the same arguments, stdin/stdout/stderr and the `consoleRestarted` variable, waits for it, and exits with its exit code (`syscall.Exec` doesn't exist on Windows).
- [x] `GOOS=windows go build ./...` succeeds for the whole repo.

**Verification:**
- [x] Build: `GOOS=windows GOARCH=amd64 go build ./...` and `go vet ./...`.
- [x] Tests pass (Linux): `go test ./cmd/anfra/...`; on Windows, `TestRestartWith` passes.
- [x] Manual, on Windows: `anfra.exe version` runs (`0.0.0+d88d0c1.dirty`).

**Dependencies:** T1

**Files likely touched:** `cmd/anfra/detach_windows.go`, `cmd/anfra/console_restart_windows.go`

**Estimated scope:** S

### Checkpoint A: It compiles
- [x] `GOOS=windows GOARCH=amd64 go build ./...` and `go vet ./...` pass.
- [x] Linux: `go test ./internal/sidecar/... ./cmd/anfra/...` pass.

---

## Phase 2: Behaviours, one at a time

### - [x] T3: Stopping sidecars when anfra has no console (Ctrl-Break vs "no window")

**Description:** Settle the spec's risk before building on it. Ctrl-Break only reaches processes that share the caller's console; a sidecar started with `CREATE_NO_WINDOW` gets its own, and an anfra started by an agent may have none.

**Acceptance criteria:**
- [x] Tried, with canal (the DLL build) as the sidecar, and the results recorded: (a) anfra in a PowerShell window, sidecar with `CREATE_NEW_PROCESS_GROUP`; (b) the same plus `CREATE_NO_WINDOW`; (c) anfra started hidden with no console (`Start-Process -WindowStyle Hidden`, and as a detached process); for each: does a window appear, and does Ctrl-Break stop canal cleanly (its log says "shutting down", its socket is removed)?
- [x] The chosen flags (and, if needed, a hidden console for anfra) are in `proc_windows.go`, with a one-line comment saying why.
- [x] (Not needed: canal stops cleanly in every case.) If no option stops canal cleanly without a console, the fallback (terminate after the wait) is recorded, and the user told.

**Verification:**
- [x] Manual, on Windows: the cases above, results in `docs/research/windows-sidecar-transport.md` ("Later: stopping canal with Ctrl-Break"). With a console, sidecars share it; without one, they get `CREATE_NO_WINDOW` and stop attaches to their console to send Ctrl-Break.
- [x] On Windows: `TestCloseSendsCtrlBreak`, `TestCloseClosesStdin`, and the new `TestCloseSendsCtrlBreakWithoutConsole` pass.

**Dependencies:** T2

**Files likely touched:** `internal/sidecar/proc_windows.go`, a small check program (spike branch)

**Estimated scope:** S

### - [x] T4: Job Object: sidecars die with anfra

**Description:** Replace Linux's `Pdeathsig` on Windows: every sidecar joins one Job Object that anfra holds, with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, so Windows kills them when anfra ends, however it ends. The stop's last step terminates through the job.

**Acceptance criteria:**
- [x] A Job Object created on the first sidecar start (once per anfra process), set to kill on close; each sidecar assigned right after `Start` (`windows.AssignProcessToJobObject`).
- [x] T1's final "terminate" step terminates the sidecar's process tree. Changed: it stays `Process.Kill`, which ends the sidecar alone. Terminating the shared job would end every sidecar, and neither anfra-node nor canal starts children; any child left stays in the job and dies when anfra ends.
- [x] Processes started through `detachProcess` aren't in the job (anfra itself isn't in it, so they don't inherit it).

**Verification:**
- [x] Manual, on Windows: a `_windows_test.go` helper starts a sidecar from a child process, kills that child with `TerminateProcess`, and checks the sidecar is gone within 1 s; and `taskkill /F` on `anfra.exe` during `anfra query` leaves no `anfra-node` or `anfra-canal`. `TestSidecarDiesWithHost` passes (gone in 0.02 s), and fails when the job is turned off. The `taskkill /F` check moves to T10, which has the sidecars.
- [x] Build: `GOOS=windows go build ./...`.

**Dependencies:** T3

**Files likely touched:** `internal/sidecar/proc_windows.go`, `stop_windows.go`, `job_windows.go` (+ test)

**Estimated scope:** M

### - [x] T5: Windows unpack: a folder per version, `.exe`, pruning that skips folders in use

**Description:** On Windows, unpack each sidecar version into `sidecars\<name>-<hash>\<name>.exe`, so it runs and sits beside other versions; prune old folders, skipping any still in use. Linux and macOS are unchanged.

**Acceptance criteria:**
- [x] `unpack_windows.go`: the folder layout, the `.exe` name, the hash over every file the folder holds (only the `.exe` until T6), a temp name per writer, and a prune that tries to remove each older folder and leaves it when Windows refuses (in use).
- [x] `unpack_other.go` keeps today's `extractRuntime` behaviour; a Linux test pins `sidecars/<name>-<hash>` and the prune.
- [x] `binary.go`'s `Binary` and `Unpack` return the `.exe`'s path on Windows.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/sidecar/` (`TestExtractRuntimeLayout`).
- [x] Manual, on Windows (`_windows_test.go`): two versions unpacked side by side; with the older `.exe` running, the prune leaves its folder, and removes it once it has exited. `TestExtractRuntimeKeepsRunningVersion` passes.

**Dependencies:** T3

**Files likely touched:** `internal/sidecar/binary.go`, `unpack_windows.go`, `unpack_other.go` (+ tests)

**Estimated scope:** M

### - [x] T6: canal's `duckdb.dll`, embedded and unpacked beside it

**Description:** Windows builds of anfra carry DuckDB's `duckdb.dll` with canal (the `win-canal` decision), and unpack it into canal's version folder.

**Acceptance criteria:**
- [x] `internal/sidecar/anfracanal`: a Windows-only embed for `assets/duckdb.dll` (release builds, `embed_sidecar`), passed to the unpack with canal's bytes; dev builds embed nothing, as today (`ANFRA_CANAL_BIN`'s folder must hold `duckdb.dll`, as with any DLL build of canal).
- [x] The folder hash covers both files, so a new DLL is a new folder.
- [x] Linux/macOS embed and unpack unchanged. (The release pipeline, `build_release.yml`, doesn't copy `duckdb.dll` yet: that is `win-release`.)

**Verification:**
- [x] Build: `GOOS=windows GOARCH=amd64 go build -tags embed_sidecar ./cmd/anfra` with the spike's `anfra-canal.exe`, `duckdb.dll` and `anfra-node.exe` copied into the assets folders (not committed).
- [x] Manual, on Windows: that `anfra.exe` unpacks `anfra-canal-<hash>\{anfra-canal.exe, duckdb.dll}` and `anfra query` works with it. Done on a copy of `anfra-demo-data`: a SQL query and an AQL query (`orders | count(orders.id)`) both answer, `anfra-node-<hash>\anfra-node.exe` unpacks too, and no sidecar is left running. `TestExtractRuntimeLibraries` passes on Windows.

**Dependencies:** T5

**Files likely touched:** `internal/sidecar/anfracanal/anfracanal.go`, `embed_release_windows.go`, `embed_release.go` (tag), `embed_dev.go`

**Estimated scope:** S

### - [x] T7: `anfra serve`'s busy-port fallback on Windows

**Description:** Windows reports a taken port as `WSAEADDRINUSE`, which `syscall.EADDRINUSE` doesn't match, so serve fails instead of moving to a free port.

**Acceptance criteria:**
- [x] `isAddrInUse(err)` in `addrinuse_windows.go` / `addrinuse_unix.go` (the folder's naming); `serve.go:292` uses it.
- [x] On Windows, with the default port taken, `anfra serve` starts on a free one; with `--addr` on a taken port, it refuses with today's message.

**Verification:**
- [x] Tests pass (Linux): `go test ./cmd/anfra/`; new `TestIsAddrInUse` passes on Linux and Windows, as does `TestListen` on Windows.
- [x] Manual, on Windows: hold the default port with another listener, run `anfra serve`. Port 7878 was held by WSL's relay: serve listened on 127.0.0.1:62657; `--addr 127.0.0.1:7878` refused ("another program is already using it"). After `Stop-Process -Force` on serve, no sidecar was left (T4's job, with real sidecars).

**Dependencies:** T2

**Files likely touched:** `cmd/anfra/serve.go`, `addrinuse_windows.go`, `addrinuse_other.go`

**Estimated scope:** XS

### - [x] T8: The console's background server stops on Windows

**Description:** (From T3: the server started with `DETACHED_PROCESS` has no console to attach to; start it with `CREATE_NO_WINDOW` instead.) `consoleServer.stop` sends SIGTERM (`console_server.go:163`), which Windows can't deliver, so the server the console started keeps running for up to its idle timeout.

**Acceptance criteria:**
- [x] On Windows, `stop` ends the server another way (Ctrl-Break to its process group, which Go delivers as `os.Interrupt` and `anfra serve` already handles; or a shutdown call), in build-tagged code. Chosen: Ctrl-Break. On Windows the server is started on the console's own console (`CREATE_NEW_PROCESS_GROUP`, not `DETACHED_PROCESS`), since Ctrl-Break reaches only processes on the sender's console, and attaching to another console would take the console off the user's terminal. Closing the terminal ends the server too, as leaving the console does. `anfra update`'s background check keeps `detachProcess`.
- [x] On Windows: leaving `anfra console` stops the server it started, and its sidecars with it; a server the console found running is left alone, as today.

**Verification:**
- [x] Tests pass (Linux): `go test ./cmd/anfra/`; on Windows, new `TestStopServer` passes.
- [x] Manual, on Windows: start `anfra console`, run a query (it starts a server), leave; no `anfra serve`, `anfra-node` or `anfra-canal` left. Done in T10, by the user. Done without it: a check program started `anfra serve` as the console does, ran a query through it, and sent Ctrl-Break: serve exited in 0.1 s with code 0, and no anfra, anfra-node or anfra-canal was left.

**Dependencies:** T3

**Files likely touched:** `cmd/anfra/console_server.go`, `consolestop_windows.go`, `consolestop_other.go`

**Estimated scope:** S

### Checkpoint B: Each behaviour tested on Windows
- Found in T5: `TestSocketPath` fails on Windows ("dir mode = 777, want 700"). Go ignores Unix modes on Windows, so the socket's folder is private only when the temp folder is (not with a shared `TMP`, such as `C:\Temp`). Decided with the user: make it private on Windows too, as on Linux and macOS (T9).
- [x] T3–T8's Windows tests and manual checks pass on Windows 11 (T8's interactive console check moved to T10).
- [x] Linux: `go test ./internal/sidecar/... ./cmd/anfra/...` pass.
- [x] Review with the user before M1.

---

## Phase 3: M1 and close

### - [x] T9: The socket's folder is private on Windows

**Description:** On Linux and macOS, `SocketPath` makes the socket's folder readable by this user only (mode `700`), so another user on the machine can't reach the sidecar. Go ignores that mode on Windows, so there the folder is only as private as the temp folder it's in, which a shared `TMP` (such as `C:\Temp`) opens up. Make it private on Windows too.

**Acceptance criteria:**
- [x] On Windows, the folder is created with an access list that gives this user full access and no one else, and takes nothing from the folder above (protected), set as it's created, not after.
- [x] Linux and macOS unchanged (`os.MkdirTemp`, mode `700`), in build-tagged files.
- [x] `TestSocketPath` checks privacy per OS: mode `700` on Unix, the access list on Windows. A Windows test points `TMP` at a folder everyone may open, and the socket's folder is still private.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/sidecar/`.
- [x] On Windows: the socket tests pass; the sidecar tests and a real query through anfra-node and canal still work with the private folder. With the old code, both socket tests fail: in a shared `TMP`, the folder gave Everyone full access. A query through `anfra serve`, and one with `TMP` moved, both answer.

**Dependencies:** T3

**Files likely touched:** `internal/sidecar/socket.go`, `socket_unix.go`, `socket_windows.go`, `socket_test.go` (+ per-OS test files)

**Estimated scope:** S

### - [x] T10: M1 end to end on Windows 11

**Description:** The milestone: anfra runs on Windows, using the spikes' sidecars.

**Acceptance criteria:**
- [x] On a copy of `anfra-demo-data` on `C:`, with `ANFRA_NODE_BIN` and `ANFRA_CANAL_BIN` set: `anfra version`, `validate`, `show`, and `query` (without the cache, and with it, so canal-lake runs) work. The cache needs anfra-node 0.16.0 (the pinned one): with the spike's 0.13.1 it answers `cache_unavailable` (no compiler key). 0.16.0 was cross-compiled for Windows from the `anfra-node-v0.16.0` tag (Bun 1.2.10, `bun-windows-x64`): first run a miss, written to Parquet by canal-lake; second run a hit.
- [x] After each command: no `anfra-node` or `anfra-canal` left; `taskkill /F` on anfra during a query leaves none.
- [x] In `anfra console`: Ctrl-C during a query cancels it and leaves the sidecars running; leaving the console stops everything it started. Run by the user in a PowerShell window, twice; from the log: the console started a server, Ctrl-C cut the slow query short (2.7 s, where it runs over 4 s), the sidecars stayed until the session ended, then canal logged "shutting down Canal Query", and no anfra process was left.
- [x] Started with no console, anfra opens no windows (T3's case c). Run with `DETACHED_PROCESS`: the query answers, no new window, no sidecar left.
- [x] With T6's embedded build: the same `query` works from the unpacked folder (T6).

**Verification:**
- [x] Manual, on Windows 11: each step above, results recorded.

Found, for `win-anfra-node`: each command logs a `watchdog.parent_death` warning from anfra-node, because closing its stdin is how anfra stops it on Windows, and anfra-node reads that as its host gone. Noise, not a failure; anfra-node handling Ctrl-Break as a normal stop would end it.

**Dependencies:** T4–T9

**Files likely touched:** a results note (PR description or the findings doc)

**Estimated scope:** S

### - [x] T11: Docs, plan, and the Linux checks

**Description:** Close the module.

**Acceptance criteria:**
- [x] `docs/designs/sidecars.md`: a Windows section (process group and console flags, the stop sequence, the Job Object, the folder layout with `duckdb.dll`).
- [x] `WINDOWS-PLAN.md`: Where it stands marks `win-process-lifecycle` done; the Summary's status and M1 are updated; its work rows reflect what was learned.
- [x] With the user's go-ahead: `make check` and `make test` pass on Linux. `make check` passes. `make test` passes but for `engine`'s `TestOnlyTheAPIPackagesAreExported`, which walks every folder and counts the spikes' ignored worktrees in `tmp/` (each its own module); in a clean checkout of the branch, `engine` passes.

**Verification:**
- [x] Manual: the spec's Success Criteria 1–7 are each met, ticked here.
  - [x] 1. Builds and vets for Windows (Checkpoint A).
  - [x] 2. Linux: tests, `make check`, `make test` pass (above); the layout is pinned by `TestExtractRuntimeLayout`, process handling unchanged (`stop_unix.go`).
  - [x] 3. `version`, `validate`, `show`, `query` with and without the cache on Windows 11 (T10).
  - [x] 4. No sidecar outlives anfra; Ctrl-C in the console keeps them; no windows without a console (T3, T4, T10).
  - [x] 5. A folder per version, `duckdb.dll` beside canal, the running one kept (T5, T6).
  - [x] 6. Serve's free port (T7), the console's server stops (T8, T10). The console's restart after an update is covered by `TestRestartWith` only; the end-to-end check, with a released Windows build, moves to `win-self-update` (agreed with the user).
  - [x] 7. `docs/designs/sidecars.md` describes the Windows lifecycle and layout.

**Dependencies:** T10

**Files likely touched:** `docs/designs/sidecars.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint C: Complete
- [x] Spec success criteria 1–7 met.
- [ ] Ready for review with the user (and the commit split proposed).
