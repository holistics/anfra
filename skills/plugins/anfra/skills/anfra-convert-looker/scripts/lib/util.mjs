// Small helpers shared by the converter: names, AML strings, comment markers, LookML re-printing.

/**
 * Markers the AI pass greps for. TODO = not converted, work left. REVIEW = converted, but check it.
 * Code writes them untagged; convert.mjs tags each one with its kind (`TODO(lookml-to-aml:<kind>)`, see kinds.mjs)
 * when it writes the files.
 */
export const TODO = 'TODO(lookml-to-aml)'
export const REVIEW = 'REVIEW(lookml-to-aml)'

export const asArray = x => (x === undefined || x === null ? [] : Array.isArray(x) ? x : [x])

/** `order_items` → `Order Items` (the label Looker shows by default). */
export const titleCase = s => String(s).replace(/_+/g, ' ').trim().replace(/(^|\s)(\S)/g, (m, a, b) => a + b.toUpperCase())

/** Words that cannot be the name of an AML field. */
const RESERVED = new Set(['const', 'else', 'false', 'if', 'true', 'use', 'null', 'func', 'import', 'export'])

/**
 * A LookML field name as an AML identifier. LookML allows names that AML does not: a leading digit (`7d_los`) and
 * AML keywords (`use`). Those get a leading / trailing underscore; every reference is rewritten the same way.
 */
export function amlName (name) {
  let s = String(name).replace(/\W/g, '_')
  if (/^\d/.test(s)) s = `_${s}`
  if (RESERVED.has(s)) s = `${s}_`
  return s
}

/** `"Shipping Address"` → `shipping_address` (name of a Dataset view group). */
export function slug (s) {
  const out = String(s).toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
  return /^[a-z]/.test(out) ? out : `g_${out}`
}

/** Remove the common indentation of a block of text and trim blank lines at both ends. */
export function dedent (text) {
  const lines = String(text).replace(/\t/g, '  ').split('\n')
  while (lines.length && !lines[0].trim()) lines.shift()
  while (lines.length && !lines[lines.length - 1].trim()) lines.pop()
  // The first line of a LookML block usually follows `sql:` on the same line, so it carries no indentation.
  const rest = lines.slice(1).filter(l => l.trim())
  const indent = rest.length ? Math.min(...rest.map(l => l.match(/^ */)[0].length)) : 0
  return lines.map((l, i) => (i === 0 ? l.trim() : l.slice(Math.min(indent, l.match(/^ */)[0].length)).trimEnd())).join('\n')
}

export const indent = (text, n) => String(text).split('\n').map(l => (l ? ' '.repeat(n) + l : l)).join('\n')

/**
 * An AML single-quoted string. `@{CONST}` becomes `${CONST}` interpolation (constants may be renamed).
 * Text with line breaks uses a `'''` string.
 */
