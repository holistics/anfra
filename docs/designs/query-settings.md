# A repo's query settings

> Status: 2026-11-17. Partly built: the settings, read strictly; the time zone and week start day defaults, `input.weekStartDay`, the locks, and `query.cache.max_duration`. Not yet: `anfra init` writing the file, and the repo's info returning the settings. Replaces `cache.query.*` in `.anfra/config.yml`, which hasn't been released, so it is renamed without a deprecation.

A repo sets how its queries run in `.anfra/config.yml`, under `query`: the values a query gets when it sets none, which of those values a query can't change, and the rules for the query cache. This doc says what each setting means, how anfra reads the file, and what `anfra init` writes.

## The settings

```yaml
query:
  default:                  # what a query gets when it sets none
    input:
      timezone: Etc/UTC
      weekStartDay: monday
    cache:
      duration: 10m
  locked_defaults:          # defaults a query can't change
    - input.weekStartDay
  cache:                    # the query cache's rules
    enabled: true
    max_duration: 168h
```

`query` holds three kinds of setting, and where a key sits says which kind it is:

| Key | Kind | Can a query change it? |
|---|---|---|
| `query.default.*` | A value a query gets when it sets none | Yes, unless it is in `locked_defaults` |
| `query.locked_defaults` | Which defaults a query can't change | No |
| `query.cache.*` | A rule for the repo | No |

### `query.default`

`query.default` has the shape of a query request, case included. Each key under it is the path of a field a query sets, as the query sets it: `query.default.input.weekStartDay` is the default of a query's `input.weekStartDay`, and `query.default.cache.duration` the default of its `cache.duration`. So a reader who knows a query's field knows its default's key, and values copy between an answer's `input` and the file unchanged. `input`'s keys are camelCase wherever they appear ([its case](query-input.md#its-case)), so under `query.default.input` they are camelCase, and the rest of the file is snake_case, as the request's other inputs are.

| Key | Default of | Without it |
|---|---|---|
| `input.timezone` | `input.timezone` | `Etc/UTC` |
| `input.weekStartDay` | `input.weekStartDay` (new) | `monday` |
| `cache.duration` | `cache.duration` | none: a query that sets no `cache.duration` runs uncached |

