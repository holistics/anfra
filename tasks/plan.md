# Implementation Plan: win-files-auth

Spec: [SPEC-win-files-auth.md](../SPEC-win-files-auth.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Task checklist: [todo.md](todo.md). Earlier modules' tasks: [archive/](archive/).

## Overview

What this module delivers and why: the spec's [Objective](../SPEC-win-files-auth.md#objective). This file holds how: the order, the checkpoints, task-level decisions and risks. First the shared piece (replacing a file by rename), then logins, which every publish and pull needs and you check by hand. Then the smaller fixes, each its own task, then everything on Windows end to end.

## Architecture Decisions

- **Per-OS code in build-tagged files,** as in `internal/sidecar` and `cmd/anfra`: a `_windows.go` beside a `!windows` file; no `runtime.GOOS` branches in shared functions.
- **One rename helper, in its own `internal/` package,** named per [naming](../docs/dev/naming.md) when it's written. On Windows it retries for a short, bounded time (about a second) on `ERROR_ACCESS_DENIED` and `ERROR_SHARING_VIOLATION`, as `cmd/go/internal/robustio` does; elsewhere it is `os.Rename`. The credential store uses it too.
- **The login file's access list is set as the file is made,** the way the socket folder is (`internal/sidecar/socket_windows.go`), before any token is written. Read side: refuse the file when an allow entry lets a reader other than this user, SYSTEM or Administrators read it. SYSTEM and Administrators stand where root does on Linux, which mode `600` doesn't keep out either. The error says which, and gives an `icacls` command that fixes it.
- **The time zone through `icu.dll`,** loaded lazily as `internal/sidecar` loads `kernel32.dll`; any failure falls back to `Etc/UTC`, so a Data App never fails over it.
- **Pull's Windows name checks live in `checkPath`'s Windows file;** Linux and macOS keep accepting what they accept.
- **Test on Windows without CI,** as before: `go test -c` in WSL, run on the Windows host.

## Dependency graph

```
T1 rename helper, used at the 5 save sites
 └─ T2 saved logins (access list, close before rename, folder sync)   → Checkpoint A: you log in on Windows
T3 exports (close before delete, file:///C:/ links)
T4 path checks: appFile, checkPath (security)
T5 RemoveLegacy skipped on Windows
T6 time zone from ICU
T7 DuckDB's lock message on Windows
                                                                       → Checkpoint B
T8 end to end on Windows 11 (needs T1–T7)
 └─ T9 docs, backlog, plan, make check / make test                    → Checkpoint C
```

T3–T7 are independent of each other and of T1–T2.

## Task List

### Phase 1: Saving files, and logins
- [ ] T1: One rename helper that survives another process holding the file, used at the 5 save sites
- [ ] T2: Saved logins work on Windows, private to the user

### Checkpoint A
- [ ] T1–T2's Windows tests pass; you run `anfra auth login` on Windows, then a command that needs it.

### Phase 2: The smaller fixes
- [ ] T3: Exports: the download removes the CLI's copy; `file:///C:/…` links
- [ ] T4: Path checks: Data App files refuse `\` and `:`; pull refuses names Windows can't hold
- [ ] T5: `RemoveLegacy` does nothing on Windows
- [ ] T6: Data Apps get Windows' time zone, from ICU
- [ ] T7: DuckDB's lock message on Windows

### Checkpoint B
- [ ] T3–T7's tests pass on Linux and Windows; review with you.

### Phase 3: End to end and close
- [ ] T8: End to end on Windows 11
- [ ] T9: Docs, backlog, plan, and the Linux checks

### Checkpoint C: Complete
- [ ] Spec success criteria 1–8 met; the commit split reviewed with you.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| The access-list check is too strict: a normal Windows setup (inherited SYSTEM and Administrators) is refused | High: login breaks for everyone | Files anfra writes have its own list; the read check allows SYSTEM and Administrators; T2's Windows test reads a file made the usual way, and you log in at Checkpoint A |
| A retry hides a real, lasting lock (antivirus scanning, another program holding the file) | Medium: a save waits, then fails | Bounded retry; the error after it names the file and says another program has it open |
| `icu.dll` answers a different name than Windows shows, or none | Low: the wrong day near midnight in Data Apps | Compared with `Get-TimeZone` on the Windows host; any failure falls back to `Etc/UTC` |
| DuckDB's Windows lock message differs by version | Low: the hint doesn't show | Checked with the DuckDB anfra ships (the `win-canal` DLL, v1.5.6); the match stays loose (the lock words and the PID) |
| More Unix assumptions turn up while testing | Medium | Each one is reported; one inside a task's area is fixed there, others go to you before being added |

## Open Questions

None.
