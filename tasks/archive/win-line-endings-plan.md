# Implementation Plan: win-line-endings

Spec: [SPEC-win-line-endings.md](../../SPEC-win-line-endings.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Task checklist: [todo.md](win-line-endings-todo.md). Earlier modules' tasks: [archive/](./).

## Overview

What this module delivers and why: the spec's [Objective](../../SPEC-win-line-endings.md#objective). This file holds how: the order, the checkpoints, task-level decisions and risks. Publish first (reading files "as published"), then pull (merging and writing back with each file's own line ending), then the two smaller writers, then the round trip on Windows with `acme`.

## Architecture Decisions

- **One function decides "as published",** in `internal/publish/workdir`, beside `Read`: CRLF → LF for a text file, a binary one (a zero byte in its first 8 KB) as it is, a lone CR kept. Every read that compares or sends goes through it; `backup` doesn't (it keeps exact bytes for restore).
- **The write-back rule lives in `Apply`,** which already reads each file before writing it (`backup`): a file whose line breaks are all CRLF gets its new content with CRLF. New, LF, mixed or binary files are written as given.
- **The two appenders share no code with publish:** `repoconfig` and `agents` each match the file's line ending where they add lines. They're small and in different packages; a shared helper would only be baggage ([naming](../../docs/dev/naming.md)).
- **No per-OS code:** line endings are bytes; nearly every test runs on Linux.
- **Working lightly** ([memory](./win-files-auth-todo.md#checkpoint-b)): `GOMAXPROCS=4 GOFLAGS=-p=2`, one Windows build and one lint per task, Windows' free memory checked first; `make check` / `make test` left to CI.

## Dependency graph

```
T1 publish reads "as published" (Read's hashes + the upload)
 └─ T2 pull: merge side, conflict check, hints, settings; Apply writes each file's own line ending
T3 init writes .gitattributes                                   (independent)
T4 config.yml and AGENTS.md keep their line ending              (independent)
                                                                → Checkpoint A
T5 round trip on Windows with acme (needs T1–T2)
 └─ T6 docs and plan                                            → Checkpoint B
```

## Task List

### Phase 1: publish and pull
- [x] T1: publish reads files "as published": a CRLF file hashes and uploads as LF
- [x] T2: pull merges CRLF files as LF, and writes each file back with its own line ending

### Phase 2: The files anfra writes
- [x] T3: `anfra init` writes `.gitattributes`
- [x] T4: Lines added to `config.yml` and `AGENTS.md` keep the file's line ending

### Checkpoint A
- [x] T1–T4's tests pass on Linux; review with you at the end of the run.

### Phase 3: Windows and close
- [x] T5: Round trip on Windows 11 with `acme`
- [x] T6: Docs and plan

### Checkpoint B: Complete
- [x] Spec success criteria 1–7 met; the commit split reviewed with you.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| A text file is taken for binary, or a binary for text | High: a binary changed byte for byte, or CRLF text sent as is | git's rule, which every git user already lives with; a test per case (CR in a binary, a NUL past 8 KB) |
| A repo already published with CRLF content: the org's blobs hold CRLF | Medium: the first publish after this sends those files once, as LF | Expected and right (the org becomes LF); said in the docs |
| `ReadBlob`'s "changed while reading" check compares the raw file to the LF hash | High: every CRLF file reported as changed | It compares the normalised content; a test with a CRLF file |
| `withBlock` misses its own block in a CRLF `AGENTS.md`, adding a second | Medium | Found while planning; T4 tests it |
| The Windows round trip publishes a CRLF-converted file by mistake | Medium: `acme` changes beyond the test | Before publishing, `publish check` lists what's sent; only the one model expected |

## Open Questions

None.
