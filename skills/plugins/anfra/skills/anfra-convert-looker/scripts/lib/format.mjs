// Looker `value_format` / `value_format_name` → Holistics number format (references/field.md#value_format).

const BUILT_IN = {
  decimal_0: '#,###',
  usd_0: '[$$]#,###',
  gbp_0: '[$£]#,###',
  eur_0: '[$€]#,###',
  usd: '[$$]#,###0.00',
  gbp: '[$£]#,###0.00',
  eur: '[$€]#,###0.00',
  percent_0: '#,###%',
  id: 'none'
}
for (let n = 1; n <= 4; n++) {
  BUILT_IN[`decimal_${n}`] = `#,###0.${'0'.repeat(n)}`
  BUILT_IN[`percent_${n}`] = `#,###0.${'0'.repeat(n)}%`
}

export const builtInFormat = name => BUILT_IN[name]

/** `decimals: N` (removed from Looker). */
export const decimalsFormat = n => (Number(n) > 0 ? `#,###0.${'0'.repeat(Number(n))}` : '#,###')

/**
 * Translate an Excel-style Looker format. Returns the Holistics pattern, or null when it cannot be translated
 * (time formats, zero padding, text in front of the number, phone masks).
 */
export function translateValueFormat (valueFormat) {
  let s = String(valueFormat).trim().replace(/\\"/g, '"') // the parser keeps `\"` escapes of the LookML string
  if (!s || /@\{/.test(s) || /\{\{|\{%/.test(s)) return null

  // Sections that only switch K/M/B by size → the first section's decimals and "A".
  if (/^\[[<>=]+\d+\]/.test(s)) {
    const first = s.split(';')[0].replace(/^\[[^\]]*\]/, '')
    const dec = (first.match(/\.([0#]+)/) || [])[1]
    return `#,###${dec ? `0.${'0'.repeat(dec.length)}` : ''}"A"`
  }
  // positive;negative sections → keep the first (not inside quotes)
  s = splitSections(s)[0]

  let rest = s
  let currency = ''
  let m
  if ((m = rest.match(/^(?:\\?([$£€¥])|"([$£€¥])")\s*/))) { currency = m[1] || m[2]; rest = rest.slice(m[0].length) }
  if (!(m = rest.match(/^([#0,]*[#0])?(\.[0#]+)?(,{1,3})?/)) || !m[0]) return null
  const [whole, intPart = '', decPart = '', scale = ''] = m
  if (/^0{2,}$/.test(intPart.replace(/,/g, '')) && !decPart) return null // zero padding
  rest = rest.slice(whole.length)

  let percent = ''
  let suffix = ''
  if (rest.startsWith('\\%')) { percent = '\\\\%'; rest = rest.slice(2) } else if (rest.startsWith('%')) { percent = '%'; rest = rest.slice(1) }
  if ((m = rest.match(/^"([^"]*)"$/)) || (m = rest.match(/^\\(.)$/))) { suffix = m[1]; rest = '' }
  if (rest.trim() !== '') return null // time tokens, masks, text we cannot place

  let out = currency ? `[$${currency}]` : ''
  out += '#,###'
  if (decPart) out += `0.${'0'.repeat(decPart.length - 1)}`
  out += percent
  if (scale) out += `${scale}"${{ 1: 'K', 2: 'M', 3: 'B' }[scale.length]}"` // the scale letter replaces the printed text
  else if (suffix) {
    if (currency || percent) return null
    out += `[$${suffix}]`
  }
  return out
}

function splitSections (s) {
  const parts = []
  let cur = ''; let quoted = false
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]
    if (ch === '\\' && i + 1 < s.length) { cur += ch + s[++i]; continue }
    if (ch === '"') quoted = !quoted
    if (ch === ';' && !quoted) { parts.push(cur); cur = '' } else cur += ch
  }
  parts.push(cur)
  return parts
}
