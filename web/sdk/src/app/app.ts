import { Observable } from './observable';
import { Query, assertDeclaration, type QueryUpdate, type RunOutcome } from './query';
import {
  Control, DateDrillControl, Filter,
} from './controls';
import { CrossFilter, Mapping, type Edge } from './mapping';
import { DataAppError, ValidationError, isDataAppError } from '../common/errors';
import {
  assertCacheDuration, assertUniqueName, assertValidFilterAggregation, assertWeekStartDay, resolveDataset, type DatasetIndex,
} from './validation';
import {
  buildExportRequest, buildQuery, buildRequest, pageRequest, runQuery, type SubmitContext,
} from './execution/submitQuery';
import { withSuggestion } from './suggest';
import { deriveSelection } from './selection';
import { openRowsMenu } from './menu';
import type { RevealRequest } from './reveal';
import { rowMatches } from './annotations';
import type { RowsTarget } from '../common/menu';
import { applyExtensions, type EntityExtensions } from './extensions';
import { withChanges } from './changes';
import { markersFor } from './structure';
import { overlayFor, scrollTo } from './overlay';
import { reportError } from './reporting';
import type { AppContext } from './internal';
import type {
  Backend,
  BackendExportRequest,
  AppDeclaration,
  AppJson,
  DateDrillDeclaration,
  ExecuteOptions,
  ExecuteSummary,
  ExportOptions,
  ExportResult,
  FilterAggregation,
  QueryRequest,
  QueryStamp,
  CallStamp,
  ConditionValue,
  FilterDeclaration,
  InspectedApp,
  InspectedControl,
  InspectedError,
  InspectedQuery,
  InspectedSelection,
  LocateKind,
  QueryDeclaration,
  QueryInputDefaults,
  Row,
  SdkFeatures,
  Selection,
  SelectionCondition,
  WeekStartDay,
} from '../common/types';

const NO_CONDITIONS: readonly SelectionCondition[] = Object.freeze([]);

function isAbort (err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError';
}

/**
 * Whether a newer run has taken this query over, in which case that run owns the query's state and
 * the older one must not write a result.
 *
 * Asked per query rather than per run. A counter bumped by every `execute()` is close, and wrong in
 * one case: a run only ever touches the queries it re-runs, so a query a later run found clean is
 * left holding the earlier run's request. Treating that as superseded drops the result and strands
 * the query in `executing`, with nothing in flight and nothing left to move it. Taking a query over
 * means assigning it a fresh controller, so that is what this asks — the same test `fetchPage` and
 * `execute`'s own `finally` already use.
 */
function superseded (query: Query, controller: AbortController): boolean {
  return query.controller !== controller;
}

/**
 * A `DataAppError` does not survive a structured clone intact: `Error` clones as its base shape, so
 * the subclass prototype goes and its fields with it. The useful ones are written out. `cause` is
 * not: it holds whatever the backend threw, which can be a `Response` or an `AbortError`, and
 * neither can cross a port.
 */
function inspectError (error: DataAppError): InspectedError {
  return {
    name: error.name,
    code: error.code,
    scope: error.scope,
    message: error.message,
    ...(error.entity ? { entity: error.entity } : {}),
    ...(typeof error.status === 'number' ? { status: error.status } : {}),
    ...(error.requestId ? { requestId: error.requestId } : {}),
    ...(error.stack ? { stack: error.stack } : {}),
  };
}

function inspectQuery (query: Query): InspectedQuery {
  const { result } = query;
  return {
    id: query.id,
    state: query.state,
    isDirty: query.isDirty,
    page: query.page,
    hasMore: query.hasMore,
    // Metadata, never rows: rows are the one unbounded thing here, and the one thing already on
    // screen in the preview beside the panel.
    columns: result ? result.columns.map((column) => ({ ...column })) : [],
    rowCount: result?.meta.numRows ?? 0,
    selectedRowCount: query.selectedRows.length,
    signature: query.signature(),
    ...(query.error ? { error: inspectError(query.error) } : {}),
    ...(result?.debug ? { debug: { ...result.debug } } : {}),
  };
}

function inspectControl (control: Control): InspectedControl {
  const { options } = control as { options?: readonly ConditionValue[] };
  return {
    kind: control.kind,
    condition: { ...control.condition },
    ...(control.appliedCondition ? { appliedCondition: { ...control.appliedCondition } } : {}),
    isDirty: control.isDirty,
    ...(options ? { options: [...options] } : {}),
  };
}

/**
 * The rows are counted rather than carried. They are the author's own objects, verbatim, so one of
 * them may hold a DOM node or a closure a chart library put there — and a payload that will not
 * clone takes the whole snapshot with it.
 */
function inspectSelection (selection: Selection): InspectedSelection {
  return {
    source: selection.source,
    fields: [...selection.fields],
    rowCount: selection.rows.length,
    conditions: selection.conditions.map((condition) => ({ ...condition })),
    ...(selection.expression ? { expression: selection.expression } : {}),
    lossy: selection.lossy,
  };
}

/**
 * What makes one selection differ from another *for the queries*: which query it came from, since
 * that decides which targets it reaches, and the conditions. Not the rows — two clicks that derive
 * the same conditions re-run nothing.
 */
