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

## Serving over HTTP

`anfra serve` keeps the sidecars warm and serves `GET /health` and `POST /call`
(`{"command": "...", "args": {...}}`) on a per-repo Unix socket, which later CLI
calls in the same repo use automatically. For clients that can't use a Unix
socket, `--http` also serves the same API over TCP:

```sh
anfra serve --http 8080             # 127.0.0.1:8080
anfra serve --http :0               # 127.0.0.1, any free port (the address is printed)
anfra serve --http 0.0.0.0:8080     # all interfaces — warns: /call has no auth
```

```sh
curl -X POST http://127.0.0.1:8080/call -H 'Content-Type: application/json' \
  -d '{"command":"query","args":{"dataset":"<dataset>","aql":"<aql>"}}'
```

The TCP listener blocks browser-originated requests: `POST /call` must send
`Content-Type: application/json` (else 415), and the `Host` header must be a
loopback name or the bound address (else 403; skipped for a `0.0.0.0`/`::` bind).
`anfra status` shows the HTTP address of a running server.
