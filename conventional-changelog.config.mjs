// Ref: https://github.com/conventional-changelog/conventional-changelog/tree/master/packages/conventional-changelog-conventionalcommits

import createPreset from 'conventional-changelog-conventionalcommits'

const config = await createPreset({
  types: [
    { type: 'feat', section: 'Features' },
    { type: 'feature', section: 'Features' },
    { type: 'fix', section: 'Bug Fixes' },
    { type: 'build', section: 'Build' },
    { type: 'perf', section: 'Performance Improvements' },
    { type: 'docs', section: 'Documentation', hidden: true },
    { type: 'style', section: 'Styles', hidden: true },
    { type: 'chore', section: 'Miscellaneous Chores', hidden: true },
    { type: 'refactor', section: 'Code Refactoring', hidden: true },
    { type: 'test', section: 'Tests', hidden: true },
    { type: 'ci', section: 'Continuous Integration', hidden: true },
    // Holistics custom types
    // TODO: make our own preset?
    { type: 'security', section: 'Security' },
    { type: 'release', section: 'New Release', hidden: true },
  ],
})

// A BREAKING CHANGE footer runs to the end of the message, so a Co-Authored-By
// trailer after it would land in the changelog as part of the note: drop those
// trailers from the end of a note before it is written.
const trailers = /(\n\s*Co-Authored-By: [^\n]*)+\s*$/i
const transform = config.writer.transform
config.writer.transform = (commit, context) => {
  const out = transform(commit, context)
  if (out?.notes) {
    out.notes = out.notes.map((note) => ({ ...note, text: note.text.replace(trailers, '').trim() }))
  }
  return out
}

export default config
