# Installing the anfra skills

> Status: research; nothing built. The checks below were run on 2026-10-28 against skills 1.7.4, plugins 1.3.5 and Claude Code 2.1.312, and each agent's docs as they were that day. Tests ran in throwaway home directories.

The anfra skills live in [holistics/anfra-skills](https://github.com/holistics/anfra-skills), as one plugin, `anfra`: eight skills (`aml`, `aql`, `write-aql`, `validate-aql`, `run-aql`, `lookup-values`, `setup-repo`, `build-data-app`), the `aql-writer` subagent (which reaches Claude Code only; see [What this means](#what-this-means)), and two hooks: one validates an AML file after each edit, one tells the agent at session start that a newer anfra is out. The repo carries a manifest for each of Claude Code, Codex and Cursor.

`anfra skills install` (`cmd/anfra/skills.go`) installs the plugin through `claude plugin` and `codex plugin`, for whichever of the two CLIs is on the `PATH`. For Cursor it prints steps, and for any other agent a hint: `npx skills add holistics/anfra-skills`.

## Problems

1. **It does not reach every agent.** Only Claude Code and Codex get the skills installed. OpenCode, and every other agent, get a hint at most.
2. **It says "Installed" and exits 0 when only one of the user's agents got them.** It reports on the agents it handled, and is silent about the ones it skipped.
3. **The skills don't stay up to date, and nothing says when they're behind.** Running it again upgrades them, but nothing prompts that. Claude Code's auto-update is off for our marketplace by default.

## Updates

The plan:
- each skill declares the anfra it needs;
- `anfra update` updates the skills along with the binary;
- anfra hints when they're out of step.

What remains open is how anfra finds the installed copies to check. That depends on the install approach: see [Checking, by approach](#checking-by-approach).

### Declaring the anfra a skill needs

Each skill folder carries `anfra.json`, written when the skills are released, never by an installer, so it reads the same however the skills were installed. Whose release writes it depends on [where the skills live](#where-the-skills-live): the skills repo's, or anfra's build.

```json
{"version": "0.3.0", "anfra": ">=0.14.0"}
```

- **A file, not frontmatter.** anfra reads JSON without parsing YAML, and the file holds structured fields. The agent never reads it. Other tools that edit frontmatter, such as `gh skill`, can't touch it. `npx skills` copies the whole skill folder, extra files included (tested).
- **In every folder.** Our installer installs every skill at once, so reading any one copy is enough. Copies in every folder keep the check working for someone who installed only some skills by hand (`npx skills add … --skill aml`). The file also marks a folder as ours, so the installer knows which folders it may replace or remove.
- **Picking the skills for an anfra:** a small index on the skills repo's `main` maps each skills tag to its `anfra` range. anfra installs the newest tag whose range includes its own version, as the VS Code marketplace does with `engines.vscode`. This handles breaking changes on both sides: an old anfra gets old skills that still work with it, and an updated anfra moves its skills forward. If the skills' sources move into this repo (see [Where the skills live](#where-the-skills-live)), no index is needed: each anfra version has skills of the same version, downloaded for it or built into it (see [Where anfra gets them](#where-anfra-gets-them)).

### Where the skills live

This is separate from how the skills are installed: it decides how installed skills match the running anfra. There are two choices:
- where the skills' sources live;
- where anfra gets them from when it installs.

#### Where the sources live

- **Their own repo, as today:**
  - The skills are released on their own, so a skill fix ships without an anfra release.
  - anfra picks a tag through the index (see [Declaring the anfra a skill needs](#declaring-the-anfra-a-skill-needs)).
- **This repo.** Each anfra version has skills of the same version, so there's no index to keep, and the update check is an equality. A command change and its skill text land in the same commit. A skill fix waits for an anfra release, unless skills-only releases are added (below). The skills can live here in two forms:
  - **As the skills themselves:** the files here are what agents install.
    - Claude Code still needs a marketplace repo: `anfra-skills` would keep `marketplace.json`, with a `git-subdir` source pinned to a tag. Not checked: whether Claude Code then clones only that folder or the whole repo.
    - A raw `npx skills add holistics/anfra` clones all of anfra.
  - **As templates, with the generated output in `anfra-skills`:**
    - How it works:
      - On each anfra release, a build turns the templates into what each agent installs: the skill folders with their `anfra.json`, the `anfra` and `anfra-hooks` plugins, and each agent's manifest and marketplace files.
      - It publishes the result to `anfra-skills`, tagged with the same version.
      - `anfra-skills` is generated only, never edited by hand: changes go to anfra.
    - For:
      - **One source, every format.** Each agent wants its own form, and the build writes each from one source:
        - `aql-writer` in Markdown for Claude Code, TOML for Codex, `.agent.md` for Copilot;
        - a hook script per agent's events and payloads;
        - `plugin.json` for each of Claude Code, Codex, Cursor, Antigravity and Devin.

        Today the repo keeps three manifests by hand. Per-agent variants, such as `${CLAUDE_PLUGIN_ROOT}` in a hook, become template parameters.
      - **Versions match by name.** `anfra-skills` tag `v0.15.0` is the skills for anfra 0.15.0. The build stamps that version into every `anfra.json`.
      - **Every install route keeps working,** since each needs a repo of its own rather than all of anfra: `npx skills add holistics/anfra-skills`, Claude Code's marketplace, Junie's and Devin's plugin installs, and `AGENTS.md`'s "install them yourself".
    - Against:
      - A template system and a build to write and keep.
      - CI that publishes to a second repo, which needs a token for it.
      - Contributors must know not to edit `anfra-skills`. Its README and CI should say so, and refuse hand edits.

#### Where anfra gets them

Whichever of the above, `anfra skills install` needs the skills for its own version:

- **Built into the binary:**
  - For: no network at install. The skills are small, 191 files and about 157 KB compressed.
  - Against: the installed skills change only with a new binary, so a skills-only fix can't reach an existing anfra.
- **Downloaded when installing,** for the running anfra's version:
  - From where: an asset on that anfra release (say `anfra-skills-0.15.0.tar.gz`, published by the same pipeline as the binaries), or the matching `anfra-skills` tag's archive. `anfra update` already downloads release assets through GitHub's API (`internal/update`), so this reuses that path.
  - Needs network, but so does installing the Claude Code plugin, which fetches its marketplace anyway.
  - Allows skills-only releases. anfra can take the newest skills release for its version (`v0.15.0`, then `v0.15.0-skills.1`), with no new binary.
- **Both:** download by default, and fall back to a copy in the binary when offline. The copy grows the binary by its size, and the fallback can be older than the latest skills release.

### Updating

- **`anfra update`** installs the skills for the new version after replacing the binary, the same way `anfra skills install` does (below). At a terminal it asks first; without one it only prints the hint.
- **`anfra skills install`** run again also updates the skills.
- **Claude Code's plugin** updates through `claude plugin marketplace update` and `claude plugin update`, run by both commands. Claude Code's own auto-update is [off for a third-party marketplace by default](https://code.claude.com/docs/en/plugins/host-marketplace), and `marketplace.json` has no field to turn it on. Only the user, under Marketplaces in `/plugin`, or an admin through managed settings can. `npx plugins` turns it on by writing `"autoUpdate": true` into `~/.claude/plugins/known_marketplaces.json`, a file Claude Code doesn't document. Whether we do the same is an [open question](#open-questions).

### Hints

- **Any anfra command, with no network:** anfra reads the installed `anfra.json` files in each skill folder under the locations it knows (`~/.agents/skills`, the repo's `.agents/skills`, the Claude Code plugin's install path). If their `anfra` range excludes the running anfra, it prints one line to stderr even when stderr isn't a terminal: agents run anfra commands and read stderr, so it reaches them. Today's update notice stays silent off a terminal, and "newer skills are available" can stay quiet the same way.
- **The background update check** (`update.Refresh`) also fetches the skills index, so the notice at a terminal can say when newer compatible skills exist.
- **`anfra version`** reports the installed skills' version and whether it fits.
- **Claude Code and Codex session hook:** `check-update.sh` already runs `anfra version`. It passes the plugin's version (from `${CLAUDE_PLUGIN_ROOT}`), and anfra says whether they match.

### Checking, by approach

Every route that installs a copy is one more place where a version must be found, read and kept current. Where each route keeps its version:

| Route | Where the installed version is | How anfra reads it | What updates it |
|---|---|---|---|
| The shared folder, or an agent's own skills folder | `anfra.json` in each skill folder | Directly, with no network; cheap enough for every command | anfra only (or `npx skills update`) |
| Claude Code plugin | `anfra.json` in the plugin's install folder | That folder's path comes from `claude plugin list --json` (documented, but starts Node: too slow for every command) or `~/.claude/plugins/installed_plugins.json` (undocumented). The plugin's session hook knows its own folder (`${CLAUDE_PLUGIN_ROOT}`), so it is the best place for the check | Claude Code, when auto-update is on; also `anfra update` |
| Codex plugin | Under `~/.codex/plugins/cache/<marketplace>/<plugin>/<commit>`, as seen in the `npx plugins` test; undocumented | Not checked: whether `codex plugin` lists versions, or whether a Codex hook knows its own folder | Codex's marketplace upgrade; also `anfra update` |
| Other agents' plugins, through `npx plugins` (Copilot CLI, VS Code…) | Each agent's own plugin store | Each agent's CLI or internal files: one more lookup per agent | Each agent |
| Hook files we write ourselves | The hook script, which is code and goes stale like a skill | A version marker in each file we write | anfra only |

What this means for each approach:

- **A** has the most places to look. Its duplicates mean two versions of one skill can be installed at once: the plugin's copy, updated by the agent, and the shared folder's copy, updated by anfra. The check has to read both and say when they differ, not only whether each fits anfra.
- **B** keeps the skills in one place, readable directly, plus the Claude Code plugin and the hook files we write, each with its own marker.
- **C** has two: the shared folder and the Claude Code plugin.
- **D** has the same two for the skills. Its `anfra-hooks` plugins carry hooks and `aql-writer`, which go stale too. They're versioned by their plugin's version, and the hooks' own session check can report it.
- **E** installs only the plugins, which update through their agents and `anfra update`. Copies installed from the printed steps sit wherever the steps put them. The check finds them only in the folders it knows, so the steps should use those folders.

Two points hold for every approach:

- **The Claude Code plugin can update past what anfra supports.** Auto-update follows whatever the marketplace points at. If that is `main`, the plugin can move beyond the running anfra's range. Pinning the marketplace to a tag (`holistics/anfra-skills#<tag>`) stops that, and `anfra update` then moves the pin. Not checked: how an existing marketplace's pin is changed, whether by removing it and adding it again.
- **How the skills are installed matters too.** If the skills' sources live in this repo, so their version always matches anfra's (see [Where the skills live](#where-the-skills-live)), the check is just whether the version equals the running anfra's. If the npx tools install them, anfra reads our `anfra.json`, not the tools' lock files, whose format is theirs to change.

## AGENTS.md from `anfra init`, the catch-all

The installer can fail, Node can be missing, the agent can be one we don't handle, or the skills may not have been installed at all. A short `AGENTS.md` that `anfra init` writes reaches the agent anyway, since most agents read it:

```markdown
# anfra

This repo is an anfra repo: models in AML, queries in AQL, Data Apps in `apps/`, run with the `anfra` CLI.
Use the anfra skills for this work (anfra-aml, anfra-write-aql, anfra-build-data-app, …).

If they aren't available to you:
1. Run `anfra skills install`, then ask the user to restart you.
2. If that fails, install them yourself as https://github.com/holistics/anfra-skills#install says for the agent you are, then ask the user to restart you.
3. Until then, read them from https://github.com/holistics/anfra-skills/tree/main/plugins/anfra/skills
```

- **Step 2 points at the skills repo's install steps** rather than naming a command. The right command depends on the agent: the plugin for Claude Code (`claude plugin marketplace add`, `claude plugin install`), the agent's own route for Codex (see [Approaches](#approaches)), `npx skills` only for agents with no plugin. A skills-only install where a plugin exists loses the hooks and the subagent. In Claude Code it also writes `~/.claude/skills`, which Cursor and OpenCode read too, and which would duplicate the plugin's skills once the plugin is installed. Keeping the steps in that README lets them change without rewriting every repo's `AGENTS.md`. The README needs a section for OpenCode and the other agents; it covers Claude Code, Cursor and Codex today.

- **Claude Code** [reads `AGENTS.md`](https://code.claude.com/docs/en/memory) (v2.1.277 or later) only when no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md` exists in the working directory or above it. When the repo has one, `anfra init` adds `@AGENTS.md` to it, which is the import Claude Code's docs recommend. Whether Codex, Cursor and OpenCode read `AGENTS.md` wasn't checked here.
- **An existing `AGENTS.md`** gets a marked section added, never a rewrite, the way `init` adds a line to `.gitignore`, and it refuses a symlink the same way.
- **The links point at `main`,** not a tag: `init` runs once and the repo outlives that anfra. The exact tag for the running anfra comes from `anfra skills install`, which prints the same three steps when it fails, with that tag, and with the command for each agent it couldn't install into.

## Installation

### Facts

#### Which agents

By the [Stack Overflow 2026 survey](https://survey.stackoverflow.co/2026/ai/data/ai-code-agent), the share of 12,255 agent users who used each tool in the past year:

| Agent | Used | Rank |
|---|---|---|
| Claude Code | 65.5% | 1 |
| GitHub Copilot | 58.7% | 2 |
| OpenAI Codex | 29.5% | 3 |
| Cursor | 25.5% | 4 |
| Google Antigravity | 16.0% | 5 |
| Gemini Code Assist | 14.6% | 6 |
| OpenCode | 13.6% | 7 |
| JetBrains AI | 12.8% | 8 |
| Windsurf | 6.5% | 9 |
| Warp | 5.2% | 10 |
| Cline | 4.5% | 11 |

[JetBrains' 2026 survey](https://blog.jetbrains.com/research/2026/08/ai-coding-agent-adoption-2026/) ranks the first four the same way, for use at work.

The tables below cover all eleven. How some survey names map to rows:

- **GitHub Copilot** has two rows: Copilot in VS Code and Copilot CLI. They read different folders.
- **JetBrains AI** is Junie, JetBrains' own agent. [AI Assistant](https://www.jetbrains.com/help/ai-assistant/agents.html) is now mainly a host for other agents (Junie, Claude Agent, Codex, Copilot, any agent over ACP), so most of its users are covered by those rows.
- **Antigravity** covers the IDE, the App and Antigravity CLI (`agy`). Their user folders differ.
- **Gemini Code Assist**'s agent mode in VS Code [runs Gemini CLI](https://docs.cloud.google.com/gemini/docs/codeassist/agent-mode), so the Gemini CLI row applies there. The IntelliJ side doesn't document skills, subagents or hooks. Gemini CLI itself isn't in the survey's list, and was replaced by Antigravity CLI for unpaid-tier users on 2026-06-18, according to its docs. Google's [migration guide](https://antigravity.google/docs/cli/gcli-migration) shows Antigravity CLI doesn't keep Gemini CLI's file layout.
- **Windsurf** is now Devin Desktop, which runs two agents with different config:
  - Cascade, the legacy agent, kept "through July";
  - Devin Local, the default for new tabs, built on the Devin CLI.

#### What each agent supports

Paths are written one way throughout: user paths from `~`, project paths from the repo root, folders without a trailing slash. Each cell lists the agent's own location first, then the ones it reads for compatibility, marked "compat:".

**User scope**

| Agent | Skills | Subagents | Hooks | MCP servers |
|---|---|---|---|---|
| Claude Code | `~/.claude/skills`; not `~/.agents/skills` (docs and test) | `~/.claude/agents` (Markdown) | `~/.claude/settings.json` | `~/.claude.json`, through `claude mcp add --scope user` |
| Copilot in VS Code | `~/.copilot/skills`, `~/.agents/skills`; compat: `~/.claude/skills` | `~/.copilot/agents` (`.agent.md`); compat: `~/.claude/agents` | `~/.copilot/hooks/<name>.json`, `~/.copilot/settings.json` | The VS Code profile's `mcp.json` (key `servers`), `~/.copilot/mcp-config.json`, through `code --add-mcp` |
| Copilot CLI | `~/.copilot/skills`, `~/.agents/skills` | `~/.copilot/agents` (`.agent.md`) | `~/.copilot/hooks/<name>.json` | `~/.copilot/mcp-config.json`, through `copilot mcp add` |
| Codex | `~/.agents/skills` | `~/.codex/agents` (TOML) | `~/.codex/hooks.json` | `~/.codex/config.toml`, under `[mcp_servers.<name>]`, through `codex mcp add` |
| Cursor | `~/.agents/skills`, `~/.cursor/skills`; compat: `~/.claude/skills`, `~/.codex/skills` | `~/.cursor/agents` (Markdown); compat: `~/.claude/agents`, `~/.codex/agents` | `~/.cursor/hooks.json` | `~/.cursor/mcp.json`; no command |
| Antigravity | IDE and App: `~/.gemini/config/skills` (the IDE also reads the old `~/.gemini/antigravity/skills`); CLI: `~/.gemini/antigravity-cli/skills`. Not `~/.agents/skills` | `~/.gemini/config/agents` (Markdown; App and CLI) | `~/.gemini/config/hooks.json` | `~/.gemini/config/mcp_config.json`; `/mcp` in the CLI, no shell command |
| OpenCode | `~/.config/opencode/skills`, `~/.agents/skills`; compat: `~/.claude/skills` | `~/.config/opencode/agents` (Markdown, `mode: subagent`) | Code in a plugin, in `~/.config/opencode/plugins` | `~/.config/opencode/opencode.json`, under `mcp`; no command |
| Junie | `~/.junie/skills`, `~/.agents/skills` | `~/.junie/agents`, `~/.agents` itself (Markdown) | `~/.junie/config.json`, under `hooks` (CLI only, Early Access) | `~/.junie/mcp/mcp.json`; `/mcp` only, no shell command |
| Windsurf: Cascade | `~/.codeium/windsurf/skills`, `~/.config/devin/skills`, `~/.agents/skills`; compat: `~/.claude/skills`, if reading Claude Code's config is turned on | None | `~/.codeium/windsurf/hooks.json` | `~/.config/devin/mcp_config.json`; no command |
| Windsurf: Devin Local | Not checked separately | `~/.config/devin/agents` (Markdown; experimental) | `~/.config/devin/config.json`; compat: `~/.claude/settings.json`, `~/.claude.json` | `~/.config/devin/mcp_config.json`, through `devin mcp add` |
| Warp | `~/.agents/skills`, `~/.warp/skills`; compat: `~/.claude/skills`, `~/.codex/skills`, `~/.cursor/skills`, `~/.gemini/skills`, `~/.copilot/skills`, `~/.opencode/skills`, `~/.factory/skills`, `~/.github/skills` | None locally (cloud agents only) | Not documented | `~/.warp/.mcp.json`; compat: `~/.claude.json`, `~/.codex/config.toml`, `~/.agents/.mcp.json` (started only if `file_based_mcp_enabled` is on) |
| Cline | `~/.cline/skills`, `~/.agents/skills` | `~/.cline/agents` (CLI and app only; format not documented) | Code in an SDK plugin, in `~/.cline/plugins` | CLI: `~/.cline/data/settings/cline_mcp_settings.json`, through `cline mcp add`; the IDE extensions' path isn't documented |
| Gemini CLI | `~/.gemini/skills`, `~/.agents/skills` | `~/.gemini/agents` (Markdown) | `~/.gemini/settings.json`, under `hooks` | `~/.gemini/settings.json`, under `mcpServers`, through `gemini mcp add --scope user` |

**Project scope**

| Agent | Skills | Subagents | Hooks | MCP servers |
|---|---|---|---|---|
| Claude Code | `.claude/skills`, and in subfolders | `.claude/agents` | `.claude/settings.json` | `.mcp.json`, approved before use |
| Copilot in VS Code | `.github/skills`, `.agents/skills`; compat: `.claude/skills` | `.github/agents`; compat: `.claude/agents` | `.github/hooks/<name>.json`, `.github/copilot/settings.json`; compat: `.claude/settings.json` | `.vscode/mcp.json` (key `servers`); `.mcp.json` behind an experimental setting |
| Copilot CLI | `.github/skills`, `.agents/skills`; compat: `.claude/skills` | `.github/agents` | `.github/hooks/<name>.json` | `.mcp.json`, `.github/mcp.json`, after the user trusts the folder |
| Codex | `.agents/skills`, from the working directory up to the repo root | `.codex/agents` | `.codex/hooks.json` | `.codex/config.toml`, in trusted projects only |
| Cursor | `.agents/skills`, `.cursor/skills`; compat: `.claude/skills`, `.codex/skills` | `.cursor/agents`; compat: `.claude/agents`, `.codex/agents` | `.cursor/hooks.json` | `.cursor/mcp.json` |
| Antigravity | `.agents/skills`; `.agent/skills`, kept for backward compatibility | `.agents/agents` (App and CLI) | `.agents/hooks.json` | `.agents/mcp_config.json` |
| OpenCode | `.opencode/skills`, `.agents/skills`; compat: `.claude/skills`; up to the git root | `.opencode/agents` | Code in a plugin, in `.opencode/plugins` | `opencode.json`, under `mcp` |
| Junie | `.junie/skills`, `.agents/skills` (trusted projects only); offers to import `.claude/skills`, `.cursor/skills`, `.codex/skills` | `.junie/agents`, `.agents` itself; offers to import `.claude/agents`, `.cursor/agents`, `.codex/agents` | `.junie/config.json`, ignored by default | `.junie/mcp/mcp.json` |
| Windsurf: Cascade | `.devin/skills`, `.windsurf/skills`, `.agents/skills`; compat: `.claude/skills`, if reading Claude Code's config is turned on | None | `.devin/hooks.json`, else `.windsurf/hooks.json` | None |
| Windsurf: Devin Local | Not checked separately | `.devin/agents`, `.agents/agents` | `.devin/hooks.v1.json`, `.devin/config.json`; compat: `.claude/settings.json` | `.devin/mcp_config.json` |
| Warp | `.agents/skills`, `.warp/skills`; compat: the same eight folders as at user scope, from the working directory up to the repo root | None | Not documented | `.warp/.mcp.json`; compat: `.mcp.json`, `.codex/config.toml`, `.agents/.mcp.json`; approved before use |
| Cline | `.cline/skills`, `.clinerules/skills`, `.agents/skills`; compat: `.claude/skills` | `.cline/agents` | Code in an SDK plugin, in `.cline/plugins` | Not documented |
| Gemini CLI | `.gemini/skills`, `.agents/skills` | `.gemini/agents` | `.gemini/settings.json` | `.gemini/settings.json` |

**Hook events we use**

| Agent | At session start, adds context | After an edit |
|---|---|---|
| Claude Code | `SessionStart` | `PostToolUse` |
| Copilot in VS Code | `sessionStart`, through `additionalContext` | `postToolUse` |
| Copilot CLI | `sessionStart`; adding context not confirmed for the CLI | `postToolUse` |
| Codex | `SessionStart` | `PostToolUse`; the user must trust non-managed hooks in `/hooks` before they run |
| Cursor | `sessionStart`, through `additional_context` | `postToolUse` can add context; `afterFileEdit` returns nothing |
| Antigravity | `SessionStart`, through `injectSteps` | `PostToolUse`, matching its file-writing tools |
| OpenCode | `session.created` fires, but no documented way to add context | `file.edited`, `tool.execute.after` |
| Junie | No: `SessionStart` ignores `additionalContext`, and shows `systemMessage` to the user only | No event after an edit; `PreToolUse` fires before one |
| Windsurf: Cascade | No session-start event | `post_write_code` |
| Windsurf: Devin Local | `SessionStart`, through `additionalContext` | `PostToolUse` |
| Warp | No hooks documented | No hooks documented |
| Cline | `beforeRun` in an SDK plugin, through `appendContext` | `afterTool`, not specific to edits |
| Gemini CLI | `SessionStart` | `AfterTool` |

**Plugins**

| Agent | Installed through | Can carry |
|---|---|---|
| Claude Code | `claude plugin install`, from a marketplace; a plugin's skills are named `<plugin>:<skill>` | Skills, subagents, hooks, MCP servers (`.mcp.json`, or `mcpServers` in `plugin.json`) |
| Copilot in VS Code | Preview, behind `chat.plugins.enabled`: the Extensions view (`@agentPlugins`), Chat: Install Plugin From Source, or `/plugin install` | Skills, subagents, hooks, MCP servers. Reads Copilot's, Claude Code's and the OpenPlugin formats; not every component works in every client |
| Copilot CLI | `copilot plugin marketplace add`, then install | Not checked |
| Codex | `codex plugin add`, from a marketplace | Skills, hooks, MCP servers; subagents not documented |
| Cursor | Its marketplace panel only, after an admin imports a team marketplace; no command | Not checked. The docs mention team plugins carrying MCP servers; our `.cursor-plugin/plugin.json` declares no components |
| Antigravity | CLI: `agy plugin install <name>@<marketplace>`, a GitHub URL or a path. App: a marketplace panel. IDE: copy the folder into `.agents/plugins` or `~/.gemini/config/plugins` | Skills, subagents, rules, hooks, MCP servers (`plugin.json`). `agy plugin import gemini` converts Gemini CLI extensions |
| OpenCode | A JS/TS file in `~/.config/opencode/plugins` or `.opencode/plugins`, or an npm package listed in `opencode.json`; no marketplace | Code that handles events; skills, subagents and MCP servers go in their own folders |
| Junie | "Extensions", in the CLI: `/extensions` (also `/plugin`), from a marketplace in a git repo, a folder or at a URL. Accepts Claude Code's `.claude-plugin/marketplace.json` | Skills, subagents, hooks (`hooks/hooks.json`), MCP servers, commands, guidelines |
| Windsurf: Devin Local | `devin plugins install <owner/repo>`, at user scope. Manifest `.devin-plugin/plugin.json`, falling back to Claude Code's `.claude-plugin/plugin.json` | Skills, subagents, rules, hooks, MCP servers |
| Windsurf: Cascade | None | None |
| Warp | None documented | None |
| Cline | `cline plugin install`, in the CLI and SDK only, not the IDE extensions | TS/JS code: tools, hooks, commands, rules |
| Gemini CLI | Extensions | Skills, hooks, MCP servers; subagents not checked |

Sources:
- Claude Code: [skills](https://code.claude.com/docs/en/skills), [MCP](https://code.claude.com/docs/en/mcp).
- Copilot in VS Code: [skills](https://code.visualstudio.com/docs/copilot/customization/agent-skills), [agents](https://code.visualstudio.com/docs/copilot/customization/custom-agents), [hooks](https://code.visualstudio.com/docs/copilot/customization/hooks) and GitHub's [hooks reference](https://docs.github.com/en/copilot/reference/hooks-configuration), [MCP](https://code.visualstudio.com/docs/copilot/customization/mcp-servers), [plugins](https://code.visualstudio.com/docs/copilot/customization/agent-plugins).
- Copilot CLI: [skills](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills), [hooks](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks), [agents](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/create-custom-agents-for-cli), [MCP](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers).
- Codex: [skills](https://learn.chatgpt.com/docs/build-skills), [subagents](https://learn.chatgpt.com/codex/subagents), [hooks](https://developers.openai.com/codex/hooks), [MCP](https://learn.chatgpt.com/codex/mcp).
- Cursor: [skills](https://cursor.com/docs/context/skills), [subagents](https://cursor.com/docs/context/subagents), [hooks](https://cursor.com/docs/agent/hooks), [MCP](https://cursor.com/docs/context/mcp).
- Antigravity: [skills](https://antigravity.google/docs/skills), [subagents](https://antigravity.google/docs/subagents), [hooks](https://antigravity.google/docs/hooks), [MCP](https://antigravity.google/docs/mcp), [plugins](https://antigravity.google/docs/plugins).
- OpenCode: [skills](https://opencode.ai/docs/skills), [agents](https://opencode.ai/docs/agents), [plugins](https://opencode.ai/docs/plugins), [MCP](https://opencode.ai/docs/mcp-servers), [config](https://opencode.ai/docs/config/).
- Junie: [skills](https://junie.jetbrains.com/docs/agent-skills.html), [subagents](https://junie.jetbrains.com/docs/junie-cli-subagents.html), [hooks](https://junie.jetbrains.com/docs/junie-cli-hooks.html), [MCP](https://junie.jetbrains.com/docs/junie-cli-mcp-configuration.html), [extensions](https://junie.jetbrains.com/docs/junie-cli-extensions.html).
- Windsurf: Cascade [skills](https://docs.devin.ai/desktop/cascade/skills), [hooks](https://docs.devin.ai/desktop/cascade/hooks), [MCP](https://docs.devin.ai/desktop/cascade/mcp); Devin Local [overview](https://docs.devin.ai/desktop/devin-local), [subagents](https://docs.devin.ai/cli/subagents), [hooks](https://docs.devin.ai/cli/extensibility/hooks/overview), [MCP](https://docs.devin.ai/cli/extensibility/mcp/configuration), [plugins](https://docs.devin.ai/cli/extensibility/plugins/overview).
- Warp: [skills](https://docs.warp.dev/agent-platform/capabilities/skills/), [MCP](https://docs.warp.dev/agent-platform/capabilities/mcp/), [orchestration](https://docs.warp.dev/platform/orchestration/).
- Cline: [skills](https://docs.cline.bot/customization/skills), [config](https://docs.cline.bot/getting-started/config), [subagents](https://docs.cline.bot/features/subagents), [plugins](https://docs.cline.bot/customization/plugins), [MCP](https://docs.cline.bot/mcp/mcp-overview).
- Gemini CLI: [skills](https://geminicli.com/docs/cli/skills/), [subagents](https://geminicli.com/docs/core/subagents/), [hooks](https://geminicli.com/docs/hooks/), [MCP](https://geminicli.com/docs/tools/mcp-server/).

The Claude Code hooks paths and Copilot CLI's plugin install come from what's already known (our plugin, and the `npx plugins` README), not a fresh check.

Beyond these: `npx skills` lists about 80 agents in 65 rows. Only 13 rows use `.agents/skills` for a project; the rest have folders of their own. That table is conservative: Windsurf, Junie, Cline and Warp all read `.agents/skills` and `~/.agents/skills` by their own docs, though it lists only their own folders. Agents beyond the eleven above, such as Goose, Roo Code, Kiro, Continue, Augment, Qwen Code, Trae and OpenHands, weren't checked.

#### What this means

- **Agents support different things.** All of them load skills in the [Agent Skills](https://agentskills.io/specification) format. Everything else differs:
  - **Plugins:** installed by command (Claude Code, Codex, Copilot CLI, Antigravity CLI, Junie, Devin Local), from a panel or marketplace UI (Cursor, Copilot in VS Code, the Antigravity App), or as code dropped in a folder (OpenCode, Cline). Warp and Cascade have none.
  - **Hooks:** a different file and event names in each agent. A session-start hook that adds context, which our update notice needs, exists in eight of the eleven, counting Windsurf through Devin Local. Junie and Warp have none, nor has Cascade, and OpenCode documents none.
  - **Subagents:** Markdown in most agents, but TOML in Codex, `mode: subagent` in OpenCode and `.agent.md` in Copilot. Cascade and Warp (locally) have none. Each format needs its own copy of `aql-writer`.
- **Claude Code's plugin format is becoming shared.** Junie installs extensions from Claude Code's `.claude-plugin/marketplace.json`, Devin Local falls back to `.claude-plugin/plugin.json`, and Copilot in VS Code reads Claude-format plugins. So our existing marketplace may install whole, with skills, subagent and hooks, in Junie, Devin Local and VS Code, with no new manifest. This is untested, and each documents that not every component may work. It would change approach A, which today counts only agents `npx plugins` targets.
- **`aql-writer` reaches Claude Code only, today:**
  - Codex's plugin format doesn't document subagents.
  - Our Cursor manifest declares no components.
  - `npx skills` copies skills only, and OpenCode doesn't read Claude Code's agent folders.

  Nothing breaks without it: `write-aql` asks for "an `aql-writer` sub-agent if available, otherwise a generic one told to use the `aql`, `write-aql`, and `validate-aql` skills". Other agents get that generic path, without the subagent's preloaded AQL reference.
- **MCP servers:** every agent takes them, each in its own file. Six have a shell command to add one (`claude mcp add`, `code --add-mcp`, `copilot mcp add`, `codex mcp add`, `devin mcp add`, `cline mcp add`, plus `gemini mcp add` for Gemini CLI).
  - Several read each other's files. Claude Code's `.mcp.json` is also read by Copilot CLI and by Warp, and Warp also reads Codex's config. One entry can reach several agents.
  - anfra serves MCP only over HTTP, at `/mcp` of `anfra serve` (`cmd/anfra/serve.go`); it has no stdio command. An agent's MCP entry for anfra would point at the URL `anfra serve` prints, and work only while it runs. Our plugin ships no MCP server today.
- **Agents read each other's folders, so one install can load twice:**

  | Folder | Read by |
  |---|---|
  | `~/.agents/skills` | Codex, Cursor, OpenCode, Copilot (CLI and VS Code), Junie, Windsurf (Cascade), Warp, Cline, Gemini CLI. Not Claude Code or Antigravity |
  | `.agents/skills` (project) | All of the above, and Antigravity |
  | `~/.claude/skills` | Claude Code, Cursor, OpenCode, Copilot in VS Code, Warp; Windsurf, if reading Claude Code's config is turned on |
  | `.claude/skills` (project) | Claude Code, Cursor, OpenCode, Copilot (CLI and VS Code), Cline, Warp; Windsurf, if turned on |
  | `~/.claude/agents`, `.claude/agents` | Claude Code, Cursor, Copilot in VS Code |
  | `.claude/settings.json` (hooks) | Claude Code, Copilot in VS Code, Devin Local |
  | Every other agent's skills folder | Warp, which reads ten: `.agents`, `.warp`, `.claude`, `.codex`, `.cursor`, `.gemini`, `.copilot`, `.opencode`, `.factory`, `.github` |
  | A plugin's skills | Only the agent the plugin is installed in |

  - **The Codex plugin** plus any install into `~/.agents/skills` loads each skill twice in Codex. Its docs: "If two skills share the same `name`, Codex doesn't merge them; both can appear in skill selectors."
  - **`npx skills`' default for Claude Code,** a link in `~/.claude/skills` to the copy in `~/.agents/skills`, loads each skill twice in Cursor, OpenCode, Copilot in VS Code and Warp.
  - **Warp** loads an install twice whenever two agents' folders hold it, for example both `~/.agents/skills` and `~/.cursor/skills`.
  - **A user install plus a project install** loads twice in every agent that reads both.
  - **Antigravity** reads `.agents/skills` in a project, but not `~/.agents/skills`. A user install needs its own folders: `~/.gemini/config/skills` for the IDE and App, `~/.gemini/antigravity-cli/skills` for the CLI.
  - **Cost:** a duplicate set of our skills costs about 1,900 characters of names and descriptions, roughly 500 tokens, per session. The bigger risk is the two copies drifting apart, each updated by a different mechanism, and the agent loading the stale one.
- **Names clash across vendors.** Outside a plugin, skill names aren't namespaced. `aql`, `write-aql`, `run-aql`, `validate-aql` and `lookup-values` exist in Holistics' `holistics-development` plugin too, and a second install of `aql` into `~/.agents/skills` would overwrite the same folder. Most vendors name their skills after themselves or their products. Supabase (`supabase-*`), Vercel (`vercel-*`), Expo (`expo-*`, `eas-*`) and Stripe (`stripe-*`, plus product names such as `connect-*` and `metronome`) all do; Cloudflare only partly (`cloudflare-one`, but also `agents-sdk`, `basin`, `k2`). So the skills should be renamed `anfra-*`, in the source: the spec requires a skill's folder name to match its `name`, and the skills refer to one another about 45 times, so renaming at install time means rewriting their text.
- **No agent offers a usable way to turn one copy off.**
  - Codex can turn off a skill by path (`[[skills.config]] enabled = false`), but its docs don't say whether that works for a plugin's skills.
  - OpenCode's `permission.skill: deny` matches by name, so it hides both copies.
  - Claude Code's `skillOverrides` doesn't apply to plugin skills.

### References: how others do it

#### `npx skills`

[vercel-labs/skills](https://github.com/vercel-labs/skills), 1.7.4, tested on our repo:

- **It finds our eight skills** through `.claude-plugin/marketplace.json`; the repo needs no change.
- **One copy, linked per agent.** It installs one copy in `~/.agents/skills` (a project's `.agents/skills` without `-g`) and links agents with their own folder to it, such as `~/.claude/skills`. `--copy` copies instead.
- **It runs without prompts:** `add holistics/anfra-skills -g -a <agents…> --skill '*' -y --json`.
- **It pins a version:** `holistics/anfra-skills#anfra-v0.2.1` installs that tag, and `update` keeps it on that tag.
- **It records what it installed:** `~/.agents/.skill-lock.json` for a user install, `skills-lock.json` for a project (committable). An update checks GitHub's tree hashes, in under a second.
- **It finds installed agents** by looking for their config folders (`~/.config/opencode`, …).
- **Skills only:** no hooks, no subagents.
- **Exit codes:** without `--json`, a failed write still exits 0. With `--json` it exits 1 and gives each skill a `status`, `"failed"` with the error.
- **`check` installs updates:** in 1.7.4 it is the same command as `update`, despite its docs, and exits 0 whether or not there were updates.
- **Requirements:** Node ≥ 22.20. It sends telemetry unless `DISABLE_TELEMETRY=1` or `DO_NOT_TRACK=1` is set.

#### `npx plugins`

[vercel-labs/plugins](https://github.com/vercel-labs/plugins), 1.3.5, tested on our repo:

- **Its targets:** Claude Code, Cursor, Codex, Grok Build, Kimi Code, Copilot CLI, VS Code. Not OpenCode.
- **It finds our plugin:** 8 skills, 1 agent, hooks.
- **Claude Code:** it registers the marketplace through Claude Code, installs `anfra@anfra-skills`, and turns on auto-update by writing `known_marketplaces.json` directly.
- **Codex, with no Codex on the machine:** it wrote `~/.codex/config.toml` (enabling `anfra@plugins-cli`, while the files went under `anfra-skills`) and a plugin cache, printed "Done", and exited 0.
- **Missing:** no `--json`, no update command, no pinning in its help.

#### Skills only, without plugins or hooks

- **[Supabase](https://supabase.com/docs/guides/ai-tools/ai-skills) and [Convex](https://docs.convex.dev/ai/agent-skills)** document `npx skills add`, into the project by default.
- **[Expo](https://docs.expo.dev/skills/) and [Cloudflare](https://flaviocopes.com/cloud-platform-plugins-ai-coding-agents/)** offer a Claude Code / Codex plugin, which adds their MCP servers, and `npx skills` for the rest. The user picks one.
- **[`gh skill`](https://github.blog/changelog/2026-04-16-manage-agent-skills-with-github-cli/)** (GitHub CLI 2.90+, preview) pins with `@tag` or `--pin`, and writes where a skill came from into its `SKILL.md` frontmatter, which `gh skill update` reads. This is from GitHub's changelog and the [Arch man page](https://man.archlinux.org/man/gh-skill-install.1.en), not GitHub's own manual.
- **Nobody found writes hooks into agents' own config files, or ties the skills' version to their CLI's.** Likely reasons:
  - Editing a user's hook config to run commands on every edit looks like what malware does; plugins come with approve and uninstall.
  - Each agent's hook format differs and changes.
  - Hooks added by hand get left behind on uninstall.
  - Most vendors' extras are MCP servers, and the skills carry most of the value.

### Approaches

There are two decisions:

- **What each agent gets:** the approaches below, A to E.
- **How anfra installs it:** with the npx tools, or in Go. This is an [implementation choice](#carrying-it-out) that applies to whichever approach is chosen.

Where the skills live is separate again: it decides how versions match, so it's under [Updates](#where-the-skills-live). Each approach also decides where installed versions must be checked; see [Checking, by approach](#checking-by-approach).

One trade-off runs through all of them: **delegate, or route each agent ourselves.**
- **Delegating** keeps up with the agents for free: their own plugin CLIs, a tool someone else maintains, or the agent installing itself from instructions we can edit (step 2 of [`AGENTS.md`](#agentsmd-from-anfra-init-the-catch-all)). A new agent, or a new install route in an existing one, works without an anfra release.
- **Routing each agent ourselves** gives exact control, which is what avoids duplicates. But our routes freeze what we know today. When an agent changes, say Claude Code starts reading `~/.agents/skills` or Cursor gets a plugin command, our installer is wrong or out of date until we release.

E delegates most, then A; B and D route most.

Whatever the approach, the installer must:

- **Check after installing.** Neither tool's exit code can be trusted. anfra confirms with `claude plugin list --json`, and checks that each agent's files exist, before it says "Installed". It reports each agent's result and exits non-zero if any failed. This fixes problem 2.
- **Ask which agents to install for, and report on each.**
  - At a terminal: a multi-select of agents, pre-checked from what's detected. Detection looks for config folders (`~/.config/opencode`, `~/.cursor`…), as `npx skills` does, not only a CLI on the `PATH`, which misses IDE agents.
  - Without a terminal: `--agent`, or what was detected, and say which.
  - Then one line per chosen agent: installed, needs a manual step (with the step), or failed.
  - The exit code tells "all installed" apart from "some need a manual step" and from "something failed".
- **Install to one scope,** user or project, and say so when the other already has a copy.
- **Clean up earlier installs:** the Codex plugin, and links in `~/.claude/skills` that `npx skills` made. A folder holding our `anfra.json` is ours to remove.

#### A. The full plugin wherever possible, skills elsewhere, duplicates accepted

Today's `anfra skills install` is roughly A, without the two parts that would make it work. It installs the full plugin for Claude Code and Codex, prints steps for Cursor, and only hints at `npx skills` for everyone else, so the fallback is never run (problem 1). And it never checks the result (problem 2).

- **How it works:**
  - The full plugin for every agent whose plugins can be installed by command:
    - Claude Code, Codex and Copilot CLI, through `npx plugins` or their own CLIs.
    - Possibly Junie, Devin Local and Copilot in VS Code too, since they read Claude Code's plugin format (untested).
    - Antigravity CLI, through `agy plugin install`, which would need a plugin in its format.
  - Skills alone for the rest, OpenCode among them.
  - Accept the duplicates this causes.
- **Why OpenCode gets no plugin:** OpenCode has no plugin of the kind A installs.
  - An OpenCode "plugin" is a JS/TS module that handles events, not a package with a manifest that bundles skills, subagents and hooks.
  - It has no marketplace and no install command.
  - `npx plugins` doesn't target it.

  Giving OpenCode hooks means writing that module ourselves, which is B's route. Cline's plugins are code too, and Warp and Cascade have none, so the same goes for them.
- **Fewer duplicates:** OpenCode reads `~/.config/opencode/skills`, which no other agent does, so its skills go there rather than in the shared folder.
- **For:** hooks and the subagent wherever a plugin can carry them. `npx plugins` plus `npx skills` already do exactly this, so it needs the least code of our own.
- **Against:** duplicates in Codex and Cursor whenever the shared folder is also written. The two copies are updated by different mechanisms and can drift apart.

#### B. Shared folder for skills, plugins for the rest

- **How it works:**
  - Every agent but Claude Code gets the skills from `~/.agents/skills`, or from its own folder if it doesn't read the shared one.
  - Claude Code gets the full plugin, and nothing in `~/.claude/skills`, which Cursor and OpenCode also read.
  - Hooks go through each agent's own route:

    | Agent | Hooks route |
    |---|---|
    | Codex | A plugin with hooks only. Not tested: whether Codex accepts a plugin with no skills. |
    | OpenCode | A JS plugin file |
    | Cursor | `~/.cursor/hooks.json`, optional, since it means editing the user's file |
- **For:** no duplicates; hooks almost everywhere.
- **Against:**
  - **It would be our own practice.** No vendor found writes hooks into agents' config files (see [References](#skills-only-without-plugins-or-hooks)). A CLI that adds commands to run on every edit in a user's hook config, or drops code into an agent's plugin folder, does what malware does. Users and security tools may see it as intrusive.
    - Codex asks the user to trust such hooks before they run; Cursor's and OpenCode's docs mention no such step.
    - It would need to ask before writing, list every file it wrote, and remove them all on uninstall.
    - The hooks-only Codex plugin is the exception: it goes through Codex's own plugin system.
  - One hook script per agent to maintain.
  - Cursor's hooks need careful merging and removal.

#### C. Skills only, except Claude Code

- **How it works:**
  - Claude Code gets the full plugin; every other agent, skills only.
  - The validate hook's job moves into the skills, which already tell the agent to run `anfra validate`.
  - The update notice moves to anfra's stderr line (see [Hints](#hints)).
- **For:** the simplest route, and what most vendors do.
- **Against:** agents other than Claude Code lose automatic validation after each edit.

#### D. Shared folder for skills, plugins without skills

B without the hand-written hooks: skills in one place, plugins carrying everything except skills.

- **How it works:**
  - **Claude Code:** the full plugin, as today. It doesn't read `~/.agents/skills`, so nothing overlaps.
  - **Every other agent:** the skills from `~/.agents/skills`. Antigravity doesn't read that folder at user scope, so it gets its own folders (`~/.gemini/config/skills`, `~/.gemini/antigravity-cli/skills`).
  - **Agents that read the shared folder and install plugins by command:** a second plugin in our marketplace, say `anfra-hooks`, carrying the hooks and `aql-writer` but no skills. That's Codex, and possibly Copilot, Junie and Devin Local, which read Claude Code's format. They keep the extras with no second copy of the skills.
  - **Agents without such plugins** (OpenCode, Cline, Warp, Cascade): skills only, and nothing hand-written.
    - Validation relies on the skills telling the agent to run `anfra validate`.
    - The update notice relies on anfra's stderr line (see [Hints](#hints)).
- **For:**
  - No duplicates.
  - Hooks and the subagent wherever an agent's own plugin system can carry them.
  - Nothing written into agents' config files by hand, so none of B's intrusiveness.
  - Few places to check for updates (see [Checking, by approach](#checking-by-approach)).
- **Against:**
  - **It routes each agent itself, so it freezes today's knowledge.** Our table of which agent reads which folder, and which agent gets `anfra-hooks`, goes out of date as agents change. It doesn't pick up a new agent or a new install route until we release. Two things soften this:
    - The routing rules can be kept as data, updated with the skills rather than the binary.
    - `AGENTS.md` step 2 still lets an agent we route badly install itself from the README.
  - Two plugins to maintain: `anfra` for Claude Code, and `anfra-hooks`.
  - OpenCode, Cline, Warp and Cascade lose validation after each edit.
- **Untested assumptions,** to check before designing further:
  1. Does Codex accept a plugin with hooks and no skills? If not, Codex gets skills only, as in C.
  2. Do Junie, Devin Local and Copilot in VS Code install our Claude-format marketplace? Can they take `anfra-hooks` without also loading the skills a second time?

#### E. Native plugins where we know they work, steps for the rest

Today's mechanism, with honest reporting.

- **How it works:**
  - anfra installs only through routes we've tested: the plugin through `claude plugin` and `codex plugin`, and others once tested.
  - For every other chosen agent, it prints that agent's steps rather than installing.
  - The steps live in the skills README's per-agent sections. The fallback in [`AGENTS.md`](#agentsmd-from-anfra-init-the-catch-all) points at the same sections.
- **For:**
  - The least code, and nothing we route by hand. A new agent, or a better install route, works as soon as the README says so, with no anfra release.
  - An agent we only print steps for can still finish the install itself in its first session, from `AGENTS.md`, using whatever method is current for it.
  - Fixes problem 2 through the per-agent report (see the list above), and is the natural first step towards any other approach.
- **Against:**
  - **Problem 1 is only partly fixed.** OpenCode and the others get exact steps, not an install.
  - **The steps must avoid duplicates.** A step saying `npx skills add` loads every skill twice in Codex next to its plugin, so the README's steps have to say which folder each agent should use.
  - **anfra doesn't update what users install from the steps.** The `anfra.json` check still sees stale copies in the folders it knows, and the stderr hint asks for an update.

### Carrying it out

These choices apply to whichever approach is chosen.

#### With the npx tools

- **How it works:**
  - anfra runs `npx plugins add` for the plugins, and `npx skills add … --json` for the skills alone.
  - That covers A directly.
  - B, C and D also need steps of anfra's own, for what the tools don't do: the hooks-only plugins, OpenCode's plugin file (B), and copying skills into the folders of agents that don't read the shared one.
- **For:**
  - Little code of our own.
  - The tools keep their own table of which agent reads which folder.
  - `npx plugins` turns on Claude Code's auto-update.
- **Against:**
  - Needs Node (≥ 22.20 for `npx skills`).
  - Both tools can report success without installing anything, so anfra's check after installing is what makes it reliable.
  - `npx plugins` writes agents' undocumented internal files.
  - No pinning through `npx plugins`. The Claude Code marketplace can be pinned with `holistics/anfra-skills#tag`.
  - Telemetry, unless anfra sets `DISABLE_TELEMETRY=1`.

#### In Go, inside anfra

- **How it works:** anfra does each step itself:
  - get the skills, from the tagged archive or from the binary (see [Where the skills live](#where-the-skills-live));
  - copy the skill folders;
  - write the hook files;
  - call `claude plugin` and `codex plugin` for the plugins.
- **For:** no Node, no telemetry, none of the tools' quirks, and exact control over checks and cleanup.
- **Against:** we maintain the table of which agent reads which folder. `npx skills`' table is MIT-licensed and can be copied.

The two can be combined: the npx tools first, a Go fallback when Node is missing.

### Open questions

- [Where the skills live](#where-the-skills-live): their own repo with a tag index, or this repo (as the skills themselves, or as templates generating `anfra-skills`).
- [Where anfra gets them](#where-anfra-gets-them): downloaded for its version, built into the binary, or both; and whether skills-only releases are needed.
- Whether to install with the npx tools, in Go, or both.
- Whether to turn on Claude Code's auto-update by writing its undocumented file, as `npx plugins` does, or ask the user to turn it on.
- Whether Codex accepts a plugin with hooks and no skills (B, D).
- Whether Junie, Devin Local and Copilot in VS Code install our Claude-format marketplace, and take a plugin with no skills (D).
- How much per-agent routing is worth the risk of falling behind the agents: how much to delegate.
- Which agents read `AGENTS.md`, besides Claude Code.
- Which agents our users actually use, to choose which hooks are worth maintaining.
