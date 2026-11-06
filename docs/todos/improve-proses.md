# TODO: make anfra's prose readable

anfra's user-facing text is often hard to read, for people and for agents. [Readable prose](../designs/readable-prose.md) describes the mistakes and how to avoid them. This plan is for fixing the text that already exists.

## What to fix, in order

Text is ranked by how many readers see it and how much they depend on it to act. Agents come first where they read text that people rarely see.

| # | Text | Where it lives | Roughly how much | Why this rank |
|---|---|---|---|---|
| 1 | Error messages and help | `apperr` codes' default messages, `apperr.New*` call sites, `fmt.Errorf` that reaches the user | 16 codes, ~40 call sites, a share of ~140 `fmt.Errorf` | People and agents read these when they're stuck, and have to act on them. |
| 2 | Command descriptions and `doc` tags | `Summary` and `Description` in `internal/command`, `doc:"…"` tags | ~20 commands, ~200 tags | Agents choose commands and build inputs from these, through MCP and the OpenAPI contract. |
| 3 | Warnings, notices and hints | `cmd/anfra/notices.go`, `internal/agents/notice.go`, `hints` in results | ~55 hint sites, a handful of notices | They're shown unasked, so they have to be short and clear. |
| 4 | CLI help | `Short`, `Long` and flag usages in `cmd/anfra` | ~30 commands, ~80 flags | The first thing a new user reads. Progressive disclosure matters most here. |
| 5 | SDK errors and appserve UI text | `web/sdk/src`, `web/appserve/src` | ~40 errors | Data App authors and their agents see these. |
| 6 | Skills | `skills/plugins/anfra/skills` | 8 skills, ~190 files | Agent-facing, but large. They probably need their own pass, checked with agent evals. |
| 7 | User docs | `README.md`, `skills/README.md`, docs.anfra.ai | — | Mostly written for people. They're read less often than errors and help. |
| 8 | Design docs and code comments | `docs/`, code | — | Secondary. Fix when a doc is next touched, not in a sweep. |

## Steps

