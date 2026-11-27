# Spec: win-line-endings

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-line-endings`). Depends on: `win-process-lifecycle` (done); builds on `win-files-auth` (done), whose publish and pull checks on Windows it repeats. Repo: **anfra**. Milestone **M3**: everything works on Windows.

## Objective

Windows editors save text files with CRLF line endings (`\r\n`); Mac and Linux use LF (`\n`). publish and pull compare and send files **byte for byte**, so a file saved on Windows looks wholly different from the same file saved on a Mac. Today that means, for a team with a member on Windows:

- **publish** sends every file a Windows editor touched, as changed, even when only its line endings differ, and the org's copy turns CRLF; the next Mac user's pull then shows every line of it changed;
- **pull** refuses to update a CRLF file it would otherwise update quietly, since its bytes no longer match what was pulled ("edited here"), and a real change on both sides of one CRLF file becomes a single whole-file conflict instead of a clean merge.

Compiling is fine already: the AML parser treats CR as whitespace and rebuilds multi-line values with LF (`aml` repo: `parser/lexer.ts`, `modules/expression/string/utils.ts`), so `validate` and `query` give the same results for CRLF and LF files. Only publish and pull, which compare raw bytes, need this module.

**How the industry handles it, and what anfra does** (decided with the user, 2026-11-27): like git, the shared copy is LF, and each working copy may differ.

1. **publish and pull read every file "as published":** text files with CRLF turned into LF; binary files untouched. Git's rules: a file with a zero byte in its first 8 KB is binary; a lone CR is kept. One function in `internal/publish/workdir`, used at the places publish and pull read the working folder to compare or send it (checked 2026-11-27):
   - `workdir/tree.go:33` (`Read`: the hashes, which also decide pull's "was it edited here?") and `:81` (`ReadBlob`, the merge's local side);
   - `publish.go:345` (the upload, which checks each file still has the hash `Read` gave it).

   Found while building: `pull.go:208` (whether a conflict's markers are gone), `pullhints.go:35` (the data sources AML names) and `settings.go:174` (whether the folder's config is `init`'s) need no change: each already reads CRLF right.

   Not `workdir/apply.go:102` (`backup`), which the plan listed: it keeps a file's exact bytes, for `pull --abort` and `--take local` to put back.

   Uploads become LF. The hash format and the server stay as they are (`shared/repoformat` untouched). The three-way merge (`shared/treemerge`, `shared/textmerge`) gets the local side through `ReadBlob`, so it compares LF with LF.
2. **pull keeps each file's own line ending** (the user's choice, as VS Code does): when pull writes over a file whose line breaks are all CRLF, it writes the new content with CRLF too, conflict markers included. A new file, or one with LF or mixed line breaks, is written as the org has it (LF). Binary files are written as they are.
3. **`anfra init` writes `.gitattributes`** for anfra's folders, `models/** datasets/** apps/** .anfra/** text=auto eol=lf` (the user's choice; GitHub's advice plus LF): git stores and checks out text there with LF, and leaves binaries (images, fonts in `apps/`) as they are. An existing `.gitattributes` gets the lines it lacks, as `.gitignore` does today (`ignoreLines`, `cmd/anfra/init.go:533`).
4. **Lines anfra adds to a file keep that file's line ending:** `internal/repoconfig/repoconfig.go:203` `appendBlock` (adding a block to `.anfra/config.yml`) and `internal/agents/agentsmd.go:210` (`withBlock`, the block in `AGENTS.md`, which also finds its existing block by exact text, so a CRLF `AGENTS.md` must not get a second one).

**Users:** every team with a member on Windows who publishes or pulls; and a Windows user alone, whose own publishes would otherwise turn the org's files CRLF.

**Out of scope:** changing what the server stores or how it hashes; a repo's files outside what publish sends; converting files on disk that anfra doesn't write (the user's editor keeps its own); CI.

## Tech Stack

Go 1.27, standard library only (`bytes`). Windows 11 x64 for the end-to-end check, with the `acme` org and the folders from `win-files-auth` (`C:\Users\nuita\wincheck\pulled`).

## Commands

```sh
# Lightly (GOMAXPROCS=4 GOFLAGS=-p=2), one command at a time
GOMAXPROCS=4 GOFLAGS=-p=2 go test ./internal/publish/... ./internal/repoconfig/ ./internal/agents/ ./cmd/anfra/ ./shared/textmerge/ ./shared/treemerge/
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go vet ./internal/publish/... ./cmd/anfra/
GOMAXPROCS=4 tmp/bin/golangci-lint run ./internal/publish/... ./internal/repoconfig/ ./internal/agents/ ./cmd/anfra/   # once per task, per OS

# Windows: one test build per task, run on the host
GOMAXPROCS=4 GOFLAGS=-p=2 GOOS=windows go test -c -o /mnt/c/Users/nuita/wincheck/publish.test.exe ./internal/publish/
```

`make check` / `make test`: left to CI unless the user asks.

## Project Structure

| Where | Change |
|---|---|
| `internal/publish/workdir/` | The "as published" reader (CRLF → LF for text), and pull writing back with a file's own line ending, beside `tree.go` and `apply.go` |
| `internal/publish/*.go` | The four reads outside `workdir` use it |
| `cmd/anfra/init.go` | `.gitattributes`, as `.gitignore` |
| `internal/repoconfig/repoconfig.go`, `internal/agents/agentsmd.go` | Added lines in the file's own line ending |
| Tests beside each | Mostly OS-independent: line endings are bytes, testable on Linux |
| `docs/usage/publishing.md`, `docs/designs/publish.md` | What publish and pull do with line endings |

No per-OS code: CRLF files reach Linux and macOS too (a repo copied or checked out from Windows), and the rules are the same everywhere.

## Code Style

anfra's conventions: [naming](docs/dev/naming.md), [readable prose](docs/designs/readable-prose.md), comments only for why. Shape:

```go
// AsPublished is content as publish sends it: a text file's CRLF line breaks
// as LF, so a file saved on Windows matches the same file saved on a Mac.
// A binary file (a zero byte in its first 8 KB, git's rule) is left as it is.
func AsPublished(content []byte) []byte { … }
```

## Testing Strategy

All on Linux unless marked: line endings are bytes, so the behaviour is the same on every OS.

- **The reader:** CRLF → LF; a lone CR kept; LF unchanged; a binary file with CR bytes unchanged.
- **publish:** a repo whose files are CRLF hashes as the LF repo does, so an unedited CRLF file isn't sent; a really edited one is sent, as LF.
- **pull:** over CRLF files that weren't edited, pull updates them without an "edited here" refusal, and they stay CRLF; a real change on each side of one CRLF file merges cleanly (no whole-file conflict), written with CRLF; a conflict's markers in a CRLF file are CRLF too; a new file is LF; a binary file is written as it is.
- **init:** `.gitattributes` written; an existing one gets only the missing lines; a non-regular one refused, as `.gitignore`.
- **Added lines:** appending to a CRLF `config.yml` and `AGENTS.md` keeps CRLF; to an LF one, LF.
- **On Windows 11, end to end, with `acme`** (the user approved publishing there; same round trip as `win-files-auth`): in the pulled folder, save a `sales` model with CRLF (as Notepad does); `publish status` shows nothing to publish; change one line, publish: only that file is sent, and pulled into a fresh folder it is LF; put it back and publish again. A pull into the CRLF folder updates it and keeps it CRLF.

## Boundaries

- **Always:** keep binaries byte for byte; keep the server's hash format; keep each file's own line ending when anfra writes it; run Go lightly (`GOMAXPROCS=4 GOFLAGS=-p=2`), build and lint once per task, check Windows' free memory before running there.
- **Ask first:** each publish to `acme` beyond the round trip above; changing `shared/` (public API) beyond what the merge already exposes; a new dependency.
- **Never:** rewrite a user's file just to change its line endings; change how a file compiles; publish anywhere but `acme`.

## Success Criteria

1. A repo saved with CRLF and the same repo with LF hash the same: `publish` sends nothing for line endings alone, and sends a really edited file as LF.
2. `pull` updates unedited CRLF files without refusing, merges a change on both sides of a CRLF file cleanly, and leaves CRLF files CRLF (new files LF).
3. Binary files pass through publish and pull byte for byte.
4. `anfra init` writes `.gitattributes` with `models/** datasets/** apps/** .anfra/** text=auto eol=lf`, adding missing lines to an existing one.
5. Lines anfra adds to `config.yml` and `AGENTS.md` keep the file's line ending.
6. On Windows 11 with `acme`: a CRLF-saved model shows nothing to publish; a one-line change publishes only that file, as LF; the round trip leaves `acme` with the same files.
7. Tests of every touched package pass on Linux; the Windows check recorded; `docs/` and `WINDOWS-PLAN.md` updated.

## Open Questions

None.
