# Philosophy

What anfra core is, who it serves, where it stops, and the guidelines that follow. It describes the *kinds* of capability core offers; what each command does is in the code and `anfra --help`.

This is a working draft. Where a question is not settled, it says so in place (`> Open:`).

## What core is

anfra core is a semantic runtime: it reads a repo's semantic layer (e.g. AML models, datasets and metrics), answers questions against it (e.g. AQL, compiled to the warehouse's SQL and run there), and runs the interactive pages built on it (Data Apps). It does this on one machine, against one repo, for whoever runs it.

Core is built to be built on. It exposes the same capabilities through several doors, each for a different kind of builder, and leaves the platform around them to whoever builds one:

- **The CLI** (`anfra <command>`), for a person or an agent at a terminal.
- **The HTTP API and MCP** (`anfra serve`), for local tools, agents and Data Apps.
- **The Anfra SDK** (`web/sdk`), for the pages that run and host Data Apps.
- **The engine** (`engine/`), for a platform that embeds anfra in its own server ([engine.md](engine.md)).

Each command is defined once, in one registry ([commands-and-api.md](commands-and-api.md)), so a capability means the same thing whichever door it is reached through. Which commands a door offers differs, by design:

- The CLI offers every command, and also commands that act on this machine (`serve`, `init`, `update`, `skills`), which are never served: a server must not do that for its callers.
- The HTTP API and MCP serve every command.
- The SDK's API client can call every served command; a Data App itself reaches only queries, through its host ([data-apps.md](data-apps.md)).
- The engine can run every command; the platform embedding it decides which to expose, and to whom.

## Who it is for

- **Local app authors:** people, and their coding agents, who model data and build Data Apps in a repo on their own machine, and run them with `anfra serve`.
- **Local platform builders:** people who build their own tools on a running anfra (a notebook, an editor plugin, an agent workflow) through the HTTP API, MCP and the SDK.
- **Platforms embedding the engine:** a BI platform, internal portal or hosted product that runs anfra in-process and adds what core leaves out: users, permissions, sharing. Anfra Cloud is one such platform; core does not assume it is the only one.
- **A preview of Anfra Cloud:** a repo that works under `anfra serve` works unchanged when a hosted platform serves it, so local work is not throwaway.

How far that goes is settled for the API: a platform's API contains the core API unchanged (the same op names, paths, inputs, answers and error codes) and only adds around it: its own ops, and who may call what. So the same query gets the same answer anywhere, except that a platform's data restrictions may narrow it. What a platform does around the API (sign-in, which ops a caller may call, how a repo reaches it, the page Data Apps open in) is the platform's own.

> Open: what carries over beyond the API. A repo's `.anfra/data_sources.yml`, for one, holds local credentials, which a hosted platform would manage its own way.

## Where core stops

Core owns what must mean the same everywhere: the semantics of the repo, what a query returns, how a Data App runs. A platform, any server that embeds the engine and serves its commands, owns what differs between deployments: who the caller is and what they may see. `anfra serve` is itself a platform, one that makes the simplest choices: one local user, one repo, no restrictions.

Most concerns have a part on each side:

| Concern | Core | A platform | `anfra serve`'s choice |
|---|---|---|---|
| The semantic layer | Compiles, validates and shows a repo's models (AML by default). | Where repos come from and live, and which one a caller works in. | The current directory. |
| Queries | Compiles semantic queries (AQL by default) and SQL, runs them on the warehouse, answers the rows. | Who may run them, and which rows they may see, passed in as data restrictions. | Anyone, every row. |
| Data sources | Reads them from the repo's `.anfra/data_sources.yml`, and passes credentials only to the query engine. | Where those credentials come from, and who may manage them. | The file as the user wrote it. |
| Callers | None: the engine knows no user, role or tenant. | Users, sign-in, sessions, tenancy. | The machine's user. |
| Data Apps | The runtime: the SDK, the sandboxed frame, the bridge, provisioning. | The page around them, who may open them, how they are shared. | The appserve pages. |
| The API | The ops, their schemas and error codes: the core API. | The transport, admission, rate limits, and its own ops around the core API. | HTTP and MCP on loopback, no sign-in. |
| History | Nothing: the repo is the record. | Hosting, audit, version history. | None. |

The engine makes no access decisions (`engine/engine.go`). A platform decides whether a caller may run a command, then passes the *consequence* in, as the data restrictions that apply (`DataPerms`). `Dispatch` refuses a call whose restrictions nobody decided, so a platform cannot forget to decide (`internal/app/app.go`).

## Guidelines

### Convenient for most, locked for none

anfra runs in setups as different as its users: an author's laptop, an agent driving the CLI, a container serving others, a platform running many repos per process. Keep all of them in mind.  
Choose each default so the most common setup needs no configuration, and make sure every setup a default doesn't fit can change it, with a setting named for what it does. A default that suits one setup must not be the only way.

