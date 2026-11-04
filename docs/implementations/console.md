# Implementation: `anfra console`

Implements [anfra console](../designs/console.md): a session at a terminal, anfra's commands typed without the `anfra`, AQL typed directly in a query mode, the output coloured for the person at the keyboard.

## Where it stands

| Step | State |
|---|---|
| 1. One process, many runs | Done (`run`, `cmd/anfra/main.go`) |
| A spike: stdin and the terminal across entries | Done on Linux; macOS and Windows to try |
| 2. The skeleton | Done (`cmd/anfra/console.go`, `cliui.ReadEntry`) |
| 3. Output for a person | Done: YAML and JSON colored (`internal/pretty/color.go`) |
| 3b. Notices in the status area | Next |

## Order

```
1 one process, many runs ──> 2 the skeleton ──> 3 output for a person ──> 4 modes and AQL ──> 5 a warm server
```

Each step ships on its own. Step 1 changes nothing visible, and is worth doing whether or not the console follows.

## 1. One process, many runs

The CLI assumes one command per process. A session runs many, each on a command tree of its own.

- **A fresh tree a line.** `newRootCmd()` builds the whole tree, every command's flags bound to variables of its constructor; the shared output flags are bound to the package's `output`, which defining them resets to their defaults. So a new tree per line starts clean. Measured: about 0.16 ms a tree.
- **`jqCode`** (`cmd/anfra/jq.go`) is set only when `--jq` is given, and `deliver` filters whenever it's set (`cmd/anfra/commands.go`): after one `--jq`, every later command would be filtered too. Cleared when `--jq` isn't given, or moved into `output`.
- **`resultTypes`** (`cmd/anfra/schema.go`), a global map keyed by command, grew by a tree's commands every rebuild. `newRootCmd` starts it empty, as defining the flags resets `output`: the package's state is the current tree's.
- **`run(ctx, args, stdout, stderr) int`** (`cmd/anfra/main.go`): what `main()` did around a command (the error, the exit code, the update notice, `agents.Succeeded`, `--json` asked for though the flags failed to parse), taking its args and streams rather than reading `os.Args`, returning the exit code rather than calling `os.Exit`. `main()` validates the settings, starts telemetry, and exits with `run`'s code. Step 3 adds its options.
- **A core command's answer goes to the run's stdout** (`delivery.stdout`, from cobra's), not to `os.Stdout`; so does an export's file. Notices and warnings still go to `os.Stderr`, the same terminal in a console.

Tests (`cmd/anfra/main_test.go`): two runs in one process, the first with `--json` and `--jq`, the second with neither, writes YAML unfiltered, the result types not grown; an unknown command exits 1 with its error on stderr.

## A spike: stdin and the terminal across entries

A throwaway (`anfra __console-spike`): a text-area prompt in a loop, alternating with the org picker, the activity spinner, `publish`'s questions, and `run`. Driven through a pseudo-terminal on Linux by a script, which also played a terminal's late replies.

| Looked for | Found |
|---|---|
| Input taken by a reader that outlived its command | **Yes, and fixed.** After Ctrl-C at a `publish` question, `readLines` (`cmd/anfra/publish.go`) stays blocked reading stdin, and takes every later line: the session looks frozen. Reading through `muesli/cancelreader` (Bubble Tea's own, already a dependency), canceled with the question's context, ends the read with the question; the session then reads normally. Answering the question normally was fine either way. |
| The terminal left changed | No: its settings (`stty -g`) were the same after every entry, picker (picked or canceled), spinner and command. |
| Late replies to terminal queries typed as text | No: a reply arriving at the prompt, several in one write, or one arriving while a command runs (spinner or none) is parsed, not typed. `cliui`'s filter of Bubble Tea's capability probes (`terminal_queries.go`, bubbletea#1590) already leaves the prompt nothing to leak. |
| Alt+Enter, Ctrl+Enter told from Enter | Yes, both: `alt+enter`, and `ctrl+enter` as the kitty protocol sends it. Enter alone continues the entry; `;;` then Enter runs it. |

So step 2 needs no reader of its own for the session: each program reads stdin while it runs, as now. What it needs:

- **Every read of stdin ends with its command.** `readLines` through `cancelreader`, and the rule for any reader to come: no read outlives its command. A test: Ctrl-C at a question, then the next line reaches whoever reads next.
- **Enter on an empty entry does nothing**, rather than adding a line; and a `\` command is one line, so a `\` after a first line isn't one.
- **The text area grows with the entry**, rather than taking three lines from the start.

Not covered, for real terminals: macOS (Terminal.app, iTerm2: Option as Alt), replies' real timing over SSH, Windows Terminal (`cancelreader` supports its console).

## 2. The skeleton

- **`anfra console`**, or `anfra c` (`cmd/anfra/console.go`): a CLI-only command, an artifact ("a session, until you leave"), refusing without a terminal or with `--non-interactive` as `precondition_failed`. The banner names the version and the folder; the prompt, the folder's name.
- **The entry reader** (`cliui.ReadEntry`, `internal/cliui/entry.go`): one inline Bubble Tea program an entry, on `bubbles/textarea`, growing with the entry, ending with it, as `cliui`'s pickers do, so the command then runs with the terminal to itself. Up and Down step through the session's entries, the one being typed kept; Ctrl-C clears the entry, or on an empty one shows "\q or Ctrl-D to leave" under the prompt; Ctrl-D on an empty entry leaves. One line an entry, for now: Enter ends it.
- **Splitting** (`splitArgs`): a `\` line into args as a POSIX shell splits words, quotes and backslashes, expanding nothing. A quote left open is said, and nothing runs.
- **Dispatch** (`consoleArgs`): command mode, the only one before step 4. `\q`, `exit`, `quit`, `\?` (and a bare `\`) are the console's; `console` is refused, being in one, until step 4 makes `console <command>` the switch; any other line runs through `run` on a new tree, a `\` before it taken too, its output and errors as the CLI writes them. A line that isn't a command fails as at a shell, and the console says how to run AQL (`isCommand`).
- **Ctrl-C**: each command runs on a context of its own, canceled by an interrupt. The session's context keeps the root's values but not its end, which main's interrupt brings: only SIGTERM ends a session.
- **No read of stdin outlives its command.** `publish`'s questions (`readLines`, `cmd/anfra/publish.go`) read through `muesli/cancelreader`, canceled with the question's context: after Ctrl-C at a question, the session reads the next entry, which the read left waiting used to take.

Tests: the reader, driven with key sequences (`internal/cliui/entry_test.go`); splitting and dispatch, and the refusal without a terminal (`cmd/anfra/console_test.go`); a canceled read leaving the next line to the next reader (`TestReadLinesEndWithTheirContext`). A session driven through a pseudo-terminal, by hand: the banner, `\version`, `\nope`, a plain line, `\?`, a quote left open, Ctrl-C, history, `\console`, `\q`.

## 3. Output for a person

- **`run`'s options** (`runOptions`, `cmd/anfra/main.go`): `color`, set by the console only, and set by `run` on every run, as the flags are reset. A shell's run leaves it off.
- **YAML and JSON colored** (`pretty.ColorYAML`, `pretty.ColorJSON`, `internal/pretty/color.go`) by the pretty format's rules ([cli-pretty.md](../designs/cli-pretty.md#color)): every key bright blue; `null` and punctuation faint; a URL as a link; an error's `code` red and a warning's yellow (`docTones`); any other value plain. yaml.v3 writes the YAML as it did, and goccy/go-yaml's lexer (already in the module graph, through huma) colors that text. The JSON, compact lines the CLI wrote, is scanned as it is.
- **Where**: a core command's answer (`renderTo`), `--jq`'s YAML and JSON (`renderJQ`; its plain lines stay plain), and errors and warnings (`writeDoc`, `writeJSONDoc`). Only at a terminal (`cliui.NewTerminalStyles`: not with `NO_COLOR`, `CI` or `TERM=dumb`).
- **The text is never changed.** Each colorer checks its output, its escape codes removed, against its input, and gives the input back uncolored when they differ: the lexer drops the text's final line break, which is put back.
- The pretty format was colored already, wherever it's written to a terminal. Its section labels now follow "siblings look alike": bold only where the section stands apart, faint among key/value lines (`TestSectionLabels`).
- **Not yet: the pretty format for a core command in the console.** The mark saying a command's view is reviewed comes with the first core command's view: today none has one.

Tests: for YAML and JSON, colored output with its escape codes removed is the input, and without styles nothing changes (`internal/pretty/color_test.go`); each rule's style; a colored run into anything but a terminal writes what a shell's run writes, errors included (`TestColorOnlyAtATerminal`). A session driven through a pseudo-terminal, by hand: `\version`, `\nope`, `--json`, `--jq` giving a scalar and a map.

## 4. Modes and AQL

- **Modes**: `console <command> [flags]` (`\console …` in a text mode) and `anfra console <command> [flags]`; `console` alone back to command mode; in a text mode, a command needs a `\`. The commands a mode can be, from their positional args (`argsOn`, `cmd/anfra/schema.go`). The hint for AQL typed in command mode becomes "enter console query", and the banner, `\?` and `anfra console --help` say how to enter a text mode (`console query`) and leave it (`console`).
- **How an entry ends** (`internal/console/ends.go`): `;;` for AQL, `;` for SQL, Enter otherwise; Alt+Enter and Ctrl+Enter; a `;;` in an open string ignored.
- **`\use <dataset>`**, checked against the repo's datasets, and the dataset in the prompt.
- Multi-line entries, with a continuation prompt.

Tests: entries ending or not (a declaration's `;`, `;;` in a string, `;;` after a `//` comment); a scripted session switching modes.

## 5. A warm server

Without a server running, each core command starts its own sidecars (`invoke`, `cmd/anfra/commands.go`). At its start, the console uses the repo's server (`findServer`), or starts `anfra serve --idle-timeout 10m` detached (`detach_unix.go`), so it stops by itself after the session. `invoke` already prefers a running server, so the commands need no change. The banner says which.

## Open questions

- **Bubble Tea in a loop**: an inline program an entry, hundreds in a session. The pickers start one a command. To check on Windows, and for leaks.
