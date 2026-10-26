/**
 * What a menu on a Data App's content is about: a marked node of the page (a right-click on a
 * block), or some of a query's result rows (a right-click on a chart's point, which the definition
 * reports with `query.menu`). Built in the frame, and checked by the host, which trusts nothing the
 * frame sends.
 */
import type { Row, SelectionCondition, StructureNode } from './types';

/** A box in the frame's viewport. */
export interface TargetRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** A marked node, as the structure has it, its ancestors outermost first (both without children's children), and where it is. */
export interface NodeTarget {
  kind: 'node';
  node: StructureNode;
  ancestors: StructureNode[];
  rect: TargetRect;
}

/**
 * Rows of a query's result: the query, by its app's index and name; the rows as the definition
 * passed them; the dimensions that took part; and the conditions they make, derived as a
 * cross-filter's are (an AQL `expression` when per-field conditions would admit more rows). With
 * the node the rows are drawn in, when the event was inside one.
 */
export interface RowsTarget {
  kind: 'rows';
  app: number;
  query: string;
  rows: Row[];
  fields: string[];
  conditions: SelectionCondition[];
  expression?: string;
  node?: NodeTarget;
}

export type MenuTarget = NodeTarget | RowsTarget;

export type MenuTargetKind = MenuTarget['kind'];

/** One entry of a menu, as the host answers it for a target. */
export interface MenuEntry {
  id: string;
  label: string;
  /** Why it can't run: shown, and the entry can't be picked. */
  disabled?: string;
}

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);
const isRect = (value: unknown): value is TargetRect => isObject(value) && ['x', 'y', 'width', 'height'].every((key) => typeof value[key] === 'number');
const isNode = (value: unknown): value is StructureNode => isObject(value) && typeof value.key === 'string' && typeof value.kind === 'string'
  && Array.isArray(value.problems) && Array.isArray(value.children);

function checkNode (value: unknown): NodeTarget | undefined {
  if (!isObject(value) || value.kind !== 'node' || !isNode(value.node) || !Array.isArray(value.ancestors) || !value.ancestors.every(isNode) || !isRect(value.rect)) return undefined;
  return {
    kind: 'node', node: value.node, ancestors: value.ancestors, rect: value.rect,
  };
}

/** A target as the frame sends it, or undefined when it isn't one: of its shape only, never trusted further. */
export function checkTarget (value: unknown): MenuTarget | undefined {
  if (!isObject(value)) return undefined;
  if (value.kind === 'node') return checkNode(value);
  if (value.kind !== 'rows') return undefined;
  if (typeof value.app !== 'number' || typeof value.query !== 'string' || !Array.isArray(value.rows) || !value.rows.every(isObject)) return undefined;
  if (!Array.isArray(value.fields) || !value.fields.every((field) => typeof field === 'string')) return undefined;
  if (!Array.isArray(value.conditions) || !value.conditions.every(isObject)) return undefined;
  if (value.expression !== undefined && typeof value.expression !== 'string') return undefined;
  const node = value.node === undefined ? undefined : checkNode(value.node);
  if (value.node !== undefined && !node) return undefined;
  return {
    kind: 'rows',
    app: value.app,
    query: value.query,
    rows: value.rows as Row[],
    fields: value.fields as string[],
    conditions: value.conditions as unknown as SelectionCondition[],
    ...(value.expression !== undefined ? { expression: value.expression } : {}),
    ...(node ? { node } : {}),
  };
}
