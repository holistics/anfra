# Security

anfra is local-first: its default user runs it on their own machine, against their own repo and their own data sources. The trust model follows from that. Inside the boundary is the person who started anfra; outside are web pages in their browser, other machines on the network, and Data App code that someone, often an agent, wrote. This document says what anfra defends against, how, and what it leaves to whoever deploys it.

## The server has no authentication

`anfra serve` answers anyone who can reach it: the core API (`/api`), MCP (`/mcp`) and the Data Apps. That is a deliberate scope, not a gap to fill here: users, logins and permissions belong to the platform that hosts anfra (see [philosophy.md](philosophy.md) and [engine.md](engine.md)). So the defaults keep the server where only its operator can reach it:

- **Loopback by default.** It listens on `127.0.0.1` (`defaultAddr` in `cmd/anfra/serve.go`). Choosing another address with `--addr` is the operator's decision; `serve` logs it and prints a warning when the address is not loopback.
- **Errors show their causes, unless told not to.** By default the `anfra` binary disables error encapsulation (`cmd/anfra/main.go`), so a failure's cause reaches the client: on the user's own machine, the operator is the one who can fix it. An `anfra serve` that serves other people should hide them, with `ANFRA_HIDE_ERROR_CAUSES=1`: a cause can carry SQL, database errors, file paths and connection details. A platform embedding the engine keeps apperr's default, which hides causes (see [errors.md](errors.md)). A command run at the terminal still prints the cause there, for the person who ran it.

Exposing `serve` beyond loopback, or to people who should not see every dataset, is the operator's job: put it behind their own SSO proxy, or embed the engine in a platform that does access control.

## Web pages in the operator's own browser

A server on `127.0.0.1` is still reachable from any page the operator opens. `guard` (`cmd/anfra/serve.go`) wraps every route, `/api`, `/mcp` and the Data Apps alike, and refuses what a hostile page could make the browser send:

- **DNS rebinding.** A page whose host name resolves to `127.0.0.1` would otherwise read answers as if same-origin. The guard refuses any `Host` other than the address the server listens on or a loopback name. When it listens on every interface (`0.0.0.0`, `::`), any host name may legitimately reach it, so the check is off: that is the warned choice above.
- **Cross-origin writes.** A request that can change anything (any method but `GET`, `HEAD` and `OPTIONS`) must pass Go's `http.CrossOriginProtection`.
- **Form posts.** A body must be `application/json`. A form can only send what needs no CORS preflight, and the server grants none: it sets no CORS headers, so other origins cannot read its answers either. The one exception is an MCP session's `DELETE`, which has no body.

Tracing follows the same line: a caller's `traceparent` is continued only when the server listens on loopback, where callers are the operator's own CLI. Otherwise it is kept as a link, so a caller cannot choose anfra's trace ids or sampling (`TrustTraceparent` in `shared/httpkit`).

## Data Apps run sandboxed

A Data App definition is HTML and JavaScript from any author, often an agent, which may not think of security while building an analysis. So anfra guards it for its author, keeping it away from everything but the two calls it needs (see [data-apps.md](data-apps.md)):

- **A sandboxed frame.** The host (`web/sdk/src/host/runtime.ts`) puts the definition in an iframe with `sandbox="allow-scripts"`, as its `srcdoc`. The frame has an opaque origin: no cookies, no storage, no same-origin access to the page around it, and its own requests to the API are cross-origin, so the guard refuses them.
- **A content policy.** The definition's own requests are its author's code reaching the network, beside the bridge, with the reader's query results in hand. So the host puts a `Content-Security-Policy` `<meta>` first in the frame's document, ahead of anything of the definition's but its doctype and an opening `<html>` and `<head>` (`provisionDocument`, `contentPolicy` in `web/sdk/src/host/policy.ts`): scripts only from cdnjs, jsDelivr, esm.sh and unpkg; styles from those and Google Fonts; fonts from those and Google's font host; images from any `https://` server, `data:` and `blob:`; fetches only to the origins styles and fonts come from (the render plugin fetches what it embeds in a picture); no frames, form targets or `<base>`. A host adds origins with `createHost({ allowOrigins })`, each an `https://` origin and nothing after it (`checkOrigin`, `repoformat.CheckOrigin` in Go), added to every list but the images'; `anfra serve` takes the repo's `.anfra/config.yml` `apps.allow_origins`. A definition's own policy can only narrow it: every policy in force applies.
- **A narrow bridge.** The frame reaches data only by postMessage. The host answers only messages from that frame's window, and only the calls its plugins let a frame make, such as `core.submitQuery`, `core.fieldSuggestions` and `lineage.trace`; every other call is refused before it runs (`web/sdk/src/host/runtime.ts`, [sdk-plugins.md](sdk-plugins.md)). Nothing else of the API is reachable. The frame script itself carries no API client, which a test on the built bundle holds (`web/sdk/src/app/bundle.test.ts`).
- **Provisioned data is escaped.** The datasets and reader are written into the frame document as JSON that cannot close its `<script>` element, and the base URL as an escaped attribute (`web/sdk/src/host/provision.ts`).
- **Files are served inert.** `/appserve/files/…` serves a Data App's files with `Content-Security-Policy: sandbox allow-scripts` and `nosniff` (`internal/appserve/files.go`), so opening a definition's URL directly does not run it with the server's origin. Pages (`/`, `/apps/…`) send `frame-ancestors 'none'`, so no other site can frame them.
- **Only files under `apps/`.** A requested path is resolved through symlinks, `apps/` included, and must stay under `apps/`; dot-files and dot-directories are refused (`appFile` in `internal/appserve/files.go`). A Data App cannot read the repo's `.anfra/data_sources.yml` through the file route.

