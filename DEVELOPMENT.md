# Developing anfra

For working on anfra itself. How it's designed is in [docs/designs](docs/designs/); start with the [philosophy](docs/designs/philosophy.md) and the [architecture](docs/designs/architecture.md). Conventions for writing its code are in [docs/dev](docs/dev/), such as [method naming](docs/dev/method-naming.md) and [error mapping](docs/dev/error-mapping.md).

## Prerequisites

- Go, at the version in `go.mod`.
- Node, at the version in `.nvmrc`, and pnpm, at the version in `package.json`'s `packageManager` (`corepack enable` picks it up).
- tmux, only for `make dev-tmux`.

Then, once:

```sh
make setup
```

It installs the web workspace (`web/`) and the git hooks that check commit messages, writes `.env.local` from `.env.local.example` if there is none (it never overwrites one), and downloads the sidecars, below. Fill `.env.local` in: `ANFRA_DEV_REPO`, the repo `make dev` serves. The `Makefile` reads it and exports it to everything it runs.

The `Makefile`'s other tools (air, hivemind, overmind, golangci-lint) are pinned in `tools/go.mod` and run with `go tool`: nothing to install.

### Sidecars

anfra built from source embeds no sidecars, so it needs them from elsewhere. `manifest.yml` pins the release of each that anfra's own releases embed, and that is the one to develop and test against: `scripts/sidecars.sh` downloads it for your machine into `.sidecars/` (git-ignored), one copy of each. anfra-node's is its unminified build, so its spans carry the source's function names (`_compile`, not `OA8`); a release from before anfra-node published one gives the minified build, which anfra's releases embed.

```sh
make sidecars   # download the pinned releases, replacing the ones there; after a pin moves
```

