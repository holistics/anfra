# CLI pretty output

*Being designed. Not built yet, except where noted. Not exposed: there's no `--pretty` flag yet.*

How the CLI shows a command's structured output to a person at a terminal: laid out for reading, from the same result `--yaml` and `--json` write, so the two can't drift. [CLI output](commands-and-api.md#cli-output) covers the structured formats; this doc covers the pretty one, and the prettifier that makes it.

## Where it stands

- **Formats.** A command's output is an artifact (a log, a script) or structured. Structured output comes as YAML, JSON, or pretty.
- **Defaults.** A core command writes YAML. A CLI-only command writes the pretty format by default, through the prettifier, since people run them more than programs do: `update`, `agent setup`, `init`, `publish` and `auth`. `--yaml` or `--json` on any command gives structured output.
- **`--pretty`** asks for the pretty format, from a command with a pretty view: every CLI-only command, and of the core commands, those whose view is reviewed (`prettyViews`, `cmd/anfra/output.go`): `query` so far. Any other refuses it, as it does beside `--yaml`, `--json` or `--jq`. A core command's default stays YAML, outside a console. Whether pretty becomes the default on an interactive terminal for every command waits for the prettifier to show every one well.
- **In [`anfra console`](console.md)**, output is for a person, so **every** command writes the pretty format by default, a core command whose view isn't reviewed too, by the conventions: a trial, ahead of "one command at a time" below, to see where the conventions fall short. `openapi`, a document, stays YAML. `--yaml` or `--json`, an entry's or the session's, still wins. Outside a console, `--pretty` stays a reviewed view's.
- **Errors and warnings, for a person** (in a console, or with `--pretty`), are pretty too, still on stderr: an error a red `✗` and its message, its chain of causes a part a line, each indented under the one it caused, its colon kept (split at `: `, a part of fewer than 4 words staying with the one before, as `apps/a.html: line 3`); the parts before the last faint, the last, the root cause, plain, what's read; a blank line after a chain; then its code, its details by the conventions (violations a table, in the envelope's order) and its help; its scope, status and an empty request id left to `--yaml`. A warning is a yellow `!` and its message, its code faint. For a program, they stay YAML or JSON documents ([commands-and-api.md](commands-and-api.md#cli-output)).

The bar for a pretty view: **it beats the YAML for a person, or it doesn't ship.** A generic renderer can do worse than YAML (a query's rows buried under its columns' metadata), so a view is reviewed, from real output, before a command uses it.

The aim is very pretty output, reached **incrementally, and by rules**:

- **One command at a time.** A command gets its view when one is reviewed and clears the bar; until then it writes YAML, which is never wrong. No date by which every command must be pretty.
- **Rules before views.** The [conventions](#conventions) first, a [hint](#hints) where they fall short, a [named view](#named-views) of a type of value only where both do. A convention improved improves every command it covers; a view is code to keep in step with its type.
- **No formatting outside the prettifier.** No console-only view, no hand-written output beside a result: one place to change, and a command reads the same wherever it's pretty.
- **No surprises where they buy nothing.** Labels from the JSON names, values as the YAML has them (`true`, not `yes`), nothing hidden: a person moving between the pretty format and `--yaml` recognizes the same result. A view departs from the result only where reading gains from it (a relative time, a tree of paths), and says so in a hint.

## Layers

```
result (Go value, or an op's JSON) ──> view model ──> renderer ──> terminal
                                       (sections,      (terminal now;
                                        key/values,     Markdown possible)
                                        tables, blocks)
```

- **`internal/pretty`** builds both, behind `pretty.Render`; `cmd/anfra` decides when a result is drawn with it (`writeResult`, by the command's default format).
- **The view model** is where the judgement lives: what a section is, what becomes a table. The [conventions](#conventions) build it from the result's shape, adjusted by [hints](#hints), and a [named view](#named-views) where a hint names one.
- **A renderer** draws a view model. The terminal one aligns and styles. A Markdown one (headings, pipe tables, fenced blocks) could follow from the same model, for pasting into an issue or for agents, if it proves better than YAML for them; it isn't planned.

A core command's answer reaches the CLI as JSON, often forwarded from the warm server: the CLI decodes it into the command's Go type again (`presentTo`, `cmd/anfra/commands.go`), so its `pretty` tags apply, whoever answered.

## Conventions

A result is a **detail** view, as `kubectl describe` is: one document, in sections. A table is for a list of like items, not for the document itself.

0. **`message` and `hints`.** A CLI-only command's result opens with `message` and closes with `hints` ([CLI output](commands-and-api.md#cli-output)). The pretty view shows `message` as its first line, unlabelled, and `hints` as its last section, one a line, after a blank line: the outcome first, what to do next last.
1. **Key/value lines.** `Label:  value`. The label is the JSON name in Title Case (`checked_at` → `Checked At`, acronyms in capitals: `URL`, `ID`), unless a hint says otherwise. Values align within a run of consecutive key/value lines at one level, as gofmt aligns a struct literal's fields: a nested section ends the run, and lines at different levels never align.
2. **Nesting.** A nested object is a `Label:` heading, its contents indented by 2. At the top, a blank line sets apart what has rows (a table, a list of blocks, a text block or a tree) and the message and hints; a section of key/values stays with its neighbours.
3. **Lists of objects** (a listing: agents, orgs, a few named fields each). A table when every column holds a scalar (or a one-item list or object, flattened) and the table fits the terminal: UPPERCASE headers, columns 3 spaces apart, no borders, as kubectl and gh draw them. Otherwise, one block per item, separated by blank lines, its first scalar as its heading, its nested parts shown in full.
4. **Lists of scalars.** One per line, aligned in the value column. In a table cell, a CSV row (rule 10).
5. **Lists of lists** (rows: data, as a query answers). A bordered grid, as psql and duckdb draw one: a value may hold spaces or be empty, and only a border says where a cell ends. Its header is its values' labels when a hint gives them (`labels=`), faint; none otherwise. Wider than the terminal, one block per row, a label and a value a line (`-[ ROW 1 ]-`), as psql's expanded mode; never cut cells. In ASCII without a UTF-8 terminal, as trees are. Which is a grid and which a table is the data's shape, never where it's shown.
5b. **Numbers are right-aligned**, in a grid or a table: a column whose every value, `<none>` aside, is a number.
5c. **Raw JSON** (a `jsontext.Value`, a query's row) is laid out as the value it holds; an object decoded from JSON as a struct is, its keys sorted (a map keeps no order), or in order when read as a `pretty.Object`.
6. **Nothing hidden.** No counts in place of items: a count can't be drilled into (there's no command for one item), so it's a dead end. A view may choose a count deliberately (`files: 42`) where the items don't matter.
7. **Empty** is `<none>`, always. What an empty value means is the result's to say, in a field of its own, not the view's: `agent setup`'s agents have a `status`, `installed` or `not-installed`, beside `install_from`, absent when installed, rather than an empty `install` that pretty would have to explain.
8. **Booleans** are `true` and `false`, as in the YAML.
9. **Multi-line text** is an indented block under its key, never squashed. In a table cell: its first line, then `…`.
10. **A list in a cell is a CSV row.** Every list joined into a table cell is written as one CSV record (RFC 4180, Go's `encoding/csv`), with no rule of ours: `,` between items, as `kubectl` joins (`80/TCP,443/TCP`), and an item quoted only when CSV needs it, a quote inside doubled: `Laptop,"Smith, John","a ""quoted"" word"`. A line break never reaches a cell (rule 9). A one-item object flattened into a cell is the same, with a space as the separator: `user 0.4.4 "behind by 2"`.
11. **Times** tagged as a moment are relative: `today`, `3 days ago`. The YAML keeps the timestamp.
12. **Truncation and color on a terminal only.** Pretty output can go anywhere: under the [current defaults](#where-it-stands), `agent setup` writes it into a pipe or a file too. Only on a terminal is a column a hint marks as truncatable cut to its width (`…`), and only there is it colored ([color](#color)). Elsewhere, there's no width to fit, so nothing is cut, and no color. The full value is always one flag away: `--yaml`.

### Hints

A field's `pretty` tag adjusts the layout; it never adds a field or text beyond its labels, so every word the pretty output shows is a label or a value the YAML has:

| Hint | Does |
|---|---|
| `pretty:"-"` | hides the field |
| `pretty:"AGENTS.md"` | its label, or its heading |
| `pretty:",block"` | multi-line text as a block, even when it has one line |
| `pretty:",moment"` | a time shown relative |
| `pretty:",truncate"` | a column that may be cut to fit the terminal |
| `pretty:",tree"`, `pretty:",tree=path"` | a list of paths, or of objects with a path in the named field, drawn as a [tree](#trees). Built for a list of paths (`publish check`); `tree=path` not yet |
| `pretty:",tone=latest:good behind:warn failed:bad"` | the [tone](#color) of some of a field's values |
| `pretty:",labels=columns.label"` | rows' values' labels, from a path from the result's top, through objects and lists: a grid's header, a block's labels |
| `pretty:",inline"` | an object's shown fields are its parent's, under no heading of their own, as encoding/json inlines an embedded one |
| `pretty:",view=timings"` | a [named view](#named-views) of the value, for one no convention shows well |

### Trees

A list of paths, such as `publish check`'s hundreds of files, repeats its folders on every line when flat. With the `tree` hint it's drawn as `tree` draws one:

```
Files:
  PATH                             CHANGED
  ├── .anfra/
  │   ├── data_sources.yml         true
  │   ├── data_sources.yml.example true
  │   └── context_sources.yml      true
  ├── models/.gitkeep              false
  ├── .gitignore                   true
  └── AGENTS.md                    true
```

- **Lines, not indentation alone**, as `tree`, `npm ls` and `cargo tree` draw them: `│` ties a folder's entries together across distance, `└──` ends it. Sections and blocks elsewhere nest by 2 spaces: they aren't trees.
- **Folders first, then files**, each sorted. A folder with one entry stays on its parent's line (`models/.gitkeep`), as GitHub shows it.
- **The other columns align**: a line's prefix is part of its first column's width.
- **ASCII without a UTF-8 terminal** (`LANG`, `LC_ALL`): `|--`, `` `-- `` and `|`, as `tree --charset=ascii`. The lines are faint.
- A path can't be copied whole from a tree; `--yaml` keeps every path whole.

## Color

*Ideas, not decided.*

Color helps a person find their way around the output, and see what needs attention. It never carries meaning alone.

1. **Color only reinforces.** Every meaning is in the text: `behind` says it's behind, yellow or not. The output reads the same without color: piped, with `NO_COLOR`, or for a person who can't tell red from green.
2. **The styles anfra already has.** `cliui.TerminalStyles` (`Heading`, `Muted`, `Accent`, `Selection`, `Success`, `Warning`, `Error`, `Code`, `Link`, `Key`), in the 16 ANSI colors of the user's own palette, so it suits light and dark terminals, but for one: anfra's violet, the design package's accent ([DESIGN.md](../../web/design/DESIGN.md), `#8a51d4`), mid-light for light and dark backgrounds alike, converted to what the terminal shows (the nearest of 256 colors; magenta, of 16). No backgrounds, but a code span's: it sets its text's color too, so it doesn't depend on the terminal's.
3. **Off where `cliui` turns it off:** not a terminal, `NO_COLOR`, `CI`, `TERM=dumb`.
4. **Weight for structure, hue for meaning.** Bold and faint lay out the document, in two steps: **bold** for the message alone, the one line saying what happened; faint for what's beside the point (punctuation, `<none>`, a table's header, the steps of timings). Bold everywhere would mark nothing. Each hue means one thing: **violet** what a person can act on (a prompt, a selection, a link, a command to run), as the design package's one accent marks the primary action, links and selection; **bright blue** a key, in every format: a pretty view's labels as YAML's and JSON's keys; **green, yellow and red** tones.

| What | Style |
|---|---|
| The `message` line | bold (`Heading`) |
| Every label, a value's (`Latest:`) or a section's (`Agents:`, `Hints:`) | bright blue (`Key`), as YAML's keys |
| A block's heading in a list (`~/.agents/skills:`), a value used as a heading | plain |
| A table's header (`NAME  STATUS`) and a grid's (a query's columns' labels) | faint (`Muted`): a header names columns, it isn't a value, and uppercase or a border marks it already |
| `<none>` | faint (`Muted`) |
| `` `code` `` in a message, a hint or a value, a command to run | `Code`, a code span without the backquotes: the design package's dark violet text (`violet-350`, `#d4c0fc`) on its violet wash (`violet-a26` over its dark panel `ink-925`, `#332349`), a space of padding each side, as Sentry's CLI shows code: a dim tint, the text standing out. Both colors set, so it reads on dark and light terminals alike (8.6:1). Of 16 colors, bold magenta, without a fill |
| A URL | `Link`, violet, underlined |
| A value with a tone | `good` green (`Success`), `warn` yellow (`Warning`), `bad` red (`Error`) |
| Everything else: values, numbers, paths | plain |

**Tones come from hints.** The prettifier can't guess that `behind` is bad news and `latest` good, so a field says which of its values have a tone: `pretty:",tone=latest:good behind:warn mixed:warn deprecated:bad unknown:warn"` on a copy's `status`, `pretty:",tone=installed:good failed:bad cli_missing:warn"` on setup's `outcome`. A value without a tone, and a boolean, stays plain: `true` isn't good news in itself.

What `agent setup check` would look like, styles noted on the right:

```
Claude Code's plugin is behind 0.4.5, and ~/.claude/skills has deprecated skills.   bold

Skills:                                                                             blue
  Latest:      0.4.5                                                                labels blue
  Checked At:  today

Agents:                                                                             blue
  NAME          STATUS          PLUGINS             COPIES                       headers faint
  Claude Code   installed       user 0.4.4 behind   ~/.claude/skills
                └ green                    └ yellow
  OpenCode      not-installed   <none>              <none>
                └ yellow        └ faint             └ faint

Hints:                                                                              blue
  Run `anfra agent setup` to update the Claude Code plugin.
      └ Code: a violet code span
```

**Siblings look alike.** Every label is a key, blue, a section's as a value's: the indentation and the blank lines show what a section holds, not a style of its own. A key in a pretty view is the key the YAML shows: someone moving between the formats finds the same names, in the same hue.

**YAML and JSON, in a console**, follow the same rules ([console.md](console.md)): every key bright blue (`Key`), as a pretty view's labels are, siblings alike, the indentation showing what opens a map or a list; `null` and punctuation faint; a URL as a link; an error's `code` red and a warning's yellow, the tones every error and warning has; any other value plain. Blue is a key's alone, as jq colors keys, bright as it reads on dark backgrounds and light, and without bold: the hue sets keys apart, and bold is for what a reader looks for first (a console's prompt, a result's message), not a long result's every line; values get no hue of their type (jq's strings green), since green is a tone's. Outside a console, YAML and JSON stay plain, since an agent reads them.

The tone inside a flattened cell (`behind`, in `user 0.4.4 behind`) is the field's own: flattening keeps each value's style.

## Named views

Where conventions and hints can't show a value well, a named view does: `views` (`internal/pretty/views.go`), each of a type of value rather than of a command, so every result holding one shows it alike, and a hint names it (`pretty:",view=timings"`). The last resort: it's code to keep in step with its type.

- **`timings`**, a run's (`command.Timings`). They record when each step *started*; how long each took is what a person reads, which no convention can compute. So: each step's duration, until the next at its level or above starts, or the run ends; a child step (`<parent>.<child>`) nested under its parent; a step run again shown again; the steps faint, the detail; then the total, plain, what's read first.

`query` needed none of its own: its rows are a grid by the conventions, labelled by a hint, and its timings are the named view.

## Examples

### `anfra agent setup check`

```
Claude Code's plugin is behind 0.4.5, ~/.agents/skills is mixed, and ~/.claude/skills has deprecated skills.

Skills:
  Latest:      0.4.5
  Checked At:  today

Agents:
  NAME          STATUS          PLUGINS             COPIES
  Claude Code   installed       user 0.4.4 behind   ~/.claude/skills
  Cursor        installed       <none>              ~/.agents/skills
  OpenCode      not-installed   <none>              <none>

Copies:
  ~/.agents/skills:
    Status:            mixed
    Versions:
      VERSION   SKILLS
      0.4.4     anfra-aql,anfra-write-aql
      0.4.5     anfra-amql,anfra-build-dashboard,anfra-develop-amql,anfra-search,anfra-setup-repo,anfra-viz
    Deprecated Names:  false

  ~/.claude/skills:
    Status:            deprecated
    Versions:          <none>
    Deprecated Names:  true

AGENTS.md:
  Block:      up-to-date
  CLAUDE.md:  imported

Hints:
  Run `anfra agent setup` to update the Claude Code plugin.
  To update the skills in ~/.agents/skills, see https://github.com/holistics/anfra-skills#updating-and-removing.
  Remove the deprecated anfra skills in ~/.claude/skills, as https://github.com/holistics/anfra-skills#updating-and-removing says.
```

- `agents` is a table: each agent has at most one plugin, flattened (`user 0.4.4 behind`), and its copies are scalars. With two plugins, an agent's row would hold a list of objects, and `agents` would become blocks.
- `copies` is blocks: `versions` is a list of objects, shown in full inside each block.
- Hints: `AgentsMD` (`AGENTS.md`), `ClaudeMD` (`CLAUDE.md`), `CheckedAt` (`moment`), and the tones of `status`, `block` and `claude_md`. `id` and `install_from` are hidden (`pretty:"-"`): the name says the same to a person, `--help` lists the ids, and a hint gives the link, so the table fits a terminal.

### `anfra agent setup`

What setup did, then the report above as setup leaves things:

```
Installed the anfra skills for Claude Code. Codex failed, and OpenCode needs them installed by hand.

Setup:
  Agents:
    NAME          OUTCOME             MESSAGE
    Claude Code   installed           Restart Claude Code, or run /reload-plugins, to load it.
    Codex         failed              `codex plugin add anfra@anfra-skills`: exit status 1
    OpenCode      instructions_only   anfra doesn't install the skills for OpenCode: install them as https://github.com/holistics/anfra-skills#opencode says.
  AGENTS.md:
    Wrote Block:   true
    Wrote Import:  false

Skills:
  …
```

On a terminal narrower than the `agents` table, `message` is the truncatable column; without the hint, the table would become blocks.

### `anfra init`

```
Set up an anfra repo in sales: changed 7 files, kept 1 already there.

Dir:  /home/me/sales

Files:
  PATH                              CHANGED
  .anfra/data_sources.yml           true
  .anfra/data_sources.yml.example   true
  .anfra/context_sources.yml        true
  models/.gitkeep                   false
  datasets/.gitkeep                 true
  apps/.gitkeep                     true
  .gitignore                        true
  AGENTS.md                         true

Hints:
  Put your database in .anfra/data_sources.yml, write models and datasets, then run `anfra validate`.
```

### `anfra query compile`

Text blocks, rule 9. Its answer is the SQL, the AQL and the columns:

```
Sql:
  SELECT "products"."name" AS "products__name", "products"."id" AS "products__id"
  FROM "public"."products" "products"
  LIMIT 3

Aql:
  products | select(products.name, products.id)

Columns:
  NAME             FIELD NAME   MODEL ID   LABEL   ADHOC   IS MEASURE   SEMANTIC TYPE
  products__name   name         products   Name    false   false        text
  products__id     id           products   ID      false   false        number
```

- `Sql` has more than one line, so it's a block under its key, as written.
- `Aql` has one line here, so it would be a key/value line (`Aql:  products | …`), but its `block` hint keeps it a block like `Sql`: a query reads the same whatever its length.
- Multi-line text in a table cell is its first line, then `…`. A diagnostic whose message carries a code frame reads:

  ```
  FILE PATH            ROW   COL   MESSAGE
  models/orders.aml    12    5     Unknown field `amount` in model `orders`…
  ```

  The whole message is in `--yaml`; a view that needs it whole makes its list blocks instead.

### `anfra query`

`query --pretty`, and `query` in a console: the SQL, the AQL and what each column is are hidden (`pretty:"-"`), left to `--yaml`; the rows, inlined from `result` (`inline`), under `Result:`, the YAML's own key, labelled by each column's `label` (`labels=columns.label`); then `has_more`; then the timings by their view.

```
Result:
  ┌───────────┬────────┬──────────┐
  │ status    │ orders │ city     │
  ├───────────┼────────┼──────────┤
  │ delivered │   1204 │ New York │
  │ pending   │     87 │ <none>   │
  │ refunded  │      3 │ Hà Nội   │
  └───────────┴────────┴──────────┘

Has More:  false

Timings:
  connecting_to_dw        60 ms
    open_connection       60 ms
  executing_query          3 ms
  fetching_query_result    5 ms
  total                   68 ms
```

Values are as the answer has them: no thousands separators, no rounding. `Has More: false` shows on every result; a hint showing a field only when true would hide it, should it prove noise.

## Other formats considered

- **Markdown**: the [layers](#layers) allow a Markdown renderer. LLMs read it fluently, and its tables state keys once, so it may cost agents fewer tokens than YAML for lists, without truncation. Raw, it's noisy on a terminal; rendered, wide tables wrap badly. Not planned; worth measuring with agents later.
- **CSV**: fits a flat table only, not a nested document, so not a format for every command. For a command whose view has one main table (`query`'s rows), `--csv` could write that table, for spreadsheets, `duckdb`, or an agent. `anfra query export` already writes CSV files.

## References

From how other CLIs show output to people; not followed to the letter.

| Tool | What we took |
|---|---|
| `kubectl describe` | the detail view: `Label:` lines, sections indented by 2, `<none>` |
| `kubectl get`, `docker ps` | tables: UPPERCASE headers, 3 spaces apart, truncation only on a terminal |
| Azure CLI `-o table` | conventions for every command, and per-command views where they fall short |
| kubectl's `additionalPrinterColumns` | view hints in the schema |
| PowerShell's format views | a table when it fits, else one block per item |
| nushell | the problem with counts: `[table 3 rows]` needs a way to drill down, which anfra has none of |
| psql | rows aligned with numbers to the right, and expanded mode for wide rows |
| CSV (RFC 4180) | quoting the items of a list joined in a cell |
| AWS CLI `-o table` | the warning: generic tables of nested JSON are hard to read |
