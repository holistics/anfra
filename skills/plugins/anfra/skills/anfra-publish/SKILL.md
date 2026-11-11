---
name: anfra-publish
description: Publish a repo to its org, and work alongside the others who publish to it — pull their changes, resolve conflicts, and check where this folder stands. Use when the user asks to publish, pull, or sync with their org; when `anfra publish` refuses with `pull_required` or `conflicts_unresolved`; when starting work in a repo that has an org; or to find out whether the user's work is published.
---

# Publishing, and working with others

A repo publishes to its org, the address in `.anfra/config.yml` (`org.url`). A publish replaces
everything the org has, so members who publish to the same org bring in each other's changes
first. anfra makes sure of it: a publish stops when someone published since this folder last
pulled or published, unless their changes are already here.

## Rules

1. **Publish only when the user asks you to.** A publish changes what everyone in the org sees.
   Finishing a task isn't a request to publish, so ask.
2. **Never publish or pull to find something out.** Both change things: publish changes the org,
   and pull changes this folder. To learn where things stand, run `anfra publish status`, which
   changes nothing.
3. **Resolve conflicts by keeping both sides' intent.** When the two sides contradict, ask the
   user which one wins. Never pick a side for them with `--take`.
4. **Don't work around a refusal.** Don't delete files, change the org, or touch
   `.anfra/publish/` to get a publish through. The refusals protect a colleague's work.

## Which workflow

How the team shares its work decides how it publishes. If the folder is a git repo with a remote,
ask the user whether the team shares work through git, unless you already know.

| The team | Shares work in | Publishes by |
|---|---|---|
| Doesn't use git | The org | Pulling, then publishing: "Without git", below |
| Uses git | git's shared branch, often `main` | Publishing what the team merged: "With git", below |
| Wants to publish from CI/CD | git's shared branch | Coming soon: "CI/CD", below |

## Without git: pull, then publish

Before starting a task, see where the folder stands, and pull what others published:

```sh
anfra publish status      # behind? anything unpublished? conflicts left?
anfra pull                # when something is incoming
```

Conflicts are smaller when you're less behind. If the pull brought changes in, tell the user which
files changed, then run `anfra validate` before you build on them.

Once the user asks to publish:

```sh
anfra validate
anfra publish --pull
```

`--pull` brings in what colleagues published since this folder last pulled or published, then
publishes. Their work is merged in, never removed. Tell the user what the result's `pulled` lists.
If the user wants to review colleagues' changes before publishing, run `anfra pull` first, show
them, then `anfra publish`.

When publish stops, its error says what to do next. Do that, then publish again.

## With git: publish what the team merged

git is where the team works together, and the org should publish what the team has merged, not
one member's work in progress. The usual practice, which anfra doesn't enforce:

- Changes reach the team's shared branch, often `main`, the way the team reviews them.
- Publishing happens from an up-to-date checkout of that branch. Colleagues' published work then
  came from it too, so publish goes through without `anfra pull`.

When the user asks to publish from a git repo, check the branch with `git status`. If the folder
isn't on the team's shared branch, or is behind it, tell the user before publishing, and let them
decide. Publishing from another branch replaces the org's files with that branch's. Leave
switching, pulling and merging in git to the user, or do them with the user's consent.

If publish stops with `pull_required`, someone published what isn't in the shared branch.
`anfra pull` brings it into the folder. How it then gets into git is the user's call.

## CI/CD: coming soon

Publishing from CI/CD will be supported soon. The tentative recommended workflow: a CI job
publishes the shared branch after each merge, and git becomes the only way changes reach the org. The missing piece is a service account token, a credential a CI job can
hold. Today anfra signs in only through a browser, as a person.

## Resolving conflicts

A pull that leaves conflicts lists each with its kind, and `how_to_resolve` says how to resolve
each kind. `anfra publish status` lists them again later, such as in a new session.

Markers show the user's version (`local`), the version both started from (`base`), and the org's,
with who published it:

```
<<<<<<< local
  label: 'Revenue (net)'
||||||| base
  label: 'Revenue'
=======
  label: 'Total revenue'
>>>>>>> org (bob@acme.com, 2026-11-08 16:20)
```

The base shows what each side meant to change. Here both renamed the label, so ask the user which
name to keep. When the changes are independent, such as two different fields added, keep both.
Then remove every marker line. A file that can't hold markers, such as an image, needs `--take`:
ask the user which side to keep.

Once every conflict is resolved, run `anfra validate`, then `anfra publish`. If the user would
rather start over, `anfra pull --abort` puts the folder back as it was before the pull.

## Joining an org's repo

- **Without git:** `anfra init`, then `anfra pull --set-org https://<server>/o/<org>`.
  [](../anfra-setup-repo/) says how to find the org's address with the user, and how to sign in.
- **With git:** clone the repo, then run `anfra init` in it, which adds what isn't committed, such
  as `.anfra/data_sources.yml`. The org's address is already in `.anfra/config.yml`, so there's
  nothing to pull.

Then connect the data sources, as [](../anfra-setup-repo/) describes.

## Related skills
* [](../anfra-setup-repo/) — set up a repo, and connect its data source.
* [](../anfra-aml/) — the models and datasets that conflicts are in.
