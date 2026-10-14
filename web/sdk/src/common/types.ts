/**
 * Public types. Nothing here may leak an internal wire shape: `VizSetting` in particular is never
 * part of this surface, and the query API's payload shapes are built in `execution/submitQuery.ts`
 * rather than exposed. See docs/adr/0001.
 */
import type { FilterAggregation } from './aggregations';

// Derived from the one runtime list `assertValidFilterAggregation` checks against, so the
// published type and the check can never disagree.
export type { FilterAggregation };

/* ----------------------------------------------------------------
Environment
---------------------------------------------------------------- */

/**
 * Metadata for one dataset, supplied by the provisioner. The SDK owns this type, so a provisioner
 * needs no particular metadata library to build one.
 */
export interface DatasetDescriptor {
  id: string | number;
  name: string;
  label?: string;
  data_models: {
    id: string | number;
    name: string;
    label?: string;
    fields: {
      name: string;
      label?: string;
      type: string;
      is_custom_measure: boolean;
      defined_in_dataset?: boolean;
    }[];
  }[];
  metrics: {
    name: string;
    label?: string;
    type: string;
  }[];
}

/**
 * Who the app is running as, supplied by the provisioner.
 *
 * Every field is copied across deliberately. The provisioner holds a much larger current-user
 * record (personal settings, client IP, shared dataset ids, tokens) and none of it belongs in
 * front of author code, so this is an allowlist rather than a projection of that record.
 *
 * Note what this is *not*: it does not authorise anything. Queries are re-authorised server-side
 * on every call, so `permissions` here is a rendering hint — an app that ignores it renders a
 * button that returns blank values, not a leak.
 */
export interface User {
  id: number;
  name: string;
  /** The external address for an embed user, whose internal one names nothing they'd recognise. */
  email: string;
  /** Free-form rather than a union: the role set is the host's to change, not this package's. */
  role: string;
  /**
   * The user's reporting timezone. Available, but not a default: an app that declares no timezone
   * still sends none and lets the server apply the tenant's. See `AppDeclaration.timezone`.
   */
  timezone: string;
  permissions: {
    canViewGeneratedSql: boolean;
    canExportData: boolean;
  };
}

/**
 * Server-side capabilities the host knows about, so a declaration can fail loudly instead of
 * silently.
 *
 * `dateDrill` mirrors `interactive_control:date_drill`: with it off the `transform_date_drill`
 * condition still reaches the server, which ignores it and answers at the declared grain. Every
 * request succeeds and the grain silently never changes.
 */
export interface SdkFeatures {
  dateDrill?: boolean;
  /**
   * Whether the host can export a query's result (`query.export()`): its Backend has
   * `exportQuery`. `mountDataApp` says so for every frame; `createSdk` reads it off the Backend
   * when the host does not. An author shows an export button when this and
   * `user.permissions.canExportData` are both true.
   */
  export?: boolean;
  /**
   * Whether `Anfra.render()` can save a rendering here: the host turned it on
   * (`mountDataApp({ render: true })`). An author shows a render button when this and
   * `user.permissions.canExportData` are both true.
   */
  render?: boolean;
  /**
   * Whether `Anfra.lineage()` can trace fields here: the host's Backend has `lineage`.
   * `mountDataApp` says so for every frame; `createSdk` reads it off the Backend when the host
   * does not.
   */
  lineage?: boolean;
}

export interface SdkEnvironment {
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** Where queries and field suggestions go. The SDK knows nothing about the server behind it. */
  backend: Backend;
  /** Absent means the host did not say, and nothing is blocked; only an explicit `false` throws. */
  features?: SdkFeatures;
  /** Draws and saves a rendering: given by the frame bootstrap, which can reach the page and the host. */
  render?: (request: RenderRequest) => Promise<Rendering>;
}

/* ----------------------------------------------------------------
Renderings
---------------------------------------------------------------- */

/** A rendering's file format. A PDF is one page the size of the rendering. */
export type RenderFormat = 'png' | 'pdf';

/**
 * What to render, and how. `block` or `container` names one node of the structure by its id;
 * neither renders the whole app.
 */
