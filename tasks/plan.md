# Implementation Plan: win-anfra-node

Spec: [SPEC-win-anfra-node.md](../SPEC-win-anfra-node.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Task checklist: [todo.md](todo.md). Earlier modules' tasks: [archive/](archive/).

## Overview

What this module delivers and why: the spec's [Objective](../SPEC-win-anfra-node.md#objective). This file holds how: the task order, the checkpoints, and task-level decisions and risks. The unknown comes first (does Bun deliver `SIGBREAK` on Windows?), because the rest of the stop change depends on it. Then the Bun 1.4.4 re-checks, then anfra's side of the stop, then everything end to end on Windows 11.

## Architecture Decisions

- **Two repos, two branches.** anfra-node's change goes on a new holistics-core branch, `feat/anfra-node-windows-stop`, from its latest `master`, in a worktree at anfra's ignored `tmp/hc-win`. Your `../holistics-core` checkout only gets a `git fetch`. anfra's change goes on `feat/windows-installer`, as before. Nothing is pushed without asking.
- **Bun 1.4.4, as the release.** Your machine has Bun 1.2.10. 1.4.4 is downloaded into `tmp/` (the official release zip) and used only through its path, so the installed Bun stays as it is.
- **anfra's stop, on Windows:** Ctrl-Break to every sidecar; a sidecar's stdin pipe is closed only after it has exited or been killed. It replaces today's choice ("stdin pipe → close it; else Ctrl-Break") in `stop_windows.go`. Linux and macOS are unchanged.
- **The re-checks reuse the spike's programs** (`dialnode`, `stdinwatch` in `tmp/wincheck/dev/wincheck/`, branch `spike/win-sidecar-transport`), built again against the 1.4.4 anfra-node. The stale-socket check is new for anfra-node (the spike checked it for canal), and runs by hand: start anfra-node, kill it, start it again on the same path.
- **Test on Windows without CI,** as in the earlier modules: Go tests built in WSL with `go test -c`, run on the Windows host.

## Dependency graph

```
T1 SIGBREAK: anfra-node handler + Bun 1.4.4 Windows build, checked   ← the risk; if it fails, stop and ask
 ├─ T2 anfra-node's tests, and its commit on the holistics-core branch
 ├─ T3 the Bun 1.4.4 re-checks (socket, stdin EOF ×3, stale socket)   → Checkpoint A
 └─ T4 anfra: Ctrl-Break to every sidecar, stdin closed after exit
      └─ T5 end to end on Windows 11 (logs clean, taskkill still warns, validate globs)
           └─ T6 docs, plan                                         → Checkpoint B
```

T2 and T3 need only T1's build; T4 needs T1's answer.

## Task List

### Phase 1: The risk
- [x] T1: anfra-node shuts down on `SIGBREAK`, checked on Windows with a Bun 1.4.4 build

### Phase 2: Each side
- [ ] T2: anfra-node's tests pass; the change committed on its branch
- [ ] T3: The Bun 1.4.4 re-checks on Windows

### Checkpoint A
- [ ] T1–T3 pass; review with the user, including whether to push the holistics-core branch.

- [ ] T4: anfra stops every sidecar with Ctrl-Break on Windows, and closes stdin after

### Phase 3: End to end and close
- [ ] T5: End to end on Windows 11
- [ ] T6: Docs and plan

### Checkpoint B: Complete
- [ ] Spec success criteria 1–6 met; the commit split reviewed with the user.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Bun on Windows doesn't deliver `SIGBREAK` to `process.on` | High: the chosen fix doesn't work | T1 first. If it fails, stop: the spec's open question (another way, a lower log level, or keep the warning) |
| `SIGBREAK` arrives, but Bun's default handler also ends the process before `shutdown` finishes | Medium: the compile cache isn't written | T1 checks the cache write and exit code 0, not only the log line |
| Bun 1.4.4 breaks something 1.2.10 did (sockets, the watchdog) | Medium | T3 re-runs every check; a failure goes back to the user before T4 |
| anfra's Ctrl-Break stop runs against an anfra-node without the handler (today's pinned 0.16.0) | Low today: no Windows anfra is released, so only dev builds on Windows are hit, and lose one cache write. It matters once `win-release` ships | The spec's boundary: anfra's change isn't merged before the pin moves to a release with the handler. Raised again at Checkpoint A: the real constraint may only be "before `win-release` ships" |
| holistics-core's `master` moved since your local copy (Nov 5) | Low | Branch from `origin/master` after a fetch; re-run anfra-node's tests there (T2) |

## Open Questions

- At Checkpoint A: when may anfra's change merge? Before the pin bump (Windows isn't released yet, so only dev builds are affected), or only after (the spec's boundary today)?
