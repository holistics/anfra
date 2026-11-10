# treemerge

> Status: proposed, not built. Two packages: `shared/treemerge`, which merges three versions of a repo's files, and `shared/textmerge`, which merges three versions of one file's text.

`treemerge` merges three versions of a set of files: a base, and two sides that each changed it ("ours" and "theirs"). It answers the merged files, what happened to each path, and the conflicts it couldn't decide. It's what `anfra pull` merges the org's files into a folder with ([publish](publish.md)), and what a platform can merge edits with, such as a change made in a web app onto a live revision that moved since.

It's a pure library: no file system, network, git or anfra state. Callers hand it the three versions as data and decide what to do with the result. That keeps the merge identical wherever it runs, and lets a platform that embeds anfra merge the same way the CLI does.

Its behaviour follows git's merge, as a baseline ([below](#git-as-the-baseline)): readers and agents already know it, and it can be checked against git exactly.

## What it owns, and what it doesn't

`treemerge` owns everything about *deciding* a merge, and nothing about where files come from or go:

| It owns | Its callers own |
|---|---|
| What each path becomes, from its three versions | Where the three versions come from (a folder, a server, a git repo) |
| Merging a file's content, by the file's merger | Choosing the base |
| What a conflict is: its kind, its paths, its versions, where it is | Keeping conflicts between runs |
| How a conflict is shown as text (markers), and how to tell it's resolved | Writing files, backups and undo |
| What taking one side of a conflict means | Whether the merged files are valid AML, and what's published |

## Concepts