The policy is a `<meta>` on a `srcdoc` frame, not a header on a frame loaded from a URL as Holistics' Data Apps do (`DataAppSandboxController` and its ADR in the Holistics repo). A `srcdoc` frame inherits its host page's policy as well as carrying its own, and both apply, so **a host page's own policy must never restrict scripts, styles or connections** (no `script-src`, `style-src`, `connect-src` or `default-src`), or every Data App breaks. Holistics' page uses `strict-dynamic`, which is why it framed a URL; appserve's pages send only `frame-ancestors`. A URL-served frame needs a route of its own on every host and a fixed bootstrap document; the meta keeps the SDK the one place the policy lives, for every host, at the cost of that rule.

The policy closes the convenient routes to the network, not every route, as Holistics' ADR says of its own:

- `img-src https:` is a channel out: a URL can carry data to any server. The render plugin and charts drawn to a canvas need images from anywhere.
- `'unsafe-eval'` and `'unsafe-inline'` are allowed, since the definition is itself inline code and libraries use eval. `script-src` limits where code comes from, a supply-chain control, not what it does.
- A compromised CDN library runs beside the reader's results. `anfra validate` and `anfra publish` warn about a CDN script or stylesheet without an `integrity` hash (`repoformat.CheckApp`), which a browser checks before running it.
- Fetches reach the CDNs and a host's added origins. They add no destination `img-src` does not already reach.

Under a dev frontend (`ANFRA_APPSERVE_DEV_URL`, maintainers only), pages redirect to the configured dev server, and only the request's path and query are carried over: its scheme and host come from the configuration (`internal/appserve/frontend.go`).

## Credentials

Data source credentials live in the repo's `.anfra/data_sources.yml`.

- `anfra init` creates it readable by its owner only, adds it to `.gitignore`, and writes a `data_sources.yml.example` without credentials to commit instead (`cmd/anfra/init.go`).
- anfra reads it per call and passes each connection only to canal-query, which runs queries. anfra-node, which compiles them, receives only each source's name and type, for its SQL dialect (`CompileDataSources` in `internal/sidecar/anfranode/client.go`), so credentials never cross into the compiler.
- An image built with the repo copied in carries the file. Such an image is as secret as the credentials, and belongs only in a private registry.

## The engine makes no access decisions

The engine has no principal, role, policy or tenant. A platform decides whether a caller may run a command, and passes the outcome as `DataPerms`: whether the caller's queries are governed (only through the semantic layer, the repo's rules applied) or ungoverned (raw SQL too). The engine refuses a call whose `DataPerms` nobody decided (`data_perms_missing`), and a governed caller's raw SQL (`data_perms_unenforceable`). `anfra serve` states `Ungoverned`: its one user owns the repo. See [engine.md](engine.md).

## Local state and processes

- The runtime file through which CLI calls find a running server (`serve.json`) is written owner-only, in an owner-only folder (`cmd/anfra/runtime.go`).
- Sidecars run as child processes of anfra, under the same user, and stop with it ([sidecars.md](sidecars.md)).
- The Docker image runs anfra as a non-root user, `anfra`, with tini as PID 1 (`Dockerfile`).

## What is not protected

These are the operator's or the platform's to handle:

- Who may reach `anfra serve`, once it is exposed beyond loopback.
- Who may see which datasets and rows: anfra has one user. Per-viewer permissions are a platform's.
- Data a Data App sends out through an image's URL, or to an origin a host or the repo allows (above).
- The confidentiality of the machine and its files: anyone who can read the repo can read its credentials file.
- What a Data App does with the data it is legitimately given. Its frame can still load from and send to the internet (that is how it loads its chart library from a CDN), so a malicious definition could send its query results elsewhere. The sandbox keeps it from anfra's API and from the page around it, not from the network: review a definition as you would any code you run.

Related: [architecture.md](architecture.md), [engine.md](engine.md), [errors.md](errors.md), [data-apps.md](data-apps.md), [sidecars.md](sidecars.md).