function selectionKey (selection: Selection | undefined): string {
  if (!selection) return 'none';
  // `expression` belongs here as much as `conditions` do: on the expression path `conditions` is
  // empty, so two different selections would key identically and the second would look applied.
  return JSON.stringify({
    source: selection.source,
    conditions: selection.conditions,
    expression: selection.expression,
  });
}

/**
 * A query's or an export's error, naming its query. Errors crossing the bridge lose their entity, so
 * it is put back here. A backend that rejects with anything but the SDK's error failed in a way
 * nobody classified: `call_failed`, the server's to answer for, its message kept out of the app
 * and logged to the console instead.
 */
function toDataAppError (err: unknown, entity: string, failed = 'The query failed.'): DataAppError {
  if (isDataAppError(err)) return err.about(entity);
  // The app reads only that the call failed: the console keeps what failed.
  // eslint-disable-next-line no-console -- the only place left to see a hidden error
  console.error(`anfra: '${entity}' failed; the app is told only that it failed.`, err);
  return new DataAppError({
    code: 'call_failed', scope: 'server', message: failed, entity, cause: err,
  });
}

export class App extends Observable implements AppContext {
  readonly title?: string;

  private _timezone?: string;

  private _weekStartDay?: WeekStartDay;

  /** The app's `query.default.input`: what its queries use over its own timezone and week start day. */
  private readonly queryInput: QueryInputDefaults;

  /** The cache duration of each of the app's queries that declares none (`query.default.cache.duration`). */
  readonly queryCacheDefaultDuration?: string;

  private readonly datasets: Record<string, DatasetIndex>;

  private readonly backend: Backend;

  readonly features?: SdkFeatures;

  private readonly _queries = new Map<string, Query>();

  private readonly _controls = new Map<string, Control>();

  private readonly _mappings: Mapping[] = [];

  private readonly _crossFilters: CrossFilter[] = [];

  private _selection?: Selection;

  private _appliedSelection?: Selection;

  /**
   * Whether a run has ever committed the selection, that is whether `execute()` has ever run. Needed
   * because `undefined` is a real applied value — "nothing selected" — unlike a control's condition,
   * which is always present.
   */
  private selectionCommitted = false;

  /** The id the next query created gets. */
  private nextQueryId = 1;

  constructor (
    declaration: AppDeclaration,
    datasets: Record<string, DatasetIndex>,
    backend: Backend,
    features?: SdkFeatures,
    /** @internal What plugins add to each query and control. */
    private readonly extensions?: EntityExtensions<Query, Control>,
  ) {
    super();
    this.title = declaration.title;
    this._timezone = declaration.timezone;
    assertWeekStartDay(declaration.weekStartDay, 'The app', 'weekStartDay');
    this._weekStartDay = declaration.weekStartDay;
    const queryInput = declaration.query?.default?.input ?? {};
    assertWeekStartDay(queryInput.weekStartDay, 'The app', 'query.default.input.weekStartDay');
    this.queryInput = { ...queryInput };
    const duration = declaration.query?.default?.cache?.duration;
    assertCacheDuration(duration, 'The app', 'query.default.cache.duration');
    this.queryCacheDefaultDuration = duration;
    this.datasets = datasets;
    this.backend = backend;
    this.features = features;
  }

  /* ----------------------------------------------------------------
  Declaration
  ---------------------------------------------------------------- */

  get queries (): Record<string, Query> {
    return Object.fromEntries(this._queries);
  }

  get controls (): Record<string, Control> {
    return Object.fromEntries(this._controls);
  }

  /** Every edge in the app's entity graph, of both kinds. Narrow on `kind`. */
  get mappings (): readonly Edge[] {
    return [...this._mappings, ...this._crossFilters];
  }

  get crossFilters (): readonly CrossFilter[] {
    return this._crossFilters;
  }

  createQuery (name: string, declaration: QueryDeclaration): Query {
    this.assertNameFree(name, 'query');
    const query = applyExtensions(new Query(name, this, declaration, this.nextQueryId++), this.extensions?.query);
    this._queries.set(name, query);
    // Declaring is a change too: a host inspecting the app sees the new entity at once, and a
    // dynamic app's render runs once more, which it already handles for an entity with no result.
    this.notify();
    return query;
  }

  createFilter (name: string, declaration: FilterDeclaration & { dataset?: string }): Filter {
    this.assertNameFree(name, 'filter');
    const dataset = this.filterDataset(name, declaration);
    const filter = applyExtensions(new Filter(name, this, declaration, dataset, this.backend), this.extensions?.control);
    this._controls.set(name, filter);
    this.notify();
    return filter;
  }

  createDateDrill (name: string, declaration: DateDrillDeclaration = {}): DateDrillControl {
    if (this.features?.dateDrill === false) {
      // Worth failing loudly: with the toggle off the condition still goes on the wire and the
      // server ignores it, so every query succeeds at the declared grain and the control appears
      // to do nothing.
      throw new ValidationError(
        `Date drill '${name}' needs a capability this workspace has not enabled. Ask an admin to `
        + 'turn on `interactive_control:date_drill`.',
        name,
      );
    }
    this.assertNameFree(name, 'date drill control');
    const control = applyExtensions(new DateDrillControl(name, this, declaration), this.extensions?.control);
    this._controls.set(name, control);
    this.notify();
    return control;
  }

