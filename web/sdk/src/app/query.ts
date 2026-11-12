import { Observable } from './observable';
import { ValidationError, type DataAppError } from '../common/errors';
import { tagCall } from '../common/runtimeErrors';
import type { DatasetIndex } from './validation';
import type { AppContext } from './internal';
import type { PluginMembers } from './pluginTypes';
import type {
  ExecuteOptions,
  ExecuteSummary,
  QueryDeclaration,
  QueryResult,
  QueryInput,
  QuerySort,
  QueryState,
  Row,
} from '../common/types';

/** What `query.update` may change. `pageSize: undefined` drops the paging. */
export interface QueryUpdate {
  dataset?: string;
  aql?: string;
  pageSize?: number;
  /** The sort to keep; left out, a new AQL or dataset clears the sort, and anything else keeps it. */
  sort?: readonly QuerySort[];
}

/** What `createQuery` and `update` check of a declaration. */
export function assertDeclaration (name: string, declaration: QueryDeclaration): void {
  if (!declaration.aql?.trim()) {
    throw new ValidationError(`Query '${name}' declares no AQL.`, name);
  }
  const { pageSize } = declaration;
  if (pageSize !== undefined && !(Number.isInteger(pageSize) && pageSize >= 1)) {
    throw new ValidationError(
      `Query '${name}' declares pageSize ${pageSize}; it must be a whole number of at least 1.`,
      name,
    );
  }
}

export class Query extends Observable {
  readonly kind = 'query' as const;

  readonly name: string;

  private _declaration: QueryDeclaration;

  private _dataset: DatasetIndex;

  /** Set once `remove()` took it out of its app; every method then throws. */
  private removed = false;

  private readonly app: AppContext & Observable;

  private _state: QueryState = 'idle';
  /** Its last run failed: not a `fetchMore`, whose failure leaves the pages already loaded to keep. */
  private runFailed = false;

  private _result?: QueryResult;

  private _error?: DataAppError;

  private _page = 1;
  // The input the result's run came back with: its next page and its export replay it.
  private _ranInput: QueryInput | undefined;

  /**
   * A sort applied to the result, independent of the query's own AQL — the same mechanism a click
   * on a table's column header uses. Kept off the declaration so setting it never re-authors the
   * query. See docs/adr/0001.
   */
  private _sort: readonly QuerySort[] = [];

  /**
   * The result belongs to the declaration before the last `update`: it stays on screen until the
   * rerun, but its pages can't be continued with the new AQL. Cleared when a run's first page lands.
   */
  private resultSuperseded = false;

  /** The input signature of the last successful run; undefined until one has happened. */
  private lastRunSignature?: string;

  /**
   * Rows in the most recent page, not in the accumulated result. `fetchMore` appends, so comparing
   * the total against the page size would report "more" forever once two pages are held.
   */
  private lastPageRows = 0;

  /** @internal Set by the executor so `abort()` can reach the in-flight request. */
  controller?: AbortController;

  constructor (name: string, app: AppContext & Observable, declaration: QueryDeclaration) {
    // Parented to the app so one `app.subscribe` sees every entity's changes.
    super(app);
    assertDeclaration(name, declaration);
    this.name = name;
    this.app = app;
    this._declaration = declaration;
    this._dataset = app.datasetIndex(declaration.dataset, name);
  }

  /** What it was declared with, or last updated to. */
  get declaration (): QueryDeclaration {
    return this._declaration;
  }

  /** The dataset its declaration names, resolved. */
  get dataset (): DatasetIndex {
    return this._dataset;
  }

  /** False once `remove()` took it out of its app. */
  get isRemoved (): boolean {
    return this.removed;
  }

  /**
   * Swaps what this query is: its dataset, its AQL, its page size, checked as `createQuery` checks
   * them. Its name, its edges and its result stay; the result until the rerun replaces it. Runs
   * nothing: the query is dirty, and the next `app.execute()` reruns it, so several edits go out
   * as one run. Back to the first page. A run or page in flight is aborted, and `fetchMore` does
   * nothing until the rerun. A new AQL or dataset clears the sort (unless `sort` is given) and a
   * selection made in this query, whose rows no longer mean the same.
   */
  update (changes: QueryUpdate): void {
    this.assertLive();
    this.app.updateQuery(this, changes);
  }

  /**
   * Takes this query out of its app: its control mappings and cross-filters go with it, a run in
   * flight is aborted, a selection made in it is cleared, and its name is free again. The queries
   * it cross-filtered rerun without it at the next `app.execute()`. Every method of it throws from
   * then on.
   */
  remove (): void {
    this.assertLive();
    this.app.removeEntity(this);
  }

  /** @internal Throws once the query was removed. */
  assertLive (): void {
    if (this.removed) {
      throw new ValidationError(`Query '${this.name}' was removed from its app; it can't be used any more.`, this.name);
    }
  }

