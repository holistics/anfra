# Tasks: win-release

Plan: [plan.md](plan.md). Spec: [SPEC-win-release.md](../SPEC-win-release.md). Project plan: [WINDOWS-PLAN.md](../WINDOWS-PLAN.md).

## Phase 1: The sidecars build for Windows

### - [x] T1: canal's `windows-x64` row

**Acceptance criteria:**
- [x] `build_and_release_binaries.yml` (on a branch from canal's `origin/master`) has `{ target: windows-x64, runner: windows-2022 }`. On it: MinGW-Builds GCC 14.2.0 (posix, SEH, UCRT) and DuckDB's `libduckdb-windows-amd64.zip` (the version the bindings pin, v1.5.6), each downloaded by URL with its sha256 checked; the build with `CGO_LDFLAGS=-L<dll folder>` and `-tags=duckdb_arrow,duckdb_use_lib`.
- [x] It outputs `<app>-windows-x64.exe` and `<app>-windows-x64-duckdb.dll`, both in the row's artifact and so in the release; `<app>-windows-x64.exe --version` answers beside the DLL.
- [x] The other rows' steps are unchanged.

**Verification:**
- [x] By hand on Windows (memory checked first): the row's commands, run in Git Bash or as the same commands in PowerShell, build `anfra-canal-windows-x64.exe`, and `--version` answers.
- [x] The workflow parses (`actionlint`, else a YAML parse). `actionlint` isn't installed: the YAML parses.

**Result** (2026-11-27, canal `feat/windows-release` `4f7cbb9`, a worktree at `../canal-win` from `origin/master`, as the local checkout has someone's changes): the row's three scripts, taken from the YAML, ran in Git Bash on Windows 11 with the runner's variables faked. MinGW's and DuckDB's checksums matched; the build used the row's `gcc`; `anfra-canal-windows-x64.exe` (164 MB) answered `--version` beside its DLL (`1.1.1`, `canal-query 2.28.0`, `canal-lake 2.10.0`); out came the `.exe` and `-duckdb.dll`. Also: the step fails when `go.mod`'s `duckdb-go-bindings` isn't the DLL's version, so the two can't drift apart.

**Dependencies:** None

**Files likely touched:** canal `.github/workflows/build_and_release_binaries.yml`

**Estimated scope:** S

### - [x] T2: holistics-core's `windows-x64` row

**Acceptance criteria:**
- [x] `anfra_node_build_release.yml` (on a branch after `feat/anfra-node-windows-stop`) has `{ target: windows-x64, runner: windows-2022 }`, with the same steps, producing `anfra-node-windows-x64.exe` and `anfra-node-windows-x64-unminified.exe`, uploaded as the others are, so the release publishes them.
- [x] A smoke step: the minified binary starts and answers (its `--version`, or what anfra-node offers instead).
- [x] The `docker` job still takes only the Linux binary; the other rows' steps are unchanged.

**Verification:**
- [x] Changed: rather than a cross-build in WSL, the row's steps ran natively on Windows 11 (portable Node 24, pnpm 11.1.3, Bun 1.4.4 in `wincheck\toolchain`), as the runner would.
- [x] The workflow parses.
- [x] The native Windows build itself only on GitHub (Checkpoint A). A failure caused by the runner is fixed in the row; one in holistics-core's own code or packages goes to you, with the error.

**Dependencies:** None

**Files likely touched:** holistics-core `.github/workflows/anfra_node_build_release.yml`

**Estimated scope:** S

**Result** (2026-11-27, holistics-core `feat/anfra-node-windows-release`, on top of the SIGBREAK commit; draft PR holistics/holistics-core#3379): the native Windows build needed two fixes in holistics-core, each its own commit: `packages/aml-std`'s build keyed `.aml` files by `path.relative`, backslashes on Windows, so the build failed (`d48ee0b40`); `build-sidecar.sh` read `package.json` through a `/c/...` path Node can't open, so the version was `unknown` (`2226ad94c`). Then the row (`1d2769991`): checkout keeps LF, the npmrc step runs in bash (PowerShell read `${GH_TOKEN}` as its own variable), and the smoke test refuses `unknown`. Both binaries built; `--version` answers `0.17.1` (aml 9.9.0). Locally, GitHub Packages was slow (0.3 MB/s), so pnpm's fetch timeout was raised for these runs only.

### Checkpoint A
- [ ] Both Windows binaries built by hand, `--version` answers.
- [ ] You decide: push both branches and run each workflow as a test build (no release)?

## Phase 2: anfra's release and its installers

### - [x] T3: anfra's release builds `anfra-windows-x64.zip`

**Acceptance criteria:**
- [x] `packaging/windows/THIRD_PARTY_NOTICES.txt`: DuckDB's MIT license (v1.5.6), saying what it covers (`duckdb.dll`, inside `anfra.exe`).
- [x] `build_release.yml` downloads `anfra-node-windows-x64.exe`, `anfra-canal-windows-x64.exe` and `anfra-canal-windows-x64-duckdb.dll`, then checks that every expected sidecar file is there, failing with the missing ones' names.
- [x] `windows-x64` is in the target loop: the three files at `assets/anfra-node`, `assets/anfra-canal`, `assets/duckdb.dll`; `anfra.exe` built with `GOOS=windows GOARCH=amd64` and the same flags; packed as `release/anfra-windows-x64.zip` with `anfra.exe` and `THIRD_PARTY_NOTICES.txt` at its top. `SHA256SUMS`, the upload and the artifacts take it without change. Linux and macOS unchanged.

**Verification:**
- [x] The loop's commands, run by hand with T1's and T2's binaries, give the `.zip`. On Windows 11 it unzips, `anfra version` runs, a DuckDB query answers, and MotherDuck's extension loads (a MotherDuck query only if you set a token).
- [x] The missing-file check, run by hand with a file taken away, names it.
- [x] The workflow parses.

**Dependencies:** T1, T2 (their binaries, for the check by hand)

**Files likely touched:** `.github/workflows/build_release.yml`, `packaging/windows/THIRD_PARTY_NOTICES.txt`

**Estimated scope:** S

**Result** (2026-11-27): the workflow's own scripts, taken from the YAML, ran in WSL with T1's and T2's Windows binaries (placeholders for the other platforms, the loop cut to `windows-x64`). The check passed with every file there, and with the DLL taken away failed naming it and both pins. The loop gave `anfra-windows-x64.zip` (127 MB: `anfra.exe` 355 MB and `THIRD_PARTY_NOTICES.txt`). On Windows 11, unzipped with `Expand-Archive`: `anfra version` 9.9.8; `anfra init demo --example`; `anfra show`; a query answered 4 rows through `duckdb.dll` (both sidecars unpacked, the DLL beside `anfra-canal.exe`). A MotherDuck data source with a made-up token got MotherDuck's own refusal (`Your request is not authenticated`), which needs the `windows_amd64` extension loaded. The notice is LF, as the repo has no `.gitattributes`; Notepad reads it.

### - [x] T4: `install.ps1`

**Acceptance criteria:**
- [x] A new `install.ps1`, mirroring `install.sh`: the same overrides (`ANFRA_HOME`, `ANFRA_INSTALL_DIR`, `ANFRA_VERSION`, `ANFRA_NO_MODIFY_PATH`) plus `ANFRA_DOWNLOAD_URL`; downloads the `.zip` and `SHA256SUMS`, refuses a mismatch or a missing line, saying why and what to do; installs `anfra.exe` and `THIRD_PARTY_NOTICES.txt`; adds the folder to the user PATH; says to open a new terminal; the same next steps.
- [x] On ARM, installs x64 with the note; another CPU refuses, saying so.
- [x] No `exit`; plain ASCII, no byte-order mark; works in PowerShell 5.1 (and 7). Every message naming it says `irm https://anfra.ai/install.ps1 | iex`.

**Verification:**
- [x] Windows, PowerShell 5.1, with `ANFRA_HOME` in a temporary folder: from the file (`Get-Content install.ps1 -Raw | iex`); through the pipe (`irm http://localhost:8000/install.ps1 | iex`, served from WSL); from a local release (`ANFRA_DOWNLOAD_URL`), installing and then refusing a wrong `SHA256SUMS`. The window stays open on each failure; `anfra version` runs in a new terminal.
- [ ] A fresh Windows user account, for a clean machine: skipped for now (decided with you), to try with the first real release.
- [x] `Invoke-ScriptAnalyzer install.ps1` (PSScriptAnalyzer: installing it is asked first).

**Dependencies:** T3 (its `.zip`)

**Files likely touched:** `install.ps1`

**Estimated scope:** M

**Result** (2026-11-27, Windows 11, Windows PowerShell 5.1, a local release served from WSL with T3's `.zip`):
- Through the pipe (`irm http://localhost:8000/install.ps1 | iex`) and from the file: installed, checksum shown, `anfra version` 9.9.8 in the same window.
- A wrong `SHA256SUMS`, and nothing at the URL: refused, nothing installed, the window open, no temporary files left.
- Reinstalling while `anfra serve` runs from the folder: refused, saying to stop anfra and how to run it again.
- The user PATH (yours, saved first and restored exactly, value and type): the folder added once, a second run adds nothing, a new session finds `anfra`. Found and fixed on the way: `[Environment]::SetEnvironmentVariable` would have rewritten the user PATH expanded, turning `%...%` entries into fixed paths, and as another registry type. The script reads and writes the registry value as stored, keeping its type, and sets and clears a user variable so open programs hear of the change.
- PSScriptAnalyzer 1.25.0 (installed for your user): only `PSAvoidUsingWriteHost`, which suits a script that talks to a person; nothing else.
- Not checked: PowerShell 7 (not installed here); ARM, by reading the code only.

### - [x] T5: `install.sh` on Git Bash points to `install.ps1`

**Acceptance criteria:**
- [x] On Git Bash, MSYS or Cygwin (`uname -s` starting `MINGW`, `MSYS` or `CYGWIN`), `install.sh` stops with a message giving `irm https://anfra.ai/install.ps1 | iex`, instead of "unsupported OS".
- [x] Its header comment no longer says verification is still to come in `anfra update`: `anfra update` verifies now.

**Verification:**
- [x] Run with a stubbed `uname` for each of the three; and on Linux, unchanged.
- [ ] `shellcheck install.sh`: not installed here.

**Dependencies:** T4

**Files likely touched:** `install.sh`

**Estimated scope:** XS

**Result** (2026-11-27): with `uname -s` stubbed as `MINGW64_NT`, `MSYS_NT` and `CYGWIN_NT`, `install.sh` stops with the PowerShell line; `FreeBSD` still gets "unsupported OS"; Linux goes on to the download as before. In Git Bash on Windows 11, the same message.

### Checkpoint B
- [ ] On Windows 11, `install.ps1` from a local release installs T3's `.zip`; `anfra` answers queries in a new terminal, MotherDuck's extension included; a wrong checksum refuses.
- [ ] Review with you.

## Phase 3: anfra's PR checks

### - [ ] T6: `GOOS=windows go vet ./...` in the Linux checks

**Acceptance criteria:**
- [ ] `continuous_integration.yml`'s `test` job runs `GOOS=windows go vet ./...` after its `go vet`.

**Verification:**
- [ ] It passes here (lightly).
- [ ] The workflow parses.

**Dependencies:** None

**Files likely touched:** `.github/workflows/continuous_integration.yml`

**Estimated scope:** XS

### - [ ] T7: the package list's tests pass on Windows

**Acceptance criteria:**
- [ ] `go test` passes on Windows 11 for `internal/sidecar/...`, `internal/cliauth`, `cmd/anfra`, `internal/publish/...`, `internal/exports`, `internal/home`, `internal/update`, `internal/fileperm`, `internal/robustio`.
- [ ] Each test that failed only because of its setup is fixed, or skipped on Windows with the reason in the skip; none of a real bug is skipped (a real one is fixed, or brought to you).

**Verification:**
- [ ] First a list of what fails on Windows today, per package; past a handful, split into tasks by package and tell you.
- [ ] Then the list passes on Windows (one test build per package, memory checked first, the notifier off, no copies left); the changed packages pass on Linux; lint, Linux and Windows.

**Dependencies:** None

**Files likely touched:** `*_test.go` in those packages

**Estimated scope:** M (to be confirmed by the list)

### - [ ] T8: the required `windows` CI job

**Acceptance criteria:**
- [ ] `continuous_integration.yml` has a `windows` job on `windows-2022`, gated by the `changes` job as the `test` job is, running `go test -count=1` on T7's list, with `ANFRA_NO_UPDATE_NOTIFIER=1`.
- [ ] `ci_passed` requires it.

**Verification:**
- [ ] The workflow parses; its `go test` line, run on Windows here, is T7's.
- [ ] On GitHub only with a pushed PR: listed for you.

**Dependencies:** T7

**Files likely touched:** `.github/workflows/continuous_integration.yml`

**Estimated scope:** XS

### Checkpoint C
- [ ] The package list passes on Windows here; the workflows parse.

## Phase 4: Close

### - [ ] T9: Docs and plan

**Acceptance criteria:**
- [ ] `docs/designs/release.md`: the Windows target (the sidecars it downloads, the `.zip` and its notice), `install.ps1`, the Windows PR checks, and the order of merging.
- [ ] `WINDOWS-PLAN.md`: Where it stands, the module's result, and what's left only GitHub can check.

**Verification:**
- [ ] Manual: the spec's Success Criteria 1–6, each ticked here, or marked as waiting for GitHub.

**Dependencies:** T1–T8

**Files likely touched:** `docs/designs/release.md`, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint D: Complete
- [ ] Spec success criteria 1–6 met, or what only GitHub can check listed, with the order of merging.
- [ ] Ready for review with you (and the commit split proposed).
