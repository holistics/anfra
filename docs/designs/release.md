# Releases

A release is one `anfra` binary per platform, with its sidecars embedded as two binaries, anfra-node and anfra-canal ([sidecars.md](sidecars.md)) and the Data App frontend built in, plus a Docker image of the Linux ones and the SDK on npm. Everything about a release follows from one file.

## `manifest.yml`, the single source

`manifest.yml` holds the release's `version`, the sidecar releases it embeds (`sidecars:`, a holistics-core release tag for anfra-node and a canal release tag for anfra-canal, with the canal-query and canal-lake releases it contains in `anfra_canal_services`, for deployments that run them as services of their own), and the cloud servers its `anfra auth login` offers (`cloud_servers:` and `cloud_support_email:`, which `scripts/cloud-url` checks and the build embeds; see [DEVELOPMENT.md](../../DEVELOPMENT.md#cloud-login-url)). The version names the git tag (`v<version>`, which is also the Go module version of `github.com/holistics/anfra`; releases up to 0.8.0 were tagged `anfra-v<version>`, which Go does not read as a version), is compiled into the binary, and names the image's tags, so the three always agree.

To cut a release, on a branch:

1. `pnpm bump <version>` (`scripts/bump.sh`), or `pnpm bump next` for the version the commits call for (below), sets `version`, and the SDK's (`web/sdk/package.json`), and prepends the changes since the last `v*` tag to `CHANGELOG.md`, from conventional commits (`conventional-changelog.config.mjs` says which commit types appear).
2. Update the sidecar pins too, if this release takes new ones. A pin change needs a version bump, or no new tag is cut.
3. Merge to `main`.

### The release pull request

`release_pr.yml` does steps 1 and 3's pull request for you, Monday to Saturday at 05:00 UTC: it runs `pnpm bump` on a branch fresh from `main`, commits `release: <version>`, and opens the pull request. Merging it is the release. The workflow only sets up and calls `scripts/release-pr.sh`, which `--dry-run` runs locally: everything but the push and the pull request, which it prints.

- **The version** comes from the conventional commits since the last tag (`scripts/next-version.mjs`), read as the changelog reads them: conventional-recommended-bump with the changelog's preset and types (`conventional-changelog.config.mjs`). A breaking change bumps the major, or the minor while below 1.0; a `feat` the minor; any other type the changelog shows (`fix`, `perf`, `build`, `security`) the patch. A type the changelog hides is not releasable; with only those since the last tag, there is nothing to release. After a prerelease, it is the release that prerelease leads to. `pnpm next-version` prints it; `pnpm bump next` bumps to it.
- **One pull request, always current.** It lives on one branch, `release/next`, which each run rebuilds from `main` and force-pushes, closing the open pull request and opening a new one: GitHub notifies no one of an edit to a pull request, but a new one notifies its watchers. A run that finds `release/next` holding this release of the current `main` already changes nothing. So there is never more than one open, and it is never more than a day behind `main`. When there is nothing to release (one was merged since), the run closes it.
- **By hand,** from the Actions tab or `gh workflow run release_pr.yml`, with an optional `version` to release instead of the computed one, and an optional `branch`.
- **A release that needs commits of its own,** sidecar pins say: run it with a new `branch` (`-f branch=release/0.19.0`), which must not exist yet, and push them there. `release/next` is the workflow's own: commits pushed onto it are lost at the next run.
- **CI does not start on its own** on the pull request, for now: the workflow opens it with its own `GITHUB_TOKEN`, and GitHub starts no workflow for what that token does. Approve its workflows, with the button GitHub shows on the pull request, to run CI; again after each run replaces it. A GitHub App's token would fix it; the workflow's header has the TODO.

