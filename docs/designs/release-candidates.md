# Release candidates

A proposal. Each anfra release starts as a release candidate (rc), such as `0.23.0-rc.1`. anfra Cloud releases with the rc first, and only then does the same code go out as the release that users get, `0.23.0`. [release.md](release.md) describes how releases work today. This doc describes what changes.

## Why

anfra Cloud runs anfra core as a Go module, and its web app uses anfra's SDK and design packages from npm. All three come from an anfra release, so a new anfra feature reaches anfra Cloud only after anfra has released it. But users already have the release by then, because `anfra update` installs it as soon as it's out. Until anfra Cloud releases and rolls out the new version, a user can run a feature that anfra Cloud doesn't have yet.

An rc reverses the order. anfra Cloud gets the feature first, and users get it once anfra Cloud can handle it.

## Terms

- **rc PR:** the pull request `release: 0.23.0-rc.N`. Merging it tags the rc.
- **Release PR:** the pull request `release: 0.23.0`. Merging it releases an rc's commit as `0.23.0`. Today the daily job (`release_pr.yml`) opens the release PR. Under this proposal it opens the rc PR, and the rc's build opens the release PR.
- **A waiting rc:** an rc that hasn't been released yet. That's the case when the newest `v*` tag, by semver, is `vX.Y.Z-rc.N`, and there's no `vX.Y.Z` tag.

## Decisions

### Updates skip rcs

An rc is published much as a release is, so anyone can install one by its version. What has to change is that `anfra update` and its version checks never pick one:

- **`anfra update`, and the background check that announces newer versions,** find the latest release through GitHub's `/releases/latest`, which skips a release marked as a prerelease. So the rc's GitHub release is marked as one. Today every release is marked as the latest.
- **The skills:** `anfra update` checks their version on holistics/anfra-skills' main branch, so an rc doesn't publish them. The release does.
- **npm:** `npm install @holistics/anfra-sdk`, with no version, installs what the `latest` dist-tag points at, not the highest version. The `sdk` job already publishes a version with a `-` under `next` instead, so `latest` moves only with a release. Version ranges such as `^0.22.0` don't match a prerelease either, so `npm update`, Renovate and Dependabot don't move anyone onto an rc unless they're on one already. The same goes for `@holistics/anfra-design`.
- **Someone running an rc** is offered the release it leads to, since `0.23.0` is newer than `0.23.0-rc.1`.

`install.sh` follows `/releases/latest` too, so it installs the release, not the rc.

### The release is the rc's commit

The release has to be exactly the code that anfra Cloud ran as the rc. If it includes commits merged after the rc, users get features anfra Cloud doesn't have, which is the gap this proposal closes.

The repo requires a linear history, so a pull request lands by squash or rebase, on top of whatever main has gained since. A commit made after the rc therefore can't hold the rc's code. Instead, **the release tags the rc's own commit**. `v0.23.0` and `v0.23.0-rc.1` point at the same commit.

That commit's `manifest.yml` says `0.23.0-rc.1`, so the version can't come from the commit. **The build takes the version from the tag.** It passes the version to the binary (`meta.Version`), the npm packages (`npm pkg set version`) and the image tags.

### The release PR names its rc

The release PR sets `version: 0.23.0`, and adds `promoted_from: 0.23.0-rc.1` to `manifest.yml`. Tagging tags exactly that rc's commit, and fails if a newer rc of `0.23.0` exists.

Without the field, tagging would have to pick the newest rc when it runs, and that can be a different rc from the one the release PR's reviewer approved. For example, if rc.2 merges while the release PR for rc.1 is still open, the release PR conflicts on `version:`. The obvious resolution keeps `0.23.0`. Tagging the newest rc would then release rc.2, which anfra Cloud never ran. With the field, that resolution keeps `promoted_from: 0.23.0-rc.1`, and tagging fails. The only way to release rc.2 is to write `0.23.0-rc.2` in the field, which is a deliberate decision to release it.

`promoted_from` stays on main until the next rc PR, which sets `version` to the next rc and removes the field. Until then the field is accurate history: 0.23.0 is that rc's commit. Only the tagging run that the release PR's merge starts reads it.

### Each rc PR writes its rc's notes

Each rc PR adds a section for its rc to the top of `CHANGELOG.md`, such as `## [0.23.0-rc.1]`, with the commits since the newest tag, rc or release. `bump.sh` does that today already, since it generates from the last `v*` tag, and rc tags count. `skills/CHANGELOG.md` gets a section the same way, when a commit since the newest tag changed the skills.

