#!/usr/bin/env node
// Builds the facade, holistics/anfra-skills, from skills/: what every route
// installs the anfra skills from (docs/designs/agent-setup.md#releasing-the-skills).
//
// Usage: build.mjs --version <version> --source <anfra commit> --out <dir>
//
// The output is a function of skills/ and these parameters alone: nothing is
// read from the clock or the environment. The parameters are recorded in the
// output's .build.json, so the publish job can rebuild what the facade holds
// and tell whether anything changed. Everything in the facade comes from here.

import { cpSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'fs'
import { dirname, join, resolve } from 'path'
import { fileURLToPath } from 'url'
import { parseArgs } from 'util'
import { stamped } from './stamp.mjs'

const SKILLS = resolve(dirname(fileURLToPath(import.meta.url)), '..')

// What the facade holds, from skills/: the marketplaces each agent reads, the
// plugins, the README and the changelog.
const PUBLISHED = ['.claude-plugin', '.cursor-plugin', '.agents', 'plugins', 'README.md', 'CHANGELOG.md']

const { values } = parseArgs({
  options: { version: { type: 'string' }, source: { type: 'string' }, out: { type: 'string' } },
})
const { version, source, out } = values
if (!version || !source || !out) {
  console.error('Usage: build.mjs --version <version> --source <anfra commit> --out <dir>')
  process.exit(1)
}
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`Invalid version: ${version}`)
  process.exit(1)
}

rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })
for (const p of PUBLISHED) {
  cpSync(join(SKILLS, p), join(out, p), { recursive: true })
}

// withVersion is obj with version after the key named after, keeping the rest
// in order, so the output's bytes don't depend on how the source spelled it.
function withVersion(obj, after) {
  const next = {}
  for (const [k, v] of Object.entries(obj)) {
    if (k === 'version') continue
    next[k] = v
    if (k === after) next.version = version
  }
  if (!('version' in next)) next.version = version
  return next
}

function rewriteJSON(path, change) {
  const json = JSON.parse(readFileSync(path, 'utf8'))
  writeFileSync(path, JSON.stringify(change(json), null, 2) + '\n')
}

// The marketplaces' versions, and each plugin's in each agent's format.
for (const m of ['.claude-plugin/marketplace.json', '.cursor-plugin/marketplace.json', '.agents/plugins/marketplace.json']) {
  rewriteJSON(join(out, m), (j) => ({ ...j, metadata: withVersion(j.metadata ?? {}, 'description') }))
}
for (const plugin of readdirSync(join(out, 'plugins'))) {
  for (const format of ['.claude-plugin', '.cursor-plugin', '.codex-plugin']) {
    const manifest = join(out, 'plugins', plugin, format, 'plugin.json')
    try {
      readFileSync(manifest)
    } catch {
      continue
    }
    rewriteJSON(manifest, (j) => withVersion(j, 'name'))
  }
  // Each skill ends with the version, for anfra to compare with the latest.
  const skills = join(out, 'plugins', plugin, 'skills')
  for (const skill of readdirSync(skills)) {
    const file = join(skills, skill, 'SKILL.md')
    writeFileSync(file, stamped(readFileSync(file, 'utf8'), version))
  }
}

// The README says where the facade comes from, and where changes go.
const readme = join(out, 'README.md')
writeFileSync(
  readme,
  `> This repo is published from [holistics/anfra's skills/](https://github.com/holistics/anfra/tree/${source}/skills), ` +
    `at anfra ${version}. Changes go there; this repo is overwritten on each release.\n\n` +
    readFileSync(readme, 'utf8'),
)

writeFileSync(join(out, '.build.json'), JSON.stringify({ version, source }, null, 2) + '\n')
console.log(`Built the anfra skills ${version} into ${out}`)
