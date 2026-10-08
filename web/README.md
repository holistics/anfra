# web

anfra's browser code: a pnpm workspace (`pnpm-workspace.yaml` at the repository root) of two
packages, both for Data Apps. The Go side never imports them; it serves what they build.

| Package | What it is | Where it ends up |
|---|---|---|
| [`sdk/`](sdk/) (`anfra-sdk`) | The Anfra SDK: the runtime a Data App uses (`anfra-sdk/app`), the code that hosts one in a page (`anfra-sdk/host`), a client for the core API (`anfra-sdk/api`), and the types they share (`anfra-sdk/common`). See its [README](sdk/README.md). | A dependency of `appserve`, and of any other host of Data Apps. |
| [`appserve/`](appserve/) (`anfra-appserve`) | The appserve frontend: the Vue app `anfra serve` shows at `/` and `/apps/<path>`. It lists the repo's Data Apps, runs the selected one through `anfra-sdk/host`, and shows the server's health and the repo's problems. | Built into `internal/appserve/dist`, which the Go binary embeds and the appserve backend (`internal/appserve`) serves. |

## How it fits with the Go side

- **The core API's types come from its spec.** `sdk/src/api/schema.d.ts` is generated from
  `api/openapi.yaml`. After an op's input or answer changes, regenerate it
  (`pnpm --filter anfra-sdk generate`); CI fails while it is stale.
- **The frontend is embedded, not served from disk.** `pnpm build:web` builds the SDK, then the
  frontend into `internal/appserve/dist`, and `go build` embeds that folder. A binary built
  without it serves a page saying how to build it. Releases always build it first.
- **A Data App never reaches the API itself.** It runs in a sandboxed frame; `anfra-sdk/host`
  answers its calls over postMessage, with the core API client from `anfra-sdk/api`.

## Developing

From the repository root, after `pnpm install`:

```sh
make dev              # the Go server, the SDK and the frontend, each rebuilt on change
                      #   (needs ANFRA_DEV_REPO in .env.local: see .env.local.example)
pnpm build:web        # the SDK, then the frontend into internal/appserve/dist
pnpm test:web         # both packages' tests (test:sdk, test:appserve); the frontend's need the build
make test             # the Go tests too
```

Under `make dev`, open the frontend from Vite at `http://localhost:5173/`: it reloads on every
edit, and proxies `/api` and `/appserve` to the Go server (`ANFRA_SERVE_URL`, else
`http://127.0.0.1:7878`). The server's own pages, at `:7878`, show the last `pnpm build:web`.

Each package also runs on its own from its folder: `pnpm dev`, `pnpm build`, `pnpm test` and
`pnpm typecheck`.