export interface RenderRequest {
  block?: string;
  container?: string;
  format: RenderFormat;
  /** The file's name, with the format's extension added when it has none. */
  filename?: string;
  /** Device pixels per CSS pixel. Defaults to the screen's, at least 2, lowered to fit a canvas. */
  pixelRatio?: number;
}

/** A picture of the app, or of one node, as the reader saw it: saved for them, and handed back. */
export interface Rendering {
  filename: string;
  format: RenderFormat;
  /** In CSS pixels: what was drawn, before `pixelRatio`. */
  width: number;
  height: number;
  blob: Blob;
  /**
   * Images left blank in the picture, by URL: their servers send no CORS headers, so the browser
   * won't let them be drawn. Their space is kept. Empty when nothing was left out.
   */
  omitted: string[];
}

/* ----------------------------------------------------------------
Conditions
---------------------------------------------------------------- */

export type ConditionValue = string | number | boolean;

/**
 * Mirrors the operator discriminator of the public query API, in full — including operators no
 * control constructs. `transform_date_drill` is what `createDateDrill` spells so an author never
 * has to; the `transform_pop_*` family has no constructor here at all and is reachable only by
 * writing the condition out, which is the point of mirroring the published list rather than the
 * subset this package happens to build.
 */
export type Operator =
  | 'is' | 'is_not' | 'is_null' | 'not_null'
  | 'greater_than' | 'less_than' | 'between'
  | 'contains' | 'does_not_contain' | 'starts_with' | 'ends_with' | 'matches'
  | 'is_true' | 'is_false'
  | 'last' | 'next' | 'before' | 'after'
  | 'matches_user_attribute'
  | 'transform_pop_relative' | 'transform_pop_absolute' | 'transform_pop_none'
  | 'transform_date_drill'
  | 'none';

export interface Condition {
  operator: Operator;
  values?: ConditionValue[];
  /** Date unit for relative operators (`last`, `next`, `transform_pop_relative`). */
  modifier?: string;
}

export type DateGrain = 'year' | 'quarter' | 'month' | 'week' | 'day' | 'hour' | 'minute';
export type DateTransformation = `datetrunc ${DateGrain}`;

export type Aggregation =
  | 'sum' | 'avg' | 'max' | 'min' | 'count' | 'count distinct' | 'median'
  | 'stdev' | 'stdevp' | 'var' | 'varp'
  | 'running sum' | 'running avg' | 'running max' | 'running min'
  | 'custom';

/* ----------------------------------------------------------------
Declaration
---------------------------------------------------------------- */

/**
 * A query is a dataset and a raw AQL body — nothing else. The SDK does not parse or validate the
 * AQL: a syntax error, an unknown field, or an ambiguous name surfaces as a `QueryError` at
 * execute, not as a `ValidationError` here. See docs/adr/0001.
 *
 * Everything the AQL text can say — dimensions, measures, adhoc fields, static filters, its own
 * sort or limit — belongs in the AQL. `pageSize` is the one exception: paging through a result is
 * a viewer interaction, not part of what the query means, so it is declared here rather than
 * re-authored on every page turn. A query that declares none isn't paged and gets every row.
 *
 * ```ts
 * app.createQuery('revenue', {
 *   dataset: 'sales',
 *   aql: `
 *     explore {
 *       dimensions { month: date_trunc(orders.created_at, "month") }
 *       measures { total: orders | sum(orders.amount) }
 *       filters { orders.status is "completed" }
 *     }
 *   `,
 * });
 * ```
 */
export interface QueryDeclaration {
  dataset: string;
  aql: string;
  /**
   * Rows per page, a whole number of at least 1. Past the first page, use `fetchMore()`. Leave it
   * out to get every row in one result. anfra can't page a pivot query (`rows { }` / `columns { }`),
   * so a pivot query must not declare one.
   */
  pageSize?: number;
}

/**
 * A sort applied to a query's result, independent of the query itself — the same mechanism a
 * click on a table's column header uses, and one that never re-runs the query against the
 * warehouse. `field` is a result column name, as `ColumnMeta.name` reports it, not a field
 * reference. See docs/adr/0001.
 */
