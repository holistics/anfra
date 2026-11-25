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

### - [ ] T3: Stopping sidecars when anfra has no console (Ctrl-Break vs "no window")

**Description:** Settle the spec's risk before building on it. Ctrl-Break only reaches processes that share the caller's console; a sidecar started with `CREATE_NO_WINDOW` gets its own, and an anfra started by an agent may have none.

**Acceptance criteria:**
- [ ] Tried, with canal (the DLL build) as the sidecar, and the results recorded: (a) anfra in a PowerShell window, sidecar with `CREATE_NEW_PROCESS_GROUP`; (b) the same plus `CREATE_NO_WINDOW`; (c) anfra started hidden with no console (`Start-Process -WindowStyle Hidden`, and as a detached process); for each: does a window appear, and does Ctrl-Break stop canal cleanly (its log says "shutting down", its socket is removed)?
- [ ] The chosen flags (and, if needed, a hidden console for anfra) are in `proc_windows.go`, with a one-line comment saying why.
- [ ] If no option stops canal cleanly without a console, the fallback (terminate after the wait) is recorded, and the user told.

**Verification:**
- [ ] Manual, on Windows: the three cases above, results in the PR description (or a short note in `docs/research/windows-sidecar-transport.md`).

**Dependencies:** T2

**Files likely touched:** `internal/sidecar/proc_windows.go`, a small check program (spike branch)

**Estimated scope:** S

### - [ ] T4: Job Object: sidecars die with anfra

**Description:** Replace Linux's `Pdeathsig` on Windows: every sidecar joins one Job Object that anfra holds, with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, so Windows kills them when anfra ends, however it ends. The stop's last step terminates through the job.

**Acceptance criteria:**
- [ ] A Job Object created on the first sidecar start (once per anfra process), set to kill on close; each sidecar assigned right after `Start` (`windows.AssignProcessToJobObject`).
- [ ] T1's final "terminate" step terminates the sidecar's process tree.
- [ ] Processes started through `detachProcess` aren't in the job.

**Verification:**
- [ ] Manual, on Windows: a `_windows_test.go` helper starts a sidecar from a child process, kills that child with `TerminateProcess`, and checks the sidecar is gone within 1 s; and `taskkill /F` on `anfra.exe` during `anfra query` leaves no `anfra-node` or `anfra-canal`.
- [ ] Build: `GOOS=windows go build ./...`.

**Dependencies:** T3

**Files likely touched:** `internal/sidecar/proc_windows.go`, `stop_windows.go`, `job_windows.go` (+ test)

**Estimated scope:** M

### - [ ] T5: Windows unpack: a folder per version, `.exe`, pruning that skips folders in use

**Description:** On Windows, unpack each sidecar version into `sidecars\<name>-<hash>\<name>.exe`, so it runs and sits beside other versions; prune old folders, skipping any still in use. Linux and macOS are unchanged.

