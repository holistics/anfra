// One LookML explore → one AML Dataset (references/explore.md, field.md#drill_fields, field.md#group_label,
// sql-references.md#viewfield).

import { TODO, REVIEW, asArray, titleCase, slug, indent, amlString, toLookml, paramToLookml, commentLines, filterPairs } from './util.mjs'
import { sqlToAql, Untranslatable, stripSqlComments, stripOuterParens, splitTopLevel } from './sql-expr.mjs'
import { filterToAql, Unsupported } from './filter-expr.mjs'
import { ViewConv } from './view.mjs'
import { convertMeasure } from './measure.mjs'

const hasLiquid = s => /\{%|\{\{/.test(s || '')
const VIEW_REF = /\$\{\s*(\w+)\.(\w+)(?:::\w+)?\s*\}/g
const viewsIn = sql => new Set([...sql.matchAll(VIEW_REF)].map(m => m[1]))
class Problem extends Error {}

export function convertExplore (project, lkModel, lkModelName, name, ex) {
  const dsName = project.datasetName(lkModelName, name)
  const where = `explore ${name} (${lkModelName})`
  const report = (level, message, extra) => project.report({ level, where, message, ...extra })
  const head = [`explore: ${name}  (model file: ${lkModelName})`]
  const bodyTodos = [] // comment lines inside the Dataset, after the relationships
  const todo = (msg, lookml, target = head) => {
    target.push(`${TODO}: ${msg}`)
    if (lookml) target.push('LookML:', ...indent(lookml, 2).split('\n'))
    report('TODO', msg)
  }

  // ---- members: the base view and one model per join ----------------------------------------------------------------
  const members = new Map() // name used in `${name.field}` → { ref, aml, conv, join }
  const addMember = (ref, viewName, { label, tableName, join } = {}) => {
    const base = project.viewConv(viewName)
    if (!base) return null
    let conv = base
    if (ref !== viewName || tableName !== undefined) conv = project.alias(ref, base, { label: label || titleCase(ref), tableName, explore: name, lkModel, usedBy: `${join ? `join ${ref}` : 'base view'} of explore ${name}` })
    const m = { ref, aml: conv.name, conv, join, isAlias: conv !== base, viewLabel: label }
    members.set(ref, m)
    return m
  }
  const baseView = ex.from || ex.view_name || name
  const baseRef = ex.view_name || name
  if (!addMember(baseRef, baseView, { label: ex.view_label, tableName: ex.sql_table_name })) {
    report('TODO', `base view \`${baseView}\` is not a view of the project; no Dataset is emitted`)
    return null
  }
  const missing = []
  for (const [jname, join] of Object.entries(ex.join || {})) {
    if (!addMember(jname, join.from || jname, { label: join.view_label, tableName: join.sql_table_name, join })) missing.push(jname)
  }

  // ---- relationships -----------------------------------------------------------------------------------------------
  const refField = (view, field) => {
    const m = members.get(view)
    if (!m) throw new Problem(`\`${view}\` is not a view of this explore`)
    const r = m.conv.resolve(field)
    if (r.kind !== 'dimension') throw new Problem(`\`\${${view}.${field}}\` is ${r.kind === 'moved' ? 'a field that reads another view (it moved to the Dataset)' : r.kind === 'unknown' ? 'not a field of the view' : `a ${r.kind}`}; a relationship key must be a model dimension`)
    return { model: m.aml, field: r.name, member: m, timeframe: r.timeframe }
  }
  const toAql = sql => sqlToAql(sql, { ref: (view, field) => { if (!view) throw new Untranslatable(`unqualified reference \${${field}}`); const f = refField(view, field); return `${f.model}.${f.field}` } }).aql
  /** A hidden helper dimension of member `m` that holds `sql` (written with `${<join name>.field}`) unchanged. */
  const helperDim = (m, helperName, label, type, sql) => m.conv.helper(helperName, label, type, sql.replace(VIEW_REF, (all, v) => (v === m.ref ? all.replace(`${v}.`, `${m.conv.lkName}.`) : all))).name

  const relationships = []
  for (const jname of missing) relationships.push({ comments: [`${TODO}: join \`${jname}\`: its view is not a view of the project; the model is left out`, 'LookML:', ...indent(toLookml('join', jname, ex.join[jname]), 2).split('\n')] })
  for (const [jname, m] of members) {
    if (!m.join) continue
    const join = m.join
    const lookml = toLookml('join', jname, join)
    const comments = []
    try {
      relationships.push({ comments, lines: buildRelationship(jname, m, join, comments) })
    } catch (e) {
      if (!(e instanceof Problem) && !(e instanceof Untranslatable)) throw e
      const msg = `join \`${jname}\`: no relationship is emitted — ${e.message}`
      relationships.push({ comments: [`${TODO}: ${msg}`, 'LookML:', ...indent(lookml, 2).split('\n')] })
      report('TODO', msg)
    }
    for (const key of ['required_joins', 'outer_only']) if (join[key] !== undefined) report('DROPPED', `join ${jname}: ${key} is not needed`)
    if (join.required_access_grants) report('SECURITY', `join ${jname}: required_access_grants [${asArray(join.required_access_grants).join(', ')}] is dropped — see references/explore.md#required_access_grants-join`)
  }

  function buildRelationship (jname, J, join, comments) {
    const cardinality = join.relationship || 'many_to_one'
    const type = join.type || 'left_outer'
    if (type === 'cross') throw new Problem('`type: cross` is unsupported; redesign it, usually as a query model that contains the CROSS JOIN — see references/explore.md#type')
    if (join.sql !== undefined) throw new Problem('raw `sql` join clause: read its ON clause by the sql_on rules; `UNNEST` needs a query model — see references/explore.md#sql')
    if (join.sql_foreign_key !== undefined) throw new Problem('`sql_foreign_key`: make the SQL a hidden key dimension and relate it to the primary key of the joined view — see references/explore.md#sql_foreign_key')

    let pairs = [] // { o: FieldRef, j: FieldRef }
    const wheres = { o: [], j: [] }
    let O = null
    let match = null
    const side = v => (v === jname ? 'j' : 'o')
    const setO = v => {
      if (v === jname) return
      if (O && O.ref !== v) throw new Problem('the condition names three or more views — see references/explore.md#unsupported-shapes')
      if (!members.has(v)) throw new Problem(`\`${v}\` is not a view of this explore`)
      O = members.get(v)
    }
    const whereCond = (v, part) => {
      const m = members.get(v)
      if (!m) throw new Problem(`\`${v}\` is not a view of this explore`)
      try { return toAql(part) } catch (e) {
        if (!(e instanceof Untranslatable)) throw e
        const h = helperDim(m, `join_filter__${v === jname ? 'self' : jname}`, `Join Filter (${jname})`, 'truefalse', part)
        comments.push(`${REVIEW}: the condition \`${part.replace(/\s+/g, ' ')}\` is kept as SQL in the hidden dimension ${m.aml}.${h}`)
        return `${m.aml}.${h}`
      }
    }

    if (join.foreign_key !== undefined) {
      const [a, b] = String(join.foreign_key).includes('.') ? String(join.foreign_key).split('.') : [baseRef, String(join.foreign_key)]
      setO(a)
      if (!J.conv.pk) throw new Problem('`foreign_key` needs a `primary_key: yes` dimension in the joined view — see references/explore.md#foreign_key')
      pairs.push({ o: refField(a, b), j: { model: J.aml, field: J.conv.pk } })
    } else {
      if (join.sql_on === undefined) throw new Problem('no join condition: this is a cross join — see references/explore.md#unsupported-shapes')
      if (hasLiquid(join.sql_on)) throw new Problem('Liquid in `sql_on`; an equality with a user attribute is a row filter (a `permission` block) — see references/explore.md#unsupported-shapes')
      const sql = stripOuterParens(stripSqlComments(join.sql_on))
      const parts = splitTopLevel(sql, 'AND').map(stripOuterParens).filter(Boolean)
      let nonEquality = cardinality === 'many_to_many'
      const classified = parts.map(part => {
        const views = viewsIn(part)
        if (/\$\{\s*\w+\s*\}/.test(part)) throw new Problem('a reference without a view name in `sql_on`')
        const kp = part.match(/^\$\{\s*(\w+)\.(\w+)\s*\}\s*=\s*\$\{\s*(\w+)\.(\w+)\s*\}$/)
        if (kp && kp[1] !== kp[3]) return { kind: 'pair', a: [kp[1], kp[2]], b: [kp[3], kp[4]] }
        if (views.size === 0) return { kind: 'constant', part }
        if (views.size === 1) return { kind: 'where', view: [...views][0], part }
        if (views.size > 2) throw new Problem('a part of the condition names three or more views — see references/explore.md#unsupported-shapes')
        const eq = splitTopLevel(part, '=')
        if (eq.length === 2 && splitTopLevel(part, 'OR').length === 1 && viewsIn(eq[0]).size === 1 && viewsIn(eq[1]).size === 1) return { kind: 'exprPair', sides: eq }
        return { kind: 'nonEquality', part }
      })
      if (classified.some(c => c.kind === 'nonEquality')) nonEquality = true
      for (const c of classified) {
        if (c.kind === 'pair') { setO(c.a[0]); setO(c.b[0]) }
        if (c.kind === 'exprPair') c.sides.forEach(s => setO([...viewsIn(s)][0]))
        if (c.kind === 'nonEquality') viewsIn(c.part).forEach(setO)
      }
      const linking = classified.filter(c => ['pair', 'exprPair', 'nonEquality'].includes(c.kind))
      if (!linking.length) {
        if (/^(false|0\s*=\s*1)$/i.test(sql.trim())) throw new Problem('the condition is FALSE: keep the model in `models` with no relationship')
        throw new Problem(classified.some(c => c.kind === 'where') ? 'only a constant condition on one view: a query model filtered by the constant, then a cross join — see references/explore.md#unsupported-shapes' : 'the condition links no two views (TRUE / 1 = 1): a cross join — see references/explore.md#unsupported-shapes')
      }
      if (!O) throw new Problem('the condition does not name another view of the explore')
      for (const c of classified) if (c.kind === 'where' && c.view !== jname && c.view !== O.ref) throw new Problem('the condition names three or more views — see references/explore.md#unsupported-shapes')

      if (nonEquality) {
        match = toAql(sql)
      } else {
        for (const c of classified) {
          if (c.kind === 'pair') {
            const fa = refField(...c.a); const fb = refField(...c.b)
            if (fa.timeframe || fb.timeframe) comments.push(`${REVIEW}: a dimension-group timeframe is used as a join key through a hidden alias dimension; prefer a \`date\` base for joins — see references/dimension-group.md#referencing-the-generated-fields`)
            pairs.push(c.a[0] === jname ? { o: fb, j: fa } : { o: fa, j: fb })
          } else if (c.kind === 'exprPair') {
            const key = s => {
              const v = [...viewsIn(s)][0]; const m = members.get(v)
              const lone = s.match(/^\$\{\s*(\w+)\.(\w+)\s*\}$/)
              if (lone) return { v, f: refField(lone[1], lone[2]) }
              const other = v === jname ? O.ref : jname
              const h = helperDim(m, `join_key__${other}`, `Join Key (${other})`, 'text', s)
              comments.push(`${REVIEW}: join key expression kept as SQL in the hidden dimension ${m.aml}.${h}; its type is set to 'text' — check it`)
              return { v, f: { model: m.aml, field: h } }
            }
            const [x, y] = c.sides.map(key)
            if (x.v === y.v) throw new Problem('both sides of an equality name the same view')
            pairs.push(x.v === jname ? { o: y.f, j: x.f } : { o: x.f, j: y.f })
          } else if (c.kind === 'where') {
            wheres[side(c.view)].push(whereCond(c.view, c.part))
          }
        }
      }
    }
    if (join.sql_where !== undefined) {
      const views = viewsIn(join.sql_where)
      if (hasLiquid(join.sql_where) || views.size !== 1 || !views.has(jname) || match) {
        comments.push(`${TODO}: sql_where is not converted (it must refer to the joined view only) — see references/explore.md#sql_where`, `LookML: ${paramToLookml('sql_where', join.sql_where).replace(/\n\s*/g, ' ')}`)
        report('TODO', `join ${jname}: sql_where is not converted`)
      } else {
        wheres.j.push(whereCond(jname, stripOuterParens(stripSqlComments(join.sql_where))))
        comments.push(`${REVIEW}: sql_where removed base rows without a matching joined row; a relationship filter only limits which rows can match`)
      }
    }

    comments.unshift(`join: ${jname}${join.from ? ` (from: ${join.from})` : ''} — ${cardinality}${type !== 'left_outer' ? `, type: ${type}` : ''}`)
    if (type === 'full_outer') { comments.push(`${TODO}: full_outer is unsupported; the default relationship is emitted — see references/explore.md#type`); report('TODO', `join ${jname}: type: full_outer is unsupported`) }
    if (type === 'inner') comments.push(`${REVIEW}: type: inner → nullable: false. It states that every row has a match; it does not remove rows without one — see references/explore.md#type`)

    // ---- emit
    if (match) {
      if (!J.conv.pk || !O.conv.pk) throw new Problem(`a ${cardinality === 'many_to_many' ? 'many-to-many' : 'range'} relationship needs a primary key on both models (${[!O.conv.pk && O.aml, !J.conv.pk && J.aml].filter(Boolean).join(', ')} has none) — see references/explore.md#non-equality-conditions`)
      const m2m = cardinality === 'many_to_many'
      const [from, to] = !m2m && cardinality === 'one_to_many' ? [J, O] : [O, J]
      comments.push(`${REVIEW}: ${m2m ? 'many-to-many' : 'range'} relationship; from / to are the primary keys, the whole condition is in \`match\`.${m2m ? '' : ' A range relationship assumes at most one match per source row.'} See references/explore.md#non-equality-conditions`)
      return [
        'RelationshipConfig {',
        `  rel: ${m2m ? 'ManyToManyRelationship' : 'RangeRelationship'} {`,
        `    type: '${m2m ? 'many_to_many' : 'range'}'`,
        `    from: FieldRef { model: '${from.aml}', field: '${from.conv.pk}' }`,
        `    to: FieldRef { model: '${to.aml}', field: '${to.conv.pk}' }`,
        '    match: @aql', ...indent(match.replace(/ (and|or) /g, '\n  $1 '), 6).split('\n'), '    ;;',
        '  }',
        '  active: true',
        ...(type === 'inner' ? ['  nullable: false'] : []),
        '}'
      ]
    }
    const flip = cardinality === 'one_to_many'
    const [f, t] = flip ? ['j', 'o'] : ['o', 'j']
    const oneToOne = cardinality === 'one_to_one'
    if (!['many_to_one', 'one_to_many', 'one_to_one'].includes(cardinality)) throw new Problem(`unknown relationship \`${cardinality}\``)
    if (pairs.length === 1 && !wheres.o.length && !wheres.j.length) {
      const p = pairs[0]
      return [`relationship(${p[f].model}.${p[f].field} ${oneToOne ? '-' : '>'} ${p[t].model}.${p[t].field}, true${type === 'inner' ? ', nullable=false' : ''})`]
    }
    const ref = x => `FieldRef { model: '${x.model}', field: '${x.field}' }`
    const lines = ['RelationshipConfig {', '  rel: EqualityRelationship {', `    type: '${oneToOne ? 'one_to_one' : 'many_to_one'}'`, `    from: ${ref(pairs[0][f])}`, `    to: ${ref(pairs[0][t])}`]
    for (const p of pairs.slice(1)) lines.push('    on {', `      from: ${ref(p[f])}`, `      to: ${ref(p[t])}`, '    }')
    if (wheres.o.length || wheres.j.length) {
      lines.push('    where {')
      if (wheres[f].length) lines.push(`      from: @aql ${wheres[f].join(' and ')} ;;`)
      if (wheres[t].length) lines.push(`      to: @aql ${wheres[t].join(' and ')} ;;`)
      lines.push('    }')
    }
    lines.push('  }', '  active: true', ...(type === 'inner' ? ['  nullable: false'] : []), '}')
    return lines
  }

  // ---- fields that moved from a Model to this Dataset (they read another view) ---------------------------------------
  const movedMeasureCount = new Map()
  for (const m of members.values()) for (const [f] of m.conv.moved) if (m.conv.lkFields.get(f).kind === 'measure') movedMeasureCount.set(f, (movedMeasureCount.get(f) || 0) + 1)
  const metricName = (m, f) => (movedMeasureCount.get(f) > 1 ? `${m.ref}_${f}` : f)
  const datasetFields = []
  const emittedMoved = new Map() // member → [field]
  for (const m of members.values()) {
    for (const [fname, needs] of m.conv.moved) {
      const lkField = m.conv.lkFields.get(fname)
      const absent = [...needs].filter(v => v !== '?' && !members.has(v))
      if (absent.length) { report('INFO', `${lkField.kind} ${m.ref}.${fname} is omitted from this Dataset: it needs ${absent.join(', ')}, which the explore does not join`); continue }
      const scope = {
        self: m.aml,
        inDataset: true,
        dataset: dsName,
        ref: (view, f, opts) => {
          const target = view && view !== m.conv.lkName && view !== m.ref ? members.get(view) : m
          if (!target) throw new Untranslatable(`\`${view}\` is not a view of this explore`)
          if (target.conv.moved.has(f)) {
            const k = target.conv.lkFields.get(f).kind
            if (k === 'measure') return { kind: 'measure', aql: metricName(target, f), type: 'number', foreign: target !== m }
            if (k === 'dimension') return { kind: 'dimension', aql: `${target.aml}.${f}`, type: target.conv.dimType(target.conv.lkFields.get(f).lk), foreign: target !== m }
            throw new Untranslatable(`\${${f}} is a dimension group that reads another view`)
          }
          const r = target.conv.resolve(f, opts)
          if (r.kind === 'dimension' || r.kind === 'measure') return { ...r, aql: `${target.aml}.${r.name}`, foreign: target !== m }
          throw new Untranslatable(r.note || `\${${view ? `${view}.` : ''}${f}} is ${r.kind === 'unknown' ? 'not a field of the view' : `a ${r.kind} field`}`)
        }
      }
      let field
      if (lkField.kind === 'measure') {
        field = convertMeasure(m.conv, fname, lkField.lk, scope)
        field.name = metricName(m, fname)
      } else {
        field = m.conv.convertMovedDimension(fname, scope)
        if (field.skeleton) report('TODO', `${lkField.kind} ${m.ref}.${fname} reads another view: write it as a Dataset dimension in AQL`)
      }
      datasetFields.push(field)
      if (!field.skeleton) emittedMoved.set(m, [...(emittedMoved.get(m) || []), field])
    }
  }

  // ---- `view` block: explore / join `fields`, field `group_label`, `view_label` ---------------------------------------
  const viewBlock = buildViewBlock()
  function buildViewBlock () {
    const setOf = (conv, setName, seen = new Set()) => {
      if (seen.has(setName)) return []
      seen.add(setName)
      const set = (conv.lk.set || {})[setName]
      return set ? asArray(set.fields).flatMap(e => (/\*$/.test(e) && !e.includes('.') ? setOf(conv, e.slice(0, -1), seen) : [e])) : []
    }
    const allOf = m => [...m.conv.lkFields.keys()]
    // step 1: each join
    const exposed = new Map() // member → { all, names: Set of LookML field names }
    const apply = (state, entries, defaultMember) => {
      for (const raw of entries) {
        const negative = raw.startsWith('-')
        const e = negative ? raw.slice(1) : raw
        let m = defaultMember; let item = e
        if (e.includes('.')) { const [v, rest] = e.split('.'); m = members.get(v); item = rest }
        if (!m) continue
        const st = state.get(m) || { all: false, names: new Set() }
        state.set(m, st)
        const names = item.endsWith('*') ? setOf(m.conv, item.slice(0, -1)).map(x => (x.includes('.') ? x.split('.')[1] : x)) : [item]
        if (negative) { if (st.all) { st.all = false; st.names = new Set(allOf(m)) } names.forEach(n => st.names.delete(n)) } else names.forEach(n => st.names.add(n))
      }
    }
    for (const m of members.values()) {
      const list = m.join && m.join.fields !== undefined ? asArray(m.join.fields) : null
      if (!list) exposed.set(m, { all: true, names: new Set() })
      else { exposed.set(m, { all: false, names: new Set() }); apply(exposed, list, m) }
    }
    // step 2: the explore
    let state = exposed
    if (ex.fields !== undefined) {
      state = new Map()
      for (const raw of asArray(ex.fields)) {
        if (raw === 'ALL_FIELDS*') { for (const [m, st] of exposed) state.set(m, { all: st.all, names: new Set(st.names) }); continue }
        const viewAll = raw.match(/^(\w+)\*$/)
        if (viewAll && members.has(viewAll[1])) { const m = members.get(viewAll[1]); const st = exposed.get(m); state.set(m, { all: st.all, names: new Set(st.names) }); continue }
        apply(state, [raw], members.get(baseRef))
      }
    }
    // LookML names → visible AML fields, in model order
    const amlFields = m => {
      const st = state.get(m)
      if (!st) return { all: false, fields: [] }
      const base = m.conv.base || m.conv
      const visible = f => f && !f.props.hidden && !f.helper
      let fields
      if (st.all) fields = base.fields.filter(visible)
      else {
        const wanted = new Set()
        for (const n of st.names) {
          const emitted = base.emitted.get(base.rn(n))
          if (emitted) emitted.forEach(x => wanted.add(x))
          else { const r = base.resolve(n, { grain: true }); if (r.kind === 'dimension' || r.kind === 'measure') wanted.add(r.name) }
        }
        fields = base.fields.filter(f => visible(f) && wanted.has(f.name))
      }
      const moved = (emittedMoved.get(m) || []).filter(f => f.kind === 'dimension' && !f.props.hidden && (st.all || [...st.names].some(n => base.rn(n) === f.name)))
      return { all: st.all, fields: [...fields, ...moved] }
    }
    const perMember = [...members.values()].map(m => ({ m, ...amlFields(m) }))
    const grouped = x => x.fields.some(f => f.src && f.src.group_label)
    // view_label groups
    const labelGroups = new Map()
    for (const m of members.values()) {
      const label = m.viewLabel
      if (!label || m.isAlias) continue
      labelGroups.set(label, [...(labelGroups.get(label) || []), m])
    }
    for (const [label, ms] of labelGroups) if (ms.length === 1 && (ms[0].conv.lk.label ?? titleCase(ms[0].conv.lkName)) === label) labelGroups.delete(label)
    if (perMember.every(x => x.all) && !perMember.some(grouped) && !labelGroups.size) return []

    const modelLines = x => {
      if (!x.fields.length) return null
      if (x.all && !grouped(x)) return [`model ${x.m.aml} { }`]
      const lines = [`model ${x.m.aml} {`]
      const done = new Set()
      for (const f of x.fields) {
        const g = f.src && f.src.group_label
        if (!g) { lines.push(`  field ${f.name}`); continue }
        if (done.has(g)) continue
        done.add(g)
        lines.push(`  group ${slug(g)} {`, ...x.fields.filter(o => o.src && o.src.group_label === g).map(o => `    field ${o.name}`), '  }')
      }
      lines.push('}')
      return lines
    }
    const out = []
    const placed = new Set()
    for (const x of perMember) {
      if (placed.has(x.m)) continue
      const entry = [...labelGroups].find(([, ms]) => ms.includes(x.m))
      if (entry) {
        const inner = entry[1].map(gm => { placed.add(gm); return modelLines(perMember.find(p => p.m === gm)) }).filter(Boolean).flat()
        if (inner.length) out.push(`group ${slug(entry[0])} {`, ...inner.map(l => `  ${l}`), '}')
      } else {
        const lines = modelLines(x)
        if (lines) out.push(...lines); else report('INFO', `model ${x.m.aml} exposes no field in this explore; it is left out of the Dataset view`)
      }
    }
    if (ex.fields !== undefined || [...members.values()].some(m => m.join && m.join.fields !== undefined)) report('INFO', 'explore / join `fields` → a Dataset `view`, which only organises the field list (users can switch back). If the exclusion restricts access, use column-level permission — see references/explore.md#fields')
    return ['view {', ...out.map(l => `  ${l}`), '}']
  }

  // ---- drill_fields → context { analysis { … } } -----------------------------------------------------------------------
  const context = buildContext()
  function buildContext () {
    const breakdown = []; const underlying = []; const seenMetric = new Set()
    const dims = (m, list) => {
      const out = []
      const expand = (conv, e, seen = new Set()) => (/\*$/.test(e) && !seen.has(e) ? (seen.add(e), asArray(((conv.lk.set || {})[e.replace(/^.*\./, '').slice(0, -1)] || {}).fields).flatMap(x => expand(conv, x, seen))) : [e])
      const entries = asArray(list).flatMap(e => expand(m.conv, e))
      const removed = new Set(entries.filter(e => e.startsWith('-')).map(e => e.slice(1)))
      for (const e of entries) {
        if (e.startsWith('-') || removed.has(e)) continue
        const [v, f] = e.includes('.') ? e.split('.') : [m.ref, e]
        const target = v === m.conv.lkName ? m : members.get(v)
        if (!target) continue
        const r = target.conv.resolve(f, { grain: true })
        if (r.kind !== 'dimension') continue
        const text = `r(${target.aml}.${r.name})`
        if (!out.includes(text)) out.push(text)
      }
      return out
    }
    for (const m of members.values()) {
      for (const [fname, { kind, lk }] of m.conv.lkFields) {
        if (m.conv.moved.has(fname)) continue
        if (kind === 'dimension' && lk.drill_fields && asArray(lk.drill_fields).length) {
          const fields = [`r(${m.aml}.${fname})`, ...dims(m, lk.drill_fields).filter(x => x !== `r(${m.aml}.${fname})`)]
          if (fields.length > 1 && m.conv.resolve(fname).kind === 'dimension') breakdown.push(`group ${members.size > 1 ? `${m.aml}_` : ''}${fname}_drill {`, `  label: ${amlString(lk.label || titleCase(fname), project.constName)}`, `  fields: [${fields.join(', ')}]`, '}')
        }
        if (kind === 'measure') {
          const list = lk.drill_fields !== undefined ? lk.drill_fields : m.conv.lk.drill_fields
          if (list === undefined) continue
          const fields = dims(m, list)
          if (!fields.length) continue
          const single = asArray(list).length === 1 && /\*$/.test(asArray(list)[0]) ? asArray(list)[0].replace(/^.*\./, '').slice(0, -1) : null
          const block = [`metric ${fname} {`, `  view ${single || 'drill'} {`, `    label: ${amlString(single ? titleCase(single) : (lk.label || titleCase(fname)), project.constName)}`, `    fields: [${fields.join(', ')}]`, '  }', '}']
          if (seenMetric.has(fname)) underlying.push(`// ${TODO}: measure \`${fname}\` exists in several models of this Dataset; \`underlying_data\` names a metric without its model. This entry (model ${m.aml}) is left out:`, ...block.map(l => `// ${l}`))
          else { seenMetric.add(fname); underlying.push(...block) }
        }
      }
    }
    if (!breakdown.length && !underlying.length) return []
    report('REVIEW', 'drill_fields → `context { analysis { … } }`: a Holistics underlying-data view lists dimensions only, and whether it applies to a model measure by name is not confirmed — see references/field.md#drill_fields')
    const out = [`// ${REVIEW}: converted from drill_fields. Measures in drill lists are dropped; once a breakdown group is declared, the default list of all dimensions is replaced — see references/field.md#drill_fields`, 'context {', '  analysis {']
    if (breakdown.length) out.push('    breakdown {', ...breakdown.map(l => `      ${l}`), '    }')
    if (underlying.length) out.push('    underlying_data {', ...underlying.map(l => `      ${l}`), '    }')
    out.push('  }', '}')
    return out
  }

  // ---- permissions, filters and the parameters without an AML form ----------------------------------------------------
  const permissions = []
  const permission = (view, field, attribute, origin) => {
    try {
      const f = refField(view, field)
      permissions.push(`// ${origin}`, `permission ${view}_${field}__${attribute} {`, `  field: r(${f.model}.${f.field})`, "  operator: 'matches_user_attribute'", `  value: '${attribute}'`, '}')
      report('INFO', `row-level permission on ${view}.${field}: the user attribute \`${attribute}\` must exist in Holistics under the same name, with the users' values moved over. Holistics does not enforce row-level permissions for admins — see references/explore.md#access_filter`)
    } catch (e) {
      if (!(e instanceof Problem)) throw e
      todo(`${origin} is not converted: ${e.message} — see references/explore.md#access_filter`, null, bodyTodos)
    }
  }
  for (const af of asArray(ex.access_filter)) {
    const [v, f] = String(af.field).includes('.') ? String(af.field).split('.') : [baseRef, String(af.field)]
    permission(v, f, af.user_attribute, `access_filter: ${af.field} / user_attribute: ${af.user_attribute}`)
  }
  if (ex.access_filter_fields !== undefined) todo('access_filter_fields (removed from Looker) was a row-level restriction; review it by hand — see references/explore.md#explore-parameters-to-avoid', paramToLookml('access_filter_fields', ex.access_filter_fields))
  if (ex.sql_always_where !== undefined) {
    const ua = String(ex.sql_always_where).trim().match(/^\$\{\s*(\w+)\.(\w+)\s*\}\s*=\s*'?\{\{\s*_user_attributes\[['"](\w+)['"]\]\s*\}\}'?$/)
    if (ua) permission(ua[1], ua[2], ua[3], `sql_always_where: ${String(ex.sql_always_where).trim()}`)
    else todo('sql_always_where has no Dataset form. Pick a workaround by the shape of the condition (a wrapping query model, the condition inside the model, …) — see references/explore.md#sql_always_where', paramToLookml('sql_always_where', ex.sql_always_where))
  }
  if (ex.sql_always_having !== undefined) todo('sql_always_having is unsupported — see references/explore.md#sql_always_having', paramToLookml('sql_always_having', ex.sql_always_having))
  if (ex.always_join !== undefined) todo(`always_join: [${asArray(ex.always_join).join(', ')}] is unsupported (Holistics joins only the models a query needs). With an inner join that restricts rows, treat it like sql_always_where — see references/explore.md#always_join`)
  for (const key of ['always_filter', 'conditionally_filter']) {
    if (ex[key] === undefined) continue
    head.push(`${key}: no AML form — a default filter for the dashboards built on this dataset (references/explore.md#always_filter-conditionally_filter)`)
    for (const [field, expr] of filterPairs(ex[key].filters)) {
      let aql = ''
      try {
        const [v, f] = field.includes('.') ? field.split('.') : [baseRef, field]
        const m = members.get(v)
        const r = m && m.conv.resolve(f, { grain: true })
        if (r && r.kind === 'dimension') { const c = filterToAql(`${m.aml}.${r.name}`, r.type, expr, { caseInsensitive: lkModel.case_sensitive === false }).cond; aql = c ? `  →  ${c}` : '  →  (any value)' } else if (r && (r.kind === 'param' || r.kind === 'filter')) aql = `  →  a ${r.kind === 'param' ? 'parameter' : 'filter'} field; see references/liquid.md`
      } catch (e) { if (!(e instanceof Unsupported)) throw e }
      head.push(`  ${field}: ${JSON.stringify(expr)}${aql}`)
    }
    if (ex[key].unless) head.push(`  unless: [${asArray(ex[key].unless).join(', ')}]`)
    report('INFO', `${key} is not converted: give the dashboards on this dataset a default filter (${filterPairs(ex[key].filters).map(([f, e]) => `${f}: ${JSON.stringify(e)}`).join(', ')})`)
  }
  if (ex.hidden === true) head.push('hidden in Looker')
  if (ex.group_label !== undefined) head.push(`group_label: ${JSON.stringify(ex.group_label)} (a dataset has no grouping property)`)
  if (ex.required_access_grants !== undefined) {
    head.push(`required_access_grants: [${asArray(ex.required_access_grants).join(', ')}] — dropped (security-relevant). Share the dataset only with the matching user group.`)
    for (const g of asArray(ex.required_access_grants)) if ((lkModel.access_grant || {})[g]) head.push(...indent(toLookml('access_grant', g, lkModel.access_grant[g]), 2).split('\n'))
    report('SECURITY', `required_access_grants [${asArray(ex.required_access_grants).join(', ')}] is dropped: the dataset is visible to everyone it is shared with — see references/explore.md#required_access_grants`)
  }
  for (const [qname, q] of Object.entries(ex.query || {})) report('INFO', `query ${qname} (a Quick Start query) is not converted; rebuild it as a report`, { lookml: toLookml('query', qname, q) })
  for (const [aname, agg] of Object.entries(ex.aggregate_table || {})) report('DROPPED', `aggregate_table ${aname}: anfra has no pre-aggregates; queries run on the source tables and return the same numbers — see references/aggregate-awareness.md`, { lookml: toLookml('aggregate_table', aname, agg) })
  for (const key of ['persist_for', 'persist_with', 'case_sensitive', 'cancel_grouping_fields', 'sql_preamble', 'tags', 'sql_analytic_model_name', 'final']) {
    if (ex[key] !== undefined) report('DROPPED', `${key}: ${JSON.stringify(ex[key])} — unsupported on a Dataset`)
  }
  if (ex.symmetric_aggregates === false) report('REVIEW', '`symmetric_aggregates: no`: Looker numbers across one-to-many joins were inflated on purpose; the Dataset returns de-duplicated numbers')
  if (members.size > 1) report('INFO', 'a Dataset has no base view: a query that uses only fields of a joined model is not limited to rows reachable from the base view, as it was in Looker — see references/explore.md')

  // ---- render --------------------------------------------------------------------------------------------------------
  const rel = []
  relationships.forEach((r, i) => {
    rel.push(...r.comments.map(c => `// ${c}`))
    if (r.lines) {
      const last = relationships.slice(i + 1).every(x => !x.lines)
      rel.push(...r.lines.slice(0, -1), r.lines[r.lines.length - 1] + (last ? '' : ','))
    }
  })
  const label = ex.label ?? titleCase(name)
  const body = [
    `label: ${amlString(label, project.constName)}`,
    ...(ex.description !== undefined ? [`description: ${amlString(String(ex.description).trim(), project.constName)}`] : []),
    `data_source_name: ${project.dataSource(lkModel)}`,
    `models: [${[...members.values()].map(m => m.aml).join(', ')}]`,
    ...(rel.length ? ['relationships: [', ...rel.map(l => `  ${l}`), ']'] : ['relationships: []'])
  ]
  const section = lines => (lines.length ? ['', ...lines] : [])
  body.push(...section(commentLines(bodyTodos)))
  body.push(...section(datasetFields.flatMap((f, i) => [...(i ? [''] : []), ...ViewConv.renderField(f, project.constName)])))
  body.push(...section(permissions), ...section(viewBlock), ...section(context))
  const text = [...commentLines([...head, ...project.dataSourceNotes(lkModel)]), `Dataset ${dsName} {`, ...indent(body.join('\n'), 2).split('\n'), '}', ''].join('\n')
  return { name: dsName, path: `datasets/${lkModelName}/${dsName}.dataset.aml`, text }
}
