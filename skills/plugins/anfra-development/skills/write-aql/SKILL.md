---
name: write-aql
description: Write AQL queries to answer data questions. Use whenever the user asks for data, wants to query a dataset, needs to filter/aggregate/analyze data, or asks about metrics and dimensions in AMQL.
---

# Writing AQL to answer a question

Goal: turn the user's analytics request into a correct AQL query, verify it, and
run it.

> **AQL is not SQL.**  Writing it from SQL habits or memory produces wrong queries and burns validate→fix cycles.
> **Read the [](../aql/) references _before_ you write — not after you fail.**
> Looking things up first is faster than guessing and retrying.

> **Prefer delegating to a sub-agent (to save context).** Reading the AQL references consumes context. If an `aql-writer` sub-agent is available, hand the request to it and let it do the lookups + write/validate/run — it returns only the final AQL and result, keeping your main context lean. Otherwise, if your runtime has a generic task/subagent tool, spawn one and tell it to use the `aql`, `write-aql`, and `validate-aql` skills. (If you *are* the `aql-writer` sub-agent, ignore this and do the work yourself.)

## Prerequisites
* Work inside an AMQL repo. Discover the available datasets/models/fields — that's your `dataset_fields`. Use only those models and fields, with their **EXACT** names.
* Have `anfra` available to validate and run queries.

## Workflow
1. **Understand the request** against the available fields. If you genuinely can't (no matching fields, ambiguous business logic, or it's not an analytic), ask the user to clarify instead of guessing.
2. **Read the AQL references first — required, before writing anything.** Don't write AQL from memory or SQL intuition. Via the [](../aql/) skill:
   * Read `references/aqlearn.md` (the core lessons/rules) if you haven't yet this session.
   * Look up the exact functions/operators you'll use in `references/aql/`.
   * For nested aggregation, top-N-per-group, period comparison, level-of-detail, cohort retention, or ranking — study the worked examples and gotchas in `references/examples/`. Always check the `[silent]` gotchas: they produce wrong-but-valid AQL with **no** validation error, so validation won't catch them for you.
3. **Verify filter values before using them.** If the request filters on values you're not certain match the stored data (case, abbreviations, typos), use [](../lookup-values/) first. Never assume filter literals are exact.
4. **Inspect opaque fields.** For structs / JSON / "settings"-type fields, never guess the structure — run a preview query to see a sample, then write the real query using the observed shape.
5. **Write a single `explore`** using what you read in step 2.
6. **Validate** with [](../validate-aql/) and fix any errors.
7. **Run** it with `anfra query` and answer from the result.

## Core principles
* Think **metric-centric**, not table-centric (SQL). Do **NOT** write joins — AQL joins automatically through relationships.
* Prefer **existing** dimensions/measures; don't redeclare them. Never invent models, fields, functions, or arguments; use documented ones with the correct argument order.
* Give human-readable **snake_case** names to explore dimensions/measures, always add `sorts`, and **narrow the result** to exactly what's asked (if the user asks for a total, return only the total — not raw rows).  * Prefer native time functions (`running_total`, `relative_period`, `period_to_date`, …) for period comparisons.
* If a feature seems missing or you hit an error, assume it's a knowledge gap (wrong function/args) — check the [](../aql/) references — not an AQL bug.

## Related skills
* [](../aql/) — AQL knowledge and reference docs.
* [](../lookup-values/) — verify exact filter values before filtering.
* [](../validate-aql/) — validate a query or expression before running it.