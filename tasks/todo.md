# Tasks: win-self-update

Plan: [plan.md](plan.md). Spec: [SPEC-win-self-update.md](../SPEC-win-self-update.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: The update

### - [x] T1: `anfra update` verifies the release's checksum, on every OS

**Acceptance criteria:**
- [x] The asset downloads to a temporary file, hashed while written; its sha256 is compared with its line in the release's `SHA256SUMS` before anything is unpacked or replaced.
- [x] A mismatch, or a release without `SHA256SUMS` (or without the asset's line), refuses: the message says why, and how to install by hand. Nothing is replaced.
- [x] The temporary file is removed after, whatever happens.
- [x] `Apply` keeps its signature; an unexported `applyTo` takes the target path, for tests.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/update/`, with a fake release: a matching sum applies (to a temporary target); a wrong sum, a missing `SHA256SUMS`, a missing line each refuse and leave the target as it was; no temporary file left.
- [x] Lint once, Linux and Windows.

**Dependencies:** None

**Files likely touched:** `internal/update/update.go` (+ tests)

**Estimated scope:** M

### - [x] T2: Each release publishes `SHA256SUMS`

**Acceptance criteria:**
- [x] `build_release.yml` writes `release/SHA256SUMS` (`sha256sum` over the assets, by file name) after the assets are built, and uploads it with them; a branch's test build keeps it in its artifacts too.
- [x] Committed with T1, so a verifying anfra never ships without it; the PR description says not to split them.

**Verification:**
- [x] The step's shell run locally over sample files gives lines `sha256sum -c` accepts.
- [x] The workflow parses (`actionlint`, if available, else a YAML check). `actionlint` isn't installed here: the YAML parses. Also: `SHA256SUMS` uploads before the assets, in a call of its own, since `anfra update` takes a release as out once its asset is.

**Dependencies:** T1

**Files likely touched:** `.github/workflows/build_release.yml`

**Estimated scope:** XS

### - [x] T3: The Windows asset: `anfra-windows-x64.zip`, unzipped

**Acceptance criteria:**
- [x] `assetName`: `windows/amd64` → `anfra-windows-x64.zip`; Linux and macOS names gain `.gz` there, and `Latest` uses the full name (no `+ ".gz"`).
- [x] On Windows, the verified `.zip`'s `anfra.exe` (at the archive's top) is what's applied; a `.zip` without it refuses, saying so.
- [x] Linux and macOS unchanged (`.gz`).

**Verification:**
- [x] Tests pass (Linux): `TestAssetName` and the `Latest` tests with full names.
- [x] Windows (`_windows_test.go`): a fake release with a `.zip` applies to a temporary target; a `.zip` without `anfra.exe` refuses. `internal/update` passes in full on Windows; `TestLatestFailsInItsStep` looked for Linux's "connection refused" and now accepts Windows' "actively refused it".

**Dependencies:** T1

**Files likely touched:** `internal/update/update.go`, a per-OS unpack pair (+ tests)

**Estimated scope:** S

### - [x] T4: Windows: the old binary, and the advice on failure

**Acceptance criteria:**
- [x] Before an update on Windows, a leftover `.anfra.exe.old` beside `anfra.exe` is removed; when Windows refuses (in use), anfra says so and goes on with the update. Changed: selfupdate moves `anfra.exe` aside to `.anfra.exe.old`, which fails while a leftover there is in use, so this update uses a name of its own (`.anfra.exe.old-<n>`) then, and every leftover goes at the next update; nothing to say, the update just works.
- [x] A failure's advice on Windows is `irm https://anfra.ai/install.ps1 | iex`; elsewhere `install.sh`, as today.

**Verification:**
- [x] Windows (`_windows_test.go`): a leftover `.old` is removed; one held open is reported and the update still applies.
- [x] Tests pass (Linux); lint once, Linux and Windows.

**Dependencies:** T3

**Files likely touched:** `internal/update/update.go` (+ per-OS files and tests)

**Estimated scope:** S

### Checkpoint A
- [x] T1–T4's tests pass on Linux and Windows (`internal/update` in full on Windows). Review with you at the end of the run.

## Phase 2: End to end, and close

### - [x] T5: End to end on Windows with a local fake release

**Acceptance criteria:**
- [x] Two Windows builds: an "old" `anfra.exe` whose update looks at a local server (`-ldflags -X …/internal/update.webBase=…`, and `rawBase`), and a "new" one, zipped as `anfra-windows-x64.zip` with its `SHA256SUMS`, served from WSL at the paths `Latest` uses.
- [x] `anfra update` from PowerShell replaces the old with the new (`anfra version` shows it); with a wrong `SHA256SUMS`, it refuses and the old stays.
- [x] Inside `anfra console`, `update` restarts the console on the new anfra (by you). It didn't at first: on Windows, `os.SameFile` reads a file's ID on first use, by its path, so the console compared the new anfra with itself. Fixed by reading the ID when the console starts (`anfraFileAt`, `TestReplacedAskedOnlyAfter`, passing on Linux and Windows), then checked again by you: "Restarted on anfra 9.9.9."

**Verification:**
- [x] Manual, on Windows 11 (2026-11-27), served from WSL: from PowerShell, "Updated anfra 0.1.0 → 9.9.9." and `anfra version` 9.9.9, the old file moved to `.anfra.exe.old`; with a wrong `SHA256SUMS`, refused (`precondition_failed`, advising `irm https://anfra.ai/install.ps1 | iex`), the old anfra 0.1.0 kept, no download left in `%TEMP%`.

**Dependencies:** T1–T4

**Files likely touched:** none (a small server script in `tmp/`)

**Estimated scope:** S

### - [x] T6: Docs and plan

**Acceptance criteria:**
- [x] `docs/designs/release.md` (the assets, `SHA256SUMS`, the `.zip`'s layout) and the `anfra update` usage doc: what update checks. There is no usage page for update: its `--help` says it instead.
- [x] `WINDOWS-PLAN.md`: the download format moved to "Decisions made" (`.zip`, and update verifying on every OS); Where it stands; the module's result.

**Verification:**
- [x] Manual: the spec's Success Criteria 1–6, each ticked here.
  - [x] 1. Windows finds, unzips and applies the `.zip` (T3, T5); Linux and macOS keep `.gz` (`TestAssetName`).
  - [x] 2. Every OS checks `SHA256SUMS`; a mismatch or none refuses and changes nothing (T1, T5).
  - [x] 3. A leftover `.anfra.exe.old` is removed; one in use doesn't stop the update (T4: a name of its own, rather than a message).
  - [x] 4. Windows' advice is `install.ps1` (T4, T5).
  - [x] 5. The console restarts on the new anfra on Windows (T5, after the fix).
  - [x] 6. `build_release.yml` publishes `SHA256SUMS` (T2); docs and `WINDOWS-PLAN.md` updated (T6).

**Dependencies:** T5

**Files likely touched:** `docs/designs/release.md`, `docs/usage/`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint B: Complete
- [x] Spec success criteria 1–6 met.
- [ ] Ready for review with you (and the commit split proposed).
