// The LookML files an AML file was converted from, written as comment lines at the top of the AML file, followed by an
// empty line:
//
//   // looker-source: The LookML files this file was converted from. Keep these lines.
//   // looker-source: path=views/base.view.lkml hash=91c4d2e0 commit=6f1e0c2a9b3d
//   // looker-source: path=views/tracks.view.lkml hash=3f2a9c0d commit=6f1e0c2a9b3d
//
// One line per file, sorted by path:
//   path    the file's path relative to the LookML folder given to convert.mjs, with `/` separators. A path with a space
//           or a `"` is written as a JSON string (`path="views/my view.view.lkml"`).
//   hash    the first 8 hex digits of the SHA-256 of the file's bytes.
//   commit  when the LookML folder is in a git repository: the HEAD commit (12 hex digits, more if git needs them to be
//           unique), if the file is the same as in HEAD. It is left out for a file that is untracked, or changed since
//           HEAD (staged or not).
// The first line is a note for whoever edits the file. It has no `path=`.

import crypto from 'node:crypto'
import { execFileSync } from 'node:child_process'

const PREFIX = '// looker-source:'
const NOTE = `${PREFIX} The LookML files this file was converted from. Keep these lines.`

/** The hash written for a file's content (a Buffer or a string). */
export const hashContent = content => crypto.createHash('sha256').update(content).digest('hex').slice(0, 8)

/**
 * The `.lkml` files under `root` that are unchanged since HEAD, mapped to the HEAD commit. The paths are relative to
 * `root`. Empty when `root` is not in a git repository, the repository has no commit, or git is not installed.
 */
export function committedFiles (root) {
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], maxBuffer: 256 * 1024 * 1024 })
  const list = out => out.split('\0').filter(Boolean)
  try {
    const head = git('rev-parse', '--verify', '--quiet', '--short=12', 'HEAD').trim()
    const changed = new Set(list(git('diff', '--name-only', '--relative', '-z', 'HEAD', '--', '*.lkml')))
    return new Map(list(git('ls-files', '-z', '--', '*.lkml')).filter(p => !changed.has(p)).map(p => [p, head]))
  } catch {
    return new Map()
  }
}

const pathValue = p => (/[\s"]/.test(p) ? JSON.stringify(p) : p)

/**
 * The header lines for `paths`, sorted and without duplicates, then an empty line. Paths with no entry in `files`
 * (files the script made up, such as the unattached model file) are left out. Returns [] when no path is left.
 * @param paths string[]
 * @param files Map<path, { hash, commit? }>
 */
export function sourceHeader (paths, files) {
  const known = [...new Set(paths)].filter(p => files.has(p)).sort()
  if (!known.length) return []
  const line = p => {
    const { hash, commit } = files.get(p)
    return `${PREFIX} path=${pathValue(p)} hash=${hash}${commit ? ` commit=${commit}` : ''}`
  }
  return [NOTE, ...known.map(line), '']
}
