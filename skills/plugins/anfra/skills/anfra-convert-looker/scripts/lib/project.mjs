// Project-level conversion: which views and explores exist, constants, data sources, alias models,
// and the conversion report. References: model.md, manifest.md.

import { TODO, REVIEW, asArray, amlString, toLookml, commentLines } from './util.mjs'
import { translateValueFormat } from './format.mjs'
import { ViewConv } from './view.mjs'
import { renderModel, renderAlias } from './model.mjs'
import { convertExplore } from './explore.mjs'

/** Name of the model file the script adds for views and explores that no real model file includes. */
export const UNATTACHED = 'unattached__'

const dirname = p => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '')
/** The path of a LookML model file (`$file_path` lists the model file first, then the files it includes). */
const modelFilePath = lkModel => asArray(lkModel.$file_path)[0]
const stable = o => JSON.stringify(o, (k, v) => (k.startsWith('$') && k !== '$name' ? undefined : v))

export class Project {
  /**
   * @param parsed  result of lookml-parser `parseFiles` (model assembly and extends / refinements applied)
   * @param opts    { dataSources: Map<connection, data source>, defaultDataSource }
   */
  constructor (parsed, opts = {}) {
    this.parsed = parsed
    this.opts = { dataSources: new Map(), ...opts }
    this.entries = []
    this.views = new Map() // view name → { lk, lkModel, lkModelName, conv }
    this.aliases = new Map() // alias model name → ViewConv
    this.datasetNames = new Map()
    this.sources = new Map() // AML file path → paths of the LookML files it was converted from (sources.mjs)
    this.origins = new Map() // AML file path → the LookML object it was converted from, for work-items.json
    this.constants = new Map()
    this.constName = this.constName.bind(this)
    this.models = Object.entries(parsed.model || {}).sort(([a], [b]) => (a === UNATTACHED) - (b === UNATTACHED) || a.localeCompare(b))

    // Where each view is declared (for the output folder).
    this.viewFiles = new Map()
    for (const file of Object.values(parsed.file || {})) {
      for (const name of Object.keys(file.view || {})) if (!name.startsWith('+') && !this.viewFiles.has(name)) this.viewFiles.set(name, file.$file_path)
    }
    // Each view is converted once, in the first model file that includes it.
    for (const [lkModelName, lkModel] of this.models) {
      for (let [name, lk] of Object.entries(lkModel.view || {})) {
        if (name.startsWith('+') || !/^[A-Za-z_]\w*$/.test(name)) continue
        if (Array.isArray(lk)) {
          // Looker allows two views of one name when different model files include different ones; AML has one namespace.
          if (!this.views.has(name)) this.report({ level: 'TODO', where: `view ${name}`, message: `${lk.length} views are named \`${name}\` (${Object.values(this.parsed.file || {}).filter(f => (f.view || {})[name]).map(f => f.$file_path).join(', ')}). Only the first is converted. Decide which one the model files use, or rename the others and rewrite the references — see references/view.md#view` })
          lk = lk[0]
        }
        const seen = this.views.get(name)
        if (!seen) this.views.set(name, { lk, lkModel, lkModelName })
        else if (lkModelName !== UNATTACHED && stable(seen.lk) !== stable(lk)) this.report({ level: 'REVIEW', where: `view ${name}`, message: `the view has a different final form in model file \`${lkModelName}\` than in \`${seen.lkModelName}\` (other refinements, or another view of the same name). Only the form of \`${seen.lkModelName}\` is converted — see references/view.md#view` })
        else if (lkModelName !== UNATTACHED && (seen.lkModel.connection !== lkModel.connection)) this.report({ level: 'REVIEW', where: `view ${name}`, message: `model files \`${seen.lkModelName}\` and \`${lkModelName}\` include the view with different connections; the first is used — see references/model.md#connection` })
      }
    }
    // Text that can read a parameter or a filter field (for readersOf).
    this.readable = []
    for (const [name, { lk }] of this.views) {
      for (const kind of ['dimension', 'dimension_group', 'measure', 'filter', 'parameter']) {
        if (!lk[kind] || typeof lk[kind] !== 'object') continue
        for (const [f, body] of Object.entries(lk[kind] || {})) this.readable.push({ view: name, label: `${name}.${f}`, text: JSON.stringify(body) })
      }
      if (lk.derived_table) this.readable.push({ view: name, label: `${name} (derived_table)`, text: JSON.stringify(lk.derived_table) })
      if (lk.sql_table_name) this.readable.push({ view: name, label: `${name} (sql_table_name)`, text: JSON.stringify(lk.sql_table_name) })
    }
    for (const [, lkModel] of this.models) {
      for (const [e, body] of Object.entries(lkModel.explore || {})) if (!e.startsWith('+')) this.readable.push({ view: null, label: `explore ${e}`, text: JSON.stringify(body) })
    }
    this.setUpConstants()
  }

