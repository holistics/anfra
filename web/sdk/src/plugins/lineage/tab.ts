/**
 * Lineage: where each number on the page comes from, as a canvas, from the controls and blocks to
 * the database tables. Click a card to pin its path; hide kinds, pick a preset, or focus on one
 * card's path. Shown with a node selected (a link from another tab, or a pick), it pins the first
 * card it draws in that node's subtree. A pinned block or query is outlined on the page.
 */
import type { LineageResult, StructureNode } from '../../common/types';
import type { TabContext } from '../../host/contributions';
import { findNode } from '../inspect/model/format';
import {
  h, icon, control, popover, replace, STYLES, tooltips, type Children, type IconName,
} from '../inspect/view';
import { createCanvas, type Canvas } from './canvas';
import {
  buildLineageGraph, filterGraph, focusGraph, hiddenCount, LINEAGE_KINDS, LINEAGE_PRESETS, lineageTargets, matchPreset,
  showsCrossFilters, type LineageGraph, type LineageGraphNode, type LineageKind,
} from './model';
import { stateOf, traceKey } from './state';

const STRUCTURE_TAB = 'inspect.structure';
const MENU_GAP = 4;
const MENU_MARGIN = 8;

/** Each kind's name on its card, as the reader knows it. */
export const KIND_LABELS: Record<LineageKind, string> = {
  control: 'Control',
  block: 'Block',
  query: 'Query',
  metric: 'Metric',
  dimension: 'Dimension',
  dataset: 'Dataset',
  model: 'Model',
  table: 'Database table',
};

const KIND_ICONS: Record<LineageKind, IconName> = {
  control: 'control',
  block: 'block',
  query: 'query',
  metric: 'metric',
  dimension: 'dimension',
  dataset: 'dataset',
  model: 'model',
  table: 'table',
};

/** The key of `key`'s node and of every node under it, in tree order. */
function subtreeKeys (nodes: StructureNode[], key: string): string[] {
  const node = findNode(nodes, key);
  if (!node) return [];
  const out: string[] = [];
  const walk = (n: StructureNode) => {
    out.push(n.key);
    n.children.forEach(walk);
  };
  walk(node);
  return out;
}

