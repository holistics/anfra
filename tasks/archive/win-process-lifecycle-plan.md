# Implementation Plan: win-process-lifecycle

Spec: [SPEC-win-process-lifecycle.md](../../SPEC-win-process-lifecycle.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Task checklist: [todo.md](win-process-lifecycle-todo.md). Earlier modules' tasks: [archive/](./).

## Overview

What this module delivers and why: the spec's [Objective](../../SPEC-win-process-lifecycle.md#objective). This file holds how: the task order, the checkpoints, and task-level decisions and risks. First anfra compiles for Windows, then the riskiest behaviour (stopping sidecars without a console) is settled, then each behaviour is built and tested on its own, and M1 proves it all end to end on Windows 11.

## Architecture Decisions

- **Per-OS code in build-tagged files,** following `proc_linux.go` / `proc_other.go`: each Windows behaviour is a function with a `_windows.go` and a Unix version, called from the shared code. `proc_other.go` becomes `!linux && !windows`.
- **One stop rule per sidecar, chosen by how it can be told to stop,** not by its name: a sidecar started with `PipeStdin` (anfra-node) is stopped by closing its stdin; one without (canal) by Ctrl-Break to its process group. Sending both at once could cut anfra-node's own shutdown short (Windows' default Ctrl-Break handler ends a process at once). After the 3 s wait, either is terminated through the Job Object.
- **One Job Object per anfra process,** created on the first sidecar start, with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`; each sidecar is assigned to it right after `Start`. anfra itself isn't in it, so processes anfra detaches (the console's background server, the update check) aren't killed with it; they start through `detachProcess`, not `StartProcess`.
- **The Windows unpack layout is a folder per content hash,** `sidecars\<name>-<hash>\`, hashed over every file it holds (the `.exe` and, for canal, `duckdb.dll`), so a new DLL is a new folder. Linux and macOS keep `sidecars/<name>-<hash>`, unchanged, pinned by a test.
- **Test on Windows without CI:** Windows-only tests are built in WSL with `go test -c` and run on the Windows host, as in the spikes, until the Windows CI job exists (`win-release`, deferred).
- **The spike's sidecars run M1:** `anfra-node.exe` and the DLL build of `anfra-canal.exe` (+ `duckdb.dll`) in `C:\Users\nuita\wincheck`, through `ANFRA_NODE_BIN` / `ANFRA_CANAL_BIN` (dev builds embed nothing), and once embedded with the `embed_sidecar` tag.

## Dependency graph

```
T1 internal/sidecar compiles (process group, per-OS stop)
 └─ T2 cmd/anfra compiles (detach, console restart)          → Checkpoint A: GOOS=windows go build ./... passes
     └─ T3 stop without a console: Ctrl-Break vs "no window"  ← risk first; may change T1's flags
         ├─ T4 Job Object: sidecars die with anfra
         ├─ T5 Windows unpack: folder per version, .exe, prune
         │    └─ T6 canal's duckdb.dll embedded and unpacked
         ├─ T7 serve's busy-port fallback
         └─ T8 the console's background server stops      → Checkpoint B: each behaviour tested on Windows
              └─ T9 the socket's folder is private on Windows
                   └─ T10 M1 end to end on Windows 11
                        └─ T11 docs, plan, Linux checks      → Checkpoint C
```

T4–T8 are independent of each other once T3 is settled.

## Task List

### Phase 1: It compiles
- [x] T1: `internal/sidecar` compiles for Windows: process group and the per-OS stop
- [x] T2: `cmd/anfra` compiles for Windows: `detachProcess` and `restartConsole`

### Checkpoint A: It compiles
- [x] `GOOS=windows go build ./...` and `go vet ./...` pass; Linux tests of the touched packages pass.

### Phase 2: Behaviours, one at a time
- [x] T3: Stopping sidecars when anfra has no console (Ctrl-Break vs "no window")
- [x] T4: Job Object: sidecars die with anfra
- [x] T5: Windows unpack: a folder per version, `.exe`, pruning that skips folders in use
- [x] T6: canal's `duckdb.dll`, embedded and unpacked beside it
- [x] T7: `anfra serve`'s busy-port fallback on Windows
- [x] T8: The console's background server stops on Windows

### Checkpoint B: Each behaviour tested on Windows
- [x] T3–T8's Windows tests pass on Windows 11; Linux tests still pass. Review with the user.

### Phase 3: M1 and close
- [x] T9: The socket's folder is private on Windows (added at Checkpoint B)
- [x] T10: M1 end to end on Windows 11
- [x] T11: Docs, plan, and the Linux checks

### Checkpoint C: Complete
- [x] Spec success criteria 1–7 met; ready for review.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Ctrl-Break can't reach a sidecar started with `CREATE_NO_WINDOW`, or from an anfra with no console (started by an agent) | High: canal-lake's clean shutdown skipped (DuckDB files not flushed), or console windows popping up | T3, first: try each case. Options: `CREATE_NO_WINDOW` only when anfra has no console; or give anfra a hidden console of its own (`AllocConsole` + hide) to share; or, last, terminate after the wait |
| A sidecar starts a child before it's assigned to the Job Object (assignment follows `Start`) | Low: that child could outlive anfra | Neither anfra-node nor canal starts children at boot; note it in `sidecars.md`. Starting suspended and resuming after assignment is the fix if it ever matters |
| anfra itself runs inside a job (some terminals and IDEs put programs in one) | Low | Nested jobs are supported since Windows 8; T4's test runs anfra from PowerShell, which may already be in one |
| The folder layout breaks a Linux/macOS path assumption | Medium | The layout code is Windows-only by build tag; a Linux test pins `sidecars/<name>-<hash>` |
| Windows tests can only run by hand until CI exists | Medium: a regression could land unseen | Every Windows test is a `_windows_test.go`, ready for the Windows CI job; the PR lists the manual runs |

## Open Questions

None.
