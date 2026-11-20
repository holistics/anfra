/** Where a link in a tab goes: the tabs' ids, and every target a link resolves to. */
import type { RevealTarget } from '../../../host/panel';

export const OVERVIEW_TAB = 'inspect.overview';
export const DATA_TAB = 'inspect.data';
export const QUERIES_TAB = 'inspect.queries';
export const STRUCTURE_TAB = 'inspect.structure';
export const DECLARATION_TAB = 'inspect.declaration';
/** The `lineage` plugin's tab: linked to through the selection, when it is installed. */
export const LINEAGE_TAB = 'lineage.lineage';

/** What Queries narrows to: everything, the queries that failed, or what is dirty. */
export type QueriesFilter = 'all' | 'errors' | 'dirty';

/**
 * Where one of `inspect`'s own links goes: a tab, with Queries' filter; a query's table in Data; one
 * of an app's checks in Declaration; a node in Structure. A query or a control is the panel's
 * reveal instead (`RevealTarget`).
 */
export type InspectLink =
  | { tab: string, filter?: QueriesFilter }
  | { table: string, app: number }
  | { check: number, app: number }
  | { node: string };

/** Where any link in a tab goes: one of `inspect`'s own targets, or a query or control the panel reveals in Queries. */
export type LinkTarget = InspectLink | RevealTarget;
