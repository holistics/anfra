# Embedding the engine

How to run anfra inside your own Go program: a BI platform, a portal, an agent runtime, any server. For what the engine promises and why it is shaped this way, read [designs/engine.md](../designs/engine.md) first.

```sh
go get github.com/holistics/anfra
```

Import `github.com/holistics/anfra/engine`, and `shared/*` for the API and error frameworks. Nothing under `internal/` is importable, nor part of the contract.

## 1. Run the sidecars

The engine does its work through two sidecars, which your deployment runs as services: anfra-node (the semantic layer's compiler, for AML and AQL today, among other Node.js functions) and canal-query (which runs queries on the data sources). See [designs/sidecars.md](../designs/sidecars.md).

- **anfra-node** listens on a port with `--port=<n>` (or `ANFRA_PORT`). Its releases are published by holistics-core, and anfra pins the version it was built against in `manifest.yml`.
- **canal-query** is published by holistics/canal; `manifest.yml` pins its release too.

Use the versions your anfra's `manifest.yml` names: the engine and its sidecars are released together.

## 2. Connect, open a repo, dispatch

```go
clients, closer, err := engine.Connect(ctx, nodeURL, canalQueryURL)
if err != nil {
	return err
}
defer closer.Close() // releases the clients; the sidecars keep running

repo := engine.OpenRepo("/srv/repos/sales") // a repo's directory

// After your platform has decided who the caller is and that they may run this:
inv := engine.Invocation{
	Clients:     clients,
	Repo:        repo,
	DataPerms:   engine.Restricted(engine.Attributes{"region": "APAC"}), // or engine.Unrestricted()
	Attribution: engine.Attribution{"user": userID},                      // for logs and traces only
}
res, err := engine.Dispatch(ctx, inv, engine.Request{
	Command: "query",
	Args:    map[string]any{"dataset": "sales", "query": "explore { dimensions { r: orders.region } }"},
})
```

- `Invocation` is the trusted half of the call: build it on the server, never decode it from a request, or a caller could state their own permissions. `Request` is the untrusted half.
- `DataPerms` must be stated: `Dispatch` refuses an invocation without them. `Restricted` attributes change which rows a query returns; `Attribution` never does.
- `res.Status` is `engine.StatusOK`, or `engine.StatusInvalid` for a command that ran and judged its input invalid (validation diagnostics). That is an outcome, not an error. `res.Data` is the answer.
- `engine.Commands()` and `engine.Describe()` list what you can dispatch, and how each is called.
- `Exports` is where `query.export` puts its file: an `engine.ExportStore` you provide, which writes the file and answers a link to it that expires (object storage with presigned URLs, say). Without one, an export fails with `engine.ExportsUnavailable`. Its links must work for whoever called: where they reach you, not where you listen. See [designs/export-store.md](../designs/export-store.md).

## 3. Handle errors

`Dispatch` fails with a code from `engine.ErrorCodes()` (or the generic `apperr.ValidationFailed` for refused args, whose violations name each arg), or an internal error. Test for a code with `errors.Is`:

```go
switch {
case errors.Is(err, engine.QueryInvalid):
	// the query does not compile; the error carries its diagnostics
case errors.Is(err, engine.SidecarUnavailable):
	// a sidecar did not answer
case err != nil:
	// treat as internal
}
```

Where your platform has its own error codes, translate the engine's into them at this boundary (`apperr.Translate`), and give each a status. Test that you translate every code in `engine.ErrorCodes()`, so an upgrade that adds one fails your build. See [designs/errors.md](../designs/errors.md).

## 4. Serve the core API as it is

To expose anfra's operations to clients (Data Apps, the SDK, agents) unchanged, serve `engine.Ops()`. You get every command as `core.<command>`, with anfra's names, schemas and codes, behind your own admission:

```go
codes := httpkit.Codes{
	Imported: engine.ErrorCodes(), // serve the engine's codes as yours
	Status: map[apperr.Code]int{
		engine.QueryInvalid.Code():           http.StatusUnprocessableEntity,
		engine.QueryFailed.Code():            http.StatusBadGateway,
		engine.SidecarUnavailable.Code():     http.StatusServiceUnavailable,
		engine.UnknownCommand.Code():         http.StatusNotFound,
		engine.DataPermsMissing.Code():       http.StatusInternalServerError,
		engine.DataPermsUnenforceable.Code(): http.StatusInternalServerError,
	},
}
admit := func(r *http.Request) (engine.Invocation, error) {
	// Authenticate r, decide what the caller may see, and build their invocation.
	return engine.Invocation{Clients: clients, Repo: repo, DataPerms: engine.Unrestricted()}, nil
}

rt, ops := apikit.NewRuntime(), engine.Ops()
mux := http.NewServeMux()
mux.Handle("/api/", apikit.HTTP[engine.Invocation]{
	Codes: codes, Title: "my platform", Version: "1",
	Request: func(_ http.ResponseWriter, r *http.Request) (engine.Invocation, error) { return admit(r) },
}.Handler(rt, ops))
mux.Handle("/mcp", apikit.MCP[engine.Invocation]{
	Name: "my platform", Version: "1", Codes: codes, Request: admit,
}.Handler(rt, ops))
http.ListenAndServe(addr, httpkit.Wrap(mux, httpkit.Config{Codes: codes}))
```

`httpkit.Wrap` adds the request id, the root span, the request log line and panic recovery. The HTTP handler also serves the OpenAPI document and discovery (`/api/ops`) for exactly the ops it mounts.

To decide per command who may call it, or to serve only some commands, register into your own registry with `apikit.Mount`. It gives each op your `Admit`, `Authorize` (which sees the input as sent) and `Bind` (which builds the `Invocation`), around anfra's own validation and handler, and may amend its doc or timeout but never its name, schemas or codes. Keep a map from `engine.Commands()` to your decision for each, and test that it covers them all.

## 5. Trace the sidecar calls

Wrap each sidecar's transport, so the engine's calls appear as spans under yours and carry your trace into the sidecars:

```go
clients, closer, err := engine.Connect(ctx, nodeURL, canalQueryURL,
	engine.WithTransport(func(sidecar string, rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return sidecar + " " + r.Method + " " + r.URL.Path
		}))
	}))
```

The engine's own spans (one per operation and step) go to the global OpenTelemetry tracer provider, which your program installs. Without one they are no-ops.
