/**
 * The Lineage tab's graph: where each number on the page comes from, left to right. A control
 * filters queries; a block draws queries; a query returns metrics and dimensions; each comes from
 * its dataset, which reads models, each over a database table. Built from the page as inspected and
 * the catalog's lineage; the tab only filters, focuses and draws it.
 */
import type {
  ColumnMeta, FieldLineage, InspectedApp, InspectedStructure, LineageDiagram, LineageTarget, StructureNode,
} from '../../common/types';
import { controlMappings, crossFilters } from '../inspect/model/checks';
import { formatCondition } from '../inspect/model/format';

export const LINEAGE_KINDS = ['control', 'block', 'query', 'metric', 'dimension', 'dataset', 'model', 'table'] as const;
export type LineageKind = (typeof LINEAGE_KINDS)[number];

export interface LineageGraphNode {
  id: string;
  kind: LineageKind;
  label: string;
  /** The name code uses, in monospace under the label: a field's or model's name, a table's data source. */
  name?: string;
  description?: string;
  /** A model's fields that the page's fields read. */
  chips?: string[];
  /** A block's, query's or control's node in the structure, to locate on the page and select in Structure. */
  structureKey?: string;
  /** A control: whether a value is applied (what the queries last ran with), and it in words. */
  isSet?: boolean;
  value?: string;
  /** A control: what the reader has typed but not applied, when it differs from the value. */
  pending?: string;
  /** Why the catalog could not trace a field. */
  error?: string;
}

/** An edge from what uses to what is used: drawn rightward, from the page to the warehouse. */
export interface LineageGraphEdge {
  id: string;
  source: string;
  target: string;
  /** The page's fields whose data flows along it: what a path follows. */
  fields: string[];
  /** A filter's edge, which says "filters" rather than "is built from". */
  kind?: 'control' | 'crossFilter';
  /** A filter's edge: whether it changes the target's numbers now. */
  active?: boolean;
}

export interface LineageGraph {
  nodes: LineageGraphNode[];
  edges: LineageGraphEdge[];
}

/** What the catalog traced, by `targetKey`. */
export type Traced = ReadonlyMap<string, FieldLineage>;

export function targetKey (target: LineageTarget): string {
  return `${target.parent}\u0000${target.child}`;
}

/** The catalog's field a column is: none for an ad hoc one, which is the query's own. */
function targetOf (column: ColumnMeta, dataset: string): LineageTarget | undefined {
  if (column.adhoc) return undefined;
  return { parent: column.modelId ?? dataset, child: column.fieldName };
}

/** Every field the page's queries return, once each, in a stable order: what to trace. */
export function lineageTargets (apps: readonly InspectedApp[]): LineageTarget[] {
  const targets = new Map<string, LineageTarget>();
  for (const app of apps) {
    for (const [name, query] of Object.entries(app.queries)) {
      const dataset = app.declaration.queries[name]?.dataset;
      if (!dataset) continue;
      for (const column of query.columns) {
        const target = targetOf(column, dataset);
        if (target) targets.set(targetKey(target), target);
      }
    }
  }
  return [...targets.values()].sort((a, b) => targetKey(a).localeCompare(targetKey(b)));
}

/** Each block in the tree, by its first node, with the queries marked inside any node of that id. */
function blocksOf (nodes: StructureNode[], out = new Map<string, { node: StructureNode, queries: Set<string> }>()) {
  for (const node of nodes) {
    if (node.kind === 'block' && node.id) {
      const block = out.get(node.id) ?? { node, queries: new Set<string>() };
      const walk = (n: StructureNode) => {
        if (n.kind === 'query' && n.name !== undefined) block.queries.add(`${n.app ?? 0}/${n.name}`);
        n.children.forEach(walk);
      };
      node.children.forEach(walk);
      out.set(node.id, block);
    }
    blocksOf(node.children, out);
  }
  return out;
}

/** Each query's or control's first node in the tree, to locate it on the page. */
function entityNodesOf (nodes: StructureNode[], kind: 'query' | 'control', out = new Map<string, StructureNode>()) {
  for (const node of nodes) {
    if (node.kind === kind && node.name !== undefined) {
      const key = `${node.app ?? 0}/${node.name}`;
      if (!out.has(key)) out.set(key, node);
    }
    entityNodesOf(node.children, kind, out);
  }
  return out;
}

