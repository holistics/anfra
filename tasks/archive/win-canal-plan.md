# Implementation Plan: win-canal

Spec: [SPEC-win-canal.md](../../SPEC-win-canal.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Task checklist: [win-canal-todo.md](win-canal-todo.md). The finished `win-sidecar-transport` tasks are in [win-sidecar-transport-plan.md](win-sidecar-transport-plan.md).

## Overview

What this module delivers and why: the spec's [Objective](../../SPEC-win-canal.md#objective). This file holds how it's built: the task order, the go/no-go gate, and task-level decisions and risks. The first tasks are the build spike, which decides the whole plan's go/no-go; the rest prove the binary on Windows and close the module.

## Architecture Decisions

- **Build canal natively on Windows** (changed in T2, with the user): Ubuntu's MinGW (GCC 13, MSVCRT) can't link DuckDB's Windows library, which needs MinGW-Builds GCC 14.2.0 with UCRT. The toolchain and a copy of canal's source live in `C:\Users\nuita\wincheck`; the build is driven from WSL through PowerShell. Check programs without cgo are still cross-compiled in WSL. No push to holistics/canal is needed.
- **Two canal branches, both local:** the check programs and build script go on `spike/win-sidecar-transport` (never merged, worktree at `anfra/tmp/canal-wincheck`). Any real code fix goes on a new `feat/win-canal` branch from `origin/master`. Nothing is pushed without asking.
- **canal's own tests are the main proof,** as in check 3: `go test -c` of `internal/query/connector` and `internal/lake` with cgo and DuckDB, run on Windows. Only the end-to-end check needs a new program.
- **`anfra-canal.exe` must need only Windows' own DLLs.** anfra embeds it as one file and unpacks only that file, so a MinGW build that depends on `libstdc++-6.dll`, `libgcc_s_seh-1.dll` or `libwinpthread-1.dll` won't start on a user's machine. The build links them statically if needed, and T2 checks the import table.
- **The go/no-go is measured in effort, not in tasks:** time spent on T1–T5 is logged in the findings doc. Past about **5 ED**, stop and re-plan (spec, Objective).

## Dependency graph

```
T1 toolchain (MinGW in WSL) + findings doc
 └─ T2 build anfra-canal.exe                 ← the deciding step
     ├─ T3 --version + canal's DuckDB connector tests on Windows
     ├─ T4 canal-lake's tests on Windows (json, icu extensions)
     └─ T5 end to end: query over a Unix socket, SELECT 1, Ctrl-Break
          └─ Checkpoint B: go/no-go
               ├─ T9 DLL build → T10 DLL checks + MotherDuck → Checkpoint B2: static or DLL
               ├─ T6 code fixes found in T2–T5, if any (feat/win-canal)
               ├─ T7 Oracle note in canal's docs
               └─ T8 close: findings final, plan updated
```

T3, T4 and T5 are independent once T2 builds. T6 and T7 are independent.

## Task List

### Phase 1: Build (the spike)
- [x] T1: MinGW toolchain in WSL, and the findings doc
- [x] T2: Cross-compile `anfra-canal.exe` (built natively on Windows instead)

### Checkpoint A: It builds
- [x] `anfra-canal.exe` built and needs only Windows' own DLLs → continue. It can't be built → record why, stop, and review the go/no-go with the user.

### Phase 2: Prove it on Windows
- [x] T3: `--version` and canal's DuckDB connector tests on Windows
- [x] T4: canal-lake's tests on Windows
- [x] T5: End to end: canal-query over a Unix socket, `SELECT 1`, Ctrl-Break

### Checkpoint B: Go / no-go
- [x] T2–T5 results recorded, with the effort spent (~1 ED). **Go** (the user, 2026-11-24); recorded in `WINDOWS-PLAN.md`.

### Phase 2b: MotherDuck on Windows (the DLL spike)
- [x] T9: Build `anfra-canal.exe` against DuckDB's official `duckdb.dll`
- [x] T10: The DLL build passes the checks, and loads MotherDuck

### Checkpoint B2: Static or DLL
- [x] T10 passed → **the Windows build uses the DLL** (the user, 2026-11-24).

### Phase 3: Close
- [x] T6: Code fixes found in T2–T5 (only if any) — skipped: none needed
- [x] T7: Oracle note in canal's docs
- [x] T8: Findings final, plan updated

### Checkpoint C: Complete
- [x] Spec success criteria 1–5 met; ready for review.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| holistics' `duckdb-go` fork, or arrow-go's cgo code, doesn't build with MinGW | High: no Windows canal | The fork only adds streaming on top of upstream, which builds for `windows-amd64`. Read the first error closely; try upstream `duckdb-go` without the fork to tell a fork problem from a toolchain one. |
| The WSL MinGW is a different version from the one DuckDB's Windows library was built with, and linking fails | Medium | Try Ubuntu's `gcc-mingw-w64` first; if linking fails on missing symbols, use the MinGW version `duckdb-go-bindings` names in its docs (an llvm-mingw or a newer mingw-w64 tarball, unpacked under the home folder, no `sudo`). |
| The `.exe` needs MinGW's runtime DLLs | Medium: it won't start on a user's machine | Link them statically (`-static-libgcc -static-libstdc++`, `-Wl,-Bstatic -lwinpthread`); check with `x86_64-w64-mingw32-objdump -p anfra-canal.exe \| grep 'DLL Name'` |
| godror (Oracle) doesn't compile on Windows with MinGW | Medium | Ask first, then compile it out of the Windows build with a build tag, so Oracle reports "not supported on Windows yet" (the escape hatch from Long-term maintenance) |
| DuckDB can't download the `json`/`icu` extensions on the Windows machine | Low | Note the network need; extensions are downloaded once into `%USERPROFILE%\.duckdb` (they downloaded fine in T3/T4) |
| **Found in T4:** MotherDuck's extension has no `windows_amd64_mingw` build, so MotherDuck data sources fail on a MinGW-built canal | Medium: a feature gap on Windows | Decide at Checkpoint B: document it as not supported on Windows yet; or link DuckDB's MSVC `duckdb.dll` (whose `windows_amd64` extensions include MotherDuck); or ask DuckDB/MotherDuck for a MinGW build |
| Ctrl-Break can't be sent to canal from the check program | Low | The program starts canal with `CREATE_NEW_PROCESS_GROUP` and sends `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT)`, as anfra will in `win-process-lifecycle` |

## Open Questions

None.
