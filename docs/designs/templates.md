# App templates

> Status: proposed, not built. Phase 1: HTML Data App templates, installed and mapped from the CLI. This is core's side: the `shared/templates` package, the parser calls it needs from anfra-node, the install record and the `anfra template` commands. The gallery, the cloud Templates page and the templates repo's CI are designed elsewhere.

A template is a set of ready-made Data Apps written against a demo dataset. A user installs one into their repo and maps the fields it uses to fields of their own datasets. The installer then rewrites the apps' AQL once, at install, and writes plain apps that query the user's data. Nothing is rewritten when a query runs.

The mapping is deterministic. A mapper recommends a target for each field, and the user or their agent confirms it. The same template, version and mapping always give the same files, byte for byte. That is what lets an install be re-mapped later, and updated to a new version, with a three-way merge over the user's own edits.

## Concepts

| Term | Meaning |
|---|---|
| Template | A folder with a `manifest.json` and one or more artifacts. Phase 1 artifacts are apps. |
| Template ID | `<owner>/<repo>/<path>`, for example `<owner>/anfra-templates/ecommerce-overview`. |
| Version | Set in `manifest.json`. Each release is a git tag, `<path>@<version>`, for example `ecommerce-overview@1.2.0`. |
| Interface | The references a template makes that nothing inside it satisfies. For an apps-only template, that is the dataset fields its AQL and SDK calls name. It is computed from the code, never declared. |
| Mapping | A target for each reference in the interface: a dataset for each template dataset, an AQL expression for each field, and a value for each mapped literal. |
| Mapper | The deterministic code that recommends a mapping (`Plan`) and rewrites a template with one (`Apply`). |
| Install record | `.anfra/templates.yml` in the user's repo: one entry per install, with its source, version, commit, mapping and file hashes. |
| Demo data | A data template (data source, models, dataset) that every app in a template runs on. CI checks the apps against it, and `--demo` installs it. |
| Index | `index.json`, built by `anfra template index`. Every client reads it to find templates, their releases and the fields they need. |

## Where it lives

```txt
shared/templates/       Fetch, Plan, Apply, the record, the app handler. No I/O of its own.
cmd/anfra/template*.go  anfra template add | map | remove | check | index
internal/sidecar/anfranode/  AppExtract and AQLReferences, the Go side of the parser calls
```

`shared/templates` is outside `internal/` so a platform that embeds anfra can import it and run the same install from its own UI. It reaches nothing directly. Each caller supplies four things:

```go
// Fetcher returns a template's files and metadata at a version or commit.
type Fetcher interface {
	Fetch(ctx context.Context, ref Ref) (Template, error)
}

// Catalog is the user's datasets and fields, read as the user, for Plan.
type Catalog interface {
	Datasets(ctx context.Context) ([]Dataset, error)
	DistinctValues(ctx context.Context, dataset, field string, limit int) ([]string, error)
}

// Parser is the parser process: extraction, references and validation.
type Parser interface {
	AppExtract(ctx context.Context, html string) (Extraction, error)
	AQLReferences(ctx context.Context, aql string, v2 bool) (References, error)
	ValidateAQL(ctx context.Context, dataset, aql string) ([]Diagnostic, error)
}
```

The fourth is the writer: the CLI writes the changeset to disk, and a platform commits it as the member. `Apply` itself writes nothing.

The `Parser` interface is why the package can live outside `internal/`. anfra's anfra-node client is internal, so the CLI wraps it, and an embedding platform wraps the clients it gets from `engine.Connect`.

The commands are CLI-only, like `anfra skills`. They are not core HTTP or MCP ops: a core server shouldn't fetch from the internet and write repo files for its callers. A GUI calls the package through its own endpoint.

## Template format

```txt
<owner>/anfra-templates/
  featured.json               # template IDs in gallery order
  demo-ecommerce/             # a data template
    manifest.json
    data/demo_ecommerce.duckdb
    models/  datasets/
  ecommerce-overview/
    manifest.json             # required
    preview.png               # required, 2:1
    apps/overview.html
    apps/retention.html
```

Any folder with a `manifest.json` is a template, and its ID is the folder's path. Artifacts sit in the same folders as in an anfra repo (`apps/`, `models/`, `datasets/`).

