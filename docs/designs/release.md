# Releases

A release is one `anfra` binary per platform, with both sidecars embedded ([sidecars.md](sidecars.md)) and the Data App frontend built in, plus a Docker image of the Linux ones and the SDK on npm. Everything about a release follows from one file.

## `manifest.yml`, the single source

`manifest.yml` holds the release's `version` and the sidecar releases it embeds (`sidecars:`, a holistics-core release tag for anfra-node and a canal release tag for canal-query). The version names the git tag (`anfra-v<version>`), is compiled into the binary, and names the image's tags, so the three always agree.

To cut a release, on a branch:

1. `pnpm bump <version>` (`scripts/bump.sh`) sets `version`, and the SDK's (`web/sdk/package.json`), and prepends the changes since the last `anfra-v*` tag to `CHANGELOG.md`, from conventional commits (`conventional-changelog.config.mjs` says which commit types appear).
2. Update the sidecar pins too, if this release takes new ones. A pin change needs a version bump, or no new tag is cut.
3. Merge to `main`.

`tag_and_release.yml` runs on a push to `main` that changes `manifest.yml`. If the version has no tag yet, it creates `anfra-v<version>` and calls `build_release.yml` in the same run. A tag pushed with the workflow's own token would not trigger another workflow, so it calls it directly. `build_release.yml` also runs on a tag pushed by hand, and on `workflow_dispatch` for a test build, which uploads artifacts and publishes nothing.

## The build

`build_release.yml`, in order:

1. **The frontend,** once: `pnpm build:web`, into `internal/appserve/dist`, which every target embeds. It fails without `dist/index.html`: a binary without its pages is a broken release.
2. **Checks:** the tag matches the manifest's version, so does the SDK's, and `CHANGELOG.md` has a section for it. Both fail before anything is built.
3. **The sidecars:** the pinned releases' binaries, downloaded from their repositories (a cross-repository read token, `ANFRA_DIST_TOKEN`).
4. **Every target,** cross-built on one runner (anfra is pure Go): each target's sidecars copied into the embed assets, `go build -tags embed_sidecar` with the version injected, then gzipped. Only the `.gz` is published, as a transport compression: the sidecars embedded in the binary stay uncompressed, so the binary itself stays delta-friendly.
5. **The image,** below.
6. **The GitHub Release,** created (or, on a re-run, edited) with the binaries.
7. **The SDK,** once the release is out: below.

**Release notes** are the version's section of `CHANGELOG.md`, under "Changelog", followed by GitHub's own generated notes: the pull requests, new contributors and compare link (`scripts/release-notes.sh`, which also previews them locally). So a release says what the changelog says, and a re-run brings its notes back in line.

## The Docker image

`ghcr.io/holistics/anfra`, built by `build_release.yml` from the `Dockerfile` at the root; how to run it is in [usage/docker.md](../usage/docker.md).

- **The release's own binaries.** The Linux binaries built above are laid out by platform as the build context (`image/<os>/<arch>/anfra`), so the image runs exactly what the release downloads.
- **One name for every platform.** Built for linux/amd64 and linux/arm64 at once, each tag is a multi-platform index; Docker pulls the build matching the machine. Tags are the version, its major.minor, and `latest`. A release pushes them; any other build only builds the image, so a broken Dockerfile shows before a release does.
- **No emulation.** Everything that needs a shell (CA certificates, the user, tini) is prepared in a stage on the builder's own platform; the final stage only copies files. So building for another architecture needs no QEMU or Rosetta.
- **Debian trixie,** because the sidecars anfra unpacks need its glibc.
- **tini as PID 1** (Debian's static build, for the target architecture). It reaps orphans, forwards signals, and keeps anfra from being PID 1, which a sidecar could read as its host having died ([sidecars.md](sidecars.md)).
- **A named user,** `anfra`, UID 1000. The sidecars look their user up, and fail without a passwd entry, so a bare UID will not do.
- **Its home already holds `~/.anfra` and `~/.cache`,** owned by `anfra`. An image built on this one may run root steps whose tools write to `$HOME/.cache` (Rosetta does, when building amd64 on Apple silicon); with those folders already anfra's, such a step only adds its own folder inside them, rather than leaving them root's and anfra unable to start.
- **`/repo`,** the working directory, owned by `anfra`: where a repo is mounted or copied.

## The SDK on npm

`@holistics/anfra-sdk` (`web/sdk`), published by `build_release.yml`'s `sdk` job; what it holds is in [data-apps.md](data-apps.md).

- **The release's version.** The SDK's API client is generated from this release's `api/openapi.yaml`, so the two go out together, at one version, even when the SDK did not change. `pnpm bump` sets both; CI fails a pull request in which they differ, and a release refuses to go out.
- **Published once.** A re-run skips a version npm already has. A prerelease version (`1.2.0-rc.1`) is published under the `next` dist-tag, so `npm install` never picks it.
- **Trusted publishing.** npm takes the publish from the workflow over OIDC, with no token, and records the package's provenance. It is configured on the package's settings on npmjs.com, naming this repository and a workflow. npm checks the workflow that started the run, not the one that publishes, so both are trusted: `tag_and_release.yml`, which calls `build_release.yml` for a release, and `build_release.yml`, which runs on its own for a tag pushed by hand. There is no npm token, and the package's settings disallow tokens, so only these workflows, or a maintainer with two-factor authentication, can publish.
- **Any other build** packs it without publishing, so a broken package shows before a release does.

## Versions

A release's version is `manifest.yml`'s, injected at build time (`internal/meta.Version`). Any other build is one from source, and says which: the release it comes after, then, as semver build metadata, the commit and whether the tree was dirty, as Go records them in the binary: `<base>+<commit>`, or `<base>+<commit>.dirty` (`internal/meta/version.go`). `make build` and `make dev` inject the base from `manifest.yml`; a bare `go build` reports `0.0.0+<commit>`.

Build metadata leaves the version's order alone, and a release never carries any, so its presence alone marks a build from source (`meta.FromSource`). That is what the updater relies on.

## Updates

`anfra update` replaces the running binary with the latest release, and anfra notices newer ones in the background: a cached check at most daily, a notice on an interactive terminal, an announcement from `serve` (`internal/update`, `cmd/anfra/update.go`). `ANFRA_NO_UPDATE_NOTIFIER` turns the notices off; `ANFRA_AUTO_UPDATE` applies updates in the background instead.

None of this applies to a build from source: it was built on purpose, and a release would replace it with something else. It is never told of updates, and `anfra update` refuses to replace it.