`tag_and_release.yml` runs on a push to `main` that changes `manifest.yml`. If the version has no tag yet, it creates `v<version>` and pushes it with a deploy key (`ANFRA_DEPLOY_KEY`), and the push triggers `build_release.yml`. The deploy key is needed twice over: the repository's tag ruleset blocks creating release tags, and the workflow's own token cannot bypass it, while a deploy key can; and a tag pushed with the workflow's own token would not trigger another workflow. `build_release.yml` also runs on a tag pushed by hand, and on `workflow_dispatch`: dispatched on a tag (`gh workflow run build_release.yml --ref v<version>`), it releases that tag again; dispatched on a branch, it is a test build, which uploads artifacts and publishes nothing.

## The build

`build_release.yml`, in order:

1. **The frontend,** once: `pnpm build:web`, into `internal/appserve/dist`, which every target embeds. It fails without `dist/index.html`: a binary without its pages is a broken release.
2. **Checks:** the tag matches the manifest's version, so does the SDK's, and `CHANGELOG.md` has a section for it. Both fail before anything is built.
3. **The sidecars:** the pinned releases' binaries, downloaded from their repositories (a cross-repository read token, `ANFRA_DIST_TOKEN`). Each sidecar repo builds every platform natively, one runner per platform, and publishes `<name>-<os>-<arch>`, with `.exe` on Windows. canal's Windows build also publishes `anfra-canal-windows-x64-duckdb.dll`: it links DuckDB's own `duckdb.dll`, the only DuckDB that loads MotherDuck's extension on Windows, and needs it beside it ([windows-canal-build.md](../research/windows-canal-build.md)). The release fails, naming the missing files, when a pinned release lacks any platform's: a pin must move to releases that have them all.
4. **Every target,** cross-built on one runner (anfra is pure Go): each target's sidecars copied into the embed assets, `go build -tags embed_sidecar` with the version injected, then gzipped. Only the `.gz` is published, as a transport compression: the sidecars embedded in the binary stay uncompressed, so the binary itself stays delta-friendly. Windows' asset is a `.zip` instead, which Windows opens with a double-click: `anfra-windows-x64.zip`, holding `anfra.exe` and `THIRD_PARTY_NOTICES.txt` at its top, where `anfra update` and `install.ps1` look. `anfra.exe` embeds `duckdb.dll` beside anfra-canal, and the notice (`packaging/windows/THIRD_PARTY_NOTICES.txt`) carries DuckDB's MIT license. Notices for every platform are still to come ([backlog](../todos/backlog.md#third-party-license-notices-in-releases)).
5. **`SHA256SUMS`:** the sha256 of each asset, by file name, in `sha256sum`'s format, which `anfra update` checks a download against (below).
6. **The image,** below.
7. **The GitHub Release,** created (or, on a re-run, edited) with the binaries. `SHA256SUMS` is uploaded first, in a call of its own: `anfra update` takes a release as out once its asset is, and must find the checksum by then.
8. **The SDK,** once the release is out: below.

**Release notes** are the version's section of `CHANGELOG.md`, under "Changelog", followed by GitHub's own generated notes: the pull requests, new contributors and compare link (`scripts/release-notes.sh`, which also previews them locally). So a release says what the changelog says, and a re-run brings its notes back in line.

## Installing

Users install a release with a script fetched from `main`, not from the release: `https://anfra.ai/install.sh` and `https://anfra.ai/install.ps1` redirect to the repo's raw files. So a change to an installer is live the moment it reaches `main`.

- **`install.sh`** (Linux, macOS): `curl -fsSL https://anfra.ai/install.sh | bash`. In Git Bash, MSYS2 or Cygwin it points to `install.ps1`. It doesn't check `SHA256SUMS` yet.
- **`install.ps1`** (Windows): `irm https://anfra.ai/install.ps1 | iex`, the only way the docs give, since PowerShell's default execution policy refuses to run a downloaded `.ps1` file. It mirrors `install.sh` section by section: the same overrides (`ANFRA_HOME`, `ANFRA_INSTALL_DIR`, `ANFRA_VERSION`, `ANFRA_NO_MODIFY_PATH`) plus `ANFRA_DOWNLOAD_URL`, a folder to download from instead of the release, for testing. It installs only a `.zip` matching the release's `SHA256SUMS`, which stands in for signing while the Windows binaries are unsigned. It adds the install folder to the user PATH as the registry stores it, keeping the PATH's other entries and its type. Run through `iex`, it runs in the user's own session, so it never calls `exit`, which would close their window, and keeps to ASCII, which Windows PowerShell 5.1 and PowerShell 7 read alike. On ARM it installs the x64 build, which Windows runs emulated.

