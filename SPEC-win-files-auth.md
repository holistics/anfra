# Spec: win-files-auth

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-files-auth`). Depends on: `win-process-lifecycle` (done). Repo: **anfra**. Milestone **M3**: everything works on Windows.

## Objective

anfra compiles and runs on Windows (M1), but the code that saves logins, replaces files, hands out exports, and checks paths assumes Unix. On Windows, `anfra auth login` fails, saves can fail at random with "Access is denied", and two path checks let through names Unix never sees. This module makes each work on Windows, keeping the same promises as on Linux and macOS. Checked against the code on 2026-11-25; the plan's line numbers have moved, and a few of its items changed (marked).

1. **Saved logins** (`internal/cliauth`). **Without these, login and everything that needs it fail on Windows.**
   - `credential_store.go:69` refuses `credentials.json` unless its mode is `600`. Go reports Windows files as `666`, so the check always fails. **Decided with the user: keep the promise, the Windows way.** On Windows, the file is written with an access list of its own (this user only, nothing inherited from the folder, as the socket folder in `win-process-lifecycle`), and a file whose access list lets anyone else read it is refused, with a command that fixes it. `ANFRA_HOME` may point at a shared folder, so the profile's own protection isn't enough.
   - The file read for the inventory stays open (`defer f.Close()`) while `saveCredentialInventory` renames the new file over it (`:117`). Go opens files on Windows without "allow delete", so I expect the rename to fail. Close it once read.
   - `refresh_recovery.go:61` `syncCredentialDirectory` syncs the folder. On Windows a folder can't be flushed that way: check it, and skip it there if it fails.
2. **Replacing a file by rename** (6 places). anfra saves by writing a temporary file and renaming it over the old one. On Windows the rename fails with "Access is denied" (or a sharing violation) while another process has the old file open, e.g. another anfra reading `runtime.json`. One helper retries the rename briefly on those two errors, as Go's own tools do (`cmd/go/internal/robustio`). Used at `cmd/anfra/console_history.go:99`, `cmd/anfra/runtime.go:62`, `internal/cliauth/credential_store.go:117`, `internal/repoconfig/repoconfig.go:303`, `internal/publish/syncstate/syncstate.go:225`. (The plan's `workdir/apply.go:231` no longer renames. `internal/sidecar`'s Windows unpack already handles it.) It lives under `internal/`, not `shared/`: `shared/` is public API (`engine/boundary_test.go`).
3. **Exports** (`cmd/anfra/commands.go`, `internal/exports/exports.go`).
   - `download` (`commands.go:578-595`) deletes the CLI's export file while it's still open (`defer src.Close()`), so on Windows the delete fails silently and the file stays. Close first.
   - The `file:` link is built from the file's path as it is (`exports.go:150`), so on Windows it holds `C:\…`, not the standard `file:///C:/…`; `openLink` (`commands.go:614-621`) reads `u.Path` back as a path. Build and read `file:///C:/…` on Windows; Unix links unchanged.
4. **`RemoveLegacy`** (`internal/home/home.go:37`) deletes `os.UserCacheDir()/anfra`, where anfra kept files before `ANFRA_HOME`. On Windows that's `%LOCALAPPDATA%\anfra`. anfra never ran on Windows before, so there is nothing of its own to remove there: deleting it would be all risk. Skip it on Windows.
5. **Path checks (security).**
   - **Data App files** (`internal/appserve/files.go:73`, `appFile`): dot-files are hidden by splitting the path on `/`. On Windows, `apps/sub\.secret` is one part that doesn't start with `.`, so a hidden file **can be served**. A `:` reaches an NTFS alternate data stream (a hidden extra content stream a file can carry). Climbing out of `apps/` is already blocked by the prefix check after it. Refuse `\` and `:` in the path.
   - **Pull** (`internal/publish/workdir/apply.go:171`, `checkPath`): it already refuses `\`, absolute paths and drives. On Windows it also refuses what Windows can't hold as written: device names (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`, with or without an extension), `:` and `<>"|?*`, control characters, and a trailing dot or space (Windows drops it, so `models/x.` would overwrite `models/x`), each with an error that names the file and why. Linux and macOS unchanged: those names are fine there.
