# Tasks: win-line-endings

Plan: [plan.md](win-line-endings-plan.md). Spec: [SPEC-win-line-endings.md](../../SPEC-win-line-endings.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md).

## Phase 1: publish and pull

### - [x] T1: publish reads files "as published": a CRLF file hashes and uploads as LF

**Acceptance criteria:**
- [x] `workdir`: one function giving a file's content as published (CRLF → LF in text; binary, by git's rule, as is; a lone CR kept).
- [x] `Read` hashes it, and `publish.go:345` uploads it (its "changed during the publish" check compares it).
- [x] The hash format and `shared/repoformat` unchanged.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/publish/...`, with: the function's cases (CRLF, LF, lone CR, binary with CR, NUL past 8,000 bytes: git's limit, not 8 KB); a CRLF repo hashes as the LF one; publish sends nothing for line endings alone, and a really edited CRLF file as LF.
- [x] Lint once, Linux and Windows.

**Dependencies:** None

**Files likely touched:** `internal/publish/workdir/tree.go` (or a new file beside it), `internal/publish/publish.go` (+ tests)

**Estimated scope:** M

### - [x] T2: pull merges CRLF files as LF, and writes each file back with its own line ending

**Acceptance criteria:**
- [x] `ReadBlob` (the merge's local side), `pull.go:208` (conflict markers gone?), `pullhints.go:35` (data source names), `settings.go:174` (`init`'s config?) read as published; `ReadBlob`'s "changed while reading" check compares the normalised content. Done for `ReadBlob` (in T1). The other three need nothing: `textmerge.Resolved` already takes `\r\n` after a marker; the hint's pattern reads only between quotes; `adoptOrgConfig` parses YAML, which reads CRLF, and a CRLF copy only makes it write the org's identical file. Left as they are (YAGNI).
- [x] `Apply`: a file whose line breaks are all CRLF is written with CRLF (conflict markers included); new, LF, mixed and binary files as given. `backup` keeps exact bytes.
- [x] `pull --abort` and `--take local` still put back the exact original bytes.

**Verification:**
- [x] Tests pass (Linux): pull over unedited CRLF files updates them, no "edited here" refusal, and they stay CRLF; a change on both sides of one CRLF file merges cleanly, written CRLF; a conflict in a CRLF file has CRLF markers, and `resolved` sees it once fixed; a new file is LF; a binary file is written as is; `--abort` restores a CRLF file byte for byte.
- [x] Lint once, Linux and Windows.

**Dependencies:** T1

**Files likely touched:** `internal/publish/workdir/tree.go`, `apply.go`, `internal/publish/pull.go`, `pullhints.go`, `settings.go` (+ tests)

**Estimated scope:** M

## Phase 2: The files anfra writes

### - [x] T3: `anfra init` writes `.gitattributes`

**Acceptance criteria:**
- [x] `models/** datasets/** apps/** .anfra/** text=auto eol=lf`, each with a short comment saying why, as `.gitignore`'s entries have.
- [x] An existing `.gitattributes` gets only the lines it lacks; a non-regular one is refused, as `.gitignore`. A line already setting a path (whatever its attributes) is left to the repo. `ignoreLines`' body is now `gitLines`, shared by both, with `ignoreLines` unchanged in what it does.
- [x] `init`'s help and its summary list the file.

**Verification:**
- [x] Tests pass (Linux): `go test ./cmd/anfra/ -run Init` with a new and an existing `.gitattributes`.

**Dependencies:** None

**Files likely touched:** `cmd/anfra/init.go`, `cmd/anfra/init_test.go`

**Estimated scope:** S

### - [x] T4: Lines added to `config.yml` and `AGENTS.md` keep the file's line ending

**Acceptance criteria:**
- [x] `repoconfig.appendBlock`: the block in the file's line ending (CRLF when its line breaks are CRLF).
- [x] `agents.withBlock`: finds its existing block in a CRLF `AGENTS.md` (no second block), and writes the block in the file's line ending. The plan's guess was off: the markers are found in CRLF too; the real problem was `CheckBlock` comparing exactly, so a CRLF block always read as outdated and was rewritten each time. Now compared as LF.

**Verification:**
- [x] Tests pass (Linux): `go test ./internal/repoconfig/ ./internal/agents/`, each with a CRLF and an LF file.

**Dependencies:** None

**Files likely touched:** `internal/repoconfig/repoconfig.go`, `internal/agents/agentsmd.go` (+ tests)

**Estimated scope:** S

### Checkpoint A
- [x] T1–T4's tests pass on Linux; lint clean on Linux and Windows. Review with you at the end of the run (nothing published before T5's round trip, which you approved in the spec).

## Phase 3: Windows and close

### - [x] T5: Round trip on Windows 11 with `acme`

**Acceptance criteria:**
- [x] In `C:\Users\nuita\wincheck\pulled`, after pulling `acme`'s latest: save `models/sales/products.model.aml` with CRLF; `publish status` shows nothing to publish.
- [x] Change one line, `publish check` lists only that file, publish it; pulled into a fresh folder, it is LF.
- [x] Put the line back, publish again: `acme` holds the files it held before.
- [x] A pull into the CRLF folder keeps the file CRLF.

**Verification:**
- [x] Manual, on Windows 11 (the user approved publishing to `acme`), results recorded here. Done 2026-11-27, in this order:
  1. `pulled`, with `products.model.aml` saved CRLF: `pull` up to date; `publish status`: "Everything here is published".
  2. `pulled2` (new, `init` + `pull`): the file arrived LF, SHA-1 `5ef68d27…` (the original).
  3. `pulled2`: one comment line added; `publish status` listed only `products.model.aml`; published (`…@9fb694ed`).
  4. `pulled` (CRLF): `pull` updated the file, no refusal; all 12 lines CRLF, the new one included.
  5. `pulled`: the line removed (file still CRLF); `publish status` listed only that file; published (`…@7ed5738d`). Pulled into `pulled2`: the original bytes again, LF, SHA-1 `5ef68d27…`.

**Dependencies:** T1, T2

**Files likely touched:** none (results here)

**Estimated scope:** S

### - [x] T6: Docs and plan

**Acceptance criteria:**
- [x] `docs/usage/publishing.md` and `docs/designs/publish.md`: what publish and pull do with line endings, and the first publish of a repo already published with CRLF.
- [x] `WINDOWS-PLAN.md`: Where it stands, and the module's result (the six places were five; `backup` keeps exact bytes; `withBlock`'s second block).

**Verification:**
- [x] Manual: the spec's Success Criteria 1–7, each ticked here.
  - [x] 1. CRLF and LF repos hash the same; nothing sent for line endings alone; an edited file sent as LF (T1, T5).
  - [x] 2. pull updates unedited CRLF files, merges both sides cleanly, keeps CRLF; new files LF (T2, T5).
  - [x] 3. Binary files byte for byte (T1, T2).
  - [x] 4. `.gitattributes` from `init`, missing lines added to an existing one (T3).
  - [x] 5. `config.yml` and `AGENTS.md` keep their line ending (T4).
  - [x] 6. The round trip on Windows with `acme` (T5).
  - [x] 7. Tests of every touched package pass on Linux; the Windows check recorded; docs and `WINDOWS-PLAN.md` updated. `make check` / `make test` left to CI.

**Dependencies:** T5

**Files likely touched:** the docs above, `WINDOWS-PLAN.md`

**Estimated scope:** S

### Checkpoint B: Complete
- [x] Spec success criteria 1–7 met.
- [x] Ready for review with you (and the commit split proposed). Reviewed; the split kept as is.