- **Each rc's notes are edited in its rc PR,** where they're reviewed like any other change. A later rc adds its own section, and leaves the earlier ones, edits included, as they are.
- **Each rc's GitHub release shows its own section,** which is what changed since the previous rc. That's what anfra Cloud needs to check it. `release-notes.sh` finds it as it finds a release's section today, by the tag's version.
- **Main's changelog doesn't claim a version is out before it is.** While an rc waits, its sections say `-rc.N`.

### The release PR merges the rc sections

The release PR replaces every rc section above the last release's with one section for the release, `## [0.23.0]`. A script does the merge, from the rc sections at the rc's tag, not on main. A change made to an rc section on main after its tag is therefore dropped, because the release's build works from the tag and wouldn't have it either. The script keeps each line as written. It puts the bullets of every rc under one heading per `###` group (Features, Bug Fixes and so on), newest rc first. Any other text in the rc sections goes above the groups, newest rc first too. The release's heading gets its own compare link, `v0.22.0...v0.23.0`, and the date of the merge. That covers rcs of a smaller version too: after `0.22.1-rc.1` and `0.23.0-rc.1`, the `0.23.0` section holds both.

The release's build checks out the rc's commit, whose changelogs have only the rc sections. So it runs the same merge in its checkout, before anything reads the changelogs. The files then match what the release PR committed to main, and everything that reads them works as it does today: the release notes (`scripts/release-notes.sh`), and the skills, which are published with `skills/CHANGELOG.md`.

### The release PR is exactly what the bot generates

To change the notes, edit an rc PR, or cut another rc. The release PR itself takes no changes, so that main's changelog always says what the release page says.

CI checks it on a PR that changes `version` from an rc to a version without a prerelease part, which is a release PR. It runs the bot's own generation (`scripts/release-pr.sh --dry-run`) for the PR's `promoted_from`, against the PR's base, and fails unless the PR's diff is exactly the generated one. That catches any change the bot didn't make: an unrelated file, a version edited by hand, or a reworded line in the merged notes. The message says to edit the notes in an rc PR and let the bot regenerate the release PR. Because the generation reads the rc sections at the rc's tag, its result doesn't change as main moves on.

The check has to be part of the required `ci_passed`, so that a failure blocks the merge. It belongs on the PR, not at tagging. By tagging, the release PR has merged, and fixing main's changelog would need another PR.

### `anfra update` reads the changelog from the release

After an update, `anfra update` prints the notes of each release since the running version. Today it reads them from `CHANGELOG.md` at the new version's tag (`internal/update/notes.go`). Under this proposal that tag is the rc's commit, whose `CHANGELOG.md` has only the rc sections, and `anfra update` skips sections whose version has a prerelease part (`notes.go:88`). So it would show nothing for the release itself.

So the release's build uploads its whole `CHANGELOG.md`, after the merge, to the GitHub release, in the same upload as the binaries. `anfra update` downloads `releases/download/v<version>/CHANGELOG.md` instead.

- **It's one request,** and it still covers releases the user skipped, because the file holds every section up to that release.
- **It uses no GitHub API, so no rate limit applies.** It's the same download path `anfra update` already uses for the binary.
- **It's fixed at release time.** Later edits to main's `CHANGELOG.md` don't change what a release says.

The link to the full changelog (`ChangelogURL`) points to the GitHub release page instead of `CHANGELOG.md` at the tag.

Versions released before this change still read `CHANGELOG.md` at the tag. When they update to a release made from an rc, they print the notes of every release in between except the newest. Ship this change in a release before the first rc, so that as few users as possible see that.

### One release at a time

While an rc waits, the daily job doesn't cut another one. Changes merged in the meantime haven't been through anfra Cloud, so they wait for the next rc, after the release. A change that has to ship before the release goes out in the next rc of the same version, such as `0.23.0-rc.2`, cut by hand.

Without the pause, the daily job would cut a new rc whenever main has something releasable. The release PR would move to each new rc, and anfra Cloud would have to bump, release, roll out and check each one before the release could merge. On an active main, anfra Cloud would never catch up, and users would never get the release. Running several releases in parallel instead, with later rcs taking the next version, would need release notes that start from a release that doesn't exist yet, and several waiting rcs that every step would have to tell apart.

## The daily job

The daily job (`release_pr.yml`, Monday to Saturday) reads one fact, from the tags: whether an rc is waiting.

| State | Daily run | Manual run |
|---|---|---|
| No rc waiting | As today, but for an rc. It rebuilds `release/rc` from main, and opens or refreshes the rc PR, `release: 0.23.0-rc.1`. If nothing on main is releasable, it closes that PR. | Cuts the rc now, or the version you name |
| An rc waiting | Doesn't cut an rc. It comments on the release PR (below). | Cuts the next rc, on `release/rc`. That's `0.23.0-rc.2`, or `0.24.0-rc.1` if main's commits now call for a bigger version. |

