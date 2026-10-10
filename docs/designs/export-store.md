# Export stores

An export's file goes to an export store (`command.ExportStore`, `engine.ExportStore` for a platform), which a server provides on the `CommandContext` as `Exports`. The store answers what [exports.md](exports.md) promises: a link that works with a plain `GET` until it expires. This document is anfra's own store, the one `anfra serve` and the CLI use, and how a platform's differs.

## anfra's own store (`internal/exports`)

**Files.** `exports.Files` are a process's exports: a server's, or one CLI run's. Each export is one file in a folder of the system's temp directory (`os.TempDir()/anfra-exports-<uid>`, so `TMPDIR` moves it), named by a token no one can guess (128 random bits). The name a download saves it as is kept beside its link, in memory, never on disk, so a caller's file name is never a path. Making `Files` touches nothing on disk; the first export makes the folder.

**Links.** `exports.Store{Files, ServedAt}` is the store a command is given. With `ServedAt`, a link is `ServedAt/<token>/<name>`, the name percent-encoded; without it, the file's own `file://` URL. The token alone decides: the name is for a client that saves a file under a link's last part, as `curl -O` and `wget` do (a browser uses the download's header), and a link works with any name after the token, or none.

- `anfra serve` gives each request a store whose links start where that request's caller reached it: `ANFRA_SITE_URL` when set (behind a proxy), otherwise the request's scheme and `Host`, then `/exports/download`. A caller who sent a made-up `Host` misleads only itself: the link goes back to it alone. `ServerInfo.URL`, where the server listens, is not a base for links: in a container it is `http://0.0.0.0:7878`.
- It serves `GET /exports/download/{token}/{name}` as its other routes are served: the download saved under its name (`Content-Disposition`, any script, encoded), resumable (`Range`), never cached, and an unknown or expired link as the same `not_found` error, since a token is all it takes to have the file.
- The CLI without a server gets `file://` links, and writes the file out itself (`anfra query export … > file`).

**Life and cleanup.** A link lives an hour from when its file is finished. One sweep removes what no link can reach any more, by two rules:

- **This process's exports** go once their links have expired, judged by their own expiry: a jump of the system clock never removes a live one.
- **Any other file in the folder**, another process's (a server that crashed, a link the CLI printed), goes once it was last written more than a link's life ago, since its expiry is not known here.

The sweep runs when a process makes its first export, and, in `anfra serve`, every ten minutes for as long as it runs. Besides it:

- `anfra serve` removes every file it made when it stops.
- The CLI removes a file once it has written it out. A file whose link it printed (`--link`) stays for the sweep.

Until a sweep runs, an expired file is only disk: its link already refuses to open.

**An export is shorter than a link's life.** An export with a header row writes its file only at its end (its rows are spooled first), so while it runs its file looks as old as the export. Its timeout (30 minutes) under a link's life (an hour) is what keeps the age rule from removing an export still being written; a test holds the two apart. The spool itself is unnamed as soon as it is open, so nothing is left of it, even by a crash (on Windows, which cannot remove an open file, it is removed when the export returns).

**The folder is the user's own**, `anfra-exports-<uid>` (`anfra-exports` on Windows, whose temp directory is a user's own already). On a machine several users share, one user's folder, made `0700`, would otherwise keep every other user's exports out.

**Where `/tmp` is in memory** (`tmpfs`, on some Linux distributions), a large export uses memory, or fails for lack of space. Point `TMPDIR` at a disk.

## A platform's store

A platform provides its own store on each `Invocation`. The interface asks only for a file written once and a link that expires, so it is free in everything else:

- **Where files go.** Object storage, usually: several instances behind a load balancer need storage they all reach, which a temp directory is not. Per-tenant prefixes, encryption at rest, a lifecycle rule that deletes what expired, are the platform's.
- **Where links point.** A presigned object-storage URL, so the platform's instances never pass the bytes on. Or the platform's own `GET`, when a download must check the caller again, be logged, or be revoked before it expires: the interface doesn't change.
- **How long a link lives**, set by its store and answered as `expires_at`.
- **What a link reaches.** Where callers reach the platform, never an internal address: the same rule as `anfra serve`'s, and the platform knows its own public URLs.
- **Caching rendered files.** A store may answer the same link for the same export, a costly XLSX made once. Core promises nothing about it.

A platform that gives no store can't export: `query.export` fails with `exports_unavailable`, before its query runs.
