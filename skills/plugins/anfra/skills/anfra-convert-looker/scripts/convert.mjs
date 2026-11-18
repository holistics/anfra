#!/usr/bin/env node
// LookML project → AML project: the deterministic part of the conversion.
//
//   node convert.mjs <lookml dir or file> --out <aml dir> [options]
//
// The script writes the structure (models, datasets, constants) by the rules in ../references/.
// Where a rule needs judgement, it writes a `// TODO(lookml-to-aml:<kind>): …` comment with the original LookML next to
// the place the result belongs. An AI pass then reads those comments and finishes the conversion. `work-items.json`
// lists the same comments as data, with everything else the AI pass needs (lib/work-items.mjs); kinds are defined in
// lib/kinds.mjs. Each AML file starts with `// looker-source:` lines that name the LookML files it was converted from,
// with a hash of each, and the git commit when the LookML folder is in a git repository (lib/sources.mjs).

import fs from 'node:fs'
import path from 'node:path'
import { parseFiles } from './lookml-parser/index.js'
import { Project, UNATTACHED } from './lib/project.mjs'
import { TODO, REVIEW } from './lib/util.mjs'
import { tagComments } from './lib/kinds.mjs'
import { buildWorkItems } from './lib/work-items.mjs'
import { hashContent, committedFiles, sourceHeader } from './lib/sources.mjs'

const USAGE = `Usage: node convert.mjs <lookml dir or file> --out <aml dir> [options]

Options:
  --out <dir>                    where the AML project is written (required)
  --data-source <conn>=<name>    Holistics data source for a Looker connection (repeatable)
  --default-data-source <name>   data source for every connection without a --data-source entry
  --force                        write into a folder that is not empty (existing files are overwritten)
  --json                         also write the parsed LookML to <out>/.lookml.json (for debugging)
`

function parseArgs (argv) {
  const opts = { dataSources: new Map() }
  const rest = []
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const value = () => { if (i + 1 >= argv.length) fail(`${a} needs a value`); return argv[++i] }
    if (a === '--out') opts.out = value()
    else if (a === '--data-source') { const v = value(); const at = v.indexOf('='); if (at < 1) fail('--data-source expects <connection>=<data source>'); opts.dataSources.set(v.slice(0, at), v.slice(at + 1)) } else if (a === '--default-data-source') opts.defaultDataSource = value()
    else if (a === '--force') opts.force = true
    else if (a === '--json') opts.json = true
    else if (a === '-h' || a === '--help') { process.stdout.write(USAGE); process.exit(0) } else if (a.startsWith('--')) fail(`unknown option ${a}`)
    else rest.push(a)
  }
  if (rest.length !== 1 || !opts.out) fail('expected one LookML path and --out <dir>')
  opts.source = rest[0]
  return opts
}

function fail (message) {
  process.stderr.write(`${message}\n\n${USAGE}`)
  process.exit(2)
}

/**
 * Read every `.lkml` file under `source`, with the hash of its bytes and its git commit (sources.mjs). LookML
 * dashboards (`.dashboard.lookml`, YAML) are out of scope.
 */
function readLookml (source) {
  const stat = fs.statSync(source)
  const root = stat.isDirectory() ? source : path.dirname(source)
  const input = []
  const commits = committedFiles(root)
  const read = (rel, p) => { const bytes = fs.readFileSync(p); input.push({ path: rel, content: bytes.toString('utf8'), hash: hashContent(bytes), commit: commits.get(rel) }) }
  const walk = dir => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const p = path.join(dir, e.name)
      if (e.isDirectory()) { if (e.name !== 'node_modules' && !e.name.startsWith('.')) walk(p) } else if (e.name.endsWith('.lkml')) read(path.relative(root, p).split(path.sep).join('/'), p)
    }
  }
  if (stat.isDirectory()) walk(root)
  else read(path.basename(source), source)
  return input
}

