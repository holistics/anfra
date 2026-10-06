# Context Map

anfra has two contexts.

- **anfra** (`CONTEXT.md`, `docs/adr/`): the Repo, its Datasets and Data Sources, running queries
  on them, and Data App serving (the Shell, Data App URLs).
- **Anfra SDK** (`sdk/CONTEXT.md`, `sdk/docs/adr/`): declaring and running a Data App's queries,
  controls and filters. It reaches anfra only through a provisioned Backend, so it never names
  anfra's API.

The Shell is where they meet: it provisions the Anfra SDK for each Data App, with a Backend that
runs queries on anfra. Query Input and Execution Options are defined in `CONTEXT.md`, as anfra
applies them; `sdk/CONTEXT.md` describes them as the SDK sends them.
