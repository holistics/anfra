---
name: anfra-convert-looker
description: Convert LookML (views, explores, model files, manifest) into Holistics AML models and datasets with AQL. Use when migrating a Looker project to Holistics or translating a LookML file or snippet into AML.
---

# LookML → Holistics AML / AQL

Convert the LookML semantic layer into AML. Dashboards are out of scope.

Each reference follows the parameter list of the matching Looker reference page. For every parameter it gives either
the conversion instruction with an example, or "Unsupported". When a parameter is unsupported, drop it and list it in
the conversion report (see "The conversion report" below); never invent an AML property for it.

| LookML | Reference |
|---|---|
| model file (`*.model.lkml`) | [references/model.md](references/model.md) |
| `view` | [references/view.md](references/view.md) |
| `derived_table` (SQL and native derived tables, persistence, schedules) | [references/derived-tables.md](references/derived-tables.md) |
| `explore`, `join` | [references/explore.md](references/explore.md) |
| `aggregate_table` (aggregate awareness) | [references/aggregate-awareness.md](references/aggregate-awareness.md) |
| field parameters shared by all fields (labels, formats, drills, …) | [references/field.md](references/field.md) |
| `dimension` | [references/dimension.md](references/dimension.md) |
| `dimension_group` | [references/dimension-group.md](references/dimension-group.md) |
| `measure` | [references/measure.md](references/measure.md) |
| `filter` | [references/filter.md](references/filter.md) |
| `parameter` | [references/parameter.md](references/parameter.md) |
| Liquid (`{% … %}`, `{{ … }}`): user attributes, `_in_query`, `{% if %}`, Liquid in `html` / `link` / `label`. Only a few uses convert; start here for any Liquid that is not a `parameter` or `filter` read | [references/liquid.md](references/liquid.md) |
| references inside `sql`: `${TABLE}`, `${field}`, `${view.field}`, `${view.SQL_TABLE_NAME}`, `@{constant}` | [references/sql-references.md](references/sql-references.md) |
| filter expressions (`"7 days"`, `"-NULL"`, `">=18 AND <=25"`): the values of `filters`, `always_filter`, `access_filter`, a filter's `default_value` | [references/filter-expressions.md](references/filter-expressions.md) |
| project manifest (`manifest.lkml`) | [references/manifest.md](references/manifest.md) |
| `extends`, `extension`, refinements (`view: +name`, `explore: +name`), `final`, imported projects | [references/extends-and-refinements.md](references/extends-and-refinements.md) |

## Workflow

For a whole project (or any set of `.lkml` files), do the conversion in two passes.

**Pass 1 — script.** It converts the structure and everything the references call mechanical. It needs only Node;
there is nothing to install.

```sh
node scripts/convert.mjs <lookml dir or file> --out <aml dir> \
  [--data-source <lookml connection>=<holistics data source>]… [--default-data-source <name>] \
  [--persist-schema <schema>] [--force]
```

It writes `manifest.aml`, `models/**/<view>.model.aml`, `models/aliases/*.model.aml`,
`datasets/<model file>/<explore>_dataset.dataset.aml`, `schedules.aml` and `work-items.json`. It writes no report.
Views and explores that no model file includes are converted under a model named `unattached__`.

**Pass 2 — you.** The script leaves two kinds of comments in the `.aml` files, each tagged with a kind
(`TODO(lookml-to-aml:aql-ref-in-sql)`, `REVIEW(lookml-to-aml:yesno-column)`; kinds are listed in
`scripts/lib/kinds.mjs`):

- `TODO(lookml-to-aml:<kind>)`: not converted. The comment quotes the original LookML and names the reference to
  follow. The code below it is a stub that only keeps the project compiling (`@sql NULL`, `@aql null`,
  `SELECT 1 /* placeholder */`, or a commented-out skeleton). Replace the stub.
- `REVIEW(lookml-to-aml:<kind>)`: converted, but the result may differ from the LookML. Check it against the reference.

