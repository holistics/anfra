# Writing anfra skills

The skills here are what anfra's users' coding agents read, to work with anfra. This file is for
whoever writes them. It isn't published with them.

## A skill is a shortcut

A skill gets an agent to the right command, and tells it what anfra can't: when to run what, what
to ask the user, and what not to do. It isn't a reference.

- **Facts come from anfra, not the skill.** Kinds, codes, flags, fields, and what to do after an
  error: anfra prints them where the agent needs them, in a result's `hints`, an error's message,
  and `--help`. A skill that lists them goes stale when anfra changes, and every agent reads the
  list up front, needed or not. For example, `anfra-publish` doesn't list the kinds of conflict:
  a pull explains each kind it leaves, in `how_to_resolve`.
- **Keep judgment and flows.** The rules an agent should follow, the order of commands, and the
  points where it should ask the user are what a skill is for.
- **When a skill needs a fact anfra doesn't print, make anfra print it.** Add a hint, a field, or
  a better error, rather than writing the fact into the skill.
- **Point to the next layer.** End with where to learn more: a command's `--help`, another skill,
  or [anfra-docs](plugins/anfra/skills/anfra-docs/SKILL.md).

## Prose

Write to [readable prose](../docs/designs/readable-prose.md). Agents follow a skill literally, so
an unclear step is a wrong step.

## Checks

`pnpm skills:check` checks each skill's front matter, and the links between skills. Link to
another skill as `[](../<skill>/)`. [DEVELOPMENT.md](../DEVELOPMENT.md#the-skills) covers the
rest: versions, the build, and commits.
