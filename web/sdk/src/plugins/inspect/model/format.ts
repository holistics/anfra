/**
 * Formatting, and finding things in the snapshot: what every tab names and draws the same way.
 */
import type {
  Condition, EntityUsage, InspectedStructure, LocateKind, StructureNode,
} from '../../../common/types';

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

/** A moment in short natural terms: `< 1m ago`, `3m ago`, `2h ago`, `4d ago`. */
export function formatAgo (value: Date | string | undefined, now: number = Date.now()): string {
  if (!value) return '';
  const time = (value instanceof Date ? value : new Date(value)).getTime();
  if (Number.isNaN(time)) return String(value);
  const seconds = Math.max(0, Math.floor((now - time) / 1000));
  if (seconds < 60) return '< 1m ago';
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

/** A count and its noun: `1 row`, `2 rows`. */
export const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

/** An entity's key, as the structure's usage names it: `query:<app>/<name>`. */
export const entityKey = (kind: LocateKind, app: number, name: string) => `${kind}:${app}/${name}`;

/** The attribute a marked element carries for an entity, as the definition writes it. */
export function entityAttribute (kind: LocateKind, app: number, name: string): string {
  return `data-anfra-${kind}="${app === 0 ? '' : `${app}/`}${name}"`;
}

/** The attribute that marks a node, as the definition writes it: `data-anfra-block="river"`. */
export function attributeFor (node: StructureNode): string {
  return node.kind === 'container' || node.kind === 'block'
    ? `data-anfra-${node.kind}="${node.id ?? ''}"`
    : entityAttribute(node.kind, node.app ?? 0, node.name ?? '');
}

/**
 * The handle an author copies for an agent: a Markdown-style link whose text is the label and
 * whose target is the attribute literal, `[River](data-anfra-block="river")`. With no label, the
 * attribute alone: an agent finds either in the file with one search.
 */
export function handleFor (node: StructureNode): string {
  const attribute = attributeFor(node);
  return node.label ? `[${node.label}](${attribute})` : attribute;
}

/** How an entity is drawn: the snapshot's usage entry, or none. */
export function usageOf (structure: InspectedStructure | undefined, kind: LocateKind, app: number, name: string): EntityUsage | undefined {
  return structure?.usage[entityKey(kind, app, name)];
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

/**
 * The node of a block by its id. Where an id repeats, the one that holds the entity wins, since
 * that is the block the entity is drawn in.
 */
export function blockNodeKey (structure: InspectedStructure | undefined, id: string, kind: LocateKind, app: number, name: string): string | undefined {
  const blocks: StructureNode[] = [];
  const holds = (node: StructureNode): boolean => node.children.some((child) => (child.kind === kind && (child.app ?? 0) === app && child.name === name) || holds(child));
  const walk = (nodes: StructureNode[]) => {
    for (const node of nodes) {
      if (node.kind === 'block' && node.id === id) blocks.push(node);
      walk(node.children);
    }
  };
  walk(structure?.nodes ?? []);
  return (blocks.find(holds) ?? blocks[0])?.key;
}

/** The key of an entity's first marker in the structure, in document order; undefined when it has none. */
export function entityNodeKey (nodes: StructureNode[], kind: 'query' | 'control', app: number, name: string): string | undefined {
  for (const node of nodes) {
    if (node.kind === kind && (node.app ?? 0) === app && node.name === name) return node.key;
    const found = entityNodeKey(node.children, kind, app, name);
    if (found) return found;
  }
  return undefined;
}

/**
 * The first query or control at or below the node with `key`, in document order. `kinds` narrows
 * it: the first query alone is what leads to Data. Undefined when the node isn't there or holds none.
 */
export function firstEntityUnder (
  nodes: StructureNode[], key: string, kinds: readonly ('query' | 'control')[] = ['query', 'control'],
): { entity: 'query' | 'control', app: number, name: string } | undefined {
  const first = (node: StructureNode): StructureNode | undefined => ((node.kind === 'query' || node.kind === 'control') && kinds.includes(node.kind)
    ? node
    : node.children.map(first).find((entity) => entity !== undefined));
  const root = findNode(nodes, key);
  const entity = root && first(root);
  return entity && (entity.kind === 'query' || entity.kind === 'control')
    ? { entity: entity.kind, app: entity.app ?? 0, name: entity.name ?? '' }
    : undefined;
}

/**
 * Strips the indentation the code was written with. The first line is usually trimmed already
 * (`aql: \`explore {`), so the common indent comes from the lines after it.
 */
export function dedent (text: string): string {
  const lines = text.replace(/\t/g, '  ').trim().split('\n');
  const indentOf = (line: string) => line.length - line.trimStart().length;
  const rest = lines.slice(1).filter((line) => line.trim());
  const indent = rest.length ? Math.min(...rest.map(indentOf)) : 0;
  return lines
    .map((line, i) => (i ? line.slice(Math.min(indent, indentOf(line))) : line).trimEnd())
    .join('\n');
}
