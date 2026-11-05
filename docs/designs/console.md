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
Type a command as after anfra; c query to type AQL. \? helps, \q leaves.

> validate
valid: true
…

> publish check
Would publish to Acme (https://cloud.example.com/o/acme).

Files:
  ├── datasets/
  …

> \-d sales
> c query
Type AQL, ending it with ;; to run it. \? helps, \c goes back.
query> orders
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

query> \c search
Type search's text, and press Enter to run it. \? helps, \c goes back.
search> revenue by region
…
search> \c
> exit
```

While typing, the entry is framed, and the line under it holds where the session is, and a notice:

```
────────────────────────────────────────────────────────────────
query> orders | count();;█
────────────────────────────────────────────────────────────────
acme · -d sales --json                     ↑ anfra 0.17.0 · Run update
```

(The answers' shapes are cut short with `…`.)

**The output is the formats the CLI has, coloured.** A command with a pretty view writes the pretty format, `query` and the CLI-only commands so far; any other writes YAML ([cli-pretty.md](cli-pretty.md#where-it-stands)). So `query` above writes a grid of its rows, the YAML sketched being what `--yaml` gives. The console gets prettier as the prettifier does, one command at a time, by its rules: no view is made for the console alone. `--yaml` and `--json` work as they do outside, coloured, and JSON, answers and errors alike, indented by 2, as gh and jq show JSON to a person: outside a console, a program reads it a line a document.

## Lines

The console is in **command mode** until a **text mode** is entered, and the prompt says which: `>`, or `query>`.

- **In command mode, a line is a command,** typed as after `anfra`: `validate`, `query compile -d sales "…"`, `publish check`. Split into args as a shell would, quotes and escapes included, and run on Enter. A `\` before it, typed from a text mode's habit, is taken too. A line that isn't a command (AQL, say) fails as at a shell, `unknown_command`, and the console says to enter `console query`.
- **In a text mode, a line goes to the mode's command,** and a command needs a `\`: `\validate`. The mode is the command that receives plain text: `console query` enters `query`'s, `console query --lang sql` SQL's, `console search` `search`'s; `anfra console query` starts in it. `console` alone, `\console` in a text mode, goes back to command mode. So `console <command>` means one thing wherever it's typed, and so does its short name: `c query`, `\c search`, `\c` back. A mode is a core command taking its input positionally (`x-cli: [positional]` in `--schema`): `query` and its subcommands, `search`, `show`, `validate`; a CLI-only command's positional is a folder or an address, not text, and is refused, naming those that work. A string argument gets the entry whole; a list, its words; either after `--`, so text starting with `-` isn't a flag.
- **`\` means "a command, not text"**, needed only where text is what a line is by default.
- **A line of flags sets them for the session:** `\-d sales`, `\--timezone UTC --json`. Each is added to every command that takes it and wasn't given it, the entry's own winning, as kubectl's default namespace is; a format flag isn't added to a command writing an artifact, which refuses it. Values are read as each flag defines them (a boolean takes none); an empty value unsets one (`\--dataset=`). In a text mode a flag must be one its command takes, a short name its command's; in command mode any command's, and a short name meaning different flags in different commands (`-s`: `query`'s `--data-source`, `ingest`'s `--source`) needs its long name. `--help`, `--version` and `--schema` can't be set. Alternatives replace each other: `\--yaml` after `\--json` switches the format, rather than leaving every command refusing both. `\--` lists them: those the mode was entered with, those set in the session, and, in a text mode, the command's others. A flag the mode was entered with is the mode's to change: in `c query --lang sql`, `\--lang aql` makes it query's AQL mode, as `c query --lang aql` would, rather than a session's flag the mode's own would beat. An error naming a flag, one missing say, says so in its `help`: ``\--data-source <value>` sets it for every command here. Run `query --help` for usage.`, the usage as typed in the console, without the `anfra`. One mechanism for every flag, rather than a command for each (`\use` for a dataset): the console knows no flag's meaning. psql's `\c sales`, sales not being a command, says `\-d sales` sets a dataset.
- **The console's own commands**, in every mode: `\q`, `exit` and `quit` leave, as Ctrl-D on an empty entry does; `\?` helps, for where the session is, scannable rather than prose: three labelled lines, `Repo:` its folder, `Mode:` the mode as the prompt shows it (bold violet; `commands` in command mode), `Flags:` those in effect as typed, names blue as keys, a target's name bold, `(view all with \--)`, labels faint; `Notices:`, the warning and any notice in full; then `Usage:`, how to type and the console's keys in two columns; then, in a text mode, `<command> flags:` as its help lists them, and `Global flags:` likewise; in command mode, the keys and the commands, a line each. Not a command's long description, written for an agent as much as a person (the backlog has descriptions for both). The intros point to it, rather than listing anything.

## How an entry ends

The mode decides.

- **AQL**: Enter adds a line, and an entry ending with `;;` runs on Enter, the `;;` removed. A single `;` can't end it: AQL's declarations end with one (`metric m = …;`). OCaml's REPL ends entries with `;;` too.
- **SQL**: a single `;`, as in psql.
- **Anything else** (`search`): Enter runs it.
- **Alt+Enter** runs the entry as it is, in every mode; so does **Ctrl+Enter** where the terminal reports it (kitty's keyboard protocol, which Bubble Tea turns on). Neither works everywhere, macOS's Terminal sending Option+Enter as a plain Enter until "Use Option as Meta key" is on, so they're shortcuts, `;;` the way that always works.
- A `;;` inside a string (`'…'`, `"…"`, `` `…` ``) or a comment (AQL's `//`, SQL's `--`) doesn't end the entry.
- Which, the mode's `--lang` decides, as its own flags, then the session's, then the command's default give it.

## The rest

- **Colour** wherever the stream is a terminal; `NO_COLOR` still turns it off. The colours follow [cli-pretty.md](cli-pretty.md#color), so YAML, JSON and the pretty format read as one family. `--jq`'s plain lines stay plain, as `jq -r`'s are.
- **Questions** are asked, as a command run at a terminal asks them.
- **Ctrl-C** clears the entry being typed, or cancels the command running. The session stays.
- **History** is kept between sessions, in a file in anfra's home, one for every repo: Up reaches yesterday's entries, as in psql or a shell. `\q`, `exit` and `quit` are kept out, so Up then Enter never leaves by surprise. No entry holds a secret to keep out: no command takes one as an argument ([commands-and-api.md](commands-and-api.md#secrets-never-in-arguments)).
- **The prompt** is the mode, which decides what a line is, in bold violet: where each entry starts, found scrolling back. A line of more than one is continued by `...`.
- **The prompt is framed** while typing, as Claude Code frames its input: a faint rule above and below, then one line, there even when blank, so nothing shifts as a badge comes and goes. On its left, after any warning, where the session is: the folder's name and the flags in effect as typed, the mode's and the session's, short names where they have them (`acme · -d sales --json`), faint, but a dataset's or a data source's name, in bold, the one thing in them checked before a query runs (a list of such flags, `targetFlags`; a command's own check could tell which it can't run without, but only in a text mode); on its right, a notice's badge. Too narrow, the left is cut short with `…` first; `\?` says it in full. Enter leaves only the prompt and the entry, on a band the terminal's width, as Claude Code shows a message sent, so each entry is found scrolling back: the design package's dark raised surface (`ink-825`, `#26232c`) on a dark terminal, its chrome (`surface-chrome`, `#f2f0eb`) on a light one, the text the terminal's own. Which, the console asks the terminal once; one that doesn't answer gets no band.
- **Notices** go in the line under the frame, as Claude Code's, rather than after each command's output: one at a time, as a badge at its far right, in yellow, since faint went unnoticed there: `↑ anfra 0.17.0 · Run update`. Apart from them, and before them on the left of the line, in yellow, where the eye lands: what the next entry would fail for, a flag the mode's command requires and nothing gives, `! --dataset is missing. Set it using \-d <value>`, or a flag that can't be used or is invalid, in the check's own words, which say what's wrong standing alone, and how to unset it (`! An AQL query runs against a dataset, not a data source. Unset it using \--data-source=`). It's the command's own check before running (`command.Check`), so the command's rule, not the console's. The context is cut short before it is. A skills badge counts its issues: `! 2 skills issues · Run agent setup`; shown while it holds, never mixed into an answer. `\?` gives it in full.
- **After `update`,** the console restarts itself on the new anfra: `Restarted on anfra 0.17.0.`
- **A warm server**, so queries answer fast. The console uses the repo's running `anfra serve`, and leaves it running. Otherwise it starts one when the session first needs the query engine: on entering a mode whose command runs on it (`c query`), in the background while the first entry is typed, or at the first such command, which waits for it with a spinner, running on its own, as at a shell, if it isn't up within 30 seconds. A server the console started is stopped when the session ends: a running server holds the repo's DuckDB files locked, for any other program ([motherduck-copy.md](motherduck-copy.md)), so it shouldn't outlive the session. It's started with a 10-minute idle timeout, for a console that dies without stopping it; a server stopped while idle is started again by the next command that needs it. The line under the frame shows it on the right, `● server`, the dot orange while it starts, green once warm, the word faint; `\?`'s `Server:` line says where it is and whose. After `update`, the console's server stops, and the new anfra starts its own.
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