## Windows in the pull request checks

Nobody develops anfra on Windows, so `continuous_integration.yml` guards it: `GOOS=windows go vet ./...` in the `test` job, and a `windows` job that runs `go test` on a Windows runner for the packages whose code differs there (the sidecars' processes, logins, publish and pull's files, exports, anfra's folder, update). `ci_passed` requires it. Tests that need what Windows withholds from a normal user, such as symlinks, skip there and say why.

## The first release with Windows

Windows needs releases from all three repos, merged in this order:

1. holistics-core: anfra-node's Windows row, and its shutdown on Ctrl-Break, then an anfra-node release.
2. canal: its Windows row, then an anfra-canal release.
3. anfra: `manifest.yml` pinned to those two releases, then a release, the first with `anfra-windows-x64.zip`.
4. `install.ps1` on `main` only after that release is out, because users fetch it from `main` and it would download a file that doesn't exist yet. `https://anfra.ai/install.ps1` needs its Cloudflare redirect too, like `install.sh`'s.

## The Docker image

`ghcr.io/holistics/anfra`, built by `build_release.yml` from the `Dockerfile` at the root; how to run it is in [usage/docker.md](../usage/docker.md).

- **The release's own binaries.** The Linux binaries built above are laid out by platform as the build context (`image/<os>/<arch>/anfra`), so the image runs exactly what the release downloads.
- **One name for every platform.** Built for linux/amd64 and linux/arm64 at once, each tag is a multi-platform index; Docker pulls the build matching the machine. Tags are the version, its major.minor, and `latest`. A release pushes them; any other build only builds the image, so a broken Dockerfile shows before a release does.
- **No emulation.** Everything that needs a shell (CA certificates, the user, tini) is prepared in a stage on the builder's own platform; the final stage only copies files. So building for another architecture needs no QEMU or Rosetta.
- **Debian trixie,** because the sidecars anfra unpacks need its glibc.
- **tini as PID 1** (Debian's static build, for the target architecture). It reaps orphans, forwards signals, and keeps anfra from being PID 1, which a sidecar could read as anfra having died ([sidecars.md](sidecars.md)).
- **A named user,** `anfra`, UID 1000. The sidecars look their user up, and fail without a passwd entry, so a bare UID will not do.
- **Its home already holds `~/.anfra` and `~/.cache`,** owned by `anfra`. An image built on this one may run root steps whose tools write to `$HOME/.cache` (Rosetta does, when building amd64 on Apple silicon); with those folders already anfra's, such a step only adds its own folder inside them, rather than leaving them root's and anfra unable to start.
- **`/repo`,** the working directory, owned by `anfra`: where a repo is mounted or copied.

## The SDK on npm

`@holistics/anfra-sdk` (`web/sdk`), published by `build_release.yml`'s `sdk` job; what it holds is in [data-apps.md](data-apps.md).

- **The release's version.** The SDK's API client is generated from this release's `api/openapi.yaml`, so the two go out together, at one version, even when the SDK did not change. `pnpm bump` sets both; CI fails a pull request in which they differ, and a release refuses to go out.
- **Published once.** A re-run skips a version npm already has. A prerelease version (`1.2.0-rc.1`) is published under the `next` dist-tag, so `npm install` never picks it.
- **Trusted publishing.** npm takes the publish from the workflow over OIDC, with no token, and records the package's provenance. It is configured on the package's settings on npmjs.com, naming this repository and a workflow. npm checks the workflow that started the run: `build_release.yml`, which a tag push starts. There is no npm token, and the package's settings disallow tokens, so only this workflow, or a maintainer with two-factor authentication, can publish.
- **Any other build** packs it without publishing, so a broken package shows before a release does.

## The skills

The anfra skills' source is `skills/`; `build_release.yml`'s `skills` job publishes them to [holistics/anfra-skills](https://github.com/holistics/anfra-skills), the facade every agent installs from, after the binaries ([agent setup](agent-setup.md#releasing-the-skills)).

- **The release's version, only when they changed.** The job rebuilds what the facade holds, with the parameters it recorded (`.build.json`), and compares. The same tree publishes nothing; a different one is built at this release's version, committed, tagged `anfra-v<version>`, and pushed (`skills/scripts/publish.sh`). A re-run publishes nothing again.
- **Their changelog,** `skills/CHANGELOG.md`, gets an entry from `pnpm bump` when a commit since the last release changed `skills/`, and is published with them.
- **The build writes the versions.** The manifests' versions and each skill's stamp come from the build (`skills/scripts/build.mjs`), not from `skills/`, so `pnpm bump` doesn't touch the skills.
- **A GitHub App** with Contents: write on the facade, allowed past its branch protection, does the push, with a token the job makes from `ANFRA_SKILLS_DEPLOYER_APP_ID` and `ANFRA_SKILLS_DEPLOYER_KEY`.
- **Any other build** reports what a release would publish, without pushing.

## Versions

A release's version is `manifest.yml`'s, injected at build time (`internal/meta.Version`). Any other build is one from source, and says which: the release it comes after, then, as semver build metadata, the commit and whether the tree was dirty, as Go records them in the binary: `<base>+<commit>`, or `<base>+<commit>.dirty` (`internal/meta/version.go`). `make build` and `make dev` inject the base from `manifest.yml`; a bare `go build` reports `0.0.0+<commit>`.

Build metadata leaves the version's order alone, and a release never carries any, so its presence alone marks a build from source (`meta.FromSource`). That is what the updater relies on.

## Updates

`anfra update` replaces the running binary with the latest release, and anfra notices newer ones in the background: a cached check at most hourly, 10 minutes apart after one fails, a notice on an interactive terminal, an announcement from `serve` (`internal/update`, `cmd/anfra/update.go`). `ANFRA_NO_UPDATE_NOTIFIER` turns the notices off; `ANFRA_AUTO_UPDATE` applies updates in the background instead.

A running `anfra serve` keeps the version it started with, and so do the commands forwarded to it, until it is restarted. The CLI warns of one with a `server_version_mismatch` warning: a command forwarded to it before its answer, `anfra version`, which is never forwarded and reports the CLI's own version, and `anfra update` at its end, for each server still running another version ([cli-vs-warm-server.md](cli-vs-warm-server.md#version-mismatch)). An MCP client, which talks to the server alone, gets no such warning.

After an update, or before one with `anfra update check`, `anfra update` prints what the new version brings: the changelog of each release since the running one, the skipped ones included, read from `CHANGELOG.md` at the new version's tag, of which each release's notes are a section, rather than from GitHub's rate-limited API (`internal/update/notes.go`). It shows the newest three and links to the rest. The notes are best-effort: when they can't be fetched, it prints a link to the releases instead, and the update still succeeds.

`anfra update` downloads the asset to a temporary file, and installs it only when its sha256 matches its line in the release's `SHA256SUMS`. A mismatch, or a release without `SHA256SUMS` or without the asset's line, refuses the update and changes nothing; the message says how to install by hand (`install.sh`, or `install.ps1` on Windows). The temporary file is removed whatever happens. On Windows, `minio/selfupdate` renames the running `anfra.exe` to `.anfra.exe.old`, which Windows allows; the next update removes it, and while an older anfra still runs from it (an `anfra serve`, say), moves the running file aside under a name of its own instead.

None of this applies to a build from source: it was built on purpose, and a release would replace it with something else. It is never told of updates, and `anfra update` refuses to replace it.
