# Exports

> Status: the core is built: `query.export` (`internal/command/query/export.go`), the `ExportStore` interface (`internal/command/export.go`), and anfra's own store, for `anfra serve` and the CLI (`internal/exports`, [export-store.md](export-store.md)). The SDK's part (`Backend.exportQuery`, the bridge, the download) is not yet.

An export is a query's whole result as a file: what a reader downloads from a Data App, what a script saves, what an agent hands to its user. A query answers a page of rows for a screen to show (once results are paginated); an export answers all of them, as a file, without anfra or the browser holding them in memory.

Exports here are data: a query's rows in a data format (CSV first). A picture of a visualization or a Data App (PNG, PDF) is a rendering, not a query's data, and is a separate design.

## The interface

One op, `query.export`, and a link:

1. **`query.export`** takes the same input as `query` (a dataset and AQL, or a data source and SQL, and the Query Input: filters, conditions, sorts, date drills), without `page` and `page_size`, plus the format and, optionally, the file's name. It runs the query to completion, writes the file to the server's storage, and answers once the file is ready:

   ```jsonc
   { "url": "…", "filename": "sales.csv", "format": "csv", "row_count": 123456, "expires_at": "…" }
   ```

2. **The download is a plain `GET` of `url`** before `expires_at`: no headers, no credentials beyond the URL. What the URL points to is the server's (anfra's own endpoint, or a presigned object-storage URL); the API promises only that a `GET` returns the file until it expires. Downloading is not an op: a browser saves a file to disk only by following a link, and a link to object storage must not pass through anfra.

The answer is an ordinary typed document, so the op is an ordinary op: its schema, Strict mode, discovery, MCP and the CLI need nothing new.

### Why the query runs before the answer, not at the download

- **Errors reach the caller.** A query that fails (the warehouse refuses it, it times out) fails `query.export` with a typed error a Data App can show. Run at the download, it would fail inside the browser's download, where the Data App never hears of it.
- **The warehouse is freed at its own speed**, not held open at the pace of a slow download, and a retried download does not run the query again.
- **The download is only bytes**, authorized when the file was made. Nothing has to be decided again when the link is followed.

The cost is latency: the download starts when the query has finished.

### Format and its options

`format` names the format, `csv` by default; `format_options` holds that format's options:

```jsonc
{ "format": "csv", "format_options": { "header": "labels" } }
```

