# Skills in step with the code

> Status: design. Nothing here is built yet. The steps are in [Plan](#plan).

The anfra skills teach a coding agent how to use anfra. They name commands and flags, AQL functions, AML properties, and every call of the Data App SDK. People write and update them by hand, in `skills/`, in the same repo as the code they describe. Nothing checks that they still match that code.

This doc covers how the skills stay correct as anfra changes. It sets one rule, a change updates the skills it affects in the same pull request, and the checks that make the rule hard to forget. It also explains why the skills don't read anfra's source instead.

For where the skills live, how they install and how their versions work, see [agent setup](agent-setup.md). For how they publish, see [release](release.md#the-skills).

## The problem

When a feature changes, its skill has to change too, and that step is easy to miss. An agent then works from the old skill. It calls a method that was renamed, misses one that was added, or passes a flag that no longer exists. The agent can't tell that the skill is wrong. It trusts the skill, and the user sees an app or a query that fails.

This already happens. In 0.19.0, the `App` class has three members that look meant for app authors, and `anfra-build-data-app` mentions none of them: `app.setTimezone()`, `app.appliedSelection` and `app.crossFilters`. Two design docs name the build-data-app skill by an old name or an old location. [numbers in the SDK](sdk-numbers.md) calls it `build-anfra-app` with an `API.md`, and [revamp query result data types](../implementations/revamp-query-result-data-types.md) places its source in `anfra-skills`. These are small, but nothing would have caught them, and nothing will catch the next one.

No rule in this repo says a code change has to update a skill. [DEVELOPMENT.md](../../DEVELOPMENT.md#the-skills) covers how to work on the skills, and `pnpm skills:check` checks their front matter, their links and their build. It doesn't check what they say.

## What a skill teaches, and where that comes from

Each fact a skill teaches comes from somewhere, and that source decides how the fact can be checked.

| What the skill teaches | Example | Comes from | Skills | How to check it |
|---|---|---|---|---|
| The SDK's calls and their shapes | `query.fetchMore()`, `app.mapControl(control, query, { field })` | `web/sdk`, through `ambient.d.ts` | build-data-app | [Surface check](#1-the-sdk-surface-check), [type-check](#2-type-check-the-examples), [run the example](#3-run-the-example) |
| Commands and flags | `anfra query --dataset <fqn>` | The CLI's command tree, and the ops in `api/openapi.yaml` | almost all | [Command check](#4-the-command-check) |
| What `anfra validate` and `anfra serve` enforce on an app | `integrity` on every CDN script, no relative `src` | `internal/appserve` and the validator | build-data-app | [Validate the example](#3-run-the-example) |
| The repo's config files | `apps.allow_origins` in `.anfra/config.yml` | The config loader | setup-repo, build-data-app | Not checked yet ([Later](#later)) |
| AQL and AML | `date_trunc`, `with_relationships`, a relationship's `type` | The pinned canal-query and amql | aql, aml, write-aql | Not checked yet ([Later](#later)) |
| How to work | Agree on the design first, then write the queries and check them | Nobody's code | all | [Evals](#later) |

The first three rows are where most of the churn is, and they come from code in this repo, so this doc checks them first. The language rows (AQL and AML) change when the pinned canal-query changes, which is less often. Checking them needs a sidecar and a fixture repo, so they come later. Advice on how to work can only be checked by giving an agent a task and looking at the result.

## Why the skills don't read the code

Instead of keeping a copy of the API in the skill, the skill could send the agent to the code: the SDK's sources or types on GitHub, or its published `.d.ts`. Then the skill could never be behind. This was considered and turned down for several reasons.

- **Most of a skill isn't in any code.** `anfra-build-data-app` has about 1,700 lines. Only `references/api.md` and the plugin references, about 600 of those lines, describe the SDK. The rest covers the sandbox and its content policy, the rules an app's AQL follows, the design guidance, and how to read the logs. All of that still has to be written by hand.
- **The code doesn't separate what authors call from what the kernel calls.** `App`, `Query` and the controls have public methods that only the SDK's own modules call, such as `mappingsTo`, `selectionConditionsFor`, `signature` and `assertGrain`. An agent reading the classes would take them as API and use them.
- **The comments are written for maintainers.** The JSDoc explains how the kernel works, as in "The kernel's, on the host's word". The skill explains what an author needs: when to use a call, what goes wrong, and what to do instead.
- **GitHub's code is the wrong version.** The SDK an app runs on is the one in the user's `anfra` binary. `main` can be ahead of the binary, so the agent uses a call the user's anfra doesn't have yet. It can also be behind a local build. The skills' version can't be used to find the right code either, because skills and binary have separate versions ([agent setup](agent-setup.md#versions)).
- **Fetching is unreliable for agents.** Some agents' fetch tools return a summary of a page rather than the page, and a summary loses the exact signatures the agent came for. Running `curl` asks the user for permission. A sandbox may have no network at all.

The published package has the right version: `@holistics/anfra-sdk@<the binary's version>` on jsDelivr. It still exposes the kernel's methods, though, and its type files have hashed names that change with every build.

The `anfra-docs` skill does send agents to the web, to anfra.ai and amql.org. That fits its job, which is concepts and guides that the other skills don't cover. Those sites describe the latest release, so they aren't a source for an exact signature either.

So the skills stay written by hand, and the checks below make sure they match the code. If agents later need exact signatures beyond the skill, the binary should print them itself ([Later](#later)).

## The rule

A change that alters what a skill teaches updates that skill in the same pull request. That includes:

- adding, renaming or removing an SDK call, or changing its arguments, result or errors
- adding, renaming or removing a command or flag the skills use
- changing what `anfra validate` or `anfra serve` enforces on a Data App
- bumping canal-query to a version that changes AQL or AML the skills teach

The commit for the skill takes the `skills` scope (`feat(skills): …`). The rule goes in `AGENTS.md` under "Before changing anything" and in [DEVELOPMENT.md](../../DEVELOPMENT.md#the-skills), which links here.

## The checks

Each check runs in `make test`, through `pnpm skills:check` or the Go tests, so CI runs it on every pull request. Each one fails with the file, the name that's wrong, and what to do about it.

### 1. The SDK surface check

This check compares the SDK's author surface with what the build-data-app skill documents, in both directions:

- **A member in the SDK that the skill never names** fails. This catches a feature added without its skill.
- **A member the skill names that the SDK doesn't have** fails. This catches a rename or a removal that left the skill teaching the old name.

**The author surface** is every type reachable from `Sdk`, the type of the `Anfra` global in `web/sdk/ambient.d.ts`, minus what is tagged `@internal` or `@deprecated`. The script reads it with the TypeScript compiler API. `typescript` is already a dev dependency, so the check adds nothing new.

Two things are needed before the check can run:

- **Tag the kernel's methods `@internal`.** Any public member that only the SDK calls gets the tag, such as `mappingsTo`, `selectionConditionsFor`, `selectionExpressionFor`, `datasetIndex`, `signature`, `assertGrain`, `isFieldBacked`, `toInspectJSON` and `toJSON`. That makes the surface the check reads the same as the surface authors are meant to use, and it helps anyone reading the types too.
- **Decide about the three undocumented members.** For `setTimezone`, `appliedSelection` and `crossFilters`, either document them in the skill or tag them `@internal`.

A member that is author API but that the skill leaves out on purpose goes on an allowlist in the script, with the reason beside it. The allowlist should stay short.

The plugins' members (`query.export`, `Anfra.render`, `Anfra.lineage`) are part of the surface. They're compared with the files under `references/plugins/`.

### 2. Type-check the examples

The surface check catches names. It doesn't catch a call with the wrong arguments. This check type-checks the inline scripts of `references/example.html`, and the `js` code blocks in the skill's references, against `ambient.d.ts`, with `checkJs`. The libraries the example loads from a CDN (ECharts, Basecoat) get short stub declarations next to the script. A code block that's a fragment rather than a whole program can be marked to skip, with a comment.

### 3. Run the example

`references/example.html` is the app agents start from, so it has to work. This check loads it on the SDK's real kernel with the `testing` entrypoint and a stub backend. Declaring the queries and wiring the controls must raise no `ValidationError`, and the first `execute()` must send the queries the example declares.

The same check runs `anfra validate` on a repo holding only the example, and expects no `data_app` warnings. That catches a CDN script without `integrity`, or a relative `src`, before an agent copies it.

### 4. The command check

A Go test in `cmd/anfra` reads each `sh` code block in `skills/` and finds every `anfra` command line in it. For each one, it checks that the command exists in the CLI's command tree and that each flag exists on that command. It also fails on a deprecated command or flag. The CLI keeps those working for a release ([commands and API](commands-and-api.md#deprecation-before-removal)), but a skill should move to the new form in the same pull request that deprecates the old one.

Placeholders such as `<fqn>` and `'<aql>'` are fine as arguments. Only command names and flag names are checked.

## Plan

1. Tag the kernel's methods `@internal`, and decide about `setTimezone`, `appliedSelection` and `crossFilters`.
2. Add the rule to `AGENTS.md` and `DEVELOPMENT.md`, and fix the stale skill names in [numbers in the SDK](sdk-numbers.md) and [revamp query result data types](../implementations/revamp-query-result-data-types.md).
3. Add the SDK surface check to `pnpm skills:check`.
4. Type-check the examples.
5. Run the example, and validate it.
6. Add the command check.

Each step is useful on its own. They can ship in one pull request or several.

## Later

- **Check the AQL and AML examples.** The skills hold about 170 `aql` blocks and 400 `aml` blocks. They could be validated with the pinned canal-query against a fixture repo, such as `anfra-aml/references/examples`. That needs the sidecars in CI, a fixture that covers the datasets the examples use, and a way to mark a block as a fragment. It's worth doing when canal-query bumps start breaking examples.
- **Check the config keys.** The keys a skill names in `.anfra/config.yml` and `data_sources.yml` could be checked against the config loader's schema, in the same way as the command check.
- **A record of the surface, and a guard on it.** A committed file could list the SDK's author surface and the command tree. CI would fail when the file is out of date, so a pull request that changes the surface shows the change in a file the reviewer reads. A pull request that changes it without touching `skills/` could be asked to say why in a commit trailer. Checks 1 and 4 already catch most of this, so the guard waits until they turn out to miss something.
- **Evals.** Give an agent with the skills a set of tasks, such as "a paged detail table" or "two charts that cross-filter each other", and check what it builds. They're too slow and costly for every pull request, so they would run before a release. [improve proses](../todos/improve-proses.md) asks for the same thing for the skills' wording.
- **The binary prints the SDK reference.** If agents need exact signatures that the skill doesn't give, `anfra` could print the SDK reference for its own version, offline, as an op, so it's on HTTP and MCP too. It's the only source that always matches the SDK an app runs on.

## Open questions

- Are `app.setTimezone()`, `app.appliedSelection` and `app.crossFilters` author API? If they are, the skill needs to document them. If they aren't, they need the `@internal` tag.
- Should the surface check cover the host-side entrypoints (`/host`, `/plugins`) too? No skill teaches them today.
