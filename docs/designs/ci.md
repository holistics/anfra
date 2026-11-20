# CI

CI checks each change before it lands on `main`, and checks `main` before a release is tagged. It runs only the checks a change can affect, so a docs change takes seconds, and cutting a release (or a hotfix's release) doesn't wait on checks that already passed.

The workflows are in `.github/workflows/`. `continuous_integration.yml` is the one a pull request runs, and `ci_passed` is its single required status check.

## Areas

The repo is split into **areas**, one for each CI job, and each area lists the files its job reads. A job runs only when a change touches its area. `scripts/ci-areas.sh <base> [head]` says which areas a change touches. Run it to see what CI will run.

| Job | Checks | Its area, roughly |
|---|---|---|
| `lint` | golangci-lint | Go files, the Go modules, `.golangci.yml` |
| `test` | tidy, generated SQL code, build, vet, the Go tests | the Go folders whole (`cmd/`, `internal/`, `engine/`, `shared/`), the Go modules |
| `api` | the API contract is fresh, and a breaking change is declared | Go files, `api/` |
| `web` | the SDK, the design package and the frontend | `web/`, the pnpm workspace, `api/openapi.yaml` |
| `skills` | `pnpm skills:check` | `skills/`, the pnpm workspace |
| `manifest` | the versions agree, the release PR is the bot's, the cloud servers parse | `manifest.yml`, the web packages' `package.json` |
| `dependency-review` | new dependencies' vulnerabilities and licenses | the Go modules, `package.json` files, the workflows |

`commitlint` runs on every pull request. A file in no area, such as `docs/` or a changelog, runs nothing else.

Two rules make a release fast:

- **A `package.json` whose only change is its `version` counts only for `manifest`.** An rc PR or a release PR changes nothing but versions and changelogs, so it runs `commitlint` and `manifest`, about a minute.
- **A change to CI itself touches every area.** That covers the workflow and `scripts/ci-areas.sh`, so a CI change is proven on every job.

An area is the files a job *reads*, not only those it's named for. For example, a Go test reads the testdata beside it, and the SDK's types come from `api/openapi.yaml`. When a job starts reading a file outside its area, add the file to the area in `scripts/ci-areas.sh`. A missing file is how a change passes without the check that would catch it, which is why `main` also gets a full run (below).

## Why skipping is safe: `main` passes

Skipping an unchanged area is only safe if the base already passed that area's checks. A pull request is tested on its own, so two pull requests can each pass, and still break `main` together.

GitHub's merge queue prevents that, but it has proved unreliable here: entries stall until someone re-queues them, and it says nothing about why. At anfra's pace, about two or three merges a day, catching a break minutes after it lands costs less than putting every merge through the queue. So instead:

- **CI runs on every push to `main`**, on the commit as merged.
  - If a pull request's CI already passed on exactly these files (the same git tree), the run reuses that result and passes at once. That's the usual case when the pull request was up to date with `main`.
  - Otherwise, it runs the areas that changed since the last commit on `main` that passed.
- **A release is tagged only on a commit whose push CI passed.** An rc is cut from `main`, so this is the check that the code an rc ships was tested as merged. A release PR tags its rc's commit, which was checked when the rc was cut.

So a hotfix runs its checks once, in its pull request. The push run on `main` and the rc's own checks take a minute or less.

`continuous_integration.yml` still handles the merge queue (`merge_group`), in case it's turned on later.

## Full runs

Every hour, a scheduled workflow checks whether `main` has changed since the last full run, and if so, it starts `Full CI`. That runs every job, whatever changed. It catches what a gap in the areas let through, and blocks nothing. A Slack channel subscribed to `Full CI` reports every run.

It compares git trees, not the areas, since the full run is there to catch the areas' mistakes. It compares with the last full run whether that run passed or failed, so a failure is reported once, not every hour. Start `Full CI` by hand to retry after a fix.

## Plan

1. **Areas on pull requests.** `scripts/ci-areas.sh`, each job gated on its area, the `manifest` job, and `ci_passed` allowing any job but `changes` to skip. Done.
2. **Push CI on `main`.** Reuse a tree that passed, or else check what changed since the last commit that passed.
3. **Tag only a commit that passed.** `tag_and_release.yml` runs when push CI passes on a version change, rather than on the push.
4. **Hourly full runs.** The check that starts `Full CI`, and the Slack channel.
5. **Locally: `make ci`.** Runs the checks for the areas a branch changed, compared with `origin/main`, uncommitted changes included. Each area's commands live in one Makefile target, which both `make ci` and the CI jobs run, so the two can't drift apart.