1. **Land the guide.** Merge `docs/designs/readable-prose.md`, and point to it from `AGENTS.md` so agents writing new text follow it.
2. **Agree on the words we teach.** Go through anfra's own words that show up in user-facing text, such as "answer", "sidecar" and "warm server". For each one, decide whether to teach it or write a plain word instead. The ones we teach go in the guide's [words we teach](../designs/readable-prose.md#words-we-teach) table, with where each is explained. The rest are decided row by row during rewrites, by the guide's [examples](../designs/readable-prose.md#words).
3. **Write the extraction script.** It collects each kind of text into a review table ([below](#how-to-fix-and-review)). It's `scripts/prose`, and `go run ./scripts/prose` writes the tables into `tmp/prose/`, which git ignores. Each kind of text gets a folder, with one file per area of the code, of at most 30 entries.
4. **Fix one kind of text at a time,** in the order of the table above, one PR each, as described below.
5. **Read each main command end to end.** Fixing by kind keeps wording consistent, but it can't check progressive disclosure. For `query`, `publish` and `agent setup`, read the summary, `--help`, the common errors and the hints in the order a reader meets them. Check that each step leads to the next.
6. **Check with an agent, on the top errors only.** Give an agent without the skills a task that fails, such as a missing data source or a wrong flag. Run it with the old text and the new, and see whether the text leads it to the fix. Where a denser, agent-style version reads well too, try it, and add what works to the guide ([stay open to how agents write](../designs/readable-prose.md#considerations)). This is costly, so it checks the approach rather than every row.
7. **Keep it from coming back.** Decide whether to keep the extraction script as a snapshot ([below](#a-catalog-of-prose)). Either way, consider a test that flags semicolons in error messages and `doc` tags, since it's cheap and catches the most common habit. The rest stays with review.

## How to fix and review

Review the text in a table, not in Go diffs. In a diff, each string is cut off from where the reader meets it, format verbs hide what's actually printed, and the reviewer switches between code and prose on every line. A table lets reviewers read the prose as prose, and spot repeated habits. Applying the approved wording to the code is then mechanical.

For each kind of text:

1. **Extract.** The script writes one row per piece of text:

   | Where | Seen when | Shown as | Now | Proposed | Verdict |
   |---|---|---|---|---|---|
   | `repoconfig.go:234` | Two anfras change config at once, or a crash left the lock | CLI error, API error | `.anfra/config.yml.lock exists: another anfra is changing config.yml, or one stopped midway; remove it if none is running` | `Can't change .anfra/config.yml, because .anfra/config.yml.lock exists. Another anfra may be changing it. If none is running, delete the lock file and try again.` | |

   **Seen when** matters most. Without the reader's situation, nobody can judge whether the text serves them. Fill in format verbs with realistic values, or use the real output where a test already triggers the text.
2. **Triage.** An agent marks each row with the guide's checklist: *fine*, *fix*, *not user-facing*, or *needs context*. A person checks a sample of the marks. Only the *fix* rows go on to be rewritten. This should drop most `fmt.Errorf` messages, and any text that already reads well.
3. **Calibrate on about 10 rows.** Rewrite a varied handful and review them together closely. A disagreement here should change the guide, not just the row. The agreed rows become examples for the rest.
4. **Rewrite in batches.** An agent rewrites 20 to 30 rows at a time, given the guide, its words we teach and the calibrated examples. The reviewer reads only Now and Proposed, and fills in Verdict: approve, reword or skip.
5. **Apply.** A script replaces each approved Now with its Proposed at its location. It fails if the old text isn't there exactly once. Then:
   - Update the tests that match on the old text.
   - If a `doc` tag or command description changed, regenerate the API contract and the SDK's types (`scripts/openapi.sh generate`, `pnpm --filter @holistics/anfra-sdk generate`).

The tables aren't committed. They're reviewed locally, before the code changes, and the PR then holds only the code. Reviewing its diff means checking that it matches the approved tables. The PR description shows a few before-and-after examples, so other reviewers can judge the result too.

## A catalog of prose

We won't move the text into a catalog that code refers to by ID. That would take the text away from the code that raises it, where its writer knows the reader's situation. Most of anfra's text is used once, and nothing needs translating. It would also make every new message harder to write, for agents too. And part of a catalog already exists: error codes are defined in one place with their default messages, and command descriptions and `doc` tags end up in the checked-in API contract.

Instead, we may keep the extraction script as a generated catalog. It would write a checked-in snapshot of all the user-facing text, which CI keeps up to date, as it does the API contract. The text stays in the code, but any change to it also shows up in one readable diff. That snapshot is also where a check on the words we teach, or a semicolon lint, would run.

It would cover only text with a known shape: error codes, `apperr` call sites, `doc` tags, command descriptions, CLI help and flags, hints and notices. Ad-hoc `fmt.Errorf` messages would make it noisy, so they stay out.

**Decide after the first kind of text.** If extraction was reliable and the table helped review, keep the script and add the snapshot. If extraction needed a lot of fixing by hand, drop the script and leave it to review.

## Found along the way

Problems the sweep turned up that a rewrite can't fix. They're for the backlog, not for the prose PRs.

- **User mistakes come back as `internal_server_error`.** Over the API and MCP, the reader then sees only the generic internal error. Examples: `anfra search` with no query, `--set-org` with a bad URL, a data source without a `connection`, and a held lock on `.anfra/config.yml`.
- **The CLI's error printer splits a message at `: `** to show its causes a line each (`messageParts` in `cmd/anfra/errors.go`). A part of fewer than 4 words stays with the one before, so most `: ` inside a sentence stay joined, and only a longer clause after one is split. It's cosmetic, and if it matters, the printer's heuristic is what should change, not the messages.
- **`anfra logs` is coming** (something like `anfra logs --filter --follow`). When it lands, text that points to the repo's log file should name that command instead. That includes the help on `sidecar_unavailable` errors (`codeHelp` in `cmd/anfra/errors.go`), which says the logs are in the file `anfra status` shows.
- **A sidecar that crashes as it starts still takes 10 to 15 seconds to fail.** `WaitReady` polls the health check until its deadline without watching whether the process has exited. The reader waits for nothing, and then gets "didn't start within" instead of the crash.
- **Text the extraction misses:** cobra's built-in help (such as `anfra completion`'s), the SDK and appserve, and the skills.

## Open questions

- **Is a message change a breaking change?** [errors.md](../designs/errors.md) says messages are never parsed and may change, so it shouldn't be. Code and field names don't change in this work.
- **Who owns the skills' wording?** If they're tuned against evals, rewriting them may need its own plan.
- **What's in scope for `fmt.Errorf`?** Many are internal wraps that only show up in a cause chain. Only the ones a user sees should change.