export default function lineageTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const state = stateOf(context.mount);
  const { view } = state;
  const hasStructure = context.plugins.some((plugin) => plugin.name === 'inspect');
  const datasetLabels = Object.fromEntries(Object.entries(context.mount.datasets).map(([name, dataset]) => [name, dataset.label]));
  /** A failed trace is traced again once per mount of the tab. */
  let retried = false;
  /** The selection the tab last landed a pin for: a new one lands again. */
  let landed: string | null | undefined;
  /** The last selection landed on nothing the canvas draws: said, so the switch of tab isn't all that happens. */
  let missed = false;
  let menu: 'panel' | 'expanded' | undefined;
  let outlined: string | undefined;

  const select = (id: string | undefined) => {
    view.selected = id;
    state.changed();
  };
  const focus = (id: string) => {
    view.focus = id;
    view.selected = id;
    state.changed();
  };

  function card (node: LineageGraphNode): Children {
    const onPage = (node.kind === 'block' || node.kind === 'query' || node.kind === 'control') && node.structureKey !== undefined;
    const action = (label: string, name: IconName, run: () => void, testid: string) => h(doc, 'button', {
      type: 'button', ...control('row-action'), 'data-tip': label, 'aria-label': label, 'data-testid': testid,
      onclick: (event: Event) => {
        event.stopPropagation();
        run();
      },
    }, icon(doc, name));
    return [
      h(doc, 'div', { class: 'lkind' }, h(doc, 'span', { class: 'badge', 'aria-hidden': 'true' }, icon(doc, KIND_ICONS[node.kind])), KIND_LABELS[node.kind]),
      h(doc, 'div', { class: `ltitle${node.kind === 'table' ? ' mono' : ''}` }, node.label),
      node.name && node.name !== node.label && h(doc, 'div', { class: 'lname' }, node.name),
      node.value !== undefined && h(doc, 'div', { class: `lvalue${node.isSet ? '' : ' unset'}` }, h(doc, 'span', { class: 'lvalue-label' }, 'Value'), h(doc, 'span', { class: 'mono' }, node.value)),
      node.pending && h(doc, 'div', { class: 'lname wrap' }, `pending: ${node.pending}`),
      node.description && h(doc, 'p', { class: 'ldesc' }, node.description),
      !!node.chips?.length && h(doc, 'div', { class: 'chips' }, node.chips.map((chip) => h(doc, 'span', { class: 'chip' }, chip))),
      node.error && h(doc, 'div', { class: 'lerror' }, `Lineage: ${node.error}`),
      h(doc, 'div', { class: 'lactions' },
        onPage && action('Locate on the page', 'locate', () => panel.locate({ node: node.structureKey! }), 'card-locate'),
        onPage && hasStructure && action('Show in Structure', 'tree', () => {
          expanded.close();
          panel.show(STRUCTURE_TAB, node.structureKey!);
        }, 'card-structure'),
        action('Focus on its lineage', 'focus', () => focus(node.id), 'card-focus')),
    ];
  }

  const legendRow = (stroke: string, title: string, text: string) => h(doc, 'div', { class: 'legend-row' },
    h(doc, 'div', { class: 'legend-title' }, h(doc, 'span', { class: `legend-line ${stroke}` }), title),
    h(doc, 'p', {}, text));
  const legend = () => [
    legendRow('uses', 'Is built from', 'Points from what uses to what it uses: a block draws a query, a query returns a field, a field comes from a dataset, a model, a table.'),
    legendRow('filter-on', 'Filters, with a value', "A control the reader has set, mapped to a query: its value changes that query's numbers. The control is not part of where they come from."),
    legendRow('filter-off', 'Filters, not set', 'The control is mapped to the query but has no value yet, so it changes nothing for now.'),
    legendRow('filter-on', 'Cross-filter, active', "A selection in the source query is applied, so it filters the target query's numbers."),
    legendRow('filter-off', 'Cross-filter, inactive', 'The queries are linked but nothing is selected in the source yet. Shown when cross-filters are on.'),
  ];

  /** The toolbar's inputs, drawn again only when they change, so an open select stays open. */
  function toolbar (where: 'panel' | 'expanded', graph: LineageGraph, focused: LineageGraphNode | undefined, loading: boolean): { key: string, draw: () => Children } {
    const counts: Partial<Record<LineageKind, number>> = {};
    for (const node of graph.nodes) counts[node.kind] = (counts[node.kind] ?? 0) + 1;
    const crossCount = graph.edges.filter((e) => e.kind === 'crossFilter').length;
    const preset = matchPreset(view);
    const hiddenTotal = hiddenCount(view, counts, crossCount);
    const crossDrawn = showsCrossFilters(view);
    const key = JSON.stringify([where, counts, crossCount, preset?.id, [...view.hidden].sort(), view.crossFilters, focused?.id, focused?.label, loading, menu === where]);
    const toggle = (kind: LineageKind) => {
      if (view.hidden.has(kind)) view.hidden.delete(kind);
      else view.hidden.add(kind);
      state.changed();
    };
    const row = (shown: boolean, onclick: () => void, badge: Children, label: string, count: string, testid: string) => h(doc, 'button', {
      type: 'button', class: 'a-menu-item', role: 'menuitemcheckbox', 'aria-checked': String(shown), 'data-testid': testid, onclick,
    }, badge, label, h(doc, 'span', { class: 'count' }, count), icon(doc, shown ? 'eye' : 'hidden'));
    const draw = () => [
      h(doc, 'div', { class: 'ltools' },
        h(doc, 'select', {
          class: 'a-input preset',
          'aria-label': 'Preset',
          'data-testid': 'lineage-preset',
          onchange: (event: Event) => {
            const picked = LINEAGE_PRESETS.find((p) => p.id === (event.target as HTMLSelectElement).value);
            if (!picked) return;
            view.hidden = new Set(picked.hidden);
            view.crossFilters = picked.crossFilters;
            state.changed();
          },
        },
        !preset && h(doc, 'option', { value: 'custom', disabled: true, selected: true }, 'Custom'),
        LINEAGE_PRESETS.map((p) => h(doc, 'option', { value: p.id, selected: p.id === preset?.id }, p.label))),
        h(doc, 'div', { class: 'menu-wrap' },
          h(doc, 'button', {
            type: 'button',
            ...control('tool'),
            'aria-haspopup': 'menu',
            'aria-expanded': String(menu === where),
            'data-testid': 'lineage-show',
            onclick: () => {
              menu = menu === where ? undefined : where;
              redraw();
            },
          }, icon(doc, hiddenTotal > 0 ? 'hidden' : 'eye'), 'Show', hiddenTotal > 0 && h(doc, 'span', { class: 'muted' }, `${hiddenTotal} hidden`)),
          menu === where && h(doc, 'div', { class: 'a-menu menu', role: 'menu', popover: 'manual', 'data-testid': 'lineage-show-menu' },
            LINEAGE_KINDS.filter((kind) => counts[kind]).map((kind) => row(
              !view.hidden.has(kind), () => toggle(kind),
              h(doc, 'span', { class: `badge kind-${kind}`, 'aria-hidden': 'true' }, icon(doc, KIND_ICONS[kind])),
              KIND_LABELS[kind], String(counts[kind]), `show-${kind}`,
            )),
            crossCount > 0 && row(
              crossDrawn, () => {
                view.crossFilters = !view.crossFilters;
                state.changed();
              },
              h(doc, 'span', { class: 'badge kind-control', 'aria-hidden': 'true' }, icon(doc, 'filter')),
              'Cross-filters', view.crossFilters && !crossDrawn ? 'with queries' : String(crossCount), 'show-cross-filters',
            ))),
        h(doc, 'span', { class: 'spacer' }),
        loading && h(doc, 'span', { class: 'dot executing', role: 'img', 'aria-label': 'Tracing', 'data-tip': 'Tracing the fields through the catalog…' }),
        popover(doc, icon(doc, 'help'), legend, { ...control('row-action', 'pop-trigger'), 'aria-label': 'What the arrows mean' }),
        where === 'panel' && h(doc, 'button', {
          type: 'button', ...control('row-action'), 'data-tip': 'Open in a large view', 'aria-label': 'Open in a large view', 'data-testid': 'lineage-expand',
          onclick: () => {
            menu = undefined;
            expanded.open();
          },
        }, icon(doc, 'expand'))),
      focused && h(doc, 'div', { class: 'focus-row', 'data-testid': 'lineage-focus' },
        h(doc, 'span', { class: 'muted' }, 'Focused on'),
        h(doc, 'span', { class: 'focus-chip' },
          h(doc, 'span', { class: 'focus-label' }, `${KIND_LABELS[focused.kind]}: ${focused.label}`),
          h(doc, 'button', {
            type: 'button', ...control('row-action'), 'aria-label': 'Show everything', 'data-testid': 'lineage-unfocus',
            onclick: () => {
              view.focus = undefined;
              state.changed();
            },
          }, icon(doc, 'close')))),
    ];
    return { key, draw };
  }

  /**
   * The Show menu, in the top layer where the browser has one, so the panel's edge doesn't clip
   * it: under its button, or above it, inside the window.
   */
  function placeMenu (element: HTMLElement): void {
    const win = doc.defaultView;
    const button = element.parentElement?.querySelector('button');
    if (!win || !button) return;
    element.showPopover?.();
    const at = button.getBoundingClientRect();
    const size = element.getBoundingClientRect();
    const below = at.bottom + MENU_GAP;
    const above = at.top - MENU_GAP - size.height;
    const top = below + size.height <= win.innerHeight - MENU_MARGIN || above < MENU_MARGIN
      ? Math.max(MENU_MARGIN, Math.min(below, win.innerHeight - MENU_MARGIN - size.height))
      : above;
    element.style.left = `${Math.max(MENU_MARGIN, Math.min(at.left, win.innerWidth - MENU_MARGIN - size.width))}px`;
    element.style.top = `${top}px`;
  }

  /** One canvas and the toolbar over it: the panel's, and the large view's. */
  function surface (where: 'panel' | 'expanded', minFitZoom: number) {
    const head = h(doc, 'div', { class: 'ltoolbar' });
    const canvas: Canvas = createCanvas(doc, { minFitZoom, card, onSelect: select });
    let drawnKey: string | undefined;
    return {
      head,
      canvas,
      draw (graph: LineageGraph, shown: LineageGraph, focused: LineageGraphNode | undefined, loading: boolean) {
        const bar = toolbar(where, graph, focused, loading);
        if (bar.key !== drawnKey) {
          drawnKey = bar.key;
          replace(doc, head, bar.draw());
          const opened = head.querySelector<HTMLElement>('.menu');
          if (opened) placeMenu(opened);
        }
        canvas.draw(shown, view.selected);
      },
    };
  }

  const panelSurface = surface('panel', 0.6);
  const notes = h(doc, 'div', { class: 'notes' });
  const empty = h(doc, 'div', { class: 'body' });
  const largeSurface = surface('expanded', 0.2);
  const dialog = h(doc, 'dialog', { class: 'expanded', 'aria-label': 'Lineage', 'data-testid': 'lineage-expanded' },
    h(doc, 'div', { class: 'dialog-head' },
      h(doc, 'div', {},
        h(doc, 'h3', {}, 'Lineage'),
        h(doc, 'p', { class: 'muted' }, "From the page's blocks to the database tables. Click a card to follow its path.")),
      h(doc, 'button', { type: 'button', ...control('row-action'), 'aria-label': 'Close', 'data-testid': 'lineage-close', onclick: () => expanded.close() }, icon(doc, 'close'))),
    largeSurface.head,
    largeSurface.canvas.element) as HTMLDialogElement;
  const expanded = {
    isOpen: () => dialog.open,
    open () {
      if (dialog.open) return;
      if (typeof dialog.showModal === 'function') dialog.showModal();
      else dialog.setAttribute('open', '');
      redraw();
    },
    close () {
      if (!dialog.open) return;
      if (typeof dialog.close === 'function') dialog.close();
      else dialog.removeAttribute('open');
    },
  };
  dialog.addEventListener('close', () => {
    if (menu === 'expanded') menu = undefined;
    redraw();
  });

  const pane = h(doc, 'div', { class: 'pane' }, panelSurface.head, notes, empty, panelSurface.canvas.element, dialog);
  root.append(h(doc, 'style', {}, STYLES, LINEAGE_STYLES), pane);
  const tooltip = tooltips(root);

  function redraw (): void {
    const snapshot = panel.snapshot;
    const apps = snapshot?.apps;
    const structure = snapshot?.structure;
    const targets = lineageTargets(apps ?? []);
    if (apps) {
      state.traceFor(targets, (asked) => context.call<LineageResult>('lineage.trace', { targets: asked }), !retried);
      retried = true;
    }
    const trace = state.trace?.key === traceKey(targets) ? state.trace : undefined;
    const graph = buildLineageGraph(apps ?? [], structure, trace?.traced, datasetLabels);
    const filtered = filterGraph(graph, view);
    const focused = view.focus ? filtered.nodes.find((n) => n.id === view.focus) : undefined;
    const shown = focused ? focusGraph(filtered, focused.id) : filtered;
    const loading = trace?.status === 'loading';

    // A selection lands once the graph it is judged against is whole: the page, and this trace.
    if (panel.selection !== landed && (panel.selection === null || (structure && apps && trace && !loading))) {
      landed = panel.selection;
      if (landed !== null && structure) {
        const keys = subtreeKeys(structure.nodes, landed);
        const hit = keys.map((key) => shown.nodes.find((n) => n.structureKey === key)).find((n) => n !== undefined);
        missed = !hit;
        if (hit) {
          view.selected = hit.id;
          queueMicrotask(() => panelSurface.canvas.center(hit.id));
        }
      }
    }

    const waiting = !apps;
    const nothing = !waiting && graph.nodes.length === 0;
    panelSurface.head.hidden = waiting || nothing;
    panelSurface.canvas.element.hidden = waiting || nothing;
    empty.hidden = !(waiting || nothing);
    replace(doc, empty, waiting
      ? h(doc, 'p', { class: 'note', 'data-testid': 'lineage-waiting' }, 'Waiting for the Data App…')
      : nothing && h(doc, 'p', { class: 'note', 'data-testid': 'lineage-none' }, "Nothing to trace yet. The lineage starts from the fields the app's queries return: it fills in once they have run."));
    replace(doc, notes,
      trace?.status === 'failed' && h(doc, 'div', { class: 'banner warn', role: 'alert', 'data-testid': 'lineage-failed' },
        icon(doc, 'info'), h(doc, 'span', {}, `Couldn't trace lineage: ${trace.error}. Fields stop at their dataset, or at the model their query names.`)),
      missed && !waiting && h(doc, 'div', { class: 'banner', 'data-testid': 'lineage-missed' },
        icon(doc, 'info'),
        h(doc, 'span', {}, 'Nothing you picked is drawn here: it holds no query or control, or the current filter or focus hides it.'),
        h(doc, 'button', {
          type: 'button', ...control('row-action'), 'aria-label': 'Dismiss',
          onclick: () => {
            missed = false;
            redraw();
          },
        }, icon(doc, 'close'))));
    if (!waiting && !nothing) panelSurface.draw(graph, shown, focused, loading);
    if (expanded.isOpen()) largeSurface.draw(graph, shown, focused, loading);
    tooltip.redrawn();

    // A pinned block or query is outlined on the page, until the pin moves or the tab closes.
    const pinned = view.selected ? shown.nodes.find((n) => n.id === view.selected) : undefined;
    const outline = pinned?.kind === 'block' || pinned?.kind === 'query' ? pinned.structureKey : undefined;
    if (outline !== outlined) {
      outlined = outline;
      panel.highlight(outline ?? null);
    }
  }

  // Escape closes the Show menu, else lets go of the pin; in the large view it closes that, keeping the pin.
  const onKey = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return;
    if (menu) {
      menu = undefined;
      redraw();
    } else if (view.selected && !expanded.isOpen()) {
      select(undefined);
    }
  };
  // A press outside the Show menu closes it.
  const onPress = (event: Event) => {
    if (!menu || event.composedPath().some((node) => node instanceof Element && node.classList.contains('menu-wrap'))) return;
    menu = undefined;
    redraw();
  };
  // The menu is placed once, so a resize or a scroll of the page closes it.
  const onMove = (event: Event) => {
    if (!menu || (event.target instanceof Node && root.contains(event.target) && (event.target as Element).closest?.('.menu'))) return;
    menu = undefined;
    redraw();
  };
  const win = doc.defaultView;
  doc.addEventListener('keydown', onKey);
  win?.addEventListener('resize', onMove);
  win?.addEventListener('scroll', onMove, true);
  root.addEventListener('pointerdown', onPress, true);
  redraw();
  const stops = [panel.subscribe(redraw), state.subscribe(redraw)];
  return () => {
    stops.forEach((stop) => stop());
    doc.removeEventListener('keydown', onKey);
    win?.removeEventListener('resize', onMove);
    win?.removeEventListener('scroll', onMove, true);
    root.removeEventListener('pointerdown', onPress, true);
    expanded.close();
    panelSurface.canvas.dispose();
    largeSurface.canvas.dispose();
    tooltip.dispose();
    if (outlined) panel.highlight(null);
    root.replaceChildren();
  };
}

