/** Where a link in a tab goes: the tabs' ids, and every target a link resolves to. */

export const OVERVIEW_TAB = 'inspect.overview';
export const DATA_TAB = 'inspect.data';
export const QUERIES_TAB = 'inspect.queries';
export const STRUCTURE_TAB = 'inspect.structure';
export const DECLARATION_TAB = 'inspect.declaration';

/** What Queries narrows to: everything, the queries that failed, or what is dirty. */
export type QueriesFilter = 'all' | 'errors' | 'dirty';

/**
 * Where a link goes. A tab, with Queries' filter; a query or control's row in Queries; a query's
 * table in Data; one of an app's checks in Declaration; a node in Structure.
 */
export type RevealTarget =
  | { tab: string, filter?: QueriesFilter }
  | { entity: 'query' | 'control', app: number, name: string }
  | { table: string, app: number }
  | { check: number, app: number }
  | { node: string };
