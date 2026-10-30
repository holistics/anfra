---
name: anfra-run-aql
description: Run an AQL query against a dataset and return its result rows (or compile it to SQL). Use to execute a validated AQL query and get data.
---

# Running AQL

## Command (swappable per scenario)
Default (Anfra) — compile and run against a dataset:

```sh
anfra query --dataset <uname> '<query>'
# or
cat query.aql | anfra query --dataset <uname>

# the compiled SQL, without running it
anfra query compile --dataset <uname> '<query>'
```

An invalid query fails (exit 1) and prints its diagnostics; fix it with
[](../anfra-validate-aql/).

## How many rows come back
`anfra query` (and the MCP query tool) answers one page: the first 10,000 rows,
or `--page-size` rows of `--page`. `has_more: true` means rows were left out, so
read it before treating a result as complete, and never report a count, a sum or
"all rows" from a page that has more.

This cap is the command's, not the data's or anfra's: a Data App's query gets
every row unless it declares a `pageSize`, and `anfra query export` writes the
whole result to a file. To answer a question, ask the query for what you need
(aggregate, filter, `limit:`) rather than fetching rows to count them.

> Replace this command for other runtimes if needed.

## Writing correct queries
Use [](../anfra-aql/) (syntax, functions, gotchas) and [](../anfra-write-aql/) (workflow) to
write the query, and [](../anfra-validate-aql/) to type-check it before running.
