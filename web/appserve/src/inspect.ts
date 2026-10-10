// Formatting for the inspect panel. What it shows is what the Anfra SDK posts out of the frame
// while the panel is open: every app's state (`App.toInspectJSON()`) and the page's structure.
// Its types are the SDK's. Rows are never part of it.
import type {
  Condition, EntityUsage, InspectedStructure, LocateKind, StructureNode,
} from '@holistics/anfra-sdk/common';

export type {
  EntityUsage, InspectedApp, InspectedControl, InspectedError, InspectedQuery, InspectedSelection,
  InspectedStructure, LocateKind, LocateTarget, StructureNode,
} from '@holistics/anfra-sdk/common';

export function formatCondition (condition: Condition | undefined): string {
  if (!condition) return 'not applied yet';
  const parts = [condition.operator, condition.modifier, ...(condition.values ?? []).map(String)];
  return parts.filter((part) => part !== undefined && part !== '').join(' ');
}

export function formatTime (value: Date | string | undefined): string {
  if (!value) return '';
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleTimeString();
}

/** The attribute a marked element carries for an entity, as the definition writes it. */
export function entityAttribute (kind: LocateKind, app: number, name: string): string {
  return `data-anfra-${kind}="${app === 0 ? '' : `${app}/`}${name}"`;
}

/**
 * The handle an author copies for an agent: a Markdown-style link whose text is the label and
 * whose target is the attribute literal, `[River](data-anfra-block="river")`. With no label, the
 * attribute alone: an agent finds either in the file with one search.
 */
export function handleFor (node: StructureNode): string {
  const attribute = node.kind === 'container' || node.kind === 'block'
    ? `data-anfra-${node.kind}="${node.id ?? ''}"`
    : entityAttribute(node.kind, node.app ?? 0, node.name ?? '');
  return node.label ? `[${node.label}](${attribute})` : attribute;
}

/** How an entity is drawn, for its card: the snapshot's usage entry, or none. */
export function usageOf (structure: InspectedStructure | undefined, kind: LocateKind, app: number, name: string): EntityUsage | undefined {
  return structure?.usage[`${kind}:${app}/${name}`];
}

export function findNode (nodes: StructureNode[], key: string): StructureNode | undefined {
  for (const node of nodes) {
    if (node.key === key) return node;
    const inner = findNode(node.children, key);
    if (inner) return inner;
  }
  return undefined;
}

export function countProblems (nodes: StructureNode[]): number {
  return nodes.reduce((n, node) => n + node.problems.length + countProblems(node.children), 0);
}
