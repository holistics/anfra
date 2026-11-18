// Stable kind ids for TODO / REVIEW comments and report entries.
//
// Each comment becomes `TODO(lookml-to-aml:<kind>): …`, so the AI pass can count, grep and bulk-resolve one kind at a
// time (`grep -rn 'REVIEW(lookml-to-aml:quoted-identifier)'`). Kinds are matched on the message text in one place,
// here, instead of at the ~150 places that write messages. A message that matches no pattern gets the kind `other`;
// convert.mjs prints a warning for it, so a new message is noticed and given a kind.
//
// Keep ids short, kebab-case and stable: work-items.json, decisions.md files and agent briefs refer to them.
// Order matters: the first matching pattern wins.

import { TODO, REVIEW } from './util.mjs'

/** @type {Array<[string, RegExp]>} */
const TODO_KINDS = [
  ['parse-error', /^LookML parse error/],
  ['duplicate-view', /views are named `/],
  ['data-source', /^data_source_name is a placeholder/],
  ['constant-liquid', /^constant \S+ contains Liquid|^the value contains Liquid/],
  // table / derived table
  ['dev-prod-table', /^sql_table_name switches between dev and prod/],
  ['liquid-table-name', /^Liquid in sql_table_name/],
  ['table-name-expression', /^sql_table_name is not a table name/],
  ['table-override-derived', /sql_table_name override on a derived-table view/],
  ['ndt', /^native derived table/],
  ['create-process', /create_process|sql_create/],
  ['query-missing-view', /^the query references .* which is not a view/],
  // fields
  ['measure-to-aql', /^translate this measure to AQL/],
  ['aql-ref-in-sql', /@sql cannot reference an @aql field/],
  ['bare-table', /^bare `\$\{TABLE\}`/],
  ['own-table-ref', /SQL_TABLE_NAME/],
  ['extended-missing', /`\$\{EXTENDED\}`: the SQL of the layer below was not found/],
  ['liquid-label', /^Liquid in (label|the view label)/],
  ['liquid-sql', /^Liquid in (the SQL|the measure SQL)/],
  ['measure-filter', /^filter .* is NOT in the where\(\)/],
  ['measure-type', /^unknown measure type/],
  ['incomplete-field', /^`type: \w+` without `/],
  ['mixed-view-sql', /^the SQL mixes fields of several views/],
  ['cross-view-field', /reads another view(, so it must be| and cannot be|: write it as)/],
  ['primary-key-cross-view', /^the primary key .*reads another view/],
  ['filter-field', /^filter field .*(is )?not emitted/],
  ['param-outside-view', /^(this param is )?read from outside its view/],
  ['param-or-filter-ref', /points at a (parameter|filter) field/],
  ['location-ref', /points at a location dimension/],
  ['missing-field-ref', /is not a field of this view|^unknown reference/],
  ['timeframe-missing', /names a dimension group without a timeframe/],
  ['timeframe-unsupported', /timeframe .* (is unsupported in AQL|is not emitted)|with sub-day timeframes is unsupported/],
  ['duration-group', /^duration group without/],
  ['tier-format', /^Looker formats the numbers inside tier labels/],
  // datasets
  ['join-missing-view', /^join \S+: its view is not a view of the project/],
  ['join-multi-view', /^join \S+: no relationship is emitted — (a part of )?the condition names three or more views/],
  ['join-needs-pk', /^join \S+: no relationship is emitted — a (range|many-to-many) relationship needs a primary key/],
  ['join-liquid', /^join \S+: no relationship is emitted — Liquid/],
  ['join-constant', /^join \S+: no relationship is emitted — (the condition links no two views|the condition is FALSE|only a constant condition)/],
  ['join-key-field', /^join \S+: no relationship is emitted — .*(is a field that reads another view|is not a field of the view)/],
  ['join-unsupported', /^join \S+: no relationship is emitted — .* is unsupported/],
  ['join-sql', /^join \S+: no relationship is emitted/],
  ['join-sql-where', /sql_where is not converted/],
  ['join-full-outer', /full_outer is unsupported/],
  ['always-join', /^always_join/],
  ['sql-always-where', /^sql_always_where/],
  ['duplicate-metric', /exists in several models of this Dataset/],
  ['base-view-missing', /^base view .* is not a view of the project/],
  // last: SQL that the translator could not read, without more context
  ['sql-untranslated', /is not translated|^unexpected|^bare identifier|^expected /]
]

