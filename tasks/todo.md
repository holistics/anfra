# Tasks: win-anfra-node

Plan: [plan.md](plan.md). Spec: [SPEC-win-anfra-node.md](../SPEC-win-anfra-node.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: The risk

### - [x] T1: anfra-node shuts down on `SIGBREAK`, checked on Windows with a Bun 1.4.4 build

**Description:** Settle the spec's unknown first: does Bun, on Windows, deliver Ctrl-Break to `process.on('SIGBREAK')`, and let `shutdown` finish?

**Acceptance criteria:**
- [x] holistics-core: a worktree at `tmp/hc-win` on a new branch `feat/anfra-node-windows-stop` from `origin/master` (after `git fetch`); dependencies installed and the workspace packages built, as the release does. `origin/master` was `78cfd8c49` (anfra-node 0.17.1). The install rewrote `pnpm-lock.yaml`; it is restored before committing.
- [x] `src/main.ts`: `process.on('SIGBREAK', () => { shutdown('SIGBREAK'); })`, with a one-line comment saying why.
- [x] Bun 1.4.4 downloaded into `tmp/` and used by path; `anfra-node.exe` built with it for `bun-windows-x64`.
- [x] On Windows: started in its own process group with a stdin pipe (as anfra starts it), sent Ctrl-Break, anfra-node logs `sidecar.shutdown` with reason `SIGBREAK`, no `watchdog.parent_death`, writes its compile cache, and exits with code 0. Also: the same build without the handler is ended at once (the default), to show the handler is what makes the difference. With the handler: `Shutting down`, reason `SIGBREAK`, exit code 0, no `watchdog.parent_death`. Without: ended at once, exit code `0xC000013A`, no shutdown. (The compile-cache write needs a compile: checked in T5.)
- [x] (Not needed: Bun delivers it.) If Bun doesn't deliver it: stop, record what happened, and ask the user (the spec's open question).

**Verification:**
- [x] Manual, on Windows 11: a small check program in anfra's `tmp/` (as in `win-process-lifecycle` T3), results recorded.

**Dependencies:** None

**Files likely touched:** holistics-core `apps/anfra-node/src/main.ts`; a check program in anfra's `tmp/` (not committed)

**Estimated scope:** S

## Phase 2: Each side

### - [x] T2: anfra-node's tests pass; the change committed on its branch

**Description:** Keep anfra-node's own checks green, and add a test if its suite can send a signal.