  /**
   * Declares one edge. Always explicit — the field is never inferred from the control's own field,
   * because "the control happens to name a field this query also has" is exactly the coupling that
   * breaks silently when someone renames a dimension.
   *
   * Edges never change: to change one, `remove()` it and map again.
   */
  mapControl (
    from: Control,
    to: Query,
    options: { field: string, aggregation?: FilterAggregation },
  ): Mapping {
    if (!(from instanceof Control)) {
      throw new ValidationError(
        'mapControl expects a control as its source. To let one query cross-filter another, use '
        + 'app.mapCrossFilter(from, to) — a cross-filter edge names no field.',
      );
    }
    from.assertLive();
    to.assertLive();
    if (this._controls.get(from.name) !== from) {
      throw new ValidationError(`Control '${from.name}' belongs to a different app.`, from.name);
    }
    if (this._queries.get(to.name) !== to) {
      throw new ValidationError(`Query '${to.name}' belongs to a different app.`, to.name);
    }

    // An aggregated mapping conditions the aggregate rather than the column: `sum(orders.amount) >
    // 100`, not `orders.amount > 100`. Only the aggregation value itself can still be checked here:
    // whether `options.field` even resolves to something aggregatable is now a server-side
    // question, since a query is a raw AQL body and the SDK has no client-side field knowledge of
    // it. See docs/adr/0001.
    if (options.aggregation) {
      assertValidFilterAggregation(options.aggregation, from.name);
    }

    const mapping = new Mapping(from, to, options.field, options.aggregation, this);
    if (this._mappings.some((existing) => existing.id === mapping.id)) {
      throw new ValidationError(`Mapping '${mapping.id}' is already declared.`, from.name);
    }

    // Vacuous over control edges alone, since a control is never a target. Kept for the day an
    // edge lands *on* a control — a linked filter would — and deliberately not extended to
    // cross-filter edges, which are bidirectional by design.
    this.assertAcyclic(mapping);

    this._mappings.push(mapping);
    this.notify();
    return mapping;
  }

  /**
   * Declares that selecting rows in `from` may cross-filter `to`.
   *
   * No field: which fields a selection conditions is decided when the reader picks rows, not here.
   * Explicit, like every other edge — there is no "cross-filter everything on this dataset",
   * because an author who adds a sixth query should not silently acquire five new edges.
   *
   * Declaring both directions is normal and is not a cycle: one selection is live at a time and
   * never cascades, so `a -> b` and `b -> a` simply means either chart may drive the other.
   */
  mapCrossFilter (from: Query, to: Query): CrossFilter {
    if (!(from instanceof Query) || !(to instanceof Query)) {
      throw new ValidationError(
        'mapCrossFilter expects two queries. To let a control condition a query, use '
        + 'app.mapControl(from, to, { field }).',
      );
    }
    from.assertLive();
    to.assertLive();
    if (this._queries.get(from.name) !== from) {
      throw new ValidationError(`Query '${from.name}' belongs to a different app.`, from.name);
    }
    if (this._queries.get(to.name) !== to) {
      throw new ValidationError(`Query '${to.name}' belongs to a different app.`, to.name);
    }
    if (from === to) {
      throw new ValidationError(
        `Query '${from.name}' cannot cross-filter itself. A source keeps its own rows so it can `
        + 'show what is selected; filtering it would erase the very rows the reader clicked.',
        from.name,
      );
    }
    if (from.declaration.dataset !== to.declaration.dataset) {
      throw new ValidationError(
        `Cross-filter '${from.name}' -> '${to.name}' spans two datasets `
        + `('${from.declaration.dataset}' and '${to.declaration.dataset}'). A selection carries `
        + 'field references, which mean nothing outside the dataset that resolved them.',
        from.name,
      );
    }
    // Whether `from` actually has a selectable column can no longer be checked here: a query is a
    // raw AQL body, so its columns are only known once it has a result. A source whose columns are
    // all adhoc or all measures simply produces no selection when its rows are picked — see
    // `deriveSelection` in selection.ts. See docs/adr/0001.

    const edge = new CrossFilter(from, to, this);
    if (this._crossFilters.some((existing) => existing.id === edge.id)) {
      throw new ValidationError(`Cross-filter '${edge.id}' is already declared.`, from.name);
    }

    this._crossFilters.push(edge);
    this.notify();
    return edge;
  }

  /** @internal Backs `query.update`: checks the change against the app, then swaps it in. */
  updateQuery (query: Query, changes: QueryUpdate): void {
    const previous = query.declaration;
    const declaration = withChanges<QueryDeclaration>(previous, changes, ['dataset', 'aql', 'pageSize', 'cache']);
    assertDeclaration(query.name, declaration, 'sets');
    const dataset = this.datasetIndex(declaration.dataset, query.name);

    if (declaration.dataset !== previous.dataset) {
      const spanning = this._crossFilters.filter((edge) => {
        if (edge.from === query) return edge.to.declaration.dataset !== declaration.dataset;
        if (edge.to === query) return edge.from.declaration.dataset !== declaration.dataset;
        return false;
      });
      if (spanning.length) {
        throw new ValidationError(
          `Query '${query.name}' can't move to dataset '${declaration.dataset}': its cross-filters `
          + `${spanning.map((edge) => `'${edge.id}'`).join(', ')} would span two datasets. Remove them first.`,
          query.name,
        );
      }
    }

    // A new AQL or dataset makes new columns: a sort or a selection named the old ones.
    const reshaped = declaration.aql !== previous.aql || declaration.dataset !== previous.dataset;
    const sort = changes.sort ?? (reshaped ? [] : query.sort);
    if (reshaped) this.dropSelectionFrom(query.name);
    query.replaceDeclaration(declaration, dataset, sort);
  }

