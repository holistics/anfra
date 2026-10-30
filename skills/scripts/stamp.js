// The stamp at the end of each SKILL.md: a rule, then the skills' version, which
// anfra reads to tell whether the skills in use are current
// (anfra's docs/designs/agent-setup.md). The blank line before the rule matters:
// a `---` right under text would make that text a heading.

import { readFileSync, writeFileSync } from 'fs'
import { resolve } from 'path'
import { glob } from 'glob'

const STAMP = /\n*---\r?\nanfra skills \S+\s*$/

// stamped is content with the stamp for version, in place of the one it has.
export function stamped(content, version) {
  return content.replace(STAMP, '').replace(/\s*$/, '') + `\n\n---\nanfra skills ${version}\n`
}

// stampedVersion is the version content's stamp states, or null without one.
export function stampedVersion(content) {
  const m = content.match(/(?:^|\n)---\r?\nanfra skills (\S+)\s*$/)
  return m ? m[1] : null
}

// stampSkills writes the stamp for version into every SKILL.md of the plugin.
export async function stampSkills(pluginDir, version) {
  const files = await glob('skills/**/SKILL.md', { cwd: pluginDir, absolute: true })
  for (const file of files) {
    writeFileSync(file, stamped(readFileSync(file, 'utf8'), version))
  }
  return files.length
}

// pluginVersion is the version in a plugin's Claude manifest, which bump.js
// keeps in lockstep with the others.
export function pluginVersion(pluginDir) {
  return JSON.parse(readFileSync(resolve(pluginDir, '.claude-plugin/plugin.json'), 'utf8')).version
}
