#!/usr/bin/env node
// The versions anfra's tags hold, for the release scripts and workflows: the last release, the
// release candidates (rcs) of a release, and the rc waiting for its release, if any
// (docs/designs/release.md). The scripts import it; shell scripts and workflows run it:
//
//   node scripts/lib/versions.mjs last-release [ref]   the last release merged into ref (HEAD), as 0.22.0
//   node scripts/lib/versions.mjs newest-rc <release>  the highest rc of release, as 0.23.0-rc.2, or nothing
//   node scripts/lib/versions.mjs waiting              the rc waiting for its release, or nothing
//
// It reads the local tags: fetch them first (git fetch --tags). A release is a `vX.Y.Z` tag and an
// rc a `vX.Y.Z-rc.N` tag. Other tags, such as the anfra-v* of releases up to 0.8.0, don't count.
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const versionTag = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/

// parse reads a version, with or without its v, as [major, minor, patch, rc]; a release's rc is
// Infinity, so it sorts after its rcs. null when it isn't a release's or an rc's.
export function parse(version) {
  const m = versionTag.exec(version.startsWith('v') ? version : `v${version}`)
  if (!m) return null
  return [Number(m[1]), Number(m[2]), Number(m[3]), m[4] === undefined ? Infinity : Number(m[4])]
}

export function compare(a, b) {
  const x = parse(a), y = parse(b)
  for (let i = 0; i < 4; i++) if (x[i] !== y[i]) return x[i] < y[i] ? -1 : 1
  return 0
}

export const isRc = (version) => version.includes('-rc.')
// releaseOf is the release an rc leads to: 0.23.0 for 0.23.0-rc.2.
export const releaseOf = (version) => version.replace(/^v/, '').split('-')[0]

// versions lists the versions of the tags, without their v, merged into ref if it's given.
function versions(ref) {
  const args = ['tag', '--list', 'v*']
  if (ref) args.push('--merged', ref)
  return execFileSync('git', args, { encoding: 'utf8' })
    .split('\n')
    .filter((t) => parse(t))
    .map((t) => t.slice(1))
}

const highest = (list) => list.sort(compare).at(-1) ?? ''

// lastRelease is the tag of the highest release merged into ref, such as v0.22.0.
export function lastRelease(ref = 'HEAD') {
  const last = highest(versions(ref).filter((v) => !isRc(v)))
  if (!last) throw new Error(`no release tag (vX.Y.Z) is merged into ${ref}: fetch the tags (git fetch --tags)`)
  return `v${last}`
}

// rcs lists the rcs of release that are tagged, such as 0.23.0-rc.1.
export const rcs = (release) => versions().filter((v) => isRc(v) && releaseOf(v) === release)

export const newestRc = (release) => highest(rcs(release))

// waiting is the rc waiting for its release: the highest tag is an rc, so its release isn't
// tagged. Empty when none is.
export function waiting() {
  const top = highest(versions())
  return isRc(top) ? top : ''
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [command, arg] = process.argv.slice(2)
  const out = {
    'last-release': () => lastRelease(arg).slice(1),
    'newest-rc': () => newestRc(arg ?? ''),
    waiting: () => waiting(),
  }[command]
  if (!out) {
    console.error('usage: scripts/lib/versions.mjs last-release [ref] | newest-rc <release> | waiting')
    process.exit(2)
  }
  const v = out()
  if (v) console.log(v)
}