`work-items.json` holds the same comments as data, plus what has no place in the AML. It is large: query it with
`node -e` or `jq` instead of opening it whole.

- `items`: one per comment, with its file, line, kind, AML field, source `.lkml` file and reference. Line numbers go
  stale once you edit a file; item ids do not.
- `kinds`: count, number of files and reference per kind.
- `files`: per AML file, its source, the models a Dataset uses (`models`), the Datasets that use a model (`usedBy`),
  and its near-copy family.
- `families`: model files that declare mostly the same fields (copy-pasted views).
- `notes`: what the script reported per object. `SECURITY`: access rules with no AML form; a person must replace them.
  `INFO`: work outside the AML code (dashboard default filters, Quick Start queries, settings, admin tasks). `DROPPED`:
  no AML form, with the original LookML when it may be re-created elsewhere. `TODO` / `REVIEW`: most repeat a
  comment, some have none (a view without a primary key, an explore declared twice).
- `droppedParams`: field parameters with no AML form, with the fields they were dropped from.

Steps:

1. Read the summary of `work-items.json` (the script also prints the kinds): `totals`, `kinds`, `families`, then the
   `notes` by level, `SECURITY` with care: those access rules have no AML form and a person must replace them.

   ```sh
   node -e 'const w=require(process.argv[1]); console.log(w.totals, w.kinds, w.families);
     for (const n of w.notes) if (n.level !== "TODO" && n.level !== "REVIEW") console.log(n.level, n.kind, "|", n.where, "|", n.message)' <aml dir>/work-items.json
   ```
2. Fix every `parse-error` TODO before anything else, and re-run the script (`--force`). Work done on the output of a
   file that failed to parse is overwritten.
3. `grep -rn 'TODO(lookml-to-aml' <aml dir>`, then resolve each comment with the reference it names. Delete the
   comment when it is resolved. If the construct is unsupported, delete the stub and note it for the conversion report.
4. Do the same for `REVIEW(lookml-to-aml`.
5. Run `anfra validate` and fix what it reports. The script's output is not guaranteed to validate: for
   example a moved cross-view field can cause a fan-out or a missing-path error that needs a judgement.
6. The LookML quoted in comments is re-printed from the parsed file, so comments and original formatting are lost.
   Open the source `.lkml` file when a comment is not enough.

For a project with more than about 40 TODO comments or 20 files with comments, do steps 3–5 as in
"Pass 2 with subagents" below.

For a single snippet, skip the script and convert by hand from the references.

Validate the AML you produce with `anfra validate`. Validation proves that the code compiles, not that it
means the same thing as the LookML.

## Pass 2 with subagents

On a large project, one agent runs out of context, and agents that each take a slice of files repeat the same
decisions, often in different ways. Split the work as follows. You stay the coordinator: you decide, subagents apply.
If your runtime has no subagents, follow the same phases yourself, one file set at a time.

### Phase 0 — you, before any subagent

1. Fix parse errors and re-run the script (step 2 above).
2. **Decide once per repeated kind.** For each entry of `kinds` with more than about 10 comments or more than 3 files
   (typical: `quoted-identifier`, `yesno-column`, `untyped-column`, `dev-prod-table`, `helper-dim-type`,
   `duplicate-metric`), read the reference and two or three instances, then write the decision in
   `<aml dir>/decisions.md`: the kind, the rule you apply, why (cite the reference), and the exceptions that need a
   case-by-case look. Ask the user what only they know: warehouse column types, the dev/prod schema names, primary
   keys of views without one (`no-pk`), the persistence schema.
3. **Apply the decisions in bulk** where the rule is mechanical: a script or `sed` over
   `grep -rln '<LEVEL>(lookml-to-aml:<kind>)'`, deleting the comments it resolves. Leave the exceptions for phase 1.
4. Keep the `SECURITY` and `INFO` notes for the conversion report. Do not delegate them.
5. Run `anfra validate` and note the errors that exist before phase 1.

