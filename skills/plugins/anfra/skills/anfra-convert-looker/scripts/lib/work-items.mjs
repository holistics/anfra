// work-items.json: the TODO / REVIEW comments of the written AML as data, for planning the AI pass.
//
// The main agent of the AI pass uses it to make project-wide decisions per kind, to split the rest into disjoint file
// sets for subagents (models first, then datasets), and to spot near-copy views that can share one fix.
// It is built from the tagged AML text, so it lists exactly the comments that are in the files.

import { TAGGED, kindOf, DEFAULT_REFERENCE } from './kinds.mjs'

const DECL = /^\s*(?:\/\/\s*)?(dimension|measure|param|metric|Model|Dataset)\s+(\w+)\s*\{/
const REF = /see (references\/[\w.-]+\.md(?:#[\w-]+)?)/

/** The AML declaration a comment belongs to: the next `dimension x {` etc. before other code. */
function target (lines, i) {
  for (let j = i + 1; j < Math.min(lines.length, i + 60); j++) {
    const l = lines[j]
    const d = l.match(DECL)
    if (d) return { decl: d[1], name: d[2] }
    if (l.trim() && !l.trim().startsWith('//')) return null
  }
  return null
}

/**
 * @param files Map<relative path, AML text> (already tagged)
 * @param entries the report entries of the project ({ level, where, message })
 * @param origins Map<relative path, { object | explore, lookmlModel, lookmlFile }>: the LookML object of a model or
 *        dataset file (Project.origins)
 */
export function buildWorkItems (files, entries, origins, meta) {
  const items = []
  const fileInfo = {}
  for (const [rel, text] of files) {
    if (!rel.endsWith('.aml')) continue
    const lines = text.split('\n')
    const scope = rel.startsWith('models/') ? 'model' : rel.startsWith('datasets/') ? 'dataset' : 'project'
    const info = { scope, ...origins.get(rel), todos: 0, reviews: 0 }
    const own = lines.map(l => l.match(/^(Model|Dataset)\s+(\w+)\s*\{/)).find(Boolean)
    if (own) info.object = own[2]
    if (scope === 'model') {
      info.fields = lines.map(l => l.match(/^\s*(dimension|measure)\s+(\w+)\s*\{/)).filter(Boolean).map(m => m[2])
    }
    if (scope === 'dataset') {
      const m = text.match(/^\s*models:\s*\[([^\]]*)\]/m)
      info.models = m ? m[1].split(',').map(s => s.trim()).filter(Boolean) : []
    }
    lines.forEach((line, i) => {
      const t = line.match(TAGGED)
      if (!t) return
      const [, level, kind, message] = t
      const tgt = target(lines, i)
      let field = tgt && !['Model', 'Dataset'].includes(tgt.decl) ? tgt.name : null
      const join = kind.startsWith('join-') && message.match(/^join `?(\w+)`?/)
      if (join) field = `join ${join[1]}`
      const filter = message.match(/^filter field `(\w+)`/)
      if (filter) field = filter[1]
      if (!field && scope === 'dataset') {
        // a relationship: the line above the comments names the join (`// join: x — many_to_one`)
        for (let j = i - 1; j >= Math.max(0, i - 4); j--) {
          const m = lines[j].match(/^\s*\/\/ join: (\w+)/)
          if (m) { field = `join ${m[1]}`; break }
        }
      }
      const ref = message.match(REF)
      items.push({
        id: `${rel}:${field || info.object || ''}:${kind}`,
        level,
        kind,
        file: rel,
        line: i + 1,
        scope,
        object: info.object || null,
        field,
        lookmlFile: info.lookmlFile || null,
        reference: ref ? ref[1] : DEFAULT_REFERENCE[kind] || null,
        message
      })
      if (level === 'TODO') info.todos++; else info.reviews++
    })
    fileInfo[rel] = info
  }

  // Which datasets use each model: a dataset agent waits for these models; changing them affects these datasets.
  const modelFile = new Map(Object.entries(fileInfo).filter(([, f]) => f.scope === 'model' && f.object).map(([rel, f]) => [f.object, rel]))
  for (const [rel, f] of Object.entries(fileInfo)) {
    if (f.scope !== 'dataset') continue
    for (const m of f.models) {
      const mf = modelFile.get(m)
      if (mf) (fileInfo[mf].usedBy ||= []).push(rel)
    }
  }

  // Duplicate ids (the same kind twice on one field) get a counter, so that every id is unique.
  const seen = new Map()
  for (const it of items) {
    const n = (seen.get(it.id) || 0) + 1
    seen.set(it.id, n)
    if (n > 1) it.id += `#${n}`
  }

  const kinds = {}
  for (const it of items) {
    const k = (kinds[it.kind] ||= { level: it.level, count: 0, files: new Set(), references: {} })
    k.count++
    k.files.add(it.file)
    if (it.reference) k.references[it.reference] = (k.references[it.reference] || 0) + 1
  }
  for (const k of Object.values(kinds)) {
    k.files = k.files.size
    k.reference = Object.entries(k.references).sort((a, b) => b[1] - a[1])[0]?.[0] || null
    delete k.references
  }

  const families = nearCopies(fileInfo)
  families.forEach((fam, n) => { for (const rel of fam) fileInfo[rel].family = n })
  for (const f of Object.values(fileInfo)) delete f.fields

  // Report entries. Most TODO / REVIEW entries repeat a comment in the AML; some have none (a view without a primary key,
  // an explore declared twice). SECURITY, INFO and DROPPED entries have no comment. A dropped field parameter
  // (`group_item_label`, `alias`) is grouped in droppedParams instead, with the fields it was dropped from.
  const notes = []
  const droppedParams = {}
  for (const e of entries) {
    if (e.level === 'DROPPED' && e.key && !e.lookml) { (droppedParams[e.message] ||= []).push(e.where); continue }
    notes.push({ level: e.level, kind: kindOf(e.level, e.message), where: e.where, message: e.message, ...(e.lookml ? { lookml: e.lookml } : {}) })
  }
  const levels = ['SECURITY', 'TODO', 'REVIEW', 'INFO', 'DROPPED']
  notes.sort((a, b) => levels.indexOf(a.level) - levels.indexOf(b.level))

  return {
    ...meta,
    howToUse: 'items: one per TODO / REVIEW comment (line numbers go stale once files are edited; ids do not). ' +
      'kinds: counts per kind, for project-wide decisions. files: scope, source, the models a dataset uses (models) ' +
      'and the datasets that use a model (usedBy), near-copy family index. families: model files with comments that ' +
      'declare mostly the same fields (copy-pasted views): fix one, then apply the fix to the others. notes: what the ' +
      'script reported per object, by level: SECURITY (access rules with no AML form; a person must replace them), ' +
      'TODO / REVIEW (most repeat a comment; some have none), INFO (to do outside the AML code: dashboards, settings, ' +
      'admin), DROPPED (no AML form, with the original LookML when it may be re-created elsewhere). droppedParams: ' +
      'field parameters without an AML form, with the fields they were dropped from.',
    totals: { todo: items.filter(i => i.level === 'TODO').length, review: items.filter(i => i.level === 'REVIEW').length, files: Object.keys(fileInfo).length },
    kinds: Object.fromEntries(Object.entries(kinds).sort((a, b) => (a[1].level > b[1].level ? -1 : a[1].level < b[1].level ? 1 : b[1].count - a[1].count))),
    families,
    files: fileInfo,
    items,
    notes,
    droppedParams
  }
}

/**
 * Model files with comments that declare mostly the same fields (Jaccard ≥ 0.7 over dimension and measure names, at
 * least 5 shared). Copy-pasted views (`accounts` / `eds_all_accounts`) land in one family, so one fix can be reused.
 */
function nearCopies (fileInfo) {
  const sets = new Map()
  for (const [rel, f] of Object.entries(fileInfo)) {
    if (f.scope === 'model' && (f.todos || f.reviews) && f.fields.length >= 5) sets.set(rel, new Set(f.fields))
  }
  const files = [...sets.keys()].sort()
  const parent = new Map(files.map(f => [f, f]))
  const find = f => (parent.get(f) === f ? f : find(parent.get(f)))
  for (let a = 0; a < files.length; a++) {
    for (let b = a + 1; b < files.length; b++) {
      const A = sets.get(files[a]); const B = sets.get(files[b])
      const inter = [...A].filter(x => B.has(x)).length
      if (inter >= 5 && inter / (A.size + B.size - inter) >= 0.7) parent.set(find(files[b]), find(files[a]))
    }
  }
  const groups = new Map()
  for (const f of files) { const r = find(f); groups.set(r, [...(groups.get(r) || []), f]) }
  return [...groups.values()].filter(g => g.length > 1)
}
