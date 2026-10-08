import type { Mapping } from './mapping';
import type { Query } from './query';
import type { DatasetIndex } from './validation';
import type {
  Row, SdkFeatures, Selection, SelectionCondition,
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
  /** Runs one more page for a query that already has a result. */
  fetchPage (query: Query): Promise<void>;
}