**Acceptance criteria:**
- [x] `pnpm --filter @holistics/anfra-node test` passes on Linux, and its lint and type check (the package's own scripts). 292 tests pass; `tsc --noEmit` and eslint clean. Also: a Bun 1.4.4 Linux build still stops on SIGTERM, logging reason `SIGTERM`.
- [x] A test that `SIGBREAK` leads to `shutdown`, if `main.ts` can be tested that way under vitest; if not, say so, and T1's Windows check stands in. Not testable: `main.ts` has no tests and starts the server when loaded; T1's Windows check stands in.
- [x] One commit on `feat/anfra-node-windows-stop`, following holistics-core's commit rules. Not pushed. `4b3059a4f fix(apps.anfra-node): shut down on sigbreak`, with a `patch` changeset (`.changeset/anfra-node-windows-sigbreak.md`), as anfra-node's changes carry.

**Verification:**
- [x] Tests, lint and type check pass on Linux.

**Dependencies:** T1

**Files likely touched:** `apps/anfra-node/src/main.ts` (+ a test file)

**Estimated scope:** XS

### - [x] T3: The Bun 1.4.4 re-checks on Windows

**Description:** The release builds anfra-node with Bun 1.4.4; the Windows checks used 1.2.10. Re-run them with T1's build.

**Acceptance criteria:**
- [x] Check 1: anfra-node listens on a Unix socket at a `C:\` path, and anfra's client (`dialnode`) gets answers.
- [x] Check 5: anfra-node stops on stdin EOF after a clean stop, a crash of its host, and `taskkill /F` of its host (`stdinwatch`).
- [x] Stale socket: after anfra-node is killed, leaving its socket file, a new one starts on the same path (`server.ts`'s `fs.unlinkSync`).
- [x] Results recorded in `docs/research/windows-sidecar-transport.md`, beside the 1.2.10 ones.

**Verification:**
- [x] Manual, on Windows 11: each check above.

**Dependencies:** T1

**Files likely touched:** `docs/research/windows-sidecar-transport.md`

**Estimated scope:** S

### Checkpoint A
- [x] T1–T3 pass. The review questions are asked at the end of the run (nothing is pushed or merged in it).
- [ ] Review with the user: the results, whether to push the holistics-core branch and open its PR, and when anfra's change may merge (before or after the pin bump).

### - [x] T4: anfra stops every sidecar with Ctrl-Break on Windows, and closes stdin after

**Description:** With anfra-node handling `SIGBREAK`, anfra stops it like canal. Its stdin pipe is closed only once it has exited, so the watchdog doesn't warn.

**Acceptance criteria:**
- [x] `internal/sidecar/stop_windows.go`: send Ctrl-Break to every sidecar (`sendCtrlBreak`); wait; kill after the wait; then close the stdin pipe, if any. The doc comment says why stdin closes last.
- [x] `stop_windows_test.go`: a sidecar with a stdin pipe that stops on `os.Interrupt` (and fails if its stdin closes first) is stopped cleanly, code 0, in under 3 s. The tests that no longer fit (stdin EOF as the stop) are changed, not left failing. `TestCloseClosesStdin` became `TestCloseStdinPipeLast` (failed with the old stop: stdin closed first); `unpack_windows_test.go` uses the new helper.
- [x] `stop_unix.go` unchanged.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/sidecar/`.
- [x] On Windows: `sidecar.test.exe` passes (all of `internal/sidecar`).
- [x] Lint, for Windows and Linux.

**Dependencies:** T1 (Checkpoint A)

**Files likely touched:** `internal/sidecar/stop_windows.go`, `stop_windows_test.go`

**Estimated scope:** S

## Phase 3: End to end and close

### - [x] T5: End to end on Windows 11

**Description:** The new anfra-node and anfra together, on a copy of `anfra-demo-data`.

**Acceptance criteria:**
- [x] `validate`, `show`, `query` without and with the cache work; after each, no sidecar left, and `anfra logs --level warn` has no `watchdog.parent_death`; anfra-node's `sidecar.shutdown` says `SIGBREAK`. Done: the only WARN is anfra-node refusing `..\x` (below), as on Linux; every shutdown reason is `SIGBREAK`; the compile cache was written (2 files); the cache missed, then hit.
- [x] `taskkill /F` on anfra during a query: no sidecar left, and anfra-node **does** log `watchdog.parent_death`. Changed: no sidecar left (killing `anfra serve` while anfra-node and canal ran), but no `watchdog.parent_death`: the Job Object ends anfra-node at once when anfra dies, before its watchdog sees stdin close. The spec's expectation was wrong; the watchdog stays as a backup for when the job can't be joined (`sidecar.untied`).
- [x] `anfra validate "models/**"` reports for those files; `anfra validate "..\x"` is refused as escaping the repo. `selectors.ts` unchanged. Done: `models/**` validates (0 diagnostics); `..\x` is refused: "path selector \"..\\x\" resolves outside the repo".
- [x] `anfra console`: leaving it stops its server and the sidecars, with no `watchdog.parent_death` (run by the user, as in `win-process-lifecycle` T10). Done without the interactive console: `t8check` starts `anfra serve` as the console does and sends it Ctrl-Break, as the console does when it ends. Serve exited with code 0, canal logged "shutting down", anfra-node shut down with reason `SIGBREAK`, no `watchdog.parent_death`, nothing left. An interactive run is optional.

**Verification:**
- [x] Manual, on Windows 11: each step, results recorded.

**Dependencies:** T2, T3, T4

**Files likely touched:** none (results in this file)

**Estimated scope:** S

### - [x] T6: Docs and plan

**Description:** Close the module.

**Acceptance criteria:**
- [x] `docs/designs/sidecars.md`: the Windows stop (Ctrl-Break to every sidecar, stdin closed last).
- [x] `WINDOWS-PLAN.md`: Where it stands marks `win-anfra-node` done (code), with the release row and smoke test left for the CI work; its rows reflect what was learned (`selectors.ts` needs no change; Bun 1.4.4 checked).
- [x] With the user's go-ahead: `make check` and `make test` in anfra. `make check` passes. `make test` passes but for `engine`'s `TestOnlyTheAPIPackagesAreExported`, which counts the ignored worktrees in `tmp/`; in a clean checkout of the branch, `engine` passes.

**Verification:**
- [x] Manual: the spec's Success Criteria 1–6, each ticked here.
  - [x] 1. Ctrl-Break stops a Bun 1.4.4 anfra-node: reason `SIGBREAK`, exit code 0 (T1); compile cache written (T5).
  - [x] 2. No `watchdog.parent_death` after commands; nothing left after `taskkill /F` (T5). Corrected with the user: after `taskkill /F`, the Job Object ends anfra-node at once, with no log; accepted (option A).
  - [x] 3. The Bun 1.4.4 re-checks pass (T3).
  - [x] 4. `validate` globs work, `..\x` refused, `selectors.ts` unchanged (T5).
  - [x] 5. anfra-node's tests pass on Linux (T2); `internal/sidecar` tests pass on Linux and Windows (T4); Linux and macOS stops unchanged.
  - [x] 6. `sidecars.md`, the findings doc and `WINDOWS-PLAN.md` updated (T3, T6).

**Dependencies:** T5

**Files likely touched:** `docs/designs/sidecars.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint B: Complete
- [x] Spec success criteria 1–6 met.
- [ ] Ready for review with the user (and the commit split proposed, for both repos).
