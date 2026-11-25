# Implementation Plan: win-sidecar-transport

Spec: [SPEC-win-sidecar-transport.md](../../SPEC-win-sidecar-transport.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Task checklist: [win-sidecar-transport-todo.md](win-sidecar-transport-todo.md).

## Overview

What this module answers and why: the spec's [Objective](../../SPEC-win-sidecar-transport.md#objective). This file holds only how it's built: the task order, the gates, and task-level decisions and risks. Each task is one check run end to end (build it, run it on Windows, record the result), plus a fix task when a check fails.

## Architecture Decisions

- **Check programs must not import `internal/sidecar` or anything that imports it** (`internal/sidecar/anfranode`, `canalquery`). That package doesn't compile for Windows yet (`proc_other.go` Setpgid, `process.go` syscall.Kill); `win-process-lifecycle` fixes that later. Each check copies the few lines it needs instead, e.g. the dialer from `anfranode.NewClientUnix` (`internal/sidecar/anfranode/client.go:32`) and the folder logic from `SocketPath` (`internal/sidecar/socket.go:17`).
- **Use the repos' existing tests as checks where they already test the behaviour.** canal's `TestListenOnStaleOrLiveSocket` (`internal/shared/utils/listen_test.go`) is check 3 as written. Cross-build it for Windows with `go test -c` and run it there; no new program.
- **Check programs live on branches, never merged:** anfra `spike/win-sidecar-transport` (folder `dev/wincheck/`) for checks 1, 4, 5; canal `spike/win-sidecar-transport` (folder `dev/wincheck/`) for check 2, since it needs canal's Flight client.
- **The riskiest check runs first.** Check 1 (anfra-node's `node:http` on Bun) decides the transport for the whole project. If it fails, stop and re-review before doing anything else.
- **A fix is written only when its check fails.** Fix tasks T7–T9 are skipped, and marked skipped, when their check passes.

## Dependency graph

```
T1 findings doc + branches
 └─ T2 check 1: anfra → anfra-node (node:http)        ← gate: fail = stop, re-plan
     ├─ T3 check 4: socket path length ── T9 fix SocketPath (if failed)
     ├─ T4 check 2: gRPC unix:C:\ ──────── T7 fix lakeclient.Location (if failed)
     ├─ T5 check 3: canal Listen restart ── T8 fix canal Listen (if failed)
     └─ T6 check 5: anfra-node stdin watchdog (record only)
          └─ T10 decision + findings doc final  (after T3–T9)
```

T3–T6 are independent and can run in one Windows session. T7–T9 are independent of each other.

## Task List

### Phase 1: Setup
- [x] T1: Findings doc skeleton and spike branches

### Phase 2: The deciding check
- [x] T2: Check 1, anfra → anfra-node over `node:http`

### Checkpoint A: Transport for anfra-node decided
- [x] Check 1 passed → continue. Check 1 failed → stop, update the spec and WINDOWS-PLAN.md for the fallback transport, and review with the user.

### Phase 3: The other checks (one Windows session)
- [x] T3: Check 4, socket path length
- [x] T4: Check 2, canal-query → canal-lake gRPC on a Windows path
- [x] T5: Check 3, canal `Listen` restarts on a leftover socket
- [x] T6: Check 5, anfra-node's stdin watchdog

### Checkpoint B: All results in
- [x] Every check has a recorded result; the fixes needed are known (none). Review with the user before writing fixes.

### Phase 4: Fixes (only for failed checks)
- [x] T7: canal `lakeclient.Location` builds an address grpc-go reads on Windows (skipped: T4 passed)
- [x] T8: canal `Listen` treats a leftover Windows socket file as stale (skipped: T5 passed)
- [x] T9: anfra `SocketPath` keeps the path short on Windows (skipped: T3 passed)

### Phase 5: Close
- [x] T10: Decision, findings doc final, plan updated

### Checkpoint C: Complete
- [x] Spec success criteria 1–5 met; no code changed in any repo, so CI is unaffected; ready for review.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Bun's `node:http` (on `Bun.serve`) makes a named pipe, not an AF_UNIX socket, for a file path | High: a fallback transport for anfra-node, +2–4 ED | It's T2, run first. Before switching to pipes, try `node:net` + a small HTTP layer, or `Bun.listen`, which the research shows make AF_UNIX. |
| Go doesn't report `ModeSocket` for an AF_UNIX file on Windows, so canal's fix for T8 conflicts with `TestListenKeepsNonSocketFile` (a non-socket file must be left alone) | Medium | Windows marks socket files with the reparse tag `IO_REPARSE_TAG_AF_UNIX`. Check that tag in a `listen_windows.go`, rather than removing any file that fails to dial. |
| A fix in `internal/sidecar` (T9) can't be tested on Windows yet, because the package doesn't compile there until `win-process-lifecycle` | Medium | Test the path choice as a plain function on Linux. Re-run check 4's program, with the fixed logic copied in, on Windows. Add the Windows-only test when the package compiles. |
| The Windows machine has AF_UNIX turned off, or is older than Windows 10 1803 | Low | Record the Windows build in the findings doc. `sc query afunix` shows whether the driver runs. |
| Microsoft Defender quarantines or slows the unsigned check `.exe` files | Low | Run from a folder excluded from scanning, or note the delay; signing comes in `win-release`. |

## Open Questions

None. Any Windows 10 1803+ or Windows 11 x64 machine can run the checks; the findings doc records which one did.