`manifest.json` holds only what authors know and the code can't say: title, description, version, `min_anfra_version`, category, tags, authors, the `demo` template, the `apps` list, and per field a `description` (required for metrics), `synonyms` and the meaning of each literal value that can be mapped (`values`). `network` lists extra origins the apps load from, and must be empty in phase 1, since in a published repo only an Admin can change `apps.allow_origins`. Names, kinds and types are derived from the apps and the demo data by `anfra template index`, never restated by the author.

An entry of `apps` is a path, or `{ "path": …, "mapping": "none" }` for an app that needs no mapping ([below](#apps-that-need-no-mapping)).

### What authors may write

Extraction is static, so apps follow these rules. `anfra template check` enforces each one.

- Every query is a `createQuery(name, { dataset, aql, ... })` call whose `dataset` and `aql` are literals. A template literal may not contain `${}`.
- `createFilter` and `mapControl` take literal field names.
- No SDK call changes a query's AQL after it is created, and no code wraps or rebinds SDK methods. SDK methods are called on the app (`app.createQuery(…)`), not destructured from it.
- An AQL literal contains no escape sequences, so its source text is its value. Use a backtick literal for AQL with quotes or line breaks.
- Every output column has an alias, so the row keys the app's JS reads survive the rewrite.
- A field is compared with a literal, not with a `const` holding one (`orders.status == 'completed'`, not `orders.status == k`). A value reached through a const isn't listed, so it can't be mapped.
- An alias never reuses the name of a field of the demo dataset. The rewrite tells an alias from a field by scope, and a shared name would be read differently by the v1 and v2 compilers.
- A query gives the same references under both AQL compilers ([v1 and v2](#v1-and-v2)), so it installs into a repo on either.
- Date ranges are relative to the demo data's latest date, not today.
- Each app queries one dataset. A `mapControl` field belongs to its target query's dataset, which static extraction can't follow through variables.

These rules apply to apps that are mapped. An app marked `"mapping": "none"` follows the opposite rule.

### Apps that need no mapping

Some apps name no field of the template's at all. A data explorer or a metric explorer lists the reader's datasets and fields at runtime (`Anfra.datasets`) and builds its queries from what the reader picks. Its interface is empty, so there is nothing to map, and installing it is a copy.

Static extraction can't tell such an app from one that hides a template field in JS, so the author says which it is, with `"mapping": "none"` on the app's entry in `apps`. For that app:

- `check` skips the literal rules, and fails instead if `app.extract` finds any literal `dataset`, `field` or `aql` argument, since that would be a template field left unmapped.
- `add` copies it byte for byte, with no Plan, and records it with an empty mapping, so `remove` and updates treat it like any other install.

A template can mix both kinds, such as mapped dashboards beside a generic explorer. An app that mixes literal queries with queries built at runtime isn't supported yet ([open questions](#open-questions)).

## Fetch

A reference is written three ways:

| Reference | Installs |
|---|---|
| `<id>` | The latest release tag. |
| `<id>@1.2.0` | That released version. |
| `<id>#<ref>` | A commit or branch, fetched from the repo directly. The escape hatch for authors. |

The default fetcher reads the index from a configurable URL (`https://anfra.ai/templates/index.json` by default). It caches the index under the anfra home folder and refetches it at most once a day. Offline, it uses the cached copy without complaint. One setting turns the fetch off, next to `ANFRA_NO_UPDATE_NOTIFIER`.

A release is a tarball whose URL and SHA-256 the index lists. The fetcher checks the checksum and refuses a mismatch. Neither path calls the GitHub API, so none hits its rate limits. A platform supplies its own fetcher: from its own cache, or from an offline bundle on premise.

A template's `min_anfra_version` newer than the running anfra fails the fetch, with a hint to run `anfra update`.

## Extraction and rewrite

This is the part the parser process provides. anfra-node already parses AQL, and runs in a JavaScript runtime that can parse JS, so it gets two read-only calls. Go splices their results.

1. `app.extract` lists each SDK call's literal arguments in an app, with their spans.
2. `aql.references` lists each field reference and compared literal in one query, with their spans.
3. Go replaces each referenced span with its target, from the end of the file backwards so earlier offsets stay valid.
4. Go validates each rewritten query against the user's dataset with `aql.validate`. Any failure fails the whole install.

Only referenced spans change, so the installed app keeps the template's formatting and comments.

### The parser calls

Both calls are pure functions of their input. They take no repo, read no files, keep no cache and generate no seeds, so they behave the same in every build and on every host, and need no repo id.

**Spans.** A span is `{ "start": n, "end": m }`: byte offsets into the input as UTF-8, start inclusive and end exclusive. Byte offsets, not JavaScript string indexes, because Go slices bytes. Input with any character outside ASCII would otherwise splice at the wrong place. Splicing every returned span with its own text must reproduce the input byte for byte.

**Problems.** Neither call fails on input it can't fully handle. It returns what it found, plus a `problems` list, each `{ "message", "line", "column" }` with 1-based positions. A column counts UTF-16 code units, as amql's diagnostics do. A JSON-RPC error means the call itself failed (bad params, a crash), never that the app or query was wrong.

#### `app.extract`

```json
// params
{ "html": "<!doctype html>…" }

// result (offsets illustrative)
{
  "calls": [
    {
      "method": "createQuery",
      "name": "revenue_by_month",
      "line": 42,
      "args": [
        { "arg": "dataset", "value": "demo_ecommerce", "span": { "start": 1530, "end": 1544 } },
        { "arg": "aql", "value": "explore { … }", "span": { "start": 1558, "end": 1702 } }
      ]
    },
    {
      "method": "mapControl",
      "line": 60,
      "args": [
        { "arg": "field", "value": "ecommerce_orders.status", "span": { "start": 2101, "end": 2124 } }
      ]
    }
  ],
  "problems": [
    { "message": "createQuery 'top_products': aql is not a literal", "line": 51, "column": 9 }
  ]
}
```

- It reads every inline `<script>` in the HTML, classic or module, finding them as an HTML tokenizer does. Nothing inside a comment or a raw-text element (`style`, `textarea`, `title`, `xmp`, `iframe`, `noembed`, `noframes`, `noscript`) is a script. Inside a script, a nested `<!--<script>…</script>` doesn't end it.
- A `<script src>`, and a data script (`application/json`, `application/ld+json`, `importmap`, `speculationrules`), are skipped without a problem. A script of any other non-JS type, such as `text/babel`, is a problem.
- A script that doesn't parse is a problem, and the others are still read. A comment, a `<script` tag or a script that is never closed is a problem. Nothing after an unclosed comment is read.
- A call is recognized by its method name and the number of its arguments, whatever the receiver is called: `app.createQuery(…)`, `a?.createQuery(…)`, `a['createQuery'](…)`. A call with the wrong number of arguments for the SDK's signature isn't the SDK's, and is skipped. A bare `createQuery(…)` isn't recognized, which the authoring rules cover.

| Method | Arguments returned |
|---|---|
| `createQuery(name, { dataset, aql })` | `dataset`, `aql` |
| `createFilter(name, { field, dataset? })` | `field`, and `dataset` when given |
| `mapControl(from, to, { field })` | `field` |
| `createDateDrill`, `mapCrossFilter` | None. Listed so a check can count them, and so a later version can add arguments without a new method. |

- `name` is the first argument when it is a string literal.
- A `span` covers the literal's contents, without its quotes or backticks.
- `value` is the spanned source text. Under the authoring rules it is also the literal's value, except that JS reads a CRLF in a template literal as LF. Keeping the source text keeps `aql.references`' offsets valid inside it.
- An argument the table names that is not a literal is a problem with its line, never silently skipped. That covers a variable, a call, a template literal with `${}`, and a literal containing any backslash, a line continuation included.
- A declaration that isn't an object literal is a problem. So is a spread, in the call's arguments or in the declaration; in the declaration, its explicit literal properties are still returned. A missing required key (`dataset` or `aql` in `createQuery`, `field` in `mapControl`) is a problem.
- A computed key (`{ ['field']: … }`) is a problem. So is a repeated key, and the last one is returned, since that is the one JS uses.
- More arguments than the method takes is a problem. So is fewer, when one of them is an object literal. Otherwise a call with too few arguments isn't the SDK's, and is skipped.
- A `createFilter` without `field` is a manual filter. It is listed with no arguments and no problem.
- Filter `default` and `options` values are not returned. `check` refuses them on a field that has mapped literals.

#### `aql.references`

```json
// params (options may be left out; a non-object options, or a non-boolean useV2Compiler, is an invalid_input error)
{ "options": { "useV2Compiler": true }, "aql": "explore {\n  dimensions { m: ecommerce_orders.created_at | month() }\n  measures { revenue }\n  filters { ecommerce_orders.status == 'completed' }\n}" }

// result
{
  "references": [
    { "kind": "field", "model": "ecommerce_orders", "name": "created_at", "span": { "start": 28, "end": 55 } },
    { "kind": "field", "name": "revenue", "span": { "start": 81, "end": 88 } },
    { "kind": "field", "model": "ecommerce_orders", "name": "status", "span": { "start": 103, "end": 126 } },
    { "kind": "literal", "value": "completed", "field": "ecommerce_orders.status", "span": { "start": 130, "end": 141 } }
  ],
  "problems": []
}
```

- A `field` reference is any name the query reads from its dataset: `model.field` (with `model` set), or a bare name, such as a dataset metric. Its span covers the whole reference, `ecommerce_orders.created_at`, so Go replaces it in one piece.
- A name the query itself defines, where the compiler can see it, is not a reference. That covers an alias, a `select` column, and a declaration (`metric`, `dimension`, `const`, `function`). A visible alias wins over a dataset field of the same name. An unaliased dimension binds no name: after `dimensions { orders.status }`, a bare `status` is read from the dataset, so it is a field. Names in `sorts` and query options are never references, since they name output columns and options.
- Function names, grains (`month()`, `interval(1 month)`, `'week'`) and keywords (`true`, `null`) are never references.
- A bare model name looks like a bare field, so it is a problem rather than a reference where it is one: when it also qualifies a field in the query (`orders.status`), as an argument of `of_all`, `exclude` or `keep`, as the first argument of an aggregation with two or more arguments, and at the head of a pipe into a table function (`orders | filter(…)`, `orders | count()`). Anywhere else, `check` catches it against the demo dataset.
- A `literal` is a string literal compared with a field reference by `==`, `!=`, `is`, `is not`, `in` or `not in`, with either side first, in an expression or a filter's params. `is null` and `is true` compare no string. Its `field` is that reference's full name, followed through an alias of a plain field (`s == 'a'` where `s: orders.status` gives `orders.status`). Its span includes the quotes, since Go writes a whole new literal.
- The call is syntactic. It does not resolve names against a dataset, because at install the template's demo dataset is not in the user's repo, and a platform reads repos by revision. Whether a bare name is a metric or a dimension, and its type, comes from the index, which is built where the demo dataset exists.
- AQL that doesn't parse returns no references and one problem. The parser has no error recovery.

A metric reference is told from a dimension by the index, not by this call. The difference matters only to the mapper's hard filter, which reads the index.

#### v1 and v2

A repo picks its AQL compiler in `.anfra/config.yml` (`aql.compiler.version`). Both compilers use the same parser, but their scopes differ:

- **v1 only:** an alias can be read by a later sibling in the same block, in `dimensions` (and from pivot `rows` into `columns`) and in `measures`. `measures { n: count(orders.id), m: n * 2 }` is valid in v1, and v2 reports that `n` is not defined.
- **Name clashes:** when an alias has the same name as a dataset metric, v2's `having` reads the metric and v1's reads the alias.
- **Constants:** a `const` body sees later metrics in v1, and only earlier declarations in v2.
- **Result filters:** a measure alias is visible only in a filter on the result, one that compares the bare alias. v2 looks for it through `and`, `or` and `not(…)`. v1 looks only at the filter's top.
- **Both:** a dimension alias is visible in measures, every filter and having, and a measure alias in having. An alias is never visible in its own definition, so in `revenue: revenue * 2` the inner `revenue` is the metric. A metric, dimension or function body sees every declaration in its program, and the query itself sees only those above it. A `select` column is visible to later pipe stages only. Params see no aliases.

So `aql.references` takes `options.useV2Compiler` (default `true`), as `aql.validate` does, and scopes as that compiler does. The installer passes the user's repo's setting. `check` runs every query under both and fails where the references differ, which with the alias rule keeps a template installable into a repo on either compiler.

### The rewrite

| Reference | Replaced with |
|---|---|
| A `dataset` argument | The mapped dataset's name. |
| A field reference, in AQL or a `field` argument | The target, as AQL text. A bare field name is written as it is. Anything else is wrapped in parentheses, `(net_revenue / orders_count)`. A `field` argument only takes a field name, so an expression target there is refused at `Apply`. |
| A literal | The mapped value, as an AQL string literal in the template's quote style. An unmapped literal is left as it is. |

The rewrite runs in two passes. First Go rewrites each AQL value from its own references. Then it splices the rewritten values and the other arguments into the HTML. Both passes splice from the last span to the first.

## Plan

`Plan(template, catalog)` recommends a mapping. It is deterministic and uses no LLM.

1. **Datasets first.** Each user dataset is ranked by how many of the template's fields it can cover, for example "`sales`: 9 of 11 fields".
2. **Hard filter.** Kind and type must agree: a metric maps to a metric, a date dimension to a date or datetime dimension, text to text, number to number.
3. **Score each candidate** on its name and label tokens against the field's name and `synonyms`, on a metric's aggregation and column type, on description and `ai_context` overlap, and, for a field with mapped literals, on how many of the user's top distinct values match.
4. **Compose derived metrics.** A template metric defined in the demo dataset as `revenue / total_orders` is recommended as `(net_revenue) / (orders_count)` once both parts are mapped. The user's own field wins if it scores well.
5. **Match values** by string similarity and the authored meaning. An unmatched literal is flagged, never guessed.

The result lists, per field, the top three candidates with their scores and the reason for each ("synonym match: net sales"), and the fields left unmatched. It serializes as the `--plan --json` output. The weights are tuned on a set of realistic user datasets kept with the tests.

## Apply

`Apply(template, mapping)` returns a changeset: the rewritten files, the resources to add (phase 1 has none outside `--demo`), and the record entry. It writes nothing.

- Every typed artifact has a handler that extracts its references and rewrites them. Phase 1 ships the app handler. A later phase adds AML, with an `aml.references` call.
- The template's interface is the union of its handlers' references, less those the template satisfies itself.
- Every field in the interface needs a target. A missing one fails `Apply` and names the field.
- An app marked `"mapping": "none"` is copied as it is, whatever the mapping.
- A rewritten query that fails `aql.validate` fails `Apply`, naming the app, the query and the mapped fields it uses.
- Same template, commit and mapping give the same bytes. The record's file hashes depend on it, and so does re-mapping.

## Install record

`.anfra/templates.yml`, written by the installer:

```yaml
installs:
  apps/ecommerce-overview:
    template: <owner>/anfra-templates/ecommerce-overview
    version: 1.2.0
    commit: 3f2a9c1
    mapping:
      demo_ecommerce:
        dataset: sales
        fields:
          ecommerce_orders.created_at: orders.placed_at
          revenue: net_revenue
          aov: net_revenue / orders_count
        values:
          ecommerce_orders.status:
            completed: delivered
    files:
      apps/ecommerce-overview/overview.html: sha256:9c41e0b7…
      apps/ecommerce-overview/retention.html: sha256:51d2a8f0…
```

- Entries are keyed by install folder, so one template can be installed twice. The writer keeps entries and keys in a stable order, so `anfra pull` merges two members' installs as text.
- A field target is AQL text. A `--demo` or `--no-map` install has an empty `mapping`.
- `repoformat.Publishable` accepts the file, like `.anfra/config.yml`, so it publishes with the repo. Unlike `config.yml`, changing it needs no Admin-only permission.
- `anfra validate` warns, without failing, on an entry whose files are missing or moved.

## Commands

| Command | Does |
|---|---|
| `anfra template add <ref> [--into DIR] [--demo] [--no-map] [--plan --json] [--dataset …] [--field …] [--value …] [--mapping FILE] [--yes]` | Fetch, plan, confirm, apply and write. |
| `anfra template map <install> [mapping flags]` | Re-plan from the recorded mapping and re-apply. |
| `anfra template remove <install>` | Delete unedited files and the entry. |
| `anfra template check <dir>` | Check a template before it is published. |
| `anfra template index <repo>` | Build `index.json` for a templates repo. |

**add.** In a terminal, it asks for the dataset (ranked by coverage), then each field (top three, the best preselected, unmatched fields first, or an AQL expression typed in), then each value. `--plan --json` prints the plan and writes nothing. That is the agent's way in: the `anfra-add-template` skill reads the plan, settles weak matches with the user, and calls `add` again with flags. `--dataset`, `--field`, `--value` and `--mapping` supply a mapping. `--yes` accepts every match above a confidence threshold, and fails listing the fields below it.

```sh
anfra template add <owner>/anfra-templates/ecommerce-overview \
  --dataset demo_ecommerce=sales \
  --field revenue=net_revenue \
  --field ecommerce_orders.created_at=orders.placed_at \
  --field 'aov=net_revenue / orders_count' \
  --value ecommerce_orders.status:completed=delivered
```

It writes the apps to `apps/<template>/`. It never overwrites or renames anything: an existing folder fails the install, with a hint to use `--into`.

**--demo** installs the apps unmapped, plus the template's demo data template if the repo doesn't have it yet. The demo's files are copied as they are, and its DuckDB data source is added to the local config with an absolute path. A cloud org doesn't accept DuckDB, so the CLI warns that a demo install must be mapped before `anfra publish`.

**map** replays the rewrite. The record keeps the confirmed mapping, so the base can be recomputed instead of stored:

| Side | Files |
|---|---|
| base | `Apply(recorded commit, recorded mapping)` |
| ours | The files in the repo |
| theirs | `Apply(recorded commit, new mapping)` |

[treemerge](treemerge.md) merges the three, as `anfra pull` does. An unedited file (its hash matches the record) is replaced. An edited one is merged, with conflict markers where the edits overlap. Mapped fields never show as user edits, because the base carries the old rewrite too. Updating to a new version, in phase 2, is the same merge with the new commit as theirs.

**remove** deletes each file whose hash still matches the record, and the entry. An edited file is kept, with a warning naming it.

**check** fails, naming the file and line or the field, when:

- `manifest.json` is invalid or lists missing files, `demo` doesn't resolve, or `preview.png` is missing;
- `network` isn't empty;
- `app.extract` returns a problem, or, for an app marked `"mapping": "none"`, returns any literal `dataset`, `field` or `aql` argument;
- `aql.references` gives different references under v1 and v2, or an alias reuses the name of a field of the demo dataset;
- a query has an output column without an alias, or doesn't validate against the demo dataset;
- a metric the apps use has no `description`, or `fields` describes a field no query uses;
- in a pull request, the template's files changed but `version` didn't, or `version` isn't greater than its latest tag.

**index** writes `index.json`: each template's manifest keys, its `featured` rank, latest tag, `updated_at`, and a tarball URL and checksum per release. Per field it derives the name, kind, type, a metric's definition in the demo dataset, the time grains used, the literals filtered on, the queries and apps that use it, and sample values from the demo data, then merges in the authored meaning. A JSON schema for it is committed with its tests.

## Build and release order

The parser calls ship in an anfra-node release before core can use them. A sidecar's wire contract is versioned with anfra ([sidecars](sidecars.md#where-the-binaries-come-from)), so:

1. anfra-node adds `app.extract` and `aql.references` (in review), and cuts a release.
2. Core bumps `manifest.yml`'s anfra-node pin, and adds `AppExtract` and `AQLReferences` to its client, with tests against the real binary. Until the release, those tests run against a local build through `ANFRA_NODE_BIN`.
3. In parallel, nothing needing the parser: the install record and its `Publishable` change.
4. `shared/templates`: Fetch and Apply with the app handler, then Plan.
5. The commands, then `--demo`, then the skill and the user docs.

## Open questions

1. **Apps that mix literal and runtime-built queries.** A marker for AQL fragments (``Anfra.aql`revenue` ``) would let an app compose queries in JS while still telling the installer which strings are AQL. `app.extract` would return each marked fragment, and `aql.references` would parse it as an expression. Wait for a template that needs it.
2. **Mapper weights and the `--yes` threshold.** These need a set of realistic user datasets to tune on.
