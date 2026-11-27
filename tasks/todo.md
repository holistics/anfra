# Tasks: win-files-auth

Plan: [plan.md](plan.md). Spec: [SPEC-win-files-auth.md](../SPEC-win-files-auth.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: Saving files, and logins

### - [x] T1: One rename helper that survives another process holding the file, used at the 5 save sites

**Description:** anfra saves by writing a temporary file and renaming it over the old one. On Windows the rename fails while another process has the old file open. One helper retries briefly there.

**Acceptance criteria:**
- [x] A new `internal/` package: on Windows, rename, retrying for about a second on access-denied and sharing-violation errors; elsewhere, `os.Rename`. Named per `docs/dev/naming.md`: `internal/robustio`, `robustio.Rename`, after Go's `cmd/go/internal/robustio`; a drop-in for `os.Rename`.
- [x] Used at `cmd/anfra/console_history.go:99`, `cmd/anfra/runtime.go:62`, `internal/cliauth/credential_store.go:117`, `internal/repoconfig/repoconfig.go:303`, `internal/publish/syncstate/syncstate.go:225`.
- [x] After the retries, the error names the file and says another program has it open.

**Verification:**
- [x] Tests pass (Linux): the new package, and the 4 packages of the save sites.
- [x] On Windows (`_windows_test.go`): a rename over a file held open by another handle succeeds once that handle closes within the retry time; fails with the clear error when it never closes. Fails without the helper (a plain `os.Rename`). `TestPlainRenameFailsWhileOpen` shows a plain rename fails while a reader holds the file; the two others failed against a stub, then pass (the give-up in 1.04 s).
- [x] Lint for Windows and Linux.

**Dependencies:** None

**Files likely touched:** the new package (+ tests), the 5 save sites

**Estimated scope:** M

### - [x] T2: Saved logins work on Windows, private to the user

**Description:** Without this, `anfra auth login` and everything that needs it fail on Windows.

**Acceptance criteria:**
- [x] On Windows, the temporary credentials file is made with an owner-only access list (this user, nothing inherited) before any token is written; the renamed file keeps it.
- [x] On read, Windows refuses a file whose access list lets a reader other than this user, SYSTEM or Administrators read it; the error says so and gives an `icacls` command that fixes it. Unix keeps the `600` check and its message.
- [x] The inventory file is closed once read, before the new one is renamed over it.
- [x] `syncCredentialDirectory`: checked on Windows; skipped there if a folder can't be synced. It can't: "Access is denied", which made **every login save fail on Windows**, the first one included; now a no-op there.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/cliauth/`.
- [x] On Windows (`_windows_test.go`): save, read, save again through the store, with its lock; the saved file's access list is owner-only; a file with an Everyone entry is refused with the fix; a file with the usual inherited list (user, SYSTEM, Administrators) is read. Three existing tests that use `chmod` now check privacy per OS (`assertPrivateFile`) or skip on Windows, saying why.

**Dependencies:** T1

**Files likely touched:** `internal/cliauth/credential_store.go`, `refresh_recovery.go`, new `_windows.go` / `_unix.go` files (+ tests)

**Estimated scope:** M

### Checkpoint A
- [x] T1–T2's Windows tests pass; Linux tests pass.
- [x] You run `anfra auth login` on Windows, then a command that needs the login (I'll give the exact steps). Done by the user (2026-11-27): signed in to `https://us.anfra.ai/o/acme`; `auth status`: saved login `valid`. `credentials.json`: `LAPTOP\nuita` full control, nothing inherited.

## Phase 2: The smaller fixes

### - [x] T3: Exports: the download removes the CLI's copy; `file:///C:/…` links

**Acceptance criteria:**
- [x] `download` closes the CLI's export file before removing it.
- [x] On Windows, the export's `file:` link is `file:///C:/…`, and `openLink` reads it back to the path; Unix links unchanged. `exports.FileLink` and `exports.FileLinkPath`, with no OS branch: `filepath` already behaves per OS. Before, Windows' link was `file://C:%5CUsers%5C…`, which didn't even parse.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/exports/ ./cmd/anfra/`; a link round-trip test.
- [x] On Windows: the round-trip test; a query export downloaded, and the CLI's copy gone after (`TestDownload`, which failed before; the real export is in T8). Also found: `internal/exports`' tests shared the real `%TEMP%` on Windows (they set `TMPDIR`, which Windows ignores) and one left an export open; both fixed in the tests. On Windows, an export removed while it's being downloaded stays until the next sweep: Windows can't delete an open file. Accepted.

**Dependencies:** None

**Files likely touched:** `cmd/anfra/commands.go`, `internal/exports/exports.go` (+ per-OS link files and tests)

**Estimated scope:** S

### - [x] T4: Path checks: Data App files refuse `\` and `:`; pull refuses names Windows can't hold

**Acceptance criteria:**
- [x] `appFile` returns "" for a path holding `\` or `:`, on every OS (neither belongs in a URL path to a Data App file).
- [x] On Windows, `checkPath` refuses device names (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`, with or without an extension, any case), `:` and `<>"|?*`, control characters, and a trailing dot or space in any part, each with an `UnsafePathError` saying why.
- [x] Linux and macOS: `checkPath` accepts what it accepted.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/appserve/ ./internal/publish/...`, with `appFile` cases for `\` and `:`.
- [x] On Windows: `checkPath` cases for each kind; before the fix, `apps/sub\.x` is served (shown, then fixed). Before: `appFile("sub\\.secret")` served `apps\sub\.secret`, and 15 of 18 bad names passed `checkPath`; after: all refused, each with its reason. Found, outside this module: on Windows, the Data Apps live-reload watcher also gets folder events, so it reports apps that didn't change (`TestWatch`, `TestLiveReloadWatchesOnlyWhileAStreamIsOpen` fail there); and `TestFiles` and `TestCheckPath` need symlinks, which Windows allows only to administrators or in developer mode. Reported to the user.

**Dependencies:** None

**Files likely touched:** `internal/appserve/files.go`, `internal/publish/workdir/apply.go` (+ a Windows names file and tests)

**Estimated scope:** M

### - [x] T5: `RemoveLegacy` does nothing on Windows

**Acceptance criteria:**
- [x] On Windows, `RemoveLegacy` removes nothing; Linux and macOS unchanged. The comment says why (anfra never kept files there on Windows).

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/home/`.
- [x] On Windows: a test with `%LOCALAPPDATA%` pointed at a temporary folder holding `anfra\`: it's still there after. It was deleted before the fix. `TestRemoveLegacy` (Linux's `XDG_CACHE_HOME`) now skips on Windows: run there, it deleted the real `%LOCALAPPDATA%\anfra`, as did every embedded anfra build's unpack in earlier modules. None exists on the Windows host now; whether one did before isn't known. `TestDir` also sets `USERPROFILE`, which Windows reads instead of `HOME`.

**Dependencies:** None

**Files likely touched:** `internal/home/home.go` (+ a per-OS file and a test)

**Estimated scope:** XS

### - [x] T6: Data Apps get Windows' time zone, from ICU

**Acceptance criteria:**
- [x] On Windows, `localTimezone`: `$TZ`, else the IANA name `icu.dll` gives for the system's zone, else `Etc/UTC`. Unix unchanged.
- [x] A missing `icu.dll` or a failed call falls back to `Etc/UTC`, never an error.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/appserve/`.
- [x] On Windows: the name matches Windows' setting (`Get-TimeZone`, mapped by hand once); `$TZ` still wins. Checked by offset instead of by hand: the zone ICU gives has Windows' own offset now (`TestLocalTimezoneIsWindows`, `Etc/UTC` before the fix). Windows here is "SE Asia Standard Time" in Vietnam; ICU answers `Asia/Saigon` (the older name of `Asia/Ho_Chi_Minh`, still ICU's standard ID, which Go and browsers accept).

**Dependencies:** None

**Files likely touched:** `internal/appserve/reader.go` (+ `_windows.go`, `_unix.go`, a test)

**Estimated scope:** S

### - [x] T7: DuckDB's lock message on Windows

**Acceptance criteria:**
- [x] DuckDB's message on Windows recorded, with the DuckDB anfra ships (v1.5.6), by opening a DuckDB file held by another process. With DuckDB's Windows CLI v1.5.6: `IO Error: Cannot open file "…": The process cannot access the file because it is being used by another process.` then `File is already open in\n<program> (PID n)`. Unlike Unix, a read-only open is refused too, when the holder has the file read-write.
- [x] `LockedBy` matches it, and gives the same hint as on Unix (with the PID, when the message has one).

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/motherduck/`, with the Windows message as a case.
- [x] On Windows: a query on the demo repo's DuckDB file while another process holds it gives the hint. Checked the case publish meets: with `anfra serve`'s canal holding the demo file (read-only), DuckDB still opens it read-only and the file copies, so publish's copy fallback works on Windows. The hint itself: the new `TestQueryErrors` case with the Windows message (MotherDuck's CLI needs a MotherDuck login: in T8, with the user).

**Dependencies:** None

**Files likely touched:** `internal/motherduck/cli.go` (+ test)

**Estimated scope:** S

### Checkpoint B
- [x] T3–T7's tests pass on Linux and Windows; review with you. Found: the files below (T8); `anfra logs`, the live-reload watcher and symlink tests fail on Windows, added to `win-terminal` in `WINDOWS-PLAN.md`; Windows ran out of memory from WSL's file cache (work is lighter now: `GOMAXPROCS=4 GOFLAGS=-p=2`, one build and lint per task).

### - [x] T8: Other files only you may read, on Windows too

**Description:** Added at Checkpoint B. On Unix, `data_sources.yml` (database passwords), the console history and `runtime.json` are mode `600`; on Windows they get whatever their folder allows.

**Acceptance criteria:**
- [x] `internal/fileperm`: `OwnerOnly(f)` (mode `600` on Unix; on Windows an access list of this user only, nothing inherited), `WriteFile(name, data)`, and `IsOwnerOnly(path)` for tests on both OSes.
- [x] Used by `init`'s `data_sources.yml`, the console history (appended and rewritten), `runtime.json`, and the login file (replacing `cliauth`'s own `makePrivate`).
- [x] Unix behaviour unchanged (mode `600`).

**Verification:**
- [x] Tests pass (Linux): the new package, `./cmd/anfra/`, `./internal/cliauth/`.
- [x] On Windows: `TestConsoleHistory` and `TestInitSetsUpARepo`'s mode checks (failing today) pass through `IsOwnerOnly`; the `fileperm` tests pass. Also passing now on Windows: `TestAuthStatusExplainsDamagedCredentials` (it looked for a path with `\` in a JSON answer, which writes `\\`; fixed in the test, `997a0a3`). The new access check accepts an ordinary file there, as it should.
- [x] Lint once, for Windows and Linux.

**Dependencies:** T2

**Files likely touched:** `internal/fileperm/` (new), `cmd/anfra/init.go`, `console_history.go`, `runtime.go`, `internal/cliauth/` (+ tests)

**Estimated scope:** M


## Phase 3: End to end and close

### - [ ] T9: End to end on Windows 11

**Acceptance criteria:**
- [ ] With the login from Checkpoint A: a command that needs it still works after a token refresh, if one can be triggered.
- [x] An export downloaded; Data App paths with `\` and `:` refused; the Data App time zone shown is Windows'; the DuckDB lock hint shows. Done on Windows 11 (2026-11-26): `query export -o` wrote the file and left no CLI copy; `--link` gave `file:///C:/Users/…/anfra-exports/…`; through `anfra serve`, `apps/x.html` is 200 and `sub%5C.secret`, `x.html%3Ahidden`, `sub/.secret` are 404; the reader's zone is `Asia/Saigon`; `init`'s `data_sources.yml` and serve's `serve.json` hold one entry, the user, full control, nothing inherited. The lock hint needs MotherDuck's CLI and a MotherDuck login: with the publish check below.
- [ ] Publish and pull to an org: **only with your go-ahead**, which org, and whether you run it.

**Verification:**
- [ ] Manual, on Windows 11: each step, results recorded.

**Dependencies:** T1–T8

**Files likely touched:** none (results here)

**Estimated scope:** S

### - [ ] T10: Docs, backlog, plan, and the Linux checks

**Acceptance criteria:**
- [x] The design and usage docs that describe each area say what differs on Windows: logins (`docs/usage/publishing.md`, `docs/designs/security.md`), exports (`docs/designs/exports.md`), Data Apps (`docs/designs/data-apps.md`).
- [x] `docs/todos/backlog.md`: file names that differ only in case; publish refusing names Windows can't hold.
- [x] `WINDOWS-PLAN.md`: Where it stands, and the module's result.
- [x] With your go-ahead: `make check` and `make test`. (Run lightly: `GOMAXPROCS=4 GOFLAGS=-p=2`.) Not run, decided by the user: CI runs them on the PR.

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–9, each ticked here.

**Dependencies:** T9

**Files likely touched:** the docs above, `docs/todos/backlog.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### - [x] T11: Export files and MotherDuck's upload copy, only you may read

**Description:** Asked at the end, decided by the user. Both hold your data (query results; a copy of a DuckDB file) and are mode `600` on Unix, in the temp folder, which `TMP` may point at a shared folder.

**Acceptance criteria:**
- [x] `internal/exports`: an export's file is made with `fileperm.OwnerOnly` before anything is written.
- [x] `internal/publish`'s `copyFile` (MotherDuck's upload copy, and its log): the same.
- [x] Unix unchanged (mode `600`).

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/exports/ ./internal/publish/`, each checking its file with `fileperm.IsOwnerOnly`.
- [x] On Windows: the same tests, one build each; lint once per OS. Both failed on Windows before the change (not owner-only), and pass after.

**Dependencies:** T8

**Files likely touched:** `internal/exports/exports.go`, `internal/publish/motherduck.go` (+ tests)

**Estimated scope:** S

### Checkpoint C: Complete
- [ ] Spec success criteria 1–9 met.
- [ ] Ready for review with you (and the commit split proposed).
