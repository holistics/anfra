/**
 * Public types. Nothing here may leak an internal wire shape: `VizSetting` in particular is never
 * part of this surface, and the query API's payload shapes are built in `execution/submitQuery.ts`
 * rather than exposed. See docs/adr/0001.
 */
import type { FilterAggregation } from './validation';

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
}

export interface SdkEnvironment {
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** Where queries and field suggestions go. The SDK knows nothing about the server behind it. */
  backend: Backend;
  /** Absent means the host did not say, and nothing is blocked; only an explicit `false` throws. */
  features?: SdkFeatures;
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

export interface FieldSuggestionsRequest {
  /** The dataset's `name`. */
  dataset: string;
  model: string;
  field: string;
  /** What the reader has typed so far; empty for the first page of options. */
  q: string;
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
}