  report (entry) { this.entries.push(entry) }

  // ---- constants (manifest.md#constant) ------------------------------------------------------------------------------

  setUpConstants () {
    const manifest = this.parsed.manifest || {}
    const taken = new Set(this.views.keys())
    for (const [name, c] of Object.entries(manifest.constant || {})) {
      this.constants.set(name, { value: String(c.value ?? ''), aml: taken.has(name) ? `${name}_const` : name, lk: c })
    }
  }

  constName (name) { return this.constants.has(name) ? this.constants.get(name).aml : name }

  /** The value of a constant that cannot be an AML const (it contains Liquid); undefined for every other constant. */
  inlineConst (name) {
    const value = this.constValue(name)
    return value !== undefined && /\{%|\{\{/.test(value) ? value : undefined
  }

  /** The text of a constant, with nested constants resolved; undefined when unknown. */
  constValue (name, seen = new Set()) {
    const c = this.constants.get(name)
    if (!c || seen.has(name)) return undefined
    seen.add(name)
    return c.value.replace(/@\{(\w+)\}/g, (m, n) => this.constValue(n, seen) ?? m)
  }

  /** `value_format_name: x` naming a `named_value_format` of the model file → the AML constant, or null. */
  namedFormat (lkModel, name) {
    const nf = (lkModel.named_value_format || {})[name]
    if (!nf) return null
    this.formatConsts = this.formatConsts || new Map()
    const src = String(nf.value_format || '').replace(/@\{(\w+)\}/g, (m, c) => this.constValue(c) ?? m)
    const fmt = translateValueFormat(src)
    if (!fmt) return null
    let constName = `fmt_${name}`
    const other = this.formatConsts.get(constName)
    if (other && other !== fmt) constName = `fmt_${lkModel.$name}_${name}`
    this.formatConsts.set(constName, fmt)
    ;(this.formatSources ||= new Set()).add(modelFilePath(lkModel))
    return constName
  }

  // ---- lookups -------------------------------------------------------------------------------------------------------

  /**
   * The SQL that `${EXTENDED}` stands for: the nearest earlier layer of the field that has plain SQL
   * (extends-and-refinements.md#merging-two-layers). The parser merges the layers but does not substitute it.
   */
  extendedSql (view, kind, field, key, seen = new Set()) {
    if (seen.has(view)) return undefined
    seen.add(view)
    if (!this.rawViews) {
      this.rawViews = new Map()
      for (const file of Object.values(this.parsed.file || {})) {
        for (const [name, raw] of Object.entries(file.view || {})) {
          const n = name.replace(/^\+/, '')
          this.rawViews.set(n, [...(this.rawViews.get(n) || []), raw])
        }
      }
    }
    const layers = this.rawViews.get(view) || []
    for (const raw of layers) {
      const sql = ((raw[kind] || {})[field] || {})[key]
      if (typeof sql === 'string' && !sql.includes('${EXTENDED}')) return sql
    }
    const parents = layers.flatMap(raw => [raw.extends || []].flat(2)).reverse()
    for (const parent of parents) {
      const sql = this.extendedSql(parent, kind, field, key, seen)
      if (sql !== undefined) return sql
    }
    return undefined
  }

  /**
   * Paths of the LookML files that declare or refine the `kind` (view, explore) `name`, and of the objects it extends,
   * among the files that `lkModel` includes.
   */
  lookmlFiles (kind, name, lkModel, seen = new Set()) {
    if (seen.has(name)) return []
    seen.add(name)
    const included = new Set(asArray(lkModel.$file_path))
    const out = []
    for (const file of Object.values(this.parsed.file || {})) {
      if (!included.has(file.$file_path)) continue
      for (const key of [name, `+${name}`]) {
        const layers = (file[kind] || {})[key]
        if (layers === undefined) continue
        out.push(file.$file_path)
        for (const layer of asArray(layers)) {
          for (const parent of asArray(layer.extends).flat(2)) out.push(...this.lookmlFiles(kind, parent, lkModel, seen))
        }
      }
    }
    return out
  }

  hasView (name) { return this.views.has(name) }
  viewFile (name) { return this.viewFiles.get(name) }

  viewConv (name) {
    const v = this.views.get(name)
    if (!v) return null
    if (!v.conv) { v.conv = new ViewConv(this, v.lk, { lkName: name, lkModel: v.lkModel }); v.conv.convert() }
    return v.conv
  }

  /** Fields, derived tables and explores whose Liquid reads the parameter / filter field `view.field`. */
  readersOf (view, field) {
    const out = []
    for (const r of this.readable) {
      if (r.label === `${view}.${field}`) continue
      const q = r.view === view ? `(?:${view}\\.)?${field}` : `${view}\\.${field}`
      const re = new RegExp(`\\{%-?\\s*(?:parameter|condition|date_start|date_end)\\s+${q}\\s*-?%\\}|\\b${q}\\._(?:parameter_value|is_filtered|in_query|is_selected)\\b|_filters\\[\\\\?['"]${view}\\.${field}\\\\?['"]\\]|\\$\\{${q}\\}`)
      if (re.test(r.text)) out.push(r.label)
    }
    return out
  }

  // ---- data source (model.md#connection) -------------------------------------------------------------------------------

  dataSource (lkModel) {
    const conn = lkModel && lkModel.connection
    if (conn === undefined) return amlString(this.opts.defaultDataSource || 'TODO_data_source')
    const constant = String(conn).match(/^@\{(\w+)\}$/)
    if (constant) return this.constName(constant[1])
    const mapped = this.opts.dataSources.get(conn) ?? this.opts.defaultDataSource
    if (mapped === undefined) {
      this.unmapped = this.unmapped || new Set()
      if (!this.unmapped.has(conn)) { this.unmapped.add(conn); this.report({ level: 'INFO', where: `connection ${conn}`, message: `no Holistics data source was given for the Looker connection \`${conn}\`; the connection name is used as data_source_name. Re-run with --data-source ${conn}=<name>, or replace it — see references/model.md#connection` }) }
    }
    return amlString(mapped ?? conn)
  }

  dataSourceNotes (lkModel) {
    if (lkModel && lkModel.connection !== undefined) return []
    if (this.opts.defaultDataSource) return []
    return [`${TODO}: data_source_name is a placeholder: no model file with a \`connection\` includes this object. Set the Holistics data source — see references/model.md#connection`]
  }

  // ---- alias models (explore.md#from) ----------------------------------------------------------------------------------

  alias (ref, base, { label, tableName, explore, lkModel, usedBy }) {
    const same = a => a.base === base && a.tableName === tableName
    let name = ref
    const clash = n => (this.views.has(n) && !(this.aliases.has(n))) || (this.aliases.has(n) && !same(this.aliases.get(n)))
    if (clash(name)) name = `${explore}__${ref}`
    if (clash(name)) name = `${explore}__${ref}__${base.lkName}`
    let alias = this.aliases.get(name)
    if (!alias) {
      alias = new ViewConv(this, base.lk, { name, lkName: base.lkName, lkModel: base.lkModel, base })
      alias.label = label
      alias.tableName = tableName
      alias.usedBy = []
      alias.explores = []
      this.aliases.set(name, alias)
      alias.convert()
    } else if (label && alias.label !== label) {
      this.report({ level: 'REVIEW', where: `alias model ${name}`, message: `${usedBy} labels the alias \`${label}\`, but it was first declared with the label \`${alias.label}\`; one model has one label` })
    }
    alias.usedBy.push(usedBy)
    alias.explores.push({ name: explore, lkModel })
    return alias
  }

  datasetName (lkModelName, explore) {
    let name = `${explore}_dataset`
    const owner = this.datasetNames.get(name)
    if (owner && owner !== lkModelName) name = `${lkModelName.replace(/\W/g, '_')}__${explore}_dataset`
    this.datasetNames.set(name, lkModelName)
    return name
  }

  // ---- run -----------------------------------------------------------------------------------------------------------

  /** Convert everything. Returns Map<path, text> of the AML files. */
  run () {
    const files = new Map()
    for (const e of this.parsed.errors || []) {
      // The parser reports key/value lists (`filters: [a: "b"]`, a query's `sorts: [a: desc]`) as a type error; they
      // parse correctly, as an object.
      if (e.code === 'INVALID_PROPERTY_TYPE' && ['filters', 'sorts'].includes(e.property)) continue
      this.report({ level: 'TODO', where: e.filePath || 'project', message: `LookML parse error — the file may be converted partly or not at all: ${e.message}` })
    }
    for (const name of this.views.keys()) this.viewConv(name)

    const datasets = []
    for (const [lkModelName, lkModel] of this.models) {
      this.reportModelFile(lkModelName, lkModel)
      for (let [name, ex] of Object.entries(lkModel.explore || {})) {
        if (name.startsWith('+') || !/^[A-Za-z_]\w*$/.test(name)) continue
        if (Array.isArray(ex)) {
          this.report({ level: 'REVIEW', where: `explore ${name} (${lkModelName})`, message: `the explore is declared ${ex.length} times in this model file; the first declaration is converted` })
          ex = ex[0]
        }
        if (lkModelName === UNATTACHED && this.datasetNames.has(`${name}_dataset`)) continue // already converted in its own model file
        const ds = convertExplore(this, lkModel, lkModelName, name, ex)
        if (!ds) continue
        files.set(ds.path, ds.text)
        this.sources.set(ds.path, [modelFilePath(lkModel), ...this.lookmlFiles('explore', name, lkModel)])
        this.origins.set(ds.path, { explore: name, lookmlModel: lkModelName, lookmlFile: lkModelName === UNATTACHED ? null : modelFilePath(lkModel) })
        datasets.push(ds.name)
      }
    }
    // Models are rendered last: explores add helper dimensions (join keys) to them.
    for (const [name, v] of this.views) {
      const dir = dirname(this.viewFile(name) || '')
      const rel = `models/${dir ? `${dir}/` : ''}${v.conv.name}.model.aml`
      files.set(rel, renderModel(v.conv))
      this.sources.set(rel, this.lookmlFiles('view', name, v.lkModel))
      this.origins.set(rel, { object: name, lookmlFile: this.viewFile(name) || null })
    }
    for (const [name, alias] of this.aliases) {
      const rel = `models/aliases/${name}.model.aml`
      files.set(rel, renderAlias(alias))
      // the view it aliases, and the explores whose joins declare it (label, sql_table_name)
      this.sources.set(rel, [
        ...this.lookmlFiles('view', alias.base.lkName, alias.base.lkModel),
        ...alias.explores.flatMap(e => this.lookmlFiles('explore', e.name, e.lkModel))
      ])
    }
    const manifest = this.renderManifest()
    if (manifest) {
      files.set('manifest.aml', manifest)
      this.sources.set('manifest.aml', [...asArray((this.parsed.manifest || {}).$file_path), ...(this.formatSources || [])])
    }
    return files
  }

  reportModelFile (name, lk) {
    const where = `model file ${name}`
    const r = (level, message, extra) => this.report({ level, where, message, ...extra })
    if (name === UNATTACHED) return
    if (lk.label !== undefined) r('INFO', `label: ${JSON.stringify(lk.label)} has no AML form; the datasets are in the folder datasets/${name}/`)
    if (lk.week_start_day !== undefined) r('INFO', `week_start_day: ${lk.week_start_day} — an admin sets the organization's Week Start Day to match (one value for all model files) — see references/model.md#week_start_day`)
    if (lk.fiscal_month_offset !== undefined) r('INFO', `fiscal_month_offset: ${lk.fiscal_month_offset} is compiled into the fiscal dimensions of this model file's views`)
    if (lk.case_sensitive === false) r('INFO', '`case_sensitive: no`: converted measure filters use lower() / ilike; filters typed in the Holistics UI are not affected')
    for (const key of ['persist_for', 'persist_with']) if (lk[key] !== undefined) r('INFO', `${key}: ${JSON.stringify(lk[key])} — no cache setting in models or datasets; set the cache duration per dashboard — see references/model.md#persist_for-persist_with`)
    for (const [g, body] of Object.entries(lk.datagroup || {})) r('INFO', `datagroup ${g} is not an AML object, and anfra has no schedules: derived tables that name it are not persisted (max_cache_age: ${body.max_cache_age ?? '—'}) — see references/model.md#datagroup`)
    for (const [g, body] of Object.entries(lk.access_grant || {})) r('SECURITY', `access_grant ${g} is unsupported: nothing hides an explore, view or field by user attribute — see references/model.md#access_grant`, { lookml: toLookml('access_grant', g, body) })
    for (const [t, body] of Object.entries(lk.test || {})) r('DROPPED', `test ${t}: AML has no data tests; re-create the assertion elsewhere`, { lookml: toLookml('test', t, body) })
    for (const [m] of Object.entries(lk.map_layer || {})) r('DROPPED', `map_layer ${m}: custom maps are uploaded by an admin in the Holistics UI`)
    for (const [f, body] of Object.entries(lk.named_value_format || {})) if (!this.namedFormat(lk, f)) r('DROPPED', `named_value_format ${f} (${JSON.stringify(body.value_format)}) cannot be translated; fields that use it get no format`)
  }

  renderManifest () {
    const lines = []
    const manifest = this.parsed.manifest || {}
    const where = 'manifest'
    for (const key of ['local_dependency', 'remote_dependency']) {
      if (manifest[key] !== undefined) this.report({ level: 'INFO', where, message: `${key}: AML cannot import a project. The imported files must be converted together with this project — see references/manifest.md#local_dependency-remote_dependency`, lookml: toLookml(key, null, manifest[key]) })
    }
    for (const key of ['localization_settings', 'application', 'visualization']) if (manifest[key] !== undefined) this.report({ level: 'DROPPED', where, message: `${key}: unsupported` })

    // constants, each after the constants it uses
    const pending = [...this.constants.entries()]
    const done = new Set()
    for (let progress = true; pending.length && progress;) {
      progress = false
      for (let i = 0; i < pending.length; i++) {
        const [name, c] = pending[i]
        const deps = [...c.value.matchAll(/@\{(\w+)\}/g)].map(m => m[1]).filter(d => this.constants.has(d))
        if (!deps.every(d => done.has(d))) continue
        pending.splice(i--, 1); done.add(name); progress = true
        if (/\{%|\{\{/.test(c.value)) {
          lines.push(...commentLines([`${TODO}: constant ${name} contains Liquid and is not emitted as a const. Inline it at each place of use and convert the result — see references/manifest.md#constant`, `LookML: constant: ${name} { value: ${JSON.stringify(c.value)} }`]), '')
          this.report({ level: 'TODO', where: `constant ${name}`, message: 'the value contains Liquid; the constant is not emitted' })
          continue
        }
        if (c.lk.export === 'override_required') lines.push(`// ${REVIEW}: export: override_required — the value is probably a placeholder that needs a real value`)
        lines.push(`const ${c.aml} = ${amlString(c.value, this.constName)}`, '')
      }
    }
    for (const [name, fmt] of this.formatConsts || []) lines.push(`const ${name} = '${fmt}'`, '')
    if (!lines.length) return null
    return ['// Constants: LookML `constant` (manifest.lkml) and `named_value_format` (model files).', '', ...lines].join('\n')
  }
}

