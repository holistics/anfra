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

A program that runs its own server (rather than sharing the repo's) can pick the
socket with `--socket <path>`. Only a live server on that same path stops it from
starting, and later CLI calls don't use it: they only reach the default socket.

```sh
anfra serve --socket /tmp/my-tool/anfra.sock
curl --unix-socket /tmp/my-tool/anfra.sock -X POST http://anfra/call \
  -H 'Content-Type: application/json' -d '{"command":"status"}'
```
