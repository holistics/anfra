# CLI pretty output

*Being designed. Not built yet, except where noted. Not exposed: there's no `--pretty` flag yet.*

How the CLI shows a command's structured output to a person at a terminal: laid out for reading, from the same result `--yaml` and `--json` write, so the two can't drift. [CLI output](commands-and-api.md#cli-output) covers the structured formats; this doc covers the pretty one, and the prettifier that makes it.

## Where it stands

- **Formats.** A command's output is an artifact (a log, a script) or structured. Structured output comes as YAML, JSON, or pretty.
- **Defaults.** A core command writes YAML. A CLI-only command writes the pretty format by default, since people run them more than programs do: `update` and `agent setup` through the prettifier, then `init`; `publish` and `auth` through their own hand-written output, the pretty format done by hand until the prettifier covers them. `--yaml` or `--json` on any command gives structured output.
- **`--pretty`** comes once the prettifier shows every command well, `query` included. Then we decide whether pretty becomes the default on an interactive terminal for every command, with YAML for everything else.

The bar for a pretty view: **it beats the YAML for a person, or it doesn't ship.** A generic renderer can do worse than YAML (a query's rows buried under its columns' metadata), so a view is reviewed, from real output, before a command uses it.

## Layers

```
result (Go value, or an op's JSON) ──> view model ──> renderer ──> terminal
                                       (sections,      (terminal now;
                                        key/values,     Markdown possible)
                                        tables, blocks)
```

- **`internal/pretty`** builds both, behind `pretty.Render`; `cmd/anfra` decides when a result is drawn with it (`writeResult`, by the command's default format).
- **The view model** is where the judgement lives: what a section is, what becomes a table. The [conventions](#conventions) build it from the result's shape, adjusted by [hints](#hints), or a command's [own view](#a-commands-own-view) builds it.
- **A renderer** draws a view model. The terminal one aligns and styles. A Markdown one (headings, pipe tables, fenced blocks) could follow from the same model, for pasting into an issue or for agents, if it proves better than YAML for them; it isn't planned.

The view model takes JSON, not only Go values: a core command's answer reaches the CLI as JSON, often forwarded from the warm server. A hint then travels in the result's JSON Schema (`x-pretty`), generated from the Go type's `pretty` tag.

## Conventions

A result is a **detail** view, as `kubectl describe` is: one document, in sections. A table is for a list of like items, not for the document itself.

0. **`message` and `hints`.** A CLI-only command's result opens with `message` and closes with `hints` ([CLI output](commands-and-api.md#cli-output)). The pretty view shows `message` as its first line, unlabelled, and `hints` as its last section, one a line, after a blank line: the outcome first, what to do next last.
1. **Key/value lines.** `Label:  value`. The label is the JSON name in Title Case (`checked_at` → `Checked At`), unless a hint says otherwise. Values align within a block.
2. **Nesting.** A nested object is a `Label:` heading, its contents indented by 2. Top-level sections are separated by a blank line.
3. **Lists of objects.** A table when every column holds a scalar (or a one-item list or object, flattened) and the table fits the terminal: UPPERCASE headers, columns 3 spaces apart. Otherwise, one block per item, separated by blank lines, its first scalar as its heading, its nested parts shown in full.
4. **Lists of scalars.** One per line, aligned in the value column. In a table cell, a CSV row (rule 10).
5. **Lists of lists** (rows): aligned columns, never joined.
6. **Nothing hidden.** No counts in place of items: a count can't be drilled into (there's no command for one item), so it's a dead end. A view may choose a count deliberately (`files: 42`) where the items don't matter.
7. **Empty** is `<none>`, always. What an empty value means is the result's to say, in a field of its own, not the view's: `agent setup`'s agents have a `status`, `installed` or `not-installed`, beside `install_from`, absent when installed, rather than an empty `install` that pretty would have to explain.
8. **Booleans** are `true` and `false`, as in the YAML.
9. **Multi-line text** is an indented block under its key, never squashed. In a table cell: its first line, then `…`.
10. **A list in a cell is a CSV row.** Every list joined into a table cell is written as one CSV record (RFC 4180, Go's `encoding/csv`), with no rule of ours: `,` between items, as `kubectl` joins (`80/TCP,443/TCP`), and an item quoted only when CSV needs it, a quote inside doubled: `Laptop,"Smith, John","a ""quoted"" word"`. A line break never reaches a cell (rule 9). A one-item object flattened into a cell is the same, with a space as the separator: `user 0.4.4 "behind by 2"`.
11. **Times** tagged as a moment are relative: `today`, `3 days ago`. The YAML keeps the timestamp.
12. **Truncation and color on a terminal only.** Pretty output can go anywhere: under the [current defaults](#where-it-stands), `agent setup` writes it into a pipe or a file too. Only on a terminal is a column a hint marks as truncatable cut to its width (`…`), and only there is it colored ([color](#color)). Elsewhere, there's no width to fit, so nothing is cut, and no color. The full value is always one flag away: `--yaml`.

### Hints

A field's `pretty` tag (and its schema's `x-pretty`) adjusts the layout; it never adds a field or text beyond its labels, so every word the pretty output shows is a label or a value the YAML has:

| Hint | Does |
|---|---|
| `pretty:"-"` | hides the field |
| `pretty:"AGENTS.md"` | its label, or its heading |
| `pretty:",block"` | multi-line text as a block, even when it has one line |
| `pretty:",moment"` | a time shown relative |
| `pretty:",truncate"` | a column that may be cut to fit the terminal |
| `pretty:",tone=latest:good behind:warn failed:bad"` | the [tone](#color) of some of a field's values |

## Color

*Ideas, not decided.*

Color helps a person find their way around the output, and see what needs attention. It never carries meaning alone.

1. **Color only reinforces.** Every meaning is in the text: `behind` says it's behind, yellow or not. The output reads the same without color: piped, with `NO_COLOR`, or for a person who can't tell red from green.
2. **The styles anfra already has.** `cliui.TerminalStyles` (`Heading`, `Muted`, `Success`, `Warning`, `Error`, `Code`, `Link`), in the 16 ANSI colors of the user's own palette, so it suits light and dark terminals. No new palette, no backgrounds.
3. **Off where `cliui` turns it off:** not a terminal, `NO_COLOR`, `CI`, `TERM=dumb`.
4. **Weight for structure, hue for meaning.** Bold and faint lay out the document; green, yellow and red are only for tones, so a color always means something.

| What | Style |
|---|---|
| The `message` line | bold (`Heading`) |
| Section labels (`Agents:`) and table headers | bold (`Heading`) |
| Key labels (`Latest:`) | faint (`Muted`) |
| `<none>` | faint (`Muted`) |
| `` `code` `` in a message, a hint or a value | `Code`, without the backquotes |
| A URL | `Link` |
| A value with a tone | `good` green (`Success`), `warn` yellow (`Warning`), `bad` red (`Error`) |
| Everything else: values, numbers, paths | plain |

**Tones come from hints.** The prettifier can't guess that `behind` is bad news and `latest` good, so a field says which of its values have a tone: `pretty:",tone=latest:good behind:warn mixed:warn deprecated:bad unknown:warn"` on a copy's `status`, `pretty:",tone=installed:good failed:bad cli_missing:warn"` on setup's `outcome`. A value without a tone, and a boolean, stays plain: `true` isn't good news in itself.

What `agent setup --check` would look like, styles noted on the right:

```
Claude Code's plugin is behind 0.4.5, and ~/.claude/skills has deprecated skills.   bold

Skills:                                                                             bold
  Latest:      0.4.5                                                                labels faint
  Checked At:  today

Agents:                                                                             bold
  NAME          STATUS          PLUGINS             COPIES                       headers bold
  Claude Code   installed       user 0.4.4 behind   ~/.claude/skills
                └ green                    └ yellow
  OpenCode      not-installed   <none>              <none>
                └ yellow        └ faint             └ faint

Hints:                                                                              bold
  Run `anfra agent setup` to update the Claude Code plugin.
      └ Code
```

The tone inside a flattened cell (`behind`, in `user 0.4.4 behind`) is the field's own: flattening keeps each value's style.

## A command's own view

Where conventions and hints can't make a result read well, the command writes its view model itself. `query` is the case: a person wants the rows, labelled by each column's `label`, numbers right-aligned by `semantic_type`, and a line saying how many rows, whether there are more, and how long it took. Its columns' metadata, the AQL, the SQL and the timing steps stay in `--yaml`.

An own view can drift from its result, as hand-written output does. Its test lists the result's fields the view leaves out on purpose, so a field added to the result fails the test until someone decides whether the view shows it.

## Examples

### `anfra agent setup --check`

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
Set up an anfra repo in /home/me/sales, creating 7 files and keeping 1 that was already there.

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

### `anfra query` (its own view)

```
NAME       ID
Laptop      1
Mouse       2
Keyboard    3

3 rows, more on the next page (--page 2) · 64 ms
```

A result wider than the terminal is one block per row (`-[ RECORD 1 ]-`), as psql's expanded mode, rather than truncated cells.

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
