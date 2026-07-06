---
name: aml
description: Write and edit AML — the declarative modeling language for models, fields, relationships, and datasets in an AMQL repo. Use whenever you need to create or change a model, dataset, relationship, or field definition.
---

# AML modeling

AML is a declarative language for defining **semantic models**: the models,
fields, relationships, and datasets that AQL queries run against.

Files use type-suffixed names: `*.model.aml` (a model), `*.dataset.aml` (a
dataset), and plain `*.aml` for shared definitions. Edit these files directly in
the repo.

## References (progressive disclosure)
Load only what the task needs. **Never assume AML syntax or properties — check
the type definition.**

1. **`references/type_definitions/*.aml`** — the schema for each AML block (its
   properties, types, defaults). Open the one you're working with:
   * `data_model.aml` — models (`TableModel`, `QueryModel`) and their fields (`dimension`, `measure`, `param`), field types, and how to reference fields with `r(...)` / `ref(...)`.
   * `dataset.aml` — datasets: which models + relationships they bundle, plus dataset-level `dimension` and `metric` fields.
   * `relationship.aml` — relationships between models (`many_to_one`, `one_to_one`) and `rel_config`.
   * `persistence.aml` — materializing a query model as a table (full / incremental / external).
   * `pre_aggregate.aml` — pre-aggregated tables for performance.
   * `format.aml` — the `Format` type used by a field's `format` property.
2. **`references/examples/*.aml`** — complete worked files to pattern-match against: `ecommerce_orders.model.aml` (a model) and `demo_ecommerce.dataset.aml` (a dataset).

## Core concepts
* **Models** define data. A `TableModel` (`type: 'table'`) maps to a table; a `QueryModel` (`type: 'query'`) maps to a SQL query. Each has `dimension`s (attributes), `measure`s (aggregations), and optional `param`s.
* **Reference fields with `r(...)`** — e.g. `r(users.country)` (model field), `r(ecommerce.revenue)` (dataset metric). `r(...)` is compile-time typechecked; prefer it over the legacy `ref('users','country')`.
* **Relationships** connect models (`from` a many-side field `to` a one-side field). AQL uses them to join automatically — you never write joins in queries.
* **Datasets** bundle models + relationships and expose dataset-level `metric`s and `dimension`s. A dataset is what AQL queries target.
* **`data_source_name`** on a model/dataset names a source configured in the repo's `.anfra/data_sources.yml` — data sources aren't defined in AML here.

## Best practices
* **Always define a model's primary key** — mark the identifying `dimension` with `primary_key: true`. Relationships and correct aggregation rely on it.
* **Prefer AQL definitions**; only fall back to raw SQL when referencing #SOURCE columns.
* Define relationships between the models you query so AQL can join them automatically — never hand-write joins.

## Workflow
1. **Model first.** Create/extend the `*.model.aml`: set `type`, `data_source_name`, `table_name` (or `query`), and define its `dimension`s and `measure`s. Check `data_model.aml` and the model example for exact syntax.
2. **Relationships.** Define relationships between the models you'll join.
3. **Dataset.** Create/extend the `*.dataset.aml`: list the `models` and `relationships`, and add any cross-model dataset-level `metric`s.
4. **Validate** with [](../validate-aql/) — run `anfra validate` (whole repo, or specific files) and fix any diagnostics.
5. **Query** the dataset with [](../write-aql/).

## Related skills
* [](../aql/) — AQL knowledge (field/metric definitions are AQL).
* [](../write-aql/) — write and run queries against a dataset.
* [](../validate-aql/) — validate AML/AQL before relying on it.
