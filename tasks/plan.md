# Implementation Plan: win-release

Spec: [SPEC-win-release.md](../SPEC-win-release.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Task checklist: [todo.md](todo.md). Earlier modules' tasks: [archive/](archive/).

## Overview

What this module delivers and why: the spec's [Objective](../SPEC-win-release.md#objective). This file holds how. The sidecars' Windows builds first, since anfra's release can't build Windows without them, and canal's is the riskiest (a C toolchain on a Windows runner). Then anfra's release, `install.ps1`, the PR checks, and the docs. Everything is checked by hand on Windows first; what only GitHub can run waits for your approval to push.

## Architecture Decisions

- **Each Windows row copies its neighbours.** canal and holistics-core already build each OS natively, one runner per target; the Windows row is one more matrix entry, with steps that differ only where Windows must (MinGW and the DLL for canal; `.exe` names). Existing rows aren't changed.
- **anfra-node builds natively on Windows, like its other rows, with no fallback.** If the Windows row fails, it's fixed in the row when the cause is the runner (CRLF checkout, say); when it's holistics-core's own code or packages, it goes to you with the error, for holistics-core's person in charge, rather than being routed around by cross-building.
- **Windows files are named `<name>-windows-x64.exe`;** canal's DLL is `<app>-windows-x64-duckdb.dll`, so `anfra-canal-*` downloads it with the rest, and it can't be mistaken for another version's.
- **The notice lives at `packaging/windows/THIRD_PARTY_NOTICES.txt`:** `release/` is the workflow's output folder. DuckDB's MIT license text, from DuckDB's repository at the version the DLL is (v1.5.6).
- **anfra's release stays on one Ubuntu runner:** anfra is pure Go, so `anfra.exe` cross-builds like the others; `zip` packs it.
- **A missing Windows sidecar fails the release by name:** after the download, a check that each expected file is there, listing what's missing.
- **`install.ps1` mirrors `install.sh` section by section,** with `ANFRA_DOWNLOAD_URL` as the one addition, for testing against a local release.
- **The Windows CI job runs a fixed package list** (the spec's), on `windows-2022`, required through `ci_passed`. Tests that fail there only because of their setup are fixed where cheap, else skipped on Windows with the reason.
- **Working safely on Windows,** as before: `GOMAXPROCS=4 GOFLAGS=-p=2`; one test build per task; Windows' free memory checked first; `ANFRA_NO_UPDATE_NOTIFIER=1` and no test-program copies left after a `cmd/anfra` run.

## Dependency graph

```
T1 canal: windows-x64 row (DLL build, the DLL published, --version)
T2 holistics-core: windows-x64 row (native Bun build, --version)
 │                                                    → Checkpoint A (push for test builds?)
 └─ T3 anfra release: Windows sidecars, anfra.exe, the .zip with the notice
     └─ T4 install.ps1 (from a local release built by T3's commands)
         └─ T5 install.sh on Git Bash points to install.ps1
                                                      → Checkpoint B
T6 GOOS=windows go vet in anfra's checks              (independent)
T7 the package list's tests pass on Windows           (independent)
 └─ T8 the required windows CI job
                                                      → Checkpoint C
T9 docs and plan                                      → Checkpoint D: complete
```

T6–T8 don't depend on T1–T5 and could go first; they stay after, so the riskiest work (canal's toolchain) fails fast.

## Task List

### Phase 1: The sidecars build for Windows
- [x] T1: canal's `windows-x64` row
- [x] T2: holistics-core's `windows-x64` row

### Checkpoint A
- [x] Both Windows binaries built by hand with the rows' commands, and `--version` answers; you decide whether to push both branches and run each workflow as a test build.

### Phase 2: anfra's release and its installers
- [x] T3: anfra's release builds `anfra-windows-x64.zip`
- [x] T4: `install.ps1`
- [x] T5: `install.sh` on Git Bash points to `install.ps1`

### Checkpoint B
- [x] On Windows 11, from a local release: `install.ps1` installs the `.zip` T3's commands built, `anfra` answers queries in a new terminal (MotherDuck's extension included), and a wrong checksum refuses.

### Phase 3: anfra's PR checks
- [x] T6: `GOOS=windows go vet ./...` in the Linux checks
- [x] T7: the package list's tests pass on Windows
- [x] T8: the required `windows` CI job

### Checkpoint C
- [x] The package list passes on Windows here; the workflows parse.

### Phase 4: Close
- [x] T9: Docs and plan

### Checkpoint D: Complete
- [x] Spec success criteria 1–6 met; what only GitHub can check listed for you, with the order of merging; the commit split reviewed with you.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| MinGW-Builds GCC 14.2.0 isn't on GitHub's Windows runner, or its setup differs from this machine's | High: no Windows canal | Download the exact release by URL with its checksum, as this machine's toolchain was; checked by hand with the same steps first |
| Installing or building holistics-core's workspace on a Windows runner fails: CRLF checkout breaking `build-sidecar.sh`, package scripts written for bash, native packages, long paths | Medium: the Windows anfra-node row fails | Checkout without line-ending conversion, as the row's first defence; a runner-side cause fixed in the row; anything in holistics-core's code goes to you, with the error, for its person in charge |
| The runs on GitHub can't be checked without pushing | Medium: a workflow error found late | The rows' commands run by hand first; YAML checked; test builds (no release) only with your approval |
| `install.ps1` behaves differently under `iex` than from a file (an `exit`, encoding, PowerShell 5.1 vs 7) | High: users' windows close, or it fails | The layered tests; no `exit`; ASCII; PowerShell 7 on GitHub's runner if you don't install it here |
| The package list has more Windows-only test failures than expected | Medium: T7 grows | T7 first lists them; past a handful, split by package and tell you |
| `install.ps1` reaches `main` before the first Windows release | High: installs fail | The order of merging in the PR, `release.md` and `WINDOWS-PLAN.md` |
| An old `manifest.yml` pin, without Windows sidecars, at release time | High: anfra's release fails | Fails by name (T3); the pins move in the order of merging, once the sidecar releases exist |

## Open Questions

None.
