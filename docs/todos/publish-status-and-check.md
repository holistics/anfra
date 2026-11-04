# `publish status`, and the rest of `publish check`

*Not built. Noted 2026-11-02.*

Two ways to ask about publishing without publishing, and the convention that tells them apart. `publish check` exists, doing the offline part of the preflight so far: the files, and where they'd go. It was `publish --list`, and before it `--dry-run`.

## The convention: `status` and `check`

- **`status` is the state of a thing.** Read-only, cheap, safe to run any time: "what is the situation?" `anfra status` (the repo and its server), `anfra auth status` (logins).
- **`<action> check` is the action's preflight.** Everything the action would do up to its first change, then what it would do: "what would happen if I ran this?" It exits non-zero when the action would fail. `anfra update check`, `anfra agent setup check`, `anfra publish check`. A part of the preflight (the files alone, offline) is the check's for now, not a command of its own, until use asks for one.

Other tools draw the same lines: `cargo package --list` and `cargo publish --dry-run`, `terraform validate` and `terraform plan`, Ansible's `--syntax-check` and `--check`, kubectl's `--dry-run=client` and `--dry-run=server`. Where a tool says "dry run", it mostly means the online level; anfra says `check`, and so has no `--dry-run`.

The first two are written down in [commands-and-api.md](../designs/commands-and-api.md#asking-without-acting).

## `anfra publish status`

Where this repo publishes, what is published there, and whether it is this folder's files as they are now.

```yaml
destination: https://cloud.example.com/o/acme
published:
  revision: acme@8f3a…
  by: alex@example.com
  at: 2026-11-01T14:02:00Z
  current: false        # this folder's files differ
```

- One call, `repos.status`, and hashing the files the publish would take: no validation, no sidecars. An agent can ask it after every change.
- Exits 0 whatever the state: it describes, it doesn't judge.
- `auth status` also names the destination, as where a login is used. Both keep it, so each answer is whole.

**`current` needs anfra-cloud.** A revision is `<repo>@<commit sha>` (`Repo.Revision` in anfra-cloud's `internal/bi/repos/repos.go`), and a commit covers its message, time and parent, so the CLI cannot compute it from the files. A git tree id it can: `repoformat.BlobID` is git's blob id, and a tree's id follows from its paths and blobs. So `repos.status` answers the published revision's tree id too, an additive field, and `current` compares it with the folder's.

Until then, the CLI can record the tree id it published beside the revision it already records (`internal/publish/state.go`): `current` is known when the published revision is this folder's last, and `unknown` when someone else published since.

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
