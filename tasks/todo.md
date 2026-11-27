# Tasks: win-terminal

Plan: [plan.md](plan.md). Spec: [SPEC-win-terminal.md](../SPEC-win-terminal.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: The code

### - [x] T1: Colors in `cmd` and PowerShell 5 windows

**Acceptance criteria:**
- [x] First, by hand (you): what `anfra auth status` shows in a PowerShell 5 window today, outside Windows Terminal: colors, or raw codes (`←[1m`). If colors already show, T1 records that and changes nothing. Replaced by reading the color library: `colorprofile` only detects, it never turns escape codes on, and nothing in anfra does; Windows Terminal and VS Code have them on, the old console host doesn't. Turning them on where they're on already changes nothing, so T1 was built, and T5's hand checks confirm it.
- [x] Otherwise: `internal/cliui` turns on escape-code processing for stdout and stderr when each is a console, called from `main` (not `run`); a console where it fails gets no styles.
- [x] macOS and Linux unchanged.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/cliui/ ./cmd/anfra/`.
- [x] Windows: a test that the function leaves a non-console alone; the console itself checked by hand in T5. (`cliui` passes on Windows; a test of "no styles where it fails" was dropped: through a pipe it would pass without the check.)

**Dependencies:** None

**Files likely touched:** `internal/cliui/terminal.go`, a `_windows.go` / `_unix.go` pair, `cmd/anfra/main.go`

**Estimated scope:** S

### - [x] T2: Unicode trees and boxes on Windows

**Acceptance criteria:**
- [x] `utf8Terminal` is true on Windows; on Unix it reads `LC_ALL`, `LC_CTYPE`, `LANG` as today.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/pretty/`.
- [x] Windows: a test that it's true; trees seen by hand in T5. `internal/pretty` passes on Windows; `TestRowsFallbacks`' ASCII check runs only where an ASCII fallback exists (Unix).

**Dependencies:** None

**Files likely touched:** `internal/pretty/pretty.go` (+ a per-OS file)

**Estimated scope:** XS

### - [ ] T3: Data Apps live reload ignores changes on folders

**Acceptance criteria:**
- [ ] A change event on a path that is a folder is ignored; a new folder is still added to the watch.
- [ ] Linux and macOS unchanged.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/appserve/`.
- [ ] Windows: `TestWatch` and `TestLiveReloadWatchesOnlyWhileAStreamIsOpen` pass (failing today).

**Dependencies:** None

**Files likely touched:** `internal/appserve/watch.go`

**Estimated scope:** XS

### - [ ] T4: The tests on Windows

**Acceptance criteria:**
- [ ] `TestReadLinesEndWithTheirContext` skips on Windows, saying a pipe can't be cancelled there and that T5's Ctrl-C at the prompt checks it.
- [ ] `TestFiles` (`internal/appserve`) and `TestInitRefusesASymlinkedGitignore` (`cmd/anfra`) skip when a symlink can't be made, as `TestCheckPath` does.
- [ ] `cmd/anfra` and `internal/appserve` pass in full on Windows (the four `anfra logs` tests included); afterwards no copy of the test program is running, and it's deleted.

**Verification:**
- [ ] Tests pass (Linux): `go test ./cmd/anfra/ ./internal/appserve/`.
- [ ] Windows: the two full runs above, after checking free memory. Lint once, Linux and Windows.

**Dependencies:** T1, T2, T3

**Files likely touched:** `cmd/anfra/publish_test.go`, `cmd/anfra/init_test.go`, `internal/appserve/appserve_test.go`

**Estimated scope:** S

### Checkpoint A
- [ ] T1–T4's tests pass on Linux and Windows; review with you.

## Phase 2: By hand, and close

### - [ ] T5: Your hand checks in four terminals

**Acceptance criteria:**
- [ ] In Windows Terminal, a PowerShell 5 window, a `cmd` window and VS Code's terminal, with an `anfra.exe` I build: `anfra auth status` and `anfra show` (colors, Unicode trees, no raw codes); `anfra console` (typing, history with the arrow keys, Ctrl-C during a slow query, leaving with `\q` and with Ctrl-D); `anfra publish` on a folder without an org (the prompt shows, a choice works, Ctrl-C at it publishes nothing).
- [ ] What breaks is fixed here when small; a fix over half a day goes to you first.

**Verification:**
- [ ] Manual, by you, with steps I give; results recorded here.

**Dependencies:** T1, T2

**Files likely touched:** whatever the checks find

**Estimated scope:** M

### - [ ] T6: Docs and plan

**Acceptance criteria:**
- [ ] Docs where a Windows terminal differs, if anything visible does.
- [ ] `WINDOWS-PLAN.md`: Where it stands, and the module's result (the `anfra logs` tests were the update-check cascade, `d3d2150`).

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–6, each ticked here.

**Dependencies:** T5

**Files likely touched:** `docs/`, `WINDOWS-PLAN.md`

**Estimated scope:** XS

### Checkpoint B: Complete
- [ ] Spec success criteria 1–6 met.
- [ ] Ready for review with you (and the commit split proposed).