  /** @internal The app's half of `update`, checked by then: swap the declaration. */
  replaceDeclaration (declaration: QueryDeclaration, dataset: DatasetIndex, sort: readonly QuerySort[]): void {
    // A run or page in flight is for the old declaration: its answer must not land.
    this.controller?.abort();
    this.controller = undefined;
    if (this._state === 'executing') this._state = this._result ? 'success' : 'idle';
    this.resultSuperseded = true;
    this._declaration = declaration;
    this._dataset = dataset;
    this._sort = [...sort];
    // The last run's input belongs to the old declaration: replaying it with the new AQL would
    // bring back a sort the edit cleared. Pages and exports build from the applied state instead.
    this._ranInput = undefined;
    this.resetPage();
    this.notify();
  }

  /** @internal The app's half of `remove`: stop what is in flight, and stay quiet from then on. */
  markRemoved (): void {
    this.removed = true;
    this.controller?.abort();
    this.controller = undefined;
  }

  /**
   * Bring the elements marked `data-anfra-query` with this query's name into view: scroll to the
   * first and show them all for a moment. False, and nothing done, when no element on the page is
   * marked with it yet; never throws, so an author's own button is safe.
   */
  locate (): boolean {
    this.assertLive();
    return this.app.locate('query', this.name);
  }

  get state (): QueryState {
    return this._state;
  }

  get result (): QueryResult | undefined {
    return this._result;
  }

  get error (): DataAppError | undefined {
    return this._error;
  }

  /** Rows per page, or undefined for a query that isn't paged and gets every row. */
  get pageSize (): number | undefined {
    return this.declaration.pageSize;
  }

  get page (): number {
    return this._page;
  }

  get sort (): readonly QuerySort[] {
    return this._sort;
  }

  /** Replaces the sort applied to this query's result. Pass `[]` to clear it back to the AQL's own. */
  setSort (sort: readonly QuerySort[]): void {
    this.assertLive();
    this._sort = [...sort];
    this.notify();
  }

  /**
   * Whether another page may follow: as the backend said of the last page, or, when it did not
   * say, whether that page came back full. False after `update` until the rerun.
   */
  get hasMore (): boolean {
    if (!this._result || this.pageSize === undefined || this.resultSuperseded) return false;
    return this._result.meta.hasMore ?? this.lastPageRows >= this.pageSize;
  }

  /**
   * Everything that changes the rows: the declaration, the conditions reaching this query through
   * mappings, the conditions reaching it from the selection, the sort override, the app's timezone,
   * and the page. Compared against the last successful run to decide whether re-running would
   * produce anything new. `pending` reads the controls and selection as the reader has them,
   * `applied` as the app last applied them; they differ only between a change and `app.execute()`.
   */
  signature (phase: 'pending' | 'applied' = 'pending'): string {
    const conditions = this.app.mappingsTo(this.name)
      .map((mapping) => ({
        field: mapping.field,
        aggregation: mapping.aggregation,
        condition: phase === 'pending' ? mapping.from.condition : mapping.from.appliedCondition,
      }))
      // Field alone is not a stable key: one query may take two conditions on the same field from
      // different controls, and their order must not flip between two otherwise identical runs.
      .sort((a, b) => a.field.localeCompare(b.field) || JSON.stringify(a).localeCompare(JSON.stringify(b)));

    return JSON.stringify({
      declaration: this.declaration,
      conditions,
      selection: this.app.selectionConditionsFor(this.name, phase),
      // Separate from `selection`, and not optional: a selection sent as one expression leaves
      // `conditions` empty, so without this every such selection has the same signature as the
      // next one and the query is never re-run.
      selectionExpression: this.app.selectionExpressionFor(this.name, phase),
      sort: this._sort,
      timezone: this.app.timezone,
      page: this._page,
    });
  }

  /**
   * False before the first run: with nothing applied there is nothing to have changed. Distinct
   * from `shouldRun`, which is true in that case precisely because there is no result yet.
   */
  get isDirty (): boolean {
    return this.lastRunSignature !== undefined && this.signature() !== this.lastRunSignature;
  }

  /**
   * @internal Also when its last run failed, whatever its inputs: so `execute()` is the retry, of
   * the failed queries only, even after a forced run (`refresh()`) failed with unchanged inputs. Not
   * after a failed `fetchMore`: rerun, it would drop the pages already loaded; `fetchMore` is its retry.
   */
  shouldRun (force = false): boolean {
    return force || this.runFailed || this.lastRunSignature === undefined || this.isDirty;
  }

  /**
   * @internal For `query.execute()`: like `shouldRun`, but against what is applied, since applying
   * the reader's pending changes is the app's, for every query at once. Its own sort and the
   * timezone count; a control or selection the reader changed and the app has not applied does not.
   */
  shouldRunAlone (force = false): boolean {
    return force || this.runFailed || this.lastRunSignature === undefined
      || this.signature('applied') !== this.lastRunSignature;
  }

  /**
   * Runs this query alone, with the inputs the app last applied, if that would produce something
   * new; with `force`, whatever its inputs (a paged query starts over at its first page). Applies
   * none of the reader's pending changes: that is `app.execute()`'s, so the query stays dirty and
   * the next `app.execute()` runs it with them. Never rejects, as `app.execute()` doesn't.
   */
  execute (options: ExecuteOptions = {}): Promise<ExecuteSummary> {
    this.assertLive();
    return this.app.executeQuery(this, options);
  }

