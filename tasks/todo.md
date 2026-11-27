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

### - [ ] T4: Windows: the old binary, and the advice on failure

**Acceptance criteria:**
- [ ] Before an update on Windows, a leftover `.anfra.exe.old` beside `anfra.exe` is removed; when Windows refuses (in use), anfra says so and goes on with the update.
- [ ] A failure's advice on Windows is `irm https://anfra.ai/install.ps1 | iex`; elsewhere `install.sh`, as today.

**Verification:**
- [ ] Windows (`_windows_test.go`): a leftover `.old` is removed; one held open is reported and the update still applies.
- [ ] Tests pass (Linux); lint once, Linux and Windows.

**Dependencies:** T3

**Files likely touched:** `internal/update/update.go` (+ per-OS files and tests)

**Estimated scope:** S

### Checkpoint A
- [ ] T1–T4's tests pass on Linux and Windows; review with you.

## Phase 2: End to end, and close

### - [ ] T5: End to end on Windows with a local fake release

**Acceptance criteria:**
- [ ] Two Windows builds: an "old" `anfra.exe` whose update looks at a local server (`-ldflags -X …/internal/update.webBase=…`, and `rawBase`), and a "new" one, zipped as `anfra-windows-x64.zip` with its `SHA256SUMS`, served from WSL at the paths `Latest` uses.
- [ ] `anfra update` from PowerShell replaces the old with the new (`anfra version` shows it); with a wrong `SHA256SUMS`, it refuses and the old stays.
- [ ] Inside `anfra console`, `update` restarts the console on the new anfra (by you).

**Verification:**
- [ ] Manual, on Windows 11; results recorded here.

**Dependencies:** T1–T4

**Files likely touched:** none (a small server script in `tmp/`)

**Estimated scope:** S

### - [ ] T6: Docs and plan

**Acceptance criteria:**
- [ ] `docs/designs/release.md` (the assets, `SHA256SUMS`, the `.zip`'s layout) and the `anfra update` usage doc: what update checks.
- [ ] `WINDOWS-PLAN.md`: the download format moved to "Decisions made" (`.zip`, and update verifying on every OS); Where it stands; the module's result.

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–6, each ticked here.

**Dependencies:** T5

**Files likely touched:** `docs/designs/release.md`, `docs/usage/`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint B: Complete
- [ ] Spec success criteria 1–6 met.
- [ ] Ready for review with you (and the commit split proposed).