### Phase 1 — subagents on model files

- Split the model files that still have comments into disjoint groups of about 5–15 files, by directory. Put every
  file of a `families` entry in the same group. Each file belongs to exactly one subagent.
- For a family, have its subagent resolve one file first and apply the same fix to the others, adapting names.
- Run the subagents in parallel. They must not edit Dataset files or files of another group.

### Phase 2 — you, between the phases

Validate. Merge the subagents' `field_changes` (fields renamed, added, removed or retyped) and send each one to the
Datasets that use that model (`usedBy`). Settle open questions with the user. Add new repeated patterns to
`decisions.md`.

### Phase 3 — subagents on Dataset files

- Start only after phase 2. One subagent per Dataset file, or a few small ones together.
- A Dataset subagent edits only its Dataset files. A change it needs in a model (a primary key, a helper dimension, a
  relationship key) comes back as a `model_change_requests` entry; you apply it, since several Datasets may share the
  model.
- The validation errors of a converted project are mostly here: fan-out (E104) and missing relationship path (E106)
  from moved cross-view fields and joins that were not converted.

### Phase 4 — you, at the end

Apply the model change requests. Run `anfra validate` until it passes. Check that
`grep -rn 'lookml-to-aml:' <aml dir>` lists only what you chose to keep, and write the conversion report.

Optionally, before phase 4, have one fresh subagent per group check meaning rather than compilation: give it the
`.lkml` files and the `.aml` files, not the earlier subagent's reasoning, and ask where the AML can return different
numbers or rows than Looker.

### The subagent brief

A subagent knows only what the brief says. Include:

```
Goal: finish the LookML → Holistics AML conversion of the files below. Pass 1 (a script) left
TODO(lookml-to-aml:<kind>) and REVIEW(lookml-to-aml:<kind>) comments in them.

Files you own (edit only these):        <absolute paths of .aml files>
Source LookML (read only):               <absolute paths of the matching .lkml files>
Conversion rules:                        <absolute path of this skill>/references/ (each comment names its section)
Decisions already made (follow them, do not re-decide):  <aml dir>/decisions.md
[Family: resolve <file> first, then apply the same fix to <files>.]
[Field changes in models this Dataset uses: <list from phase 2>]

For each comment in your files: resolve it with the reference it names, replace the stub, delete the comment.
If the construct is unsupported, delete the stub and report it. Never invent an AML property.
Validate with `anfra validate <your files>` run in <aml dir>; other agents are editing other files at the same
time, so fix only errors in your files. Run `anfra validate` without files at the end, and report errors elsewhere
that look caused by your files.

Reply with only:
resolved:              <file> <field> — one line each
unsupported:           <file> <field> — reason, closest workaround
field_changes:         <model>.<field> renamed / added / removed / retyped — what     (model files)
model_change_requests: <model>.<field> — what and why                                 (Dataset files)
open_questions:        questions for the user, with the file and field
```

Keep the reply format: you merge many of them, and a free-form report hides the field changes that the next phase
depends on.

## The conversion report

When pass 2 is done, write `<aml dir>/conversion-report.md` for the person who takes over the project. The references
say "list it in the conversion report" for whatever has no AML form; it goes here. Write it from the `notes` and
`droppedParams` of `work-items.json`, `decisions.md`, and what you and the subagents marked unsupported:

1. **Security**: every `SECURITY` note, and what the person must set up instead (dataset sharing, column-level or
   row-level permission).
2. **Outside the AML code**: the `INFO` notes, grouped by task (dashboard default filters, Quick Start queries to
   rebuild, data source and schema settings, user attributes, cache durations).
3. **Not converted**: what was unsupported, with the closest workaround and the original LookML (from `DROPPED` notes
   and your own findings).
4. **Behaves differently**: the decisions and REVIEW points where numbers or rows can differ from Looker.
5. **Open questions** for the user.

Leave out what needs no action (`droppedParams` such as `alias: not needed`); give counts instead.
