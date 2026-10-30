# Agent setup

> Status: proposed. The findings behind it are in [skills install](../research/skills-install.md); this doc says what we build.

anfra's skills, `aql-writer` subagent and hooks live in [holistics/anfra-skills](https://github.com/holistics/anfra-skills), published as a Claude Code plugin marketplace. `anfra skills install` (`cmd/anfra/skills.go`) installs that plugin into Claude Code and Codex, and has three problems:

1. It doesn't reach every agent, such as OpenCode.
2. It says "Installed" and exits 0 when only some of the user's agents got the skills.
3. The skills don't stay up to date, and nothing says when they're behind.

The design in short:

- **Install:** a plugin wherever an agent's CLI installs one. Every other agent is pointed at the skills repo, whose README gives each agent's best install method, a plugin first when the agent supports one. Duplicates are accepted. In an anfra repo, setup also writes the anfra block in `AGENTS.md`.
- **Versions:** both the binary and the skills are nudged to their latest, decoupled from each other. Each skill states its version, and anfra says when either side is behind. A map of which anfra each skills version needs is an optional [phase 2](#phase-2-a-compatibility-map).
- **Updates:** `anfra update` updates the binary, and `anfra agent setup`, run again, the skills; `anfra update` and a daily notice say when the skills need it. Hints reach the agent through the session hook, `AGENTS.md` and an `agent-setup` skill.

## Commands

| Question | Command |
|---|---|
| Where do I stand? Offline and cheap, for agents and hooks, once per session | `anfra version --skills <version>` |
| Update anfra itself | `anfra update` |
| Is a newer anfra out? Asked fresh | `anfra update --check` |
| Install, update or repair the skills for my agents | `anfra agent setup` |
| What state is each agent in, and the `AGENTS.md` block? | `anfra agent setup --check` |

`anfra skills install` is kept as an alias of `anfra agent setup` for now, with a deprecation note.

`version` is an op, so it's also served over HTTP and MCP. Its `--skills` compares with the cached check only: it reads no folders and runs no programs. `update` and `agent setup` are CLI commands only, never ops: they run programs on the user's machine, which a server must not do for its callers.

There's no `anfra agent uninstall`. anfra keeps no install state of its own: every install goes through a route that has its own removal, such as `claude plugin uninstall anfra@anfra-skills` or `npx skills remove`. The skills repo's README gives each route's removal steps.

## Setup

### Choosing agents

`--agent` takes [`npx skills`](https://github.com/vercel-labs/skills)' agent ids (`claude-code`, `codex`, `cursor`, `opencode`, `github-copilot`, …), one or more. For an agent `npx skills` doesn't list, we pick an id in the same form: lowercase, hyphenated.

Without `--agent`, setup takes the agents it detects, either from their CLI on the `PATH` or from their home folder, the way `npx skills` detects them (`~/.claude`, `~/.codex`, `~/.config/opencode`, …). If it detects no agent, it fails and lists the ids. It asks nothing: `--agent` picks a subset, as the CLI has no multi-select prompt yet.

It installs per user, for every project. The former `anfra skills install --scope project`, which wrote the plugin into a repo's `.claude/settings.json`, isn't carried over: the alias refuses it and names `anfra agent setup`. A committed project setting reaches no cloud agent either, as [Cloud agents](#cloud-agents) says.

### Each agent's route

| Agent | Route | What it gets |
|---|---|---|
| Claude Code | anfra runs its CLI: `claude plugin marketplace add`, `marketplace update`, `plugin install` (as today) | Skills, `aql-writer`, hooks |
| Codex | anfra runs its CLI: `codex plugin marketplace add`, `marketplace upgrade`, `plugin add` (as today) | Skills, hooks |
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

Run in an anfra repo (one with `.anfra`), setup also writes the anfra block in `AGENTS.md`, since the block belongs to the repo. Outside a repo it touches no files of the repo. `anfra init` calls the same code.

- **The block** sits between `<!-- anfra:begin -->` and `<!-- anfra:end -->`. Setup adds it to an existing `AGENTS.md`, or creates the file. It replaces only that block, and refuses a symlink, as `init` does for `.gitignore`.
- **`CLAUDE.md`:** when the repo has one, setup adds `@AGENTS.md` to it. Claude Code reads `AGENTS.md` only when no `CLAUDE.md` exists.
- **The block's content** is fixed by the anfra that writes it:

```markdown
<!-- anfra:begin -->
## anfra

This is an anfra repo.

- Use the anfra skills for work in this repo. They know anfra's languages and commands, so you'll work more
  accurately and efficiently with them than from memory.

Keeping the skills working:

- Once per session, before your first anfra command, run `anfra version --skills <version>`, with the version
  from the `anfra skills` line at the end of an anfra skill you've loaded, or `--skills unknown` if it has no such
  line. Follow the hints it prints. If anfra doesn't know `--skills`, it's out of date: ask the user to run
  `anfra update`.
- If the anfra skills aren't available to you, run `anfra agent setup --agent <your id>` and follow what it says,
  then ask the user to restart you. If that fails, install them as https://github.com/holistics/anfra-skills says
  for the agent you are.
<!-- anfra:end -->
```

The block is committed, so cloud agents get it too. It stays short and leaves the details to the skills repo and the [`agent-setup` skill](#the-agent-setup-skill).

### Skill names

Every skill's name starts with `anfra-` (`anfra-aql`, `anfra-write-aql`, …), from the skills' 0.5.0. The reasons are in the research's [What this means](../research/skills-install.md#what-this-means), under "Names clash across vendors":

- **Shared folders hold skills by name.** `npx skills` installs into folders several agents read, such as `~/.agents/skills`, one folder per skill name. Other vendors use our old names too: Holistics' `holistics-development` plugin has `aql`, `write-aql`, `run-aql`, `validate-aql` and `lookup-values`. Two skills of one name overwrite each other there, silently, whichever was installed last. Inside a plugin, names don't clash, but most agents get the skills alone ([Each agent's route](#each-agents-route)).
- **It's what other vendors do.** Supabase (`supabase-*`), Vercel (`vercel-*`), Expo (`expo-*`) and Stripe (`stripe-*`) name their skills after themselves ([Vendor CLIs that install skills](../research/skills-install.md#vendor-clis-that-install-skills)).
- **The source is renamed, not the installed copy.** The spec requires a skill's folder to match its `name`, and the skills refer to one another about 45 times. Renaming at install time would mean rewriting their text on every machine.
- **The prefix marks our skills.** `--check` takes a folder's `anfra-*` skills as a copy, and reads each one's stamp ([The check report](#the-check-report)). Before it, anfra could only guess a folder was ours from one skill name that no other vendor used.
- **Copies of the old names stay behind.** A plugin update replaces its skills, but a copy installed with `npx skills` before 0.5.0 keeps its old folders, which an agent loads next to the new ones. `--check` lists them under "Deprecated skills", and the skills repo's README gives the command that removes them.
- **The cost:** in Claude Code, a plugin's skills are shown as `<plugin>:<skill>`, so `anfra:aql` becomes `anfra:anfra-aql`. Stripe's plugin shows the same.
- **The `aql-writer` agent keeps its name.** Agents come only through plugins, which namespace them, so it never shares a folder with another vendor's.

### Duplicates are accepted

Agents read each other's folders, so a skill can load twice. For example, Codex reads both its plugin and `~/.agents/skills`, and Cursor reads `~/.claude/skills`. The research's [What this means](../research/skills-install.md#what-this-means) has the full table. Setup removes nothing and leaves out no agent to avoid a duplicate:

- **No reliable rule exists.** No agent offers a usable way to turn off one copy, and copies installed by other routes are outside our control anyway.
- **A duplicate costs little:** about 500 tokens of names and descriptions per session.
- **The real risk is drift:** two copies, updated by different mechanisms, at different versions. [`anfra agent setup --check`](#the-check-report) lists each copy it can see and marks any that has fallen behind.

### The report and exit code

Setup runs its installs, writes the `AGENTS.md` block in an anfra repo, then prints the [check report](#the-check-report) of the state it leaves: the same sections, so there's one format to read. After it come setup's own notes, each only when it applies:

```
Restart Claude Code, or run /reload-plugins, to load it.
Codex: failed: `codex plugin add anfra@anfra-skills`: exit status 1
To update a copy of the skills, see https://github.com/holistics/anfra-skills#updating-and-removing.
```

- **The restart note,** for each agent it installed into.
- **A failed step,** or an agent whose CLI isn't on the `PATH`.
- **The pointer for copies,** when a copy is behind: setup updates plugins, not copies.

The `AGENTS.md` section says when setup just wrote the block: `the anfra block is up-to-date (setup just wrote it)`.

It exits non-zero when any install failed, or when an agent named with `--agent` ends up not installed. An agent that was only detected and got pointed at the repo doesn't fail the run. Otherwise anyone with a second agent's folder would see setup fail on every run.

### The check report

`anfra agent setup --check` changes nothing. It has three sections, and in an anfra repo (step 5) a fourth, for the `AGENTS.md` block:

```
$ anfra agent setup --check
Latest anfra skills: 0.4.5, from GitHub (checked today)

Agents:
  Claude Code     plugin, skills  plugin 0.4.4; ~/.claude/skills
  Cursor          skills          ~/.agents/skills, ~/.cursor/skills, ~/.claude/skills
  GitHub Copilot  skills          ~/.agents/skills, ~/.copilot/skills
  OpenCode        none            see https://github.com/holistics/anfra-skills#opencode

Skills:
  Claude Code plugin  behind  0.4.4
  ~/.agents/skills    mixed   anfra-aql, anfra-write-aql at 0.4.4; 6 skills at 0.4.5
  ~/.cursor/skills    latest  0.4.5
  ~/.copilot/skills   behind  0.4.4
  .claude/skills      behind  no version

Deprecated skills, from before 0.5.0:
  ~/.claude/skills
  ~/.cursor/skills
Clean them up as https://github.com/holistics/anfra-skills#updating-and-removing says.

anfra only looks in the folders above, so a copy somewhere else won't be shown.

Run `anfra agent setup` to install or update them.
```

- **The heading** says where the latest version comes from and when it was checked: "checked today", a date, or "not known yet".
- **Agents** says where each agent's skills come from: `plugin`, `skills` (copies in the folders it reads), both, or `none`, with where to install from. It shows duplicates, such as a plugin and a copy in one agent. Which folders each agent reads is data in the agent list, from the agents' docs.
- **Skills** lists each plugin and each copy, by where it is, as `latest`, `behind` or `mixed`, with its version, or which skills are at which version, oldest first. A copy is a folder's `anfra-*` skills, each read for its own stamp, so a partial install is found too. A skill with no stamp is `behind`, with `no version`. Against an unknown latest, the status is `unknown`.
- **Deprecated skills** are copies of the names from before the `anfra-` prefix (`aml`, `aql`, …), found by their old `setup-repo` skill when its text names anfra. An agent loads them next to the new names. The section appears only when there are some, with the README's cleanup steps once.
- **The list of copies** doesn't decide whether the skills in use fit, since it can't tell which copy is in use. That's [the check](#the-check)'s job.
- **What it reads:**
  - the skills folders the known agents read, at home and in the repo, and each `anfra-*` skill's stamp line;
  - the background check's cache, for the latest skills version;
  - the agents' CLIs, for the plugins' installed versions, as setup itself does: their answer is current, and setup runs them anyway.

  It uses no network of its own.

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

- **anfra is behind:** the hint is `anfra update`, which the agent asks the user to run, since it replaces the binary in use. It ends by saying when the skills need `anfra agent setup` too.
- **The skills are behind:** the hint is `anfra agent setup --agent <your id>`. Setup is the one command to give, whatever the agent: for an agent it doesn't install into, it points at the README. When anfra later installs into more agents, such as Junie once its plugin route is tested, the hint stays the same, and no skill, `AGENTS.md` block or README needs rewriting.
- **Any version older than the latest** gets the same hint, on either side, whether it's a patch or a minor release behind.
- **Both current:** no hints.
- **A skills version newer than the cached latest** only means the cache is old. anfra then reports that version as `latest`, since it's the newest it has seen, so it counts as current.
- **Incomplete information is said:**
  - with no check run yet, `skills.latest` is absent, and a hint says "latest not known yet";
  - when the last check is more than two days old, for example when offline, the answer gives its date (`skills.checked: 2026-10-03`).

`skills.latest` is never older than `skills.version`, so a program needs no version comparison:
- `latest` absent: not known yet;
- equal: current;
- different: behind.

`hints` are for agents. `update` keeps its shape, `available` included, since it's an existing contract, and the session hook's parsing of `available: true` still works.

Without `--skills`, the answer is today's, plus `hints` when anfra is behind, so agents that already run plain `anfra version`, such as the `anfra-setup-repo` skill, get that hint too:

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

`--skills unknown` means the version in use isn't known, for example because a copy installed before the stamp existed has no stamp line. The answer is plain `version`'s, plus `skills.latest` and a hint to see the installed copies:

```
$ anfra version --skills unknown
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

`unknown` is a value rather than a bare `--skills`: a flag with an optional value would read `--skills 0.16.0` as `--skills` followed by a stray argument, and an op's input can't tell an empty value from a missing one.

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

`anfra agent setup` is the one place that acts on the skills, and reports them. Everything else only points at it.

- **`anfra update`** updates only the binary: it never runs an agent's CLI. Running `anfra agent setup` again updates the plugins. `anfra update`, and its `--check`, end with [the attention line](#the-attention-line) when something needs it.
- **The background check** is the existing `__update-check`, now twice a day (its cache lasts 12 hours, for anfra's release and the skills' alike):
  - it fetches the latest skills version: the `version` in the skills repo's plugin manifest on `main`;
  - it records the installed plugins and their versions, through `claude plugin list --json` and `codex plugin list --marketplace anfra-skills --json`, in `~/.anfra/agents-check.json`. Setup records them too, after it acts, so the notice is current after it.

  Agent CLIs start slowly (Claude Code's starts Node), so they run here and in `anfra agent setup` only, never on an ordinary command.
- **The `AGENTS.md` block** is committed, so only `anfra agent setup` and `anfra init` write it, never `anfra update`: that would put a change the user didn't ask for into their repo.
- **Every other copy,** such as one installed by `npx skills`, is updated as the skills repo's README says for its agent, never by anfra. Setup's report points there.

### The attention line

One line says what of the skills needs attention, and to run `anfra agent setup`. It names the first two things behind, and counts the rest:

```
anfra skills 0.5.0 are out: the Claude Code plugin and ~/.agents/skills are behind. Run `anfra agent setup`.
anfra skills 0.5.0 are out: the Claude Code plugin and ~/.agents/skills are behind, and 2 more things need attention. Run `anfra agent setup`.
~/.cursor/skills has deprecated anfra skills. Run `anfra agent setup`.
This repo's AGENTS.md block is out of date. Run `anfra agent setup`.
```

- **What it reads:** the plugins as the background check or setup last recorded them; the copies, in the folders the agents read; and the `AGENTS.md` block of the repo it runs in. No network and no agent CLI, so it's cheap enough after any command.
- **Where it shows:**
  - at the end of `anfra update` and `anfra update --check`;
  - as the notice after a command at a terminal, at most once a day, not after `anfra agent setup` itself. It respects `ANFRA_NO_UPDATE_NOTIFIER`. Agents get no notice: the session hook and the `AGENTS.md` block are theirs.
- **Its limits:** the plugins' versions are up to 12 hours old, so an update made in the agent's own CLI shows only after the next check. Outside an anfra repo it can't see any repo's block.

Claude Code's own auto-update is off for our marketplace, because it's a third-party one. See [Open questions](#open-questions).

## Hints

Each channel reaches a different set of agents:

| Channel | Reaches | Says |
|---|---|---|
| The plugin's session hook (`check-update.sh`) | Claude Code and Codex | It already runs `anfra version` for the update notice. It adds `--skills`, with the version from its own stamp line, or alone if there's none, and puts the hints in the session's context when there are any |
| The `AGENTS.md` block | Agents in a repo where setup or `init` wrote it, cloud agents included | See [`AGENTS.md`](#agentsmd) |
| The `agent-setup` skill | An agent setting up or updating the skills, or hitting a version mismatch | See [below](#the-agent-setup-skill) |
| The notice after a command | People, at a terminal | [The attention line](#the-attention-line), at most once a day |

An agent without a session hook and outside a repo with the block hears nothing until a command fails and it loads the `agent-setup` skill. Phase 2's stderr line would reach it sooner, but only for the copies anfra can see.

### The `agent-setup` skill

A new skill in the skills repo, its description covering:
- setting up or updating the anfra skills;
- an anfra command reported as unknown, a flag anfra doesn't take, or output that doesn't match what a skill says.

It carries the detail no other skill does:

- **The check:** run `anfra version --skills <version>`, with the version from the stamp line, or `--skills unknown` if there's none, and what each hint means.
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
- **The names:** every skill is renamed with an `anfra-` prefix (`anfra-aql`, `anfra-write-aql`, …), with the references between them. Other vendors' skills share names such as `aql`, and `npx skills` installs into shared folders, where two skills of one name overwrite each other. The prefix also tells anfra's skills apart in a folder. The README says how to remove a copy of the old names, which an agent would load next to the new ones.

### 2. anfra: the background check learns the skills

In `internal/update`:

- **The cache** gains the latest skills version, and when it was checked. That time is the skills' own: `anfra update` records anfra's check alone.
- **`Refresh`,** which `__update-check` runs, fetches `plugins/anfra/.claude-plugin/plugin.json` from the skills repo's `main` (`LatestSkills`), from `raw.githubusercontent.com` rather than GitHub's API: the API allows 60 anonymous calls an hour, shared by everyone behind one address. It reuses `get`. One check failing doesn't stop the other.
- **A failure is cached as unknown,** never as a version, so a failed fetch can't count as "current".
- **`serve`'s hourly `Check`** goes by anfra's check alone, so it never asks GitHub for anfra's release because of the skills.

The plugins' installed versions aren't here: listing them takes the agents' CLIs, which step 4's agent list knows. They come with step 6, the first step that needs them.

### 3. anfra: `version --skills`

- **`status.Version`** (`internal/command/status`) takes an input with an optional `skills`, a version or `unknown`, in place of `NoInput`. Its result gains `skills` (`version`; `latest`, never older than `version`; and `checked` when the cache is more than two days old) and `hints`. `update` keeps its shape.
- **`CommandContext`** gains a function that reports the cached skills version, set in `cmd/anfra/host.go` next to `knownUpdate`. So the op reads only the cache, the same way it reads `Update` today.
- **Tests** cover: both sides current; each side behind; a patch release behind, which gets the same hint as a minor; nothing cached; a stale cache; a skills version newer than the cached latest; `--skills unknown`; no `--skills` at all.

### 4. anfra: `anfra agent setup`

`internal/agents` holds the logic, and `cmd/anfra/agent.go` replaces `cmd/anfra/skills.go`:

- **The agent list:** for each agent, its id, how it's detected (a CLI on the `PATH`, or a home folder, including one an environment variable names), its route, the skills folders it reads at home and in a repo, and its README anchor. Claude Code and Codex keep today's steps, and Claude Code's add `plugin update`, so running setup again upgrades.
- **Per user only:** no `--scope`. The `skills install` alias keeps a hidden `--scope` only to refuse `project`, naming `anfra agent setup`.
- **Selection:** `--agent`, else the detected agents. No prompt.
- **The report and exit code,** as [described above](#the-report-and-exit-code).
- **`--check`,** reporting without changing anything, as [The check report](#the-check-report) describes: the plugins through the agents' CLIs, the copies in the folders the agents read, by their `anfra-*` skills' stamps, and the deprecated copies of the old names.
- **Only the agent CLIs it installs through run:** the system's commands are built by literal name (`claude`, `codex`), whatever reaches them.
- **`anfra skills install`** becomes an alias, printing "This command is deprecated. Use `anfra agent setup` instead."
- **The docs that named the old command:** anfra's `README.md` quickstart and `install.sh`'s next steps say `anfra agent setup`. Their prompts name the skills by their new names (`anfra-build-data-app`, `anfra-setup-repo`), which ship with the skills' 0.5.0.

The tests fake the machine: the home folder, the CLIs on the `PATH`, and what the agents' CLIs answer.

### 5. anfra: the `AGENTS.md` block

In `internal/agents` (`agentsmd.go`), used by setup and `anfra init` alike:

- **Writing the block:** add it to `AGENTS.md`, creating the file, or replace the marked block that's there; add `@AGENTS.md` to an existing `CLAUDE.md`, and create none. A symlink is refused, as it would write outside the repo.
- **Setup** writes it in an anfra repo, with one line after the agents: `wrote the anfra block`, `…, and @AGENTS.md in CLAUDE.md`, `the anfra block is up-to-date`, or `failed: …`, which fails the run.
- **`--check`** adds an `AGENTS.md:` section: the block is `up-to-date`, `out of date` (other than what this anfra writes) or `missing`, and `CLAUDE.md doesn't import it (@AGENTS.md)` when that's so.
- **`anfra init`** lists `AGENTS.md`, as `created` when it wrote the block or the import, else `kept`.

This comes after steps 3 and 4, since the block names `version --skills` and `agent setup`.

### 6. anfra: what needs attention, and where it shows

- **Setup's report** reuses the check report, then its own notes, as [described above](#the-report-and-exit-code).
- **The attention line** (`agents.Attention`), as [described above](#the-attention-line), at the end of `anfra update` and its `--check`, and as the notice after a command, at most once a day (`agents.Notice`).
- **The plugins recorded** (`agents.RecordPlugins`, `~/.anfra/agents-check.json`), by the background check and by setup, for the line to read without starting the agents' CLIs.
- **The background check twice a day:** its cache lasts 12 hours.
- **`anfra update` doesn't update plugins:** setup is the one place that acts on the skills.

### 7. Skills repo: the hook

`hooks/check-update.sh`:
- read the stamp from `${CLAUDE_PLUGIN_ROOT}`;
- run `anfra version --skills <version>`, and if anfra rejects the flag, run plain `anfra version` as today;
- add the hints to the session's context.

`anfra-setup-repo`'s `SKILL.md`: its line "`anfra skills install` installs these skills…" names `anfra agent setup` instead.

This waits for the anfra release that has steps 3 and 4: the hook and the skill name commands that must exist.

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
- **Missing and extra skills in a copy.** `--check` reports only the `anfra-*` skills a folder has. A partial install (`npx skills add … -s anfra-write-aql`), a skill a newer release added, or one it removed goes unnoticed. The fix:
  - the skills repo's build writes the release's list of skills next to its manifest, such as `plugins/anfra/skills.json` (`{"version": …, "skills": […]}`), checked against the folders;
  - the background check fetches that file in place of `plugin.json`, still one request, and caches the list with the version;
  - `--check` compares each copy with it: a copy at the latest version that lacks skills is `partial`, and a copy that's behind says what it's missing; a skill no longer in the release is listed as extra. A plugin can't be partial, since its agent installs it whole.

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