6. **Time zone** (`internal/appserve/reader.go:38`, `localTimezone`): `$TZ`, else `/etc/localtime`'s link, else `Etc/UTC`. Windows has no `/etc/localtime`, so every Data App gets `Etc/UTC`. **Decided with the user:** on Windows, after `$TZ`, ask Windows' own ICU (`icu.dll`, Windows 10 1903+) for the IANA name of the system's zone; fall back to `Etc/UTC` when it's missing (Windows 10 1803/1809, out of support) or fails. No new dependency and no table to keep up: Windows Update keeps ICU's data current ([Microsoft: ICU in Windows](https://devblogs.microsoft.com/oldnewthing/20210527-00/?p=105255)).
7. **DuckDB's lock message** (`internal/motherduck/cli.go:269-278`, `LockedBy`): matches "Could not set lock on file" and "Conflicting lock is held in … (PID n)". DuckDB's message on Windows isn't recorded yet: check it (a DuckDB file open in two processes), and match it too, so the hint naming the process holding the file shows on Windows.

8. **Other files only you may read** (added at Checkpoint B, decided with the user). On Unix, anfra writes `data_sources.yml` (database passwords, by `anfra init`, `cmd/anfra/init.go:250`), the console history (`cmd/anfra/console_history.go:69`) and `runtime.json` (`cmd/anfra/runtime.go:60`) as mode `600`. On Windows they get whatever their folder allows. One helper, `internal/fileperm`, makes a file only you can read (mode `600` on Unix, an owner-only access list on Windows), used for these three and the login file. Export files and the MotherDuck upload's temporary copy also hold your data: asked about at the end.

**Users:** anyone on Windows who logs in, publishes or pulls, exports, or runs Data Apps.

**Out of scope:**
- **File names that differ only in case** (`Orders.aml`, `orders.aml`): pull on Windows writes one over the other. **Backlog** (decided with the user), together with publish refusing names Windows can't hold, so a repo stays portable.
- Line endings (`win-line-endings`), the terminal (`win-terminal`), self-update (`win-self-update`).
- Windows Credential Manager: considered for logins, not chosen.
- CI.

## Tech Stack

- Go 1.27; `golang.org/x/sys/windows` (already used): `SecurityDescriptorFromString`, `GetNamedSecurityInfo`, `GetCurrentProcessToken`, error codes `ERROR_ACCESS_DENIED`, `ERROR_SHARING_VIOLATION`; `icu.dll` called through `windows.NewLazySystemDLL`, as `internal/sidecar` calls `kernel32.dll`.
- Windows 11 x64, the host from the earlier modules (`C:\Users\nuita\wincheck`).

## Commands

```sh
# Each package changed, one command at a time
GOOS=windows GOARCH=amd64 go vet ./internal/cliauth/ ./internal/appserve/ ./internal/publish/... ./internal/home/ ./cmd/anfra/
go test ./internal/cliauth/ ./internal/appserve/ ./internal/publish/... ./internal/home/ ./internal/exports/ ./internal/motherduck/ ./cmd/anfra/

# Windows tests: built in WSL, run on the Windows host
GOOS=windows GOARCH=amd64 go test -c -o /mnt/c/Users/nuita/wincheck/cliauth.test.exe ./internal/cliauth/
GOOS=windows GOARCH=amd64 go build -o /mnt/c/Users/nuita/wincheck/anfra.exe ./cmd/anfra

# Before finishing, with the user's go-ahead
make check
make test
```

## Project Structure

Windows-only code in `_windows.go` files beside a `!windows` one, as in `internal/sidecar` and `cmd/anfra`; no `runtime.GOOS` branches in shared functions.

