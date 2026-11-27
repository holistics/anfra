# Spec: win-self-update

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-self-update`). Depends on: `win-process-lifecycle` (done). Repo: **anfra**. Milestone **M3**. `win-release` builds the Windows asset this module downloads.

## Objective

`anfra update` replaces the running anfra with the newest release (`internal/update`). On Windows it can't today: there's no Windows asset name, the download is read as `.gz` only, and the advice on failure points to `install.sh`. Checked against the code on 2026-11-27.

**Decided with the user (2026-11-27):**
- **The Windows asset is `anfra-windows-x64.zip`,** what Windows users expect, opened with a double-click for a manual install. `anfra update` unzips it on Windows; Linux and macOS keep their `.gz` assets.
- **`anfra update` verifies a checksum, on every OS.** Windows ships unsigned at first, with `install.ps1` verifying SHA-256; `anfra update` makes the same promise, on Linux and macOS too, where it checks nothing today.

**What changes:**

1. **The Windows asset** (`internal/update/update.go:79`, `assetName`): `windows/amd64` → `anfra-windows-x64.zip`. `Latest` (`update.go:124`) looks for the asset by its full name, `.gz` or `.zip`, rather than adding `.gz`.
2. **Unzip on Windows** (`update.go:242`): the `.zip` holds `anfra.exe`; it's read from the archive and handed to `selfupdate.Apply`, as the `.gz`'s content is today.
3. **Checksums, every OS:** each release publishes `SHA256SUMS` (the sha256 of each asset file, `sha256sum` format). `anfra update` downloads the asset to a temporary file, checks its sha256 against `SHA256SUMS`, and only then unpacks and applies it. A mismatch, or a release without `SHA256SUMS`, refuses the update with a message that says why and how to install by hand. The temporary file is removed after, whatever happens. A Windows asset is about 370 MB with its sidecars embedded, so it's never held in memory.
4. **The release publishes `SHA256SUMS`** (`.github/workflows/build_release.yml`): one step after the assets are built. **Order matters:** an anfra that verifies must never update to a release without `SHA256SUMS`, so the workflow step ships **in or before** the first release whose anfra verifies.
5. **The old binary on Windows:** `minio/selfupdate` renames the running `anfra.exe` to `.anfra.exe.old` and writes the new one in its place. Before updating, anfra removes a leftover `.anfra.exe.old` from an earlier update; when Windows refuses (an older `anfra serve` still runs it), it says so, and that the update still goes ahead.
6. **The advice on failure** (`update.go:249`): on Windows, `irm https://anfra.ai/install.ps1 | iex` instead of `install.sh`.
7. **The console restarts on the new anfra** after `anfra update` inside `anfra console` (`restartConsole`, from `win-process-lifecycle`, so far checked by `TestRestartWith` only). Checked end to end on Windows with a **local fake release** (the update code's `webBase`/`rawBase`, as its tests use), not a real release.

**Users:** anyone updating anfra, on any OS (the checksum), and on Windows (all of it).

**Out of scope:** building the Windows asset and `install.ps1` (`win-release`); signing; `install.sh` verifying its checksum (it prints it, "no verification yet"); arm64.

## Tech Stack

Go 1.27, standard library (`archive/zip`, `crypto/sha256`); `minio/selfupdate` (already used). GitHub Releases. Windows 11 x64 for the end-to-end check.

## Commands

```sh
# Lightly (GOMAXPROCS=4 GOFLAGS=-p=2), one command at a time
GOMAXPROCS=4 GOFLAGS=-p=2 go test ./internal/update/ ./cmd/anfra/
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go vet ./internal/update/ ./cmd/anfra/
GOMAXPROCS=4 tmp/bin/golangci-lint run ./internal/update/ ./cmd/anfra/   # once per task, per OS

# Windows: one test build per task; after any run of cmd/anfra's test program, no copies left and the program deleted
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go test -c -o /mnt/c/Users/nuita/wincheck/update.test.exe ./internal/update/
```

## Project Structure

| Where | Change |
|---|---|
| `internal/update/update.go` | Windows asset name; the asset found by its full name; download to a temporary file; checksum check; unzip on Windows; the leftover `.old`; the advice per OS |
| `internal/update/*_test.go` | A fake release with `SHA256SUMS`: a good update, a bad sum, a missing `SHA256SUMS`, a `.zip` |
| `.github/workflows/build_release.yml` | Publish `SHA256SUMS` (approved at planning) |
| `docs/designs/release.md`, `docs/usage/` | `SHA256SUMS`, and what `anfra update` checks |

## Code Style

anfra's conventions: [naming](docs/dev/naming.md), [readable prose](docs/designs/readable-prose.md) for every message (each refusal says why and how to install by hand), comments only for why; per-OS code in build-tagged files where it differs (the unzip, the `.old`, the advice).

## Testing Strategy

- **Linux** (`internal/update`, the existing fake-release server): an update with a matching `SHA256SUMS` applies; a mismatch refuses and changes nothing; a release without `SHA256SUMS` refuses; the temporary file is gone after each.
- **Windows** (`_windows_test.go`, built in WSL, run on the host): the `.zip` asset unzips and applies; a leftover `.anfra.exe.old` is removed, or reported when in use.
- **End to end on Windows 11:** a local fake release (a newer `anfra.exe` zipped, with `SHA256SUMS`) served from WSL; `anfra update` from a PowerShell window, then from inside `anfra console`, which restarts on the new version (`anfra version` shows it). Run by the user where a terminal is needed.

## Boundaries

- **Always:** verify before replacing anything; never leave a half-written anfra (selfupdate's rename does this; keep it); remove temporary files; keep a refusal's message actionable; run Go lightly, and check Windows' memory before running there.
- **Ask first:** changing `build_release.yml` beyond the `SHA256SUMS` step (approved); cutting a release.
- **Never:** apply an update whose checksum doesn't match, or that has none; download from anywhere but the release (or a test's own server).

## Success Criteria

1. On Windows, `anfra update` finds `anfra-windows-x64.zip`, unzips it, and replaces `anfra.exe`; Linux and macOS still use their `.gz`.
2. On every OS, `anfra update` applies only an asset whose sha256 matches the release's `SHA256SUMS`; a mismatch or a missing `SHA256SUMS` refuses, saying why and how to install by hand, and changes nothing.
3. On Windows, a leftover `.anfra.exe.old` is removed before an update, or reported when in use, and the update goes ahead.
4. On Windows, a failure's advice points to `install.ps1`.
5. Inside `anfra console` on Windows, `anfra update` restarts the console on the new anfra (checked with a local fake release).
6. `build_release.yml` publishes `SHA256SUMS`, in or before the first release whose anfra verifies; docs and `WINDOWS-PLAN.md` updated (the format decision recorded).

## Open Questions

None. The `build_release.yml` step is in this module (decided with the user at planning), committed with the verifying update so the order is safe.
