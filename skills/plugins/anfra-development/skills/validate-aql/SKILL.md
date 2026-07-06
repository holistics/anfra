---
name: validate-aql
description: Validate an AQL query (or a single expression) without running it. Use to type-check AQL before running or presenting it.
---

# Validating AQL

## Command (swappable per scenario)
Default (Anfra) — type-check without running:

```sh
anfra validate --dataset <uname> --aql '<query>'
# or
cat query.aql | anfra validate --dataset <uname> --aql
```

You can validate a **single expression** (a metric, a filter, one calculation) —
not only a whole `explore`.

> Replace this command for other runtimes if needed.

## Writing valid queries
Use [](../aql/) (syntax, functions, gotchas) and [](../write-aql/) (workflow) to
write correct AQL and to fix whatever validation reports.