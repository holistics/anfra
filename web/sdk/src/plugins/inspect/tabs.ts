/**
 * `inspect`'s tabs, loaded the first time a host shows one. Structure shows the page's semantic
 * parts as a tree, with hover-highlight, click-to-locate, pick, and the selected node's detail;
 * Data shows every app's queries, controls and selection. Both draw from the panel (its snapshot
 * and selection), so a node selected in one is the node the other shows.
 */
import type {
  InspectedApp, InspectedControl, InspectedQuery, InspectedSelection, LocateTarget, RenderFormat, RenderRequest, Rendering,
  StructureNode,
} from '../../common/types';
import type { TabContext } from '../../host/contributions';
import {
  STYLES, copyText, countProblems, entityAttribute, findNode, formatCondition, formatTime, h, handleFor, icon, usageOf,
} from './view';

export const DATA_TAB = 'inspect.data';

/** Draw a tab: the stylesheet, then `draw` again on each change to the panel, keeping the scroll. */
function drawTab (root: ShadowRoot, context: TabContext, draw: () => HTMLElement): () => void {
  const doc = context.document;
  const style = h(doc, 'style', {}, STYLES);
  let pane: HTMLElement | undefined;
  const redraw = () => {
    const scroll = [...(pane?.querySelectorAll('.body, .detail') ?? [])].map((element) => element.scrollTop);
    const next = draw();
    if (pane) pane.replaceWith(next);
    else root.append(style, next);
    pane = next;
    pane.querySelectorAll('.body, .detail').forEach((element, index) => { element.scrollTop = scroll[index] ?? 0; });
  };
  redraw();
  return context.panel.subscribe(redraw);
}

/** A button whose label says "Copied" for a moment after it copies. */
function copyButton (doc: Document, className: string, text: () => string, labels: { idle: Child, done: Child }, attributes: Record<string, string> = {}): HTMLElement {
  const button = h(doc, 'button', { type: 'button', class: className, ...attributes }, labels.idle);
  button.addEventListener('click', async (event) => {
    event.stopPropagation();
    if (!await copyText(doc, text())) return;
    button.replaceChildren(labels.done);
    setTimeout(() => button.replaceChildren(labels.idle), 1500);
  });
  return button;
}
type Child = Node | string;

/** One query's or control's card: its state and what it is drawn by. */
function entityCard (context: TabContext, app: number, name: string, entity: { query?: InspectedQuery, control?: InspectedControl }): HTMLElement {
  const doc = context.document;
  const { query, control } = entity;
  const kind = query ? 'query' : 'control';
  const usage = usageOf(context.panel.snapshot?.structure, kind, app, name);
  const attribute = entityAttribute(kind, app, name);
  const drawn = !usage ? 'Not drawn yet'
    : `In ${usage.markers} place${usage.markers === 1 ? '' : 's'}${usage.blocks.length ? `: ${usage.blocks.join(', ')}` : ', outside any block'}`;
  const locate = () => context.panel.locate({ app, kind, name } as LocateTarget);
  const debug = query?.debug;
  return h(doc, 'article', { class: 'card', 'data-entity': `${kind}:${app}/${name}`, 'data-testid': `inspect-${kind}-${name}` },
    h(doc, 'header', {},
      h(doc, 'strong', {}, name),
      query && h(doc, 'span', { class: `pill ${query.state}`, 'data-testid': 'query-state' }, query.state),
      query?.isDirty && h(doc, 'span', { class: 'pill warn' }, 'dirty'),
      control && h(doc, 'span', { class: 'pill' }, control.kind === 'dateDrill' ? 'Date drill' : 'Filter'),
      control?.isDirty && h(doc, 'span', { class: 'pill warn' }, 'unapplied'),
      h(doc, 'button', {
        type: 'button',
        class: 'link locate',
        disabled: !usage,
        title: usage ? 'Scroll to it on the page' : `No element is marked ${attribute} yet`,
        'data-testid': 'locate',
        onclick: locate,
      }, 'Locate')),
    h(doc, 'dl', {},
      h(doc, 'dt', {}, 'Drawn'),
      h(doc, 'dd', { 'data-testid': `${kind}-usage` }, drawn, ' ',
        copyButton(doc, 'link', () => attribute, { idle: 'Copy handle', done: 'Copied' }, { title: attribute })),
      query && [
        h(doc, 'dt', {}, 'Rows'),
        h(doc, 'dd', {}, `${query.rowCount}${query.selectedRowCount ? ` (${query.selectedRowCount} selected)` : ''}`),
        debug && h(doc, 'dt', {}, 'Source'),
        debug && h(doc, 'dd', {}, `${debug.fromCache ? 'cache' : 'fresh run'}${debug.executedAt ? ` at ${formatTime(debug.executedAt)}` : ''}`),
      ],
      control && [
        h(doc, 'dt', {}, 'Set to'), h(doc, 'dd', { 'data-testid': 'control-condition' }, formatCondition(control.condition)),
        h(doc, 'dt', {}, 'Applied'), h(doc, 'dd', { 'data-testid': 'control-applied' }, formatCondition(control.appliedCondition)),
        control.options && h(doc, 'dt', {}, 'Options'), control.options && h(doc, 'dd', {}, control.options.join(', ')),
      ]),
    query && [
      h(doc, 'details', { class: 'signature' }, h(doc, 'summary', {}, 'Signature'), h(doc, 'code', { class: 'wrap' }, query.signature)),
      query.error && h(doc, 'p', { class: 'error-text', role: 'alert' }, `${query.error.name}: ${query.error.message}`),
      (['executedAql', 'executedSql'] as const).map((field) => debug?.[field] && h(doc, 'div', { class: 'code' },
        h(doc, 'div', { class: 'code-head' },
          h(doc, 'span', {}, field === 'executedAql' ? 'Executed AQL' : 'Executed SQL'),
          copyButton(doc, 'link', () => debug[field]!, { idle: 'Copy', done: 'Copied' }, { 'data-testid': `copy-${field}` })),
        h(doc, 'pre', {}, debug[field]!))),
    ]);
}

