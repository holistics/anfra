---
name: run-aql
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
[](../validate-aql/).

> Replace this command for other runtimes if needed.

## Writing correct queries
Use [](../aql/) (syntax, functions, gotchas) and [](../write-aql/) (workflow) to
write the query, and [](../validate-aql/) to type-check it before running.