| Format | Options |
|---|---|
| `csv` | `header`: `labels` (default; what readers see), `names` (each column's key, as in `fields`; stable for programs), or `none`.<br>`bom`: start the file with a UTF-8 byte-order mark (default `false`). Excel needs it to read non-ASCII text right; most programs need it absent. A caller exporting for a person to open, such as a Data App's download button, should set it. |

A format added later (XLSX, Parquet) adds its value to `format` and its options to `format_options`. An option that doesn't belong to the format given is refused by the command's `Check`, as a violation on that field (`format_options.header` is not an option of `parquet`), never ignored.

Both are plain: an enum and an object, not a union. That keeps the schema easy for a generated client or an agent to fill, and every arg is one the CLI already takes. What it gives up is the schema saying which options go with which format; `Check` says it instead. That's sound while the data formats are tables sharing most options. If they stop being, a union discriminated by format is the shape to move to.

**What a CSV holds.** Values as the query answers them, unformatted: dates in ISO 8601, numbers with every digit, `null` as an empty cell. Applying the model's display formats is a separate, later option, not part of `header`. Labels can repeat where names don't; a `labels` header with two equal labels is valid CSV but ambiguous to a program, which should ask for `names`. The response's `Content-Type` says whether there is a header row: `text/csv; charset=utf-8; header=present` (RFC 4180).

**The file's name** is the caller's `filename` (blank is unset), or the dataset's or data source's name; either way with the format's extension added when it has none. It is never a path on disk: a store keeps it beside the link, and the download's header carries it encoded. So it need only be one name: any script is welcome, and one that is a path or no name at all (only dots, a `/` or `\`, a control character, over 255 bytes) is refused as a violation on `filename`, not changed. What one system forbids in a name, such as a colon on Windows, is for the browser to replace where the file lands.

**No row cap.** An export is the whole result. What bounds it is the server's storage, not anfra's memory: rows are written to the file as they arrive. With a header row they go to a spool file in the system's temp directory first, since canal-query names the columns only after the last row, and the header comes first.

## The layers

| Layer | Owns |
|---|---|
| **command** (`internal/command/query`) | `query.export`: check and compile the query as `query` does (its data restrictions included), run it, write each row to the file as it arrives, answer the link. It knows no storage and no URL. |
| **the export store** (`ExportStore`, on `CommandContext` as `Exports`, given by the server) | Where files go and how they are reached: create a file, write it once, then get its URL and when it expires. Nothing more: no reading back, listing or keys, which a cache or uploads would need, and would get as an interface of their own. |
| **app / engine** | Nothing new: `query.export` is a command like any other, dispatched with the same rules. |
| **the server** (`anfra serve`, a platform) | Its export store, and serving what it stores. |
| **SDK** | `Backend.exportQuery`, and downloading the link from the page around the Data App. |

### The export store

The one thing a server provides for exports, as it provides its clients and data restrictions. [export-store.md](export-store.md) is anfra's own, its cleanup, and how a platform's differs. A server that provides none can't export: `query.export` refuses, as a command does without a sidecar it needs.

| Server | Store | URL |
|---|---|---|
| `anfra serve` | a file per export, named by an unguessable token, in the system's temp directory (`os.TempDir()/anfra-exports-<uid>`, so `TMPDIR` moves it) | its own `GET /exports/download/{token}/{name}`, under the URL the caller reached it at, or `ANFRA_SITE_URL` behind a proxy |
| the CLI, without a server | the same folder | a `file://` URL |
| a platform (anfra-cloud) | object storage | a presigned URL |

**Temp files, not `~/.anfra`.** An export is not something anfra keeps. A file stays until its link expires, not until its first download, so a failed download can be retried or resumed; when and how anfra's own store removes it is in [export-store.md](export-store.md).

**How long a link lives is the store's.** It answers `expires_at` with the URL, and `query.export` passes it on; the API promises only that the link works until then. `anfra serve` picks its own lifetime, as it picks its port; a platform's store picks its own. The CLI without a server copies the file and deletes it, so its link outlives nothing.

**The link is a bearer link:** whoever has it can download until it expires. Expiry is short and the token unguessable. A platform that must check the caller at the download points the URL at its own `GET` instead of at object storage; the interface doesn't change.

**A platform caching rendered files** (an XLSX costly to make, made again for the same query) is the platform's store's business: it can answer the same URL for the same export. Core promises nothing about it.

## Each door

| Door | Flow |
|---|---|
| **Data App** | The app runtime builds the export from the query's current state (filters, cross-filter selection, sort; no page), so a download is everything the reader is looking at, and asks for a byte-order mark, since a person will open it. The request crosses the bridge; the page around the Data App calls `query.export` and downloads the `url` with `<a download>`, so the browser writes the file to disk without holding it. The frame stays `sandbox="allow-scripts"`, without `allow-downloads`. A failed export is an error the Data App shows. |
| **SDK** (`Backend`) | `exportQuery(request)` beside `submitQuery`, from the same request builder, so a page of a query and its export cannot drift apart. |
| **HTTP** | `POST …/core.query.export` answers the link; `GET` the link. |
| **MCP** | The tool answers the link, which the agent gives its user. The data never enters the model's context. |
| **CLI** | `anfra query export … > sales.csv` (or `-o sales.csv`): the CLI calls the op and writes the file it links to (`file://` or `https://`) to its output. `--link` prints the answer instead. Options are flags by their path, `--format-options.header names`, or the whole object as JSON, `--format-options '{"header":"names"}'`, not both. |

## Later, and separate

- **Asynchronous exports** (an export too long for the op's timeout, "email me when it's ready"): their own API design, not a `status` field guessed now.
- **Renderings** (PNG, PDF): a separate export, of a visualization or a Data App, made by rendering it.
- **Formats beyond CSV**, each a new value of `format`, with its options.
- **Display formatting** of values, as its own option.