export interface QuerySort {
  field: string;
  direction?: 'asc' | 'desc';
}

export type FilterValueType = 'string' | 'number' | 'date' | 'boolean';

export interface FilterDeclaration {
  /** Present for a field-backed filter: where options come from. Absent means a manual filter. */
  field?: string;
  /** Required for a manual filter; inferred from `field` otherwise. */
  type?: FilterValueType;
  /** Static option list for a manual filter. Absent or empty means free input. */
  options?: ConditionValue[];
  label?: string;
  default?: Condition;
}

export interface DateDrillDeclaration {
  default?: DateGrain;
  label?: string;
}

export interface AppDeclaration {
  title?: string;
  /**
   * Omitted means the server applies the tenant reporting timezone — the SDK does not guess the
   * browser's. Part of the declaration, not the environment: two apps in one host may differ.
   */
  timezone?: string;
}

/* ----------------------------------------------------------------
Results
---------------------------------------------------------------- */

/**
 * Describes one result column. Classified server-side, since the SDK never parses the AQL that
 * produced it: an adhoc field first, else a real dataset field — the same order a filter's target
 * resolves in. This is what lets a control mapping or cross-filter know which columns it can
 * target. See docs/adr/0001.
 */
export interface ColumnMeta {
  /** This column's key in each result row — see `Row`. Can differ from `fieldName`; see there. */
  name: string;
  /**
   * This column's own field name — always the real one, regardless of what `name` resolved to.
   * Paired with `modelId`, this is what a control mapping or cross-filter actually targets.
   * Diverges from `name` exactly when the compiled query declared its own alias for an existing
   * field or a transformed dimension (`orders: count_i`, or
   * `time: date_trunc(beer_orders.order_time, "month")`).
   */
  fieldName: string;
  /** The model this column's field is attached to, absent for a metric-shaped reference. */
  modelId?: string;
  label: string;
  /** True when this column is a query-local AQL expression, not a field the dataset defines. */
  adhoc: boolean;
  isMeasure: boolean;
  aggregation?: string;
  /**
   * What the column's values mean: decide by it, never by `typeof`. A `number` column's values are
   * strings with every digit (`"9007199254740993"`, `"12345.67"`, `"NaN"`). `unknown` when the
   * query's type has no meaning in AML yet. See docs/designs/result-values.md.
   */
  semantic_type: SemanticType;
  /** The Arrow type the values were read as: whether a JS number would keep every digit, say. */
  arrow_type: ArrowType;
  /** The database's own name of the type: for debugging, never to decide on. */
  db_type: string;
}

/**
 * What a column's values mean, in AML's types, plus `bit`: a true or false stored as a bit, written
 * as `true` and `false` or as `"1"` and `"0"` by how the database produced it. `unknown` for a type
 * AML gives no meaning yet.
 */
export type SemanticType = 'text' | 'number' | 'date' | 'datetime' | 'truefalse' | 'json' | 'bit' | 'unknown';

/**
 * An Arrow type: Arrow's `Type` of format/Schema.fbs as JSON. `type` is its name (`int`,
 * `floating_point`, `decimal`, `utf8`, `timestamp`, …), its other keys that type's fields in
 * snake_case (`bit_width`, `scale`, `unit`, …), and `label` how arrow-go spells it, for people.
 */
export interface ArrowType {
  type: string;
  label: string;
  [field: string]: unknown;
}

export type Row = Record<string, unknown>;

/**
 * Where a result came from.
 *
 * `executedAql` and `executedSql` are present only for readers permitted to see generated SQL —
 * absent means not permitted, rather than "there was none". `executedAt` on a cached result is
 * when the *cached* run happened, which may be hours earlier, so it should be labelled rather
 * than shown as if it just ran — the figures can be stale even though they are this reader's own.
 */
export interface QueryDebug {
  executedAql?: string;
  executedSql?: string;
  fromCache: boolean;
  executedAt?: Date;
}