const opts = parseArgs(process.argv.slice(2))
if (!fs.existsSync(opts.source)) fail(`not found: ${opts.source}`)
const input = readLookml(opts.source)
if (!input.length) fail(`no .lkml file under ${opts.source}`)
const sourceFiles = new Map(input.map(f => [f.path, f]))
if (fs.existsSync(opts.out) && fs.readdirSync(opts.out).length && !opts.force) fail(`${opts.out} is not empty; pass --force to overwrite files in it`)

// Views and explores that no model file includes (a single view file, a project without model files) still get
// converted: an extra model file includes everything. Its objects have no connection.
input.push({ path: `${UNATTACHED}.model.lkml`, content: 'include: "/**/*.lkml"\n' })

const quiet = { log () {}, warn () {}, error () {} }
const parsed = await parseFiles({ input, strings: false, console: quiet })
const project = new Project(parsed, opts)
const unknownKinds = new Set()
const files = new Map([...project.run()].map(([rel, text]) => {
  const head = sourceHeader(project.sources.get(rel) || [], sourceFiles)
  return [rel, [...head, tagComments(text, unknownKinds)].join('\n')]
}))

// ---- write ----------------------------------------------------------------------------------------------------------
// Markers are tagged `TODO(lookml-to-aml:<kind>)`; count the prefix without the closing parenthesis.
const count = (text, marker) => text.split(marker.slice(0, -1)).length - 1
let todoTotal = 0; let reviewTotal = 0
const perFile = []
for (const [rel, text] of files) {
  const target = path.join(opts.out, rel)
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(target, text)
  const todos = count(text, TODO); const reviews = count(text, REVIEW)
  todoTotal += todos; reviewTotal += reviews
  if (todos || reviews) perFile.push({ rel, todos, reviews })
}
if (opts.json) fs.writeFileSync(path.join(opts.out, '.lookml.json'), JSON.stringify(parsed, null, 1))

const work = buildWorkItems(files, project.entries, project.origins, { source: opts.source, out: opts.out })
fs.writeFileSync(path.join(opts.out, 'work-items.json'), JSON.stringify(work, null, 1))

// The script writes no report: work-items.json holds the comments, the report entries (notes) and dropped parameters.
// A conversion-report.md left by an older version of this script would be stale; remove it.
const oldReport = path.join(opts.out, 'conversion-report.md')
if (fs.existsSync(oldReport) && fs.readFileSync(oldReport, 'utf8').startsWith('# LookML → AML conversion report')) fs.rmSync(oldReport)

const noteCount = level => work.notes.filter(n => n.level === level).length
process.stdout.write([
  `Wrote ${files.size} AML files to ${opts.out}`,
  `  models:   ${[...files.keys()].filter(f => f.startsWith('models/')).length}`,
  `  datasets: ${[...files.keys()].filter(f => f.startsWith('datasets/')).length}`,
  `  ${todoTotal} ${TODO} comments, ${reviewTotal} ${REVIEW} comments`,
  `  in ${perFile.length} files, ${Object.keys(work.kinds).length} kinds; notes: ${noteCount('SECURITY')} security, ${noteCount('INFO')} outside AML, ${work.notes.filter(n => n.level === 'DROPPED').length + Object.keys(work.droppedParams).length} dropped`,
  '',
  'Comments by kind (a kind that occurs many times is usually one decision):',
  ...Object.entries(work.kinds).map(([k, v]) => `  ${v.level.padEnd(6)} ${k.padEnd(24)} ${String(v.count).padStart(5)} in ${v.files} file(s)`),
  ...(work.families.length ? ['', `Near-copy model files: ${work.families.length} families (work-items.json: families)`] : []),
  '',
  `Work items: ${path.join(opts.out, 'work-items.json')}`,
  `Next: grep -rn "TODO(lookml-to-aml" ${opts.out}   and resolve each comment by the reference it names.`,
  ''
].join('\n'))
if (unknownKinds.size) {
  process.stderr.write(`warning: ${unknownKinds.size} comment message(s) match no kind in lib/kinds.mjs and are tagged \`other\`:\n${[...unknownKinds].slice(0, 20).map(m => `  ${m}`).join('\n')}\n`)
}