const renderable = (node: StructureNode) => (node.kind === 'container' || node.kind === 'block') && !!node.id && !node.problems.length;

/** The Structure tab. */
export function structureTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const canRender = context.plugins.some((plugin) => plugin.name === 'render');
  let status: string | undefined;

  async function render (node: StructureNode, format: RenderFormat): Promise<void> {
    if (!node.id) return;
    const request: RenderRequest = node.kind === 'block' ? { block: node.id, format } : { container: node.id, format };
    try {
      const { omitted } = await context.call<Rendering>('render.draw', request);
      status = omitted.length ? `Saved, with ${omitted.join(', ')} left blank: ${omitted.length === 1 ? 'it sends' : 'they send'} no CORS headers.` : undefined;
    } catch (err) {
      status = `Couldn't render: ${(err as Error).message}`;
    }
    redrawStatus();
  }
  let redrawStatus = () => {};

  const app = (node: StructureNode) => panel.snapshot?.apps[node.app ?? 0];
  const canView = (node: StructureNode) => {
    if ((node.kind !== 'query' && node.kind !== 'control') || node.problems.length) return false;
    const owner = app(node);
    const name = node.name ?? '';
    return !!owner && (node.kind === 'query' ? name in owner.queries : name in owner.controls);
  };

  function rows (nodes: StructureNode[], depth: number): HTMLElement {
    return h(doc, 'ul', { class: 'tree', role: depth === 0 ? 'tree' : 'group' }, nodes.map((node) => h(doc, 'li', { role: 'treeitem', 'aria-selected': String(node.key === panel.selection) },
      h(doc, 'div', {
        class: `node${node.key === panel.selection ? ' selected' : ''}${node.problems.length ? ' problem' : ''}`,
        style: `padding-left:${8 + depth * 14}px`,
        title: node.problems.length ? node.problems.join('\n') : undefined,
        'data-testid': `node-${node.key}`,
        onmouseenter: () => panel.highlight(node.key),
        onclick: () => {
          panel.select(node.key);
          panel.locate({ node: node.key });
        },
      },
      h(doc, 'span', { class: `kind ${node.kind}` }, node.kind),
      h(doc, 'span', { class: 'name' }, (node.kind === 'container' || node.kind === 'block' ? node.id : node.name) ?? ''),
      node.app ? h(doc, 'span', { class: 'app' }, `app ${node.app}`) : null,
      node.label && h(doc, 'span', { class: 'label' }, node.label),
      node.problems.length > 0 && h(doc, 'span', { class: 'pill error', 'aria-label': 'Problems' }, '!'),
      h(doc, 'span', { class: 'actions' },
        canView(node) && h(doc, 'button', {
          type: 'button',
          class: 'row-action',
          title: 'View data',
          'aria-label': 'View data',
          'data-testid': 'view-data',
          onclick: (event: Event) => {
            event.stopPropagation();
            panel.show(DATA_TAB, node.key);
          },
        }, icon(doc, 'table')),
        canRender && renderable(node) && h(doc, 'button', {
          type: 'button',
          class: 'row-action',
          title: 'Download PNG',
          'aria-label': 'Download PNG',
          'data-testid': 'download-node',
          onclick: (event: Event) => {
            event.stopPropagation();
            void render(node, 'png');
          },
        }, icon(doc, 'download')),
        copyButton(doc, 'row-action', () => handleFor(node), { idle: icon(doc, 'copy'), done: icon(doc, 'check') }, {
          title: 'Copy handle', 'aria-label': 'Copy handle', 'data-testid': 'copy-handle',
        }))),
      node.children.length > 0 && rows(node.children, depth + 1))));
  }

  function detail (node: StructureNode): HTMLElement {
    const boxed = node.kind === 'container' || node.kind === 'block';
    const owner = app(node);
    const name = node.name ?? '';
    const entity = !boxed && !node.problems.length && owner
      ? (node.kind === 'query' ? owner.queries[name] && { query: owner.queries[name] } : owner.controls[name] && { control: owner.controls[name] })
      : undefined;
    const statusLine = h(doc, 'p', { class: 'status', role: 'status' }, status ?? '');
    redrawStatus = () => { statusLine.textContent = status ?? ''; };
    return h(doc, 'section', { class: 'detail', 'data-testid': 'node-detail' },
      h(doc, 'h5', {}, node.kind),
      h(doc, 'dl', {},
        h(doc, 'dt', {}, boxed ? 'Id' : 'Name'), h(doc, 'dd', {}, (boxed ? node.id : node.name) ?? ''),
        node.label && [h(doc, 'dt', {}, 'Label'), h(doc, 'dd', {}, node.label)],
        h(doc, 'dt', {}, 'Handle'), h(doc, 'dd', {}, h(doc, 'code', { class: 'wrap' }, handleFor(node))),
        node.children.length > 0 && [h(doc, 'dt', {}, 'Holds'), h(doc, 'dd', {}, `${node.children.length} node${node.children.length === 1 ? '' : 's'}`)]),
      node.problems.map((problem) => h(doc, 'p', { class: 'error-text', role: 'alert' }, problem)),
      canRender && renderable(node) && h(doc, 'div', { class: 'detail-actions' },
        (['png', 'pdf'] as const).map((format) => h(doc, 'button', {
          type: 'button', class: 'tool', 'data-testid': `download-${format}`, onclick: () => { void render(node, format); },
        }, icon(doc, 'download'), format.toUpperCase()))),
      statusLine,
      entity && entityCard(context, node.app ?? 0, name, entity));
  }

  return drawTab(root, context, () => {
    const structure = panel.snapshot?.structure;
    const selected = structure && panel.selection !== null ? findNode(structure.nodes, panel.selection) : undefined;
    const problems = structure ? countProblems(structure.nodes) : 0;
    return h(doc, 'div', { class: 'pane' },
      h(doc, 'div', { class: 'toolbar' },
        h(doc, 'button', {
          type: 'button',
          class: 'tool',
          'aria-pressed': String(panel.picking),
          title: 'Click a part of the page to select it here. Esc stops.',
          'data-testid': 'pick',
          onclick: () => panel.pick(!panel.picking),
        }, icon(doc, 'pick'), 'Pick'),
        problems > 0 && h(doc, 'span', { class: 'pill error', title: 'Problems in the markup' }, `${problems} problem${problems === 1 ? '' : 's'}`)),
      h(doc, 'div', { class: 'body', onmouseleave: () => panel.highlight(null) },
        !structure ? h(doc, 'p', { class: 'note', 'data-testid': 'inspect-waiting' }, 'Waiting for the Data App…')
          : structure.nodes.length === 0 ? h(doc, 'p', { class: 'note', 'data-testid': 'structure-none' },
            'Nothing is marked yet. Add ', h(doc, 'code', {}, 'data-anfra-container'), ', ', h(doc, 'code', {}, 'data-anfra-block'), ', ',
            h(doc, 'code', {}, 'data-anfra-query'), ' and ', h(doc, 'code', {}, 'data-anfra-control'),
            " to the page's elements and they show up here.")
            : rows(structure.nodes, 0)),
      selected && detail(selected));
  });
}