interface Entity {
  id: string;
  type: string;
  label: string;
  props: Record<string, unknown>;
  links: { to: string, type: string }[];
  container?: Entity;
}

/** Every entity in a diagram, cards and their sections' nodes alike, by id. */
function entitiesOf (diagram: LineageDiagram): Map<string, Entity> {
  const out = new Map<string, Entity>();
  for (const card of diagram.cards) {
    const entity: Entity = { id: card.id, type: card.type, label: card.label, props: card.props ?? {}, links: card.links ?? [] };
    out.set(card.id, entity);
    for (const section of card.sections ?? []) {
      for (const node of section.nodes) {
        out.set(node.id, { id: node.id, type: node.type, label: node.label, props: node.props ?? {}, links: node.links ?? [], container: entity });
      }
    }
  }
  return out;
}

const text = (value: unknown) => (typeof value === 'string' && value ? value : undefined);

/**
 * The model fields a traced field reads, following `refers` links through metrics and dimensions
 * that refer to others, each with the model it is in. A model's own field reads itself.
 */
function modelFieldsOf (lineage: FieldLineage | undefined, target: LineageTarget): { model: string, field: string, card?: Entity }[] {
  const diagram = lineage?.diagram;
  if (!diagram) return [];
  const entities = entitiesOf(diagram);
  const fqn = `${target.parent}.${target.child}`;
  const start = [...entities.values()].find((e) => e.container && text(e.props.fqn) === fqn);
  const out = new Map<string, { model: string, field: string, card?: Entity }>();
  const seen = new Set<string>();
  const visit = (id: string) => {
    if (seen.has(id)) return;
    seen.add(id);
    const entity = entities.get(id);
    if (!entity) return;
    const model = entity.container?.type === 'aml.model' ? entity.container : undefined;
    const modelName = model && (text(model.props.fqn) ?? text(model.props.name));
    const field = text(entity.props.name) ?? entity.label;
    if (modelName) out.set(`${modelName}.${field}`, { model: modelName, field, card: model });
    for (const link of entity.links) if (link.type === 'refers') visit(link.to);
  };
  if (start) visit(start.id);
  return [...out.values()];
}

/**
 * The graph of the whole page: every block, the queries it draws, the fields they return, and what
 * each field is built from, as far as the catalog says. Shared models and tables appear once.
 */