  /** @internal Backs `remove()` on a query or a control: takes it out, with its edges. */
  removeEntity (entity: Query | Control): void {
    if (entity instanceof Query) {
      if (this._queries.get(entity.name) !== entity) {
        throw new ValidationError(`Query '${entity.name}' belongs to a different app.`, entity.name);
      }
      this._queries.delete(entity.name);
      this.dropEdges((edge) => edge.to === entity || edge.from === entity);
      this.dropSelectionFrom(entity.name);
    } else {
      if (this._controls.get(entity.name) !== entity) {
        throw new ValidationError(`Control '${entity.name}' belongs to a different app.`, entity.name);
      }
      this._controls.delete(entity.name);
      this.dropEdges((edge) => edge.from === entity);
    }
    entity.markRemoved();
    this.removedListeners.forEach((listener) => {
      try { listener(entity); } catch { /* one listener must not stop the others */ }
    });
    this.notify();
  }

  /** @internal Backs `remove()` on an edge. */
  removeEdge (edge: Edge): void {
    if (!this.dropEdges((candidate) => candidate === edge)) {
      throw new ValidationError(`Edge '${edge.id}' belongs to a different app.`, edge.from.name);
    }
    this.notify();
  }

  private readonly removedListeners = new Set<(entity: Query | Control) => void>();

  /** @internal Each query or control removed from this app, for what the kernel shows about one. */
  onRemoved (listener: (entity: Query | Control) => void): () => void {
    this.removedListeners.add(listener);
    return () => { this.removedListeners.delete(listener); };
  }

  /** Takes out the edges that match; whether any did. */
  private dropEdges (matches: (edge: Edge) => boolean): boolean {
    const gone = this.mappings.filter(matches);
    gone.forEach((edge) => edge.markRemoved());
    const keep = <T extends Edge>(list: T[]) => {
      const kept = list.filter((edge) => !gone.includes(edge));
      list.splice(0, list.length, ...kept);
    };
    keep(this._mappings);
    keep(this._crossFilters);
    return gone.length > 0;
  }

  /** Clears the selection, the reader's and the applied one, when it was made in this query. */
  private dropSelectionFrom (name: string): void {
    if (this._selection?.source === name) this._selection = undefined;
    if (this._appliedSelection?.source === name) this._appliedSelection = undefined;
  }

  /* ----------------------------------------------------------------
  Selection
  ---------------------------------------------------------------- */

  /**
   * What the reader has picked, if anything.
   *
   * Held here rather than on a query because it belongs to none of them: picking rows in a second
   * query replaces it. Every control still owns its own condition — this is the one piece of
   * reader state with no natural owner.
   */
  get selection (): Selection | undefined {
    return this._selection;
  }

  /** The selection the cross-filtered queries last ran with. */
  get appliedSelection (): Selection | undefined {
    return this._appliedSelection;
  }

  clearSelection (): void {
    if (!this._selection) return;
    this._selection = undefined;
    this.notify();
  }

  /** Part of the declaration, not the environment. Changing it makes every query dirty. */
  setTimezone (timezone: string | undefined): void {
    this._timezone = timezone;
    this.notify();
  }

  get timezone (): string | undefined {
    return this._timezone;
  }

  /**
   * The app's own, like `setTimezone`: for its UI, and for its queries unless
   * `query.default.input.weekStartDay` says otherwise. Changing it makes every query it reaches dirty.
   */
  setWeekStartDay (day: WeekStartDay | undefined): void {
    assertWeekStartDay(day, 'The app', 'weekStartDay', 'sets');
    this._weekStartDay = day;
    this.notify();
  }

  /** The app's own week start day, for its UI: undefined when it declares none. */
  get weekStartDay (): WeekStartDay | undefined {
    return this._weekStartDay;
  }

  /**
   * What the app's queries get in their input when they set none: its `query.default.input`, or else
   * its own timezone and week start day. A value absent from both is left to the backend's default.
   */
  get queryInputDefaults (): QueryInputDefaults {
    const timezone = this.queryInput.timezone ?? this._timezone;
    const weekStartDay = this.queryInput.weekStartDay ?? this._weekStartDay;
    return { ...(timezone ? { timezone } : {}), ...(weekStartDay ? { weekStartDay } : {}) };
  }

  /* ----------------------------------------------------------------
  AppContext
  ---------------------------------------------------------------- */

  datasetIndex (uname: string, entity: string): DatasetIndex {
    return resolveDataset(this.datasets, uname, entity);
  }

