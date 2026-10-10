import { Observable } from './observable';
import { ValidationError, type DataAppError } from '../common/errors';
import type { DatasetIndex } from './validation';
import type { AppContext } from './internal';
import type {
  QueryDeclaration,
  QueryResult,
  QuerySort,
  QueryState,
  Row,
} from '../common/types';

export class Query extends Observable {
  readonly kind = 'query' as const;

  readonly name: string;

  readonly declaration: QueryDeclaration;

  readonly dataset: DatasetIndex;

  private readonly app: AppContext & Observable;

  private _state: QueryState = 'idle';

  private _result?: QueryResult;

  private _error?: DataAppError;

  private _page = 1;

  /**
   * A sort applied to the result, independent of the query's own AQL — the same mechanism a click
   * on a table's column header uses. Kept off the declaration so setting it never re-authors the
   * query. See docs/adr/0001.
   */
  private _sort: readonly QuerySort[] = [];

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
    this.name = name;
    this.app = app;
    this.declaration = declaration;
    this.dataset = app.datasetIndex(declaration.dataset, name);
  }

  /**
   * Bring the elements marked `data-anfra-query` with this query's name into view: scroll to the
   * first and show them all for a moment. False, and nothing done, when no element on the page is
   * marked with it yet; never throws, so an author's own button is safe.
   */
  locate (): boolean {
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
    this._sort = [...sort];
    this.notify();
  }

  get hasMore (): boolean {
    if (!this._result || this.pageSize === undefined) return false;
    return this.lastPageRows >= this.pageSize;
  }

  /**
   * Everything that changes the rows: the declaration, the conditions reaching this query through
   * mappings, the conditions reaching it from the selection, the sort override, the app's timezone,
   * and the page. Compared against the last successful run to decide whether re-running would
   * produce anything new.
   */
  signature (): string {
    const conditions = this.app.mappingsTo(this.name)
      .map((mapping) => ({
        field: mapping.field,
        aggregation: mapping.aggregation,
        condition: mapping.from.condition,
      }))
      // Field alone is not a stable key: one query may take two conditions on the same field from
      // different controls, and their order must not flip between two otherwise identical runs.
      .sort((a, b) => a.field.localeCompare(b.field) || JSON.stringify(a).localeCompare(JSON.stringify(b)));

    return JSON.stringify({
      declaration: this.declaration,
      conditions,
      selection: this.app.selectionConditionsFor(this.name, 'pending'),
      // Separate from `selection`, and not optional: a selection sent as one expression leaves
      // `conditions` empty, so without this every such selection has the same signature as the
      // next one and the query is never re-run.
      selectionExpression: this.app.selectionExpressionFor(this.name, 'pending'),
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

  /** @internal */
  shouldRun (force = false): boolean {
    return force || this.lastRunSignature === undefined || this.isDirty;
  }

  /** Stops the in-flight request for this query. The pending `execute()` resolves without it. */
  abort (): void {
    this.controller?.abort();
  }

  /** Fetches the next page and appends it. Rejects rather than swallowing, unlike `execute()`. */
  async fetchMore (): Promise<void> {
    if (this.pageSize === undefined) {
      throw new ValidationError(`Query '${this.name}' declares no pageSize, so it has no more pages.`, this.name);
    }
    if (!this.hasMore) return;
    this._page += 1;
    await this.app.fetchPage(this);
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
    this._state = 'executing';
    this._error = undefined;
    this.notify();
  }

  /** @internal */
  markSuccess (result: QueryResult, signature: string, append: boolean): void {
    this._state = 'success';
    this._error = undefined;
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

  /** @internal */
  markError (error: DataAppError): void {
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
    return this.declaration;
  }
}
