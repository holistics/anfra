// The stamp at the end of each SKILL.md: a rule, then the skills' version, which
// anfra reads to tell whether the skills in use are current
// (docs/designs/agent-setup.md#the-skills-stamp). The build writes it into the
// facade's copy; the source in skills/ carries none. The blank line before the
// rule matters: a `---` right under text would make that text a heading.

const STAMP = /\n*---\r?\nanfra skills \S+\s*$/

// stamped is content with the stamp for version, in place of any it has.
export function stamped(content, version) {
  return content.replace(STAMP, '').replace(/\s*$/, '') + `\n\n---\nanfra skills ${version}\n`
}

// stampedVersion is the version content's stamp states, or null without one.
export function stampedVersion(content) {
  const m = content.match(/(?:^|\n)---\r?\nanfra skills (\S+)\s*$/)
  return m ? m[1] : null
}