export function amlString (value, constName = n => n) {
  let s = String(value).replace(/\\/g, '\\\\').replace(/\$\{/g, '\\${')
  s = s.replace(/@\{(\w+)\}/g, (m, n) => '${' + constName(n) + '}')
  if (s.includes('\n')) return `'''${s.replace(/'''/g, "\\'\\'\\'")}'''`
  return `'${s.replace(/'/g, "\\'")}'`
}

/** An AQL text literal. */
export const aqlString = v => `'${String(v).replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`

/** Lines of a `//` comment. Accepts a string (may hold line breaks) or a list. */
export function commentLines (text) {
  return asArray(text).flatMap(t => String(t).split('\n')).map(l => (l.trim() ? `// ${l.trimEnd()}` : '//'))
}

// ---------------------------------------------------------------------------------------------------------------------
// LookML re-printing. The parser gives JSON, not source text; comments quote LookML rebuilt from that JSON.

const SQL_KEY = /^(sql|sql_.+|html|expression|expression_custom_filter)$/
const BARE = /^[A-Za-z_][\w.*+-]*$/
const skipKey = k => k.startsWith('$') || k === 'errors'

const scalar = (key, v) => {
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  if (SQL_KEY.test(key)) return `${dedent(v)} ;;`
  if (['type', 'relationship', 'datatype', 'style', 'direction', 'kind', 'period', 'extension', 'units',
    'week_start_day', 'from', 'view_name', 'field', 'user_attribute', 'foreign_key', 'list_field', 'based_on',
    'based_on_time', 'datagroup_trigger', 'from_field', 'to_field', 'distribution_style', 'suggest_dimension',
    'suggest_explore', 'order_by_field', 'value_format_name', 'percentile', 'precision', 'fiscal_month_offset',
    'increment_offset', 'limit', 'explore_source'].includes(key) && BARE.test(String(v))) return String(v)
  return JSON.stringify(String(v))
}

const isNamedBlocks = v => v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length > 0 &&
  Object.values(v).every(c => c && typeof c === 'object' && !Array.isArray(c) && c.$name !== undefined)

function printBody (obj, depth) {
  const pad = '  '.repeat(depth)
  const out = []
  for (const [key, v] of Object.entries(obj)) {
    if (skipKey(key) || v === undefined) continue
    if (key === 'filters' && Array.isArray(v) && v.every(e => e && typeof e === 'object')) {
      const pairs = filterPairs(v)
      out.push(`${pad}filters: [${pairs.map(([f, e]) => `${f}: ${JSON.stringify(e)}`).join(', ')}]`)
    } else if (Array.isArray(v)) {
      if (v.every(e => e && typeof e === 'object')) {
        for (const e of v) out.push(`${pad}${key}: {`, ...printBody(e, depth + 1), `${pad}}`)
      } else {
        out.push(`${pad}${key}: [${v.map(e => (BARE.test(String(e)) ? e : JSON.stringify(String(e)))).join(', ')}]`)
      }
    } else if (isNamedBlocks(v)) {
      for (const [name, child] of Object.entries(v)) {
        const inner = printBody(child, depth + 1)
        if (inner.length) out.push(`${pad}${key}: ${name} {`, ...inner, `${pad}}`); else out.push(`${pad}${key}: ${name} {}`)
      }
    } else if (v && typeof v === 'object') {
      out.push(`${pad}${key}: {`, ...printBody(v, depth + 1), `${pad}}`)
    } else {
      const text = scalar(key, v)
      const lines = text.split('\n')
      out.push(`${pad}${key}: ${lines[0]}`, ...lines.slice(1).map(l => `${pad}  ${l}`))
    }
  }
  return out
}

/** Print one LookML object, e.g. `toLookml('dimension', 'status', {...})`. */
export function toLookml (kind, name, obj) {
  const body = printBody(obj || {}, 1)
  const head = name ? `${kind}: ${name}` : `${kind}:`
  if (!body.length) return `${head} {}`
  return body.length ? [`${head} {`, ...body, '}'].join('\n') : `${head} {}`
}

/** Print one LookML parameter, e.g. `paramToLookml('sql_on', '...')`. */
export const paramToLookml = (key, value) => printBody({ [key]: value }, 0).join('\n')

/**
 * `filters` in every syntax → `[[field, expression], …]`.
 * New syntax parses to `[{ status: "complete", id: ">5" }]`, old syntax to `[{ field: ["status"], value: ["a"] }]`.
 */
export function filterPairs (filters) {
  const out = []
  for (const entry of asArray(filters)) {
    if (!entry || typeof entry !== 'object') continue
    const keys = Object.keys(entry).filter(k => !skipKey(k))
    if (keys.includes('field') && keys.includes('value') && keys.length === 2) {
      out.push([String(asArray(entry.field)[0]), String(asArray(entry.value)[0])])
    } else {
      for (const k of keys) out.push([k, String(asArray(entry[k])[0])])
    }
  }
  return out
}
