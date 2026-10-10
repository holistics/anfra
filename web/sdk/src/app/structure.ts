/**
 * The structure: a Data App's semantic parts, declared by the definition on its own elements with
 * `data-anfra-*` attributes and read from the running page. Optional and partial: anything
 * unmarked is simply not in it. Built only while a host inspects, and never thrown over: a problem
 * in the markup is reported on the node it concerns.
 */
import type { App } from './app';
import type { OverlayLabel } from './overlay';
import type {
  EntityUsage, InspectedStructure, LocateKind, StructureNode,
} from '../common/types';

export const ATTR = {
  container: 'data-anfra-container',
  block: 'data-anfra-block',
  query: 'data-anfra-query',
  control: 'data-anfra-control',
  label: 'data-anfra-label',
} as const;

/** Every element that is in the structure. */
export const MARKED = `[${ATTR.container}],[${ATTR.block}],[${ATTR.query}],[${ATTR.control}]`;

/** A `[app/]name` reference as a marker writes it. */
export interface EntityRef {
  app: number;
  name: string;
}

/** Parse one token of a `data-anfra-query` or `data-anfra-control` attribute; undefined if malformed. */
export function parseRef (token: string): EntityRef | undefined {
  const match = /^(?:(\d+)\/)?([^/\s]+)$/.exec(token);
  if (!match) return undefined;
  return { app: match[1] === undefined ? 0 : Number(match[1]), name: match[2] };
}

const tokens = (value: string | null): string[] => (value ?? '').split(/\s+/).filter(Boolean);

/** The elements marked as drawn by one entity, in DOM order. */
export function markersFor (doc: Document, kind: LocateKind, app: number, name: string): Element[] {
  const attr = kind === 'query' ? ATTR.query : ATTR.control;
  return [...doc.querySelectorAll(`[${attr}]`)].filter((element) => tokens(element.getAttribute(attr))
    .some((token) => { const ref = parseRef(token); return ref?.app === app && ref.name === name; }));
}

export interface BuiltStructure extends InspectedStructure {
  /** The marked element behind each node key, for highlighting and picking. */
  elements: Map<string, Element>;
  /** The node keys an element stands for: its own node, and one per entity it is marked with. */
  keysOf: Map<Element, string[]>;
}

const hasEntity = (apps: readonly App[], kind: LocateKind, ref: EntityRef): boolean => {
  const app = apps[ref.app];
  if (!app) return false;
  return Object.prototype.hasOwnProperty.call(kind === 'query' ? app.queries : app.controls, ref.name);
};

/**
 * Read the structure off the document: one node per container or block, and one per entity a
 * marked element names, placed under the nearest marked ancestor. Keys are index paths (`0.2.1`)
 * so an unchanged tree keeps the same keys from one snapshot to the next.
 */
export function buildStructure (doc: Document, apps: readonly App[]): BuiltStructure {
  const roots: StructureNode[] = [];
  const elements = new Map<string, Element>();
  const keysOf = new Map<Element, string[]>();
  const nodeOf = new Map<Element, StructureNode>();
  const kindOf = new Map<Element, 'container' | 'block'>();
  const seenIds: Record<'container' | 'block', Set<string>> = { container: new Set(), block: new Set() };
  const usage: Record<string, { elements: Set<Element>, blocks: Set<string> }> = {};

  const place = (parent: Element | null, node: Omit<StructureNode, 'key' | 'children'>, element: Element): StructureNode => {
    const siblings = parent ? nodeOf.get(parent)!.children : roots;
    const prefix = parent ? `${nodeOf.get(parent)!.key}.` : '';
    const full: StructureNode = { ...node, key: `${prefix}${siblings.length}`, children: [] };
    siblings.push(full);
    elements.set(full.key, element);
    keysOf.set(element, [...(keysOf.get(element) ?? []), full.key]);
    return full;
  };

  for (const element of doc.querySelectorAll(MARKED)) {
    const parent = element.parentElement?.closest(MARKED) ?? null;
    const containerId = element.getAttribute(ATTR.container);
    const blockId = element.getAttribute(ATTR.block);
    const label = element.getAttribute(ATTR.label) ?? undefined;
    const enclosingBlock = parent && (kindOf.get(parent) === 'block'
      ? parent
      : parent.closest(`[${ATTR.block}]`));

    // The element's own node, if it is a container or block.
    let own: StructureNode | undefined;
    if (containerId !== null || blockId !== null) {
      const kind = containerId !== null ? 'container' : 'block';
      const id = (containerId ?? blockId ?? '').trim();
      const problems: string[] = [];
      if (containerId !== null && blockId !== null) problems.push('Marked as both a container and a block; read as a container.');
      if (!id) problems.push(`An empty ${kind} id.`);
      else if (seenIds[kind].has(id)) problems.push(`Another ${kind} already has the id "${id}".`);
      else seenIds[kind].add(id);
      if (enclosingBlock) {
        problems.push(`Inside block "${enclosingBlock.getAttribute(ATTR.block)}", but a block holds no container or block.`);
      }
      own = place(parent, {
        kind, id, ...(label !== undefined ? { label } : {}), problems,
      }, element);
      nodeOf.set(element, own);
      kindOf.set(element, kind);
    }

    // One node per entity the element is marked with, under the element's own node if it has one.
    const under = own ? element : parent;
    for (const kind of ['query', 'control'] as const) {
      for (const token of tokens(element.getAttribute(kind === 'query' ? ATTR.query : ATTR.control))) {
        const ref = parseRef(token);
        const problems: string[] = [];
        if (!ref) problems.push(`"${token}" is not a reference: write "name" or "app/name".`);
        else if (!hasEntity(apps, kind, ref)) {
          problems.push(apps[ref.app]
            ? `App ${ref.app} has no ${kind} named "${ref.name}".`
            : `There is no app ${ref.app} in this frame.`);
        }
        place(under, {
          kind, name: ref?.name ?? token, app: ref?.app ?? 0, problems,
        }, element);
        if (ref && !problems.length) {
          const key = `${ref.app}/${ref.name}`;
          const entry = usage[`${kind}:${key}`] ??= { elements: new Set(), blocks: new Set() };
          entry.elements.add(element);
          const block = own?.kind === 'block' ? own.id : enclosingBlock?.getAttribute(ATTR.block);
          if (block) entry.blocks.add(block);
        }
      }
    }
  }

  const usageOut: Record<string, EntityUsage> = Object.fromEntries(
    Object.entries(usage).map(([key, entry]) => [key, { markers: entry.elements.size, blocks: [...entry.blocks] }]),
  );
  return {
    nodes: roots, usage: usageOut, elements, keysOf,
  };
}

/** The usage key the snapshot uses for one entity. */
export const usageKey = (kind: LocateKind, app: number, name: string): string => `${kind}:${app}/${name}`;

/** The element's size as the overlay shows it. */
export function sizeOf (element: Element): string {
  const rect = element.getBoundingClientRect();
  return `${Math.round(rect.width)} × ${Math.round(rect.height)}`;
}

/** What the overlay's label says for a node. */
export function describeNode (node: StructureNode, element: Element, structure: InspectedStructure): OverlayLabel {
  const size = sizeOf(element);
  if (node.kind === 'container' || node.kind === 'block') {
    return {
      kind: node.kind, name: node.id ?? '', ...(node.label ? { label: node.label } : {}), size,
    };
  }
  const use = structure.usage[usageKey(node.kind, node.app ?? 0, node.name ?? '')];
  return {
    kind: node.kind,
    name: node.name ?? '',
    ...(use && use.markers > 1 ? { note: `1 of ${use.markers}` } : {}),
    size,
  };
}
