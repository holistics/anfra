# A query's input

> Status: 2026-11-12. Partly built: `input` holds the page, the time zone and the seeds, the deprecated inputs beside it move into it, and answers return it. The CLI sets each of `input`'s fields by a flag of its own (`--input.pagination.page-size`), and matches flags ignoring case, `_` and `-`. The SDK sends a query's input in `input`, keeps the seeds a run returned, and sends them with its next page and its export; a run of its own sends none. Not yet: suggesting the name meant when one is refused. Built for the [query cache](query-cache.md).

A query has two parts: its text (AQL, or SQL), and everything else a caller chooses about running it. The second part is one input, `input`, and this doc says what goes in it, and why it is kept together.

## Why it is kept together

Callers often run a query again:

- the next page of a result;
- an export of the result on screen;
- a refresh;
- an agent reading what a user saw in a Data App;
- a requery on a cached result, by a platform that has them.

With everything in `input`, a caller does any of these by sending the query's text and the `input` the response returned, and nothing else. Inputs spread across the request are each one more thing to carry, and to forget. A forgotten input often doesn't fail: a query without its seeds runs as of now, a different query that looks the same. A client or an agent that has to know which inputs to carry gets it wrong, and the SDK would need code for each one.

Kept together, running a query again is copying one object, and an input added later is carried by every client without any of them changing. That is what makes replaying and requerying cheap enough to be how callers work, rather than something each feature builds for itself.

## What it holds

| Part | What it is |
|---|---|
| The Query Input: `filters`, `conditions`, `sorts`, `dateDrills` | What a reader changes about the query, applied to its `explore { }` before it compiles ([Data Apps](data-apps.md)). |
| `pagination`: `page`, `pageSize` | Which rows of the result to return: `pageSize` rows, from the 1-based `page` (1 by default). Without it, every row. |
| `timezone` | The IANA time zone relative dates and date truncation use. |
| `seeds` | Values the server generates for each request when they're unset (below). |

Rules for what goes in:

- **An input about a query goes in `input`,** not beside it. A new one is added there.
- **What decides access is never in `input`.** User attributes and data restrictions come from who is asking, on every query, filled in by the server or the platform from its own identity, never from what a client sends. So a replayed `input` can't widen what a caller sees.
- **What isn't about the query stays outside:** how the result is returned (a stream, an export's format and file name) and how the cache treats it (`cache`: its duration, and whether to refresh). These are the caller's choices for this run, not part of the query it replays.

## The response

The response returns `input` as the query ran, every default filled in, its seeds included. So what a caller replays is exactly what produced the result it has, and it can read which time zone and reference time were used without knowing the defaults.

`input` means the same to every command that takes it: `query`, `query compile` and `query export`. An export of a paged `input` exports that page. A caller replaying a query for another purpose changes only what that purpose needs: the next page sets `pagination.page`, and an export of the whole result leaves `pagination` out, one key, as the SDK's `query.export()` does ([exports](exports.md)).

`pagination` is one object, rather than two inputs, for that reason and one more: a page needs a page size, which the object's shape says (`pageSize` is required in it).

## Seeds

A seed is an input whose default is generated anew on each request. Leave any other input out twice, and the query compiles the same way twice. Leave a seed out twice, and it may not. Today there is one:

| Seed | What it is |
|---|---|
| `seeds.referenceTime` | The time the query runs as of. Relative dates, such as `last 7 days`, are counted from this time's day. |

`seeds` is described in the schema as: "values the server generates for each request when they're unset. Generated seeds typically change with every request, and the response returns them. Set seeds to generate the same SQL again, for example to replay a query as of a given time."

- **A replay keeps them:** the next page, an export, an agent reading what a user saw. All of them are the result the user had, as of when it ran.
- **A fresh run leaves them out:** a refresh, a reader changing a filter, an app loading. The SDK does this for its callers ([query cache](query-cache.md#in-a-data-app)).
- **Any caller may set one,** because a seed lets a caller ask for nothing it couldn't otherwise: "last 7 days as of last Tuesday" is a query anyone may run.
- **A seed never decides access.** It is an input like the others, under the rule above.

The query cache's key depends on a seed only through what it resolves: a `last 7 days` query keeps its key for a day ([query cache](query-cache.md#keys)).

One hazard comes with replaying: a client that stores a response's `input` and sends it on every load freezes the time, and keeps seeing the result as of then. A load is a fresh run, and leaves the seeds out.

## Its case

`input`'s keys are camelCase, all of them: `dateDrills`, `pagination.pageSize`, `seeds.referenceTime`. The request's other inputs, and the rest of anfra's API, are snake_case (`cache.read_only`).

`input` is the query's description, and the code that builds and reads it speaks camelCase: the SDK (JavaScript), anfra-node and amql. Core passes most of it through. The Query Input already kept amql's case for that reason ([Data Apps](data-apps.md)), so the case changes once, at `input`, rather than inside it. Nesting the Query Input's part under its own key (`input.transforms`) was considered: it would only move the mixed case down a level, and the line it draws isn't sound, since the page and the time zone change the query too.

## Moving to it

`page`, `page_size` and `timezone` move into `input`, as `pagination.page`, `pagination.pageSize` and `timezone`. The inputs beside it are deprecated, and kept working until they're removed ([deprecation before removal](commands-and-api.md#deprecation-before-removal)). A request that sets one both ways takes `input`'s: a client in the middle of moving replays a response's `input` and may still send the old input beside it, and `input` is what it means. The response's `input` shows what ran.

Two changes to how anfra reads names come with it, since callers used to `page_size` will send it inside `input`, where it is now `pageSize`:

- **The CLI sets an object's nested fields by flags of their own,** a level deeper than today's `--format-options.header`: `--input.pagination.page-size`. These are the fields' only names at the CLI ([one name per field](commands-and-api.md#one-name-per-field)). Today's `--page`, `--page-size` and `--timezone` are the deprecated inputs' flags, and go with them.
- **The CLI names a camelCase field in hyphens** (`pageSize` as `page-size`, as `page_size` is `--page-size` today), **and matches a flag ignoring case, `_` and `-`**, so `--input.pagination.pageSize` works too (pflag's `SetNormalizeFunc`, on the root command).
- **A refused name suggests the one meant.** The API stays strict, one name per field, but its refusal of an unknown property names the nearest one the schema allows at that place ("Unexpected property. Did you mean pageSize?"), in `shared/apikit/internal/decode/violations.go`. So does the CLI's refusal of an unknown flag (`flagRefused`). The nearest is a name equal with case, `_` and `-` ignored, or else one within a small edit distance; one is suggested, or none. An enhancement, after the move.
