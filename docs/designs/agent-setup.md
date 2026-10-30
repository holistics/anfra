# Agent setup

> Status: proposed. The findings behind it are in [skills install](../research/skills-install.md); this doc says what we build.

anfra's skills, `aql-writer` subagent and hooks live in [holistics/anfra-skills](https://github.com/holistics/anfra-skills), published as a Claude Code plugin marketplace. `anfra skills install` (`cmd/anfra/skills.go`) installs that plugin into Claude Code and Codex, and has three problems:

1. It doesn't reach every agent, such as OpenCode.
2. It says "Installed" and exits 0 when only some of the user's agents got the skills.
3. The skills don't stay up to date, and nothing says when they're behind.

The design in short:

- **Install:** a plugin wherever an agent's CLI installs one. Every other agent is pointed at the skills repo, whose README gives each agent's best install method, a plugin first when the agent supports one. Duplicates are accepted. In an anfra repo, setup also writes the anfra block in `AGENTS.md`.
- **Versions:** both the binary and the skills are nudged to their latest, decoupled from each other. Each skill states its version, and anfra says when either side is behind. A map of which anfra each skills version needs is an optional [phase 2](#phase-2-a-compatibility-map).
- **Updates:** `anfra update` updates both the binary and the plugins anfra installed. Hints reach the agent through the session hook, `AGENTS.md` and an `agent-setup` skill.

## Commands

| Question | Command |
|---|---|
| Where do I stand? Offline and cheap, for agents and hooks, once per session | `anfra version --skills <version>` |
| Make everything current: the binary, and the plugins anfra installed | `anfra update` |
| What would `anfra update` change? Asked fresh | `anfra update --check` |
| Install or repair the skills for my agents | `anfra agent setup` |
| What state is each agent in, and the `AGENTS.md` block? | `anfra agent setup --check` |

`anfra skills install` is kept as an alias of `anfra agent setup` for now, with a deprecation note.

`version` is an op, so it's also served over HTTP and MCP. Its `--skills` compares with the cached check only: it reads no folders and runs no programs. `update` and `agent setup` are CLI commands only, never ops: they run programs on the user's machine, which a server must not do for its callers.

There's no `anfra agent uninstall`. anfra keeps no install state of its own: every install goes through a route that has its own removal, such as `claude plugin uninstall anfra@anfra-skills` or `npx skills remove`. The skills repo's README gives each route's removal steps.

## Setup

### Choosing agents

`--agent` takes [`npx skills`](https://github.com/vercel-labs/skills)' agent ids (`claude-code`, `codex`, `cursor`, `opencode`, `github-copilot`, …), one or more. For an agent `npx skills` doesn't list, we pick an id in the same form: lowercase, hyphenated.

Without `--agent`, setup takes the agents it detects, either from their CLI on the `PATH` or from their home folder, the way `npx skills` detects them (`~/.claude`, `~/.codex`, `~/.config/opencode`, …). At a terminal it first shows a multi-select with the detected agents checked. If it detects no agent, it fails and lists the ids.

`--scope user` (the default) or `--scope project`, as today. Codex installs plugins only at user scope.

### Each agent's route

| Agent | Route | What it gets |
|---|---|---|
| Claude Code | anfra runs its CLI: `claude plugin marketplace add`, `marketplace update`, `plugin install` (as today) | Skills, `aql-writer`, hooks |
| Codex | anfra runs its CLI: `codex plugin marketplace add`, `marketplace upgrade`, `plugin add` (as today), at user scope only | Skills, hooks |
| Every other agent, Cursor included | anfra installs nothing. It points at the skills repo's install section for that agent | Whatever that method installs |

A route that installs runs the agent's own CLI, so new features and fixes in that CLI reach us without changes here. Run again, setup updates what it installed.

For the other agents, setup prints one line from the same template, filled in with the agent's name and id. It names no install command:

```
<Agent>   not installed: ask <Agent> to install the anfra plugin, or the skills if it takes no plugin, as
          https://github.com/holistics/anfra-skills#<id> says
```

One action and one link: the line names the preference, and the README section gives the steps.

The skills repo's README has a section for each agent in anfra's list, anchored by the agent's id, so the steps can change without an anfra release. Adding an agent to that list means adding its README section in the same change. For an id outside the list, setup links to `#other-agents` instead: a README section that gives the general order for any agent. So every link lands on a section that exists, rather than opening the README at its top without explanation. Each section gives the best install method for that agent, in this order:

1. **The agent's own plugin install, if it takes ours.**
   - Cursor installs plugins from its marketplace panel.
   - Copilot (in VS Code, and its CLI) and Junie install plugins in Claude Code's format, so their sections give that method, from their docs.
2. **Skills only:** `npx skills add holistics/anfra-skills -a <id> -g -y`. A "Skills vs plugins" section says what the skills alone lack: the `aql-writer` agent and the hooks.

One note at the top of the Install section covers every agent's steps: agents change how they install often, so if a step doesn't work, follow the agent's own docs.

The README's "Other agents" section says the same order for an agent with no section of its own: a plugin through your own plugin system if it accepts Claude Code's format, otherwise the skills. An agent reading it, sent there by setup's output or by the [`AGENTS.md` block](#agentsmd), can then install itself.

### More plugins, in order

"Plugin" means three different things across agents, so each kind of agent comes in its own step. The shares are from the [Stack Overflow 2026 survey](../research/skills-install.md#which-agents):

| Order | Agents | Their plugin format | Work |
|---|---|---|---|
| 1 | Copilot in VS Code (Copilot: 58.7%), Junie (JetBrains AI: 12.8%), Devin Local | Read Claude Code's format | A test of our existing plugin in each, and nothing else |
| 2 | Antigravity (16.0%), Gemini CLI (via Gemini Code Assist: 14.6%) | A manifest of their own: Antigravity's `plugin.json`, Gemini CLI's extensions | A manifest generated from the Claude Code plugin, plus hooks under each agent's event names |
| Later, if at all | OpenCode (13.6%), Cline (4.5%) | Code (JS/TS) that reacts to events, and can't carry skills or subagents | Code of our own, for the AML validation hook alone. OpenCode documents no way to add context at session start, so the update notice can't go there. These agents stay skills-only |

As each agent's plugin works:
- **The README** moves it to a plugin method.
- **Setup** runs that method itself, if the agent has a CLI that installs plugins (`agy plugin install`, Junie's `/extensions`, `devin plugins install`, `gemini extensions install`), and the agent moves into the [route table](#each-agents-route).

The hint stays `anfra agent setup` either way.

How to install or update the skills for an agent anfra doesn't install into is never written into anfra. anfra's output points at the README, which owns those steps.

### `AGENTS.md`

Run in an anfra repo (one with `.anfra`), setup also writes the anfra block in `AGENTS.md`, whatever the scope, since the block belongs to the repo. Outside a repo it touches no files of the repo. `anfra init` calls the same code.

- **The block** sits between `<!-- anfra:begin -->` and `<!-- anfra:end -->`. Setup adds it to an existing `AGENTS.md`, or creates the file. It replaces only that block, and refuses a symlink, as `init` does for `.gitignore`.
- **`CLAUDE.md`:** when the repo has one, setup adds `@AGENTS.md` to it. Claude Code reads `AGENTS.md` only when no `CLAUDE.md` exists.
- **The block's content** is fixed by the anfra that writes it:

```markdown
<!-- anfra:begin -->
## anfra

This is an anfra repo: models in AML, queries in AQL, run with the `anfra` CLI. Use the anfra skills for this work.

- Once per session, before your first anfra command, run `anfra version --skills <version>`, with the version from
  the `anfra skills` line at the end of an anfra skill you've loaded, or `--skills` alone if the skill has no such
  line, and follow its hints. If anfra doesn't know
  `--skills`, it's out of date: ask the user to run `anfra update`.
- If the anfra skills aren't available to you, run `anfra agent setup --agent <your id>` and follow what it says,
  then ask the user to restart you. If that fails, install them as https://github.com/holistics/anfra-skills says
  for the agent you are.
<!-- anfra:end -->
```

The block is committed, so cloud agents get it too. It stays short and leaves the details to the skills repo and the [`agent-setup` skill](#the-agent-setup-skill).

### Duplicates are accepted

Agents read each other's folders, so a skill can load twice. For example, Codex reads both its plugin and `~/.agents/skills`, and Cursor reads `~/.claude/skills`. The research's [What this means](../research/skills-install.md#what-this-means) has the full table. Setup removes nothing and leaves out no agent to avoid a duplicate:

- **No reliable rule exists.** No agent offers a usable way to turn off one copy, and copies installed by other routes are outside our control anyway.
- **A duplicate costs little:** about 500 tokens of names and descriptions per session.
- **The real risk is drift:** two copies, updated by different mechanisms, at different versions. [`anfra agent setup --check`](#the-check-report) lists each copy it can see and marks any that has fallen behind.

### The report and exit code

Setup ends with one line per selected agent, giving one of these results:

- installed, with the version;
- already current;
- not installed, with where to install it from;
- failed, with the step that failed and its error.

It never says "installed" for an agent it only pointed at the repo. In an anfra repo, one more line says whether it wrote or refreshed the `AGENTS.md` block.

It exits non-zero when any install failed, or when an agent named with `--agent` ends up not installed. An agent that was only detected and got pointed at the repo doesn't fail the run. Otherwise anyone with a second agent's folder would see setup fail on every run.

### The check report

`anfra agent setup --check` changes nothing. It reports the same agents setup would act on, then every copy of the skills it can see, listed by where it is, not by agent, since one folder can serve several agents. In an anfra repo, it also reports the `AGENTS.md` block:

```
$ anfra agent setup --check
Agents:
  Claude Code   plugin, user scope   0.15.0   behind
  Codex         plugin               0.16.0   current
  OpenCode      not installed        see https://github.com/holistics/anfra-skills#opencode

anfra skills found (latest 0.16.0, checked today):
  ~/.agents/skills   0.14.1   behind
  .claude/skills     0.16.0   current
anfra only looks in the folders above, so a copy somewhere else won't be shown.

AGENTS.md: the anfra block is out of date.

Run `anfra agent setup` to fix these.
```

- **The block's state** is a comparison of its content with what this anfra would write, as Convex does with its `AGENTS.md` section. The block has no version stamp.
- **A copy with no stamp** is listed as "no version, older than the stamp", so behind.
- **The list of copies** doesn't decide whether the skills in use fit, since it can't tell which copy is in use. That's [the check](#the-check)'s job.
- **What it reads:**
  - the skill folders anfra knows, such as `~/.agents/skills`, `.agents/skills`, `~/.claude/skills` and `.claude/skills`, and each copy's stamp line;
  - the background check's cache, for the latest skills version and the plugins' installed versions.

  It uses no network and runs no agent CLI.

## Versions

Both the binary and the skills should be at their latest, and anfra nudges toward both. If both are, they fit: each skills release is written against the anfra released by then. A mismatch comes from one side lagging, and the fix is to update that side:

| Mismatch | Breaks when | Fix |
|---|---|---|
| Skills ahead of the binary | A skill uses a command or flag the user's anfra doesn't have | `anfra update`, which today's update notice already suggests |
| Skills behind the binary | anfra removed or changed something a skill uses | Update the skills. The [deprecation period](#deprecation) keeps older skills working meanwhile |

Telling a mismatch that breaks from one that doesn't needs a map of which anfra each skills version needs. That's [phase 2](#phase-2-a-compatibility-map), optional.

### Which copy to check

Only the agent knows which skills it loaded. anfra can't find that copy on its own:

- a copy may be in a folder anfra doesn't know, installed by another route;
- the folders where plugins are stored aren't documented;
- an agent that reads several folders may have loaded any of the copies in them.

So the version comes from the copy in use: `anfra version --skills <version>`, with the version from that copy's stamp line. The agent passes it, as the `AGENTS.md` block asks. A plugin's session hook passes it too: the hook knows its own folder (`${CLAUDE_PLUGIN_ROOT}`), so it reads the stamp line there.

### The skills' stamp

The skills repo keeps one version for all its skills: the `version` in its plugin manifests, bumped on every release. Claude Code needs that bump to see an update. The repo's build writes it at the end of each `SKILL.md`'s body, under a rule:

```markdown
…the skill's last paragraph.

---
anfra skills 0.16.0
```

- **The rule** sets the line apart as metadata, not part of the skill's last instruction.
- **The blank line before the rule** is required: a `---` directly under text makes that text a heading.
- **The shape is fixed:** the file ends with `---` and then `anfra skills <version>`. anfra and the hook match exactly that, and the skills repo's test checks it.
- **Not an HTML comment,** which GitHub would hide: no agent documents passing comments to the model, and the agent needs to see the line.

That stamp is all a skill carries: a few tokens, with no instructions. The instruction to use it is in the [`AGENTS.md` block](#agentsmd) and the [`agent-setup` skill](#the-agent-setup-skill). The same line serves anfra, the session hook, and the agent, which reads only the body: whether frontmatter reaches the model isn't documented.

### Deprecation

A command or flag keeps working, with a deprecation warning, for at least one minor release before it's removed or changed. That's the default for every command, whether skills use it or not: people and their scripts call the CLI too. For the skills, it means a copy a release or two behind still works.

The binary keeps no minimum skills version. Under "always the latest", any copy older than the latest gets the same advice, whether it still works or not, so a minimum would only change how loudly anfra says it. It would also need raising by hand on every breaking change.

### The check

`anfra version --skills <version>` compares both sides with the latest the background check has cached. It adds `skills` to `version`'s answer, and a `hints` list that holds only what needs acting on:

```
$ anfra version --skills 0.15.0
version: 0.15.2
update:
  latest: 0.16.1
  available: true
skills:
  version: 0.15.0
  latest: 0.16.0
hints:
  - anfra 0.16.1 is out: ask the user to run `anfra update`.
  - anfra skills 0.15.0 are behind the latest, 0.16.0: run `anfra agent setup --agent <your id>`.
```

- **anfra is behind:** the hint is `anfra update`, which the agent asks the user to run, since it replaces the binary in use. It updates the plugins too, so no separate skills step follows.
- **The skills are behind:** the hint is `anfra agent setup --agent <your id>`. Setup is the one command to give, whatever the agent: for an agent it doesn't install into, it points at the README. When anfra later installs into more agents, such as Junie once its plugin route is tested, the hint stays the same, and no skill, `AGENTS.md` block or README needs rewriting.
- **Any version older than the latest** gets the same hint, on either side, whether it's a patch or a minor release behind.
- **Both current:** no hints.
- **A skills version newer than the cached latest** only means the cache is old. anfra then reports that version as `latest`, since it's the newest it has seen, so it counts as current.
- **Incomplete information is said:**
  - with no check run yet, `skills.latest` is absent, and a hint says "latest not known yet";
  - when the last check is more than two days old, for example when offline, the answer gives its date (`checked: 2026-10-03`).

`skills.latest` is never older than `skills.version`, so a program needs no version comparison:
- `latest` absent: not known yet;
- equal: current;
- different: behind.

`hints` are for agents. `update` keeps its shape, `available` included, since it's an existing contract, and the session hook's parsing of `available: true` still works.

Without `--skills`, the answer is today's, plus `hints` when anfra is behind, so agents that already run plain `anfra version`, such as the `setup-repo` skill, get that hint too:

```
$ anfra version
version: 0.15.2
update:
  latest: 0.16.1
  available: true
hints:
  - anfra 0.16.1 is out: ask the user to run `anfra update`.
```

It has no `skills`, and `hints` is absent when there's nothing to act on.

`--skills` with no version means the version in use isn't known, for example because a copy installed before the stamp existed has no stamp line. The answer is plain `version`'s, plus `skills.latest` and a hint to see the installed copies:

```
$ anfra version --skills
version: 0.15.2
update:
  latest: 0.15.2
  available: false
skills:
  latest: 0.16.0
hints:
  - To see which copies of the anfra skills are installed, and their versions, run `anfra agent setup --check`.
```

anfra doesn't guess that these skills are behind: a missing stamp can have other causes, such as a copy someone edited, or an agent that didn't find the line. `anfra agent setup --check` shows the facts: a copy it finds with no stamp is listed as older than the stamp, so behind.

As an op, the input tells the two apart: `skills` missing is plain `version`, and `skills: ""` means the version isn't known. On the CLI, `--skills` takes an optional value.

Today's hook keeps working, with one constraint on field order:
- It reads `version:` and `available:` at a line's start, and a hint line starts with `- anfra`, so neither can match by mistake.
- It reads `latest:` at any indentation and takes the first match, so it would also match `skills.latest`. `skills` must therefore come after `update` in `VersionResult`, whose field order the YAML output follows.

An anfra too old to know `--skills` fails with an unknown flag. The `AGENTS.md` block and the `agent-setup` skill both say what that means: anfra is behind, so ask the user to run `anfra update`.

### Phase 2: a compatibility map

Optional, and only if phase 1 turns out not to be enough. Phase 1 can't tell a mismatch that breaks from one that doesn't, so it nudges toward the latest without urgency. A map would add that:

- **The map:** the skills repo keeps the oldest anfra each skills version needs, raised by whoever makes a skill use a newer command. Its build publishes the whole history as one file at the repo's root, such as `{ "0.16.0": "0.15.0", "0.15.0": "0.14.0" }`, and the background check caches it.
- **"Incompatible":** the check then also says when the skills in use need a newer anfra than the one running. That's the one case the agent can't work around and the user must act on.
- **A stderr line from any anfra command** when a copy anfra can see is incompatible, printed even when stderr isn't a terminal, since agents read stderr. Never for a copy that's just behind.
- **The table in the `agent-setup` skill,** for an anfra too old to know `--skills`, to compare with `anfra version` by hand.
- **An unknown version,** one the cached map doesn't have yet, means anfra can't judge compatibility until its next check.

## Updates

- **The background check** is the existing daily `__update-check` (`internal/update`), extended to:
  - fetch the latest skills version: the `version` in the skills repo's plugin manifest on `main`;
  - list installed plugins and their versions, through `claude plugin list --json` and `codex plugin list --marketplace anfra-skills --json`.

  Agent CLIs start slowly (Claude Code's starts Node), so they run here, in `anfra update` and in `anfra agent setup` only, never on an ordinary command.
- **`anfra update`** replaces the binary when a newer one is out, then updates each plugin anfra installed, even when the binary was already current: `claude plugin marketplace update anfra-skills` and `claude plugin update anfra@anfra-skills`, and Codex's `plugin marketplace upgrade`.
  - At a terminal it says what it will run first.
  - It ends by saying that updated skills load after the agent restarts.
  - `--no-skills` skips the plugins.
  - There's no `--skills` for the plugins alone: running `anfra agent setup` again does that.
  - `ANFRA_AUTO_UPDATE` updates only the binary: it doesn't run other tools' CLIs in the background.
- **`anfra update --check`** reports what `anfra update` would change, asked fresh: the latest release from GitHub, the latest skills version, and the installed plugins' versions from their CLIs.
- **The `AGENTS.md` block** is committed, so `anfra update` never edits it: that would put a change the user didn't ask for into their repo. When the block is out of date, `update` says so and names `anfra agent setup`.
- **Every other copy,** such as one installed by `npx skills`, is updated as the skills repo's README says for its agent, never by anfra. The hints say to run `anfra agent setup`, which points there for such an agent.

Claude Code's own auto-update is off for our marketplace, because it's a third-party one. See [Open questions](#open-questions).

## Hints

Each channel reaches a different set of agents:

| Channel | Reaches | Says |
|---|---|---|
| The plugin's session hook (`check-update.sh`) | Claude Code and Codex | It already runs `anfra version` for the update notice. It adds `--skills`, with the version from its own stamp line, or alone if there's none, and puts the hints in the session's context when there are any |
| The `AGENTS.md` block | Agents in a repo where setup or `init` wrote it, cloud agents included | See [`AGENTS.md`](#agentsmd) |
| The `agent-setup` skill | An agent setting up or updating the skills, or hitting a version mismatch | See [below](#the-agent-setup-skill) |
| The terminal notice | People | Extends today's update notice: newer skills are available, from the cache |

An agent without a session hook and outside a repo with the block hears nothing until a command fails and it loads the `agent-setup` skill. Phase 2's stderr line would reach it sooner, but only for the copies anfra can see.

### The `agent-setup` skill

A new skill in the skills repo, its description covering:
- setting up or updating the anfra skills;
- an anfra command reported as unknown, a flag anfra doesn't take, or output that doesn't match what a skill says.

It carries the detail no other skill does:

- **The check:** run `anfra version --skills <version>`, with the version from the stamp line, or `--skills` alone if there's none, and what each hint means.
- **For an anfra too old to know `--skills`:** anfra is behind, so ask the user to run `anfra update`.
- **The agent ids** `anfra agent setup --agent` takes, so an agent can find its own. Without `--agent`, setup covers every agent it detects, which also includes this one.
- **Each route's install and update commands,** and the order to prefer them in, matching the README, for when setup fails.
- **What to do with each hint:**
  - ask the user to run `anfra update`, rather than running it, because it replaces the binary in use and a running `anfra serve` keeps the old version;
  - run `anfra agent setup`;
  - after an install or update, ask the user to restart the agent.

## Cloud agents

A cloud agent reads neither the user's home folder nor a gitignored project install. Its environment's setup script installs anfra and runs `anfra agent setup --agent <id>`, or the skills repo's method for that agent, and gets the latest of both on each fresh start. Claude Code's cloud sessions don't load plugins that a repo's `.claude/settings.json` turns on, so the setup script is the route there. Updates in the cloud are advisory: the `AGENTS.md` block reports a side that's behind, and the user fixes the setup script.

## Implementation plan

The work spans two repos: anfra, and [anfra-skills](https://github.com/holistics/anfra-skills). Each step is one pull request, in this order, because later steps print links and commands that earlier ones create. Every anfra step that changes what a command takes or answers regenerates the API contract and the SDK's types (`scripts/openapi.sh generate`, `pnpm --filter @holistics/anfra-sdk generate`).

### 1. Skills repo: the stamp and the README

- **The stamp:** `scripts/bump.js` already sets every manifest's `version` in lockstep. It also writes the [stamp](#the-skills-stamp), a rule and `anfra skills <version>`, at the end of each `SKILL.md`'s body, replacing an existing one. A test checks that every skill ends with the stamp in that shape, matching `.claude-plugin/plugin.json`.
- **The README:**
  - a section for each agent in anfra's list, anchored by its id, giving the install methods in order: plugin first, then the skills alone;
  - an `#other-agents` section, with the same order in general terms;
  - update and removal steps for each route.

### 2. anfra: the background check learns the skills

In `internal/update`:

- **The cache** gains the latest skills version, and when it was checked. That time is the skills' own: `anfra update` records anfra's check alone.
- **`Refresh`,** which `__update-check` runs, fetches `plugins/anfra/.claude-plugin/plugin.json` from the skills repo's `main` (`LatestSkills`), from `raw.githubusercontent.com` rather than GitHub's API: the API allows 60 anonymous calls an hour, shared by everyone behind one address. It reuses `get`. One check failing doesn't stop the other.
- **A failure is cached as unknown,** never as a version, so a failed fetch can't count as "current".
- **`serve`'s hourly `Check`** goes by anfra's check alone, so it never asks GitHub for anfra's release because of the skills.

The plugins' installed versions aren't here: listing them takes the agents' CLIs, which step 4's agent list knows. They come with step 6, the first step that needs them.

### 3. anfra: `version --skills`

- **`status.Version`** (`internal/command/status`) takes an input with an optional `skills`, in place of `NoInput`, telling missing (plain `version`) from empty (version not known). On the CLI, `--skills` takes an optional value. Its result gains `skills` (`version`, and `latest`, never older than `version`), `checked` when the cache is more than two days old, and `hints`. `update` keeps its shape.
- **`CommandContext`** gains a function that reports the cached skills version, set in `cmd/anfra/host.go` next to `knownUpdate`. So the op reads only the cache, the same way it reads `Update` today.
- **Tests** cover: both sides current; each side behind; a patch release behind, which gets the same hint as a minor; nothing cached; a stale cache; a skills version newer than the cached latest; `--skills` with no version; no `--skills` at all.

### 4. anfra: `anfra agent setup`

`cmd/anfra/agent.go` replaces `cmd/anfra/skills.go`:

- **The agent list:** for each agent, its id, how it's detected (a CLI on the `PATH`, or a home folder), its route, and its README anchor. Claude Code and Codex keep today's steps.
- **Selection:** `--agent`, else the detected agents, shown as a multi-select at a terminal.
- **The report and exit code,** as [described above](#the-report-and-exit-code).
- **`--check`,** reporting without changing anything, using the copies-found list from the cache and the known folders.
- **`anfra skills install`** becomes an alias, printing a deprecation note.

The tests extend today's: `skillsEnv` already fakes the `PATH` and the agent CLIs.

### 5. anfra: the `AGENTS.md` block

- **Writing the block:** add or replace the marked block, refuse a symlink, add `@AGENTS.md` to an existing `CLAUDE.md`. Shared by `anfra init` and setup, beside `init`'s `ignoreLine`.
- **Setup runs it** in an anfra repo, and `--check` compares the block with what this anfra would write.

This comes after steps 3 and 4, since the block names `version --skills` and `agent setup`.

### 6. anfra: `anfra update` covers the skills

- **After the binary,** update the plugins anfra installed, even when the binary is already current. Say first what it will run, at a terminal, and end with the restart note.
- **`--no-skills`.**
- **`--check`** reports the plugins too, asked fresh from their CLIs.
- **A stale `AGENTS.md` block:** say so and name `anfra agent setup`. Never edit the block.
- **The terminal notice** (`update.Notice`) also says when newer skills are out. That needs the plugins' installed versions, so the background check also lists them, through the agents' CLIs, for whichever agents are on the `PATH`.

### 7. Skills repo: the hook

`hooks/check-update.sh`:
- read the stamp from `${CLAUDE_PLUGIN_ROOT}`;
- run `anfra version --skills <version>`, and if anfra rejects the flag, run plain `anfra version` as today;
- add the hints to the session's context.

This waits for the anfra release that has steps 3 and 4: the hook names commands that must exist.

### 8. Skills repo: Cursor's plugin

A hooks file in Cursor's format, and paths in its manifest if Cursor needs them. Then a test of the plugin, installed from Cursor's UI. If it can't be made to work, the README's Cursor section gives the skills method.

### 9. Policy: deprecation

Add the rule to [commands and API](commands-and-api.md), for every command and flag: each keeps working, with a deprecation warning, for at least one minor release before it's removed or changed.
- **Ops** already need a breaking change declared, through CI's contract check. This adds the warning period before it.
- **CLI-only commands,** such as `update`, `init` and `agent setup`, aren't in the contract, so the rule is their only protection.

### Later

- **The [`agent-setup` skill](#the-agent-setup-skill),** once the commands it describes are released and in use.
- **More plugins,** in the [order described above](#more-plugins-in-order). Each lands in the README first, then in setup's agent list.
- **Phase 2's compatibility map,** if phase 1 isn't enough.
- **Claude Code's auto-update,** once an [open question](#open-questions) is settled.

## Not doing

- **Coupling the skills to the binary's version** (a lock file, or skills picked per anfra version). The check in both directions covers mismatches. Copies installed by other routes, and cloud agents, would bypass coupling anyway.
- **Writing hooks or subagents into agents' own config.** They come only through plugins. Agents without a plugin get skills only, so they don't get the AML validation hook. `write-aql` already falls back to a generic subagent when `aql-writer` is missing.
- **Removing duplicates.** See [Duplicates are accepted](#duplicates-are-accepted).
- **An uninstall command.** Each route removes its own copies.
- **A separate `anfra agent status`.** `anfra version --skills` answers agents and hooks, and `anfra agent setup --check` answers people.
- **Listing in Anthropic's official marketplace** (`claude-plugins-official`). It doesn't keep up with its plugins' latest versions.

## Open questions

- **Claude Code auto-update,** which is off by default for a third-party marketplace like ours. The ways left to turn it on:
  - **Anthropic's directory:** submission through a portal, a review for each version, and the plugin syncs to signed-in users. Each review delays each release.
  - **Writing `autoUpdate: true` into `~/.claude/plugins/known_marketplaces.json`,** with the user's consent, as `npx plugins` does. That file isn't documented.

  Until one of these is in place, `anfra update` drives Claude Code's updates.
- **Cursor's plugin needs fixing and a test.** Our `.cursor-plugin/plugin.json` lists no paths, but [Cursor's docs](https://cursor.com/docs/plugins) say components "are discovered from their default directories", so it probably finds `skills/`. What's likely broken:
  - **Hooks:** `hooks/hooks.json` is in Claude Code's format. Cursor's events and output differ (`sessionStart`, `additional_context`), and its docs don't say where a plugin's hooks file goes.
  - **`aql-writer`:** the docs don't name a default folder for agents.

  The fix is a Cursor-format hooks file, and paths in the manifest if they're needed. Each needs a test in Cursor, which installs plugins only from its UI.
- **Which plugin parts each agent in the first plugin step actually loads.** Each one documents that not every component may work, and `aql-writer` and the hooks matter most.
- **Junie's `/extensions` is an in-session command,** so setup may not be able to run it from a shell. Not checked.
