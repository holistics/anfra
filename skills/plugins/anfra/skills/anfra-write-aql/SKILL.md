---
name: anfra-write-aql
description: Write AQL queries to answer data questions. Use whenever the user asks for data, wants to query a dataset, needs to filter/aggregate/analyze data, or asks about metrics and dimensions in AMQL.
---

# Writing AQL to answer a question

Goal: turn the user's analytics request into a correct AQL query, verify it, and
run it.

> **AQL is not SQL.**  Writing it from SQL habits or memory produces wrong queries and burns validate→fix cycles.
> **Read the [](../anfra-aql/) references _before_ you write — not after you fail.**
> Looking things up first is faster than guessing and retrying.

## Sub-agent or inline?
Prefer writing the AQL yourself. You can instead hand it to a sub-agent — an `aql-writer` sub-agent if available, otherwise a generic one told to use the `anfra-aql`, `anfra-write-aql`, and `anfra-validate-aql` skills. It does the lookups, writes and validates, and returns only the final AQL. Decide per task:
* **Do it inline** (the default) when you've already read the AQL references this session (a sub-agent re-reads them every time it starts), when it's a small edit to an existing query, or when the work needs back-and-forth with the user or depends on surrounding decisions.
* **Delegate** only when several independent queries can be written in parallel, or when the session is long and keeping the AQL references out of your context matters.
* **Always inline** when no sub-agent is available (e.g. Codex), or when you *are* the sub-agent.

## Prerequisites
* Work inside an AMQL repo. Discover the available datasets/models/fields — that's your `dataset_fields`. Use only those models and fields, with their **EXACT** names.
* Have `anfra` available to validate and run queries.

## Workflow
1. **Understand the request** against the available fields. If you genuinely can't (no matching fields, ambiguous business logic, or it's not an analytic), ask the user to clarify instead of guessing.
2. **Read the AQL references first — required, before writing anything.** Don't write AQL from memory, SQL intuition, or by copying queries found in the project. Load the [](../anfra-aql/) skill (the files below live in its directory), then:
   * Read `references/aqlearn.md` (the core lessons/rules) if you haven't yet this session.
   * Look up the exact functions/operators you'll use in `references/aql/`.
   * For nested aggregation, top-N-per-group, period comparison, level-of-detail, cohort retention, or ranking — study the worked examples and gotchas in `references/examples/`. Always check the `[silent]` gotchas: they produce wrong-but-valid AQL with **no** validation error, so validation won't catch them for you.
3. **Verify filter values before using them.** If the request filters on values you're not certain match the stored data (case, abbreviations, typos), use [](../anfra-lookup-values/) first. Never assume filter literals are exact.
4. **Inspect opaque fields.** For structs / JSON / "settings"-type fields, never guess the structure — run a preview query to see a sample, then write the real query using the observed shape.
5. **Write a single `explore`** using what you read in step 2.
6. **Validate** with [](../anfra-validate-aql/) and fix any errors.
7. **Run** it with [](../anfra-run-aql/) and answer from the result.

## Core principles
* Think **metric-centric**, not table-centric (SQL). Do **NOT** write joins — AQL joins automatically through relationships.
* Prefer **existing** dimensions/measures; don't redeclare them. Never invent models, fields, functions, or arguments; use documented ones with the correct argument order.
* Give human-readable **snake_case** names to explore dimensions/measures, always add `sorts`, and **narrow the result** to exactly what's asked (if the user asks for a total, return only the total — not raw rows).  * Prefer native time functions (`running_total`, `relative_period`, `period_to_date`, …) for period comparisons.
* If a feature seems missing or you hit an error, assume it's a knowledge gap (wrong function/args) — check the [](../anfra-aql/) references — not an AQL bug.

## Related skills
* [](../anfra-aql/) — AQL knowledge and reference docs.
* [](../anfra-lookup-values/) — verify exact filter values before filtering.
* [](../anfra-validate-aql/) — validate a query or expression before running it.
* [](../anfra-run-aql/) — run a validated query and get its result.