- `anfra serve` serves everything at once (the API, MCP, the Data Apps and their live reload), so an author working on a repo gets the whole experience from one command. Someone hosting Data Apps for others, or serving only agents, turns off what they don't want (`--no-mcp`, `--no-apps`, `--no-watch`).
- anfra keeps its state in `~/.anfra` by default, which suits an individual on their own machine. Containers and other deployments, with their own layouts and volumes, move it with `ANFRA_HOME`.
- anfra runs its sidecars from its own binary, so one download is everything a user needs. A platform that runs them as services, scaled and monitored its own way, points anfra at them instead (`ANFRA_NODE_URL`, `ANFRA_CANAL_QUERY_URL`).
- anfra listens on a loopback address, so a server on a laptop is reachable only from it, and moves to a free port when another repo's server is already running. Someone serving others chooses an address (`--addr`).
- anfra-node can keep compiled programs in memory, which makes a server answer much faster. But this is configurable and disabled by default, to prevent high costs in multi-repo, multi-tenant setups.

A convenient default should cost nothing to those who don't use it: live reload is on, but watches the repo only while a Data App is open (`internal/appserve/watch.go`).

### Don't bloat the user's machine or repo

- Everything anfra keeps goes in one folder, `~/.anfra` or `ANFRA_HOME` (`internal/home/home.go`): one place to find, mount or delete.
- A repo holds only what its author writes. anfra's state about a repo (logs, caches, the catalog, the running server) lives under that folder, never in the repo.
- A repo's own config folder is always `.anfra/`. It is part of the repo's format, like `.git`, so it is the same for everyone who clones it, and no setting renames it.
- Commands generate nothing into the repo unless that is their purpose (`anfra init`), and even then never overwrite (`cmd/anfra/init.go`).

### Make interfaces work for agents and programs, not only people

Most callers of anfra are programs and coding agents. An interface they can use without guessing is one a person can use too.

- **Typed inputs.** Every command's input is a schema (`internal/command/command.go`). The CLI's flags, the HTTP op and the MCP tool all come from it.
- **Outputs a program can keep using.** An answer is a document, not prose to parse: JSON over HTTP and MCP, and the same document as YAML at the CLI (`search` alone prints a compact list), alone on stdout: logs go to the repo's log file, failures to stderr. So it can be piped into a script or a YAML or JSON tool, stored, compared with an earlier run, or handed to the next command or agent step, without scraping.
- **Failures a program can act on.** A failure is as structured as an answer: a stable code to branch on, which side must act (the caller, or the server), a message to show, and typed details. A refused input names each argument at fault, as a violation on that field, so a caller can fix exactly that and retry ([errors.md](errors.md)). And what isn't a failure isn't reported as one: a command that judges something, such as whether a query is valid, answers its verdict as data (`Valid` in `internal/command/command.go`), so a caller can tell "the query is invalid, and here is why" from "the check could not run".
- **Discovery.** A caller can learn what it may call without reading the source: `/api/ops`, the OpenAPI document (`/api/openapi.json`, `anfra openapi`), and each command's `--help`.
- **Short feedback loops.** Check before running: `anfra validate` for the repo, `query validate` and `query compile` for a query, `anfra show` for what a dataset offers. An agent can iterate in seconds without touching the warehouse.

### Define once, serve everywhere

A capability is defined once, as one command definition, and every door reads it: the CLI's command and flags, the HTTP op `core.<name>`, the MCP tool and the published schema (`internal/app/commands.go`). Adding a command adds it to every surface anfra serves itself; nothing is registered per surface, so the surfaces cannot drift. A platform embedding the engine still decides whether to expose a new command ([engine.md](engine.md)).

### Safe by default

A default must be safe for the person who never reads the docs.

- `anfra serve` listens on loopback and has no authentication; it warns when told to listen elsewhere, and refuses requests a web page could forge ([security.md](security.md)).
- A Data App definition, which any author or agent may write, runs in a sandboxed frame and reaches data only through the SDK's bridge, so a definition that forgets about security is still safe ([data-apps.md](data-apps.md)).
- Credentials stay in the repo's git-ignored `.anfra/data_sources.yml` and are passed to the query engine only, never to anfra-node.

### Interfaces, not implementations

What a caller sees is what anfra promises; how it is produced may change. `core.show` answers an object's interface (names, types, roles, definitions), never its implementation (SQL, tables, data sources), so the semantic catalog can replace compiled AML behind it. The same applies to the sidecars: platforms and callers see anfra's ops, never anfra-node's RPCs.

### Fail where the problem is

Refuse early, in the process that can name the cause, with the input that caused it. A command's `Check` refuses an input the schema allows but the command cannot run, before any sidecar starts (`internal/command/command.go`). A missing data source fails with the name of the file to fix (`internal/datasource/datasource.go`). A telemetry problem never stops a command (`cmd/anfra/telemetry.go`).