  filterDataset (entity: string, declaration: { dataset?: string, field?: string }): DatasetIndex | undefined {
    return declaration.dataset
      ? this.datasetIndex(declaration.dataset, entity)
      : this.soleDataset(entity, declaration.field);
  }

  mappingsTo (queryName: string): Mapping[] {
    return this._mappings.filter((mapping) => mapping.to.name === queryName);
  }

  /** The expression form, set only when the structured conditions would have been lossy. */
  selectionExpressionFor (queryName: string, phase: 'pending' | 'applied'): string | undefined {
    return this.selectionFor(queryName, phase)?.expression;
  }

  private selectionFor (queryName: string, phase: 'pending' | 'applied'): Selection | undefined {
    const selection = phase === 'pending' ? this._selection : this._appliedSelection;
    if (!selection) return undefined;
    if (selection.source === queryName) return undefined;

    const reached = this._crossFilters.some(
      (edge) => edge.from.name === selection.source && edge.to.name === queryName,
    );
    return reached ? selection : undefined;
  }

  selectionConditionsFor (
    queryName: string,
    phase: 'pending' | 'applied',
  ): readonly SelectionCondition[] {
    const selection = phase === 'pending' ? this._selection : this._appliedSelection;
    if (!selection) return NO_CONDITIONS;
    // The source is never its own target: it keeps every row so the author can dim the ones the
    // reader did not pick. `mapCrossFilter` refuses the self-edge, so this is belt and braces.
    if (selection.source === queryName) return NO_CONDITIONS;

    const reached = this._crossFilters.some(
      (edge) => edge.from.name === selection.source && edge.to.name === queryName,
    );
    return reached ? selection.conditions : NO_CONDITIONS;
  }

  /**
   * The transformation each of this query's fields is currently drawn at, from the date drills
   * mapped onto it.
   *
   * `appliedCondition`, not `condition`: a drill the reader has changed but not applied has not
   * rebucketed anything yet, so the rows they are clicking still belong to the applied grain.
   */
  private drillTransformations (query: Query): Record<string, string> {
    const byField: Record<string, string> = {};

    this._mappings.forEach((mapping) => {
      if (mapping.to !== query) return;
      const condition = mapping.from.appliedCondition;
      if (condition?.operator !== 'transform_date_drill') return;

      const [value] = condition.values ?? [];
      if (typeof value === 'string') byField[mapping.field] = value;
    });

    return byField;
  }

  menu (query: Query, event: Event, rows: readonly Row[], fields?: readonly string[], measure?: string): boolean {
    if (this._queries.get(query.name) !== query) {
      throw new ValidationError(`Query '${query.name}' belongs to a different app.`, query.name);
    }
    if (measure !== undefined) {
      const columns = query.result?.columns ?? [];
      const column = columns.find((candidate) => candidate.name === measure);
      if (!column?.isMeasure) {
        throw new ValidationError(
          column
            ? `Query '${query.name}' can't open a menu on '${measure}' as its measure: it is a dimension.`
            : withSuggestion(
              `Query '${query.name}' has no measure '${measure}' in its last result.`,
              measure,
              columns.filter((candidate) => candidate.isMeasure).map((candidate) => candidate.name),
            ),
          query.name,
        );
      }
    }
    return openRowsMenu(event, this.rowsTarget(query, rows, fields, measure));
  }

  /**
   * @internal What a menu on these rows of the query is about: their conditions, derived as a
   * cross-filter's, and the annotations plugins set on them (on `measure`'s, when given).
   */
  rowsTarget (query: Query, rows: readonly Row[], fields?: readonly string[], measure?: string): RowsTarget {
    const derived = deriveSelection(query, rows, fields, this.drillTransformations(query));
    const annotations = query.annotations.filter((annotation) => (annotation.kind === 'mark' || annotation.kind === 'focus')
      && (measure === undefined || annotation.anchor.measure === undefined || annotation.anchor.measure === measure)
      && annotation.anchor.rows.some((named) => rows.some((row) => rowMatches(named, row))));
    return {
      kind: 'rows',
      app: this.index,
      query: query.name,
      rows: [...rows],
      fields: derived?.fields ? [...derived.fields] : [],
      ...(measure !== undefined ? { measure } : {}),
      conditions: derived?.conditions ? [...derived.conditions] : [],
      ...(derived?.expression ? { expression: derived.expression } : {}),
      ...(annotations.length ? { annotations } : {}),
    };
  }

  /**
   * @internal Backs the kernel's `queryRequest(query)`: what the query runs with now, from its applied
   * state, as `fetchPage` and `exportQuery` build theirs.
   */
  requestFor (query: Query): QueryRequest {
    if (this._queries.get(query.name) !== query) {
      throw new ValidationError(`Query '${query.name}' belongs to a different app.`, query.name);
    }
    return buildQuery(
      query,
      this.mappingsTo(query.name),
      {
        conditions: this.selectionConditionsFor(query.name, 'applied'),
        expression: this.selectionExpressionFor(query.name, 'applied'),
      },
      this.queryInputDefaults,
    );
  }

