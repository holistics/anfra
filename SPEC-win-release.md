# Spec: win-release

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-release`). Depends on: `win-process-lifecycle`, `win-canal`, `win-anfra-node`, `win-self-update` (all done in code). Repos: **anfra, canal, holistics-core**. Milestones **M2** (sidecars ship) and **M4** (users can get it), and the Windows CI job of **M3**.

## Objective

Make a Windows anfra that people can install and that CI keeps working. Today no repo builds Windows: anfra's release (`build_release.yml`) downloads its two sidecars, anfra-node and anfra-canal, from their own repos' releases, and neither repo publishes a Windows build. Checked against the code on 2026-11-27.

**Decided with the user (2026-11-27):**
- **The same as the other OSes, as much as possible.** Each repo builds Windows the way it already builds Linux and macOS: one more `windows-x64` row in the same build list (matrix), on a native runner, published in the same release, under the same file-name pattern. anfra downloads and embeds them as it does the others. So win-release covers all three repos.
- **MotherDuck works on Windows:** anfra embeds canal's DLL build, which needs DuckDB's own `duckdb.dll`, and `anfra-windows-x64.zip` carries DuckDB's license (MIT) in a `THIRD_PARTY_NOTICES.txt`. Notices for every OS stay in the [backlog](docs/todos/backlog.md#third-party-license-notices-in-releases).
- **The Windows CI job is required,** as the Linux tests are: part of `ci_passed`.
- **On Windows on ARM, `install.ps1` installs the x64 anfra,** with a note that it runs through emulation, slower.

**What changes:**

1. **canal** (`.github/workflows/build_and_release_binaries.yml`): a `windows-x64` row on a Windows runner. It builds the DLL build as `build-windows-dll.ps1` does: MinGW-Builds GCC 14.2.0 (posix, SEH, UCRT), DuckDB's `libduckdb-windows-amd64.zip` of the version the Go bindings pin (today v1.5.6, its sha256 checked), `CGO_LDFLAGS=-L<dll folder>`, `-tags=duckdb_arrow,duckdb_use_lib`. It publishes `<app>-windows-x64.exe`, plus `<app>-windows-x64-duckdb.dll`, the DLL it was built against. Work from canal's `origin/master` (the local checkout is old).
2. **holistics-core** (`.github/workflows/anfra_node_build_release.yml`): a `windows-x64` row on a Windows runner, the same steps (Bun 1.4.4), publishing `anfra-node-windows-x64.exe` and its `-unminified` one. It goes after `feat/anfra-node-windows-stop` (the `SIGBREAK` handler), which ships in the same anfra-node release.
3. **anfra's release** (`build_release.yml`): `windows-x64` in the target list. It downloads `anfra-node-windows-x64.exe`, `anfra-canal-windows-x64.exe` and `anfra-canal-windows-x64-duckdb.dll`, puts them at `assets/anfra-node`, `assets/anfra-canal` and `assets/duckdb.dll` (`embed_release_windows.go` already expects them), builds `anfra.exe` with `GOOS=windows GOARCH=amd64`, and packs `anfra-windows-x64.zip` with `anfra.exe` and `THIRD_PARTY_NOTICES.txt` at its top. `SHA256SUMS` already covers every `anfra-*` file. A pinned sidecar release without its Windows file fails the release, saying which file is missing, as a missing Linux or macOS one does.
4. **`install.ps1`** (new, repo root, beside `install.sh`), run as `irm https://anfra.ai/install.ps1 | iex`. It does what `install.sh` does, the Windows way:
   - the same overrides: `ANFRA_HOME`, `ANFRA_INSTALL_DIR`, `ANFRA_VERSION`, `ANFRA_NO_MODIFY_PATH`; plus `ANFRA_DOWNLOAD_URL`, where to download the release from, for testing;
   - downloads `anfra-windows-x64.zip` and the release's `SHA256SUMS`, **refuses a mismatch** (what stands in for signing), and installs `anfra.exe` and `THIRD_PARTY_NOTICES.txt` into `%USERPROFILE%\.anfra\bin`;
   - adds that folder to the **user** PATH (no administrator), unless `ANFRA_NO_MODIFY_PATH`; says to open a new terminal; prints the same next steps;
   - on ARM, installs x64 with the note.
   - Rules for a script run through `iex`: no `exit` (it would close the user's window: `return` or `throw`), plain ASCII with no byte-order mark, works in Windows PowerShell 5.1 and PowerShell 7.
   - Every message that names it says `irm https://anfra.ai/install.ps1 | iex`, never `.\install.ps1` (PowerShell's execution policy blocks a downloaded `.ps1` file).
5. **`install.sh`** on Git Bash, MSYS or Cygwin points to `install.ps1`, instead of "unsupported OS". It also stops saying its download is unchecked in a way that's now wrong: its comment points at `anfra update`, which does verify. (`install.sh` checking `SHA256SUMS` itself stays out of scope.)
6. **anfra's PR checks** (`continuous_integration.yml`):
   - `GOOS=windows go vet ./...` in the Linux job, so a Windows compile break shows in any PR.
   - A required `windows` job (Windows runner) running `go test` for the packages with Windows code: `internal/sidecar/...`, `internal/cliauth`, `cmd/anfra`, `internal/publish/...`, `internal/exports`, `internal/home`, `internal/update`, `internal/fileperm`, `internal/robustio`. Their tests that fail on Windows only because of how they're set up (file modes, `HOME`, symlinks, `/tmp`, `/bin/sh` fakes) are fixed, or skipped on Windows with the reason.
7. **A smoke test in each sidecar repo's Windows row:** the built binary answers `--version` (anfra-canal next to its `duckdb.dll`), so a Windows build that doesn't start fails its own repo's release, not anfra's.

**Users:** Windows users installing anfra (`install.ps1`, the `.zip` by hand); anfra's, canal's and holistics-core's maintainers (their releases and PR checks).

**Out of scope:**
- **Signing** (later).
- **arm64 builds.**
- **The Cloudflare redirect for `anfra.ai/install.ps1`, and the install docs** (`win-skills-docs`).
- **A minimum-OS check of every release binary** (the plan's guard): see Open Questions.
- **Third-party notices for Linux and macOS** (backlog).
- **A Windows Docker image.**

## Tech Stack

GitHub Actions (`windows-2022` runners, pinned as the plan says; Ubuntu for anfra's release, as now). Go 1.27; MinGW-Builds GCC 14.2.0 and DuckDB v1.5.6's `duckdb.dll` (canal); Bun 1.4.4 and pnpm (holistics-core). PowerShell 5.1 and 7 (`install.ps1`), PSScriptAnalyzer. Windows 11 x64 for the checks by hand.

## Commands

```sh
# anfra: lightly (GOMAXPROCS=4 GOFLAGS=-p=2), one command at a time
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go vet ./...
GOMAXPROCS=4 tmp/bin/golangci-lint run ./...                  # once per task, per OS (GOOS=windows too)
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go test -c -o /mnt/c/Users/nuita/wincheck/<pkg>.test.exe ./<pkg>/   # one build per task

# The release build, by hand: sidecars built locally, then the same commands build_release.yml runs
GOOS=windows GOARCH=amd64 go build -tags embed_sidecar -ldflags "-s -w -X …/meta.Version=<v>" -o release/anfra.exe ./cmd/anfra

# install.ps1, layer by layer (decided 2026-11-27), on Windows:
#   Get-Content install.ps1 -Raw | iex                                   # from the file
#   irm http://localhost:8000/install.ps1 | iex                          # through the pipe, served from WSL
#   $env:ANFRA_DOWNLOAD_URL='http://localhost:8000/…'; irm … | iex       # a local release, and a wrong SHA256SUMS
#   Invoke-ScriptAnalyzer install.ps1
# Workflows: actionlint if available, else a YAML parse.
```

## Project Structure

| Where | Change |
|---|---|
| canal `.github/workflows/build_and_release_binaries.yml` | The `windows-x64` row: MinGW, DuckDB's DLL, the DLL build, `--version` |
| holistics-core `.github/workflows/anfra_node_build_release.yml` | The `windows-x64` row, `--version` |
| anfra `.github/workflows/build_release.yml` | Download the Windows sidecars; build and zip `anfra.exe` with the notice |
| anfra `release/windows/THIRD_PARTY_NOTICES.txt` (or where the plan puts it) | DuckDB's MIT license |
| anfra `install.ps1` (new), `install.sh` | The Windows installer; Git Bash points to it |
| anfra `.github/workflows/continuous_integration.yml` | `GOOS=windows go vet`; the required `windows` job |
| anfra `*_test.go` in the packages above | Fixed or skipped on Windows, with the reason |
| anfra `manifest.yml` | The sidecar pins move to releases with Windows builds, at the end |
| anfra `docs/designs/release.md`, `WINDOWS-PLAN.md` | The Windows target, `install.ps1`, the order of merging |

## Code Style

anfra's conventions: [naming](docs/dev/naming.md), [readable prose](docs/designs/readable-prose.md) for every message `install.ps1` prints (each refusal says why and what to do), comments only for why. In each workflow, the Windows row looks like its neighbours: the same step names, `shell: bash` where the others' steps are bash, inputs passed through `env:`, never `${{ }}` inside `run:`. `install.ps1` mirrors `install.sh` section by section, so the two are read and changed together.

## Testing Strategy

- **The sidecars' Windows rows:** run first by hand on Windows with the same commands (canal's DLL build, Bun's build), then on GitHub as a test build (`workflow_dispatch`, no release), with the user's approval to push a branch.
- **anfra's release:** the build commands run by hand with those sidecars, giving an `anfra-windows-x64.zip`. On Windows 11 it unzips, `anfra version` runs, a DuckDB query and a MotherDuck connection work (MotherDuck only with a token the user sets). `anfra update` from the win-self-update test build takes it.
- **`install.ps1`:** the plan's layers 1–3 and 6 (from the file, through the pipe, from a local release including a wrong `SHA256SUMS`, the script analyzer), in PowerShell 5.1, and in PowerShell 7 if you install it (`winget install Microsoft.PowerShell`; otherwise on GitHub's Windows runner, which has both). Layer 5, a clean machine: this machine is Windows 11 Home, without Windows Sandbox, so a fresh Windows user account stands in. Layer 4, the pushed branch's raw URL, only once the branch is pushed.
- **The Windows CI job:** the package list run on Windows here first (lightly, with the update notifier off), so the job is green before it's required.
- **What can only run on GitHub** (a real release of each repo, the required job on a PR) is listed for the user, as the order of merging below.

## Order of merging (for the PRs, when the user opens them)

1. holistics-core: `SIGBREAK` + the Windows row → an anfra-node release.
2. canal: the Windows row → an anfra-canal release.
3. anfra: the earlier modules' branch, this module's release and CI changes, and `manifest.yml` pinned to those two releases → the first release with `anfra-windows-x64.zip`.
4. anfra: `install.ps1` reaches `main` **only after** step 3's release is out: users fetch it from `main`, so earlier it would download a file that doesn't exist.

## Boundaries

- **Always:**
  - keep each Windows row like its neighbours;
  - pass workflow inputs through `env:`;
  - pin new actions by commit, as each repo does;
  - verify the checksum before installing;
  - never `exit` in `install.ps1`;
  - run Go lightly, and check Windows' memory before running there;
  - work from canal's `origin/master`.
- **Ask first:**
  - pushing any branch, in any of the three repos;
  - running a workflow on GitHub;
  - cutting a release or a tag;
  - changing what an existing Linux or macOS row does;
  - a new action, runner or secret;
  - adding the required check to branch protection (a repo setting).
- **Never:**
  - publish a release from a test;
  - install without a matching checksum;
  - change the Linux and macOS asset names;
  - print or store a token (MotherDuck's included).

## Success Criteria

1. canal's and holistics-core's release workflows have a `windows-x64` row like their others; each builds a Windows binary that answers `--version` (canal's beside its `duckdb.dll`), checked on Windows by hand, and on GitHub as a test build if the user approves the push.
2. anfra's `build_release.yml` builds `anfra-windows-x64.zip` (`anfra.exe` and `THIRD_PARTY_NOTICES.txt` at its top), covered by `SHA256SUMS`; it fails, naming the file, when a pinned release lacks a Windows sidecar. Its commands, run by hand with locally built sidecars, give a `.zip` whose `anfra.exe` runs queries on Windows 11, MotherDuck's extension included.
3. `install.ps1`, from a local release, installs into `%USERPROFILE%\.anfra\bin`, adds it to the user PATH, and `anfra version` runs in a new terminal; a wrong `SHA256SUMS` refuses and installs nothing; the window stays open on every failure; it passes PSScriptAnalyzer, in PowerShell 5.1 and 7. On ARM it installs x64 with the note (checked by reading the code, without an ARM PC).
4. `install.sh` on Git Bash points to `install.ps1`.
5. anfra's PR checks run `GOOS=windows go vet ./...` and a required `windows` job whose package list passes on Windows.
6. `docs/designs/release.md` and `WINDOWS-PLAN.md` say how Windows is built and installed, and the order of merging.

## Open Questions

None. Decided with the user (2026-11-27):
- **The minimum-OS check stays out of win-release.** It covers every OS, and Linux and macOS have no stated minimum yet; it becomes its own small piece once they do. The Windows versions are decided in `win-skills-docs`.
- **No Windows Sandbox** (this machine is Windows 11 Home): a fresh Windows user account is the clean machine.