- **Each kind of PR has its own branch, which the bot owns.** `release/rc` holds the rc PR, and `release/next` holds the release PR. Each is rebuilt and force-pushed by the step that opens its PR, so there are no branches to name or clean up, and cutting an rc never overwrites the release PR. The manual run's `branch` input stays, for an rc that needs commits of its own, such as sidecar pins.
- **The pause starts when the rc's tag is pushed.** Until then, the rc PR is the daily job's ordinary PR, rebuilt each day from main.
- **The pause ends when the release's tag is pushed.** On its next run, the daily job finds the release is the newest tag, and starts over from the commits since.
- **Closing the release PR doesn't end the pause,** because the rc is still waiting. An abandoned rc usually needs fixing, and the fix goes out as the next rc, cut by hand. Ending the pause any other way means releasing the rc or deleting its tag, and both are deliberate. Because the pause depends on tags only, a PR closed by mistake can't start a new cycle on top of an rc that anfra Cloud may already run.

### The daily comment

While an rc waits, each daily run comments on the release PR, if main has releasable commits since the rc. If it doesn't, nothing is held back, and the run doesn't comment. The comment says:

- which rc is waiting, and for how long: "`0.23.0-rc.1` was tagged 3 days ago"
- what it holds back: the releasable commits merged since the rc, as the changelog would list them
- what to do: merge the release PR once anfra Cloud runs the rc, or cut the next rc by hand to include those commits, with a link to the workflow

Each new comment notifies the PR's watchers, which an edit wouldn't. The run also hides its previous comment as outdated (GraphQL `minimizeComment`), so only the latest one shows.

If an rc is waiting but no release PR is open, the run fails, with a message to cut the next rc by hand or to reopen the release PR. A failed scheduled run notifies whoever last changed the workflow's schedule, and shows in the Actions tab.

## The sequence

For `0.23.0`, after `v0.22.0`. 🤖 marks a CI step, and 👤 marks a person.

### anfra cuts the rc

1. 🤖 The daily job opens the rc PR, `release: 0.23.0-rc.1`, from main on `release/rc`. It sets `version` in `manifest.yml`, `web/sdk/package.json` and `web/design/package.json`, and removes any `promoted_from`. It adds `## [0.23.0-rc.1]` to the top of `CHANGELOG.md`, and to `skills/CHANGELOG.md` if the skills changed. Edit the notes here if they need it.
2. 👤 Merge it.
3. 🤖 `tag_and_release.yml` sees a version with a prerelease part, and tags `v0.23.0-rc.1` on main's HEAD, as it tags every release today.
4. 🤖 `build_release.yml` builds the rc's tag, and publishes it, marked as a prerelease ([Updates skip rcs](#updates-skip-rcs)). The GitHub release's notes are the rc's section, and GitHub's generated notes since the previous tag.
5. 🤖 The same run opens the release PR, `release: 0.23.0`, from main on `release/next`. It sets the three versions to `0.23.0`, adds `promoted_from: 0.23.0-rc.1`, and merges the rc sections of both changelogs into `## [0.23.0]`. Its body says that merging it releases rc.1's commit, once anfra Cloud runs it.

### anfra Cloud releases with the rc

6. 👤 `make bump-anfra REF=0.23.0-rc.1`, in a pull request, and merge it.
7. 👤 Release anfra Cloud as usual, and roll it out.
8. 👤 Check that the new features work on anfra Cloud.

### anfra releases

9. 👤 Merge the release PR, once anfra Cloud runs the rc it names. Nothing checks this, so the person merging decides.
10. 🤖 `tag_and_release.yml` sees a version without a prerelease part. It checks that `promoted_from` is the newest rc of `0.23.0`. Then it tags the rc's commit `v0.23.0`.
11. 🤖 `build_release.yml` builds `v0.23.0`. It checks that an rc of the same version points at the same commit. It merges the rc sections in its checkout's changelogs, and publishes everything a release publishes today, with `CHANGELOG.md` uploaded beside the binaries.
12. Users get `0.23.0` from `anfra update`, and anfra Cloud supports it already.
13. 🤖 The daily job starts again, from the commits since `v0.23.0`.

### anfra Cloud catches up (optional)

14. 👤 `make bump-anfra REF=0.23.0`, at any time. It's the same commit as the rc, so it changes no code.

### When the rc has a problem

Found at step 8:

