# anfra skills

The skills and plugin that let your coding agent build with [anfra](https://anfra.ai), the
open-source framework for building trusted custom BI apps with AI coding agents.

They need nothing beyond anfra itself: no other tools or platform.

## Install

Install the `anfra` plugin where your agent takes it, and the skills alone where it doesn't ([Skills vs plugins](#skills-vs-plugins)).

Find your agent below; each section is anchored by the agent's id, as [`npx skills`](https://github.com/vercel-labs/skills) names it. Not listed? See [Other agents](#other-agents).

Agents change how they install plugins and skills often. If a step below doesn't work, follow your agent's own docs for installing a plugin, or skills, from a GitHub repo.

### Claude Code
From your shell:
```bash
claude plugin marketplace add holistics/anfra-skills
claude plugin install anfra@anfra-skills
```

Or within Claude Code: `/plugin marketplace add holistics/anfra-skills`, then `/plugin` > `Marketplaces` > `anfra-skills` > `Browse plugins` > Install.

(Optional) enable auto-update via `/plugin` > `Marketplaces` > `anfra-skills` > `Enable auto-update`.

### Codex
```bash
codex plugin marketplace add holistics/anfra-skills
codex plugin add anfra@anfra-skills
```

The Codex plugin includes the skills and the AML validation hook. The `aql-writer` agent is only available in Claude Code, because the Codex plugin format doesn't document support for subagents.

### Cursor
Cursor installs plugins from its marketplace panel, and its plugin marketplaces are managed at the team/org level (Teams or Enterprise plans). An admin imports the marketplace once, then teammates install individual plugins.

1. As an admin, go to **Dashboard** → **Settings** → **Plugins**.
2. Under **Team Marketplaces**, click **Import** and paste `https://github.com/holistics/anfra-skills`.
3. Review the parsed plugins, set Team Access groups if needed, name the marketplace, and save.
4. Teammates open the marketplace panel in Cursor and install the plugins they want (required plugins install automatically).

Without a team marketplace, install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a cursor -g -y
```

### GitHub Copilot
In VS Code, which reads Claude Code's plugin format: turn on the `chat.plugins.enabled` setting (agent plugins are in preview), run **Chat: Install Plugin From Source**, and enter `https://github.com/holistics/anfra-skills`.

In Copilot CLI:
```bash
copilot plugin marketplace add holistics/anfra-skills
copilot plugin install anfra@anfra-skills
```

Or install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a github-copilot -g -y
```

### OpenCode
OpenCode's plugins are code, and can't carry skills. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a opencode -g -y
```

### Gemini CLI
No extension of ours yet. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a gemini-cli -g -y
```

### Antigravity
No plugin of ours yet. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a antigravity -g -y
```

### Junie
Junie installs extensions in Claude Code's plugin format. In a Junie CLI session:
```
/extensions marketplace add holistics/anfra-skills
/extensions install anfra
```

Or install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a junie -g -y
```

### Windsurf
Windsurf has no plugins. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a windsurf -g -y
```

### Cline
Cline's plugins are code, and can't carry skills. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a cline -g -y
```

### Warp
Warp has no plugins. Install the skills alone (see [Skills vs plugins](#skills-vs-plugins)):
```bash
npx skills add holistics/anfra-skills -a warp -g -y
```

### Other agents
1. **A plugin, if your agent takes one in Claude Code's format** (`.claude-plugin/marketplace.json`): add `https://github.com/holistics/anfra-skills` as a marketplace in it, and install `anfra`.
2. **Otherwise, the skills alone** (see [Skills vs plugins](#skills-vs-plugins)), with [`npx skills`](https://github.com/vercel-labs/skills), naming your agent by its id there:
   ```bash
   npx skills add holistics/anfra-skills -a <id> -g -y
   ```

### Skills vs plugins
The skills give your agent the knowledge to work with anfra. The plugin adds the automation that keeps that work checked and up to date. Always install the plugin if your agent supports it. Install the skills alone only as a fallback, for agents that can't take our plugin.

### Updating and removing
| Installed with | Update | Remove |
|---|---|---|
| Claude Code's plugin | `claude plugin update anfra@anfra-skills`, or auto-update | `claude plugin uninstall anfra@anfra-skills` |
| Codex's plugin | `codex plugin marketplace upgrade anfra-skills` | In Codex's `/plugins` |
| Copilot CLI's plugin | `copilot plugin update anfra` | `copilot plugin uninstall anfra` |
| Junie's extension | `/extensions` > Installed > Update | `/extensions` > Installed > Remove |
| Cursor's plugin | Cursor's marketplace panel | Cursor's marketplace panel |
| `npx skills` | `npx skills update` | `npx skills remove` |

Each skill ends with its version (`anfra skills <version>`), so you and anfra can tell which version you have.

## Plugins

### `anfra`
Develop analytics with Anfra.

| Skill | What it does |
|---|---|
| `aql` | Core AQL knowledge base (lessons, per-function reference docs, worked examples). |
| `write-aql` | Author and validate an AQL query to answer a data question. |
| `validate-aql` | Type-check an AQL query or expression without running it. |
| `run-aql` | Run a validated AQL query and return rows (or compile it to SQL). |
| `lookup-values` | Verify a field's exact stored values before filtering. |
| `aml` | Write and edit AML models, fields, relationships, and datasets. |
| `setup-repo` | Set up a new Anfra repo: `anfra init`, connect a data source, a first model and dataset. |
| `build-data-app` | Build a Data App (one HTML file in `apps/`) on the repo's datasets, and check it in `anfra serve`. |

| Agent | What it does |
|---|---|
| `aql-writer` | Optionally writes and validates AQL in a subagent — e.g. several independent queries in parallel. By default the agent writes AQL itself. |

| Hook | What it does |
|---|---|
| Update notice | At session start in an Anfra repo, tells the agent when a newer `anfra` release is out, so it can mention it to you (it won't update anfra unless asked). Reads anfra's own cached daily check; set `ANFRA_NO_UPDATE_NOTIFIER=1` to turn it off. |
| Validate AML | After each `Write`/`Edit` (or Codex `apply_patch`) of an AML file, runs `anfra validate` on it and reports diagnostics. |

More plugins (e.g. for consumers/explorers) may be added to this marketplace over time.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for working on these skills and releasing them.
