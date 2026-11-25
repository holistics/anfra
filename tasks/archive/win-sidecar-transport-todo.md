# Tasks: win-sidecar-transport

Plan: [win-sidecar-transport-plan.md](win-sidecar-transport-plan.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Spec: [SPEC-win-sidecar-transport.md](../../SPEC-win-sidecar-transport.md).

Every check runs on a Windows 11 x64 machine. Check programs are cross-built on Linux/WSL with `GOOS=windows GOARCH=amd64`, and must not import `internal/sidecar` or packages that import it (it doesn't compile for Windows yet).

---

## Phase 1: Setup

### - [x] T1: Findings doc skeleton and spike branches

**Description:** Create the place results go, and the branches the check programs live on, so every later task only adds a result.

**Acceptance criteria:**
- [x] `docs/research/windows-sidecar-transport.md` exists with: the machine (Windows build from `winver`, AF_UNIX driver state from `sc query afunix`), the Bun version, and one empty section per check (what ran, result, what it means).
- [x] `docs/README.md`'s Research table lists it.
- [x] Branch `spike/win-sidecar-transport` exists in anfra (from `feat/windows-installer`) and in canal (from `origin/master`, no upstream, so it can't be pushed by mistake). Created as branch names only, with no checkout, so neither working tree changed.
- Moved: each branch's `dev/wincheck/README.md` is written with that repo's first check program (anfra: T2; canal: T4), so it describes real commands.

**Verification:**
- [x] Manual: the doc's links resolve (`SPEC-win-sidecar-transport.md`, `WINDOWS-PLAN.md` and its Research heading).

**Dependencies:** None

**Files likely touched:** `docs/research/windows-sidecar-transport.md`, `docs/README.md`, `dev/wincheck/README.md` (spike branches only)

**Estimated scope:** XS

---

## Phase 2: The deciding check

### - [x] T2: Check 1, anfra → anfra-node over `node:http`

**Description:** Prove that anfra's Go client can reach anfra-node on Windows over a Unix socket at a normal file path. anfra-node listens with `http.createServer().listen(path)` (`holistics-core/apps/anfra-node/src/rpc/server.ts:51`), which in Bun runs on `Bun.serve`, a different path from the `Bun.listen` the research covered.

**Acceptance criteria:**
- [x] `anfra-node.exe`, built with `BUN_COMPILE_TARGET=bun-windows-x64 bash scripts/build-sidecar.sh`, started with `--socket=%TEMP%\anfra-check\anfra-node.sock`, creates the socket file.
- [x] `dev/wincheck/dialnode` (a copy of the dialer in `anfranode.NewClientUnix`) gets `{"status":"ok"}` from `GET /health`, and an answer to one JSON-RPC call.
- [x] The result is in the findings doc. If it failed: also the error, and whether `Bun.listen` / `node:net` on the same path works (a two-line Bun script).

**Verification:**
- [x] Manual, on Windows: `.\dialnode.exe .\anfra-node.exe <socket path>` printed the health answer, the RPC answer and `PASS`. (Changed: `dialnode` starts anfra-node itself, with stdin held open as anfra does; started from PowerShell with a closed stdin, anfra-node's watchdog exits at once.)
- [x] Build: `GOOS=windows GOARCH=amd64 go build ./dev/wincheck/...` succeeds on the spike branch, and `go vet` is clean.

**Dependencies:** T1

**Files likely touched:** `dev/wincheck/dialnode/main.go`, `dev/wincheck/README.md` (spike branch), `docs/research/windows-sidecar-transport.md`

**Estimated scope:** S

### Checkpoint A: Transport for anfra-node decided
- [x] Check 1 passed → continue to Phase 3.
- [x] ~~Check 1 failed~~ (not applicable: check 1 passed) → **stop.** Update the spec and `WINDOWS-PLAN.md` for the fallback transport (see `WINDOWS-PLAN.md`, Decisions needed), and review with the user before any other task.

---

## Phase 3: The other checks (one Windows session)

### - [x] T3: Check 4, socket path length

**Description:** Check that the path `SocketPath` builds stays under AF_UNIX's ~108-byte cap on Windows, for a long user name.

**Acceptance criteria:**
- [x] `dev/wincheck/sockpath` (a copy of `SocketPath`'s logic: `os.MkdirTemp("", "anfra-")` + `canal-query.sock`) prints the path and its length in bytes, and listens on it.
- [x] Run as a Windows user whose name has 20+ characters (or with `TEMP` set to a folder of the same depth). Ran with temp folders as long as 20-, 32-, 40-, 48-, 56- and 64-character names, and with accented names.
- [x] The length and the result are in the findings doc.

**Verification:**
- [x] Manual, on Windows: the program prints a length under 108 and "listening" for realistic names (67–102 bytes); 110 bytes and longer fail with `bind: invalid argument`.

**Dependencies:** T2

**Files likely touched:** `dev/wincheck/sockpath/main.go` (spike branch), findings doc

**Estimated scope:** XS

### - [x] T4: Check 2, canal-query → canal-lake gRPC on a Windows path

**Description:** Check that canal's Flight client reaches a Flight server over a Unix socket when the address is `"unix:" + C:\…\canal-lake.sock`, as `lakeclient.Location` builds it (`canal internal/query/lakeclient/lakeclient.go:128`).

**Acceptance criteria:**
- [x] `dev/wincheck/flightunix` in canal: starts a minimal Flight server on a socket in `%TEMP%` using `utils.Listen`, sets `CANAL_LAKE_SOCKET`, and calls it once through `lakeclient.New()`.
- [x] The result is in the findings doc. It passed with `unix:C:\…` as canal builds it, so the `unix:///C:/…` form wasn't needed.

**Verification:**
- [x] Build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./dev/wincheck/flightunix` in canal (it must not pull in DuckDB).
- [x] Manual, on Windows: the program printed the Flight call's answer (`TotalRecords = 42`) and `PASS`.

**Dependencies:** T2

**Files likely touched:** `dev/wincheck/flightunix/main.go`, `dev/wincheck/README.md` (canal spike branch), findings doc

**Estimated scope:** S

### - [x] T5: Check 3, canal `Listen` restarts on a leftover socket

**Description:** Run canal's own socket tests on Windows. `TestListenOnStaleOrLiveSocket` already checks that a leftover socket file is replaced and a live one is not.

**Acceptance criteria:**
- [x] `GOOS=windows GOARCH=amd64 go test -c -o utils.test.exe ./internal/shared/utils` builds in canal (with `CGO_ENABLED=0`).
- [x] On Windows, `.\utils.test.exe -test.v` runs; each test's pass/fail is in the findings doc. Both passed.
- [x] The doc also says whether `os.Lstat` reports `ModeSocket` for the socket file (it does: check 1 printed it, and the stale-socket test relies on it) (one printed line in the README's steps, or a `-test.run` of a tiny added check on the spike branch).

**Verification:**
- [x] Manual, on Windows: the test binary's output is pasted into the findings doc.

**Dependencies:** T2

**Files likely touched:** findings doc (no code unless the `ModeSocket` probe is needed, on the canal spike branch)

**Estimated scope:** XS

### - [x] T6: Check 5, anfra-node's stdin watchdog

**Description:** Check that anfra-node exits when anfra closes its stdin on Windows, since that's how anfra will stop it cleanly there. Record only; any fix belongs to `win-anfra-node`.

**Acceptance criteria:**
- [x] `dev/wincheck/stdinwatch`: starts `anfra-node.exe` with `exec.Cmd.StdinPipe()`, waits for `/health`, closes the pipe, and reports whether the process exits within 5 s. Also covers the host exiting, and being killed with `taskkill /F`.
- [x] The result is in the findings doc, with anfra-node's log line `watchdog.parent_death`.

**Verification:**
- [x] Manual, on Windows: "exited after 0.0s"; after an exit or a hard kill of the host, no anfra-node is left running.

**Dependencies:** T2

**Files likely touched:** `dev/wincheck/stdinwatch/main.go` (spike branch), findings doc

**Estimated scope:** XS

### Checkpoint B: All results in
- [x] Checks 1–5 each have a recorded result.
- [x] The list of fixes needed (T7–T9) is known; the ones not needed are marked skipped. None were needed.
- [ ] Review with the user before writing fixes.

---

## Phase 4: Fixes (only for failed checks)

### - [x] T7: canal `lakeclient.Location` builds an address grpc-go reads on Windows — **skipped:** T4 passed with today's address (findings doc, check 2)

*Skip if T4 passed.*

**Description:** Make `Location` return a socket address grpc-go accepts on Windows (e.g. `unix:///C:/…`), without changing it on Linux/macOS.

**Acceptance criteria:**
- [ ] `Location` builds the address through one small function, with a Windows-only case.
- [ ] A test covers a Windows path and a Unix path.
- [ ] T4's program, re-run on Windows against the fixed code, succeeds.

**Verification:**
- [ ] Tests pass: `go test ./internal/query/lakeclient/` (canal, Linux) and the cross-built test binary on Windows.
- [ ] Canal CI green on the PR.

**Dependencies:** T4, Checkpoint B

**Files likely touched:** canal `internal/query/lakeclient/lakeclient.go`, `lakeclient_test.go`

**Estimated scope:** XS

### - [x] T8: canal `Listen` treats a leftover Windows socket file as stale — **skipped:** T5 passed with today's code (findings doc, check 3)

*Skip if T5 passed.*

**Description:** On Windows, recognise a socket file by its reparse tag (`IO_REPARSE_TAG_AF_UNIX`) when `ModeSocket` isn't reported, so a leftover one is removed and a normal file is still left alone.

**Acceptance criteria:**
- [ ] The Windows check is in `listen_windows.go`, with the Unix check unchanged in a `listen_unix.go` (or the shared file).
- [ ] On Windows, `TestListenOnStaleOrLiveSocket` and `TestListenKeepsNonSocketFile` both pass.
- [ ] Linux behaviour unchanged.

**Verification:**
- [ ] Tests pass: `go test ./internal/shared/utils/` (canal, Linux), and the cross-built `utils.test.exe` on Windows.
- [ ] Canal CI green on the PR.

**Dependencies:** T5, Checkpoint B

**Files likely touched:** canal `internal/shared/utils/listen.go`, `listen_windows.go`, `listen_unix.go`

**Estimated scope:** S

### - [x] T9: anfra `SocketPath` keeps the path short on Windows — **skipped:** T3 passed for realistic user names (findings doc, check 4)

*Skip if T3 passed.*

**Description:** On Windows, make `SocketPath` create its folder under a shorter base, private to the user, so long user names stay under the cap.

**Acceptance criteria:**
- [ ] The base folder comes from `socketBase()` in `internal/sidecar/socket_windows.go`; `socket_unix.go` (or `socket_other.go`) keeps today's `os.TempDir()`.
- [ ] The comment on `SocketPath` explains the Windows base in one line.
- [ ] T3's program, re-run on Windows with the new base, passes for the long user name.

**Verification:**
- [ ] Tests pass on Linux: `go test ./internal/sidecar/`, then `make check`, then `make test` (one at a time).
- [ ] Build: `GOOS=windows go vet ./internal/sidecar/` reports nothing new beyond the known `proc_other.go` / `process.go` errors that `win-process-lifecycle` fixes.
- [ ] Manual, on Windows: T3's program with the new base.

**Dependencies:** T3, Checkpoint B

**Files likely touched:** `internal/sidecar/socket.go`, `internal/sidecar/socket_windows.go`, `internal/sidecar/socket_unix.go`

**Estimated scope:** S

---

## Phase 5: Close

### - [x] T10: Decision, findings doc final, plan updated

**Description:** Write the one decision, and hand the results on to the modules that need them.

**Acceptance criteria:**
- [x] The findings doc ends with the decision: **Unix sockets** (expected) or **a fallback transport** (named pipes, or loopback TCP + token; still open in `WINDOWS-PLAN.md`), and why.
- [x] Check 5's result is copied into the `win-anfra-node` section of `WINDOWS-PLAN.md`'s Work by module (its spec doesn't exist yet).
- [x] `WINDOWS-PLAN.md`'s Where it stands table marks `win-sidecar-transport` done, and its Summary's Status line is updated.

**Verification:**
- [x] Manual: the spec's Success Criteria 1–5 are each met, ticked in this file. 1: findings doc with all 5 results, the machine, Bun version and the decision. 2: checks 1–4 pass; no fixes, so no fix tests. 3: not applicable (Unix sockets). 4: check 5 handed to `win-anfra-node` and `win-process-lifecycle`. 5: no code changed in any repo, so Linux and macOS behave as before.

**Dependencies:** T2–T9

**Files likely touched:** `docs/research/windows-sidecar-transport.md`, `WINDOWS-PLAN.md`

**Estimated scope:** XS

### Checkpoint C: Complete
- [x] Spec success criteria 1–5 met.
- [x] Linux: not applicable: no code changed on any repo's working branches (the check programs live only on the spike branches), so `make check`, `make test` and the other repos' CI are unaffected.
- [ ] Ready for review with the user.