  /** Stops the in-flight request for this query. The pending `execute()` resolves without it. */
  abort (): void {
    this.assertLive();
    this.controller?.abort();
  }

  /** Fetches the next page and appends it. Rejects rather than swallowing, unlike `execute()`. */
  async fetchMore (): Promise<void> {
    try {
      this.assertLive();
      if (this.pageSize === undefined) {
        throw new ValidationError(`Query '${this.name}' declares no pageSize, so it has no more pages.`, this.name);
      }
      if (!this.hasMore) return;
      this._page += 1;
      await this.app.fetchPage(this);
    } catch (err) {
      // Left unhandled, it is reported as `call.fetch_more`.
      tagCall(err, 'fetch_more');
      throw err;
    }
  }

  /**
   * Opens the menu on these rows of this query: the items plugins and the host offer for a point
   * of data, such as "View underlying data". Call it from your chart's own right-click handler,
   * with the event and the rows under the pointer, as for `select`; `fields` narrows the dimensions
   * that take part, and `measure` names the measure column the point shows (a chart's series), for
   * items that are about one measure. Whether a menu opens: when one does, the event's default (the
   * browser's menu) is prevented, and when none would (no plugin offers one), nothing happens.
   */
  menu (event: Event, rows: Row | readonly Row[], options: { fields?: readonly string[], measure?: string } = {}): boolean {
    this.assertLive();
    return this.app.menu(this, event, Array.isArray(rows) ? rows : [rows as Row], options.fields, options.measure);
  }

  /**
   * Cross-filters this query's targets by the rows the reader picked.
   *
   * Replaces whatever was selected, here or in another query: an app holds one selection at a time.
   * Pass no rows to clear it. The SDK never sees the click, so the author calls this from their own
   * chart's handler, passing the result rows they rendered.
   *
   * `fields` narrows which result columns take part, for a chart that renders fewer than the query
   * returned. A measure or an adhoc column never takes part, whether or not it is named here.
   */
  select (rows: Row | readonly Row[], options: { fields?: readonly string[] } = {}): void {
    this.assertLive();
    this.app.select(this, Array.isArray(rows) ? rows : [rows as Row], options.fields);
  }

  /**
   * The rows of this query's current result that the reader has selected, for redrawing its own
   * highlight. Empty unless this query is the selection's source.
   *
   * Matched by value rather than by object identity. A source is never filtered by its own
   * selection, but it does re-run when it stops being some *other* query's target — which is
   * exactly what happens when the reader moves their selection from another panel to this one. Its
   * rows come back as new objects, and identity would drop the highlight at that moment.
   *
   * `app.selection.rows` still holds what the author passed, verbatim.
   */
  get selectedRows (): readonly Row[] {
    const { selection } = this.app;
    if (selection?.source !== this.name) return [];

    const rows = this._result?.rows;
    // Before a result arrives there is nothing to match against, so the author's own rows are it.
    if (!rows) return selection.rows;

    const key = (row: Row): string => JSON.stringify(
      selection.fields.map((alias) => row[alias] ?? null),
    );
    const picked = new Set(selection.rows.map(key));
    return rows.filter((row) => picked.has(key(row)));
  }

  /** @internal */
  markExecuting (): void {
    if (this.removed) return;
    this._state = 'executing';
    this._error = undefined;
    this.notify();
  }

  /** @internal The input the result's run came back with, for its next page and its export. */
  get ranInput (): QueryInput | undefined {
    return this._ranInput;
  }

  /** @internal */
  markSuccess (result: QueryResult, signature: string, append: boolean, ranInput?: QueryInput): void {
    if (this.removed) return;
    this._state = 'success';
    if (!append) {
      this._ranInput = ranInput;
      this.resultSuperseded = false;
    }
    this._error = undefined;
    if (!append) this.runFailed = false;
    this.lastRunSignature = signature;
    this.lastPageRows = result.rows.length;

    if (append && this._result) {
      const rows = [...this._result.rows, ...result.rows];
      this._result = { ...result, rows, meta: { ...result.meta, numRows: rows.length } };
    } else {
      this._result = result;
    }

    this.notify();
  }

  /** @internal `page` when it is a `fetchMore` that failed, not a run. */
  markError (error: DataAppError, page = false): void {
    if (this.removed) return;
    if (!page) this.runFailed = true;
    this._state = 'error';
    this._error = error;
    this.notify();
  }

  /** @internal Reverts an optimistic page bump when a `fetchMore` fails. */
  rewindPage (): void {
    if (this._page > 1) this._page -= 1;
  }

  /** @internal */
  resetPage (): void {
    this._page = 1;
    this.lastPageRows = 0;
  }

  toJSON (): QueryDeclaration {
    return this._declaration;
  }
}

/** What the installed plugins add to every query (`AnfraPlugins`): each member is there only where its plugin is. */
// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging -- plugins' members, on the class they are added to
export interface Query extends PluginMembers<'query'> {}
