// Render a converted view as an AML Model file: table model, query model (derived table), alias model.
// References: view.md, derived-tables.md, explore.md#from.

import { TODO, REVIEW, asArray, titleCase, dedent, indent, amlString, toLookml, paramToLookml, commentLines } from './util.mjs'
import { ViewConv } from './view.mjs'

const hasLiquid = s => /\{%|\{\{/.test(s || '')
const NOT_AN_ALIAS = 'WHERE|JOIN|ON|LEFT|RIGHT|INNER|OUTER|FULL|CROSS|GROUP|ORDER|LIMIT|UNION|HAVING|USING|NATURAL|WINDOW|QUALIFY|EXCEPT|INTERSECT|AS|AND|OR|SELECT|FROM'
const PERSISTENCE = ['datagroup_trigger', 'sql_trigger_value', 'interval_trigger', 'persist_for', 'increment_key', 'increment_offset',
  'materialized_view', 'publish_as_db_view', 'indexes', 'sortkeys', 'distribution', 'distribution_style', 'partition_keys',
  'cluster_keys', 'table_compression', 'table_format']

/** Decide what the Model block looks like: `{ type: 'table', tableName }` or `{ type: 'query', models, query }`. */
function modelSource (conv) {
  const { lk, project } = conv
  const out = { comments: [], consts: [], lines: [] }
  const todo = (msg, lookml) => {
    out.comments.push(`${TODO}: ${msg}`)
    if (lookml) out.comments.push('LookML:', ...indent(lookml, 2).split('\n'))
    conv.report('TODO', null, msg)
  }
  const query = sql => {
    const r = conv.rewriteSql(sql, { query: true })
    let text = r.text.replace(/;\s*$/, '')
    // `{{ #v }} AS a` → `{{ #v AS a }}`: the alias moves inside the braces
    text = text.replace(new RegExp(`\\{\\{ #(\\w+) \\}\\}(\\s+(?:AS\\s+)?(?!(?:${NOT_AN_ALIAS})\\b)([A-Za-z_]\\w*))?`, 'gi'), (m, v, rest, alias) => (alias ? `{{ #${v} AS ${alias} }}` : m))
    for (const t of r.todos) todo(t)
    for (const t of new Set(r.reviews)) out.comments.push(`${REVIEW}: ${t}`)
    const missing = r.models.filter(v => !project.hasView(v))
    if (missing.length) todo(`the query references \`${missing.join('`, `')}\`, which is not a view of this project; the reference cannot be resolved — see references/derived-tables.md#references-to-other-views`)
    if (r.models.length) out.comments.push(`${REVIEW}: \`{{ #model }}\` exposes the model's dimensions by name, where Looker's \${view.SQL_TABLE_NAME} exposed the table's columns. Check every column the query reads — see references/derived-tables.md#references-to-other-views`)
    if (/^\s*with\b/i.test(text)) out.comments.push(`${REVIEW}: the query starts with WITH and becomes a CTE inside a CTE (check on SQL Server, BigQuery WITH RECURSIVE)`)
    out.type = 'query'
    out.lines = [`models: [${r.models.filter(v => project.hasView(v)).join(', ')}]`, '', ...ViewConv.renderDefinition('query', { lang: 'sql', text }).map(l => l.slice(2))]
    return r
  }

  const dt = lk.derived_table ? { ...lk.derived_table } : null
  if (dt) {
    const dtLookml = toLookml('derived_table', null, dt)
    if (lk.sql_table_name) out.comments.push(`${REVIEW}: the view has both sql_table_name and derived_table after merging its layers; the derived table is used`)
    // {% incrementcondition %} is removed with the WHERE or AND it leaves behind (derived-tables.md#persistence-parameters).
    if (typeof dt.sql === 'string' && /\{%-?\s*incrementcondition/.test(dt.sql)) {
      const IC = '\\{%-?\\s*incrementcondition\\s*-?%\\}[\\s\\S]*?\\{%-?\\s*endincrementcondition\\s*-?%\\}'
      const sql = dt.sql
        .replace(new RegExp(`\\bWHERE\\s+${IC}\\s+AND\\b`, 'gi'), 'WHERE')
        .replace(new RegExp(`\\s*\\b(?:WHERE|AND)\\s+${IC}`, 'gi'), '')
      if (!/incrementcondition/.test(sql)) { dt.sql = sql; out.comments.push(`${REVIEW}: {% incrementcondition %} was removed from the SQL, with the WHERE / AND it left behind`) }
    }
    dropPersistence(conv, dt)
    if (dt.sql) {
      query(dt.sql)
    } else {
      // No SQL to copy: a native derived table, or create_process / sql_create. A placeholder query keeps the
      // model (and every Dataset that joins it) compiling.
      out.type = 'query'
      out.lines = ['models: []', 'query: @sql SELECT 1 /* placeholder */ ;;']
      if (dt.explore_source) todo('native derived table (explore_source): it has no single target. Choose by its shape — Dataset dimensions, a window dimension, or a SQL query model — and replace or remove this placeholder model — see references/derived-tables.md#explore_source', dtLookml)
      else todo('derived table built by create_process / sql_create: write `query` (the SELECT that fills the table); drop the other statements and report them — see references/derived-tables.md#create_process-sql_create', dtLookml)
    }
    return out
  }

  const raw = lk.sql_table_name
  if (raw === undefined) {
    out.type = 'table'
    out.lines = [`table_name: ${amlString(conv.lkName)}`]
    conv.report('INFO', null, `no sql_table_name: the table is \`${conv.lkName}\`, in the default schema of the data source`)
    return out
  }
  const text = dedent(raw).trim()
  const table = t => { out.type = 'table'; out.lines = [`table_name: ${t}`]; return out }
  if (/^\(\s*(select|with)\b/i.test(text)) {
    query(text.replace(/^\(/, '').replace(/\)\s*$/, ''))
    return out
  }
  const dev = text.match(/--\s*if dev\s*--\s*([^\n]*)/i)
  const prod = text.match(/--\s*if prod\s*--\s*([^\n]*)/i)
  if (dev && prod) {
    const d = dev[1].trim(); const p = prod[1].trim()
    if (hasLiquid(d + p)) {
      todo('sql_table_name switches between dev and prod with Liquid (user attributes): declare a `const` with `if (H.git.is_production) { H.current_user.x } else { … }` and interpolate it — see references/view.md#with-user-attributes', paramToLookml('sql_table_name', raw))
      return table(amlString(p))
    }
    if (d === p) return table(amlString(p, project.constName))
    const name = `${conv.name}__table_name`
    out.consts = [`const ${name} =`, `  if (H.git.is_production) { ${amlString(p, project.constName)} }`, `  else { ${amlString(d, project.constName)} }`, '']
    out.comments.push(`${REVIEW}: Looker switches dev / prod per user session; H.git.is_production switches per workspace mode`)
    return table(name)
  }
  if (hasLiquid(text)) {
    todo('Liquid in sql_table_name: user attributes become `H.current_user.x` through a `const`; other Liquid is unsupported (use the `{% else %}` branch) — see references/view.md#sql_table_name', paramToLookml('sql_table_name', raw))
    return table(amlString(text))
  }
  if (/[()]/.test(text)) {
    todo('sql_table_name is not a table name (UNNEST, table function or expression): unsupported as a stand-alone model — see references/view.md#sql_table_name', paramToLookml('sql_table_name', raw))
    return table(amlString(text))
  }
  if (!/[.@]/.test(text)) conv.report('INFO', null, `unqualified table name \`${text}\`: it resolves against the default schema of the Holistics data source; compare with the Looker connection`)
  return table(amlString(text, project.constName))
}

// Persistence is unsupported: anfra does not persist query models and has no schedules
// (derived-tables.md#persistence-parameters). Every persistence parameter is dropped and reported with its LookML.
function dropPersistence (conv, dt) {
  const params = PERSISTENCE.filter(k => dt[k] !== undefined).map(k => [k, dt[k]])
  if (conv.lk.distkey !== undefined) params.push(['distkey', conv.lk.distkey])
  if (!params.length) return
  const dg = dt.datagroup_trigger !== undefined && (conv.lkModel.datagroup || {})[dt.datagroup_trigger]
  const lookml = [...params.map(([k, v]) => paramToLookml(k, v)), ...(dg ? [toLookml('datagroup', dt.datagroup_trigger, dg)] : [])].join('\n')
  conv.report('DROPPED', 'derived_table', `persistence (${params.map(([k]) => k).join(', ')}): anfra does not persist query models; the SQL runs inside every query that uses the model. If it is expensive, the user can build the table outside anfra and use a table model on it — see references/derived-tables.md#persistence-parameters`, { lookml })
  if (dt.publish_as_db_view === true) conv.report('INFO', 'derived_table', 'publish_as_db_view: the database view Looker kept for this derived table is gone; repoint other tools that read it')
  if (dt.materialized_view === true) conv.report('INFO', 'derived_table', 'materialized_view: this derived table is now a query model that runs its SQL in every query; or create the materialized view in the database and use a table model on it — see references/derived-tables.md#persistence-parameters')
}

/** The `.model.aml` text of a view. */
export function renderModel (conv) {
  const { lk, project } = conv
  const src = modelSource(conv)
  const label = lk.label ?? lk.view_label ?? titleCase(conv.lkName)
  // The view's LookML files are named in the `looker-source` lines that convert.mjs writes above this.
  const head = []
  if (lk.extends) head.push(`// extends: ${asArray(lk.extends).flat().join(', ')} — inherited fields are flattened into this model (references/extends-and-refinements.md)`)
  for (const key of ['suggestions', 'test', 'calendar_definition', 'derived_analytic_model', 'sql_analytic_model_name', 'final']) {
    if (lk[key] !== undefined) conv.report('DROPPED', null, `${key}: unsupported`)
  }
  if (lk.required_access_grants) {
    head.push(`// required_access_grants: [${asArray(lk.required_access_grants).join(', ')}] — dropped (security-relevant, see the conversion report)`)
    conv.report('SECURITY', null, `required_access_grants [${asArray(lk.required_access_grants).join(', ')}] is dropped: the view is visible to everyone who can open a Dataset that contains it — see references/view.md#required_access_grants`)
  }
  if (hasLiquid(label)) src.comments.push(`${TODO}: Liquid in the view label — see references/liquid.md`)
  const body = [
    `type: '${src.type}'`,
    `label: ${amlString(label, project.constName)}`,
    `data_source_name: ${project.dataSource(conv.lkModel)}`,
    ...src.lines
  ]
  const fields = conv.fields.flatMap(f => ['', ...ViewConv.renderField(f, project.constName)])
  const trailing = conv.comments.length ? ['', ...commentLines(conv.comments)] : []
  return [
    ...head,
    ...commentLines([...src.comments, ...project.dataSourceNotes(conv.lkModel)]),
    ...src.consts,
    `Model ${conv.name} {`,
    ...indent([...body, ...fields.slice(0), ...trailing].join('\n'), 2).split('\n'),
    '}',
    ''
  ].join('\n')
}

/**
 * An alias model: a view joined under another name (`from`), or with another table (`sql_table_name` on a join).
 * `extend` does not rewrite the model name inside AQL, so every @aql field is declared again with the alias name.
 */
export function renderAlias (alias) {
  const base = alias.base
  const lines = [
    `// alias of view \`${base.lkName}\` — used by ${alias.usedBy.join(', ')} (references/explore.md#from)`,
    '// @aql definitions are declared again with the alias name; @sql definitions are inherited unchanged.',
    `Model ${alias.name} = ${base.name}.extend({`,
    `  label: ${amlString(alias.label, alias.project.constName)}`
  ]
  if (alias.tableName !== undefined) {
    if (base.lk.derived_table) lines.splice(2, 0, `// ${TODO}: sql_table_name override on a derived-table view is unsupported — see references/explore.md#sql_table_name-join`)
    lines.push(`  table_name: ${amlString(dedent(alias.tableName).trim(), alias.project.constName)}`)
  }
  // Every @aql field of the view, also the helpers that explores added to it after this alias was created.
  const qualified = new RegExp(`(?<![\\w.])${base.name}\\.`, 'g')
  for (const b of base.fields) {
    if (!b.def || b.def.lang !== 'aql' || b.kind === 'param') continue
    const own = alias.byName.get(b.name)
    const f = own && own.def && own.def.lang === 'aql' ? own : { ...b, def: { lang: 'aql', text: b.def.text.replace(qualified, `${alias.name}.`) } }
    if (f.def.text === b.def.text) continue
    const def = ViewConv.renderDefinition('definition', f.def)
    if (def.length === 1) lines.push(`  ${f.kind} ${f.name} { ${def[0].trim()} }`)
    else lines.push(`  ${f.kind} ${f.name} {`, ...def.map(l => `  ${l}`), '  }')
  }
  lines.push('})', '')
  return lines.join('\n')
}
