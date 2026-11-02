# Plugin: export

`query.export()`, from the `export` plugin. It is there only where the host installed the plugin:
check `Anfra.plugins.has('export')`, and `Anfra.user.permissions.canExportData`, before offering it
(see [api.md](../api.md#plugins)). Its button follows [downloads.md](../downloads.md).


`query.export()` has the host make a file of the query's **whole result** and save it: every row,
not only the pages loaded, with what the result on screen was last run with (the mapped controls,
the selection, the sort, the timezone). A control the reader changed but has not applied is not in
it; `execute()` first if it should be. It changes nothing on the query (`state`, `result` and
`error` stay as they are). Don't build a CSV from `query.result.rows` instead: a paged result holds
only the pages loaded.

- Resolves once the file is made and its download has started, with
  `{ url, filename, format, rowCount, expiresAt }`.
- Options, all optional: `filename` (default: the app's title and the query's name, `Sales -
  byRegion.csv`; the extension is added), `format` (`'csv'`), `formatOptions`
  (`{ header: 'labels' | 'names' | 'none', bom }`; `bom` defaults to `true`, which Excel needs for
  non-ASCII text), `signal` (an `AbortSignal` to cancel it; `query.abort()` does not).
- Values are written raw: numbers with every digit, dates in ISO 8601, `null` as an empty cell.
- A large result takes as long as the query does to run in full.
- Rejects with the query's error classes, naming it in `entity`: a `QueryError` for a bad option,
  a `TransportError` when the server can't be reached.
