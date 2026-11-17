// A small SQL expression → AQL translator. It covers only the shapes the references call mechanical
// (sql-references.md "Step 4", measure.md "number"): references, literals, arithmetic, comparisons, AND / OR / NOT,
// IS [NOT] NULL, BETWEEN, COALESCE, NULLIF, `a / NULLIF(b, 0)`, CASE WHEN, and one-argument SQL aggregates.
// Anything else throws Untranslatable, and the caller leaves a TODO for the AI pass.

import { aqlString } from './util.mjs'

export class Untranslatable extends Error {
  constructor (message) { super(String(message).replace(/\s+/g, ' ')) }
}

const KEYWORDS = new Set(['AND', 'OR', 'NOT', 'IS', 'NULL', 'TRUE', 'FALSE', 'CASE', 'WHEN', 'THEN', 'ELSE', 'END',
  'BETWEEN', 'DISTINCT', 'IN', 'LIKE'])
const AGGREGATES = { SUM: 'sum', AVG: 'average', MIN: 'min', MAX: 'max', COUNT: 'count', MEDIAN: 'median' }

function tokenize (sql) {
  const tokens = []
  const re = /\s+|--[^\n]*|\/\*[\s\S]*?\*\/|(\$\{TABLE\}\.(?:"[^"]+"|`[^`]+`|\[[^\]]+\]|\w+))|(\$\{\s*[A-Za-z_][\w.]*(?:::\w+)?\s*\})|(\d+\.\d+|\.\d+|\d+)|('(?:[^']|'')*')|([A-Za-z_]\w*)|(<>|!=|<=|>=|\|\||[=<>+\-*/(),])/y
  let pos = 0
  while (pos < sql.length) {
    re.lastIndex = pos
    const m = re.exec(sql)
    if (!m) throw new Untranslatable(`unexpected text at "${sql.slice(pos, pos + 20)}"`)
    const at = pos
    pos = re.lastIndex
    if (m[1]) tokens.push({ t: 'column', v: m[1], at, end: pos })
    else if (m[2]) tokens.push({ t: 'ref', v: m[2], at, end: pos })
    else if (m[3]) tokens.push({ t: 'num', v: m[3], at, end: pos })
    else if (m[4]) tokens.push({ t: 'str', v: m[4].slice(1, -1).replace(/''/g, "'"), at, end: pos })
    else if (m[5]) tokens.push(KEYWORDS.has(m[5].toUpperCase()) ? { t: 'kw', v: m[5].toUpperCase(), at, end: pos } : { t: 'id', v: m[5], at, end: pos })
    else if (m[6]) tokens.push({ t: 'op', v: m[6], at, end: pos })
  }
  return tokens
}

/**
 * Translate `sql` to AQL.
 *
 * env.ref(view | null, field)  → AQL text of a `${field}` / `${view.field}` reference; may throw Untranslatable
 * env.column(sqlText)          → AQL text for a `${TABLE}.col` reference (a dimension over that column); optional
 * env.operand(sqlText)         → AQL operand of a SQL aggregate argument (measure context only); optional
 * env.countAll()               → AQL for COUNT(*); optional
 *
 * Returns { aql, hasDivision, hasDecimal, hasAggregate, refs }.
 */
export function sqlToAql (sql, env) {
  const tokens = tokenize(sql)
  let i = 0
  const info = { hasDivision: false, hasDecimal: false, hasAggregate: false, hasArithmetic: false, refs: [] }
  const peek = () => tokens[i]
  const is = (t, v) => peek() && peek().t === t && (v === undefined || peek().v === v)
  const eat = (t, v) => { if (!is(t, v)) throw new Untranslatable(`expected ${v || t}`); return tokens[i++] }

  const parseOr = () => { let l = parseAnd(); while (is('kw', 'OR')) { i++; l = `${l} or ${parseAnd()}` } return l }
  const parseAnd = () => { let l = parseNot(); while (is('kw', 'AND')) { i++; l = `${l} and ${parseNot()}` } return l }
  const parseNot = () => { if (is('kw', 'NOT')) { i++; return `not (${parseNot()})` } return parseCmp() }

  function parseCmp () {
    const l = parseAdd()
    if (is('op') && ['=', '<>', '!=', '<', '>', '<=', '>='].includes(peek().v)) {
      const op = tokens[i++].v
      return `${l} ${{ '=': '==', '<>': '!=' }[op] || op} ${parseAdd()}`
    }
    if (is('kw', 'IS')) {
      i++
      const not = is('kw', 'NOT') ? (i++, true) : false
      eat('kw', 'NULL')
      return `${l} is ${not ? 'not ' : ''}null`
    }
    if (is('kw', 'BETWEEN')) {
      i++
      const a = parseAdd(); eat('kw', 'AND'); const b = parseAdd()
      return `${l} >= ${a} and ${l} <= ${b}`
    }
    if (is('kw', 'IN') || is('kw', 'LIKE') || is('kw', 'NOT')) throw new Untranslatable(`${peek().v} is not translated`)
    return l
  }

  function parseAdd () {
    let l = parseMul()
    while (is('op', '+') || is('op', '-') || is('op', '||')) {
      const op = tokens[i++].v
      if (op === '||') throw new Untranslatable('string concatenation is not translated')
      info.hasArithmetic = true
      l = `${l} ${op} ${parseMul()}`
    }
    return l
  }

  function parseMul () {
    let l = parseUnary()
    while (is('op', '*') || is('op', '/')) {
      const op = tokens[i++].v
      info.hasArithmetic = true
      const r = parseUnary()
      if (op === '/' && r && r.nullifZero) l = `safe_divide(${l}, ${r.nullifZero})`
      else { if (op === '/') info.hasDivision = true; l = `${l} ${op} ${r}` }
    }
    return String(l)
  }

  function parseUnary () {
    if (is('op', '-')) { i++; return `-${parseUnary()}` }
    return parsePrimary()
  }

  function parsePrimary () {
    const tok = peek()
    if (!tok) throw new Untranslatable('unexpected end of expression')
    if (tok.t === 'num') { i++; if (tok.v.includes('.')) info.hasDecimal = true; return tok.v }
    if (tok.t === 'str') { i++; return aqlString(tok.v) }
    if (tok.t === 'ref') {
      i++
      const m = tok.v.match(/^\$\{\s*([\w.]+?)(?:::\w+)?\s*\}$/)
      const parts = m[1].split('.')
      if (parts.length > 2) throw new Untranslatable(`reference ${tok.v}`)
      const [view, field] = parts.length === 2 ? parts : [null, parts[0]]
      info.refs.push({ view, field })
      return env.ref(view, field)
    }
    if (tok.t === 'column') {
      i++
      if (!env.column) throw new Untranslatable('${TABLE} column in AQL')
      return env.column(tok.v)
    }
    if (tok.t === 'kw' && ['TRUE', 'FALSE', 'NULL'].includes(tok.v)) { i++; return tok.v.toLowerCase() }
    if (tok.t === 'op' && tok.v === '(') { i++; const e = parseOr(); eat('op', ')'); return `(${e})` }
    if (tok.t === 'kw' && tok.v === 'CASE') return parseCase()
    if (tok.t === 'id') return parseCall()
    throw new Untranslatable(`unexpected "${tok.v}"`)
  }

  function parseCase () {
    eat('kw', 'CASE')
    if (!is('kw', 'WHEN')) throw new Untranslatable('CASE <value> WHEN is not translated')
    const args = []
    while (is('kw', 'WHEN')) {
      i++
      const c = parseOr(); eat('kw', 'THEN'); const v = parseOr()
      args.push(`when: ${c}, then: ${v}`)
    }
    if (is('kw', 'ELSE')) { i++; args.push(`else: ${parseOr()}`) }
    eat('kw', 'END')
    return `case(${args.join(', ')})`
  }

  function parseCall () {
    const name = tokens[i++].v.toUpperCase()
    if (!is('op', '(')) throw new Untranslatable(`bare identifier "${name}"`)
    const open = tokens[i++]
    if (AGGREGATES[name]) {
      if (!env.operand) throw new Untranslatable(`SQL aggregate ${name} outside a measure`)
      info.hasAggregate = true
      let distinct = false
      if (is('kw', 'DISTINCT')) { distinct = true; i++ }
      if (name === 'COUNT' && is('op', '*')) {
        i++; eat('op', ')')
        if (!env.countAll) throw new Untranslatable('COUNT(*)')
        return env.countAll()
      }
      // The argument is handed over as SQL text: the caller turns it into a dimension operand.
      const start = peek() ? peek().at : open.end
      let depth = 0
      while (i < tokens.length && !(depth === 0 && is('op', ')'))) {
        if (is('op', '(')) depth++
        if (is('op', ')')) depth--
        if (depth === 0 && is('op', ',')) throw new Untranslatable(`${name} with several arguments`)
        i++
      }
      const close = eat('op', ')')
      const operand = env.operand(sql.slice(start, close.at).trim(), name)
      if (distinct && name !== 'COUNT') throw new Untranslatable(`${name}(DISTINCT …)`)
      return `${distinct ? 'count_distinct' : AGGREGATES[name]}(${operand})`
    }
    const args = []
    if (!is('op', ')')) { args.push(parseOr()); while (is('op', ',')) { i++; args.push(parseOr()) } }
    eat('op', ')')
    if (name === 'COALESCE' && args.length >= 1) return `coalesce(${args.join(', ')})`
    if (name === 'NULLIF' && args.length === 2) {
      const out = new String(`nullif(${args.join(', ')})`) // eslint-disable-line no-new-wrappers
      if (/^0(\.0+)?$/.test(args[1])) out.nullifZero = args[0]
      return out
    }
    throw new Untranslatable(`function ${name}() is not translated`)
  }

  const aql = String(parseOr())
  if (i < tokens.length) throw new Untranslatable(`unexpected "${tokens[i].v}"`)
  return { aql, ...info }
}

// ---------------------------------------------------------------------------------------------------------------------
// Helpers for raw SQL text.

/** Remove `--` and block comments (not inside string literals). */
export function stripSqlComments (sql) {
  return sql.replace(/('(?:[^']|'')*')|--[^\n]*|\/\*[\s\S]*?\*\//g, (m, str) => str || ' ')
}

/** Remove parentheses that wrap the whole expression. */
export function stripOuterParens (sql) {
  let s = sql.trim()
  for (;;) {
    if (!s.startsWith('(') || !s.endsWith(')')) return s
    let depth = 0; let wraps = true
    for (let k = 0; k < s.length; k++) {
      if (s[k] === '(') depth++
      else if (s[k] === ')') { depth--; if (depth === 0 && k < s.length - 1) { wraps = false; break } }
    }
    if (!wraps) return s
    s = s.slice(1, -1).trim()
  }
}

/** Split on a keyword (`AND`, `OR`) or `=` at parenthesis depth 0, outside strings. BETWEEN … AND stays together. */
export function splitTopLevel (sql, sep) {
  const parts = []
  let depth = 0; let start = 0; let between = false
  const re = /'(?:[^']|'')*'|\$\{[^}]*\}|\(|\)|\b(?:AND|OR|BETWEEN)\b|<=|>=|!=|<>|=/gi
  let m
  while ((m = re.exec(sql))) {
    const tok = m[0].toUpperCase()
    if (tok === '(') depth++
    else if (tok === ')') depth--
    else if (depth === 0 && tok === 'BETWEEN') between = true
    else if (depth === 0 && tok === 'AND' && between) between = false
    else if (depth === 0 && tok === sep) { parts.push(sql.slice(start, m.index).trim()); start = re.lastIndex }
  }
  parts.push(sql.slice(start).trim())
  return parts
}

/** True when `re` matches at parenthesis depth 0, outside strings. */
export function hasTopLevel (sql, words) {
  return words.some(w => splitTopLevel(sql, w).length > 1)
}
