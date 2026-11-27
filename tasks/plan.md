# Implementation Plan: win-self-update

Spec: [SPEC-win-self-update.md](../SPEC-win-self-update.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md). Task checklist: [todo.md](todo.md). Earlier modules' tasks: [archive/](archive/).

## Overview

What this module delivers and why: the spec's [Objective](../SPEC-win-self-update.md#objective). This file holds how. The checksum first, on every OS, with the release step that publishes `SHA256SUMS` in the same change (decided with the user: the order stays safe in one PR). Then the Windows `.zip`, the old binary and the advice. Then an end-to-end check on Windows against a local fake release.

## Architecture Decisions

- **`Apply` updates a path a test can choose:** an unexported `applyTo(ctx, r, progress, target)`, with `Apply` passing the running executable. `minio/selfupdate` already takes a target path (`Options.TargetPath`). Tests then update a temporary file, never the test program.
- **Download to a temporary file, then verify, then unpack:** the asset is hashed while it's written (one pass), compared with its line in `SHA256SUMS`, and only then read back to unpack. Never held in memory (a Windows asset is about 370 MB).
- **`SHA256SUMS` sits beside the assets in the release,** downloaded from the same `releases/download/<tag>/` path, in `sha256sum`'s format.
- **The unpacking differs per OS** (`.gz` on Linux and macOS, `.zip` on Windows), in build-tagged files; the checksum doesn't.
- **The end-to-end check uses a test build** with `webBase`/`rawBase` pointed at a local server by `-ldflags -X`, serving a newer `anfra.exe` zipped and its `SHA256SUMS`. No code for it.
- **Working safely on Windows,** as before: `GOMAXPROCS=4 GOFLAGS=-p=2`; one build per task; Windows' free memory checked first; no test-program copies left after a `cmd/anfra` run.

## Dependency graph

```
T1 checksum on every OS (+ applyTo for tests)
 ├─ T2 the release publishes SHA256SUMS (same change, for the order)
 └─ T3 the Windows .zip: asset name, found by full name, unzip
      └─ T4 Windows: the leftover .anfra.exe.old, and the advice to install.ps1
                                                     → Checkpoint A
T5 end to end on Windows: update, and update inside the console (needs T1–T4)
 └─ T6 docs and plan                                 → Checkpoint B
```

## Task List

### Phase 1: The update
- [ ] T1: `anfra update` verifies the release's checksum, on every OS
- [ ] T2: Each release publishes `SHA256SUMS`
- [ ] T3: The Windows asset: `anfra-windows-x64.zip`, unzipped
- [ ] T4: Windows: the old binary, and the advice on failure

### Checkpoint A
- [ ] T1–T4's tests pass on Linux and Windows; review with you.

### Phase 2: End to end, and close
- [ ] T5: End to end on Windows with a local fake release
- [ ] T6: Docs and plan

### Checkpoint B: Complete
- [ ] Spec success criteria 1–6 met; the commit split reviewed with you.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| A verifying anfra ships before releases publish `SHA256SUMS` | High: every update refused | T2 is in the same change as T1 (the user's choice); the PR says not to split them |
| The `.zip`'s layout differs from what update expects (a folder inside, another name) | High: Windows updates fail | T3 looks for `anfra.exe` at the archive's top only, and `win-release` builds the `.zip` to match; the format is written in `docs/designs/release.md` |
| A temporary file left behind when the download fails or is cancelled | Low: disk | Removed in a `defer`, whatever happens; a test |
| Replacing `anfra.exe` while a console or `serve` from it runs | Medium | `selfupdate` renames the running file aside (Windows allows that); T4 handles the leftover; T5 checks the console's restart |
| The fake release differs from GitHub's | Medium: the end-to-end check proves less | It serves the same paths `Latest` uses today (`/releases/latest` redirect, `/releases/download/<tag>/<asset>`), as `internal/update`'s tests do |

## Open Questions

None.
