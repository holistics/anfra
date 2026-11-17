// Looker filter expressions → AQL conditions (references/filter-expressions.md).
// Only the forms listed in that reference are converted. Everything else throws Unsupported, and the caller leaves
// a TODO for the AI pass.

import { aqlString } from './util.mjs'

export class Unsupported extends Error {}

const paren = c => (/ (or|and) /.test(c) ? `(${c})` : c)

/**
 * @param f     the filtered dimension in AQL, e.g. `orders.status`
 * @param type  its AML type: 'text' | 'number' | 'truefalse' | 'date' | 'datetime'
 * @param expr  the Looker filter expression
 * @param opts  { caseInsensitive } — `case_sensitive: no` on the model file
 * @returns { cond, review } — cond is null for "is any value"; review lists points to check
 */
export function filterToAql (f, type, expr, opts = {}) {
  const review = []
  if (/\{\{|\{%/.test(expr)) throw new Unsupported('the value contains Liquid')
  if (expr.trim() === '') return { cond: null, review }
  let cond
  if (type === 'truefalse') cond = yesno(f, expr)
  else if (type === 'number') cond = number(f, expr, review)
  else if (type === 'date' || type === 'datetime') cond = date(f, expr, review)
  else cond = string(f, expr, review, opts)
  return { cond, review }
}

// ---------------------------------------------------------------------------------------------------------------------

function yesno (f, expr) {
  const v = expr.trim().toLowerCase()
  if (v === 'yes' || v === 'true') return `${f} == true`
  if (v === 'no' || v === 'false') return `${f} == false`
  throw new Unsupported(`yes/no value "${expr}"`)
}

function string (f, expr, review, { caseInsensitive }) {
  // 1. split on commas; `^,` is a literal comma
  const terms = []
  let cur = ''
  for (let i = 0; i < expr.length; i++) {
    if (expr[i] === '^' && i + 1 < expr.length) { cur += expr[i] + expr[i + 1]; i++ } else if (expr[i] === ',') { terms.push(cur); cur = '' } else cur += expr[i]
  }
  terms.push(cur)

  const pos = { plain: [], conds: [] }
  const neg = { plain: [], conds: [], onlyValues: true }
  const lhs = caseInsensitive ? `lower(${f})` : f
  const lit = v => aqlString(caseInsensitive ? v.toLowerCase() : v)
  const like = caseInsensitive ? 'ilike' : 'like'

  for (const raw of terms.map(t => t.trim()).filter(t => t !== '')) {
    const negative = raw.startsWith('-') && raw.length > 1
    const body = negative ? raw.slice(1) : raw
    if (body === 'EMPTY') { (negative ? neg : pos).conds.push(negative ? `${f} is not null and ${f} != ''` : `${f} is null or ${f} == ''`); if (negative) neg.onlyValues = false; continue }
    if (body === 'NULL') { (negative ? neg : pos).conds.push(negative ? `${f} is not null` : `${f} is null`); if (negative) neg.onlyValues = false; continue }
    // 2. unescape `^x`, and remember which wildcards were escaped
    let text = ''; let wildcard = false; let escapedWildcard = false
    for (let i = 0; i < body.length; i++) {
      if (body[i] === '^' && i + 1 < body.length) { i++; text += body[i]; if ('%_'.includes(body[i])) escapedWildcard = true } else { if ('%_'.includes(body[i])) wildcard = true; text += body[i] }
    }
    if (wildcard && escapedWildcard) throw new Unsupported(`"${raw}" mixes an escaped and an unescaped wildcard`)
    if (wildcard) (negative ? neg : pos).conds.push(`${f} ${negative ? 'not ' : ''}${like} ${aqlString(text)}`)
    else (negative ? neg : pos).plain.push(text)
  }

  const posParts = [...pos.conds]
  if (pos.plain.length === 1) posParts.unshift(`${lhs} == ${lit(pos.plain[0])}`)
  else if (pos.plain.length > 1) posParts.unshift(`${lhs} in [${pos.plain.map(lit).join(', ')}]`)
  const negParts = [...neg.conds]
  if (neg.plain.length === 1) negParts.unshift(`${lhs} != ${lit(neg.plain[0])}`)
  else if (neg.plain.length > 1) negParts.unshift(`${lhs} not in [${neg.plain.map(lit).join(', ')}]`)

  const posCond = posParts.join(' or ')
  let negCond = negParts.join(' and ')
  if (!posParts.length && negParts.length && neg.onlyValues) {
    negCond = `${f} is null or ${negParts.length > 1 ? `(${negCond})` : negCond}`
    review.push('a negated filter value keeps NULL rows (`is null or …`); not confirmed by the Looker docs')
  }
  if (!posParts.length && !negParts.length) return null
  if (!negParts.length) return posCond
  if (!posParts.length) return negCond
  return `${posParts.length > 1 ? `(${posCond})` : paren(posCond)} and ${negCond}`
}

// ---------------------------------------------------------------------------------------------------------------------

const NUM = '-?(?:\\d+\\.?\\d*|\\.\\d+)'
const numRe = new RegExp(`^${NUM}$`)

function number (f, expr, review) {
  // split on commas outside brackets
  const terms = []
  let depth = 0; let cur = ''
  for (const ch of expr) {
    if ('([' .includes(ch)) depth++
    if (')]'.includes(ch)) depth--
    if (ch === ',' && depth === 0) { terms.push(cur); cur = '' } else cur += ch
  }
  terms.push(cur)
  const trimmed = terms.map(t => t.trim()).filter(t => t !== '')

  // "NOT 66, 99, 4": a leading NOT in a list without another NOT negates every term
  const nots = trimmed.filter(t => /^NOT\b/i.test(t)).length
  const allNegated = trimmed.length > 1 && nots === 1 && /^NOT\b/i.test(trimmed[0]) && trimmed.slice(1).every(t => numRe.test(t))

  const pos = { values: [], conds: [] }
  const neg = { values: [], conds: [], onlyValues: true }
  trimmed.forEach((term, idx) => {
    const t = allNegated && idx > 0 ? `NOT ${term}` : term
    const orParts = t.split(/\s+OR\s+/i)
    if (orParts.length > 1) {
      pos.conds.push(orParts.map(p => paren(andTerm(f, p))).join(' or '))
      return
    }
    if (/\s+AND\s+/i.test(t)) { pos.conds.push(andTerm(f, t)); return }
    const a = atom(f, t)
    if (a.kind === 'eq') pos.values.push(a.value)
    else if (a.kind === 'ne') neg.values.push(a.value)
    else if (a.kind === 'neg') { neg.conds.push(a.cond); neg.onlyValues = false } else pos.conds.push(a.cond)
  })

  const posParts = [...pos.conds]
  if (pos.values.length === 1) posParts.unshift(`${f} == ${pos.values[0]}`)
  else if (pos.values.length > 1) posParts.unshift(`${f} in [${pos.values.join(', ')}]`)
  const negParts = [...neg.conds]
  if (neg.values.length === 1) negParts.unshift(`${f} != ${neg.values[0]}`)
  else if (neg.values.length > 1) negParts.unshift(`${f} not in [${neg.values.join(', ')}]`)

  const posCond = posParts.length > 1 ? posParts.map(paren).join(' or ') : posParts.join('')
  let negCond = negParts.map(paren).join(' and ')
  if (!posParts.length && negParts.length && neg.onlyValues) {
    negCond = `${f} is null or ${negCond}`
    review.push('a negated filter value keeps NULL rows (`is null or …`); not confirmed by the Looker docs')
  }
  if (!negParts.length) return posCond
  if (!posParts.length) return negCond
  return `${paren(posCond)} and ${negCond}`
}

function andTerm (f, text) {
  return text.split(/\s+AND\s+/i).map(p => { const a = atom(f, p); return a.cond || (a.kind === 'eq' ? `${f} == ${a.value}` : `${f} != ${a.value}`) }).join(' and ')
}

function atom (f, text) {
  const t = text.trim()
  let m
  if (/^NULL$/i.test(t)) return { kind: 'cond', cond: `${f} is null` }
  if (/^NOT\s+NULL$/i.test(t)) return { kind: 'neg', cond: `${f} is not null` }
  if ((m = t.match(new RegExp(`^=?\\s*(${NUM})$`)))) return { kind: 'eq', value: m[1] }
  if ((m = t.match(new RegExp(`^(?:NOT\\s+|<>\\s*|!=\\s*)(${NUM})$`, 'i')))) return { kind: 'ne', value: m[1] }
  if ((m = t.match(new RegExp(`^(>=|<=|>|<)\\s*(${NUM})$`)))) return { kind: 'cond', cond: `${f} ${m[1]} ${m[2]}` }
  if ((m = t.match(new RegExp(`^(NOT\\s+)?(${NUM})?\\s*to\\s*(${NUM})?$`, 'i'))) && (m[2] || m[3])) {
    if (m[1]) {
      if (!m[2] || !m[3]) throw new Unsupported(`number filter "${t}"`)
      return { kind: 'neg', cond: `${f} < ${m[2]} or ${f} > ${m[3]}` }
    }
    return { kind: 'cond', cond: [m[2] && `${f} >= ${m[2]}`, m[3] && `${f} <= ${m[3]}`].filter(Boolean).join(' and ') }
  }
  if ((m = t.match(new RegExp(`^(NOT\\s+)?([\\[(])\\s*(${NUM}|-inf)?\\s*,\\s*(${NUM}|inf)?\\s*([\\])])$`, 'i')))) {
    const lo = m[3] && m[3] !== '-inf' ? m[3] : null
    const hi = m[4] && m[4] !== 'inf' ? m[4] : null
    if (!lo && !hi) throw new Unsupported(`number filter "${t}"`)
    if (m[1]) {
      // complement of the interval
      const parts = [lo && `${f} ${m[2] === '(' ? '<=' : '<'} ${lo}`, hi && `${f} ${m[5] === ')' ? '>=' : '>'} ${hi}`].filter(Boolean)
      return { kind: 'neg', cond: parts.join(' or ') }
    }
    const parts = [lo && `${f} ${m[2] === '(' ? '>' : '>='} ${lo}`, hi && `${f} ${m[5] === ')' ? '<' : '<='} ${hi}`].filter(Boolean)
    return { kind: 'cond', cond: parts.join(' and ') }
  }
  throw new Unsupported(`number filter "${t}"`)
}

// ---------------------------------------------------------------------------------------------------------------------

const UNIT = '(second|minute|hour|day|week|month|quarter|year)s?'
const DATE = '(\\d{4})(?:[-/](\\d{1,2}))?(?:[-/](\\d{1,2}))?'
const pad = n => String(n).padStart(2, '0')
const dateLit = (y, mo, d) => [y, mo && pad(mo), d && pad(d)].filter(Boolean).join('-')
const plural = (n, unit) => `${n} ${unit}${Number(n) === 1 ? '' : 's'}`

function date (f, expr, review) {
  const parts = expr.split(',').map(p => p.trim()).filter(Boolean)
  return parts.map(p => dateTerm(f, p, review)).map(c => (parts.length > 1 ? paren(c) : c)).join(' or ')
}

function dateTerm (f, text, review) {
  const t = text.trim().toLowerCase().replace(/\s+/g, ' ')
  const matches = lit => `${f} matches @(${lit})`
  let m
  if (t === 'null') return `${f} is null`
  if (t === 'not null' || t === '-null') return `${f} is not null`
  if (['today', 'yesterday', 'tomorrow'].includes(t)) return matches(t)
  if ((m = t.match(/^this (week|month|quarter|year)$/))) return matches(`this ${m[1]}`)
  if ((m = t.match(/^(last|next) (week|month|quarter|year)$/))) return matches(`${m[1]} 1 ${m[2]}`)
  if ((m = t.match(new RegExp(`^(last )?(\\d+) ${UNIT}$`)))) {
    const n = Number(m[2]); const unit = m[3]
    if (m[1]) review.push('`last N <unit>` is converted like `N <unit>` (includes the current period); Looker\'s docs are ambiguous')
    const now = unit === 'day' ? 'today' : `this ${unit}`
    if (n < 1) throw new Unsupported(`date filter "${text}"`)
    return matches(n === 1 ? now : `${plural(n - 1, unit)} ago - ${now}`)
  }
  if ((m = t.match(new RegExp(`^(\\d+) ${UNIT} ago$`)))) return matches(`${plural(m[1], m[2])} ago`)
  if ((m = t.match(new RegExp(`^(\\d+) ${UNIT} from now$`)))) return matches(`${plural(m[1], m[2])} from now`)
  if ((m = t.match(new RegExp(`^(\\d+) ${UNIT} ago for (\\d+) ${UNIT}$`)))) {
    const n = Number(m[1]); const k = Number(m[3]); const unit = m[2]
    if (unit !== m[4] || k > n) throw new Unsupported(`date filter "${text}"`)
    if (k === n) return matches(`last ${plural(n, unit)}`)
    return matches(`${plural(n, unit)} ago until ${plural(n - k, unit)} ago`)
  }
  if ((m = t.match(new RegExp(`^${DATE}$`)))) return `${f} matches @${dateLit(m[1], m[2], m[3])}`
  if ((m = t.match(new RegExp(`^before ${DATE}$`))) && m[3]) return `${f} < @${dateLit(m[1], m[2], m[3])}`
  if ((m = t.match(new RegExp(`^after ${DATE}$`))) && m[3]) return `${f} >= @${dateLit(m[1], m[2], m[3])}`
  if ((m = t.match(new RegExp(`^before (\\d+) ${UNIT} ago$`)))) return `${f} < @(${plural(m[1], m[2])} ago)`
  if ((m = t.match(new RegExp(`^${DATE} to ${DATE}$`))) && m[3] && m[6]) return matches(`${dateLit(m[1], m[2], m[3])} until ${dateLit(m[4], m[5], m[6])}`)
  if ((m = t.match(new RegExp(`^${DATE} for (\\d+) days?$`))) && m[3]) {
    const end = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]) + Number(m[4])))
    return matches(`${dateLit(m[1], m[2], m[3])} until ${end.toISOString().slice(0, 10)}`)
  }
  throw new Unsupported(`date filter "${text}"`)
}
