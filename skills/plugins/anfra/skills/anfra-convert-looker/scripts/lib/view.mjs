// One LookML view → one AML Model (references/view.md, dimension.md, dimension-group.md, sql-references.md,
// derived-tables.md). Measures are in measure.mjs; the Model block itself is rendered by model.mjs.
//
// A ViewConv holds the converted fields in memory. Explores may add helper dimensions to it later (join keys), so
// model files are rendered only after every explore is converted.

import { TODO, REVIEW, asArray, titleCase, dedent, indent, amlString, amlName, toLookml, filterPairs } from './util.mjs'
import { sqlToAql, Untranslatable, hasTopLevel, stripSqlComments } from './sql-expr.mjs'
import { builtInFormat, decimalsFormat, translateValueFormat } from './format.mjs'
import { convertMeasure } from './measure.mjs'

const TRUNCATIONS = new Set(['raw', 'time', 'date', 'week', 'month', 'quarter', 'year', 'hour', 'minute', 'second',
  'millisecond', 'microsecond'])
const SUB_DAY = /^(time|time_of_day|hour|hour_of_day|hour\d+|minute|minute\d+|second|millisecond\d*|microsecond)$/
const DEFAULT_TIMEFRAMES = ['date', 'day_of_month', 'day_of_week', 'day_of_week_index', 'day_of_year', 'hour',
  'hour_of_day', 'minute', 'month', 'month_name', 'month_num', 'quarter', 'quarter_of_year', 'time', 'time_of_day',
  'week', 'week_of_year', 'year']
const TIMEFRAME = /^(raw|yesno|time|time_of_day|hour|hour_of_day|hour\d+|minute|minute\d+|second|millisecond\d*|microsecond|date|week|day_of_week|day_of_week_index|month|month_num|fiscal_month_num|month_name|day_of_month|quarter|fiscal_quarter|quarter_of_year|fiscal_quarter_of_year|year|fiscal_year|day_of_year|week_of_year)$/
const INTERVALS = ['second', 'minute', 'hour', 'day', 'week', 'month', 'quarter', 'year']
const EARTH_RADIUS = { kilometers: 6371, meters: 6371000, miles: 3958.8, nautical_miles: 3440.1, feet: 20902231, yards: 6967410 }
const SQL_KEYS = ['sql', 'sql_start', 'sql_end', 'sql_distinct_key', 'sql_latitude', 'sql_longitude']
const timeframeTitle = tf => titleCase(tf).replace(/ Of /g, ' of ')