export interface QueryResult {
  columns: ColumnMeta[];
  rows: Row[];
  meta: {
    /** Present only for a query that declares a `pageSize`. */
    page?: number;
    /** Present only for a query that declares a `pageSize`. */
    pageSize?: number;
    numRows: number;
  };
  debug?: QueryDebug;
}

export type QueryState = 'idle' | 'executing' | 'success' | 'error';

export interface ExecuteSummary {
  succeeded: string[];
  failed: string[];
}

export interface ExecuteOptions {
  /** Re-run every query, not only dirty ones. */
  force?: boolean;
  /** Ask the server to skip its result cache. */
  bustCache?: boolean;
}

export type Unsubscribe = () => void;
export type Listener = () => void;

/* ----------------------------------------------------------------
Selection
---------------------------------------------------------------- */

/**
 * One condition derived from a selection.
 *
 * A selection's values are never absent — a condition with nothing to match is not emitted at all —
 * and it never carries a modifier: the three operators a selection produces — `is`, `is_null` and
 * `matches` — take none.
 */
export interface SelectionCondition {
  field: string;
  operator: Operator;
  values: ConditionValue[];
}

/**
 * What the reader has picked out of one query's results, and the conditions it puts on that
 * query's cross-filter targets.
 *
 * App-scoped and singular: a selection belongs to no single query, and picking rows in a second
 * query replaces it. Held on the app for that reason alone — every control still owns its own
 * condition.
 */
export interface Selection {
  /** Name of the query the rows came from. */
  source: string;
  /** The rows as the author passed them, verbatim. */
  rows: readonly Row[];
  /**
   * The dimension aliases the conditions were derived from — every declared one, or those
   * `select` narrowed to. What makes a re-run of the source comparable against `rows`.
   */
  fields: readonly string[];
  conditions: readonly SelectionCondition[];
  /**
   * Set instead of `conditions` when the field-by-field collapse would have been lossy: one AQL
   * condition saying exactly what the reader picked, as an OR over the rows.
   *
   * Only reached this way. The structured form is the proven path and stays in charge whenever it
   * can say what the selection means, which is every single-dimension and every contiguous case.
   */
  expression?: string;
  /**
   * True when the conditions sent admit rows the reader did not pick, or condition on less than
   * they picked.
   *
   * Two causes, and only one is fixable. A field-by-field collapse over-selects — picking
   * `(APAC, Jan)` and `(EMEA, Feb)` ANDs to `region IN (APAC, EMEA) AND month IN (Jan, Feb)`,
   * which also admits `(APAC, Feb)` — and that case is sent as an `expression` instead, so it is
   * not lossy. What survives is a dropped query-local column: an adhoc field exists only in this
   * query's result, so no condition of any shape can name it in the target. Reported rather than
   * hidden, because otherwise it reads as a data bug.
   */
  lossy: boolean;
}

/* ----------------------------------------------------------------
Serialised form
---------------------------------------------------------------- */

export type EntityKind = 'query' | 'filter' | 'dateDrill';

export interface EntityRef {
  kind: EntityKind;
  name: string;
}

/**
 * One directed edge in the app's entity graph, tagged by kind.
 *
 * A single list rather than one per kind, mirroring Dashboard as Code's `definition.interactions`,
 * which carries all five of its interaction kinds discriminated by `type`. The tag is what makes
 * that workable: a consumer branches explicitly instead of inferring the kind from which optional
 * fields happen to be present. See docs/adr/0005.
 */
/** A control conditioning one field of one query. */
export interface ControlMappingJson {
  kind: 'control';
  id: string;
  from: EntityRef;
  to: EntityRef;
  field: string;
  aggregation?: FilterAggregation;
}

/**
 * A query that may cross-filter another. Deliberately carries no field: which fields a selection
 * conditions is decided when the reader clicks, not when the edge is declared — the same asymmetry
 * that makes `CrossFilterInteraction` the one Dashboard as Code interaction without a `field_path`.
 */
export interface CrossFilterJson {
  kind: 'crossFilter';
  id: string;
  from: EntityRef;
  to: EntityRef;
}

export type MappingJson = ControlMappingJson | CrossFilterJson;