- **Tree:** a set of files, as slash-separated path → blob id. A blob id is git's SHA-1 of the file's bytes, which anfra already uses for publishing (`repoformat.BlobID`).
- **Blob reader:** how `treemerge` reads a file's content by blob id, when it needs it. The caller decides where content lives.
- **Base, ours, theirs:** the three trees. Callers name the sides for their readers through labels: `anfra pull` calls ours "local" and theirs "org".
- **Outcome:** what happened to a path: `unchanged`, `ours` (ours kept), `theirs` (theirs taken), `merged`, or `conflict`, with the source path when it was renamed.
- **File merger:** what merges one file's content when both sides changed it, chosen by the file's path. The default is the text merger (`textmerge`, git's behaviour), and binary files get the binary merger, which always conflicts. A different merger, such as one that understands AML, is a [declared extension](#declared-extensions), like a merge driver that git's `.gitattributes` assigns to some paths.
- **Conflict:** a path, or pair of paths, that `treemerge` won't decide. It has a kind, the blob ids of its three versions (any may be absent), and, for a conflict inside a file, its regions ([below](#conflicts-inside-a-file)).
- **Region:** one conflicting place inside a file: the base's, ours' and theirs' fragments, the lines it covers in the rendered file, and its location when the merger knows it.
- **Location:** where a region sits by meaning, beyond its lines: for an AML merger, "dimension `revenue` of model `orders`". The text merger gives none.
- **Hunk:** a stretch of a merged text file: clean lines, or a conflict holding the base's, ours' and theirs' lines. Each conflicting hunk becomes a region.
- **Markers:** a conflict written into the file as text, for a person or an agent to resolve in place.

### Conflict kinds

Each kind is a stable code: callers branch on it, show it, and return it in their own results. The names follow the conflict types git's merge reports (`CONFLICT (content)`, `(add/add)`, `(modify/delete)` and so on), so that readers who know git recognise them. `path_collision` is ours: git only warns about such paths when it checks files out.

| Kind | When |
|---|---|
| `content` | Both sides changed a text file, and the changes overlap or touch |
| `add_add` | Both sides added a file at the same path, with different content, and their lines conflict |
| `modify_delete` | Ours changed a file that theirs deleted |
| `delete_modify` | Ours deleted a file that theirs changed |
| `binary` | Both sides changed a binary file differently |
| `rename_rename` | The sides renamed the same file to different paths |
| `rename_delete` | One side renamed a file the other deleted |
| `file_folder` | A path is a file on one side and a folder on the other |
| `path_collision` | Two paths would be one file on a system that ignores case or Unicode form (`Orders.aml` and `orders.aml`) |

### Conflicts inside a file

A conflict is kept per file, as git and jj keep it. A `content` or `add_add` conflict also holds its **regions**: one per conflicting place in the file, in order. File-level kinds (`modify_delete`, `binary`, `rename_rename` and so on) have none.

| A region holds | Why |
|---|---|
| The base's, ours' and theirs' fragments | A caller can show and resolve the region without re-running the merge |
| The lines it covers in the rendered file | Markers and regions match one to one, so a caller showing the file can point at each |
| Its location, when the merger knows it | A caller can show the conflict by meaning ("dimension `revenue` of model `orders`, changed on both sides") instead of by line numbers |

**Locations come from the merger that knows them.** The text merger sees lines, so its regions have no location. An AML merger ([declared extensions](#declared-extensions)) matches models, datasets and fields by name as it merges, so each region it leaves has one. A location is a path into the file's structure, such as model `orders`, then dimension `revenue`. Either way, the regions render to the same git markers, so a caller that only edits files (the CLI) treats every conflict alike, and only a caller that shows meaning (a web app) reads locations.

**Resolution stays per file.** A file is resolved when its markers are gone ([flow 4](#4-check-whether-a-conflict-is-resolved)), however many regions it had. A caller that resolves through data, not by editing the file, will take a side for one region at a time (`TakeRegion`, later): the file is rendered again from the conflict's regions, with that region resolved and the others still between markers. That's a way to resolve after the merge, like the per-conflict choices of git's merge tools, so it doesn't contradict git's result.

**References.** Keeping conflicts as data, per file, follows jj. Addressing a conflict by identity rather than by line, and taking a side for one such conflict, follows Dolt, which keeps each conflicting row of a table, by its key, as a row of its own conflicts table, resolved with `--ours` or `--theirs` per row. How an AML merger would find locations follows Mergiraf, a structured merge driver for git, which matches syntax nodes by a signature, such as a name, and still writes git's markers.

## Design rules

- **git is the baseline.** For any input, the default merge gives git's result, or a conflict where git would have resolved. Anything beyond git is a [declared extension](#declared-extensions).
- **A loud conflict beats a quiet guess.** When `treemerge` can't tell what both sides meant, it reports a conflict instead of choosing. Changes that overlap or touch conflict. Only identical changes on both sides resolve on their own.
- **Never drop a side.** Every conflict keeps every version that exists: a modified file isn't deleted, a renamed file isn't lost, and two paths that collide are both reported.
- **Content before names.** Paths are compared by blob id before any content is read, and a rename is found by content (the same blob id at a new path), not guessed from names.
- **A conflict is data, and markers are one way to show it.** A conflict is its kind, its blob ids and its location. Markers are rendered from that, so a caller can keep only the data and show the conflict however suits its readers.
- **Deterministic.** The same input gives the same output on every platform: sorted paths, no dependence on map order, locale or time.
- **Stable codes, apart from wording.** Outcomes and conflict kinds are codes that callers rely on. Messages for people are the callers'.

## Supported flows

These are the flows `treemerge` supports and keeps working. Each is a function of its inputs. Callers combine them into their own flows, as [below](#how-callers-use-it).

### 1. Merge three trees

`Merge(base, ours, theirs, blobs, options) → Result`

1. **Check paths.** Find paths that collide ignoring case or Unicode form, and paths that are a file on one side and a folder on the other. Each becomes a `path_collision` or `file_folder` conflict, and isn't merged further.
2. **Pair renames.** Among paths that one side deleted and the other changed, pair each with a path the deleting side added with the same blob id. A pair is then merged as one file, at its new path. Only these paths are considered, so the cost stays small however many files moved.
3. **Classify each path,** or rename pair, from its three blob ids alone:

   | Base | Ours | Theirs | Outcome |
   |---|---|---|---|
   | any | same as theirs | same as ours | `unchanged` (or both took the same change) |
   | X | X | Y, or absent | `theirs` |
   | absent | absent | Y | `theirs` |
   | X | Y, or absent | X | `ours` |
   | absent | Y | absent | `ours` |
   | X | Y | Z | merge the content (step 4) |
   | absent | Y | Z | merge the content from an empty base (step 4) |
   | X | absent | Y | `delete_modify` |
   | X | Y | absent | `modify_delete` |

4. **Merge content,** only for paths both sides changed, with the file's merger. Read the versions through the blob reader. If any is binary (a NUL byte in its first 8,000 bytes, or not valid UTF-8), the binary merger makes it a `binary` conflict. Otherwise the text merger ([flow 6](#6-merge-one-files-text)) gives a `merged` file from clean hunks, or a `content` (or `add_add`) conflict, rendered with markers ([flow 3](#3-show-a-conflict-as-markers)).
5. **Answer the Result:** for every path, its outcome, its conflict if any, its blob ids, the source path of a rename, and the content of any new file (merged, or with markers).

Content is read only in step 4, and only for the files that need it.

### 2. Merge with no base

The same flow with an empty base tree, for a caller that has no common version, as git does when merging unrelated histories. Files only one side has are kept, identical files are unchanged, and files that differ are merged from an empty base, so their differing lines conflict as `add_add`. A file one side deleted and the other still has is kept: with no base, a deletion and an addition look the same, and keeping loses nothing.

### 3. Show a conflict as markers

`Render(conflict, blobs, options) → bytes`

A conflict's text form, written into the file for a person or an agent to resolve. It's rendered from the conflict's blob ids, so a caller that kept only the ids can render it again, and gets the same bytes. The format is git's, so it's what anyone who has resolved a git conflict expects:

```
<<<<<<< ours (local)
  measure revenue { type: 'number' definition: @aql sum(orders.amount) ;; }
||||||| base (acme@8f3a91c)
  measure revenue { type: 'number' definition: @aql sum(orders.price) ;; }
=======
  measure revenue { type: 'number' definition: @aql sum(orders.net_amount) ;; }
>>>>>>> theirs (org acme@4c1d07e)
```

- **diff3 style, with the base.** The base's section shows what each side changed, which a person or an agent needs to decide.
- **Labels are the caller's,** after the side's name.
- **Marker length** is 7, git's default, unless one of the three versions already has a line that looks like a marker. Then it's that line's length + 4 (jj's rule for choosing it), which git allows through its per-path marker size. The length is part of the conflict, so checking resolution uses the same length.
- **Line endings** follow the file's: CRLF when the lines around use it, as git does.
- **A side without a final newline** gets one before the next marker, as git does. That the side had none is lost, which is rare in AML and doesn't affect the merged file.
- **`modify_delete` and `delete_modify`** put the whole file between markers, with the deleted side empty.
- **`binary`, `rename_rename`, `rename_delete`, `file_folder` and `path_collision`** have no markers. A caller shows them its own way, and resolves them by taking a side ([flow 5](#5-resolve-by-taking-a-side)).

### 4. Check whether a conflict is resolved

`Resolved(conflict, current) → resolved, or still conflicted`

Given a conflict and the file's current content, whether it's resolved:

- **Absent:** resolved, as a deletion.
- **Text:** still conflicted if any line starts with a marker character repeated at least the conflict's marker length, followed by a space or the line's end. Otherwise resolved, as the current content.
- **No markers to check** (`binary`, and the other kinds without markers): never resolved by content, only by taking a side.

This isn't part of git's merge. git learns that a conflict is resolved when the user says so (`git add`). `treemerge`'s callers have no such step, so resolution is read from the file, as jj does. It's stricter than jj: a malformed marker line still counts as unresolved, because a caller may publish what's resolved, and marker lines in a published file are never wanted.

### 5. Resolve by taking a side

`Take(conflict, side) → the paths and content that side means`

What "take ours" or "take theirs" means for each kind: that side's version of every path the conflict covers, where an absent version means the path is deleted. For `rename_rename`, taking a side keeps that side's path and removes the other. For `file_folder` and `path_collision`, it keeps that side's paths. The caller writes the result. This matches git's `checkout --ours` and `--theirs`.

Later, for a caller that resolves through data rather than by editing the file, such as a web app: `TakeRegion(conflict, region, side)`, which re-renders the file from the conflict's regions with that region taken from one side and the rest still conflicting ([below](#conflicts-inside-a-file)).

### 6. Merge one file's text

`textmerge.Merge(base, ours, theirs, options) → hunks`

The text merger, on its own, for a caller merging a single file (an editor, say), and what [flow 1](#1-merge-three-trees) uses for text files. It's a diff3 merge ([below](#how-diff3-works)), with git's rules at what git calls its "eager" level:

1. Split each version into lines, keeping each line's ending.
2. Diff the base against ours, and against theirs, line by line.
3. Walk the base's lines. A region only one side changed takes that side. A region both sides changed the same way is taken once. A region both changed differently, or two changes that touch with no unchanged line between them, is a conflicting hunk.

The eager level is the most git does with diff3-style markers: its further levels shrink conflicts in ways a marker with the base can't show. `textmerge.Render` and `textmerge.Parse` write and read the markers for flows 3 and 4.

#### How diff3 works

diff3 is the classic three-way text merge, named after the Unix tool that introduced it, and the algorithm behind `git merge-file`. It never compares the two sides with each other directly. It compares each side with the base, and uses the base to tell who changed what:

1. **Diff base → ours and base → theirs.** Each diff marks which of the base's lines that side kept, and what it changed.
2. **Find the stable lines:** base lines that *both* sides kept unchanged. They split the file into regions.
3. **Decide each region between stable lines:**

   | Ours | Theirs | Result |
   |---|---|---|
   | Same as base | Same as base | The base (a stable region) |
   | Changed | Same as base | Ours |
   | Same as base | Changed | Theirs |
   | Changed | Changed the same way | Either, once |
   | Changed | Changed differently | Conflict: ours, base and theirs |

For example, with a base of five lines, A to E:

```
base:    A  B  C  D  E
ours:    A  B2 C  D  E        (changed B)
theirs:  A  B  C  D2 E        (changed D)
```

A, C and E are stable, so B and D are separate regions. B takes ours, D takes theirs, and the merge is clean: `A B2 C D2 E`. Had theirs changed C instead of D, the changes to B and C would touch, with no stable line between them, and the region `B C` would be a conflict, though the two edits might not have been related. That's the cost of a line merge: it sees lines, not meaning, so it conflicts whenever it can't separate two changes by an unchanged line. A merger that understands AML is how that cost goes down ([declared extensions](#declared-extensions)).

### 7. Classify without merging

`Classify(base, ours, theirs) → each path's outcome`

Steps 1 to 3 of [flow 1](#1-merge-three-trees), without merging content: path checks, rename pairing and classification, from blob ids alone. No content is read, and no blob reader is needed. Paths both sides changed differently come back as needing a merge, without saying whether it would be clean.

It answers questions about a merge without doing it, cheaply enough to ask before every publish. `anfra publish` uses it to tell whether a pull would change the folder: it wouldn't when every path is `unchanged`, or kept as ours has it ([publish](publish.md#when-a-pull-would-change-nothing)).

## git as the baseline

**The rule:** for any input, the default merge gives git's result, or a conflict where git would have resolved. The references are git's merge (its `ort` strategy, which `git merge-tree` runs) and `git merge-file`, configured to diff the way `textmerge` does (Myers, for now). git's code is never copied.

The rule keeps scope a choice: `treemerge` never has to match all of git's behaviour, only never contradict it. Each case it doesn't handle yet is a conflict where git would resolve, and handling it later turns that conflict into git's result. It also makes the merge exactly testable against git ([testing](#testing)).

### What `treemerge` does as git does

- Paths decided by blob id before reading content.
- The diff3 line merge at the eager level: touching changes conflict, identical changes resolve.
- Markers in diff3 style, with the base, in the file's line endings. 7 characters, or a longer per-path size.
- A missing final newline, added before a marker.
- Binary files detected by a NUL byte in the first 8,000 bytes, and never given markers.
- add/add merged from an empty base. modify/delete a conflict, with the modified file kept.
- Exact renames, looked for only among paths one side deleted and the other changed. rename/rename and rename/delete as conflicts, keeping every path.
- With no base, an empty one, as for unrelated histories.
- Taking a side, as `checkout --ours` and `--theirs`.

### Where it conflicts instead, for now

Each of these is a case git resolves, or handles more helpfully, and `treemerge` reports as a conflict until it's built:

| Case | git | `treemerge` now |
|---|---|---|
| Renames by similarity (unique basename, or content at least 50% similar) | Followed | Conflicts, as modify/delete and an add |
| Directory renames | Suggested, with a conflict | Not followed |
| A file on one side, a folder on the other | The file moved to `path~side`, conflict | `file_folder` conflict, nothing moved |
| Histogram diff | git's default for merges | Myers, which may conflict where histogram wouldn't, or place a conflict differently. Measured on AML before deciding |

### Where it's louder than git

| Case | git | `treemerge` | Why |
|---|---|---|---|
| Paths colliding by case or Unicode form | Merged as different paths, a warning when checking out | `path_collision` conflict | On macOS and Windows, writing both silently loses one |
| A file that isn't valid UTF-8 | Merged as text, unless it has a NUL byte | Binary | AML and app files are text, so it's almost certainly a binary file |

### What it adds outside git's merge

- **Conflicts as data,** rendered as git's markers on demand. git keeps conflicts in its index. Storing them as data is jj's model, and lets a caller keep and show conflicts its own way.
- **Resolution read from the file** ([flow 4](#4-check-whether-a-conflict-is-resolved)), where git waits for `git add`. Also from jj, made stricter.

### What it leaves out

Not planned, though each could come back later, as git's options:

- Shrinking conflicts further: trimming lines both sides share at a conflict's edges (`zdiff3`), and merging conflicts less than 3 lines apart.
- Ignoring whitespace-only changes (`-X ignore-space-change`).
- Renormalising line endings before merging: blob ids must stay the exact bytes.
- Remembering resolutions (`rerere`).

jj-only behaviour is out: word-level merging would contradict git's result, and so would reading a half-edited file back as a smaller conflict. Taking a side for one region at a time ([below](#conflicts-inside-a-file)) is different: it's a way to resolve, from the conflict's data, after the merge, as git's merge tools offer, and changes nothing the merge decided.

## Declared extensions

The one way past git. An extension is chosen deliberately, has its own rules and tests, and, compared with the baseline, may only:

- **resolve what the baseline left as a conflict,** or
- **conflict where the baseline resolved,** when it knows better (for example, two sides adding the same AML field at different places, which merges cleanly as text into invalid AML).

It never resolves something differently from how the baseline resolved it.

| Extension | What it adds | Status |
|---|---|---|
| **A file merger for AML** | Merges by models, datasets and fields, keyed by name, so changes to different fields don't conflict however close their lines. Falls back to the text merger when a version doesn't parse, or for a change it doesn't understand. Gives each conflict a location (model, field), and still renders git's markers | Later |

Maybe later: renames recorded by an editor that knows them, such as a web app's rename action, passed in instead of detected, if detection by content proves insufficient.

## How callers use it

Two callers, as examples. Neither flow is `treemerge`'s: each caller combines the flows above with its own storage.

**`anfra pull`** ([publish](publish.md)) picks the base from its sync state, builds the three trees from its state, the folder and the org, and runs [flow 1](#1-merge-three-trees) (or [2](#2-merge-with-no-base) with no base). It writes the result, and keeps each conflict's kind, blob ids and marker length in its pending record. At the next pull or publish it runs [flow 4](#4-check-whether-a-conflict-is-resolved) on each conflicted file, and `anfra pull --take` runs [flow 5](#5-resolve-by-taking-a-side).

**A platform merging an edit** (a change made in a web app, started from a revision that has since moved) runs [flow 1](#1-merge-three-trees) with the starting revision as the base, the edit as ours, and the live revision as theirs. A clean result is published as usual. Conflicts are stored as data, with their kinds, blob ids and locations, and shown in the platform's own way, rendered with [flow 3](#3-show-a-conflict-as-markers) or not at all, and resolved with [flow 5](#5-resolve-by-taking-a-side).

## Architecture

```
 treemerge                          three trees in, Result out
   paths      check and normalise paths: case and Unicode collisions, file vs folder
   renames    pair deleted and added paths
   classify   each path's outcome from its three blob ids
   mergers    the file merger per path: binary, text (textmerge), later AML
 ─────────────────────────────────────────────────────────────────────────────
 textmerge                          one file's three versions in, hunks out
   markers    hunks ⇄ text with markers
   diff3      the line merge and its policy
   linediff   line diff (internal), behind an interface
```

- **Dependencies point down.** `treemerge` uses `textmerge`. Neither uses anything else in anfra, nor any I/O. An AML merger would live outside both, and be passed in.
- **Every input is data or an interface:** trees, a blob reader, options. A caller's context cancels a long merge.

### The API

```go
package treemerge

type BlobID [20]byte
type Tree map[string]BlobID // slash-separated paths

type BlobReader interface {
	ReadBlob(ctx context.Context, id BlobID) ([]byte, error)
}

// FileMerger merges one file's content. The text and binary mergers are built in.
type FileMerger interface {
	Merge(ctx context.Context, path string, base, ours, theirs []byte) (FileResult, error)
}

type Options struct {
	Labels      [3]string                 // shown after "base", "ours" and "theirs" in markers
	Mergers     func(path string) FileMerger // nil, or nil for a path: the built-in mergers
	MaxFileSize int                       // larger files are treated as binary
}

type Result struct {
	Paths    []PathResult      // sorted by path
	NewBlobs map[BlobID][]byte // merged files, and files with markers
}

type PathResult struct {
	Path, From string // From is set for a rename
	Outcome    Outcome
	Conflict   *Conflict
	Result     *BlobID // the file's new content, absent when it's deleted
}

type Conflict struct {
	Kind               Kind
	Paths              []string   // every path it covers
	Base, Ours, Theirs *BlobID    // absent versions are nil
	Regions            []Region   // conflicts inside the file, for content and add_add
	MarkerSize         int        // for kinds with markers
}

type Region struct {
	Base, Ours, Theirs []byte     // the fragments; empty when a side has none
	Lines              LineRange  // in the rendered file, between its markers
	Location           []Step     // e.g. model "orders", dimension "revenue"; empty from the text merger
}

func Merge(ctx context.Context, base, ours, theirs Tree, blobs BlobReader, o Options) (Result, error)
func Render(ctx context.Context, c Conflict, blobs BlobReader, o Options) ([]byte, error)
func Resolved(c Conflict, current []byte, exists bool) bool
func Take(c Conflict, side Side) []Change
func TakeRegion(c Conflict, region int, side Side) ([]byte, error) // later
func Classify(ctx context.Context, base, ours, theirs Tree, o Options) ([]PathResult, error)
```

The names are a sketch, to settle when it's built.

### Extension points

What's planned plugs in at a named place, without changing the flows:

| Extension point | Where | Now | Later |
|---|---|---|---|
| Line diff | `textmerge` | Myers | Histogram, if it merges AML better |
| File merger, chosen per path | `treemerge` mergers | Binary, or text | AML by structure, a [declared extension](#declared-extensions) |
| Rename pairing | `treemerge` renames | Exact | By content similarity, as git does, with a cap on how many pairs it scores |
| Path policy | `treemerge` paths | Report collisions and file-vs-folder as conflicts | Move the file to a sibling path in a file-vs-folder clash, as git does |

## Testing

| What | How |
|---|---|
| The baseline rule | Run the same merges through `git merge-tree --write-tree` and `git merge-file --diff3`, configured to diff as `textmerge` does, with git's version pinned, since its behaviour changes between releases. Each result must be git's, or a conflict where git resolved. Inputs: generated edits, and pairs of changes from real AML repos' histories |
| `textmerge` | Fixed cases for each diff3 rule, plus the comparison with `git merge-file` |
| Markers | Rendering then checking resolution: a fresh render is unresolved, removing every marker resolves it, for every line ending and marker length. Fuzzing the check |
| `treemerge` | A test for each classification row and each conflict kind, and scenario fixtures (renames, collisions, no base) |
| Declared extensions | Against the baseline: each result is the baseline's, a resolution where the baseline conflicted, or a conflict where it resolved, never a different resolution |
| Determinism | The same merge on every platform CI runs, compared byte for byte |

## Dependencies

- **The line diff:** `github.com/znkr/diff` (Myers, Apache-2.0), behind the line diff interface.
- **diff3:** written here, from the published algorithm.
- **git's merge rules** are taken by behaviour, and checked by the tests against git. Its code is never copied: git's `xdiff/xmerge.c` is LGPL.
- **Later, if needed:** a port of git's histogram diff (`xdiff/xhistogram.c`, which comes from JGit under the Eclipse Distribution License, BSD-style), and a vendored copy of go-git's rename similarity code (Apache-2.0, also from JGit). Vendored code keeps its licence header, and lives where it can be listed.

## Planned

| | `textmerge` | `treemerge` |
|---|---|---|
| **First** | Myers, diff3 at git's eager level, git's markers with the base, the strict resolution check | Path checks (collisions and file-vs-folder reported), classification, `Classify`, no base, binary, `Render`, `Resolved`, `Take`, the comparison with git |
| **Next** | Marker size grown past marker-like lines. Measuring Myers against histogram on AML | Exact renames, `rename_rename`, `rename_delete`. `Conflict` with regions, from the text merger, without locations |
| **Later** | Histogram, if measured better | Renames by similarity, moving the file in a file-vs-folder clash, the AML merger and its locations, `TakeRegion` |