export function buildLineageGraph (
  apps: readonly InspectedApp[],
  structure: InspectedStructure | undefined,
  traced: Traced | undefined,
  datasetLabels: Readonly<Record<string, string | undefined>> = {},
): LineageGraph {
  const nodes = new Map<string, LineageGraphNode>();
  const edges = new Map<string, LineageGraphEdge>();
  const chips = new Map<string, Set<string>>();

  const node = (n: LineageGraphNode) => {
    const existing = nodes.get(n.id);
    if (existing) return existing;
    nodes.set(n.id, n);
    return n;
  };
  const edge = (source: string, target: string, fields: Iterable<string>, extra: Pick<LineageGraphEdge, 'kind' | 'active'> & { id?: string } = {}) => {
    const id = extra.id ?? `${source}->${target}`;
    const existing = edges.get(id) ?? { id, source, target, fields: [] };
    edges.set(id, { ...existing, ...extra, id, fields: [...new Set([...existing.fields, ...fields])] });
  };
  /** A model by the name a column or the catalog uses, which can be its fqn: one node either way. */
  const modelNode = (name: string, card?: Entity) => {
    const fqn = text(card?.props.fqn) ?? name;
    const id = `model:${fqn}`;
    const model = node({
      id,
      kind: 'model',
      label: card?.label ?? fqn,
      name: text(card?.props.name) ?? fqn,
      description: text(card?.props.description),
    });
    const table = text(card?.props.table_name);
    const dataSource = text(card?.props.data_source_name);
    if (table) {
      const tableId = `table:${dataSource ?? ''}/${table}`;
      node({ id: tableId, kind: 'table', label: table, name: dataSource });
      edge(id, tableId, []);
    }
    return model;
  };

  const queryNodes = entityNodesOf(structure?.nodes ?? [], 'query');
  const controlNodes = entityNodesOf(structure?.nodes ?? [], 'control');
  const queryFields = new Map<string, Set<string>>();
  const appName = (app: InspectedApp, index: number) => (apps.length > 1 ? (app.title ?? `app ${index + 1}`) : undefined);

  apps.forEach((app, appIndex) => {
    for (const [name, query] of Object.entries(app.queries)) {
      const dataset = app.declaration.queries[name]?.dataset;
      const queryKey = `${appIndex}/${name}`;
      const queryId = `query:${queryKey}`;
      node({ id: queryId, kind: 'query', label: name, name: appName(app, appIndex), structureKey: queryNodes.get(queryKey)?.key });
      const fields = new Set<string>();
      queryFields.set(queryId, fields);
      if (!dataset) continue;
      const datasetId = `dataset:${dataset}`;

      for (const column of query.columns) {
        const target = targetOf(column, dataset);
        if (!target) continue;
        const kind = column.isMeasure ? 'metric' : 'dimension';
        const fieldId = `${kind}:${target.parent}.${target.child}`;
        const lineage = traced?.get(targetKey(target));
        node({
          id: fieldId,
          kind,
          label: column.label ?? column.fieldName,
          // A model's field aggregated in the query says how: `customers.id · count_distinct`.
          name: column.modelId
            ? `${column.modelId}.${column.fieldName}${column.isMeasure && column.aggregation ? ` · ${column.aggregation}` : ''}`
            : column.fieldName,
          error: lineage?.error,
        });
        fields.add(fieldId);
        edge(queryId, fieldId, [fieldId]);

        node({ id: datasetId, kind: 'dataset', label: datasetLabels[dataset] ?? dataset, name: dataset });
        edge(fieldId, datasetId, [fieldId]);

        // What the field reads: the catalog's trace, else the model the column names.
        const reads = modelFieldsOf(lineage, target);
        if (reads.length === 0 && column.modelId) reads.push({ model: column.modelId, field: column.fieldName });
        for (const read of reads) {
          const model = modelNode(read.model, read.card);
          edge(datasetId, model.id, [fieldId]);
          const set = chips.get(model.id) ?? new Set();
          set.add(read.field);
          chips.set(model.id, set);
        }
      }
    }
  });

  // A control filters the queries it is mapped to. Like a block, its edge carries the query's fields,
  // so a path runs from the control through them.
  apps.forEach((app, appIndex) => {
    for (const mapping of controlMappings(app)) {
      const queryId = `query:${appIndex}/${mapping.to.name}`;
      if (!nodes.has(queryId)) continue;
      const name = mapping.from.name;
      const controlId = `control:${appIndex}/${name}`;
      // The applied condition is what the queries ran with; the typed one is only pending.
      const control = app.controls[name];
      const isSet = (control?.appliedCondition?.values?.length ?? 0) > 0;
      const value = isSet ? formatCondition(control?.appliedCondition) : 'not set';
      const typed = control?.condition?.values?.length ? formatCondition(control.condition) : 'not set';
      node({
        id: controlId,
        kind: 'control',
        label: name,
        name: appName(app, appIndex),
        structureKey: controlNodes.get(`${appIndex}/${name}`)?.key,
        isSet,
        value,
        pending: control?.isDirty && typed !== value ? typed : undefined,
      });
      const fields = queryFields.get(queryId);
      edge(controlId, queryId, fields?.size ? fields : [queryId], { kind: 'control', active: isSet });
    }
  });

  // A cross-filter links two queries: a selection in the source filters the target. It is active
  // while the reader's applied selection is in the source query.
  apps.forEach((app, appIndex) => {
    for (const cross of crossFilters(app)) {
      const source = `query:${appIndex}/${cross.from.name}`;
      const target = `query:${appIndex}/${cross.to.name}`;
      if (!nodes.has(source) || !nodes.has(target)) continue;
      const fields = queryFields.get(target);
      edge(source, target, fields?.size ? fields : [target], {
        id: `crossFilter:${appIndex}/${cross.id}`,
        kind: 'crossFilter',
        active: app.appliedSelection?.source === cross.from.name,
      });
    }
  });

  // A block draws the queries marked in it, and those the frame says it draws.
  const blocks = blocksOf(structure?.nodes ?? []);
  for (const [usageKey, usage] of Object.entries(structure?.usage ?? {})) {
    if (!usageKey.startsWith('query:')) continue;
    for (const id of usage.blocks) blocks.get(id)?.queries.add(usageKey.slice('query:'.length));
  }
  for (const [id, { node: structureNode, queries }] of blocks) {
    const drawn = [...queries].filter((q) => nodes.has(`query:${q}`));
    if (drawn.length === 0) continue;
    const blockId = `block:${id}`;
    node({ id: blockId, kind: 'block', label: structureNode.label ?? id, name: id, structureKey: structureNode.key });
    for (const q of drawn) {
      const queryId = `query:${q}`;
      // A query that hasn't run has no fields yet: the edge carries the query, so a path still follows it.
      const fields = queryFields.get(queryId);
      edge(blockId, queryId, fields?.size ? fields : [queryId]);
    }
  }

  // What reaches a model reaches its table.
  for (const e of edges.values()) {
    if (!e.target.startsWith('table:')) continue;
    const into = [...edges.values()].filter((x) => x.target === e.source).flatMap((x) => x.fields);
    e.fields = [...new Set(into)];
  }
  for (const [id, set] of chips) {
    const model = nodes.get(id);
    if (model) model.chips = [...set].sort();
  }

  return { nodes: [...nodes.values()], edges: [...edges.values()] };
}