export interface AppJson {
  title?: string;
  timezone?: string;
  queries: Record<string, QueryDeclaration>;
  filters: Record<string, FilterDeclaration>;
  dateDrills: Record<string, DateDrillDeclaration>;
  mappings: MappingJson[];
}

/* ----------------------------------------------------------------
Inspection
---------------------------------------------------------------- */

/**
 * An app's declaration and runtime state as plain data, for a host to render outside the sandbox.
 *
 * Built by reading the live objects rather than serialising them. Everything worth showing sits
 * behind a getter, and getters do not survive a structured clone; the objects holding them carry
 * an `AbortController`, a backend and a bag of closures, which do not survive one either. So this
 * is written out field by field, and stays plain all the way down. See docs/adr/0007.
 */
export interface InspectedError {
  name: string;
  message: string;
  /** The query or control the error is about. Lost if the error itself is cloned. */
  entity?: string;
  status?: number;
  stack?: string;
}

/** The two kinds of entity a marked element can be drawn by. */
export type LocateKind = 'query' | 'control';

/* ----------------------------------------------------------------
Structure: the semantic parts a definition marks on its own elements
---------------------------------------------------------------- */

export type StructureKind = 'container' | 'block' | LocateKind;

/**
 * One node of the structure. A container or block has an `id`; a query or control node has the
 * entity's `app` index and `name`. `key` is the node's index path in the tree (`0.2.1`), what a
 * host uses to point at it; it is stable while the tree is unchanged.
 */
export interface StructureNode {
  key: string;
  kind: StructureKind;
  id?: string;
  label?: string;
  app?: number;
  name?: string;
  /** What is wrong with this node's markup, in words. Reported, never thrown. */
  problems: string[];
  children: StructureNode[];
}

/** Where one entity is drawn: how many marked elements, and which blocks they are in. */
export interface EntityUsage {
  markers: number;
  blocks: string[];
}

export interface InspectedStructure {
  nodes: StructureNode[];
  /** Keyed `query:<app>/<name>` or `control:<app>/<name>`; only entities that exist. */
  usage: Record<string, EntityUsage>;
}

/** What a host asks a frame to locate: a node of the structure, or an entity wherever it is drawn. */
export type LocateTarget =
  | { node: string }
  | { app: number, kind: LocateKind, name: string };

export interface InspectedQuery {
  state: QueryState;
  /** True when re-running would produce something new. */
  isDirty: boolean;
  page: number;
  hasMore: boolean;
  /** Column metadata only. Rows are deliberately absent — see docs/adr/0007. */
  columns: ColumnMeta[];
  rowCount: number;
  selectedRowCount: number;
  /** Everything that decides whether this query re-runs, which is the question a stale panel asks. */
  signature: string;
  error?: InspectedError;
  debug?: QueryDebug;
}

export interface InspectedControl {
  kind: EntityKind;
  /** What the reader has set. */
  condition: Condition;
  /** What the query last ran with, absent before the first execution. */
  appliedCondition?: Condition;
  isDirty: boolean;
  /** Field-backed filters only, and only once loaded. */
  options?: ConditionValue[];
}

export interface InspectedSelection {
  source: string;
  fields: readonly string[];
  rowCount: number;
  conditions: readonly SelectionCondition[];
  expression?: string;
  /** Surfaced here because it otherwise only ever reaches a `console.warn`. */
  lossy: boolean;
}

export interface InspectedApp {
  title?: string;
  timezone?: string;
  hasChanges: boolean;
  declaration: AppJson;
  queries: Record<string, InspectedQuery>;
  controls: Record<string, InspectedControl>;
  selection?: InspectedSelection;
  appliedSelection?: InspectedSelection;
}

/* ----------------------------------------------------------------
Backend
---------------------------------------------------------------- */

/** One control-mapping or cross-filter condition on a query, as structured data. */
export interface BackendFilter {
  /** `model.field` for a dataset field, or a bare name for a dataset metric. */
  field: string;
  operator: Operator;
  values: ConditionValue[];
  /** Date unit for relative operators (`last`, `next`). */
  modifier?: string;
  /** Present on an aggregated mapping: the condition applies to this aggregate of `field`. */
  aggregation?: FilterAggregation;
}

