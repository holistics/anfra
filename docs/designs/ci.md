# CI

Status: the plan is on the branch. The Slack channel and its subscription are set up by hand.

CI runs only the checks a change can affect, and still makes sure `main` passes before a release is tagged. A docs change takes seconds, an rc or release PR about a minute, and a hotfix runs its checks once, in its pull request.

## Goals

- **Run only what a change can affect.** Most changes touch one or two parts of the repo.
- **Make a release fast.** An rc or release PR changes only versions and changelogs. A hotfix shouldn't wait on a second full round of checks after it merges.
- **Never tag a release on a commit that wasn't checked as merged.** Two pull requests can each pass and still break `main` together.
- **Run the same checks locally, with one command.**

## Design

### Areas

The repo is split into **areas**, one per CI job. An area is the files its job reads. `scripts/ci-areas.sh` holds the lists.

| Job | Checks | Its area, roughly |
|---|---|---|
| `lint` | golangci-lint | Go files, the Go modules, `.golangci.yml` |
| `go` | tidy, generated SQL code, vet, build, the Go tests | the Go folders whole (`cmd/`, `internal/`, `engine/`, `shared/`), the Go modules |
| `api` | the API contract is fresh | Go files, `api/` |
| `web` | the SDK, the design package and the frontend | `web/`, the pnpm workspace, `api/openapi.yaml` |
| `skills` | `pnpm skills:check` | `skills/`, the pnpm workspace |
| `manifest` | the versions agree, the cloud servers parse | `manifest.yml`, the web packages' `package.json`, the release scripts |

Three rules complete the lists:

- **CI's own files are in every area:** the workflow, the `Makefile`, the `ci-*` scripts. Changing how a job runs reruns every job.
- **A file in no area runs every job,** unless it's on a short list of files no job reads (`docs/`, the READMEs, `LICENSE`, dev tooling). A mistake in the lists then costs a full run, not a skipped check.
- **A `package.json` whose only change is its `version` counts only for `manifest`.** So an rc or release PR runs `manifest` alone.

### A job skips when its area's files already passed

Each job's area has a hash: the paths and content hashes of its files, which git already has in the commit's tree. When a job passes, it records its area hash as an artifact, `passed-<area>-<hash>`, kept 30 days. Before it runs, it looks that name up, and skips if the record is there.

So a job runs again only when the files it reads differ from every version that has passed. This is how Turborepo and Nx skip tasks, and how Bazel reports a test `(cached) PASSED`. The lookup is one API call per job, a few seconds in all, and needs only the commit's tree.

Only runs from this repo count. The repo is public, and a fork's pull request runs its own copy of the workflow, so it could upload any artifact name.

### Checks against the base run on pull requests only

Some checks judge a change against the branch it lands on, not its files alone. A record of passed files can't stand in for them, so they run on pull requests, against the PR's base:

- commitlint, always;
- the breaking-change check: when `api/openapi.yaml` changes, any break must be declared by a commit (`feat!:` or `BREAKING CHANGE:`);
- the release PR check, when `manifest.yml`'s version changes;
- the dependency review, when a module, `package.json`, lockfile or workflow changes.

They don't run on `main` or in the merge queue, since they judged the PR's own change. A PR that's behind `main` is judged against the base it had, not the `main` it lands on. That's accepted.

### CI runs on `main` too, and a release waits for it

The same workflow runs on every push to `main`, on the commit as merged:

- **When a PR was up to date with `main`,** every area matches what its PR run recorded, so every job skips. That takes under a minute.
- **When `main` moved after the PR's run,** only the jobs whose areas differ run. That is the check that the two PRs work together.

`tag_and_release.yml` tags an rc only once its commit's run on `main` has passed. A release PR tags its rc's commit, which was checked when the rc was cut.

### Hourly full runs

Every hour, `full_ci.yml` checks whether `main`'s files changed since the last full run. If they did, it starts a full run: CI started by hand (`workflow_dispatch`), which runs every job with no skipping. Its summary lists the commits since the previous full run.

It catches what the area lists miss, such as a job that starts reading a file from another area. It blocks nothing.

It compares with the last full run whether that run passed or failed, so a failure is reported once, not every hour. To retry after a fix, start one by hand: `gh workflow run continuous_integration.yml --ref main`.

A Slack channel reports every full run, through the GitHub app's subscription to CI's dispatched runs:

```
/github subscribe holistics/anfra workflows:{name:"Continuous Integration" event:"workflow_dispatch" branch:"main"}
```

### Locally

Each job runs a make target, so its commands are written once:

- `make ci` runs the areas your branch changed since `origin/main`, uncommitted changes included, and the breaking-change check.
- `make ci-<area>` runs one job.
- `make check` runs every static check, and `make test` every build and test.

## What it costs

| Change | Runs |
|---|---|
| Docs only | commitlint |
| An rc or release PR | commitlint, `manifest` |
| A PR's merge to `main`, PR up to date | nothing (every job skips) |
| A PR's merge to `main`, `main` moved | the jobs whose areas differ |
| Every hour `main` changed | everything, in the background |

The bookkeeping costs a few seconds per run: one lookup per job, and one artifact per passed job.

## What it relies on

**The area lists are complete:** a job reads only files in its area. Nothing enforces this. Bazel does, with a sandbox, but that would mean adopting Bazel. Three things limit the damage:

- broad areas (whole folders);
- a file in no area runs everything;
- the hourly full run catches the rest within the hour.

## Alternatives considered

| Alternative | Why not |
|---|---|
| GitHub's merge queue | It stalls without saying why, and an entry has to be re-queued by hand. At about three merges a day, finding a break minutes after it lands costs less. The workflow still handles `merge_group`, in case. |
| Full checks on every rc | It adds a second round of checks to every hotfix. |
| Comparing with the last commit that passed (`nx-set-shas` style) | It re-runs every job a stale merge touches, even when only other areas moved. It needs the whole history, and base-relative checks end up compared against the wrong base. |
| GitHub's record of runs, instead of artifacts | A PR's run is recorded under the PR branch's tree, not the merged files it tested. They differ whenever the branch is behind `main`, as 40% of merged PRs were. |
| `skip-duplicate-actions`, `nx-set-shas` | They skip whole runs, not jobs. They look back by commit count, and `nx-set-shas` falls back to `HEAD~1` by default, which skips checks. |
| Bazel, or an Nx or Turborepo workspace | They would enforce or infer the area lists, but that's a large migration for one Go module whose full CI takes about 3 minutes. |

## Plan

1. **Areas on pull requests.** Done (`e3ddf06d`).
2. **Each job through `make ci-<area>`, and `make ci`.** Done (`2c9604d7`).
3. **Area hashes.** Each file job records and looks up `passed-<area>-<hash>`; the base checks are PR-only jobs (`api-breaking`, `release`, `dependency-review`, commitlint); CI runs on push to `main`; CI's own files are in every area, and a file in no area runs everything. Done.
4. **Tag only a commit that passed on `main`.** Done: `tag_and_release.yml` is the last job of CI on a push to `main`, after `ci_passed`, when the push changed the version.
5. **Hourly full runs, and the Slack channel.** Done: `full_ci.yml`, and CI's `workflow_dispatch` runs everything. The channel is to set up.