  select (query: Query, rows: readonly Row[], fields?: readonly string[]): void {
    if (this._queries.get(query.name) !== query) {
      throw new ValidationError(`Query '${query.name}' belongs to a different app.`, query.name);
    }

    const selection = deriveSelection(query, rows, fields, this.drillTransformations(query));

    if (selection?.lossy) {
      // Silence here would show the reader rows they did not pick and look like a data bug. See
      // docs/adr/0005 for why the wire cannot express what this selection means.
      // eslint-disable-next-line no-console -- the author needs to see this the first time it happens
      console.warn(
        `[anfra-sdk] The selection in '${query.name}' filters more broadly than it was made. `
        + 'Its conditions are combined field by field and ANDed, which cannot express '
        + '"(A and B) or (C and D)", so the targets will also show combinations that were not '
        + 'selected. Select within one dimension, or narrow with `fields`, to avoid this.',
      );
    }

    this._selection = selection;
    this.notify();
  }

  /* ----------------------------------------------------------------
  Execution
  ---------------------------------------------------------------- */

  /**
   * True when the next `execute()` would apply something: a control or the selection changed since
   * the queries reading them last ran, a query changed since its last run (edited, or an edge or a
   * control taken from it), or, once the app has run, a query was created and never run. A run in
   * flight with the current inputs is not a change, and nor is a failed run: retrying it is
   * `execute()`'s, unasked.
   */
  get hasChanges (): boolean {
    return [...this._controls.values()].some((control) => control.isDirty)
      || this.isSelectionDirty
      || [...this._queries.values()].some((query) => query.hasUnappliedChange(this.selectionCommitted));
  }

  /**
   * False before the first execution, matching `Control.isDirty`: with nothing applied there is
   * nothing to differ from, so a selection restored at startup is not an unapplied change.
   */
  private get isSelectionDirty (): boolean {
    if (!this.selectionCommitted) return false;
    return selectionKey(this._selection) !== selectionKey(this._appliedSelection);
  }

  /**
   * Runs every query that would produce something new, and resolves with which succeeded and which
   * failed. Never rejects: a failed query sets its own `state` and `error`, because one broken
   * query must not blank the other four. See docs/adr/0002. A query it would run whose run in
   * flight already has its current inputs is not sent again: this waits for that run and reports
   * how it ended. A query with nothing new to run is left alone, and not waited for, even while a
   * run of it (a `refresh()`) is in flight.
   */
  async execute (options: ExecuteOptions = {}): Promise<ExecuteSummary> {
    const targets = [...this._queries.values()].filter((query) => query.shouldRun(options.force));

    // Committing before the run, not after, so a control edited mid-flight is a change against the
    // values this run used rather than against the previous run's.
    this._controls.forEach((control) => control.commit());
    this._appliedSelection = this._selection;
    this.selectionCommitted = true;

    const context: SubmitContext = {
      backend: this.backend,
      ...this.queryInputDefaults,
      bustCache: options.bustCache,
    };

    const succeeded: string[] = [];
    const failed: string[] = [];
    await Promise.all(targets.map(async (query) => {
      const outcome = await this.runWith(query, 'pending', context, options.force);
      if (outcome === 'succeeded') succeeded.push(query.name);
      else if (outcome === 'failed') failed.push(query.name);
    }));

    return { succeeded, failed };
  }

  /**
   * @internal Backs `query.execute()`: that query alone, with what is applied, committing nothing.
   * Its signature is the applied one, so a query the reader has pending changes for stays dirty.
   */
  async executeQuery (query: Query, options: ExecuteOptions = {}): Promise<ExecuteSummary> {
    if (!query.shouldRunAlone(options.force)) return { succeeded: [], failed: [] };
    const outcome = await this.runWith(query, 'applied', {
      backend: this.backend,
      ...this.queryInputDefaults,
      bustCache: options.bustCache,
    }, options.force);
    return {
      succeeded: outcome === 'succeeded' ? [query.name] : [],
      failed: outcome === 'failed' ? [query.name] : [],
    };
  }

  /**
   * The query's run with its `phase` inputs: the one in flight when it was sent with them, so the
   * same request is not sent twice, or else a new one that supersedes whatever is in flight. With
   * `force`, or when asked to skip the server's cache, always a new one: the run in flight may be
   * answered from that cache, and the signature doesn't say.
   */
  private runWith (query: Query, phase: 'pending' | 'applied', context: SubmitContext, force = false): Promise<RunOutcome> {
    const inFlight = force || context.bustCache ? undefined : query.runInFlightWith(query.signature(phase));
    if (inFlight) return inFlight;
    // Supersede: a still-running request for a query this run is about to redo is dead weight.
    query.controller?.abort();
    query.resetPage();
    // Taken after the reset: the page is part of it, and the new run starts at the first.
    return this.run(query, query.signature(phase), context);
  }

  /**
   * One query's run from its first page, with the applied inputs, recorded under `signature`.
   * Undefined when a newer run took the query over, or it was aborted.
   */
  private run (query: Query, signature: string, context: SubmitContext): Promise<RunOutcome> {
    const controller = new AbortController();
    query.controller = controller;
    query.runningSignature = signature;
    // Recorded before `markExecuting` notifies: a listener that executes the query in reaction must
    // find this run and wait for it, not supersede it.
    let settle!: { resolve: (outcome: RunOutcome) => void; reject: (reason: unknown) => void };
    const outcome = new Promise<RunOutcome>((resolve, reject) => { settle = { resolve, reject }; });
    query.startedRun = { controller, signature, outcome };
    query.markExecuting();
    this.runFirstPage(query, signature, context, controller).then(settle.resolve, settle.reject);
    return outcome;
  }

