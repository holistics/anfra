# Anfra skills

A marketplace of agent skills for working with [Anfra](https://anfra.ai) —
building and exploring analytics via **AML** (modeling) and **AQL** (querying)
against a local AMQL repo, using the `anfra` CLI (`anfra query`, `anfra validate`).

The skills are self-contained: they assume only the `anfra` command, not any
other tools or platform.

## Install (Claude Code)

```
/plugin marketplace add <this-repo-url>
/plugin install anfra-development@anfra-skills
```

## Plugins

### `anfra-development`
Develop analytics with Anfra. Skills:

| Skill | What it does |
|---|---|
| `aql` | Core AQL knowledge base (lessons, per-function reference docs, worked examples). |
| `write-aql` | Author, validate, and run an AQL query to answer a data question. |
| `lookup-values` | Verify a field's exact stored values before filtering. |
| `validate-aql` | Type-check an AQL query or expression. |
| `aml` | Write and edit AML models, fields, relationships, and datasets. |

More plugins (e.g. for consumers/explorers) may be added to this marketplace over time.