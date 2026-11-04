# anfra console

*Being designed. Not built yet: the steps are in [implementations/console.md](../implementations/console.md).*

A session at a terminal, as psql, `rails console` and duckdb's shell are: anfra's commands typed without the `anfra`, AQL typed directly in a query mode, the output for the person at the keyboard.

## Why

Agents are anfra's most frequent users, so its output defaults suit them: YAML for core commands, no colour outside the pretty format. A person would like colour on everything, but every way to switch it on outside a session is either unsafe or unused:

- **Automatic on a terminal**: some agents run commands in a terminal of their own (Cursor's and VS Code's agent modes), and would get escape codes.
- **An environment variable or a config setting**: set in a person's shell profile, every agent started from that shell inherits it, colour forced through pipes too.
- **A flag**: safe, but nobody types it on every command.

A session limits it to the person who opened it: no agent starts one by accident, nor could easily drive one.

The session also holds what a one-shot command can't: the dataset queries run against, a warm server so each query answers fast, history, and later the last result.

## A session

```
$ anfra console
anfra console 0.17.0 in ~/work/acme
Type a command as after anfra: validate, query -d sales "orders | count()".
\? lists the console's commands, \q or exit leaves.

acme> validate
valid: true
…

acme> publish check
Would publish to Acme (https://cloud.example.com/o/acme).

Files:
  ├── datasets/
  …

acme> console query
acme query> \use sales
acme/sales query> orders
              ... | group(orders.status)
              ... | select(orders.status, count(orders.id));;
aql: …
sql: …
columns:
  - name: status
    …
result:
  - [delivered, "1204"]
  - [pending, "87"]
has_more: false

acme/sales query> \console search
acme/sales search> revenue by region
…
acme/sales search> \console
acme> exit
```

(Drafts: the prompt, the banner, and the answers' shapes, cut short with `…`.)

**The output is the formats the CLI has, coloured.** A command whose pretty view is reviewed writes the pretty format, a core command included; any other writes YAML ([cli-pretty.md](cli-pretty.md#where-it-stands)). Today that is the CLI-only commands' views, so `query` above writes YAML. The console gets prettier as the prettifier does, one command at a time, by its rules: no view is made for the console alone. `--yaml` and `--json` work as they do outside, coloured, and JSON, answers and errors alike, indented by 2, as gh and jq show JSON to a person: outside a console, a program reads it a line a document.

## Lines

The console is in **command mode** until a **text mode** is entered, and the prompt says which: `acme>`, or `acme/sales query>`.

- **In command mode, a line is a command, typed as after `anfra`: `validate`, `query compile -d sales "…"`, `publish check`. Split into args as a shell would, quotes and escapes included, and run on Enter. A `\` before it, typed from a text mode's habit, is taken too. A line that isn't a command (AQL, say) fails as at a shell, `unknown_command`, and the console says how to type AQL.
- **In a text mode, a line goes to the mode's command,** and a command needs a `\`: `\validate`. The mode is the command that receives plain text: `console query` enters `query`'s, `console query --lang sql` SQL's, `console search` `search`'s; `anfra console query` starts in it. `console` alone, `\console` in a text mode, goes back to command mode. So `console <command>` means one thing wherever it's typed, and so does its short name: `c query`, `\c search`, `\c` back. psql's `\c` connects to another database, the nearest thing to `\use`: `\c sales`, sales not being a command, says to `\use sales`. A command can be a mode when it takes its main input positionally, which `--schema` marks with `x-cli: [positional]`: today `query`, `search`, `show` and `validate`. A string argument gets the entry whole; a list, its words.
- **`\` means "a command, not text"**, needed only where text is what a line is by default.
- **The console's own commands**, in every mode: `\use <dataset>` sets the dataset any `query` in the session runs against, unless it says `-d`; `\q`, `exit` and `quit` leave, as Ctrl-D on an empty entry does; `\?` lists these, then anfra's commands as `anfra --help` groups them, one line each, the names in violet (`help` and `--help` stay cobra's, plain, for now).

## How an entry ends

The mode decides.

- **AQL**: Enter adds a line, and an entry ending with `;;` runs on Enter, the `;;` removed. A single `;` can't end it: AQL's declarations end with one (`metric m = …;`). OCaml's REPL ends entries with `;;` too.
- **SQL**: a single `;`, as in psql.
- **Anything else** (`search`): Enter runs it.
- **Alt+Enter** runs the entry as it is, in every mode; so does **Ctrl+Enter** where the terminal reports it (kitty's keyboard protocol, which Bubble Tea turns on). Neither works everywhere, macOS's Terminal sending Option+Enter as a plain Enter until "Use Option as Meta key" is on, so they're shortcuts, `;;` the way that always works.
- A `;;` inside an open string doesn't end the entry.

## The rest

- **Colour** wherever the stream is a terminal; `NO_COLOR` still turns it off. The colours follow [cli-pretty.md](cli-pretty.md#color), so YAML, JSON and the pretty format read as one family. `--jq`'s plain lines stay plain, as `jq -r`'s are.
- **Questions** are asked, as a command run at a terminal asks them.
- **Ctrl-C** clears the entry being typed, or cancels the command running. The session stays.
- **History** is kept between sessions, in a file in anfra's home: Up reaches yesterday's entries, as in psql or a shell. An entry starting with a space is kept out, for one carrying a secret.
- **The prompt** shows the repo, the dataset when one is set, and the mode, in bold violet: where each entry starts, found scrolling back.
- **Notices** (anfra or the skills behind, a server started) go in a status area under the prompt, as Claude Code's, rather than after each command's output: shown while they hold, not repeated after every command, and never mixed into an answer.
- **A warm server**: the console uses the repo's running server, or starts one that stops by itself once the session has been gone a while. The banner says which.
- **Without a terminal**, `anfra console` refuses, as does `--yaml` or `--json` on it: it's an artifact, writing "a session, until you leave".

## The name

`console`, as `rails console`, `fly console` and Symfony's, with `c` as its short name: `anfra c`, as `rails c`. It can be confused with a web console: AWS's, Google Cloud's, and anfra Cloud reserves `console` as a path (`internal/bi/tenancy/tenancy.go` in anfra-cloud). Accepted: the command's help says what it is, and anfra Cloud's page would be the web console, this the terminal's. The others considered:

| Name | Against |
|---|---|
| `shell` | The system shell; `\shell` reads like psql's `\!` |
| `repl` | Jargon to analysts |
| `enter` | Collides with the Enter key, which the session is all about |
| `go`, `do` | One-off actions; `GO` runs a batch in SQL tools; `do` and `ask` are best kept for asking in plain words |
| a bare `anfra` | An agent running it in a terminal would hang in a session |

## Later

- Completion: commands and flags, then datasets, then fields.
- The last result: paging, drilling into a row, exporting it.

## Open questions

- **Which commands are modes.** `show` and `validate` qualify by the rule, but neither is something to type at a prompt over and over. Every command with a positional, or a list?
- **The prompt and the banner**: drafts above, to review.
