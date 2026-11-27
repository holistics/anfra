# Spec: win-terminal

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-terminal`). Depends on: `win-process-lifecycle` (done). Repo: **anfra**. Milestone **M3**: everything works on Windows.

## Objective

anfra runs on Windows, but its terminal output and a few things around it were written for macOS and Linux terminals. This module makes what a Windows user sees in a terminal right, and fixes the Windows failures found by the earlier modules' tests. Checked against the code and on Windows 11 on 2026-11-27.

**What a user sees:**

1. **Colors in older console windows.** anfra styles its output with ANSI escape codes (`internal/cliui/terminal.go`, lipgloss and `colorprofile`). Windows Terminal shows them (the user's `auth status` showed colors). The older console host, a `cmd` or Windows PowerShell 5 window opened outside Windows Terminal, shows them only once the program turns on `ENABLE_VIRTUAL_TERMINAL_PROCESSING` for its output; without it the codes print as text (`←[1m`). Turn it on at startup, on Windows, when stdout or stderr is a console. Where it can't be turned on, write no styles.
2. **Unicode trees and boxes.** `internal/pretty/pretty.go:741` `utf8Terminal` reads `LC_ALL`, `LC_CTYPE` or `LANG`, which Windows doesn't set, so anfra always draws ASCII on Windows. On Windows, a console takes Unicode whatever its code page (Go writes to it with `WriteConsoleW`): `utf8Terminal` is true there.
3. **Leaving the console.** "\q, exit or Ctrl-D to leave" (`cmd/anfra/console.go:229`): the console reads keys through bubbletea, which gets Ctrl-D as a key on Windows too, so the hint stays. (The plan's "Ctrl-Z then Enter" was for reading plain lines, which the console doesn't.) Checked by hand below.
4. **The console and the publish prompt, by hand,** in Windows Terminal, a PowerShell 5 window and a `cmd` window: typing, history, Ctrl-C during a query, leaving; the publish prompt's choices and Ctrl-C at it (`cmd/anfra/publish.go:216` `readLines`, `muesli/cancelreader`, which can cancel a read from a Windows console). What breaks is fixed here; what's large is reported first.

**Failures found by the earlier modules (tests that fail on Windows):**

5. **`anfra logs`** (`cmd/anfra`: `TestLogsPretty`, `TestLogsNarrowed`, `TestLogsHintsWhyNoAppReports`, `TestLogsFollow`): **solved before this module** (`d3d2150`). Their output was right; their cleanup failed because the background update check had started a copy of the test program in the test's repo folder, which Windows won't delete while a program works in it. With the check off in `cmd/anfra`'s tests, `TestLogsPretty` passes on Windows; the other three are confirmed with this module's Windows run.
6. **`TestReadLinesEndWithTheirContext`** reads from a pipe, which `cancelreader` can't cancel on Windows (only a console). Prompts read only from a terminal, so the test checks what matters on Unix only, and skips on Windows saying why; item 4's Ctrl-C at the prompt is the Windows check.
7. **Data Apps live reload** (`internal/appserve/watch.go`; `TestWatch`, `TestLiveReloadWatchesOnlyWhileAStreamIsOpen`): on Windows, a file written in a folder also reports the folder as changed, so the reload names apps that didn't change (`""`, `team`). Ignore change events on folders; a new folder is still watched.
8. **Tests that make symlinks** (`internal/appserve` `TestFiles`, `cmd/anfra` `TestInitRefusesASymlinkedGitignore`): Windows lets only administrators, or developer mode, make them. They skip when the symlink can't be made, saying so, as `TestCheckPath` already does.

**Users:** everyone on Windows using anfra in a terminal; and the Windows CI job (`win-release`), which needs these tests to pass.

**Assumptions** (to correct now if wrong):
- Supported terminals: Windows Terminal, a PowerShell 5 window, a `cmd` window (the old console host), and VS Code's terminal. PowerShell 7 behaves like PowerShell 5 here.
- Windows 10 1809 and later (the console host supports escape codes from 1511; earlier modules already need 1803).
- The hand checks (item 4) are run by the user, in each terminal, with steps I give.

**Out of scope:** the Data Apps' own UI (a browser); the skills and docs (`win-skills-docs`); CI (`win-release`).

## Tech Stack

Go 1.27; `golang.org/x/sys/windows` (`GetConsoleMode`, `SetConsoleMode`); charm's lipgloss v2, bubbletea v2, `colorprofile`; `muesli/cancelreader`; `fsnotify` (the watcher). Windows 11 x64, the host from the earlier modules.

## Commands

```sh
# Lightly (GOMAXPROCS=4 GOFLAGS=-p=2), one command at a time
GOMAXPROCS=4 GOFLAGS=-p=2 go test ./internal/cliui/ ./internal/pretty/ ./internal/appserve/ ./cmd/anfra/
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go vet ./internal/cliui/ ./internal/pretty/ ./internal/appserve/ ./cmd/anfra/
GOMAXPROCS=4 tmp/bin/golangci-lint run <packages>          # once per task, per OS

# Windows: one test build per task, run on the host after checking its free memory
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go test -c -o /mnt/c/Users/nuita/wincheck/<pkg>.test.exe ./<pkg>/
```

## Project Structure

| Where | Change |
|---|---|
| `internal/cliui/` (+ a `_windows.go`) | Turn on escape-code processing at startup on Windows; no styles where it can't be |
| `internal/pretty/pretty.go` (+ per-OS) | `utf8Terminal` true on Windows |
| `cmd/anfra/` | `TestReadLinesEndWithTheirContext` and `TestInitRefusesASymlinkedGitignore` per item 6 and 8 |
| `internal/appserve/watch.go` | Folder events ignored |
| `internal/appserve/appserve_test.go` | `TestFiles` skips without symlinks |
| `docs/` | Where a Windows terminal differs, if anything visible does |

## Code Style

anfra's conventions: [naming](docs/dev/naming.md), [readable prose](docs/designs/readable-prose.md), comments only for why; per-OS code in build-tagged files, as `internal/sidecar` does.

## Testing Strategy

- **Linux:** existing tests unchanged; new ones where the logic is shared (the watcher's folder events).
- **Windows** (`_windows_test.go`, built in WSL, run on the host): escape-code processing turned on for a console (or skipped, saying why, when the test isn't run from one); `utf8Terminal` true; the four `anfra logs` tests (item 5, expected to pass already), the two watcher tests and the symlink tests pass or skip with a reason; `internal/appserve` and `cmd/anfra` pass in full on Windows.
- **By hand, on Windows 11, by the user:** in Windows Terminal, a PowerShell 5 window, a `cmd` window and VS Code's terminal: `anfra auth status` and `anfra show` (colors, Unicode trees, no raw codes); `anfra console` (typing, history, Ctrl-C during a slow query, `\q` and Ctrl-D to leave); `anfra publish --set-org …` on a folder without an org (the prompt, a choice, Ctrl-C at it, which must not publish).

## Boundaries

- **Always:** keep macOS and Linux output exactly as today; write no escape codes to a console that can't show them; run Go lightly and check Windows' free memory before running there.
- **Ask first:** changing a library's version (lipgloss, bubbletea, cancelreader, fsnotify); a fix for item 4 larger than about half a day; publishing anywhere.
- **Never:** turn off styles for everyone to fix one terminal; skip a test on Windows without saying why in the skip.

## Success Criteria

1. In a `cmd` and a PowerShell 5 window (the old console host), anfra's colors show, with no raw escape codes; Windows Terminal and VS Code unchanged.
2. On Windows, trees and boxes are Unicode in a console.
3. `anfra console` and the publish prompt work in the four terminals: typing, history, Ctrl-C, leaving with `\q` and Ctrl-D, the prompt's choices, and Ctrl-C at the prompt publishing nothing.
4. `cmd/anfra` and `internal/appserve` pass in full on Windows (tests that need symlinks skip, saying why); Linux unchanged.
5. Data Apps live reload names only the apps that changed, on Windows.
6. `WINDOWS-PLAN.md` updated; docs where a Windows terminal differs.

## Open Questions

None, if the assumptions above hold.