  private async runFirstPage (
    query: Query,
    signature: string,
    context: SubmitContext,
    controller: AbortController,
  ): Promise<RunOutcome> {
    try {
      const request = buildRequest(
        query,
        this.mappingsTo(query.name),
        {
          conditions: this.selectionConditionsFor(query.name, 'applied'),
          expression: this.selectionExpressionFor(query.name, 'applied'),
        },
        context,
      );
      const { result, run } = await runQuery(query, request, context.backend, controller.signal, this.stampOf(query));
      // Also after `remove()` or `update()`, which take the controller away.
      if (superseded(query, controller)) return undefined;
      query.markSuccess(result, signature, false, run);
      return 'succeeded';
    } catch (err) {
      if (superseded(query, controller)) return undefined;
      if (isAbort(err)) return undefined;
      const error = toDataAppError(err, query.name);
      query.markError(error, false, signature);
      // A failed run is the app's to show, and its host's to know of. A failed `fetchMore` is not
      // reported here: it rejects, so the app handles it, or it is an unhandled rejection.
      reportError('call.query', error);
      return 'failed';
    } finally {
      if (query.controller === controller) {
        query.controller = undefined;
        query.runningSignature = undefined;
      }
    }
  }

  /** Re-runs everything and asks the server to skip its result cache. */
  async refresh (): Promise<ExecuteSummary> {
    return this.execute({ force: true, bustCache: true });
  }

  abort (): void {
    this._queries.forEach((query) => query.abort());
  }

  /** @internal Backs `query.fetchMore()`. Rejects, so an explicit page request reports its failure. */
  async fetchPage (query: Query): Promise<void> {
    const controller = new AbortController();
    // The applied one, as the page runs with: a change the reader has not applied keeps it dirty.
    const signature = query.signature('applied');
    query.controller = controller;
    query.runningSignature = signature;
    query.markExecuting();

    try {
      // The run continued with the next page: its input replayed, so a page fetched after midnight
      // continues the same last 7 days, and no older than its first page. Before any run came
      // back, the query as the app has it applied.
      const request = query.run
        ? pageRequest(query, query.run)
        : buildRequest(query, this.mappingsTo(query.name), {
          conditions: this.selectionConditionsFor(query.name, 'applied'),
          expression: this.selectionExpressionFor(query.name, 'applied'),
        }, { backend: this.backend, ...this.queryInputDefaults });
      const { result } = await runQuery(query, request, this.backend, controller.signal, this.stampOf(query));
      // A newer run, `remove()` or `update()` took the query over: this page is not its any more.
      if (superseded(query, controller)) return;
      query.markSuccess(result, signature, true);
    } catch (err) {
      // Its page count was reset by whatever took it over: nothing to rewind.
      if (superseded(query, controller)) return;
      query.rewindPage();
      if (isAbort(err)) return;
      const error = toDataAppError(err, query.name);
      query.markError(error, true);
      throw error;
    } finally {
      if (query.controller === controller) {
        query.controller = undefined;
        query.runningSignature = undefined;
      }
    }
  }

  /**
   * @internal Backs the `export` plugin's `query.export()`. Built from the applied state, as
   * `fetchPage` is, so the file holds what the reader is looking at, every row of it. `send` makes
   * the export (the plugin's call); this touches nothing on the query.
   */
  async exportQuery (
    query: Query,
    options: ExportOptions,
    send: (request: BackendExportRequest, signal: AbortSignal, stamp: CallStamp) => Promise<ExportResult>,
  ): Promise<ExportResult> {
    const request = buildExportRequest(
      query,
      this.mappingsTo(query.name),
      {
        conditions: this.selectionConditionsFor(query.name, 'applied'),
        expression: this.selectionExpressionFor(query.name, 'applied'),
      },
      { ...this.queryInputDefaults, title: this.title },
      options,
      // As the result on screen ran, so the file holds what the reader sees.
      query.run,
    );
    try {
      // Sent through `kernel.call`, which adds the query's id.
      return await send(request, options.signal ?? new AbortController().signal, { app: this.index, query: query.name });
    } catch (err) {
      if (isAbort(err)) throw err;
      throw toDataAppError(err, query.name, 'The export failed.');
    }
  }

  /* ----------------------------------------------------------------
  Serialisation
  ---------------------------------------------------------------- */

  /**
   * The declaration and nothing else — no datasets, no backend, no reader state. This is the
   * shape a persisted Data App record will take, which is why mapping edges are already generic.
   */
  toJSON (): AppJson {
    const byKind = <T>(kind: string): Record<string, T> => Object.fromEntries(
      [...this._controls.entries()]
        .filter(([, control]) => control.kind === kind)
        .map(([name, control]) => [name, control.toJSON() as T]),
    );

    return {
      ...(this.title ? { title: this.title } : {}),
      ...(this._timezone ? { timezone: this._timezone } : {}),
      ...(this._weekStartDay ? { weekStartDay: this._weekStartDay } : {}),
      queries: Object.fromEntries([...this._queries.entries()].map(([name, q]) => [name, q.toJSON()])),
      filters: byKind<FilterDeclaration>('filter'),
      dateDrills: byKind<DateDrillDeclaration>('dateDrill'),
      mappings: [
        ...this._mappings.map((mapping) => mapping.toJSON()),
        ...this._crossFilters.map((edge) => edge.toJSON()),
      ],
    };
  }

