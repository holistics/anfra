import { Observable } from './observable';
import { Query } from './query';
import {
  Control, DateDrillControl, Filter,
} from './controls';
import { CrossFilter, Mapping, type Edge } from './mapping';
import { DataAppError, QueryError, ValidationError } from './errors';
import {
  assertUniqueName, assertValidFilterAggregation, resolveDataset, type DatasetIndex,
} from './validation';
import { runQuery, type SubmitContext } from './execution/submitQuery';
import { deriveSelection } from './selection';
import type { AppContext } from './internal';
import type {
  Backend,
  AppDeclaration,
  AppJson,
  DateDrillDeclaration,
  ExecuteOptions,
  ExecuteSummary,
  FilterAggregation,
  ConditionValue,
  FilterDeclaration,
  InspectedApp,
  InspectedControl,
  InspectedError,
  InspectedQuery,
  InspectedSelection,
  QueryDeclaration,
  Row,
  SdkFeatures,
  Selection,
  SelectionCondition,
} from './types';

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
 * the subclass prototype goes and `entity`, `status` and `cause` go with it. `entity` and `status`
 * are the useful half, so they are written out. `cause` is not: it holds whatever the backend
 * threw, which can be a `Response` or an `AbortError`, and neither can cross a port.
 */
function inspectError (error: DataAppError): InspectedError {
  return {
    name: error.name,
    message: error.message,
    ...(error.entity ? { entity: error.entity } : {}),
    ...(typeof (error as { status?: number }).status === 'number'
      ? { status: (error as { status?: number }).status }
      : {}),
    ...(error.stack ? { stack: error.stack } : {}),
  };
}

function inspectQuery (query: Query): InspectedQuery {
  const { result } = query;
  return {
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

function toDataAppError (err: unknown, entity: string): DataAppError {
  if (err instanceof DataAppError) return err;
  return new QueryError((err as Error)?.message ?? 'The query failed.', entity, err);
}

export class App extends Observable implements AppContext {
  readonly title?: string;

  private _timezone?: string;

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
   * Whether a run has ever committed the selection. Needed because `undefined` is a real applied
   * value — "nothing selected" — unlike a control's condition, which is always present.
   */
  private selectionCommitted = false;

  constructor (
    declaration: AppDeclaration,
    datasets: Record<string, DatasetIndex>,
    backend: Backend,
    features?: SdkFeatures,
  ) {
    super();
    this.title = declaration.title;
    this._timezone = declaration.timezone;
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
    const query = new Query(name, this, declaration);
    this._queries.set(name, query);
    return query;
  }

  createFilter (name: string, declaration: FilterDeclaration & { dataset?: string }): Filter {
    this.assertNameFree(name, 'filter');
    const dataset = declaration.dataset
      ? this.datasetIndex(declaration.dataset, name)
      : this.soleDataset(name, declaration.field);
    const filter = new Filter(name, this, declaration, dataset, this.backend);
    this._controls.set(name, filter);
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
    const control = new DateDrillControl(name, this, declaration);
    this._controls.set(name, control);
    return control;
  }

  /**
   * Declares one edge. Always explicit — the field is never inferred from the control's own field,
   * because "the control happens to name a field this query also has" is exactly the coupling that
   * breaks silently when someone renames a dimension.
   *
   * There is no `unmap`: a mapping you do not want is one you do not declare.
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

    const mapping = new Mapping(from, to, options.field, options.aggregation);
    if (this._mappings.some((existing) => existing.id === mapping.id)) {
      throw new ValidationError(`Mapping '${mapping.id}' is already declared.`, from.name);
    }

    // Vacuous over control edges alone, since a control is never a target. Kept for the day an
    // edge lands *on* a control — a linked filter would — and deliberately not extended to
    // cross-filter edges, which are bidirectional by design.
    this.assertAcyclic(mapping);

    this._mappings.push(mapping);
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

    const edge = new CrossFilter(from, to);
    if (this._crossFilters.some((existing) => existing.id === edge.id)) {
      throw new ValidationError(`Cross-filter '${edge.id}' is already declared.`, from.name);
    }

    this._crossFilters.push(edge);
    return edge;
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

  /* ----------------------------------------------------------------
  AppContext
  ---------------------------------------------------------------- */

  datasetIndex (uname: string, entity: string): DatasetIndex {
    return resolveDataset(this.datasets, uname, entity);
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

  /** True when a control or the selection has changed since the queries reading it last ran. */
  get hasChanges (): boolean {
    return [...this._controls.values()].some((control) => control.isDirty) || this.isSelectionDirty;
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
   * query must not blank the other four. See docs/adr/0002.
   */
  async execute (options: ExecuteOptions = {}): Promise<ExecuteSummary> {
    const targets = [...this._queries.values()].filter((query) => query.shouldRun(options.force));

    // Committing before the run, not after, so a control edited mid-flight is a change against the
    // values this run used rather than against the previous run's.
    this._controls.forEach((control) => control.commit());
    this._appliedSelection = this._selection;
    this.selectionCommitted = true;

    targets.forEach((query) => {
      // Supersede: a still-running request for a query this run is about to redo is dead weight.
      query.controller?.abort();
      query.resetPage();
    });

    const context: SubmitContext = {
      backend: this.backend,
      timezone: this._timezone,
      bustCache: options.bustCache,
    };

    const succeeded: string[] = [];
    const failed: string[] = [];

    await Promise.all(targets.map(async (query) => {
      const controller = new AbortController();
      query.controller = controller;
      const signature = query.signature();
      query.markExecuting();

      try {
        const result = await runQuery(
          query,
          this.mappingsTo(query.name),
          {
            conditions: this.selectionConditionsFor(query.name, 'applied'),
            expression: this.selectionExpressionFor(query.name, 'applied'),
          },
          context,
          controller.signal,
        );
        if (superseded(query, controller)) return;
        query.markSuccess(result, signature, false);
        succeeded.push(query.name);
      } catch (err) {
        if (superseded(query, controller)) return;
        if (isAbort(err)) return;
        query.markError(toDataAppError(err, query.name));
        failed.push(query.name);
      } finally {
        if (query.controller === controller) query.controller = undefined;
      }
    }));

    return { succeeded, failed };
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
    query.controller = controller;
    const signature = query.signature();
    query.markExecuting();

    try {
      const result = await runQuery(
        query,
        this.mappingsTo(query.name),
        {
          conditions: this.selectionConditionsFor(query.name, 'applied'),
          expression: this.selectionExpressionFor(query.name, 'applied'),
        },
        {
          backend: this.backend,
          timezone: this._timezone,
        },
        controller.signal,
      );
      query.markSuccess(result, signature, true);
    } catch (err) {
      query.rewindPage();
      if (isAbort(err)) return;
      const error = toDataAppError(err, query.name);
      query.markError(error);
      throw error;
    } finally {
      if (query.controller === controller) query.controller = undefined;
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

  /* ----------------------------------------------------------------
  Internals
  ---------------------------------------------------------------- */

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