/** @type {Array<[string, RegExp]>} */
const REVIEW_KINDS = [
  ['quoted-identifier', /^quoted column identifier/],
  ['yesno-column', /^a yesno over a plain column/],
  ['untyped-column', /over a column whose type LookML does not state/],
  ['helper-dim-type', /^helper dimension for measure/],
  ['number-helper', /LookML declares no number type/],
  ['moved-cross-view', /^moved from view/],
  ['written-in-aql', /^written in @aql because/],
  ['aql-aggregate', /^a raw SQL aggregate inside the measure/],
  ['foreign-aggregate', /^aggregate over a dimension of another model/],
  ['foreign-filter', /^a filter condition on a field of another model/],
  ['distinct-one-value', /^one value per key is assumed/],
  ['no-pk', /no primary key/],
  ['division', /^plain division/],
  ['median', /^median over an even number/],
  ['percentile', /percentile rule/],
  ['approx-count-distinct', /^approx_count_distinct/],
  ['list-measure', /separator and order of a list/],
  ['running-total', /running_total/],
  ['percent-of-total', /pivoted equivalent of a row direction/],
  ['percent-of-previous', /percent_of_previous/],
  ['period-over-period', /period_over_period/],
  ['filters-ignored', /`filters` on a non-aggregate measure/],
  ['fiscal', /^fiscal dimensions/],
  ['timeframe-in-expr', /dimension-group timeframe inside a larger expression/],
  ['text-sort', /^text sorts alphabetically/],
  ['date-format', /date_format patterns/],
  ['date-diff-days', /date_diff/],
  ['week-start', /day-of-week numbering/],
  ['sub-day-bucket', /bucket/],
  ['duration-rounding', /rounding of durations|^duration dimensions of group/],
  ['suffix-dropped', /suffix is dropped/],
  ['tier-labels', /^tier label text/],
  ['duplicate-case-label', /^duplicate case label/],
  ['not-a-field', /is not a field of this view/],
  ['extended-replaced', /`\$\{EXTENDED\}` was replaced/],
  ['cte-in-cte', /starts with WITH/],
  ['model-ref-columns', /`\{\{ #model \}\}` exposes/],
  ['incrementcondition', /incrementcondition/],
  ['table-and-derived', /both sql_table_name and derived_table/],
  ['dev-prod-switch', /switches dev \/ prod/],
  ['override-required', /override_required/],
  ['relationship-match', /^(many-to-many|range) relationship; from \/ to are the primary keys/],
  ['join-key-sql', /kept as SQL in the hidden dimension/],
  ['join-timeframe-key', /timeframe is used as a join key/],
  ['inner-join', /^type: inner/],
  ['sql-where-removed', /^sql_where removed/],
  ['negated-filter-null', /negated filter value/],
  ['drill-fields', /drill_fields/],
  ['symmetric-aggregates', /symmetric_aggregates/],
  ['duplicate-declaration', /is declared \d+ times|declares \S+ `\S+` \d+ times/],
  ['renamed-field', /is not a valid AML name/],
  ['view-forms-differ', /different final form in model file|different connections/]
]

/** Kinds of report-only entries (INFO = outside the AML code, SECURITY = access rules). */
const NOTE_KINDS = [
  ['access-grant', /required_access_grants/],
  ['cache', /^persist_(for|with)/],
  ['week-start-day', /^week_start_day/],
  ['case-sensitive', /case_sensitive/],
  ['folder-label', /^label: .* has no AML form/],
  ['fiscal-offset', /fiscal_month_offset/],
  ['row-level-permission', /row-level permission|access_filter/],
  ['default-filter', /always_filter|conditionally_filter/],
  ['quick-start-query', /Quick Start query/],
  ['param-default', /must become the default of the dashboard filter/],
  ['no-base-view', /has no base view/],
  ['field-omitted', /is omitted from this Dataset|exposes no field in this explore/],
  ['fields-list', /`fields` → a Dataset `view`/],
  ['data-source-mapping', /no Holistics data source was given/],
  ['default-schema', /no sql_table_name|unqualified table name/],
  ['persistence', /derived table|publish_as_db_view|materialized_view|datagroup|sql_trigger_value/],
  ['location', /type: location/],
  ['imported-project', /cannot import a project/]
]

/** Reference for kinds whose messages do not name one. */
export const DEFAULT_REFERENCE = {
  'quoted-identifier': 'references/sql-references.md#table',
  'helper-dim-type': 'references/measure.md#sql',
  'number-helper': 'references/measure.md#sql',
  'aql-aggregate': 'references/measure.md#number',
  'duplicate-metric': 'references/field.md#drill_fields',
  'text-sort': 'references/dimension-group.md#date-timeframes',
  'relationship-match': 'references/explore.md#non-equality-conditions',
  'join-missing-view': 'references/explore.md#join',
  'join-key-sql': 'references/explore.md#equality-on-a-sql-expression'
}

const match = (table, message) => (table.find(([, re]) => re.test(message)) || ['other'])[0]

/** Kind of a message, by level ('TODO', 'REVIEW', 'INFO', 'SECURITY', 'DROPPED'). */
export function kindOf (level, message) {
  const m = String(message)
  if (level === 'TODO') return match(TODO_KINDS, m)
  if (level === 'REVIEW') return match(REVIEW_KINDS, m)
  if (level === 'INFO' || level === 'SECURITY') return match(NOTE_KINDS, m)
  return 'dropped'
}

const MARKER = /(TODO|REVIEW)\(lookml-to-aml\): (.*)/g

/**
 * Tag every marker in an AML file: `TODO(lookml-to-aml): msg` → `TODO(lookml-to-aml:<kind>): msg`.
 * `unknown` collects messages that matched no kind.
 */
export function tagComments (text, unknown = new Set()) {
  return text.replace(MARKER, (m, level, message) => {
    const kind = kindOf(level, message)
    if (kind === 'other') unknown.add(`${level}: ${message.slice(0, 120)}`)
    return `${level}(lookml-to-aml:${kind}): ${message}`
  })
}

/** Matches a tagged marker; groups: level, kind, message. */
export const TAGGED = /(TODO|REVIEW)\(lookml-to-aml:([\w-]+)\): (.*)/

export { TODO, REVIEW }