| Where | Change |
|---|---|
| `internal/cliauth/` | Per-OS: write the file private, and check it's private (`600` on Unix, the access list on Windows); close before rename; the folder sync |
| An `internal/` package for the rename helper | `Replace(tmp, path)`: rename, retrying briefly on Windows' access-denied and sharing-violation errors; a plain rename elsewhere |
| The 5 rename sites above | Use the helper |
| `cmd/anfra/commands.go`, `internal/exports/exports.go` | Close before delete; `file:` links built and read per OS |
| `internal/home/home.go` | `RemoveLegacy` does nothing on Windows |
| `internal/appserve/files.go` | Refuse `\` and `:` |
| `internal/publish/workdir/apply.go` | Windows: the names Windows can't hold |
| `internal/appserve/reader.go` (+ `_windows.go`) | The zone from ICU on Windows |
| `internal/motherduck/cli.go` | Windows' lock message |
| `docs/` | Where each is designed: `docs/designs/` for logins, exports and Data Apps, as each already describes them |
| `docs/todos/backlog.md` | Case-only differences; portable names on publish |

## Code Style

anfra's conventions: [naming](docs/dev/naming.md) before naming anything; [readable prose](docs/designs/readable-prose.md) before any message a user reads, which includes every new error here (each says what's wrong and how to fix it); comments only for why. Shape of a per-OS check:

```go
//go:build windows

// checkPrivate refuses the authentication file when anyone but this user can
// read it: Windows has no mode 600, so its access list makes the same promise.
func checkPrivate(path string) error { … }
```

## Testing Strategy

- **Unit tests on Linux** for the shared logic: the rename helper (a plain rename), `file:` link building and reading, `appFile` refusing `\` and `:`, the lock-message match.
- **Windows tests** (`_windows_test.go`, built with `go test -c` in WSL, run on the Windows host, as in the earlier modules):
  - logins: a saved file has an owner-only access list; a file others can read is refused, with the fix; save, read, save again, with the store's own lock;
  - the rename helper: a rename over a file another handle holds open succeeds once it's closed within the retry time, and fails with a clear error when it never is;
  - `checkPath` refuses each kind of Windows name; `file:` links round-trip with a `C:\` path;
  - the time zone: an IANA name, matching Windows' setting (`Get-TimeZone`).
- **On Windows 11, end to end:**
  - `anfra auth login`, then a command that needs it: **run by the user** (a browser login to a real org);
  - `anfra query … export`, then the download: the file arrives and the CLI's copy is gone;
  - a Data App at `apps/sub\.x` and `apps/x.html:s` is refused;
  - DuckDB's lock: the repo's DuckDB file held by another process gives the hint naming it;
  - publish and pull, **only with the user's go-ahead** (they change an org's data).
- **Linux and macOS unchanged:** existing tests pass; `make check` and `make test` before finishing (with the user's go-ahead).

## Boundaries

- **Always:** keep each Unix promise on Windows, the Windows way (private files stay private); give every new refusal a message saying what and how to fix; run Go checks one command at a time; keep tokens and credentials out of files, logs and this conversation.
- **Ask first:** logging in, publishing or pulling against a real org (the user runs or approves it); adding a dependency; changing what Linux or macOS accept.
- **Never:** relax a check on Unix to make Windows pass; print or store a token anywhere but the credential file; delete a folder anfra didn't create.

## Success Criteria

1. On Windows 11, `anfra auth login` succeeds and a command that needs the login works; `credentials.json` has an owner-only access list; one others can read is refused with a fix. On Unix, the `600` check is unchanged.
2. Saving `runtime.json`, the console's history, the repo config, the sync state and the credentials survives another process holding the old file open briefly (Windows test).
3. On Windows, an export's `file:` link is `file:///C:/…`, the download works, and the CLI's copy is removed after.
4. `RemoveLegacy` touches nothing on Windows.
5. On Windows, `appFile` refuses `\` and `:`; pull's `checkPath` refuses device names, reserved characters, and trailing dots and spaces, each with a clear error. Unix accepts what it accepted before.
6. On Windows, Data Apps get the system's IANA zone (matching `Get-TimeZone`); `$TZ` still wins; `Etc/UTC` when ICU is missing.
7. On Windows, DuckDB's lock message gives the same hint as on Unix.
8. Linux tests, `make check` and `make test` pass; the design docs and `WINDOWS-PLAN.md` are updated; the backlog has the case-only item.
9. On Windows, `data_sources.yml` from `anfra init`, the console history and `runtime.json` have an owner-only access list; on Unix, mode `600` as before.

## Open Questions

None.