function selectionCard (doc: Document, label: string, selection: InspectedSelection): HTMLElement {
  return h(doc, 'article', { class: 'card' },
    h(doc, 'header', {}, h(doc, 'strong', {}, label), selection.lossy && h(doc, 'span', { class: 'pill warn' }, 'lossy')),
    h(doc, 'dl', {},
      h(doc, 'dt', {}, 'From'), h(doc, 'dd', {}, selection.source),
      h(doc, 'dt', {}, 'Rows'), h(doc, 'dd', {}, String(selection.rowCount)),
      selection.expression && [h(doc, 'dt', {}, 'Expression'), h(doc, 'dd', {}, h(doc, 'code', { class: 'wrap' }, selection.expression))],
      selection.conditions.map((condition) => [h(doc, 'dt', {}, condition.field), h(doc, 'dd', {}, formatCondition(condition))])));
}

function appSection (context: TabContext, app: InspectedApp, index: number, count: number): HTMLElement {
  const doc = context.document;
  const queries = Object.entries(app.queries);
  const controls = Object.entries(app.controls);
  return h(doc, 'section', { 'data-testid': 'inspect-app' },
    count > 1 && h(doc, 'h4', {}, app.title || `App ${index + 1}`),
    h(doc, 'h5', {}, 'App'),
    h(doc, 'dl', {},
      h(doc, 'dt', {}, 'Title'), h(doc, 'dd', {}, app.title || '—'),
      h(doc, 'dt', {}, 'Timezone'), h(doc, 'dd', {}, app.timezone || 'tenant default'),
      h(doc, 'dt', {}, 'Unapplied changes'), h(doc, 'dd', {}, h(doc, 'span', { class: `pill${app.hasChanges ? ' warn' : ''}` }, app.hasChanges ? 'yes' : 'no'))),
    h(doc, 'h5', {}, 'Queries'),
    queries.length ? queries.map(([name, query]) => entityCard(context, index, name, { query })) : h(doc, 'p', { class: 'note' }, 'None.'),
    h(doc, 'h5', {}, 'Controls'),
    controls.length ? controls.map(([name, control]) => entityCard(context, index, name, { control })) : h(doc, 'p', { class: 'note' }, 'None.'),
    (app.selection || app.appliedSelection) && [
      h(doc, 'h5', {}, 'Selection'),
      app.selection && selectionCard(doc, 'Current', app.selection),
      app.appliedSelection && selectionCard(doc, 'Applied', app.appliedSelection),
    ]);
}