/**
 * An AQL condition applied on top of the query: the escape hatch for a cross-filter selection the
 * ANDed `filters` cannot express, such as `(A and B) or (C and D)`. Generated, never author-written.
 */
export interface BackendCondition {
  expr: string;
}

/** A reader's sort, naming a result column. Replaces the query's own `sorts` when present. */
export interface BackendSort {
  field: string;
  direction: 'asc' | 'desc';
}

/** Redraw every dimension built directly on `field` at `grain`, keeping its alias. Not a filter. */
export interface BackendDateDrill {
  field: string;
  grain: DateGrain;
}

/** The structured additions a query carries on this run, applied on top of its AQL. */
export interface QueryInput {
  filters: BackendFilter[];
  conditions: BackendCondition[];
  sorts: BackendSort[];
  dateDrills: BackendDateDrill[];
}

export interface BackendQueryRequest {
  /** The dataset's `name`, as in `DatasetDescriptor.name`. */
  dataset: string;
  /** The query's AQL, verbatim. */
  aql: string;
  input: QueryInput;
  /** 1-based. Sent together with `pageSize`, and only for a query that declares one. */
  page?: number;
  /** Absent for an unpaged query: the backend returns every row. */
  pageSize?: number;
  /** Absent when the app declares none, leaving the backend's default. */
  timezone?: string;
  /** Present on `refresh()`: skip any result cache. */
  bustCache?: boolean;
}

/** One page of a query's rows. `values` is positional: each row lines up with `columns`. */
export interface BackendQueryResult {
  columns: ColumnMeta[];
  values: unknown[][];
  meta?: {
    page?: number;
    pageSize?: number;
    numRows?: number;
  };
  debug?: QueryDebug;
}

/** A file format an export can be written in. CSV is the only one yet; a server may add more. */
export type ExportFormat = 'csv' | (string & {});

/** A format's options. Each applies to the formats it names; the server refuses one given to another. */
export interface ExportFormatOptions {
  /** csv: the header row: the columns' labels (the default), their names, or none. */
  header?: 'labels' | 'names' | 'none';
  /** csv: start the file with a UTF-8 byte-order mark, which Excel needs to read non-ASCII text. */
  bom?: boolean;
}

/** What `query.export()` takes. Every field is optional. */
export interface ExportOptions {
  /**
   * The file's name, with the format's extension added when it has none. Defaults to the app's
   * title and the query's name (`Sales - revenueByMonth.csv`), or the query's name alone.
   */
  filename?: string;
  /** `csv` unless given. */
  format?: ExportFormat;
  /** For CSV, `bom` defaults to `true`: a person opens what a Data App exports. */
  formatOptions?: ExportFormatOptions;
  /** Cancels the export. `query.abort()` does not: an export is not the query's run. */
  signal?: AbortSignal;
}

/**
 * A query's whole result as a file: what the Backend made, and the host has already started
 * saving for the reader. `url` works with a plain `GET` until `expiresAt`.
 */
export interface ExportResult {
  url: string;
  filename: string;
  format: ExportFormat;
  /** The rows the file holds. */
  rowCount: number;
  expiresAt: Date;
}

/**
 * An export: the same query a run sends, built by the same code so the two cannot drift, but with
 * no page (an export is every row) and no cache to bust; plus the file's format and name.
 */
export interface BackendExportRequest {
  /** The dataset's `name`, as in `DatasetDescriptor.name`. */
  dataset: string;
  /** The query's AQL, verbatim. */
  aql: string;
  input: QueryInput;
  /** Absent when the app declares none, leaving the backend's default. */
  timezone?: string;
  format?: ExportFormat;
  formatOptions?: ExportFormatOptions;
  filename?: string;
}

export interface FieldSuggestionsRequest {
  /** The dataset's `name`. */
  dataset: string;
  model: string;
  field: string;
  /** What the reader has typed so far; empty for the first page of options. */
  q: string;
}

