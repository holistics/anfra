# anfra

Local-first agentic analytics infrastructure — a single binary that runs an
AML/AQL engine and query layer against your data warehouse.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/holistics/anfra/main/install.sh | bash
```

The installer downloads the latest release for your platform, places the `anfra`
binary in `~/.anfra/bin`, and prints the line to add it to your `PATH`.

Supported platforms: linux (x64/arm64) and macOS (x64/arm64).

You can configure the installer with environment variables:

- `ANFRA_INSTALL_DIR` — install somewhere else (default: `~/.anfra/bin`)
- `ANFRA_VERSION` — install a specific version, e.g. `0.1.0` (default: latest)

## Updating

```sh
anfra update          # replace the binary with the latest release
anfra update --check  # check for a newer release without installing
```

## Usage

Run `anfra --help` for commands, or `anfra <command> --help` for a specific one.

## Serving

`anfra serve` keeps the sidecars warm and serves `GET /health` and `POST /call`
(`{"command": "...", "args": {...}}`) on a per-repo Unix socket, which later CLI
calls in the same repo use automatically.

## Data Apps

A Data App is one HTML file under the repo's `apps/` that queries its datasets
through the Anfra SDK (`sdk/`). `anfra serve --apps` also serves them in a
browser, through the Shell, at http://127.0.0.1:5173/ (`--port` to change it):

```sh
cd my-repo
anfra serve --apps
```

The Shell lists every Data App in `apps/` and runs the selected one, with its
queries going to the server's own sidecars. It reloads a Data App when its file
changes, and re-reads the datasets when the AML does. The repo needs
`.anfra/context_sources.yml` and `.anfra/data_sources.yml`, and its databases
must be reachable.

The Shell and the SDK bundle are built into the binary. From a checkout, build
them before `go build`; without them `--apps` refuses to start:

```sh
pnpm install
pnpm build:apps   # the Anfra SDK, then the Shell, into internal/apps/assets
go build ./cmd/anfra
pnpm test:apps    # the Shell's Playwright tests, against a fake anfra
```

`sdk/skills/build-anfra-app` is an agent skill for writing Data Apps.