It needs [gh](https://cli.github.com), logged in (`gh auth login`) as an account that can read holistics/holistics-core and holistics/canal, whose releases are private.

`make dev`, `make test-sidecars` and `make build` run `scripts/sidecars.sh check` first: it downloads a sidecar `.sidecars/` lacks, and fails when one is not the pinned version (each says its version with `--version`), with how to fix it. It never replaces one on its own: `make sidecars` does.

To run your own build of a sidecar instead, set, in `.env.local`, an absolute path to it. The check holds it to the pin too; `ANFRA_SIDECAR_SKIP_VERSION_CHECK=1` runs one whose version is not the pin on purpose.

- `ANFRA_NODE_BIN`: anfra-node. In anfra-node's repo, from its folder: `bash scripts/build-sidecar.sh <out path>`.
- `ANFRA_CANAL_QUERY_BIN`: canal-query. In canal-query's repo, from its root: `CGO_ENABLED=1 go build -tags=duckdb_arrow -o <out path> ./app/query`. It needs cgo, for DuckDB, so it builds for the machine it runs on only.

Or set `ANFRA_NODE_URL` / `ANFRA_CANAL_QUERY_URL` to dial sidecars already running, so a rebuild of anfra doesn't respawn them; the check leaves those alone. See [sidecars](docs/designs/sidecars.md).

## Running it

```sh
make dev        # the Go server, the SDK and the frontend, each rebuilt on change, in one terminal
make dev-tmux   # the same under overmind, in tmux: restart or attach to each process on its own
```

`make dev` runs `anfra serve` in the repo `ANFRA_DEV_REPO` names (in `.env.local`), rebuilt by air on every Go change. Its pages redirect to the frontend's Vite dev server (`ANFRA_APPSERVE_DEV_URL`), which reloads on every edit. `make` lists every target.

To run a command against a repo yourself, build and call it from the repo's folder:

```sh
make build                           # bin/anfra, with the frontend and the sidecars built in, as a release is
cd <repo> && <path to>/bin/anfra show
```

## Checks

What CI runs (`.github/workflows/continuous_integration.yml`), in two commands:

```sh
make check      # lint, type-check, and check the API contract is fresh: no tests, no build
make test       # the Go tests, then the web workspace's
```

`make check` runs golangci-lint at the version `tools/go.mod` pins, which must match the one CI's workflow pins; `go mod tidy --diff`; `scripts/openapi.sh diff`; the SDK's generated types against the spec; and both web packages' typechecks. It builds the SDK's type declarations, which the frontend's typecheck reads, and nothing else. CI also builds the frontend (`pnpm build:web`) and checks that a breaking change to the API is declared by a commit.

Some Go tests run commands against real sidecars (`internal/app/sidecars_test.go`), and the dev warehouse (`docker compose -f docker-compose.dev.yml up -d postgres`). They skip themselves unless `ANFRA_NODE_BIN` and `ANFRA_CANAL_QUERY_BIN` are both set, and `make test` then says so. `make test-sidecars` runs the Go tests with them, against the pinned sidecars or your own; `make test` runs them too when `.env.local` names both, after checking their versions.

### When the API changes

An op's input or answer is part of the core API's contract:

```sh
scripts/openapi.sh generate            # api/openapi.yaml, from the commands
pnpm --filter @holistics/anfra-sdk generate       # the SDK's types (web/sdk/src/api/schema.d.ts), from it
```

Commit both. CI refuses a stale one, and a breaking change that no commit declares (`feat!:`, or a `BREAKING CHANGE:` footer). See [commands and API](docs/designs/commands-and-api.md).

## Commits and releases

Commit messages are checked by commitlint, in a git hook and in CI: the rules, types and scopes are in `commitlint.config.mjs`. `pnpm commit` writes one interactively.

A release is a version bump merged to main: `pnpm bump <version>` updates `manifest.yml`, the SDK's version and `CHANGELOG.md`, and the release workflows tag, build and publish: the binaries, the image, and the SDK on npm. See [release](docs/designs/release.md).

The Docker image is built from release binaries; the `Dockerfile`'s header says how to build it locally.

### Cloud login URL

Optionally set `ANFRA_CLOUD_URL` when running `make build` or `make build-dev` to embed a
default cloud URL. Release CI uses the optional repository Actions variable of the same
name. Builds validate supplied values, but do not require one.

At runtime, `ANFRA_CLOUD_URL` overrides the built-in default. An explicitly empty value
disables it. Bare `anfra auth login` shows the effective URL alongside organization URL
entry; with neither setting, it goes straight to URL entry. The binary does not load
`.env.local`; set the runtime override in the shell or process environment.
Without terminal stdin and stderr, supply an explicit organization URL.

The value must be an absolute HTTPS origin, or HTTP on literal `127.0.0.1` or `[::1]` for
local testing, without userinfo, query, fragment or a path other than `/`. Login lowercases
the scheme and hostname and removes the default port and final slash.

For a local cloud server, run `ANFRA_CLOUD_URL=http://127.0.0.1:5173 bin/anfra auth login`.
Use the cloud's public origin (Vite in local development), not `localhost` for HTTP.

## Debugging

- **Logs:** each repo's go to `<ANFRA_HOME>/repos/<repo id>/logs/anfra.log` (`~/.anfra` by default), anfra's records and the sidecars' stderr together. `anfra serve` also writes them to stderr, and `ANFRA_LOG_STDERR` changes that for any command (`1` on, `0` off); `LOG_LEVEL` sets the level.
- **Traces:** set `OTEL_EXPORTER_OTLP_ENDPOINT` to a collector's OTLP/HTTP receiver. One command or request becomes one trace, through to anfra-node.
- **Versions:** a build from source reports `<release>+<commit>[.dirty]` (`anfra version`), so a log or trace says which source it came from.

See [configs](docs/usage/configs.md) for every setting.

## Known pitfalls

- **"anfra serve is already running for this repo"** under `make dev`: air can leave a server running when changes arrive while it rebuilds, and the next one then refuses to start. Stop the stray `anfra serve` (it is still listed by `anfra status` in that repo).
- **A build with `-tags embed_sidecar`** embeds whatever is in `internal/sidecar/*/assets/` (git-ignored), which can be stale. Only releases need that tag.
- **Images built on anfra's:** a root `RUN` step that writes to `$HOME` leaves root's files in anfra's home. See [docker](docs/usage/docker.md).