/** What the reader chose to see: the kinds hidden, and whether cross-filters are drawn. */
export interface LineageFilter {
  hidden: ReadonlySet<LineageKind>;
  crossFilters: boolean;
}

/** Whether the view draws cross-filters: a cross-filter joins two queries, so it goes with the toggle, and with the queries. */
export function showsCrossFilters (view: LineageFilter): boolean {
  return view.crossFilters && !view.hidden.has('query');
}

/**
 * The graph without the hidden kinds, joined across them: hiding queries links each block to the
 * fields its queries return. A joined edge carries the fields both its halves carry, and the kind
 * of a filter edge among them: hiding queries keeps a control's edge a control's.
 */
export function filterGraph (graph: LineageGraph, view: LineageFilter): LineageGraph {
  const { hidden } = view;
  const dropCross = !showsCrossFilters(view);
  if (hidden.size === 0 && !dropCross) return graph;
  let nodes = graph.nodes;
  let edges = dropCross ? graph.edges.filter((e) => e.kind !== 'crossFilter') : graph.edges;
  for (const removed of graph.nodes.filter((n) => hidden.has(n.kind))) {
    const into = edges.filter((e) => e.target === removed.id);
    const out = edges.filter((e) => e.source === removed.id);
    const kept = new Map(edges.filter((e) => e.source !== removed.id && e.target !== removed.id).map((e) => [e.id, e]));
    /** Adds an edge: one of the same kind merges, one of another kind stays beside it under its own id. */
    const put = (e: LineageGraphEdge): void => {
      const existing = kept.get(e.id);
      if (!existing) {
        kept.set(e.id, e);
      } else if (existing.kind === e.kind) {
        kept.set(e.id, {
          ...existing,
          fields: [...new Set([...existing.fields, ...e.fields])],
          ...(e.kind ? { active: Boolean(existing.active || e.active) } : {}),
        });
      } else if (e.kind) {
        put({ ...e, id: `${e.source}->${e.target}:${e.kind}` });
      } else {
        // The plain edge takes the plain id; the filter edge that held it moves to its own.
        kept.set(e.id, e);
        put({ ...existing, id: `${existing.source}->${existing.target}:${existing.kind}` });
      }
    };
    for (const a of into) {
      for (const b of out) {
        // An edge with no fields (a model to its table before anything reads it) passes everything.
        const fields = a.fields.length === 0 ? b.fields : b.fields.length === 0 ? a.fields : a.fields.filter((f) => b.fields.includes(f));
        if (fields.length === 0 && a.fields.length && b.fields.length) continue;
        // A joined edge keeps the kind of the edge into the removed node, else of the one out of it.
        const from = a.kind ? a : b;
        put({
          id: `${a.source}->${b.target}`,
          source: a.source,
          target: b.target,
          fields,
          ...(from.kind ? { kind: from.kind, active: from.active } : {}),
        });
      }
    }
    nodes = nodes.filter((n) => n.id !== removed.id);
    edges = [...kept.values()];
  }
  return { nodes, edges };
}

/**
 * A node's path: everything upstream and downstream of it, following only the edges that carry
 * what it carries. A metric's path reaches the models it reads, not every model of its dataset.
 */
