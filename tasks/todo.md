# Tasks: win-files-auth

Plan: [plan.md](plan.md). Spec: [SPEC-win-files-auth.md](../SPEC-win-files-auth.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: Saving files, and logins

### - [ ] T1: One rename helper that survives another process holding the file, used at the 5 save sites

**Description:** anfra saves by writing a temporary file and renaming it over the old one. On Windows the rename fails while another process has the old file open. One helper retries briefly there.

**Acceptance criteria:**
- [ ] A new `internal/` package: on Windows, rename, retrying for about a second on access-denied and sharing-violation errors; elsewhere, `os.Rename`. Named per `docs/dev/naming.md`.
- [ ] Used at `cmd/anfra/console_history.go:99`, `cmd/anfra/runtime.go:62`, `internal/cliauth/credential_store.go:117`, `internal/repoconfig/repoconfig.go:303`, `internal/publish/syncstate/syncstate.go:225`.
- [ ] After the retries, the error names the file and says another program has it open.

**Verification:**
- [ ] Tests pass (Linux): the new package, and the 4 packages of the save sites.
- [ ] On Windows (`_windows_test.go`): a rename over a file held open by another handle succeeds once that handle closes within the retry time; fails with the clear error when it never closes. Fails without the helper (a plain `os.Rename`).
- [ ] Lint for Windows and Linux.

**Dependencies:** None

**Files likely touched:** the new package (+ tests), the 5 save sites

**Estimated scope:** M

### - [ ] T2: Saved logins work on Windows, private to the user

**Description:** Without this, `anfra auth login` and everything that needs it fail on Windows.

**Acceptance criteria:**
- [ ] On Windows, the temporary credentials file is made with an owner-only access list (this user, nothing inherited) before any token is written; the renamed file keeps it.
- [ ] On read, Windows refuses a file whose access list lets a reader other than this user, SYSTEM or Administrators read it; the error says so and gives an `icacls` command that fixes it. Unix keeps the `600` check and its message.
- [ ] The inventory file is closed once read, before the new one is renamed over it.
- [ ] `syncCredentialDirectory`: checked on Windows; skipped there if a folder can't be synced.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/cliauth/`.
- [ ] On Windows (`_windows_test.go`): save, read, save again through the store, with its lock; the saved file's access list is owner-only; a file with an Everyone entry is refused with the fix; a file with the usual inherited list (user, SYSTEM, Administrators) is read.

**Dependencies:** T1

**Files likely touched:** `internal/cliauth/credential_store.go`, `refresh_recovery.go`, new `_windows.go` / `_unix.go` files (+ tests)

**Estimated scope:** M

### Checkpoint A
- [ ] T1–T2's Windows tests pass; Linux tests pass.
- [ ] You run `anfra auth login` on Windows, then a command that needs the login (I'll give the exact steps).

## Phase 2: The smaller fixes

### - [ ] T3: Exports: the download removes the CLI's copy; `file:///C:/…` links

**Acceptance criteria:**
- [ ] `download` closes the CLI's export file before removing it.
- [ ] On Windows, the export's `file:` link is `file:///C:/…`, and `openLink` reads it back to the path; Unix links unchanged.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/exports/ ./cmd/anfra/`; a link round-trip test.
- [ ] On Windows: the round-trip test; a query export downloaded, and the CLI's copy gone after.

**Dependencies:** None

**Files likely touched:** `cmd/anfra/commands.go`, `internal/exports/exports.go` (+ per-OS link files and tests)

**Estimated scope:** S

### - [ ] T4: Path checks: Data App files refuse `\` and `:`; pull refuses names Windows can't hold

**Acceptance criteria:**
- [ ] `appFile` returns "" for a path holding `\` or `:`, on every OS (neither belongs in a URL path to a Data App file).
- [ ] On Windows, `checkPath` refuses device names (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`, with or without an extension, any case), `:` and `<>"|?*`, control characters, and a trailing dot or space in any part, each with an `UnsafePathError` saying why.
- [ ] Linux and macOS: `checkPath` accepts what it accepted.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/appserve/ ./internal/publish/...`, with `appFile` cases for `\` and `:`.
- [ ] On Windows: `checkPath` cases for each kind; before the fix, `apps/sub\.x` is served (shown, then fixed).

**Dependencies:** None

**Files likely touched:** `internal/appserve/files.go`, `internal/publish/workdir/apply.go` (+ a Windows names file and tests)

**Estimated scope:** M

### - [ ] T5: `RemoveLegacy` does nothing on Windows

**Acceptance criteria:**
- [ ] On Windows, `RemoveLegacy` removes nothing; Linux and macOS unchanged. The comment says why (anfra never kept files there on Windows).

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/home/`.
- [ ] On Windows: a test with `%LOCALAPPDATA%` pointed at a temporary folder holding `anfra\`: it's still there after.

**Dependencies:** None

**Files likely touched:** `internal/home/home.go` (+ a per-OS file and a test)

**Estimated scope:** XS

### - [ ] T6: Data Apps get Windows' time zone, from ICU

**Acceptance criteria:**
- [ ] On Windows, `localTimezone`: `$TZ`, else the IANA name `icu.dll` gives for the system's zone, else `Etc/UTC`. Unix unchanged.
- [ ] A missing `icu.dll` or a failed call falls back to `Etc/UTC`, never an error.

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/appserve/`.
- [ ] On Windows: the name matches Windows' setting (`Get-TimeZone`, mapped by hand once); `$TZ` still wins.

**Dependencies:** None

**Files likely touched:** `internal/appserve/reader.go` (+ `_windows.go`, `_unix.go`, a test)

**Estimated scope:** S

### - [ ] T7: DuckDB's lock message on Windows

**Acceptance criteria:**
- [ ] DuckDB's message on Windows recorded, with the DuckDB anfra ships (v1.5.6), by opening a DuckDB file held by another process.
- [ ] `LockedBy` matches it, and gives the same hint as on Unix (with the PID, when the message has one).

**Verification:**
- [ ] Tests pass (Linux): `go test ./internal/motherduck/`, with the Windows message as a case.
- [ ] On Windows: a query on the demo repo's DuckDB file while another process holds it gives the hint.

**Dependencies:** None

**Files likely touched:** `internal/motherduck/cli.go` (+ test)

**Estimated scope:** S

### Checkpoint B
- [ ] T3–T7's tests pass on Linux and Windows; review with you.

## Phase 3: End to end and close

### - [ ] T8: End to end on Windows 11

**Acceptance criteria:**
- [ ] With the login from Checkpoint A: a command that needs it still works after a token refresh, if one can be triggered.
- [ ] An export downloaded; Data App paths with `\` and `:` refused; the Data App time zone shown is Windows'; the DuckDB lock hint shows.
- [ ] Publish and pull to an org: **only with your go-ahead**, which org, and whether you run it.

**Verification:**
- [ ] Manual, on Windows 11: each step, results recorded.

**Dependencies:** T1–T7

**Files likely touched:** none (results here)

**Estimated scope:** S

### - [ ] T9: Docs, backlog, plan, and the Linux checks

**Acceptance criteria:**
- [ ] The design and usage docs that describe each area say what differs on Windows: logins (`docs/usage/publishing.md`, `docs/designs/security.md`), exports (`docs/designs/exports.md`), Data Apps (`docs/designs/data-apps.md`).
- [ ] `docs/todos/backlog.md`: file names that differ only in case; publish refusing names Windows can't hold.
- [ ] `WINDOWS-PLAN.md`: Where it stands, and the module's result.
- [ ] With your go-ahead: `make check` and `make test`.

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–8, each ticked here.

**Dependencies:** T8

**Files likely touched:** the docs above, `docs/todos/backlog.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint C: Complete
- [ ] Spec success criteria 1–8 met.
- [ ] Ready for review with you (and the commit split proposed).
