# `publish status`, and the rest of `publish check`

*`publish status` built 2026-11-11. The rest of `publish check` not built. Noted 2026-11-02.*

Two ways to ask about publishing without publishing, and the convention that tells them apart. `publish check` exists, doing the offline part of the preflight so far: the files, and where they'd go. It was `publish --list`, and before it `--dry-run`.

## The convention: `status` and `check`

- **`status` is the state of a thing.** Read-only, cheap, safe to run any time: "what is the situation?" `anfra status` (the repo and its server), `anfra auth status` (logins).
- **`<action> check` is the action's preflight.** Everything the action would do up to its first change, then what it would do: "what would happen if I ran this?" It exits non-zero when the action would fail. `anfra update check`, `anfra agent setup check`, `anfra publish check`. A part of the preflight (the files alone, offline) is the check's for now, not a command of its own, until use asks for one.

Other tools draw the same lines: `cargo package --list` and `cargo publish --dry-run`, `terraform validate` and `terraform plan`, Ansible's `--syntax-check` and `--check`, kubectl's `--dry-run=client` and `--dry-run=server`. Where a tool says "dry run", it mostly means the online level; anfra says `check`, and so has no `--dry-run`.

The first two are written down in [commands-and-api.md](../designs/commands-and-api.md#asking-without-acting).

## `anfra publish status`

Built, with pull: [publish](../designs/publish.md#seeing-the-conflicts-again). It compares the folder with the base and the published revision through `repos.list_files`, so it needs nothing more from the server, and it lists the last pull's unresolved conflicts. `anfra status` hints at it, a hint the CLI adds, since the core command has no CLI commands to point to over HTTP or MCP.

## The rest of `anfra publish check`

The whole preflight: every step of `publish.Run` up to the upload, each step's answer, and whether a publish would succeed. Today `publish check` does the first two steps, offline.

```
$ anfra publish check
! Not published: 3 files differ from acme@8f3a…, published by alex@example.com on 2026-11-01 14:02.
✓ 42 files of the repo
✓ Publishes to https://cloud.example.com/o/acme (signed in)
✓ Valid
! The org lacks 1 data source this repo uses: warehouse (publish offers to add it)
✓ Within the org's limits: 42 of 500 files, largest 120 KB of 5 MB
· 3 files to upload
Would publish. Run `anfra publish` to publish.
```

| Step | From |
|---|---|
| Already published? (the headline) | `publish status`'s `current` |
| Files, and those left out | the local selection |
| Destination, signed in | `.anfra/config.yml`, the stored login |
| Valid | `validate`, forwarded to a warm server |
| Data sources the org lacks | `data_sources.list` |
| Within limits | `repos.status` |
| Published by someone else since this folder's last publish | `repos.status`, against local state |
| Files to upload | `repos.list_missing_blobs` |

Non-zero exit when a publish would fail: not signed in, no org, invalid, over a limit. A warning alone (`!`) is not a failure. It asks nothing and writes nothing: no org selection, no data source added, no config written.

Which files would change or be deleted, it cannot say: the server answers a revision, not its files. Files to upload is the nearest it gets, unless anfra-cloud adds an op listing a revision's files.
