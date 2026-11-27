# Implementation Plan: win-terminal

Spec: [SPEC-win-terminal.md](../SPEC-win-terminal.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Task checklist: [todo.md](todo.md). Earlier modules' tasks: [archive/](archive/).

## Overview

What this module delivers and why: the spec's [Objective](../SPEC-win-terminal.md#objective). This file holds how. First the code changes, each small and testable on its own (colors, Unicode, live reload, the tests that fail on Windows), then one full Windows run of `cmd/anfra` and `internal/appserve`, then your hand checks in four terminals, then docs.

## Architecture Decisions

- **Escape codes are turned on in `main`, not `run`:** `run` is what the tests call, and they mustn't change the console they run in. A per-OS function in `internal/cliui` turns on `ENABLE_VIRTUAL_TERMINAL_PROCESSING` for stdout and stderr when each is a console; where that fails, `NewTerminalStyles` gives no styles for it.
- **`utf8Terminal` per OS:** Windows answers true (Go writes to a console with `WriteConsoleW`, whatever its code page); Unix keeps reading `LC_ALL`/`LC_CTYPE`/`LANG`.
- **The watcher ignores a change on a folder** (not its creation, which still adds the folder's watches). Shared code; no OS branch: Linux and macOS don't send such events.
- **Tests that can't run on Windows skip, saying why,** with the check that replaces them named.
- **Working safely on Windows** ([memory](archive/win-files-auth-todo.md#checkpoint-b), and the update-check cascade fixed in `d3d2150`): `GOMAXPROCS=4 GOFLAGS=-p=2`; one Windows build per task; Windows' free memory checked first; after a run of `cmd/anfra`'s test program, no copies left (checked) and the program deleted.

## Dependency graph

```
T1 colors in the old console host      (independent)
T2 Unicode trees on Windows            (independent)
T3 live reload ignores folder changes  (independent)
T4 tests on Windows: skips, then cmd/anfra and appserve in full on Windows (after T1–T3)
                                                     → Checkpoint A
T5 your hand checks in four terminals (needs T1, T2)
 └─ T6 docs and plan                                 → Checkpoint B
```

## Task List

### Phase 1: The code
- [x] T1: Colors in `cmd` and PowerShell 5 windows (escape codes turned on, or no styles)
- [x] T2: Unicode trees and boxes on Windows
- [x] T3: Data Apps live reload ignores changes on folders
- [x] T4: The tests on Windows: skips where Windows can't, then `cmd/anfra` and `internal/appserve` in full

### Checkpoint A
- [x] T1–T4's tests pass on Linux and Windows; review with you before T5.

### Phase 2: By hand, and close
- [x] T5: Your hand checks (Windows Terminal only, decided by you)
- [x] T5a: Typed answers on Windows (found in T5: `cancelreader`'s raw mode; the console's mode put back)
- [x] T6: Docs and plan

### Checkpoint B: Complete
- [ ] Spec success criteria 1–6 met; the commit split reviewed with you.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Turning on escape codes changes the console for the process that started anfra | Medium: their colors after anfra exits | The mode is per process handle; restored on exit if Windows doesn't already; checked by hand in T5 |
| `colorprofile.Detect` already handles Windows, so T1's change is redundant | Low: wasted work | T1 starts by checking what a PowerShell 5 window shows today (in T5's first hand check, early), before writing code |
| A hand check finds the console (bubbletea) broken in one terminal | High: more work than planned | The spec's boundary: a fix over half a day is reported to you first |
| Running `cmd/anfra`'s test program on Windows again | High, today's crash | Fixed in `d3d2150`; still checked after each run, and the program deleted |

## Open Questions

None.