export function pathOf (graph: LineageGraph, id: string): { nodes: Set<string>, edges: Set<string> } {
  // What a node filters is not what it is built from: the fields of a cross-filter it sources are
  // its target's, so they are not carried.
  const filtersOut = (e: LineageGraphEdge) => e.source === id && e.kind === 'crossFilter';
  const incident = graph.edges.filter((e) => (e.source === id || e.target === id) && !filtersOut(e));
  const carried = new Set(incident.flatMap((e) => e.fields));
  const node = graph.nodes.find((n) => n.id === id);
  if (node?.kind === 'metric' || node?.kind === 'dimension') carried.add(id);
  const follows = (e: LineageGraphEdge) => filtersOut(e) || e.fields.length === 0 || e.fields.some((f) => carried.has(f));

  const edges = new Set<string>();
  const filtered = new Set<string>();
  // Down is toward the warehouse (what it is built from); up is toward the page (what uses it).
  // The way down stops at a node reached over a cross-filter: it shows what the node filters, not
  // what that is built from.
  const walk = (from: string, down: boolean, seen: Set<string>) => {
    for (const e of graph.edges) {
      if ((down ? e.source : e.target) !== from || !follows(e)) continue;
      edges.add(e.id);
      const next = down ? e.target : e.source;
      if (down && e.kind === 'crossFilter') {
        filtered.add(next);
        continue;
      }
      if (seen.has(next)) continue;
      seen.add(next);
      walk(next, down, seen);
    }
    return seen;
  };
  const nodes = new Set([...walk(id, true, new Set([id])), ...walk(id, false, new Set([id])), ...filtered]);
  return { nodes, edges };
}

/** The node and everything on its path: the rest of the graph hidden. */
export function focusGraph (graph: LineageGraph, id: string): LineageGraph {
  if (!graph.nodes.some((n) => n.id === id)) return graph;
  const path = pathOf(graph, id);
  return {
    nodes: graph.nodes.filter((n) => path.nodes.has(n.id)),
    edges: graph.edges.filter((e) => path.edges.has(e.id)),
  };
}

/** A view of the graph in one click: the kinds hidden, and whether cross-filters are drawn. */
export interface LineagePreset {
  id: string;
  label: string;
  hidden: LineageKind[];
  crossFilters: boolean;
}

export const LINEAGE_PRESETS: LineagePreset[] = [
  { id: 'default', label: 'Default', hidden: [], crossFilters: false },
  { id: 'everything', label: 'Everything', hidden: [], crossFilters: true },
  { id: 'interaction', label: 'Interaction mapping', hidden: ['metric', 'dimension', 'dataset', 'model', 'table'], crossFilters: true },
  { id: 'query', label: 'Query lineage', hidden: ['control'], crossFilters: false },
];

/** The preset a view is exactly, or none once the reader has changed it. */
export function matchPreset (view: LineageFilter): LineagePreset | undefined {
  return LINEAGE_PRESETS.find((p) => p.crossFilters === view.crossFilters && p.hidden.length === view.hidden.size && p.hidden.every((k) => view.hidden.has(k)));
}

/** How many things the graph has that the view hides: each kind with nodes, and its cross-filters, if any, when not drawn. */
export function hiddenCount (view: LineageFilter, counts: Partial<Record<LineageKind, number>>, crossFilterCount: number): number {
  const kinds = LINEAGE_KINDS.filter((k) => counts[k] && view.hidden.has(k)).length;
  return kinds + (crossFilterCount > 0 && !showsCrossFilters(view) ? 1 : 0);
}

/**
 * The nodes in the order to lay them out: queries joined by a cross-filter next to each other, so
 * the layout stacks them together. Every other node keeps its place.
 */
export function orderForLayout (graph: LineageGraph): LineageGraphNode[] {
  const links = graph.edges.filter((e) => e.kind === 'crossFilter');
  if (links.length === 0) return graph.nodes;
  const next = new Map<string, string[]>();
  for (const { source, target } of links) {
    next.set(source, [...(next.get(source) ?? []), target]);
    next.set(target, [...(next.get(target) ?? []), source]);
  }
  const queries = graph.nodes.filter((n) => n.kind === 'query');
  const placed = new Set<string>();
  const ordered: LineageGraphNode[] = [];
  const visit = (n: LineageGraphNode) => {
    if (placed.has(n.id)) return;
    placed.add(n.id);
    ordered.push(n);
    for (const id of next.get(n.id) ?? []) {
      const linked = queries.find((q) => q.id === id);
      if (linked) visit(linked);
    }
  };
  queries.forEach(visit);
  // The queries take the slots the queries had, in their new order.
  return graph.nodes.map((n) => (n.kind === 'query' ? ordered.shift()! : n));
}