/** What `Anfra.lineage()` takes besides its fields. Every field is optional. */
export interface LineageOptions {
  /** At most this many upstream entities per field. All of them unless given. */
  limit?: number;
  /** Skip this many upstream entities first, to page with `limit`. */
  offset?: number;
  /** Cancels the call. */
  signal?: AbortSignal;
}

/** A field to trace: its parent, and its name in it. */
export interface LineageTarget {
  /** The fqn of what holds the field: a model, or a dataset. */
  parent: string;
  /** The field's name in its parent: a dimension, a measure or a metric. */
  child: string;
}

/** A lineage call: the fields to trace, and the page of what each is built from. */
export interface BackendLineageRequest {
  targets: LineageTarget[];
  limit?: number;
  offset?: number;
}

/** Each field's lineage, in the order asked. */
export interface LineageResult {
  results: FieldLineage[];
}

/** One field's lineage, or why it could not be traced: exactly one of `diagram` and `error`. */
export interface FieldLineage {
  /** The field, as asked. */
  target: LineageTarget;
  /** The catalog search its lineage is. */
  query: string;
  diagram?: LineageDiagram;
  error?: string;
}

/**
 * A field's lineage, laid out to be drawn, in the API's own names: cards (the entities drawn at the
 * top level, their children nested in `sections`) in layers, upstream first, linked by what uses
 * what. `schema` says which entity types it has and how they connect.
 */
export interface LineageDiagram {
  version: 1;
  query: string;
  /** How many entities the search matched, over every page. */
  total: number;
  /** The ids of this page's matched entities. */
  matched: string[];
  schema: {
    /** Edge type → what it means. */
    edges: Record<string, string>;
    /** Entity type → edge type → the entity types it links to; `top_level: true` for a type no section holds. */
    types: Record<string, Record<string, unknown>>;
  };
  layers: { index: number, types: string[], card_ids: string[] }[];
  /** Every card, in layer order. */
  cards: LineageCard[];
}

/** An entity of a lineage diagram. */
export interface LineageNode {
  /** The catalog's id of the entity. */
  id: string;
  /** The catalog's entity type, such as `aml.model` or `aml.metric`. */
  type: string;
  label: string;
  /** The entity this node is a copy of. */
  ref?: string;
  tags?: string[];
  props?: Record<string, unknown>;
  /** Edge type → the entities it links to: an id, or an id with the edge's properties. */
  links?: Record<string, (string | { id: string, props: Record<string, unknown> })[]>;
  /** The node's children, grouped by type. */
  sections?: { type: string, nodes: LineageNode[] }[];
}

/** A node drawn at the top level, in its layer. */
export interface LineageCard extends LineageNode {
  layer: number;
  /** Its type is not a top-level one, but its container is not in the lineage. */
  detached?: boolean;
}

/**
 * What the SDK sends queries to, supplied by the provisioner. Each call either resolves or
 * rejects; how the backend gets there (synchronously, as polled jobs, over a bridge) is its own
 * business. A call whose `signal` aborts may reject with an `AbortError`, and its result is
 * ignored either way. Reject with a `DataAppError` (`PermissionError`, `QueryError`, ...) to choose
 * the error an author sees; anything else surfaces as a `QueryError`.
 */
export interface Backend {
  submitQuery (request: BackendQueryRequest, signal: AbortSignal): Promise<BackendQueryResult>;
  fieldSuggestions (request: FieldSuggestionsRequest, signal: AbortSignal): Promise<ConditionValue[]>;
  /**
   * Export a query's whole result as a file, and answer its link once the file is ready. Optional:
   * a host that cannot export leaves it out, and `query.export()` rejects with a `TransportError`.
   */
  exportQuery? (request: BackendExportRequest, signal: AbortSignal): Promise<ExportResult>;
  /**
   * Trace fields through the catalog: what each is built from. Optional: a host that cannot leaves
   * it out, and `Anfra.lineage()` rejects with a `TransportError`.
   */
  lineage? (request: BackendLineageRequest, signal: AbortSignal): Promise<LineageResult>;
}
