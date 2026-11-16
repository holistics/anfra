---
name: anfra-read-logs
description: Read anfra's log to find out why something went wrong, such as a query that ran without the cache, a sidecar that didn't start or stopped answering, or a Data App's runtime errors. Use when an error or hint says to run `anfra logs`, or when something misbehaved and its answer didn't say why.
---

# Reading anfra's log

`anfra logs` shows the last entries of the repo's log, oldest first: anfra's records, its
sidecars' own lines, and its Data Apps' reports. `anfra logs --help` lists how to narrow them.

## Worth knowing
* Each repo has its own log, which `anfra logs` reads in the repo's folder. For a Data App, that
  is the repo `anfra serve` serves.
* A failure's cause is often a sidecar's own line, just before anfra's record of the failure. A
  sidecar's line has no level or event, so filter by level or event only once you know what
  you're after.
* A Data App's report and the server's record of the same call share a `request_id`.
* `-f` never ends. Use it only while a person reproduces something, and you're watching.
* When `anfra logs` finds nothing, its hints say why. Read them before widening the search.

## Examples
* Why a query ran without the cache, or a sidecar didn't start, right after it happened:
  ```sh
  anfra logs --since 5m --yaml
  ```
* A Data App's errors and console warnings since you opened it. Read only the entries after its
  last `app.mounted`, since earlier ones are the old file's:
  ```sh
  anfra logs --event app --since 10m --jq '.entries[] | select(.attrs.app == "sales/overview.html")'
  ```
* The server's side of a call the app reported:
  ```sh
  anfra logs --jq '.entries[] | select(.attrs.request_id == "<id>")'
  ```
* What the query cache did in the last hour:
  ```sh
  anfra logs --event query_cache --since 1h
  ```

Don't open or grep the log file, and don't ask the user to paste it: read it with `anfra logs`.

## Related skills
* [](../anfra-build-data-app/) — build a Data App, and check it in `anfra serve`.