**Acceptance criteria:**
- [ ] `unpack_windows.go`: the folder layout, the `.exe` name, the hash over every file the folder holds, a temp name per writer, and a prune that tries to remove each older folder and leaves it when Windows refuses (in use).
- [ ] `unpack_other.go` keeps today's `extractRuntime` behaviour; a Linux test pins `sidecars/<name>-<hash>` and the prune.
- [ ] `binary.go`'s `Binary` and `Unpack` return the `.exe`'s path on Windows.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/sidecar/`.
- [ ] Manual, on Windows (`_windows_test.go`): two versions unpacked side by side; with the older `.exe` running, the prune leaves its folder, and removes it once it has exited.

**Dependencies:** T3

**Files likely touched:** `internal/sidecar/binary.go`, `unpack_windows.go`, `unpack_other.go` (+ tests)

**Estimated scope:** M

### - [ ] T6: canal's `duckdb.dll`, embedded and unpacked beside it

**Description:** Windows builds of anfra carry DuckDB's `duckdb.dll` with canal (the `win-canal` decision), and unpack it into canal's version folder.

**Acceptance criteria:**
- [ ] `internal/sidecar/anfracanal`: a Windows-only embed for `assets/duckdb.dll` (release builds, `embed_sidecar`), passed to the unpack with canal's bytes; dev builds embed nothing, as today (`ANFRA_CANAL_BIN`'s folder must hold `duckdb.dll`, as with any DLL build of canal).
- [ ] The folder hash covers both files, so a new DLL is a new folder.
- [ ] Linux/macOS embed and unpack unchanged.

**Verification:**
- [ ] Build: `GOOS=windows GOARCH=amd64 go build -tags embed_sidecar ./cmd/anfra` with the spike's `anfra-canal.exe`, `duckdb.dll` and `anfra-node.exe` copied into the assets folders (not committed).
- [ ] Manual, on Windows: that `anfra.exe` unpacks `anfra-canal-<hash>\{anfra-canal.exe, duckdb.dll}` and `anfra query` works with it.

**Dependencies:** T5

**Files likely touched:** `internal/sidecar/anfracanal/anfracanal.go`, `embed_release_windows.go`, `embed_release.go` (tag), `embed_dev.go`

**Estimated scope:** S

### - [ ] T7: `anfra serve`'s busy-port fallback on Windows

**Description:** Windows reports a taken port as `WSAEADDRINUSE`, which `syscall.EADDRINUSE` doesn't match, so serve fails instead of moving to a free port.

**Acceptance criteria:**
- [ ] `isAddrInUse(err)` in `addrinuse_windows.go` / `addrinuse_other.go`; `serve.go:292` uses it.
- [ ] On Windows, with the default port taken, `anfra serve` starts on a free one; with `--addr` on a taken port, it refuses with today's message.

**Verification:**
- [ ] Tests pass (Linux): `go test ./cmd/anfra/ -run Serve` (behaviour unchanged).
- [ ] Manual, on Windows: hold the default port with another listener, run `anfra serve`.

**Dependencies:** T2

**Files likely touched:** `cmd/anfra/serve.go`, `addrinuse_windows.go`, `addrinuse_other.go`

**Estimated scope:** XS

### - [ ] T8: The console's background server stops on Windows

**Description:** `consoleServer.stop` sends SIGTERM (`console_server.go:163`), which Windows can't deliver, so the server the console started keeps running for up to its idle timeout.

**Acceptance criteria:**
- [ ] On Windows, `stop` ends the server another way (Ctrl-Break to its process group, which Go delivers as `os.Interrupt` and `anfra serve` already handles; or a shutdown call), in build-tagged code.
- [ ] On Windows: leaving `anfra console` stops the server it started, and its sidecars with it; a server the console found running is left alone, as today.

**Verification:**
- [ ] Tests pass (Linux): `go test ./cmd/anfra/ -run Console`.
- [ ] Manual, on Windows: start `anfra console`, run a query (it starts a server), leave; no `anfra serve`, `anfra-node` or `anfra-canal` left.

**Dependencies:** T3

**Files likely touched:** `cmd/anfra/console_server.go`, `consolestop_windows.go`, `consolestop_other.go`

**Estimated scope:** S

### Checkpoint B: Each behaviour tested on Windows
- [ ] T3–T8's Windows tests and manual checks pass on Windows 11.
- [ ] Linux: `go test ./internal/sidecar/... ./cmd/anfra/...` pass.
- [ ] Review with the user before M1.

---

## Phase 3: M1 and close

### - [ ] T9: M1 end to end on Windows 11

**Description:** The milestone: anfra runs on Windows, using the spikes' sidecars.

**Acceptance criteria:**
- [ ] On a copy of `anfra-demo-data` on `C:`, with `ANFRA_NODE_BIN` and `ANFRA_CANAL_BIN` set: `anfra version`, `validate`, `show`, and `query` (without the cache, and with it, so canal-lake runs) work.
- [ ] After each command: no `anfra-node` or `anfra-canal` left; `taskkill /F` on anfra during a query leaves none.
- [ ] In `anfra console`: Ctrl-C during a query cancels it and leaves the sidecars running; leaving the console stops everything it started.
- [ ] Started with no console, anfra opens no windows (T3's case c).
- [ ] With T6's embedded build: the same `query` works from the unpacked folder.

**Verification:**
- [ ] Manual, on Windows 11: each step above, results recorded.

**Dependencies:** T4–T8

**Files likely touched:** a results note (PR description or the findings doc)

**Estimated scope:** S

### - [ ] T10: Docs, plan, and the Linux checks

**Description:** Close the module.

**Acceptance criteria:**
- [ ] `docs/designs/sidecars.md`: a Windows section (process group and console flags, the stop sequence, the Job Object, the folder layout with `duckdb.dll`).
- [ ] `WINDOWS-PLAN.md`: Where it stands marks `win-process-lifecycle` done; the Summary's status and M1 are updated; its work rows reflect what was learned.
- [ ] With the user's go-ahead: `make check` and `make test` pass on Linux.

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–7 are each met, ticked here.

**Dependencies:** T9

**Files likely touched:** `docs/designs/sidecars.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint C: Complete
- [ ] Spec success criteria 1–7 met.
- [ ] Ready for review with the user (and the commit split proposed).
