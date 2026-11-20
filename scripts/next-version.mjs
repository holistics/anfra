#!/usr/bin/env node
// Print the version of the next release candidate (rc), such as 0.23.0-rc.1, from the
// conventional commits since the last release; print nothing when none of them is releasable.
// `pnpm next-version` runs it, `pnpm bump next` bumps to it, and scripts/release-pr.sh picks its
// version with it. A release is always made from an rc (docs/designs/release.md), so
// this never prints a release's version.
//
// The release the rc leads to comes from the commits since the last release, not the last rc, so
// a feat merged after an rc of 0.22.1 makes the next rc one of 0.23.0. The commits are read as the
// changelog reads them: conventional-recommended-bump, with conventional-changelog.config.mjs's
// preset (its parser, so the same breaking changes), and its types (a hidden type, left out of the
// changelog, is not releasable either). The rule:
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
// The rc's number is one more than the highest rc of that release tagged yet.
//
// Usage: scripts/next-version.mjs [ref]   (default HEAD)
import { execFileSync } from 'node:child_process'
import { Bumper } from 'conventional-recommended-bump'
import config, { types } from '../conventional-changelog.config.mjs'
import { lastRelease, rcs } from './lib/versions.mjs'

const ref = process.argv[2] ?? 'HEAD'
const git = (...args) => execFileSync('git', args, { encoding: 'utf8' }).trim()

const root = git('rev-parse', '--show-toplevel')
const last = lastRelease(ref)
const [major, minor, patch] = last.slice(1).split('.').map(Number)

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

let release
switch (recommendation.releaseType) {
  case 'major': release = `${major + 1}.0.0`; break
  case 'minor': release = `${major}.${minor + 1}.0`; break
  case 'patch': release = `${major}.${minor}.${patch + 1}`; break
  default: process.exit(0) // nothing releasable
}
const n = Math.max(0, ...rcs(release).map((v) => Number(v.split('-rc.')[1])))
console.log(`${release}-rc.${n + 1}`)
