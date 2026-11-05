#!/usr/bin/env node
// Print the version the next release should take, from the conventional commits since the last
// `v*` tag; print nothing when none of them is releasable. `pnpm next-version` runs it,
// `pnpm bump next` bumps to it, and scripts/release-pr.sh picks its version with it.
//
// The commits are read as the changelog reads them: conventional-recommended-bump, with
// conventional-changelog.config.mjs's preset (its parser, so the same breaking changes), and its
// types (a hidden type, left out of the changelog, is not releasable either). The rule:
//
// - A breaking change (`type!:`, or a `BREAKING CHANGE:` footer) bumps the major version, or the
//   minor while below 1.0.
// - A feat bumps the minor.
// - Any other type the changelog shows (fix, perf, build, security) bumps the patch.
// - Only hidden types (docs, ci, chore, refactor, test, style, release): nothing to release.
//
// The preset's own rule doesn't fit: it bumps the patch for any commit at all, and below 1.0
// (its preMajor) makes a feat a patch.
//
// Usage: scripts/next-version.mjs [ref]   (default HEAD)
import { execFileSync } from 'node:child_process'
import { Bumper } from 'conventional-recommended-bump'
import config, { types } from '../conventional-changelog.config.mjs'

const ref = process.argv[2] ?? 'HEAD'
const git = (...args) => execFileSync('git', args, { encoding: 'utf8' }).trim()

const root = git('rev-parse', '--show-toplevel')
const last = git('describe', '--tags', '--abbrev=0', '--match', 'v*', ref)
const [major, minor, patch] = last.slice(1).split('-')[0].split('.').map(Number)

const shown = new Set(types.filter((t) => !t.hidden).map((t) => t.type))
const breaking = (commit) =>
  commit.notes.length > 0 || config.parser.breakingHeaderPattern.test(commit.header ?? '')

const MAJOR = 0, MINOR = 1, PATCH = 2
const recommendation = await new Bumper(root)
  .config(config)
  .commits({ from: last, to: ref })
  .bump((commits) => {
    let level = null
    for (const commit of commits) {
      if (breaking(commit)) level = MAJOR
      else if (commit.type === 'feat' || commit.type === 'feature') level = Math.min(level ?? MINOR, MINOR)
      else if (shown.has(commit.type)) level ??= PATCH
    }
    if (level === null) return null
    // Below 1.0, a breaking change is a minor bump.
    if (level === MAJOR && major === 0) level = MINOR
    return { level, reason: `${commits.length} commits since ${last}` }
  })

// After a prerelease (1.2.0-rc.1), the release it leads to (1.2.0), whatever the commits.
if (recommendation.releaseType && last.includes('-')) console.log(`${major}.${minor}.${patch}`)
else switch (recommendation.releaseType) {
  case 'major': console.log(`${major + 1}.0.0`); break
  case 'minor': console.log(`${major}.${minor + 1}.0`); break
  case 'patch': console.log(`${major}.${minor}.${patch + 1}`); break
  default: break // nothing releasable
}
