# Developing anfra

For working on anfra itself. How it's designed is in [docs/designs](docs/designs/); start with the [philosophy](docs/designs/philosophy.md) and the [architecture](docs/designs/architecture.md).

## Prerequisites

- Go, at the version in `go.mod`.
- Node, at the version in `.nvmrc`, and pnpm, at the version in `package.json`'s `packageManager` (`corepack enable` picks it up).
- tmux, only for `make dev-tmux`.

Then, once:

```sh
make setup
```

It installs the web workspace (`web/`) and the git hooks that check commit messages, and writes `.env.local` from `.env.local.example` if there is none (it never overwrites one). Fill it in: `ANFRA_DEV_REPO`, the repo `make dev` serves, and the sidecars, below. The `Makefile` reads it and exports it to everything it runs.

The `Makefile`'s other tools (air, hivemind, overmind, golangci-lint) are pinned in `tools/go.mod` and run with `go tool`: nothing to install.

### Sidecars

anfra built from source embeds no sidecars, so it needs them from elsewhere. Build each from its own repo, or take the binary from one of its releases, and set, in `.env.local`, absolute paths to them:

- `ANFRA_NODE_BIN`: anfra-node. In anfra-node's repo, from its folder: `bash scripts/build-sidecar.sh <out path>`.
- `ANFRA_CANAL_QUERY_BIN`: canal-query. In canal-query's repo, from its root: `CGO_ENABLED=1 go build -tags=duckdb_arrow -o <out path> ./app/query`. It needs cgo, for DuckDB, so it builds for the machine it runs on only.

`manifest.yml` names the release of each that anfra's own releases embed. Or set `ANFRA_NODE_URL` / `ANFRA_CANAL_QUERY_URL` to dial sidecars already running, so a rebuild of anfra doesn't respawn them. See [sidecars](docs/designs/sidecars.md).

## Running it

```sh
make dev        # the Go server, the SDK and the frontend, each rebuilt on change, in one terminal
make dev-tmux   # the same under overmind, in tmux: restart or attach to each process on its own
```

`make dev` runs `anfra serve` in the repo `ANFRA_DEV_REPO` names (in `.env.local`), rebuilt by air on every Go change. Its pages redirect to the frontend's Vite dev server (`ANFRA_APPSERVE_DEV_URL`), which reloads on every edit. `make` lists every target.

To run a command against a repo yourself, build and call it from the repo's folder:

```sh
make build                           # bin/anfra, with the frontend built in, as a release is
cd <repo> && <path to>/bin/anfra show
```

## Checks

What CI runs (`.github/workflows/continuous_integration.yml`), in two commands:

```sh
make check      # lint, type-check, and check the API contract is fresh: no tests, no build
make test       # the Go tests, then the web workspace's
```

`make check` runs golangci-lint at the version `tools/go.mod` pins, which must match the one CI's workflow pins; `go mod tidy --diff`; `scripts/openapi.sh diff`; the SDK's generated types against the spec; and both web packages' typechecks. It builds the SDK's type declarations, which the frontend's typecheck reads, and nothing else. CI also builds the frontend (`pnpm build:web`) and checks that a breaking change to the API is declared by a commit.

Some Go tests run commands against real sidecars, and skip themselves unless `ANFRA_NODE_BIN` and `ANFRA_CANAL_QUERY_BIN` are set (`internal/app/sidecars_test.go`).

### When the API changes

An op's input or answer is part of the core API's contract:

```sh
scripts/openapi.sh generate            # api/openapi.yaml, from the commands
pnpm --filter anfra-sdk generate       # the SDK's types (web/sdk/src/api/schema.d.ts), from it
```

Commit both. CI refuses a stale one, and a breaking change that no commit declares (`feat!:`, or a `BREAKING CHANGE:` footer). See [commands and API](docs/designs/commands-and-api.md).

## Commits and releases

Commit messages are checked by commitlint, in a git hook and in CI: the rules, types and scopes are in `commitlint.config.mjs`. `pnpm commit` writes one interactively.

A release is a version bump merged to main: `pnpm bump <version>` updates `manifest.yml` and `CHANGELOG.md`, and the release workflows tag, build and publish. See [release](docs/designs/release.md).

The Docker image is built from release binaries; the `Dockerfile`'s header says how to build it locally.

## Debugging

- **Logs:** each repo's go to `<ANFRA_HOME>/repos/<repo id>/logs/anfra.log` (`~/.anfra` by default), anfra's records and the sidecars' stderr together. `ANFRA_LOG_STDERR=1` also writes them to stderr; `LOG_LEVEL` sets the level.
- **Traces:** set `OTEL_EXPORTER_OTLP_ENDPOINT` to a collector's OTLP/HTTP receiver. One command or request becomes one trace, through to anfra-node.
- **Versions:** a build from source reports `<release>+<commit>[.dirty]` (`anfra version`), so a log or trace says which source it came from.

See [configs](docs/usage/configs.md) for every setting.

## Known pitfalls

- **"anfra serve is already running for this repo"** under `make dev`: air can leave a server running when changes arrive while it rebuilds, and the next one then refuses to start. Stop the stray `anfra serve` (it is still listed by `anfra status` in that repo).
- **A build with `-tags embed_sidecar`** embeds whatever is in `internal/sidecar/*/assets/` (git-ignored), which can be stale. Only releases need that tag.
- **Images built on anfra's:** a root `RUN` step that writes to `$HOME` leaves root's files in anfra's home. See [docker](docs/usage/docker.md).
