# Data App serving runs in-process in `anfra serve --apps`

Data App serving started as a separate demo binary (holistics/anfra-demo-shell): a Go server that carried an anfra binary, extracted it to the user cache, started it on a private Unix socket (a `--socket` flag added to `anfra serve` for it) and called its commands over `/call`, because anfra's `internal/` packages can't be imported from another module. Inside anfra, the Shell's server is just another surface of the command registry. `anfra serve --apps` serves the repo's Data Apps on `127.0.0.1:<port>` beside the usual socket, and runs every query, suggestion, validation and dataset search with `app.Dispatch` on the server's own warm sidecars. The Repo is the one anfra runs in, as for every other command.

## Considered Options

- **Keep a separate binary that drives anfra over `/call`.** It kept the Shell and the SDK bundle out of the anfra product, but nested one anfra inside another binary, needed its own socket, health polling and version checks, and had to be rebuilt from four checkouts. Rejected.
- **A separate `anfra apps` command.** It would start its own sidecars, so the CLI and the browser would not share one warm server. Rejected.
- **Serve the Shell over the `/call` socket hop to our own process.** No gain over calling `app.Dispatch` directly. Rejected.

## Consequences

- The anfra binary carries the built Shell and the Anfra SDK's IIFE bundle (`go:embed` under `internal/apps/assets`). They are build outputs, not committed: `pnpm build:apps` writes them before `go build`. A binary built without them still builds and runs; only `--apps` refuses to start, and says how to build them.
- `--apps` checks the repo's context and data source config, that its databases listen, and claims its port before the sidecars start, and a failure in Data App serving stops the whole server.
- The Shell's e2e tests run `anfra serve --apps` built with `-tags apps_e2e`, which swaps the sidecars for canned answers (`dispatch.Fake`).
- Embeds (a Data App framed by other websites without the Shell) were not carried over. The Shell still refuses to be framed.