const REF = /\$\{TABLE\}(\.(?:"[^"]+"|`[^`]+`|\[[^\]]+\]|@\{\w+\}|\w+))?|\$\{\s*([A-Za-z_][\w.]*?)(?:::(\w+))?\s*\}|@\{(\w+)\}/g
const CROSS_REF = /\$\{\s*(\w+)\.(\w+)(?:::\w+)?\s*\}/g
const OWN_REF = /\$\{\s*(?:(\w+)\.)?(\w+)(?:::\w+)?\s*\}/g
const hasLiquid = s => /\{%|\{\{/.test(s || '')
const norm = sql => String(sql || '').replace(/\s+/g, ' ').trim()
/** `${name}` and nothing else → name (with an optional view prefix). */
const loneRef = sql => { const m = norm(sql).match(/^\$\{\s*(?:(\w+)\.)?(\w+)(?:::\w+)?\s*\}$/); return m && { view: m[1] || null, field: m[2] } }

// Parameters that are dropped without a trace in the AML. They go to the conversion report only.
const DROPPED = {
  alias: 'not needed', required_fields: 'not needed', string_datatype: 'not needed', precision: 'not needed',
  approximate_threshold: 'not needed', allow_approximate_optimization: 'not needed',
  can_filter: 'unsupported', skip_drill_filter: 'unsupported', case_sensitive: 'unsupported (Holistics text filters are case-insensitive)',
  group_item_label: 'unsupported', allow_fill: 'unsupported (a report setting in Holistics)',
  suggestions: 'unsupported', suggestable: 'unsupported', full_suggestions: 'unsupported',
  bypass_suggest_restrictions: 'unsupported', suggest_persist_for: 'unsupported',
  suggest_dimension: 'unsupported; the fast suggestion source is lost', suggest_explore: 'unsupported; the fast suggestion source is lost',
  tags: 'unsupported', map_layer_name: 'unsupported; choose the map in the visualization',
  fanout_on: 'unsupported (BigQuery Legacy SQL); review the field', label_from_parameter: 'unsupported; the label is static',
  view_label: 'unsupported on a field; the field stays under its own model',
  order_by_field: 'unsupported; reports must add the sort field and sort by it'
}
const DROPPED_WITH_LOOKML = { action: 'unsupported', link: 'unsupported; re-create as a "Go to URL" action per report', html: 'unsupported; use the HTML format or conditional formatting per report' }

export class ViewConv {
  /**
   * @param project   shared state: constants, report, options (see project.mjs)
   * @param lk        the parsed view, after extends and refinements are applied
   * @param opts.name     AML model name (differs from the view name for an alias model)
   * @param opts.lkName   the view name (default: lk.$name)
   * @param opts.lkModel  the parsed LookML model file that includes the view (connection, fiscal offset, …)
   * @param opts.base     for an alias: the ViewConv of the view itself
   */
  constructor (project, lk, { name, lkName, lkModel, base } = {}) {
    this.project = project
    this.lk = lk
    this.lkName = lkName || lk.$name
    this.name = name || this.lkName
    this.lkModel = lkModel || {}
    this.base = base || null
    this.fields = []
    this.byName = new Map()
    this.comments = [] // comment lines at the end of the Model body
    this.lkFields = new Map() // field name (AML spelling) → { kind, lk }
    this.renamed = new Map() // LookML field name → AML name, for names that are not AML identifiers
    this.groups = new Map() // dimension group name → info
    this.emitted = new Map() // field name → AML field names it produced (for Dataset `view` lists)
    this.locations = new Map()
    this.sqlToDim = new Map() // normalized SQL → dimension that holds exactly this SQL
    this.operandHelpers = new Map()
    this.moved = new Map() // field name → Set of other views it needs (the field goes to the Dataset)
    this.unhide = new Set()
    this.pk = null
    this.converted = false
  }

  where (what) { return `view ${this.lkName}${this.name !== this.lkName ? ` (as ${this.name})` : ''}${what ? ` › ${what}` : ''}` }
  /** Add a line to the conversion report. An alias model repeats its view, so it reports nothing. */
  report (level, what, message, extra = {}) { if (!this.base) this.project.report({ level, where: this.where(what), message, ...extra }) }
  /** The AML spelling of a LookML field name of this view. */
  rn (name) { return this.renamed.get(name) || name }

  // -------------------------------------------------------------------------------------------------------------------
  // Index

  index () {
    for (const kind of ['parameter', 'filter', 'dimension', 'dimension_group', 'measure']) {
      for (let [name, lk] of Object.entries(this.lk[kind] || {})) {
        if (Array.isArray(lk)) {
          this.report('REVIEW', `${kind} ${name}`, `the view declares ${kind} \`${name}\` ${lk.length} times; the last declaration is converted`)
          lk = lk[lk.length - 1]
        }
        lk = this.substituteExtended(kind, name, lk)
        const safe = amlName(name)
        if (safe !== name) {
          this.renamed.set(name, safe)
          this.report('REVIEW', `${kind} ${name}`, `\`${name}\` is not a valid AML name (leading digit or AML keyword); the field is named \`${safe}\` and every reference to it is rewritten`)
          lk = { label: titleCase(name), ...lk }
        }
        this.lkFields.set(safe, { kind, lk })
      }
    }
    for (const [name, { kind, lk }] of this.lkFields) {
      if (kind === 'dimension' && lk.primary_key === true && !this.pk) this.pk = name
      if (kind === 'dimension_group') this.groups.set(name, this.groupInfo(name, lk))
    }
    // Dimensions whose AML definition is exactly their SQL: an `@aql` operand over the same SQL can reuse them.
    for (const [name, { kind, lk }] of this.lkFields) {
      if (kind === 'dimension' && ['string', 'number', 'int', 'zipcode', 'date_time', 'date_raw', undefined].includes(lk.type) && !lk.case) {
        const key = norm(lk.sql || `\${TABLE}.${name}`)
        if (!this.sqlToDim.has(key)) this.sqlToDim.set(key, name)
      }
    }
    for (const [name, g] of this.groups) {
      const key = norm(g.lk.sql || `\${TABLE}.${name}`)
      if (g.type === 'time' && g.plainBase && !this.sqlToDim.has(key)) this.sqlToDim.set(key, g.baseName)
    }
    this.findMoved()
    if (this.pk && this.moved.has(this.pk)) {
      this.report('TODO', `dimension ${this.pk}`, 'the primary key reads another view, so it cannot be a model dimension; the model has no primary key. Pick one by hand — see references/dimension.md#primary_key')
      this.comments.push(`${TODO}: the primary key \`${this.pk}\` reads another view and cannot be a model dimension. This model has no primary key (count measures count a hidden row_marker); pick one by hand — see references/dimension.md#primary_key`, '')
      this.pk = null
    }
  }

  /** Replace `${EXTENDED}` by the SQL of the layer below (the parent view, or the refined view). */
  substituteExtended (kind, name, lk) {
    let out = lk
    for (const key of SQL_KEYS) {
      if (typeof lk[key] !== 'string' || !lk[key].includes('${EXTENDED}')) continue
      const parent = this.project.extendedSql(this.lkName, kind, name, key)
      if (parent === undefined) continue
      out = { ...out, [key]: lk[key].replace(/\$\{EXTENDED\}/g, () => parent.trim()), $extended: parent.trim() }
    }
    return out
  }

  groupInfo (name, lk) {
    const type = lk.type || 'time'
    if (type !== 'time') return { name, lk, type, intervals: lk.intervals ? asArray(lk.intervals) : INTERVALS }
    const timeframes = lk.timeframes ? asArray(lk.timeframes) : DEFAULT_TIMEFRAMES
    const subDay = timeframes.some(tf => SUB_DAY.test(tf))
    let baseType = 'datetime'
    let cast = false
    if (lk.datatype === 'date' || lk.datatype === 'yyyymmdd') baseType = 'date'
    else if (lk.convert_tz === false && lk.datatype !== 'epoch' && !subDay) { baseType = 'date'; cast = true }
    const taken = [...this.lkFields].some(([n, f]) => n === name && f.kind !== 'dimension_group')
    return {
      name, lk, type, timeframes, subDay, baseType, cast,
      baseName: taken ? `${name}_${baseType === 'date' ? 'date' : 'time'}` : name,
      plainBase: !cast && lk.datatype !== 'epoch' && lk.datatype !== 'yyyymmdd',
      label: lk.label || titleCase(name)
    }
  }

  /** sql-references.md "Step 1": the fields that use another view, directly or through another field. */
  findMoved () {
    const own = new Map() // field → own names it references
    for (const [name, { kind, lk }] of this.lkFields) {
      const texts = [...SQL_KEYS.map(k => lk[k]), ...asArray(lk.case && lk.case.when).map(w => w.sql)].filter(t => typeof t === 'string')
      const views = new Set()
      const names = new Set()
      for (const text of texts) {
        for (const m of text.matchAll(CROSS_REF)) if (m[1] !== this.lkName && m[2] !== 'SQL_TABLE_NAME' && m[1] !== 'TABLE') views.add(m[1])
        for (const m of text.matchAll(OWN_REF)) if (!m[1] || m[1] === this.lkName) names.add(this.rn(m[2]))
      }
      if (kind === 'measure') {
        const keys = [...filterPairs(lk.filters).map(([key]) => key), ...['based_on', 'based_on_time', 'list_field'].map(k => lk[k]).filter(v => typeof v === 'string')]
        for (const key of keys) {
          const [a, b] = key.split('.')
          if (b && a !== this.lkName) views.add(a); else names.add(this.rn(b || a))
        }
      }
      if (views.size) this.moved.set(name, views)
      own.set(name, names)
    }
    const ownerOf = n => (this.lkFields.has(n) ? n : (this.splitTimeframe(n) || {}).group)
    for (let changed = true; changed;) {
      changed = false
      for (const [name, names] of own) {
        for (const n of names) {
          const target = ownerOf(n)
          if (!target || target === name || !this.moved.has(target)) continue
          const mine = this.moved.get(name) || new Set()
          const before = mine.size
          for (const v of this.moved.get(target)) mine.add(v)
          if (!this.moved.has(name) || mine.size !== before) { this.moved.set(name, mine); changed = true }
        }
      }
    }
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Resolving `${name}`

  /** `created_date` → { group: 'created', timeframe: 'date' }, matching the longest group name. */
  splitTimeframe (name) {
    let best = null
    for (const [from, to] of this.renamed) if (name.startsWith(`${from}_`)) name = to + name.slice(from.length)
    for (const [g, info] of this.groups) {
      if (info.type === 'time' && name.startsWith(`${g}_`) && TIMEFRAME.test(name.slice(g.length + 1)) && (!best || g.length > best.group.length)) {
        best = { group: g, timeframe: name.slice(g.length + 1), info }
      }
      if (info.type === 'duration') {
        const m = name.match(/^(second|minute|hour|day|week|month|quarter|year)s_(.+)$/)
        if (m && m[2] === g && (!best || g.length > best.group.length)) best = { group: g, interval: m[1], info }
      }
    }
    return best
  }

  /**
   * Resolve a field name of this view to the AML field that stands for it.
   * opts.grain: true → a truncation timeframe resolves to the base dimension (filters, durations, field lists)
   *             instead of a hidden alias dimension.
   * Returns { kind: 'dimension' | 'measure' | 'param' | 'filter' | 'location' | 'moved' | 'unknown' | 'error', name, type, note }.
   */
  resolve (name, opts = {}) {
    name = this.rn(name)
    const f = this.lkFields.get(name)
    if (f && this.moved.has(name)) return { kind: 'moved', name, field: f.kind }
    if (f && f.kind === 'dimension') {
      if (f.lk.type === 'location') return { kind: 'location', name }
      return { kind: 'dimension', name, type: this.dimType(f.lk) }
    }
    if (f && f.kind === 'measure') return { kind: 'measure', name, type: 'number' }
    if (f && f.kind === 'parameter') return { kind: 'param', name }
    if (f && f.kind === 'filter') return { kind: 'filter', name }
    if (f && f.kind === 'dimension_group') {
      const g = this.groups.get(name)
      if (g.type === 'time' && g.timeframes.includes('yesno')) return { kind: 'dimension', name: `${name}_yesno`, type: 'truefalse' }
      return { kind: 'error', name, note: `\`\${${name}}\` names a dimension group without a timeframe (an error in Looker); do not guess a timeframe` }
    }
    const tf = this.splitTimeframe(name)
    if (!tf) return { kind: 'unknown', name }
    if (this.moved.has(tf.group)) return { kind: 'moved', name, field: 'dimension_group' }
    const g = tf.info
    if (tf.interval) return { kind: 'dimension', name, type: 'number' }
    const t = tf.timeframe
    if (t === 'raw' || (t === 'time' && g.baseType === 'datetime') || (t === 'date' && g.baseType === 'date')) return { kind: 'dimension', name: g.baseName, type: g.baseType }
    if (TRUNCATIONS.has(t)) {
      if (opts.grain) return { kind: 'dimension', name: g.baseName, type: g.baseType, grain: t }
      const alias = this.ensureAlias(g, t)
      return { kind: 'dimension', name: alias.name, type: alias.props.type, timeframe: true }
    }
    if (/^millisecond\d+$/.test(t)) return { kind: 'dimension', name: g.baseName, type: g.baseType, note: `${name}: millisecondX is unsupported; the base dimension is used` }
    this.ensureExplicit(g, t)
    const explicit = this.byName.get(`${tf.group}_${t}`)
    return { kind: 'dimension', name: `${tf.group}_${t}`, type: explicit ? explicit.props.type : (this.timeframeType(t) || 'text'), timeframe: true }
  }

  /** AML type of a LookML dimension (dimension.md "type"). */
  dimType (lk) {
    if (lk.case) return 'text'
    const t = lk.type || 'string'
    if (['number', 'int', 'distance'].includes(t) || t.startsWith('duration_')) return 'number'
    if (t === 'yesno') return 'truefalse'
    if (t === 'date_time' || t === 'date_raw') return lk.datatype === 'date' ? 'date' : 'datetime'
    if (['date', 'date_date', 'date_week', 'date_month', 'date_quarter'].includes(t)) return 'date'
    if (['date_hour', 'date_minute', 'date_second', 'date_millisecond', 'date_microsecond'].includes(t) || /^date_(hour|minute)\d+$/.test(t)) return 'datetime'
    if (['date_year', 'date_hour_of_day', 'date_day_of_week_index', 'date_day_of_month', 'date_month_num', 'date_week_of_year', 'date_day_of_year', 'date_fiscal_month_num'].includes(t)) return 'number'
    return 'text'
  }

  /** AML type of an explicit timeframe dimension, before it is emitted. */
  timeframeType (tf) {
    if (tf === 'yesno') return 'truefalse'
    if (['hour_of_day', 'day_of_week_index', 'month_num', 'day_of_month', 'day_of_year', 'week_of_year', 'fiscal_month_num'].includes(tf)) return 'number'
    if (/^(hour|minute)\d+$/.test(tf)) return 'datetime'
    return 'text'
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Rewriting SQL references (sql-references.md)

  /**
   * Copy a LookML SQL block into `@sql` text: only the references change.
   * Returns { text, todos, reviews, cross, models } — todos / reviews are comment texts for the field.
   */
  rewriteSql (sql, { query = false } = {}) {
    const todos = []; const reviews = []; const cross = new Set(); const models = []
    // A constant whose value contains Liquid is not an AML const: it is inlined (manifest.md#constant).
    const source = dedent(sql).replace(/@\{(\w+)\}/g, (m, c) => this.project.inlineConst(c) ?? m)
    if (hasLiquid(source)) todos.push('Liquid in the SQL is copied as it is and does not work in Holistics. Convert it — see references/liquid.md (parameter.md for `{% parameter %}`, filter.md for `{% condition %}`)')
    const isLone = !!loneRef(source)
    // \u0001{ marks AML interpolation that this function writes; every other `${` is escaped at the end.
    let text = source.replace(REF, (m, column, name, suffix, constant) => {
      if (m.startsWith('${TABLE}')) {
        if (!column) { todos.push('bare `${TABLE}` has no AML form (`#SOURCE` always needs a column) — see references/sql-references.md#table'); return m }
        const col = column.slice(1)
        const c = col.match(/^@\{(\w+)\}$/)
        if (/^["`[]/.test(col)) reviews.push('quoted column identifier inside `{{ #SOURCE.… }}`: compiles, not run against a database')
        return `{{ #SOURCE.${c ? '\u0001{' + this.project.constName(c[1]) + '}' : col} }}`
      }
      if (constant) return '\u0001{' + this.project.constName(constant) + '}'
      const parts = name.split('.')
      if (parts.length === 2 && parts[1] === 'SQL_TABLE_NAME') {
        if (query) { if (!models.includes(parts[0])) models.push(parts[0]); return `{{ #${parts[0]} }}` }
        todos.push(`\`\${${name}}\` in a field: unsupported, \`{{ #model }}\` exists only in a query model — see references/sql-references.md#viewsql_table_name`)
        return m
      }
      if (name === 'EXTENDED') { todos.push('`${EXTENDED}`: the SQL of the layer below was not found. Substitute it by hand — see references/extends-and-refinements.md#merging-two-layers'); return m }
      if (name === 'SQL_TABLE_NAME') { todos.push('`${SQL_TABLE_NAME}` (the view\'s own table) is not converted — see references/sql-references.md#viewsql_table_name'); return m }
      if (parts.length > 2) { todos.push(`unknown reference \`${m}\``); return m }
      let field = name
      if (parts.length === 2) {
        if (parts[0] !== this.lkName) { cross.add(parts[0]); return m }
        field = parts[1]
      }
      const r = this.resolve(field)
      if (r.note && r.kind !== 'error') reviews.push(r.note)
      if (suffix && r.kind === 'dimension' && !['date', 'datetime'].includes(r.type)) reviews.push(`\`${m}\`: the \`::${suffix}\` suffix is dropped on a field that is not a date or time`)
      if (r.kind === 'dimension' || r.kind === 'measure') {
        if (r.timeframe && !isLone) reviews.push(`\`\${${field}}\` is a dimension-group timeframe inside a larger expression: Looker substitutes a time-zone-converted, usually formatted value; the AML dimension is typed — see references/dimension-group.md#referencing-the-generated-fields`)
        return `{{ ${r.name} }}`
      }
      if (r.kind === 'param' || r.kind === 'filter') { todos.push(`\`\${${field}}\` points at a ${r.kind === 'param' ? 'parameter' : 'filter'} field, which is not a value — see references/${r.kind === 'param' ? 'parameter' : 'filter'}.md`); return m }
      if (r.kind === 'location') { todos.push(`\`\${${field}}\` points at a location dimension, which was replaced by a latitude and a longitude dimension — see references/dimension.md#location`); return m }
      if (r.kind === 'error') { todos.push(r.note); return m }
      if (r.kind === 'moved') { cross.add('?'); return m }
      reviews.push(`\`\${${field}}\` is not a field of this view (broken LookML, or a missing parent view); emitted unchanged`)
      return `{{ ${field} }}`
    })
    // Every `${` that is left is LookML that was not converted: escape it, or AML reads it as interpolation.
    text = text.replace(/\$\{/g, '\\${').replace(/\u0001\{/g, '${')
    return { text, todos, reviews, cross, models }
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Building AML fields

  /** Create a field from the shared parameters of a LookML field (field.md). */
  makeField (kind, name, lk, { type, label, hidden, def }) {
    const inline = v => (typeof v === 'string' ? v.replace(/@\{(\w+)\}/g, (m, c) => this.project.inlineConst(c) ?? m) : v)
    const props = { label: inline(label || lk.label) || titleCase(name), type }
    const comments = []
    let description = inline(lk.description)
    if (lk.synonyms) description = `${description ? `${String(description).trim().replace(/\.?$/, '.')} ` : ''}Also known as: ${asArray(lk.synonyms).join(', ')}.`
    if (description !== undefined) props.description = description
    if (hasLiquid(props.label) || hasLiquid(description)) comments.push(`${TODO}: Liquid in label / description — see references/liquid.md#liquid-in-presentation-parameters`)
    if (hidden || lk.hidden === true || (this.lk.fields_hidden_by_default === true && lk.hidden !== false)) props.hidden = true
    if (lk.$extended) comments.push(`${REVIEW}: \`\${EXTENDED}\` was replaced by the SQL of the layer below: \`${lk.$extended.replace(/\s+/g, ' ')}\` — see references/extends-and-refinements.md#merging-two-layers`)
    const field = { kind, name, props, def, comments, src: lk }
    if (type === 'number') this.applyFormat(field, lk)
    return field
  }

  applyFormat (field, lk) {
    const what = `${field.kind} ${field.name}`
    if (lk.value_format_name) {
      const fmt = builtInFormat(lk.value_format_name)
      const named = fmt ? null : this.project.namedFormat(this.lkModel, lk.value_format_name)
      if (fmt) field.props.format = fmt
      else if (named) field.props.formatConst = named
      else this.report('DROPPED', what, `value_format_name: ${lk.value_format_name} could not be translated — see references/field.md#value_format_name`)
    } else if (lk.value_format || lk.format) {
      const src = String(lk.value_format || lk.format).replace(/@\{(\w+)\}/g, (m, c) => this.project.constValue(c) ?? m)
      const fmt = translateValueFormat(src)
      if (fmt) field.props.format = fmt
      else this.report('DROPPED', what, `value_format ${JSON.stringify(src)} has no Holistics pattern — see references/field.md#value_format`)
    } else if (lk.decimals !== undefined) field.props.format = decimalsFormat(lk.decimals)
  }

  add (field, { after } = {}) {
    if (this.byName.has(field.name)) return this.byName.get(field.name)
    this.byName.set(field.name, field)
    const at = after ? this.fields.findIndex(f => f.name === after) : -1
    if (at >= 0) this.fields.splice(at + 1, 0, field); else this.fields.push(field)
    return field
  }

  /** A hidden helper dimension that holds SQL unchanged. */
  helper (name, label, type, sql, opts = {}) {
    if (this.base) return this.base.helper(name, label, type, sql, opts) // an alias inherits @sql fields
    if (this.byName.has(name)) return this.byName.get(name)
    const r = this.rewriteSql(sql)
    const field = { kind: 'dimension', name, props: { label, type, hidden: true }, def: { lang: 'sql', text: r.text }, comments: [], helper: true, src: { sql } }
    this.attach(field, r, null)
    this.add(field, opts)
    if (this.refsFixed) this.fixSqlOverAql(field) // a helper added after convert(), e.g. by a Dataset
    return field
  }

  /** Names of the @aql fields that the @sql definition of `field` references. */
  aqlTargets (field) {
    if (!field.def || field.def.lang !== 'sql') return []
    const names = new Set([...field.def.text.matchAll(/\{\{ (\w+) \}\}/g)].map(m => m[1]))
    return [...names].filter(n => { const t = this.byName.get(n); return t && t !== field && t.def && t.def.lang === 'aql' })
  }

  /**
   * sql-references.md "A reference to an @aql dimension": `@sql` cannot reference an `@aql` field (E210). Such a field
   * is written in `@aql`; when its SQL is not translated mechanically it becomes a placeholder with a TODO.
   * Returns true when the field changed.
   */
  fixSqlOverAql (field) {
    const targets = this.aqlTargets(field)
    if (!targets.length) return false
    const list = targets.map(n => `\`${n}\``).join(', ')
    const where = 'see references/sql-references.md#a-reference-to-an-aql-dimension'
    const src = field.src && field.src.sql != null ? field.src : null
    let reason = src ? null : 'the definition was built by the script, not copied from one `sql`'
    if (src && hasLiquid(src.sql)) reason = 'Liquid in the SQL'
    if (!reason) {
      try {
        const r = sqlToAql(src.sql, {
          ref: (view, f) => {
            const x = this.modelScope().ref(view, f)
            if (x.kind !== 'dimension') throw new Untranslatable(`\`\${${f}}\` is a ${x.kind}`)
            return x.aql
          },
          column: col => {
            const column = col.replace(/^\$\{TABLE\}\./, '')
            return `${this.name}.${this.sqlToDim.get(norm(col)) || this.helper(`${field.name}__${amlName(column.replace(/\W/g, ''))}`, `${field.props.label} (${column})`, 'text', col).name}`
          }
        })
        field.def = { lang: 'aql', text: src.yesno ? `coalesce(${r.aql}, false)` : r.aql }
        field.comments.push(`${REVIEW}: written in @aql because it references the @aql dimension${targets.length > 1 ? 's' : ''} ${list} (@sql cannot reference an @aql field). The SQL was translated: comparisons and NULL handling follow AQL, not the database — ${where}`)
        return true
      } catch (e) {
        if (!(e instanceof Untranslatable)) throw e
        reason = e.message
      }
    }
    const sqlText = field.def.text
    field.stub = true
    if (field.kind === 'measure') {
      // measure.md "number", step 5: the raw-SQL fallback is not possible either.
      field.def = { lang: 'aql', text: 'null' }
      const note = `The raw-SQL fallback is not possible: the SQL references the @aql field${targets.length > 1 ? 's' : ''} ${list}, and @sql cannot reference an @aql field (E210). The definition below is a placeholder`
      const at = field.comments.findIndex(c => c.includes('Until then it is the raw-SQL fallback'))
      if (at >= 0) field.comments[at] = field.comments[at].replace(/Until then it is the raw-SQL fallback, which is not protected against fan-out/, note)
      else this.todo(field, `${note} — see references/measure.md#number`, null)
      field.comments.push('Definition before the placeholder:', ...indent(`@sql ${sqlText};;`, 2).split('\n'))
      return true
    }
    field.def = { lang: 'sql', text: 'NULL' }
    const message = `this field references the @aql dimension${targets.length > 1 ? 's' : ''} ${list}, and @sql cannot reference an @aql field (E210). Write it in @aql (not translated: ${reason}) — ${where}. The definition below is a placeholder.`
    this.todo(field, message, src && src.lookml)
    if (!src || !src.lookml) field.comments.push('Definition before the placeholder:', ...indent(`@sql ${sqlText};;`, 2).split('\n'))
    this.report('TODO', `dimension ${field.name}`, message)
    return true
  }

  /** Put the todos / reviews of a rewrite on a field; `lookml` is quoted under a TODO. */
  attach (field, r, lookml) {
    for (const t of r.todos) field.comments.push(`${TODO}: ${t}`)
    for (const t of new Set(r.reviews)) field.comments.push(`${REVIEW}: ${t}`)
    if (r.todos.length && lookml) field.comments.push('LookML:', ...indent(lookml, 2).split('\n'))
    if (r.todos.length) field.todo = true
  }

  todo (field, message, lookml) {
    field.comments.push(`${TODO}: ${message}`)
    if (lookml && !field.comments.includes('LookML:')) field.comments.push('LookML:', ...indent(lookml, 2).split('\n'))
    field.todo = true
    return field
  }

  /** A field that could not be converted: it keeps its name, label and type; the definition is a placeholder. */
  stub (kind, name, lk, type, message) {
    const field = this.makeField(kind, name, lk, { type, def: kind === 'dimension' ? { lang: 'sql', text: 'NULL' } : { lang: 'aql', text: 'null' } })
    field.stub = true
    return this.todo(field, `${message} The definition below is a placeholder.`, toLookml(lk.$type || kind, name, lk))
  }

  /** Report the parameters of a field that have no AML form. */
  reportDropped (name, kind, lk) {
    for (const key of Object.keys(lk)) {
      if (DROPPED[key]) this.report('DROPPED', `${kind} ${name}`, `${key}: ${DROPPED[key]}`, { key })
      if (DROPPED_WITH_LOOKML[key]) this.report('DROPPED', `${kind} ${name}`, `${key}: ${DROPPED_WITH_LOOKML[key]}`, { key, lookml: toLookml(kind, name, { [key]: lk[key] }) })
      if (key === 'required_access_grants') this.report('SECURITY', `${kind} ${name}`, `required_access_grants [${asArray(lk[key]).join(', ')}] is dropped: the field is visible to everyone who can open the Dataset — see references/field.md#required_access_grants`)
    }
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Convert

  convert () {
    if (this.converted) return this
    this.converted = true
    this.index()
    const emit = (name, fields) => this.emitted.set(name, fields.filter(Boolean).map(f => f.name))
    for (const [name, { kind, lk }] of this.lkFields) {
      if (this.moved.has(name)) continue
      this.reportDropped(name, kind, lk)
      if (kind === 'parameter') emit(name, [this.convertParameter(name, lk)])
      else if (kind === 'filter') this.convertFilter(name, lk)
      else if (kind === 'dimension') emit(name, this.convertDimension(name, lk))
      else if (kind === 'dimension_group') emit(name, this.convertGroup(name, lk))
      else if (kind === 'measure') emit(name, [convertMeasure(this, name, lk, this.modelScope())])
    }
    // @sql cannot reference @aql (E210). Fixing one field makes it @aql, so repeat until nothing changes.
    for (let changed = true; changed;) {
      changed = false
      for (const f of [...this.fields]) if (this.fixSqlOverAql(f)) changed = true
    }
    this.refsFixed = true
    for (const n of this.unhide) if (this.byName.has(n)) delete this.byName.get(n).props.hidden
    if (!this.pk) this.report('REVIEW', null, 'the view declares no primary key; `type: count` and some relationships need one — a person should pick it')
    return this
  }

  /** How a measure of this model reads fields: everything is `<model>.<field>`; another view is out of reach. */
  modelScope () {
    return {
      self: this.name,
      inDataset: false,
      ref: (view, name, opts) => {
        if (view && view !== this.lkName) throw new Untranslatable(`\`\${${view}.${name}}\` is a field of another view`)
        const r = this.resolve(name, opts)
        if (r.kind === 'dimension' || r.kind === 'measure') return { ...r, aql: `${this.name}.${r.name}` }
        throw new Untranslatable(r.kind === 'error' ? r.note : `\`\${${name}}\` is ${r.kind === 'unknown' ? 'not a field of this view' : `a ${r.kind} field`}`)
      }
    }
  }

  // ---- parameter, filter ------------------------------------------------------------------------------------------

  convertParameter (name, lk) {
    const readers = this.project.readersOf(this.lkName, name)
    if (!readers.length) {
      this.report('DROPPED', `parameter ${name}`, 'nothing reads this parameter; it is not converted — see references/parameter.md#parameter')
      return null
    }
    const type = { number: 'number', yesno: 'truefalse', date: 'date', date_time: 'date' }[lk.type] || 'text'
    const field = this.makeField('param', name, lk, { type })
    delete field.props.format
    const allowed = asArray(lk.allowed_value)
    const notes = []
    if (allowed.some(a => a.label !== undefined && a.label !== a.value)) notes.push(`Values: ${allowed.map(a => (a.label !== undefined && a.label !== a.value ? `${a.value} (${a.label})` : a.value)).join(', ')}`)
    if (lk.default_value !== undefined) {
      notes.push(`Looker default: ${lk.default_value}`)
      this.report('INFO', `parameter ${name}`, `default_value ${JSON.stringify(lk.default_value)} must become the default of the dashboard filter that drives this param — see references/parameter.md#default_value`)
    }
    if (notes.length) field.props.description = [field.props.description, ...notes].filter(Boolean).map(t => String(t).trim().replace(/\.$/, '')).join('. ')
    if (allowed.length) field.props.allowed_values = allowed.map(a => (type === 'number' ? String(a.value) : amlString(a.value)))
    field.comments.push(`read by: ${readers.join(', ')} (each reader carries its own TODO)`)
    if (readers.some(r => !r.startsWith(`${this.lkName}.`) && !r.startsWith(`${this.lkName} `))) {
      field.comments.push(`${TODO}: this param is read from outside its view. It may have to move to a small param model that is added to the Dataset — see references/parameter.md#parameter (rule 4)`)
      this.report('TODO', `parameter ${name}`, 'read from outside its view: it may have to move to a param model')
    }
    return this.add(field)
  }

  convertFilter (name, lk) {
    const readers = this.project.readersOf(this.lkName, name)
    this.comments.push(`${TODO}: filter field \`${name}\` is not emitted. Decide by its readers (${readers.length ? readers.join(', ') : 'none found'}): a \`param\`, a truefalse dimension, or nothing — see references/filter.md#filter`,
      'LookML:', ...indent(toLookml('filter', name, lk), 2).split('\n'), '')
    this.report('TODO', `filter ${name}`, `filter field not emitted; readers: ${readers.join(', ') || 'none'} — see references/filter.md#filter`)
  }

  // ---- dimension --------------------------------------------------------------------------------------------------

  convertDimension (name, lk) {
    const what = toLookml('dimension', name, lk)
    const type = lk.type || 'string'
    const out = []
    const push = f => { out.push(this.add(f)); return f }
    const sqlField = (amlType, sql, wrap = t => t) => {
      const r = this.rewriteSql(sql ?? `\${TABLE}.${name}`)
      const f = this.makeField('dimension', name, lk, { type: amlType, def: { lang: 'sql', text: wrap(r.text) } })
      f.src = { sql: sql ?? `\${TABLE}.${name}`, yesno: amlType === 'truefalse', lookml: what }
      this.attach(f, r, what)
      return f
    }
    const finish = () => {
      const main = out.find(f => f.name === name)
      if (main && lk.primary_key === true) main.props.primary_key = true
      for (const f of out) if (f.todo) this.report('TODO', `dimension ${f.name}`, f.comments.find(c => c.startsWith(TODO)).slice(TODO.length + 2))
      return out
    }

    if (lk.case) { this.convertCase(name, lk, push, what); return finish() }
    if (['tier', 'bin'].includes(type)) { this.convertTier(name, lk, push); return finish() }
    if (type === 'location') { this.convertLocation(name, lk, push); return finish() }
    if (type === 'distance') { this.convertDistance(name, lk, push); return finish() }
    if (type.startsWith('duration_')) { this.convertDurationDim(name, lk, push); return finish() }
    if (type.startsWith('date')) { this.convertDateDim(name, lk, push, sqlField); return finish() }

    if (['string', 'number', 'int', 'zipcode'].includes(type)) {
      push(sqlField(type === 'string' || type === 'zipcode' ? 'text' : 'number', lk.sql))
      if (type === 'zipcode') this.report('DROPPED', `dimension ${name}`, 'type: zipcode → text; the map layer is lost')
    } else if (type === 'yesno') {
      const sql = lk.sql ?? `\${TABLE}.${name}`
      const bare = stripSqlComments(sql).trim()
      const lone = loneRef(sql)
      const target = lone && (!lone.view || lone.view === this.lkName) ? this.resolve(lone.field, { grain: true }) : null
      let f
      if (target && target.kind === 'dimension' && ['date', 'datetime'].includes(target.type)) {
        f = this.makeField('dimension', name, lk, { type: 'truefalse', def: { lang: 'sql', text: `{{ ${target.name} }} IS NOT NULL` } })
        f.src = { sql: `\${${target.name}} IS NOT NULL`, lookml: what }
      } else if ((/\bIS\s+(NOT\s+)?NULL$/i.test(bare) || /^(NOT\s+)?EXISTS\s*\([\s\S]*\)$/i.test(bare)) && !hasTopLevel(bare, ['AND', 'OR'])) {
        f = sqlField('truefalse', sql)
      } else {
        f = sqlField('truefalse', sql, t => (t.includes('\n') ? `CASE WHEN (\n${indent(t, 2)}\n) THEN TRUE ELSE FALSE END` : `CASE WHEN (${t}) THEN TRUE ELSE FALSE END`))
        if (/^\$\{TABLE\}\.\w+$/.test(norm(sql))) f.comments.push(`${REVIEW}: a yesno over a plain column. If the column is a date or time, Looker means "has a value": use \`<column> IS NOT NULL\` — see references/dimension.md#yesno`)
      }
      push(f)
    } else if (['time', 'duration', 'custom_calendar'].includes(type)) {
      push(this.stub('dimension', name, lk, 'text', `\`type: ${type}\` is a dimension_group type; on a dimension it is a LookML error — see references/dimension.md#custom_calendar-duration-time.`))
    } else {
      push(this.stub('dimension', name, lk, 'text', `unknown dimension type \`${type}\`.`))
    }
    return finish()
  }

  convertCase (name, lk, push, what) {
    const whens = []; const seen = new Set(); const agg = { todos: [], reviews: [] }
    for (const w of asArray(lk.case.when)) {
      if (seen.has(w.label)) { agg.reviews.push(`duplicate case label ${JSON.stringify(w.label)}: only the first is kept — see references/dimension.md#case`); continue }
      seen.add(w.label)
      const r = this.rewriteSql(w.sql || '')
      agg.todos.push(...r.todos); agg.reviews.push(...r.reviews)
      whens.push({ cond: r.text.replace(/\s*\n\s*/g, ' '), label: String(w.label ?? '') })
    }
    const sqlLit = v => `'${String(v).replace(/'/g, "''")}'`
    const body = value => ['CASE', ...whens.map((w, i) => `  WHEN ${w.cond} THEN ${value(w, i)}`)]
    const main = body(w => sqlLit(w.label))
    if (lk.case.else !== undefined) main.push(`  ELSE ${sqlLit(lk.case.else)}`)
    const f = this.makeField('dimension', name, lk, { type: 'text', def: { lang: 'sql', text: [...main, 'END'].join('\n') } })
    this.attach(f, agg, what)
    push(f)
    if (lk.alpha_sort !== true) {
      const sort = body((w, i) => i)
      if (lk.case.else !== undefined) sort.push(`  ELSE ${whens.length}`)
      push({ kind: 'dimension', name: `${name}__sort`, props: { label: `${f.props.label} (sort order)`, type: 'number', hidden: true }, def: { lang: 'sql', text: [...sort, 'END'].join('\n') }, comments: [], helper: true })
    }
  }

  /** `sql: ${dim}` → `<model>.<dim>`; any other SQL → a hidden helper dimension. Returns the AQL operand. */
  aqlOperand (sql, helperName, helperLabel, helperType, opts = {}) {
    // AQL type-checks its functions, so a dimension of another type is wrapped in a helper of the wanted type.
    const fits = type => (helperType === 'number' ? type === 'number' : ['date', 'datetime'].includes(type))
    const lone = sql && loneRef(sql)
    if (lone && (!lone.view || lone.view === this.lkName)) {
      const r = this.resolve(lone.field, opts)
      if (r.kind === 'dimension' && fits(r.type)) return `${this.name}.${r.name}`
    }
    const key = norm(sql)
    if (this.sqlToDim.has(key) && fits(this.resolve(this.sqlToDim.get(key)).type)) return `${this.name}.${this.sqlToDim.get(key)}`
    const h = this.helper(helperName, helperLabel, helperType, sql)
    return `${this.name}.${h.name}`
  }

  convertTier (name, lk, push) {
    const tiers = asArray(lk.tiers || lk.bins).map(String)
    if (!tiers.length || tiers.some(t => isNaN(Number(t))) || hasLiquid(lk.sql)) { push(this.stub('dimension', name, lk, 'text', 'tier without numeric `tiers`, or with Liquid — see references/dimension.md#tier.')); return }
    const label = lk.label || titleCase(name)
    const x = this.aqlOperand(lk.sql ?? `\${TABLE}.${name}`, `${name}__value`, `${label} (value)`, 'number')
    const style = lk.style || 'classic'
    const n = tiers.length
    const pad2 = i => String(i).padStart(2, '0')
    const minus1 = t => String(Number(t) - 1)
    const labels = {
      classic: [`T00 (-inf,${tiers[0]})`, ...tiers.slice(0, -1).map((t, i) => `T${pad2(i + 1)} [${t},${tiers[i + 1]})`), `T${pad2(n)} [${tiers[n - 1]},inf)`, 'TXX Undefined'],
      interval: [`(-inf,${tiers[0]})`, ...tiers.slice(0, -1).map((t, i) => `[${t},${tiers[i + 1]})`), `[${tiers[n - 1]},inf)`, 'Undefined'],
      integer: [`Below ${tiers[0]}`, ...tiers.slice(0, -1).map((t, i) => `${t} to ${minus1(tiers[i + 1])}`), `${tiers[n - 1]} or Above`, 'Undefined'],
      relational: [`< ${tiers[0]}`, ...tiers.slice(0, -1).map((t, i) => `>= ${t} and < ${tiers[i + 1]}`), `>= ${tiers[n - 1]}`, 'Undefined']
    }[style] || null
    if (!labels) { push(this.stub('dimension', name, lk, 'text', `unknown tier style \`${style}\`.`)); return }
    const conds = [`${x} < ${tiers[0]}`, ...tiers.slice(0, -1).map((t, i) => `${x} >= ${t} and ${x} < ${tiers[i + 1]}`), `${x} >= ${tiers[n - 1]}`]
    const caseOf = (values, other) => ['case(', ...conds.map((c, i) => `  when: ${c}, then: ${values[i]}${i < conds.length - 1 || other !== undefined ? ',' : ''}`), ...(other !== undefined ? [`  else: ${other}`] : []), ')'].join('\n')
    const q = v => `'${v.replace(/'/g, "\\'")}'`
    const f = this.makeField('dimension', name, lk, { type: 'text', def: { lang: 'aql', text: caseOf(labels.slice(0, -1).map(q), q(labels[labels.length - 1])) } })
    f.comments.push(`${REVIEW}: tier label text is not fully documented by Looker (labels below the first tier, "Undefined", decimals); compare with real output if saved filters match on labels — see references/dimension.md#tier`)
    if (lk.value_format || lk.value_format_name) f.comments.push(`${TODO}: Looker formats the numbers inside tier labels with the value format; write the format into the label strings by hand — see references/dimension.md#tier`)
    push(f)
    if (style !== 'classic') push({ kind: 'dimension', name: `${name}__sort`, props: { label: `${label} (sort order)`, type: 'number', hidden: true }, def: { lang: 'aql', text: caseOf(conds.map((c, i) => i)) }, comments: [], helper: true })
  }

  convertLocation (name, lk, push) {
    const label = lk.label || titleCase(name)
    const part = (suffix, sql) => {
      const lone = sql && loneRef(sql)
      const r = lone && (!lone.view || lone.view === this.lkName) ? this.resolve(lone.field) : null
      if (r && r.kind === 'dimension') { if (lk.hidden !== true) this.unhide.add(r.name); return r.name }
      const rw = this.rewriteSql(sql ?? `\${TABLE}.${name}_${suffix}`)
      const f = this.makeField('dimension', `${name}_${suffix}`, { ...lk, label: `${label} ${titleCase(suffix)}` }, { type: 'number', def: { lang: 'sql', text: rw.text } })
      this.attach(f, rw, toLookml('dimension', name, lk))
      push(f)
      return f.name
    }
    this.locations.set(name, { lat: part('latitude', lk.sql_latitude), lng: part('longitude', lk.sql_longitude) })
    this.report('INFO', `dimension ${name}`, `type: location has no AML type; replaced by the dimensions ${this.locations.get(name).lat} and ${this.locations.get(name).lng}. Location filters and references to \${${name}} need manual work — see references/dimension.md#location`)
  }

  convertDistance (name, lk, push) {
    const own = v => this.rn(String(v || '').replace(`${this.lkName}.`, ''))
    const start = this.locations.get(own(lk.start_location_field))
    const end = this.locations.get(own(lk.end_location_field))
    const radius = EARTH_RADIUS[lk.units || 'miles']
    if (!start || !end || !radius) {
      push(this.stub('dimension', name, lk, 'number', 'distance between locations that are not both location dimensions of this view, declared before it (another view → a Dataset dimension) — see references/dimension.md#distance.'))
      return
    }
    const m = this.name
    const f = this.makeField('dimension', name, lk, {
      type: 'number',
      def: {
        lang: 'aql',
        text: [`2 * ${radius} * asin(sqrt(`,
          `  pow(sin(radians(${m}.${end.lat} - ${m}.${start.lat}) / 2), 2)`,
          `  + cos(radians(${m}.${start.lat})) * cos(radians(${m}.${end.lat}))`,
          `    * pow(sin(radians(${m}.${end.lng} - ${m}.${start.lng}) / 2), 2)`,
          '))'].join('\n')
      }
    })
    f.comments.push(`${REVIEW}: haversine formula; Looker does not document its own — see references/dimension.md#distance`)
    push(f)
  }

  /** `sql_start` / `sql_end` → an AQL operand (dimension-group.md#sql_start-sql_end). */
  durationOperand (sql, helperName, helperLabel, datatype) {
    const bare = norm(stripSqlComments(sql || '')).toUpperCase().replace(/\s*\(\s*\)$/, '')
    if (['CURRENT_TIMESTAMP', 'NOW', 'GETDATE', 'SYSDATE'].includes(bare)) return '@now'
    if (bare === 'CURRENT_DATE') return '@today'
    return this.aqlOperand(sql, helperName, helperLabel, datatype === 'date' ? 'date' : 'datetime', { grain: true })
  }

  durationAql (unit, start, end) {
    if (unit === 'second') return `epoch(${end}) - epoch(${start})`
    if (unit === 'minute') return `floor((epoch(${end}) - epoch(${start})) / 60)`
    if (unit === 'hour') return `floor((epoch(${end}) - epoch(${start})) / 3600)`
    return `date_diff('${unit}', ${start}, ${end})`
  }

  convertDurationDim (name, lk, push) {
    const unit = lk.type.slice('duration_'.length)
    if (!INTERVALS.includes(unit) || !lk.sql_start || !lk.sql_end || hasLiquid(lk.sql_start + lk.sql_end)) {
      push(this.stub('dimension', name, lk, 'number', `\`type: ${lk.type}\` needs \`sql_start\` and \`sql_end\` without Liquid — see references/dimension.md#individual-duration-types.`))
      return
    }
    const label = lk.label || titleCase(name)
    const start = this.durationOperand(lk.sql_start, `${name}__start`, `${label} (start)`, lk.datatype)
    const end = this.durationOperand(lk.sql_end, `${name}__end`, `${label} (end)`, lk.datatype)
    const f = this.makeField('dimension', name, lk, { type: 'number', def: { lang: 'aql', text: this.durationAql(unit, start, end) } })
    f.comments.push(`${REVIEW}: rounding of durations near a boundary is not specified on either side — see references/dimension.md#individual-duration-types`)
    push(f)
  }

  /** A `datetime` dimension over `sql`, shared by the `date_*` dimensions over the same SQL. */
  dateBase (name, lk) {
    const sql = lk.sql ?? `\${TABLE}.${name}`
    const key = norm(sql)
    if (this.sqlToDim.has(key)) {
      const dim = this.sqlToDim.get(key)
      const src = this.lkFields.get(dim)
      if (dim !== name && (!src || ['date_time', 'date_raw'].includes(src.lk.type) || src.kind === 'dimension_group')) return dim
    }
    const col = key.match(/^\$\{TABLE\}\.(\w+)$/)
    const baseName = `${col ? col[1] : name}__base`
    this.helper(baseName, `${titleCase(col ? col[1] : name)} (base)`, 'datetime', sql)
    return baseName
  }

  convertDateDim (name, lk, push, sqlField) {
    const t = lk.type
    if (t === 'date_time') { push(sqlField(lk.datatype === 'date' ? 'date' : 'datetime', lk.sql)); return }
    if (t === 'date_raw') { const f = sqlField(lk.datatype === 'date' ? 'date' : 'datetime', lk.sql); f.props.hidden = true; push(f); return }
    if (hasLiquid(lk.sql)) { push(this.stub('dimension', name, lk, this.dimType(lk), 'Liquid in the SQL of a date dimension — see references/liquid.md.')); return }
    if (t === 'date' || t === 'date_date') {
      if (lk.datatype === 'date') { push(sqlField('date', lk.sql)); return }
      if (lk.convert_tz === false) { push(sqlField('date', lk.sql, s => `CAST(${s} AS DATE)`)); return }
      const b = `${this.name}.${this.dateBase(name, lk)}`
      const f = this.makeField('dimension', name, lk, { type: 'date', def: { lang: 'aql', text: `cast(${b}, 'date')` } })
      f.comments.push(`${REVIEW}: \`type: date\` over a column whose type LookML does not state. If the column is a DATE, use \`type: 'date'\` with the column directly; whether this cast runs after Holistics' time zone conversion is not documented — see references/dimension.md#date-date_time-date_raw`)
      push(f)
      return
    }
    const tf = t.slice('date_'.length)
    const baseName = this.dateBase(name, lk)
    const b = `${this.name}.${baseName}`
    const trunc = { hour: ['datetime', `date_trunc(${b}, 'hour')`], minute: ['datetime', `date_trunc(${b}, 'minute')`], week: ['date', `cast(date_trunc(${b}, 'week'), 'date')`], month: ['date', `cast(date_trunc(${b}, 'month'), 'date')`], quarter: ['date', `cast(date_trunc(${b}, 'quarter'), 'date')`], year: ['number', `date_part('year', ${b})`], second: ['datetime', b], millisecond: ['datetime', b], microsecond: ['datetime', b] }[tf]
    const def = trunc ? { type: trunc[0], aql: trunc[1] } : this.timeframeDef(tf, b, { name, baseName, baseType: 'datetime', label: lk.label || titleCase(name) })
    if (!def) { push(this.stub('dimension', name, lk, 'text', `\`type: ${t}\` has no AQL form — see references/dimension.md#other-date_-types.`)); return }
    const f = this.makeField('dimension', name, lk, { type: def.type, def: { lang: 'aql', text: def.aql } })
    if (def.review) f.comments.push(`${REVIEW}: ${def.review}`)
    push(f)
  }

  // ---- dimension_group --------------------------------------------------------------------------------------------

  /** The explicit dimension of a timeframe that is not a truncation (dimension-group.md#timeframes). */
  timeframeDef (tf, b, g) {
    const part = (p, x = b) => `date_part('${p}', ${x})`
    const text = x => `cast(${x}, 'text')`
    let m
    switch (tf) {
      case 'yesno': return { type: 'truefalse', aql: `${b} is not null` }
      case 'time_of_day': return { type: 'text', aql: `date_format(${b}, '%H:%M')`, review: 'date_format patterns are not supported by every database' }
      case 'hour_of_day': return { type: 'number', aql: part('hour') }
      case 'day_of_week': return { type: 'text', aql: `date_format(${b}, '%A')`, review: 'text sorts alphabetically (Looker sorts by calendar order); date_format patterns are not supported by every database' }
      case 'day_of_week_index': return { type: 'number', aql: part('dow'), review: 'day-of-week numbering follows the organization\'s Week Start Day; not verified against Looker\'s 0 = first day of week' }
      case 'month_num': return { type: 'number', aql: part('month') }
      case 'month_name': return { type: 'text', aql: `date_format(${b}, '%B')`, review: 'text sorts alphabetically (Looker sorts by calendar order); names depend on the database locale' }
      case 'day_of_month': return { type: 'number', aql: part('day') }
      case 'quarter_of_year': return { type: 'text', aql: `concat('Q', ${text(part('quarter'))})` }
      case 'day_of_year': return { type: 'number', aql: `date_diff('day', ${b} | year(), ${b}) + 1`, review: 'assumes date_diff(\'day\', …) counts calendar days' }
      case 'week_of_year': return { type: 'number', aql: part('week') }
      case 'fiscal_year': case 'fiscal_quarter': case 'fiscal_quarter_of_year': case 'fiscal_month_num': {
        const f = this.fiscalBase(g, b)
        const review = 'fiscal dimensions are text / number, not a date grain; check the exact text Looker shows (FY2017-Q3 or 2017-Q3)'
        if (tf === 'fiscal_year') return { type: 'text', aql: `concat('FY', ${text(part('year', f))})`, review }
        if (tf === 'fiscal_quarter') return { type: 'text', aql: `concat('FY', ${text(part('year', f))}, '-Q', ${text(part('quarter', f))})`, review }
        if (tf === 'fiscal_quarter_of_year') return { type: 'text', aql: `concat('Q', ${text(part('quarter', f))})`, review }
        return { type: 'number', aql: part('month', f), review }
      }
    }
    if ((m = tf.match(/^hour(\d+)$/)) && 24 % Number(m[1]) === 0) {
      const x = Number(m[1]); const lines = []
      for (let h = 24 - x; h > 0; h -= x) lines.push(`  when: ${part('hour')} >= ${h}, then: (${b} | day()) + interval(${h} hour),`)
      return { type: 'datetime', aql: ['case(', ...lines, `  else: ${b} | day()`, ')'].join('\n'), review: 'hourX bucket — see references/dimension-group.md#hourx' }
    }
    if ((m = tf.match(/^minute(\d+)$/))) {
      const s = Number(m[1]) * 60
      return { type: 'datetime', aql: `from_unixtime(floor(epoch(${b}) / ${s}) * ${s})`, review: 'the minuteX bucket is computed in UTC; it equals Looker\'s only when the reporting time zone offset is a multiple of X minutes' }
    }
    return null
  }

  fiscalBase (g, b) {
    const n = Number(this.lkModel.fiscal_month_offset || 0)
    if (!n) return b
    const name = `${g.name}__fiscal`
    this.add({ kind: 'dimension', name, props: { label: `${g.label} (fiscal-shifted)`, type: g.baseType, hidden: true }, def: { lang: 'aql', text: `${b} ${n > 0 ? '-' : '+'} interval(${Math.abs(n)} month)` }, comments: [`fiscal_month_offset: ${n} (from the model file)`], helper: true }, { after: g.baseName })
    return `${this.name}.${name}`
  }

  /** The hidden alias dimension that reproduces the value Looker substitutes for `${g_<timeframe>}`. */
  ensureAlias (g, tf) {
    const name = `${g.name}_${tf}`
    if (this.byName.has(name)) return this.byName.get(name)
    const b = `${this.name}.${g.baseName}`
    const defs = {
      date: ['date', `cast(${b}, 'date')`],
      time: ['datetime', `cast(${b}, 'datetime')`],
      week: ['date', g.baseType === 'date' ? `${b} | week()` : `cast(${b} | week(), 'date')`],
      month: ['text', `date_format(${b}, '%Y-%m')`],
      quarter: ['text', `date_format(${b}, '%Y-Q%q')`],
      year: ['number', `date_part('year', ${b})`],
      hour: ['text', `date_format(${b}, '%Y-%m-%d %H')`],
      minute: ['text', `date_format(${b}, '%Y-%m-%d %H:%M')`],
      second: ['text', `date_format(${b}, '%Y-%m-%d %H:%M:%S')`],
      millisecond: ['text', `date_format(${b}, '%Y-%m-%d %H:%M:%S')`],
      microsecond: ['text', `date_format(${b}, '%Y-%m-%d %H:%M:%S')`]
    }
    const [type, aql] = defs[tf]
    const field = { kind: 'dimension', name, props: { label: `${g.label} ${timeframeTitle(tf)}`, type, hidden: true }, def: { lang: 'aql', text: aql }, comments: [`alias for \${${name}}: other SQL references this timeframe by name (references/dimension-group.md#referencing-the-generated-fields)`], helper: true }
    if (this.base) this.base.ensureAlias(this.base.groups.get(g.name), tf)
    return this.add(field, { after: g.baseName })
  }

  ensureExplicit (g, tf) {
    const name = `${g.name}_${tf}`
    if (this.byName.has(name) || g.timeframes.includes(tf)) return
    if (this.base) this.base.ensureExplicit(this.base.groups.get(g.name), tf)
    const def = this.timeframeDef(tf, `${this.name}.${g.baseName}`, g)
    if (def) this.add({ kind: 'dimension', name, props: { label: `${g.label} ${timeframeTitle(tf)}`, type: def.type, hidden: true }, def: { lang: 'aql', text: def.aql }, comments: [`${REVIEW}: \${${name}} is referenced, but the dimension group does not list the timeframe \`${tf}\``], helper: true }, { after: g.baseName })
  }

  convertGroup (name, lk) {
    const g = this.groups.get(name)
    const what = toLookml('dimension_group', name, lk)
    const out = []
    const done = () => { for (const f of out) if (f.todo) this.report('TODO', `dimension_group ${name} › ${f.name}`, f.comments.find(c => c.startsWith(TODO)).slice(TODO.length + 2)); return out }
    if (g.type === 'duration') {
      if (!lk.sql_start || !lk.sql_end || hasLiquid(lk.sql_start + lk.sql_end)) {
        out.push(this.add(this.stub('dimension', `days_${name}`, lk, 'number', 'duration group without `sql_start` / `sql_end`, or with Liquid — see references/dimension-group.md#duration-type-dimension-groups.')))
        return done()
      }
      const label = lk.label || titleCase(name)
      const start = this.durationOperand(lk.sql_start, `${name}__start`, `${label} (start)`, lk.datatype)
      const end = this.durationOperand(lk.sql_end, `${name}__end`, `${label} (end)`, lk.datatype)
      if (['epoch', 'yyyymmdd'].includes(lk.datatype)) this.comments.push(`${TODO}: dimension_group ${name} has \`datatype: ${lk.datatype}\`: convert its __start / __end helper dimensions — see references/dimension-group.md#datatype`, '')
      g.intervals.forEach((unit, i) => {
        if (!INTERVALS.includes(unit)) return
        const f = this.makeField('dimension', `${unit}s_${name}`, { ...lk, label: `${titleCase(unit)}s ${label}` }, { type: 'number', def: { lang: 'aql', text: this.durationAql(unit, start, end) } })
        if (i === 0) f.comments.push(`${REVIEW}: duration dimensions of group \`${name}\`: Looker floors and counts complete periods; AQL date_diff rounding is not documented — see references/dimension-group.md#intervals`)
        out.push(this.add(f))
      })
      return done()
    }
    if (g.type !== 'time') {
      out.push(this.add(this.stub('dimension', name, lk, 'datetime', `\`type: ${g.type}\` dimension groups are unsupported; closest workaround in references/dimension-group.md#custom-calendar-type-dimension-groups.`)))
      return done()
    }

    // base dimension
    const sql = lk.sql ?? `\${TABLE}.${name}`
    let base
    if (lk.datatype === 'epoch') {
      const h = this.helper(`${name}__epoch`, `${g.label} (epoch seconds)`, 'number', sql)
      base = this.makeField('dimension', g.baseName, lk, { type: 'datetime', label: g.label, def: { lang: 'aql', text: `from_unixtime(${this.name}.${h.name})` } })
    } else {
      const r = this.rewriteSql(sql)
      base = this.makeField('dimension', g.baseName, lk, { type: g.baseType, label: g.label, def: { lang: 'sql', text: g.cast ? `CAST(${r.text} AS DATE)` : r.text } })
      this.attach(base, r, what)
      if (lk.datatype === 'yyyymmdd') this.todo(base, 'datatype: yyyymmdd — wrap the SQL so that it parses the integer into a date, in the database\'s dialect (PostgreSQL: TO_DATE(CAST(x AS VARCHAR), \'YYYYMMDD\')) — see references/dimension-group.md#datatype', what)
    }
    if (!g.timeframes.some(tf => TRUNCATIONS.has(tf))) base.props.hidden = true
    if (lk.convert_tz === false && g.baseType === 'datetime') this.todo(base, '`convert_tz: no` with sub-day timeframes is unsupported: Holistics converts every datetime to the organization time zone. A person must choose a workaround — see references/dimension-group.md#convert_tz', what)
    if (g.baseType === 'date' && g.subDay) base.comments.push(`${REVIEW}: a date base with sub-day timeframes (${g.timeframes.filter(tf => SUB_DAY.test(tf)).join(', ')}): they have no meaning on a date`)
    if (g.baseName !== name) base.comments.push(`${REVIEW}: the view already has a field named \`${name}\`, so the base dimension of the group is named \`${g.baseName}\``)
    if (lk.primary_key === true) {
      const only = g.timeframes.length === 1 ? g.timeframes[0] : null
      if (only === 'raw' || (only === 'time' && g.baseType === 'datetime') || (only === 'date' && g.baseType === 'date')) base.props.primary_key = true
      else base.comments.push(`${TODO}: \`primary_key: yes\` on a dimension group whose single timeframe is not the base value — see references/dimension-group.md#things-to-consider`)
    }
    if (lk.drill_fields) this.report('DROPPED', `dimension_group ${name}`, 'drill_fields on a time group is dropped: Holistics date drill offers every grain')
    out.push(this.add(base))

    // explicit dimensions
    for (const tf of g.timeframes) {
      if (TRUNCATIONS.has(tf)) continue
      const def = this.timeframeDef(tf, `${this.name}.${g.baseName}`, g)
      if (!def) {
        this.comments.push(`${TODO}: dimension_group ${name}: timeframe \`${tf}\` is unsupported in AQL and is not emitted; references point at the base dimension — see references/dimension-group.md#timeframes`, '')
        this.report('TODO', `dimension_group ${name}`, `timeframe ${tf} is not emitted`)
        continue
      }
      const f = this.makeField('dimension', `${name}_${tf}`, { ...lk, label: `${g.label} ${timeframeTitle(tf)}` }, { type: def.type, def: { lang: 'aql', text: def.aql } })
      if (def.review) f.comments.push(`${REVIEW}: ${def.review}`)
      out.push(this.add(f))
    }
    return done()
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Fields that move to a Dataset (sql-references.md#viewfield)

  /**
   * Convert a moved dimension in the scope of one Dataset.
   * Returns a field ({ kind: 'dimension', props.model, … }), with `skeleton: true` when the SQL around the
   * references could not be translated to AQL: a skeleton is printed as a comment.
   */
  convertMovedDimension (name, scope) {
    const { kind, lk } = this.lkFields.get(name)
    const what = toLookml(kind, name, lk)
    const type = kind === 'dimension' ? this.dimType(lk) : 'datetime'
    const field = this.makeField('dimension', name, lk, { type })
    field.props = { model: scope.self, ...field.props }
    const simple = kind === 'dimension' && !lk.case && ['string', 'number', 'int', 'zipcode', 'yesno', 'date_time', 'date_raw', undefined].includes(lk.type) && lk.sql && !hasLiquid(lk.sql)
    if (simple) {
      try {
        const r = sqlToAql(lk.sql, {
          ref: (view, f) => scope.ref(view, f).aql,
          column: col => {
            const column = col.replace(/^\$\{TABLE\}\./, '')
            return `${scope.self}.${this.sqlToDim.get(norm(col)) || this.helper(`${name}__${column.replace(/\W/g, '')}`, `${field.props.label} (${column})`, 'text', col).name}`
          }
        })
        field.def = { lang: 'aql', text: lk.type === 'yesno' ? `coalesce(${r.aql}, false)` : r.aql }
        field.comments.push(`${REVIEW}: moved from view \`${this.lkName}\` because it reads another view. Check that \`model: ${scope.self}\` reaches every other model through many-to-one relationships (else E104 fan-out) — see references/sql-references.md#step-6-choose-model-for-a-dataset-dimension`)
        return field
      } catch (e) {
        if (!(e instanceof Untranslatable)) throw e
        field.reason = e.message
      }
    }
    field.skeleton = true
    field.def = { lang: 'aql', text: '<translate the SQL to AQL>' }
    field.comments.push(`${TODO}: this ${kind} of view \`${this.lkName}\` reads another view, so it must be a Dataset dimension written in AQL${field.reason ? ` (not translated: ${field.reason})` : ''}. Keep single-view sub-expressions as hidden @sql helper dimensions of the model — see references/sql-references.md#step-4-write-the-definition-in-aql`,
      'LookML:', ...indent(what, 2).split('\n'))
    return field
  }

  // -------------------------------------------------------------------------------------------------------------------
  // Rendering

  /** Lines of one field block. `skeleton` fields are printed as comments. */
  static renderField (field, constName = n => n) {
    const p = field.props
    const lines = [`${field.kind} ${field.name} {`]
    if (p.model) lines.push(`  model: ${p.model}`)
    lines.push(`  label: ${amlString(p.label, constName)}`, `  type: '${p.type}'`)
    if (p.description !== undefined) lines.push(`  description: ${amlString(String(p.description).trim(), constName)}`)
    if (p.hidden) lines.push('  hidden: true')
    if (p.primary_key) lines.push('  primary_key: true')
    if (p.allowed_values) lines.push(`  allowed_values: [${p.allowed_values.join(', ')}]`)
    if (p.format) lines.push(`  format: '${p.format.replace(/'/g, "\\'")}'`)
    if (p.formatConst) lines.push(`  format: ${p.formatConst}`)
    if (field.def) lines.push(...ViewConv.renderDefinition('definition', field.def))
    lines.push('}')
    const body = field.skeleton ? lines.map(l => `// ${l}`) : lines
    return [...field.comments.flatMap(c => String(c).split('\n')).map(c => (c ? `// ${c}` : '//')), ...body]
  }

  static renderDefinition (key, def) {
    const text = def.text.trim()
    // A `--` comment on the last line would swallow the `;;` terminator, so such SQL gets `;;` on its own line.
    if (!text.includes('\n') && !text.includes('--') && text.length <= 100) return [`  ${key}: @${def.lang} ${text};;`]
    return [`  ${key}: @${def.lang}`, ...indent(text, 4).split('\n'), '  ;;']
  }
}
