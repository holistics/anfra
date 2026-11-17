// LookML measure → AML measure (in a Model) or metric (in a Dataset) — references/measure.md.
//
// `scope` says how the measure reads fields:
//   scope.self          AML name of the measure's own model
//   scope.inDataset     true when the measure moved to a Dataset (it uses another view)
//   scope.ref(view, name, opts) → { aql, kind, type, foreign }   or throws Untranslatable

import { TODO, REVIEW, titleCase, indent, toLookml, filterPairs } from './util.mjs'
import { sqlToAql, Untranslatable } from './sql-expr.mjs'
import { filterToAql, Unsupported } from './filter-expr.mjs'

const AGGREGATES = { sum: 'sum', average: 'average', min: 'min', max: 'max', median: 'median', count_distinct: 'count_distinct' }
const DISTINCT = { sum_distinct: 'sum', average_distinct: 'average', median_distinct: 'median', percentile_distinct: 'percentile_cont' }
const hasLiquid = s => /\{%|\{\{/.test(s || '')
const norm = sql => String(sql || '').replace(/\s+/g, ' ').trim()
const loneRef = sql => { const m = norm(sql).match(/^\$\{\s*(?:(\w+)\.)?(\w+)(?:::\w+)?\s*\}$/); return m && { view: m[1] || null, field: m[2] } }

class Problem extends Error {}

/** Convert one measure. In a Model the field is added to `conv`; in a Dataset it is returned for the caller. */
export function convertMeasure (conv, name, lk, scope) {
  const what = toLookml('measure', name, lk)
  const kind = scope.inDataset ? 'metric' : 'measure'
  const type = lk.type || 'string'
  const reviews = []
  const todos = []
  const where = `${kind} ${name}`
  let amlType = type === 'list' ? 'text' : 'number'
  let def = null

  /** The AQL operand of an aggregate (measure.md#sql). */
  const operand = (sql, { numeric = true, suffix = 'operand' } = {}) => {
    if (!sql) throw new Problem(`\`type: ${type}\` without \`sql\` cannot be converted`)
    if (hasLiquid(sql)) throw new Problem('Liquid in the measure SQL — see references/liquid.md, parameter.md, filter.md')
    const lone = loneRef(sql)
    if (lone) {
      let r
      try { r = scope.ref(lone.view, lone.field) } catch (e) { if (e instanceof Untranslatable) throw new Problem(e.message); throw e }
      if (r.kind !== 'dimension') throw new Problem(`\`\${${lone.field}}\` is a ${r.kind}, not a dimension: an aggregate over a measure is not converted`)
      if (r.note) reviews.push(r.note)
      if (numeric && r.type !== 'number' && !r.foreign && !scope.inDataset) {
        // AQL type-checks: sum(), average(), … need a number. Looker does not care what the dimension is declared as.
        const h = conv.helper(`${r.name}__number`, `${titleCase(r.name)} (as number)`, 'number', `\${${lone.field}}`)
        reviews.push(`\`${r.name}\` is a '${r.type}' dimension (LookML declares no number type), and AQL \`${type}\` needs a number: the hidden number dimension \`${h.name}\` over the same SQL is aggregated. If the column is numeric, make \`${r.name}\` itself \`type: 'number'\` and drop the helper`)
        return { ...r, aql: `${scope.self}.${h.name}`, type: 'number' }
      }
      return r
    }
    const cross = [...sql.matchAll(/\$\{\s*(\w+)\.(\w+)(?:::\w+)?\s*\}/g)].filter(m => m[1] !== conv.lkName && m[1] !== 'TABLE')
    if (cross.length) throw new Problem('the SQL mixes fields of several views: move the single-view parts into hidden helper dimensions and write the rest in AQL — see references/sql-references.md#step-4-write-the-definition-in-aql')
    const key = norm(sql)
    if (conv.sqlToDim.has(key)) {
      const dim = conv.resolve(conv.sqlToDim.get(key))
      if (!numeric || dim.type === 'number') return { aql: `${scope.self}.${dim.name}`, kind: 'dimension', type: dim.type }
    }
    let helper = conv.operandHelpers.get(key)
    if (!helper) {
      const label = lk.label || titleCase(name)
      const h = conv.helper(`${name}__${suffix}`, `${label} (${suffix})`, numeric ? 'number' : 'text', sql)
      if (!numeric) h.comments.push(`${REVIEW}: helper dimension for measure \`${name}\`; its type was not derived from the SQL — set it`)
      helper = h.name
      conv.operandHelpers.set(key, helper)
    }
    return { aql: `${scope.self}.${helper}`, kind: 'dimension' }
  }
  /** `agg(x)`, or `agg(<own model>, <other model>.x)` when the operand belongs to another model. */
  const agg = (fn, op, extra = '') => {
    if (op.foreign) reviews.push(`aggregate over a dimension of another model: the measure's own model \`${scope.self}\` is the table that is aggregated — see references/sql-references.md#step-5-measures`)
    return `${fn}(${op.foreign ? `${scope.self}, ` : ''}${op.aql}${extra})`
  }
  const countAll = () => {
    if (conv.pk) return `count(${scope.self}.${conv.pk})`
    conv.helper('row_marker', 'Row Marker', 'number', '1')
    reviews.push('the view has no primary key: a hidden `row_marker` dimension is counted. A person should pick a real primary key — see references/measure.md#count')
    return `count(${scope.self}.row_marker)`
  }
  const measureRef = sql => {
    const lone = sql && loneRef(sql)
    if (!lone) throw new Problem(`\`type: ${type}\` needs \`sql\` to be one \`\${measure}\` reference`)
    let r
    try { r = scope.ref(lone.view, lone.field) } catch (e) { if (e instanceof Untranslatable) throw new Problem(e.message); throw e }
    if (r.kind !== 'measure') throw new Problem(`\`\${${lone.field}}\` is not a measure`)
    return r.aql
  }

  try {
    if (type === 'count') {
      def = countAll()
    } else if (AGGREGATES[type]) {
      const fn = type === 'count_distinct' && lk.approximate === true ? 'approx_count_distinct' : AGGREGATES[type]
      if (fn === 'approx_count_distinct') reviews.push('approx_count_distinct exists on Snowflake, BigQuery, Databricks, MotherDuck and Presto/Athena only; use count_distinct elsewhere')
      if (type === 'median') reviews.push('median over an even number of rows may differ from Looker — see references/measure.md#median')
      def = agg(fn, operand(lk.sql, { numeric: !['count_distinct', 'min', 'max'].includes(type) }))
    } else if (type === 'list') {
      if (!lk.list_field) throw new Problem('`type: list` without `list_field`')
      const r = scope.ref(...(lk.list_field.includes('.') ? lk.list_field.split('.') : [null, lk.list_field]))
      def = `string_agg(${r.aql}, sep: ', ', distinct: true)`
      reviews.push('separator and order of a list differ per database in Looker; SQL Server does not support `distinct` here — see references/measure.md#list')
    } else if (type === 'percentile') {
      if (lk.percentile === undefined) throw new Problem('`type: percentile` without `percentile`')
      def = agg('percentile_cont', operand(lk.sql), `, ${Number(lk.percentile) / 100}`)
      reviews.push('Looker\'s percentile rule is neither percentile_cont nor percentile_disc exactly; on BigQuery, MySQL and Presto/Athena use percentile_disc — see references/measure.md#percentile')
    } else if (DISTINCT[type]) {
      const op = operand(lk.sql)
      const extra = type === 'percentile_distinct' ? `, ${Number(lk.percentile) / 100}` : ''
      const keyLone = lk.sql_distinct_key && loneRef(lk.sql_distinct_key)
      const keyIsPk = !lk.sql_distinct_key || (keyLone && (!keyLone.view || keyLone.view === conv.lkName) && keyLone.field === conv.pk)
      if (keyIsPk) {
        if (!conv.pk) throw new Problem(`\`type: ${type}\` without \`sql_distinct_key\`, in a view without a primary key`)
        def = agg(DISTINCT[type], op, extra)
      } else {
        const key = operand(lk.sql_distinct_key, { numeric: false, suffix: 'distinct_key' })
        def = `unique(${key.aql}) | ${DISTINCT[type]}(max(${op.aql})${extra})`
        reviews.push('one value per key is assumed; with several values per key the AQL takes the largest — see references/measure.md#average_distinct-median_distinct-percentile_distinct-sum_distinct')
      }
    } else if (type === 'percent_of_previous') {
      const m = measureRef(lk.sql)
      def = `safe_divide(\n  ${m} - previous(${m}, order: 'rows'),\n  previous(${m}, order: 'rows')\n)`
      reviews.push('Looker computes this over the displayed rows, AQL inside the query — see references/measure.md#percent_of_previous')
    } else if (type === 'percent_of_total') {
      def = `percent_of_total(${measureRef(lk.sql)}, 'column_total')`
      reviews.push(`\`direction: ${lk.direction || '(none)'}\` — 'row_total' is the pivoted equivalent of a row direction; check the 0–1 / 0–100 scale before attaching a percent format — see references/measure.md#percent_of_total`)
    } else if (type === 'running_total') {
      def = `running_total(${measureRef(lk.sql)})`
      reviews.push('Looker runs along the displayed rows; AQL running_total runs along the date dimensions of the query — see references/measure.md#running_total')
    } else if (type === 'period_over_period') {
      def = periodOverPeriod(lk, scope)
      reviews.push('behaviour on partial periods has not been compared — see references/measure.md#period_over_period')
    } else if (['number', 'string', 'yesno', 'date', 'date_time'].includes(type)) {
      if (!lk.sql) throw new Problem(`\`type: ${type}\` without \`sql\``)
      if (hasLiquid(lk.sql)) throw new Problem('Liquid in the measure SQL — see references/liquid.md, parameter.md, filter.md')
      const refTypes = []
      const r = sqlToAql(lk.sql, {
        ref: (view, f) => { const x = scope.ref(view, f); refTypes.push(x.type); return x.aql },
        operand: (sql, fn) => { const op = operand(sql, { numeric: ['SUM', 'AVG', 'MEDIAN'].includes(fn) }); refTypes.push(op.type); return op.foreign ? `${scope.self}, ${op.aql}` : op.aql },
        countAll
      })
      def = r.aql
      amlType = { number: 'number', yesno: 'truefalse', date: 'date', date_time: 'datetime' }[type] || (r.hasArithmetic && !lk.type ? 'number' : 'text')
      if (type === 'date' && refTypes.includes('datetime')) def = `cast(${def}, 'date')`
      if (r.hasDivision && !r.hasDecimal) reviews.push('plain division: on two integers some databases truncate; whether AQL `/` does is not documented — see references/measure.md#number')
      if (r.hasAggregate) reviews.push('a raw SQL aggregate inside the measure is now an AQL aggregate, which is protected against fan-out; numbers can differ across a one-to-many join')
      if (lk.filters) reviews.push('`filters` on a non-aggregate measure is ignored')
    } else {
      throw new Problem(`unknown measure type \`${type}\``)
    }
  } catch (e) {
    if (!(e instanceof Problem) && !(e instanceof Untranslatable)) throw e
    todos.push(e.message)
  }

  // filters → where()
  if (def && lk.filters && !['number', 'string', 'yesno', 'date', 'date_time'].includes(type)) {
    const conds = []
    for (const [key, expr] of filterPairs(lk.filters)) {
      try {
        const [view, field] = key.includes('.') ? key.split('.') : [null, key]
        const r = scope.ref(view, field, { grain: true })
        if (r.kind !== 'dimension') throw new Unsupported(`\`${key}\` is not a dimension`)
        const out = filterToAql(r.aql, r.type, expr, { caseInsensitive: conv.lkModel.case_sensitive === false })
        reviews.push(...out.review)
        if (r.foreign) reviews.push('a filter condition on a field of another model; not checked on data')
        if (out.cond) conds.push(out.cond)
      } catch (e) {
        if (!(e instanceof Unsupported) && !(e instanceof Untranslatable)) throw e
        todos.push(`filter \`${key}: ${JSON.stringify(expr)}\` is NOT in the where() below (${e.message}). Add its condition — see references/filter-expressions.md`)
      }
    }
    if (conds.length === 1 && def.length + conds[0].length < 80) def += ` | where(${conds[0]})`
    else if (conds.length) def += `\n  | where(\n${conds.map(c => `    ${c}`).join(',\n')}\n  )`
  }

  const stubType = { yesno: 'truefalse', date: 'date', date_time: 'datetime', string: 'text', list: 'text' }[type] || 'number'
  let field
  if (def) {
    field = conv.makeField(kind, name, lk, { type: amlType, def: { lang: 'aql', text: def } })
  } else if (!scope.inDataset && ['number', 'string', 'yesno', 'date', 'date_time'].includes(type) && lk.sql && !hasLiquid(lk.sql)) {
    // measure.md#number, rule 4: keep the SQL. It compiles, but a raw SQL measure is not protected against fan-out.
    const r = conv.rewriteSql(lk.sql)
    field = conv.makeField(kind, name, lk, { type: stubType, def: { lang: 'sql', text: r.text } })
    todos.unshift(...r.todos)
    reviews.push(...r.reviews)
    todos[todos.length - 1] = `translate this measure to AQL (${todos[todos.length - 1]}). Until then it is the raw-SQL fallback, which is not protected against fan-out — see references/measure.md#number`
  } else {
    field = conv.makeField(kind, name, lk, { type: stubType, def: { lang: 'aql', text: scope.inDataset ? '<write the AQL>' : 'null' } })
    if (scope.inDataset) field.skeleton = true
    else { field.stub = true; todos[todos.length - 1] += '. The definition below is a placeholder' }
  }
  for (const t of todos) field.comments.push(`${TODO}: ${t}`)
  for (const t of new Set(reviews)) field.comments.push(`${REVIEW}: ${t}`)
  if (todos.length) {
    field.todo = true
    field.comments.push('LookML:', ...indent(what, 2).split('\n'))
    if (!conv.base || scope.inDataset) conv.project.report({ level: 'TODO', where: `${conv.where(where)}${scope.inDataset ? ` in ${scope.dataset}` : ''}`, message: todos.join('; ') })
  }
  if (scope.inDataset) field.comments.unshift(`moved from view \`${conv.lkName}\`: it uses another view — see references/sql-references.md#viewfield`)
  for (const key of ['convert_tz', 'datatype']) if (lk[key] !== undefined && !scope.inDataset) conv.report('DROPPED', where, `${key}: not used on a measure`)
  return scope.inDataset ? field : conv.add(field)
}

function periodOverPeriod (lk, scope) {
  const split = v => (String(v).includes('.') ? String(v).split('.') : [null, String(v)])
  if (!lk.based_on || !lk.based_on_time) throw new Problem('`period_over_period` needs `based_on` and `based_on_time`')
  const period = String(lk.period || '')
  if (!['day', 'week', 'month', 'quarter', 'year'].includes(period)) throw new Problem(`period_over_period with \`period: ${period || '(none)'}\` is unsupported — see references/measure.md#period_over_period`)
  const m = scope.ref(...split(lk.based_on))
  const t = scope.ref(...split(lk.based_on_time), { grain: true })
  if (m.kind !== 'measure' || t.kind !== 'dimension') throw new Problem('`based_on` must be a measure and `based_on_time` a time dimension')
  const prev = `${m.aql} | relative_period(${t.aql}, interval(-1 ${period}))`
  const kind = lk.kind || 'previous'
  if (kind === 'previous') return prev
  if (kind === 'difference') return `${m.aql} - (${prev})`
  if (kind === 'relative_change') return `safe_divide(\n  ${m.aql} - (${prev}),\n  ${prev}\n)`
  throw new Problem(`period_over_period with \`kind: ${kind}\``)
}

