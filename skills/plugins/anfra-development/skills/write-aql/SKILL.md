---
name: write-aql
description: Write AQL queries to answer data questions. Use whenever the user asks for data, wants to query a dataset, needs to filter/aggregate/analyze data, or asks about metrics and dimensions in AMQL.
---

# Writing AQL to answer a question

Goal: turn the user's analytics request into a correct AQL query, verify it, and
run it. Lean on the [](../aql/) skill for syntax, examples, and gotchas.

## Prerequisites
* Work inside an AMQL repo. Discover the available datasets/models/fields — that's your `dataset_fields`. Use only those models and fields, with their **EXACT** names.
* Have `anfra` available to validate and run queries.

## Workflow
1. **Understand the request** against the available fields. If you genuinely
   can't (no matching fields, ambiguous business logic, or it's not an analytic),
   ask the user to clarify instead of guessing.
2. **Verify filter values before using them.** If the request filters on values
   you're not certain match the stored data (case, abbreviations, typos), use
   [](../lookup-values/) first. Never assume filter literals are exact.
3. **Inspect opaque fields.** For structs / JSON / "settings"-type fields, never
   guess the structure — run a preview query to see a sample, then write the real
   query using the observed shape.
4. **Write a single `explore`**, consulting [](../aql/) references as needed.
5. **Validate** expressions with [](../validate-aql/) if needed.
6. **Run** it with `anfra query` and answer from the result.

## Reach for examples before writing
Before writing AQL for: nested aggregation, top-N-per-group, period comparison,
level-of-detail calculations, cohort retention, or ranking — and any time you
suspect a silent failure — look up worked examples and gotchas via the
[](../aql/) skill (`references/examples/`). The gotchas marked `[silent]` produce
wrong-but-valid AQL with no validation error; check them proactively.

## Core principles
* Think **metric-centric**, not table-centric (SQL). Do **NOT** write joins — AQL
  joins automatically through relationships.
* Prefer **existing** dimensions/measures; don't redeclare them. Never invent
  models, fields, functions, or arguments; use documented ones with the correct
  argument order.
* Give human-readable **snake_case** names to explore dimensions/measures, always
  add `sorts`, and **narrow the result** to exactly what's asked (if the user
  asks for a total, return only the total — not raw rows).
* Prefer native time functions (`running_total`, `relative_period`,
  `period_to_date`, …) for period comparisons.
* If a feature seems missing or you hit an error, assume it's a knowledge gap
  (wrong function/args) — check the [](../aql/) references — not an AQL bug.

## Related skills
* [](../aql/) — AQL knowledge and reference docs.
* [](../lookup-values/) — verify exact filter values before filtering.
* [](../validate-aql/) — validate a query or expression before running it.