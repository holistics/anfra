#!/usr/bin/env node
// Merge the release candidates' (rcs') sections at the top of each changelog into one section for
// the release they lead to, in place (docs/designs/release.md). Each rc PR adds its
// rc's section, `## [0.23.0-rc.1]`, which may be edited there. The release PR runs this to replace
// them with `## [0.23.0]`, and so does the release's build, on the rc's commit, so both get the
// same result from the same input.
//
// - Every rc section above the first release's section is merged, an rc of a smaller release
//   included: after 0.22.1-rc.1 and 0.23.0-rc.1, the 0.23.0 section holds both.
// - Lines are kept as written. Each `###` group (Features, Bug Fixes and so on) becomes one, with
//   the rcs' lines under it, newest rc first, and the groups ordered as the changelog's config
//   lists their types, breaking changes first. Any text an rc section has outside a group goes
//   above the groups, newest rc first too.
// - The release's heading links to the comparison with the last release, which is the first
//   release section of the first file, and takes the newest rc's date: the date of its code, the
//   same whenever and wherever this runs.
// - A changelog without rc sections, such as skills/CHANGELOG.md when no rc changed the skills,
//   is left as it is.
//
// Usage: scripts/changelog-merge.mjs <release> <changelog>...   e.g. 0.23.0 CHANGELOG.md skills/CHANGELOG.md
import { readFileSync, writeFileSync } from 'node:fs'
import { types } from '../conventional-changelog.config.mjs'
import { isRc, parse } from './lib/versions.mjs'

const [release, ...files] = process.argv.slice(2)
if (!release || files.length === 0 || !parse(release) || isRc(release)) {
  console.error('usage: scripts/changelog-merge.mjs <release> <changelog>...   e.g. 0.23.0 CHANGELOG.md skills/CHANGELOG.md')
  process.exit(2)
}

// A section's heading, as conventional-changelog writes it:
// "## [0.23.0-rc.1](https://github.com/holistics/anfra/compare/v0.22.0...v0.23.0-rc.1) (2026-11-19)".
const heading = /^## \[([^\]]+)\](?:\(([^)]*)\))?(?: \(([^)]*)\))?/

// sections splits a changelog into its text before the first section, and its sections, each
// with its version, link, date and lines (its heading included).
function sections(text) {
  const lines = text.split('\n')
  const head = []
  const out = []
  for (const line of lines) {
    const m = heading.exec(line)
    if (m) out.push({ version: m[1], link: m[2] ?? '', date: m[3] ?? '', lines: [line] })
    else if (out.length) out.at(-1).lines.push(line)
    else head.push(line)
  }
  return { head, sections: out }
}

const trim = (lines) => {
  let a = 0, b = lines.length
  while (a < b && lines[a].trim() === '') a++
  while (b > a && lines[b - 1].trim() === '') b--
  return lines.slice(a, b)
}

// groups reads a section's text outside its groups, and its groups by name, in order.
function groups(section) {
  const intro = []
  const byName = new Map()
  let current = intro
  for (const line of section.lines.slice(1)) {
    if (line.startsWith('### ')) {
      const name = line.slice(4).trim()
      if (!byName.has(name)) byName.set(name, [])
      current = byName.get(name)
      current.push('') // a blank line between one rc's lines and the next's
    } else current.push(line)
  }
  return { intro: trim(intro), groups: byName }
}

// The groups' order: breaking changes, then the changelog config's sections, then any other, in
// the order they first appear.
const order = ['⚠ BREAKING CHANGES', ...new Set(types.map((t) => t.section))]

function merge(rcSections, previous) {
  const newest = rcSections[0]
  const base = newest.link.split('/compare/')[0]
  const link = base && previous ? `(${base}/compare/v${previous}...v${release})` : ''
  const date = newest.date ? ` (${newest.date})` : ''
  const intros = []
  const merged = new Map()
  for (const s of rcSections) {
    const g = groups(s)
    if (g.intro.length) intros.push(g.intro.join('\n'))
    for (const [name, lines] of g.groups) {
      const body = trim(lines)
      if (!body.length) continue
      if (!merged.has(name)) merged.set(name, [])
      merged.get(name).push(...body)
    }
  }
  const names = [...merged.keys()].sort((a, b) => {
    const i = order.indexOf(a), j = order.indexOf(b)
    return (i < 0 ? order.length : i) - (j < 0 ? order.length : j)
  })
  const out = [`## [${release}]${link}${date}`, '']
  for (const intro of intros) out.push(intro, '')
  for (const name of names) out.push(`### ${name}`, '', ...merged.get(name), '')
  return out
}

let previous = ''
files.forEach((file, i) => {
  const { head, sections: all } = sections(readFileSync(file, 'utf8'))
  const rest = all.findIndex((s) => !isRc(s.version))
  const rcSections = all.slice(0, rest < 0 ? all.length : rest)
  if (i === 0) previous = rest < 0 ? '' : all[rest].version
  if (rcSections.length === 0) return
  if (rcSections.some((s) => parse(s.version) === null)) {
    throw new Error(`${file} has a section that isn't a version's: ${rcSections.map((s) => s.version).join(', ')}`)
  }
  const merged = merge(rcSections, previous)
  // Each section ends with a single newline before the next heading, as conventional-changelog
  // writes them.
  const after = all.slice(rcSections.length).flatMap((s) => s.lines)
  const text = [...head, ...trim(merged), ...after].join('\n')
  writeFileSync(file, after.length ? text : `${text}\n`)
})