A query's answer returns its `input` with every default filled in ([a query's input](query-input.md#the-response)), so a replay runs with the values it ran with, even after the repo's defaults change.

Only fields a repo would reasonably want the same for every query have a default. Others don't:

- **`input.pagination`:** without it, a query returns every row. A default page size would cut off every caller that expects the whole result.
- **`input.seeds`, the Query Input, and `cache.refresh`, `cache.read_only` and `cache.cached_only`:** these are about one request, not the repo.

### `query.locked_defaults`

A list of paths into `query.default`. A query may leave a locked field out, or send the same value. It can't send a different one:

```
input.weekStartDay is sunday, but the repo locks it at monday
(query.locked_defaults in .anfra/config.yml). Leave it out, or send monday.
```

A query that sends the locked value is accepted, because callers send back the `input` an answer returned, for the next page, an export or a replay, and it holds the locked value.

Each path must name something `query.default` sets. A path that doesn't is a config error.

A path names a single value, such as `input.timezone`, or an object. Locking an object locks it exactly as `query.default` sets it, so a key the default doesn't set is locked as absent. That is only safe when every field of the object can have a default. Otherwise the lock would forbid fields no default can hold: locking `input` would forbid `filters`, `pagination` and `seeds`, so every filtered or paged query would fail. So a path may name an object only if every field of it can have a default. No object qualifies today, so only single values can be locked:

```
query.locked_defaults in .anfra/config.yml names input, which has fields
that can't have a default. Lock its fields one by one, such as
input.timezone and input.weekStartDay.
```

The list keeps whole-object locks possible for when such an object exists, which is why it is a list of paths ([alternatives](#alternatives-considered)).

### `query.cache`

The query cache's rules ([query cache](query-cache.md)). A query can't override them.

| Key | Meaning | Without it |
|---|---|---|
| `enabled` | `false` turns the query cache off for the repo ([turning it on](query-cache.md#turning-it-on)). | `true` |
| `max_duration` | The longest a query's result may be cached, between `1s` and `168h` (7 days). | `168h` |

A query that asks for longer than `max_duration` is refused, not cut down to it. An agent or an app's author fixes the query in one edit, and a quiet cap would leave them believing a result is kept longer than it is:

```
cache.duration is 48h, but the repo caches a result for at most 24h
(query.cache.max_duration in .anfra/config.yml). Use 24h or less.
```

`ANFRA_QUERY_CACHE_ENABLED` (was `ANFRA_CACHE_QUERY_ENABLED`) overrides `enabled` for a process, as before.

## The week start day

`input.weekStartDay` is the day a week starts on, for everything in a query that uses weeks: grouping by week, relative dates such as `this week`, and the day-of-week number. It is new in `input`, and anfra sends it to anfra-node as `options.weekStartDay`, which anfra-node already takes. Its values are `monday` to `sunday`.

anfra always sends a day, `monday` unless the query or the repo sets another. Today it sends none, so anfra-node uses `"default"`, and on some databases that splits a query's idea of a week:

| What uses the week | With `"default"`, on MySQL |
|---|---|
| Grouping by week | Sunday, because the MySQL dialect builds it from `DAYOFWEEK` (`sql-generation/src/sqlGenerators/mysql/operations.ts`) |
| Relative dates (`this week`, `last week`) | Monday (`amql/src/utilities.ts`, `parseTime`) |
| The day-of-week number | Monday, on every database but BigQuery (`weekStartDayToIndex`) |

So on MySQL today, a `this week` filter covers Monday to Sunday while its weekly buckets run Sunday to Saturday. A given day makes every part agree, on every database.

This changes results: a MySQL repo's weekly groupings move from Sunday to Monday, and so do BigQuery's and any other database whose own week starts on Sunday. The changelog says so, with the fix for a repo that wants Sunday: `query.default.input.weekStartDay: sunday`.

## Reading the file

anfra reads `.anfra/config.yml` strictly. A key it doesn't know is an error, not ignored, because an ignored key fails without a sign: a misspelled `weekStartDay` leaves every week starting on Monday, and nobody notices until the numbers look wrong.

The keys anfra knows are the fields of `repoconfig.Config`, decoded with yaml.v3's `KnownFields(true)`. So the settings are an allowlist, kept by the struct itself: `query.default.input.pagination` is refused because the struct has no such field, not because a list forbids it, and a new default exists once its field is added. The errors list the allowed keys by reading the struct's `yaml` tags, so they can't drift from it either. `locked_defaults` needs no list of its own, because it may only name what `query.default` sets.

```
.anfra/config.yml line 6: query.default.input.week_start_day isn't a setting. Did you mean weekStartDay?
```

The suggestion is the known key equal to it with case, `_` and `-` ignored, or else the nearest within a small edit distance, as for a query's input ([a query's input](query-input.md#moving-to-it)).

A newer anfra may know a key an older one doesn't, for example after a teammate upgrades and adds a setting. So the error for a key that isn't a typo of a known one also says that a newer anfra may have added it, and to update anfra (`anfra update`).

The other config errors:

| Case | Message |
|---|---|
| A default or a locked duration over the cap | `query.default.cache.duration in .anfra/config.yml is 48h, more than query.cache.max_duration (24h). Lower the default or raise the cap.` |
| A path in `locked_defaults` that `query.default` doesn't set | `query.locked_defaults in .anfra/config.yml names input.timezone, which query.default doesn't set. Set query.default.input.timezone, or remove it from query.locked_defaults.` |
| A locked `cache.duration` with the cache off | `query.locked_defaults in .anfra/config.yml locks cache.duration, but query.cache.enabled is false. Remove the lock, or turn the cache on.` |
| A lock on an object with fields that can't have a default | `query.locked_defaults in .anfra/config.yml names input, which has fields that can't have a default. Lock its fields one by one, such as input.timezone and input.weekStartDay.` |
| A path that can't be a default | ``.anfra/config.yml line 4: query.default.input.pagination isn't a setting. The settings under query.default.input are timezone and weekStartDay. If a newer anfra added it, update anfra (`anfra update`).`` |

## What `anfra init` writes

`anfra init` writes `.anfra/config.yml` with every setting at its default, so a new repo shows what it can set without anyone opening the docs. Like init's other files, it is written only when it isn't there.

```yaml
# How this repo's queries run. https://docs.anfra.ai/repo-settings
query:
  default:            # what a query gets when it sets none
    input:
      timezone: Etc/UTC
      weekStartDay: monday
    cache:
      # duration: 10m   # cache every query's result for this long, unless it sets cache.duration
  locked_defaults: [] # defaults a query can't change, such as input.weekStartDay
  cache:
    enabled: true
    max_duration: 168h
```

`cache.duration` is written as a comment, because its default is none: a new repo's queries run uncached unless they ask. Writing `0s` instead would mean the same today, but reads as a choice the repo made.

The values init writes are the values anfra uses without the file, so a repo set up before this change and one set up after it behave the same.

`anfra publish` sets `org.url` in the same file, through `repoconfig.SetString`, which changes only that value and keeps init's comments.

## Where a host gets them

The file is how anfra's CLI and `anfra serve` learn a repo's settings. The engine doesn't read it: a host sets them on each invocation, so a platform embedding the engine, such as anfra Cloud, keeps them wherever it keeps a repo's settings.

| On the invocation | Holds | The file's keys |
|---|---|---|
| `QueryDefaults` | `Timezone`, `WeekStartDay`, `CacheDuration`, `Locked`, and `Err`: why they can't be used | `query.default.*`, `query.locked_defaults` |
| `QueryCache.Settings()` | `Disabled`, `MaxDuration`, `Namespace` | `query.cache.*` |

`QueryDefaults.Check()` holds the rules every host's defaults follow: a time zone and a week start day anfra takes, a cache duration of 0s or more, and locks only on defaults. The engine calls it before every query, which fails with `repo_settings_invalid` when it refuses, or when `Err` is set. anfra's own reading of the file calls it too (`repoconfig.Config.QueryDefaults`, the one place the file's defaults become the engine's), adding where the value came from. A platform calls it when it saves a repo's settings, so a mistake is refused there, not on the next query. `QueryCacheSettings` has the same pair for its cap: `Check()` for its range, and `CheckDefault(QueryDefaults)` for a default duration over it, which would refuse every query that sets none. The engine calls both before each query, and anfra's reading of the file calls the same functions, so each rule has one copy.

The file's reading keeps what is about the file: unknown keys, line numbers, and that a locked default is set in it explicitly. A platform's empty value means anfra's default, so locking an unset time zone locks it at `Etc/UTC`.

## Levels

A setting may have a default at several levels, each added where something needs it, and the more specific level wins. For a query's time zone and week start day, the first that is set:

1. the query's own `input.timezone` and `input.weekStartDay`;
2. its Data App's `query.default.input.*`, for the app's queries only;
3. its Data App's own `timezone` and `weekStartDay`, for the app: its UI (`app.timezone`, `app.weekStartDay`) and its queries;
4. the repo's `query.default.input.*`, applied by the server;
5. anfra's own, `Etc/UTC` and `monday`.

The SDK sends a value only when 1–3 sets one, so a host's own defaults apply underneath. An app sets 2 only to give its queries another value than its page uses, and since `app.setTimezone()` and `app.setWeekStartDay()` change 3, they reach no query while 2 is set: `inspect` warns when 2 and 3 differ.

The repo has no level like 3 yet: nothing but queries uses its time zone and week start day. When something does, such as a Data App's page using the repo's when the app sets none, the repo gains its own `timezone` and `weekStartDay` at that level, and `query.default.input` keeps overriding them for queries.

## What a caller can read

The repo's info, which already returns the default cache duration (`api/openapi.yaml`), returns every setting here: the defaults, the locked fields and the cap. An app or an agent can then send a query that fits the repo's settings the first time, without learning them from an error.

## Alternatives considered

- **`cache.query.*`, grouped by the cache.** It would put every cache in one place, but there is one cache, and a query's own settings would have no home. `anfra cache status` and `anfra cache clean` still group by cache, because they manage what is stored, not how queries run.
- **`query.cache.default.duration`, each setting's default inside its own block.** It reads well with one default, but every new setting repeats `default`, and its key doesn't follow from the query's field.
- **A `locked` block shaped like `default`, holding the locked values.** Each value would be in one place. But YAML can't tell "lock this object" from "lock these keys of it": both are written the same. A list of paths can say either.
- **Accepting both `query.default.cache.duration` and `query.cache.default.duration`.** Two spellings for one value, with one silently winning, is harder to read than one, and an error naming the right key does the same job.
- **Keeping the database's own week start when none is set.** Weekly groupings wouldn't change, but relative dates would, and the default would differ by database, and need a table of each one's day.

## Open

- **Syncing a repo's settings to anfra Cloud.** The plan is for `anfra publish` to send the repo's `QueryDefaults` and cache rules, typed, through Cloud's publish API, which checks them with `QueryDefaults.Check()`. Cloud never parses the file, so its format stays anfra's, and a newer CLI's setting is an API change rather than a key an older Cloud refuses. If Cloud ever reads repos without publish, such as from git, the file's reading moves to a `shared/` package, which makes the format public.
- **The docs page** the init comment links to doesn't exist yet.