/** The Data tab. Shown with a query or control node selected, it brings that one's card into view. */
export function dataTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  let shown: string | null = null;
  const unsubscribe = drawTab(root, context, () => {
    const apps = panel.snapshot?.apps;
    return h(doc, 'div', { class: 'pane' },
      h(doc, 'div', { class: 'body' },
        !apps ? h(doc, 'p', { class: 'note', 'data-testid': 'inspect-waiting' }, 'Waiting for the Data App…')
          : apps.length === 0 ? h(doc, 'p', { class: 'note', 'data-testid': 'inspect-none' }, "This Data App hasn't created an SDK app.")
            : apps.map((app, index) => appSection(context, app, index, apps.length))));
  });
  const reveal = () => {
    const structure = panel.snapshot?.structure;
    const node = structure && panel.selection !== null ? findNode(structure.nodes, panel.selection) : undefined;
    if (!node || (node.kind !== 'query' && node.kind !== 'control') || panel.selection === shown) return;
    const entity = `${node.kind}:${node.app ?? 0}/${node.name ?? ''}`;
    const card = [...root.querySelectorAll<HTMLElement>('[data-entity]')].find((element) => element.dataset.entity === entity);
    if (!card) return;
    shown = panel.selection;
    card.scrollIntoView?.({ block: 'start', behavior: 'smooth' });
    card.classList.add('flash');
  };
  reveal();
  const unreveal = panel.subscribe(reveal);
  return () => {
    unsubscribe();
    unreveal();
  };
}
