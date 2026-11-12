import type { Control } from './controls';
import type { Mapping } from './mapping';
import type { Query, QueryUpdate } from './query';
import type { DatasetIndex } from './validation';
import type {
  ExecuteOptions, ExecuteSummary, LocateKind, Row, SdkFeatures, Selection, SelectionCondition,
} from '../common/types';

/**
 * What an entity needs from the app that owns it. An interface, so `query.ts` and `controls.ts`
 * never import `app.ts` at runtime.
 */
export interface AppContext {
  readonly timezone?: string;
  /** Server-side capabilities the host declared, so a declaration can refuse what would be dropped. */
  readonly features?: SdkFeatures;
  /** The live selection, if any. App-scoped: at most one query is the source at a time. */
  readonly selection?: Selection;
  datasetIndex (uname: string, entity: string): DatasetIndex;
  /** The dataset a field-backed filter resolves in: the one it names, or the app's only one. */
  filterDataset (entity: string, declaration: { dataset?: string, field?: string }): DatasetIndex | undefined;
  /** Backs `query.update`: checks the change against the app, then swaps the declaration. */
  updateQuery (query: Query, changes: QueryUpdate): void;
  /** Backs `remove()` on a query or a control: takes it out, with its edges. */
  removeEntity (entity: Query | Control): void;
  /** Control edges only — cross-filter edges reach a query through the selection instead. */
  mappingsTo (queryName: string): Mapping[];
  /**
   * Conditions the selection puts on this query, empty unless a cross-filter edge reaches it.
   *
   * `pending` is what the reader has picked and `applied` is what the query last ran with, matching
   * `Control.condition` / `Control.appliedCondition`: signatures read pending, payloads read
   * applied, and `execute()` commits between the two.
   */
  selectionConditionsFor (queryName: string, phase: 'pending' | 'applied'): readonly SelectionCondition[];
  /** The expression form, set instead of the conditions when the collapse would have been lossy. */
  selectionExpressionFor (queryName: string, phase: 'pending' | 'applied'): string | undefined;
  /** Replaces the app's selection with these rows of this query. */
  select (query: Query, rows: readonly Row[], fields?: readonly string[]): void;
  /** Runs this query alone, with what is applied, unless that would produce nothing new or `force`. */
  executeQuery (query: Query, options: ExecuteOptions): Promise<ExecuteSummary>;
  /** Runs one more page for a query that already has a result. */
  fetchPage (query: Query): Promise<void>;
  /** Brings an entity's marked elements into view; false when none is on the page. */
  locate (kind: LocateKind, name: string): boolean;
  /** Opens the content menu on these rows of this query, for the event; whether one opens. */
  menu (query: Query, event: Event, rows: readonly Row[], fields?: readonly string[], measure?: string): boolean;
}