  /**
   * Everything a host needs to render this app, as plain data.
   *
   * Separate from `toJSON()` rather than an option on it: `toJSON()` is the declaration and is the
   * shape a persisted app takes, while this is a reading taken at a moment and already behind by
   * the time anyone sees it. Written out field by field because the live objects cannot cross a
   * port — getters are not cloned, and `Query.controller` holds an `AbortController` for exactly
   * as long as someone is likely to be watching. See docs/adr/0007.
   */
  toInspectJSON (): InspectedApp {
    return {
      ...(this.title ? { title: this.title } : {}),
      ...(this._timezone ? { timezone: this._timezone } : {}),
      ...(this._weekStartDay ? { weekStartDay: this._weekStartDay } : {}),
      ...(Object.keys(this.queryInput).length > 0 ? { queryInput: { ...this.queryInput } } : {}),
      hasChanges: this.hasChanges,
      declaration: this.toJSON(),
      queries: Object.fromEntries(
        [...this._queries.entries()].map(([name, query]) => [name, inspectQuery(query)]),
      ),
      controls: Object.fromEntries(
        [...this._controls.entries()].map(([name, control]) => [name, inspectControl(control)]),
      ),
      ...(this._selection ? { selection: inspectSelection(this._selection) } : {}),
      ...(this._appliedSelection
        ? { appliedSelection: inspectSelection(this._appliedSelection) }
        : {}),
    };
  }

  /** @internal The frame's reveal, set by its bootstrap; absent outside a frame. */
  revealer?: (request: RevealRequest) => Promise<boolean>;

  /**
   * @internal Backs `query.inspect()` and `control.inspect()`: ask the host to open its panel at
   * the entity. False outside a frame.
   */
  inspect (entity: 'query' | 'control', name: string, tab?: string): Promise<boolean> {
    if (!this.revealer) return Promise.resolve(false);
    return this.revealer({
      app: this.index, entity, name, ...(tab !== undefined ? { tab } : {}),
    });
  }

  /**
   * @internal This app's index among the frame's apps, which is how a marked element names it
   * (`data-anfra-query="1/revenue"`); 0 unless set by the instance that created it.
   */
  index = 0;

  /**
   * Bring an entity's marked elements into view: scroll to the first, show the overlay on all for
   * a moment. False, and nothing done, when the document has none marked with it (or there is no
   * document at all). Used by `Query.locate()` and `Control.locate()`, and by the bootstrap for a
   * host's locate-by-entity.
   */
  locate (kind: LocateKind, name: string): boolean {
    const doc = globalThis.document;
    if (!doc) return false;
    const elements = markersFor(doc, kind, this.index, name);
    if (!elements.length) return false;
    scrollTo(elements[0]);
    overlayFor(doc).flash(elements, {
      kind, name, ...(elements.length > 1 ? { note: `in ${elements.length} places` } : {}),
    });
    return true;
  }

  /* ----------------------------------------------------------------
  Internals
  ---------------------------------------------------------------- */

  /** What a query's calls are stamped with, so the host can show each under its query. */
  private stampOf (query: Query): QueryStamp {
    return { app: this.index, query: query.name, id: query.id };
  }

  private assertNameFree (name: string, kind: string): void {
    assertUniqueName([...this._queries.keys(), ...this._controls.keys()], name, kind);
  }

  /**
   * A field-backed filter usually lives in an app with one dataset, so requiring `dataset` on every
   * filter would be noise. With several, it has to be said.
   */
  private soleDataset (entity: string, field?: string): DatasetIndex | undefined {
    if (!field) return undefined;

    const unames = Object.keys(this.datasets);
    if (unames.length === 1) return this.datasets[unames[0]];

    throw new ValidationError(
      unames.length
        ? `Filter '${entity}' names a field but the environment has ${unames.length} datasets. `
          + `Say which one: ${unames.join(', ')}.`
        : `Filter '${entity}' names a field but the environment has no datasets.`,
      entity,
    );
  }

  private assertAcyclic (candidate: Mapping): void {
    const edges = [...this._mappings, candidate];
    const outgoing = new Map<string, string[]>();

    edges.forEach((edge) => {
      const key = edge.from.name;
      outgoing.set(key, [...(outgoing.get(key) ?? []), edge.to.name]);
    });

    const visiting = new Set<string>();
    const done = new Set<string>();

    const walk = (node: string): boolean => {
      if (visiting.has(node)) return true;
      if (done.has(node)) return false;
      visiting.add(node);
      const cycle = (outgoing.get(node) ?? []).some(walk);
      visiting.delete(node);
      done.add(node);
      return cycle;
    };

    if ([...outgoing.keys()].some(walk)) {
      throw new ValidationError(
        `Mapping '${candidate.id}' would make the app's mappings cyclic.`,
        candidate.from.name,
      );
    }
  }
}