/** What the Lineage tab adds to `inspect`'s stylesheet. Each kind's accent is a fixed colour, as in ahost. */
const LINEAGE_STYLES = `
.pane { position: relative; }
.mono { font-family: var(--anfra-font-mono); }
.wrap { overflow-wrap: anywhere; }
.kind-control { --kind: oklch(0.705 0.187 47.6); } .kind-block { --kind: oklch(0.585 0.204 277.1); } .kind-query { --kind: oklch(0.696 0.149 162.5); } .kind-metric { --kind: oklch(0.704 0.123 182.5); }
.kind-dimension { --kind: oklch(0.685 0.148 237.3); } .kind-dataset { --kind: oklch(0.769 0.165 70.1); } .kind-model { --kind: oklch(0.606 0.219 292.7); } .kind-table { --kind: oklch(0.554 0.041 257.4); }
.badge { flex: none; display: grid; place-items: center; width: 20px; height: 20px; border-radius: 4px; background: color-mix(in srgb, var(--kind) 16%, transparent); color: var(--kind); }
/* Toolbar */
.ltoolbar { flex: none; position: relative; z-index: 3; padding: 8px 14px; border-bottom: 1px solid var(--anfra-color-border); }
.ltools { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.preset { width: auto; min-width: 0; max-width: 180px; height: 24px; padding: 0 6px; background: var(--anfra-color-bg); font-size: 12px; line-height: 16px; }
.menu-wrap { position: relative; }
/* The class layer's Menu, placed in the top layer. */
.menu { position: fixed; inset: auto; margin: 0; z-index: 10; min-width: 220px; max-height: calc(100vh - 16px); border: 0; }
.menu .a-menu-item { gap: 8px; cursor: pointer; }
.menu .a-menu-item[aria-checked="false"] { color: var(--anfra-color-muted); }
.menu button[aria-checked="false"] .badge { background: var(--hover); color: var(--anfra-color-muted); }
.menu .badge { width: 16px; height: 16px; }
.menu .badge .icon { width: 12px; height: 12px; }
.count { margin-left: auto; color: var(--anfra-color-muted); font-variant-numeric: tabular-nums; }
.focus-row { display: flex; align-items: center; gap: 6px; margin-top: 6px; font-size: 12px; }
.focus-chip { display: inline-flex; align-items: center; gap: 2px; min-width: 0; padding: 0 2px 0 8px; border-radius: 6px; background: color-mix(in srgb, oklch(0.606 0.219 292.7) 16%, transparent); font-weight: 500; }
.focus-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* The legend's trigger is a popover's and a row action's: centred as a row action. */
.pop-trigger.row-action { display: grid; place-items: center; }
.legend-row + .legend-row { margin-top: 10px; }
.legend-title { display: flex; align-items: center; gap: 8px; font-weight: 600; }
.legend-row p { margin: 2px 0 0; color: var(--anfra-color-muted); }
.legend-line { flex: none; width: 40px; height: 0; border-top: 2px solid; }
.legend-line.uses { border-color: oklch(0.709 0.159 293.5); } .legend-line.filter-on { border-color: oklch(0.758 0.159 55.9); } .legend-line.filter-off { border-color: light-dark(oklch(0.928 0.006 264.5), oklch(0.446 0.026 256.8)); }
/* Notes */
.notes { flex: none; }
.banner { display: flex; align-items: flex-start; gap: 6px; padding: 6px 14px; border-bottom: 1px solid var(--anfra-color-border); background: var(--subtle); color: var(--anfra-color-muted); font-size: 12px; }
.banner.warn { background: var(--warn-bg); color: var(--anfra-color-fg); }
.banner.warn .icon { color: var(--warn); }
.banner > span { flex: 1; min-width: 0; }
.banner .icon { margin-top: 2px; }
.banner .row-action { margin: -3px 0; }
/* Canvas */
.canvas { flex: 1; min-height: 0; position: relative; overflow: hidden; cursor: grab; touch-action: none; user-select: none;
  background-color: var(--subtle); background-image: radial-gradient(circle, color-mix(in srgb, var(--anfra-color-muted) 40%, transparent) 1px, transparent 1.2px); }
.canvas.panning { cursor: grabbing; }
.world { position: absolute; left: 0; top: 0; transform-origin: 0 0; }
.edges { position: absolute; left: 0; top: 0; overflow: visible; pointer-events: none; }
.edge { fill: none; stroke-width: 1.25; }
.edge.on { stroke-width: 2; } .edge.dim { opacity: 0.15; }
.edge.uses { stroke: oklch(0.709 0.159 293.5); } .edge.uses.on { stroke: oklch(0.541 0.247 293); }
.edge.filter-on { stroke: oklch(0.758 0.159 55.9); } .edge.filter-on.on { stroke: oklch(0.646 0.194 41.1); }
.edge.filter-off { stroke: light-dark(oklch(0.928 0.006 264.5), oklch(0.446 0.026 256.8)); } .edge.filter-off.on { stroke: oklch(0.714 0.019 261.3); }
.m.uses path { fill: oklch(0.709 0.159 293.5); } .m.uses.on path { fill: oklch(0.541 0.247 293); }
.m.filter-on path { fill: oklch(0.758 0.159 55.9); } .m.filter-on.on path { fill: oklch(0.646 0.194 41.1); }
.m.filter-off path { fill: light-dark(oklch(0.928 0.006 264.5), oklch(0.446 0.026 256.8)); } .m.filter-off.on path { fill: oklch(0.714 0.019 261.3); }
.lcard { position: absolute; left: 0; top: 0; width: 240px; padding: 8px 12px 10px; border: 1px solid var(--anfra-color-border); border-left: 3px solid var(--kind); border-radius: 8px;
  background: var(--anfra-color-bg); box-shadow: 0 1px 2px oklch(0 0 0 / 0.06); cursor: pointer; transition: opacity 0.15s, box-shadow 0.15s; }
.lcard.selected { box-shadow: 0 0 0 2px color-mix(in srgb, oklch(0.606 0.219 292.7) 70%, transparent); }
.lcard.on-path { border-top-color: oklch(0.811 0.101 293.6); border-right-color: oklch(0.811 0.101 293.6); border-bottom-color: oklch(0.811 0.101 293.6); }
.lcard.dim { opacity: 0.3; }
.lkind { display: flex; align-items: center; gap: 6px; font-size: 10px; font-weight: 500; letter-spacing: 0.06em; text-transform: uppercase; color: var(--anfra-color-muted); }
.ltitle { margin-top: 4px; font-size: 13px; font-weight: 600; line-height: 1.25; overflow-wrap: anywhere; }
.lname { margin-top: 2px; font-family: var(--anfra-font-mono); font-size: 11px; color: var(--anfra-color-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.lname.wrap { white-space: normal; }
.lvalue { display: flex; align-items: baseline; gap: 6px; margin-top: 6px; padding: 3px 8px; border-radius: 4px; background: var(--hover); font-size: 12px; overflow-wrap: anywhere; }
.lvalue.unset { color: var(--anfra-color-muted); font-style: italic; }
.lvalue-label { flex: none; font-size: 10px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--anfra-color-muted); font-style: normal; }
.ldesc { margin: 4px 0 0; font-size: 12px; line-height: 1.35; color: var(--anfra-color-muted); display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.chips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.chip { padding: 1px 6px; border-radius: 4px; background: color-mix(in srgb, oklch(0.606 0.219 292.7) 16%, transparent); color: light-dark(oklch(0.491 0.241 292.6), oklch(0.811 0.101 293.6)); font-family: var(--anfra-font-mono); font-size: 10px; }
.lerror { margin-top: 6px; font-size: 11px; color: var(--warn); overflow-wrap: anywhere; }
.lactions { position: absolute; top: -12px; right: 8px; display: none; gap: 2px; padding: 1px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: var(--anfra-color-bg); box-shadow: 0 1px 2px oklch(0 0 0 / 0.08); }
.lcard:hover .lactions, .lcard:focus-within .lactions { display: flex; }
.controls { position: absolute; left: 10px; bottom: 10px; z-index: 2; display: flex; flex-direction: column; padding: 2px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: var(--anfra-color-bg); }
/* The large view */
dialog.expanded { width: 95vw; height: 90vh; max-width: 95vw; max-height: 90vh; padding: 0; border: 1px solid var(--anfra-color-border); border-radius: 10px; background: var(--anfra-color-bg); color: var(--anfra-color-fg); overflow: hidden; }
dialog.expanded[open] { display: flex; flex-direction: column; }
dialog.expanded::backdrop { background: oklch(0 0 0 / 0.4); }
.dialog-head { flex: none; display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--anfra-color-border); }
.dialog-head h3 { margin: 0; font-size: 15px; }
.dialog-head p { margin: 2px 0 0; font-size: 12px; }
`;

