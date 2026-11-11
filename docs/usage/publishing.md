# Publishing and login

Sign in to an organization, publish a repo, pull what others published to it, and manage the credentials stored on this machine.

## Sign in

```sh
anfra auth login https://bi.example.com/o/acme
anfra publish --set-org https://bi.example.com/o/acme
anfra auth status
anfra auth logout https://bi.example.com/o/acme
```

Login opens a browser with a loopback callback. Consent chooses the org; it may differ
from the address's hint. A server's address alone, with no org, works too. Login saves the returned org's credentials but never changes
the repo's destination. Device/headless login is not supported yet.

Human-readable login also prints the authorization URL to stderr so you can open it
manually on the same machine. If the browser launcher fails, login keeps waiting for
approval until you cancel or the ten-minute login timeout expires. Treat this URL as
private; it contains login state. It is not included in JSON output or diagnostic logs.
With `--json`, a browser-launch failure still exits with an error instead of waiting.

Bare `anfra auth login` offers the build's optional default cloud URL alongside
`Enter your organization URL`. Runtime `ANFRA_CLOUD_URL` overrides that default;
an explicitly empty value disables it. With neither a build default nor a runtime value,
login goes straight to URL entry.
The cloud choice uses the configured origin without an organization hint. In the browser,
consent preselects your last used eligible organization; you can change it before approving.
Entering an organization URL sends its slug as a hint. Login always starts fresh browser
approval; it does not reuse credentials or switch the repo.
Use up/down and Enter, Esc to go back/cancel, and Ctrl-C to cancel anywhere.
Interaction requires terminal stdin and stderr, outside CI and `TERM=dumb`; stdout may be
redirected. Otherwise supply an explicit URL. `ANFRA_CLOUD_URL` must be an HTTPS origin,
or HTTP on literal `127.0.0.1` or `[::1]` for local testing; see
[DEVELOPMENT.md](../../DEVELOPMENT.md#cloud-login-url).

## Stored credentials and logout

Credentials are in `<ANFRA_HOME>/credentials.json`, defaulting to
`~/.anfra/credentials.json`, with mode `0600`. Keep this file private and out of git.
Multiple server/org logins coexist. Status lists cached names and canonical URLs without
network access or tokens, so a login revoked on its server still shows as valid. It marks
the current repo's org (its publish destination), not a global active org, and lists that
org even when no login for it is saved.
From nested directories it reads the nearest `.anfra/config.yml`, stopping at the nearest
Git root (including worktrees), home directory or filesystem root. A nearer config without
`org.url` stops lookup; malformed config is an error. A login is `valid`, `invalid`
(locally expired grants, an uncertain refresh, a damaged entry; access expiry alone isn't),
or `missing`, for the current repo's org with none saved. Cached names can be stale. Logout uses its address, else repo `org.url`, else an
interactive picker. It removes only the selected local login, even when remote
revocation fails. In that case, revoke it on the server's `/account/connected-apps` page.

## Scripted use

All auth commands accept `--yaml` or `--json` for one secret-free result on stdout, and
`--non-interactive` (or `--no-interactive`) to disable prompts and animation. A script passes both:
`--json` alone still asks on a terminal. Explicit
login still needs browser approval in either mode. Each result opens with `message`, the
outcome, and status's and logout's end with `hints`, what to do next. Login reports `org_url`;
it never changes the repo's destination. Status reports `orgs`, each with `name` (absent
without a saved login), `url`, `saved_login` (`valid`, `invalid` or `missing`) and
`current_repo`. Logout reports `org_url`, `local_removed` and `remote_revocation_confirmed`;
a failed revocation after local removal fails with `command_failed`, whose details are that
result. Progress goes to stderr; final results go to stdout. `NO_COLOR` disables colors, not
keyboard navigation. Auth remains machine-local, outside the HTTP/MCP command registry.

## Publish a repo

`anfra publish` validates the repo, then sends its files to its org, replacing what's published
there.

```sh
# ... work ...
anfra publish             # the first time: anfra publish --set-org https://<server>/o/<org>

# Optional, any time:
anfra validate            # optional because `anfra publish` also runs this
anfra publish check       # what it would send, and where to
```

Publish uses `org.url` or `--set-org` and that destination's credentials. With no
destination, an interactive terminal always offers stored logins and new authentication.
After selection and successful credential loading, it saves `org.url` before publishing,
preserving other config. A later publish failure leaves that destination configured.
Without a terminal, provide a destination and matching stored credentials; there is no
automatic selection or browser launch.

`anfra publish check` lists the files a publish would send, and where to, without authentication
or config writes: the preflight, which checks more as it grows. It was `--list`, and before it
`--dry-run`, which still work for now, with a warning. `--yes` is deprecated and does nothing.

## Publishing with others

A publish replaces everything the org has. So when several members publish to one org, each brings
in the others' changes first, and anfra makes sure of it: a publish stops with `pull_required` when
someone published since this folder last pulled or published, unless their changes are already
here. Nothing a colleague published is removed unseen.

How a team shares its work decides how it publishes:

| Your team | Where you share work | How you publish |
|---|---|---|
| Doesn't use git | The org | [`anfra pull`, then `anfra publish`](#without-git-pull-and-publish) |
| Uses git | git's shared branch, such as `main` | [Merge, then publish from the shared branch](#with-git-merge-first-then-publish) |
| Uses git, and wants to publish from CI/CD | git's shared branch | [Coming soon: CI publishes each merge](#with-cicd-coming-soon) |

### Without git: pull and publish

The org is the shared copy. Each member works in their own folder, brings in what others published
with `anfra pull`, and publishes with `anfra publish`.

```sh
# ... work ...
anfra publish --pull      # pulls what others published since, if anything, then publishes

# Optional, any time:
anfra validate            # optional because `anfra publish` also runs this
anfra publish status      # where this folder stands, changing nothing
anfra pull                # bring in what others published, to review it or start from it
```

`anfra publish status` changes nothing. It shows what a pull would bring in (`incoming`), what a
publish would send (`unpublished`), and the last pull's conflicts.

`anfra publish --pull` is a shortcut for `anfra pull`, then `anfra publish`. Without `--pull`,
`anfra publish` stops if the org's version has drifted from this folder's.

When your changes and a colleague's overlap, the pull leaves conflicts for you to resolve, and
says how.

<details>
<summary>Resolving conflicts</summary>

`anfra pull` merges file by file, against the version this folder last pulled or published.
Changes that don't overlap combine. Where both sides changed the same lines, the file gets
conflict markers, showing your version (`local`), the one you both started from (`base`), and the
org's, with who published it and when:

```
<<<<<<< local
  label: 'Revenue (net)'
||||||| base
  label: 'Revenue'
=======
  label: 'Total revenue'
>>>>>>> org (bob@acme.com, 2026-11-08 16:20)
```

Resolve each conflict by editing the file and removing the markers, or keep one side's whole file
with `anfra pull --take local <path>...` or `--take org <path>...`. Files that can't hold markers,
such as images, need `--take`. Then run `anfra validate` and `anfra publish`. Until every conflict
is resolved, `anfra pull` and `anfra publish` refuse with `conflicts_unresolved`, naming each file
and its kind. `anfra pull --abort` undoes the pull instead, putting back every file it changed.

</details>

**Joining.** To work on a repo the org already publishes, start a folder with `anfra init`, then
`anfra pull --set-org https://<server>/o/<org>`. The pull's hints name the data sources the repo
uses that `.anfra/data_sources.yml` doesn't have yet, with their types.

**What pull writes.** Only the files a publish sends. It keeps a copy of each file it changes in
`.anfra/publish/`, a git-ignored folder that also records what this folder last pulled or
published. Two pulls or publishes can't run at once in one folder.

`anfra pull`, `anfra publish`, and `anfra publish status` take `--yaml` or `--json` for their full
result. A pull with conflicts exits 0, since its conflicts are its result.

### With git: merge first, then publish

git is the shared copy, and the org publishes it. anfra doesn't require any practice, but staying
close to the usual one keeps the org in step with git: work on a branch, merge it through your
usual review, then publish from `main`, up to date. Here, `main` stands for your team's shared
branch, whatever its name:

```sh
git switch -c my-change
# ... work, commit, push, and merge my-change to main through your usual review ...
git switch main
git pull
anfra publish
```

That's all a git team needs. `anfra publish` goes through, because what colleagues published came
from `main` too, and is already in your clone.

`anfra publish` and `anfra publish check` warn when the folder is on a branch other than the
remote's default, or has commits not pushed or files not committed. They never refuse for it.

You need `anfra pull` only when `anfra publish` stops with `pull_required`: someone published
something that isn't in `main`, such as from another branch, or without pushing. Bring it into
git, then publish:

```sh
git switch main
git pull
anfra publish           # stops with pull_required
anfra pull
# ... resolve any conflicts, then commit and push the result ...
anfra publish
```

Keep to these, and the org stays a copy of `main`:

- **Don't publish from another branch.** The org then has that branch's files, not `main`'s, and a
  colleague's next publish from `main` stops for a pull.
- **Commit before `anfra pull`,** so `git diff` shows only the pull's changes. If you pulled with
  uncommitted changes, the pull says so, and `anfra pull --abort` undoes it.
- **To join, clone the repo,** then run `anfra init` in it for what isn't committed, such as
  `.anfra/data_sources.yml`. The org's address is committed in `.anfra/config.yml`. The clone's
  first `anfra publish`, or `anfra pull`, sees that it has what's published, and records it. If
  the clone differs from the org, a pull gives the differing files conflict markers, since anfra
  can't tell which side changed them. `git log -- <path>` helps you decide.

### With CI/CD (coming soon)

Publishing from CI/CD will be supported soon. The tentative recommended workflow: a CI job publishes
`main` after each merge, and git becomes the only way changes reach the org. A publish from a
laptop is then refused, with a message naming the branch to merge to instead.

The missing piece is a service account token: a credential a CI job can hold. Today, anfra signs in
only through a browser, as a person. Until then, publish from `main` by hand, as above.

## Refresh and recovery

Refresh happens on demand within five minutes of access expiry, under one cross-process
lock. If a process crashes, a response is lost or a credential save fails, the login may
be marked uncertain. Run `anfra auth login <org address>` again; do not clear pending
state or retry the old refresh token by hand. Revoke the old grant in connected apps.
The former development session-cookie environment hook is no longer used.