1. 👤 Fix it on main.
2. 👤 Run `release_pr.yml` by hand. It cuts `0.23.0-rc.2` from main, on `release/rc`, so the rc includes everything merged since rc.1, and anfra Cloud has to check all of it.
3. 🤖 The rc.2 PR adds `## [0.23.0-rc.2]`, with the commits since rc.1, above rc.1's section. The rc.2 build replaces the release PR with one for rc.2, whose `## [0.23.0]` merges both sections.
4. Continue from step 6, with rc.2.

If the commits since `v0.22.0` now call for a bigger version, such as a `feat` after an rc of `0.22.1`, the next rc is `0.23.0-rc.1`. Its build replaces the `0.22.1` release PR, and `0.22.1` is never released.

## Guards

Each of these fails with a message that says what to do instead.

- **Tagging refuses a release without an rc.** A version without a prerelease part needs `promoted_from`, naming an rc of that version that exists. So every release goes through anfra Cloud first.
- **Tagging refuses a release of an rc that isn't the newest of its version.** The message says to merge the release PR for the newest rc.
- **CI refuses a release PR that differs from what the bot generates** ([The release PR is exactly what the bot generates](#the-release-pr-is-exactly-what-the-bot-generates)).
- **Tagging refuses an rc of a version that's already out,** such as `0.23.0-rc.2` after `v0.23.0`. The rc PR's CI fails for the same reason, so it shows before the merge. The changes go into the next version's rc instead.
- **Tagging runs one at a time,** in the order of the pushes (a `concurrency` group, as anfra Cloud's `tag_and_release.yml` already has). So an rc is always tagged before a later release's tagging runs.

These don't check that anfra Cloud runs the rc before the release PR merges. The person merging it decides (step 9).

## What changes

### anfra

- **`scripts/next-version.mjs`** computes the version from the last release, skipping `v*-rc.*` tags, and prints `X.Y.Z-rc.1`. It prints nothing while an rc is waiting, unless asked for the next rc.
- **`scripts/release-pr.sh`** cuts rcs on `release/rc`. While an rc is waiting, a daily run comments on the release PR instead, and a manual run still cuts the next rc on `release/rc`.
- **`scripts/release-pr.sh`** finds the last release by semver (`git tag --merged <ref> --sort=-v:refname`), not with `git describe`, which may pick the rc tag on the same commit.
- **`scripts/bump.sh`** writes an rc's section, from the newest tag, as it writes a release's today. After a release, its tag and its rc's tag are on the same commit, so the range is the same whichever it picks.
- **A merge script** merges the rc sections of a changelog into the release's section. It reads the rc sections at the rc's tag. The bot runs it for the release PR, and the release's build in its checkout.
- **CI** checks that a release PR is exactly what `scripts/release-pr.sh --dry-run` generates for it.
- **`tag_and_release.yml`** tags HEAD for an rc, and the rc named by `promoted_from` for a release, after the [guards](#guards).
- **`build_release.yml`:**
  - It takes the version from the tag.
  - Its check that the tag matches `manifest.yml` and the SDK changes. An rc's tag has to match its commit's `manifest.yml`. A release's tag has to point at the commit of an rc of the same version.
  - For an rc, it marks the GitHub release as a prerelease. It tags the image with the version only, skips the skills, and then opens the release PR.
  - For a release, it merges the rc sections in its checkout's changelogs, and uploads `CHANGELOG.md` with the binaries.
  - The `sdk` job sets the packages' version from the tag before it chooses the dist-tag, because it chooses by that version. Otherwise a release would read the rc commit's `0.23.0-rc.1`, and publish under `next`. A release also moves `next` to itself (`npm dist-tag add <package>@0.23.0 next`), so that `next` is never older than `latest`.
- **`internal/update/notes.go`** reads `CHANGELOG.md` from the GitHub release, and links to the release page.
- **[release.md](release.md)** describes the new process once it's built.

### anfra Cloud

- **`scripts/bump-anfra.sh` and `scripts/check-anfra-web.sh`** accept `X.Y.Z-rc.N` as a release. Today both read it as a commit, and `bump-anfra.sh` then leaves the npm packages alone.

## Open questions

- **Checking step 9.** For now, the person merging the release PR decides whether anfra Cloud runs the rc. A check could do it later, by reading which anfra version anfra Cloud's latest release requires in its `go.mod`. It would only confirm that anfra Cloud released the rc, not that it rolled it out.
- **Where a later rc comes from.** Cutting it from main is simple, but brings in everything merged since, which anfra Cloud has to check again. Cutting it from a branch at the previous rc, with cherry-picked fixes, keeps each rc small, at the cost of maintaining that branch.
- **CI on the release PR.** The build opens it with `GITHUB_TOKEN`, so CI doesn't start on its own, as with the daily job's PR today. The fix is the same: a GitHub App's token.
