/**
 * Structure: the page's semantic parts as a tree. Hover a node to highlight it on the page, click
 * it to select it and scroll it into view, collapse what is in the way, and copy a node's handle to
 * point an agent at it. The selected node's detail sits under the tree, with links to its query or
 * control in Queries and its table in Data, and pictures of a block when `render` is installed.
 */
import type { Rendering, RenderFormat, RenderRequest, StructureNode } from '../../../common/types';
import type { TabContext } from '../../../host/contributions';
import {
  attributeFor, countProblems, findNode, handleFor, plural,
} from '../model/format';
import { stateOf } from '../state';
import {
  copyIcon, drawTab, fields, h, icon, link, note, replace,
} from '../view';

const boxed = (node: StructureNode) => node.kind === 'container' || node.kind === 'block';
const renderable = (node: StructureNode) => boxed(node) && !!node.id && !node.problems.length;

/** What the markup names a node by: its id (a container or block), or its name (a query or control). */
const idOf = (node: StructureNode) => (boxed(node) ? node.id ?? '' : node.app ? `${node.app}/${node.name ?? ''}` : node.name ?? '');

/** The keys of the nodes on the way to `key`, outermost first; undefined when it isn't there. */
function ancestorsOf (nodes: StructureNode[], key: string, trail: string[] = []): string[] | undefined {
  for (const node of nodes) {
    if (node.key === key) return trail;
    const found = ancestorsOf(node.children, key, [...trail, node.key]);
    if (found) return found;
  }
  return undefined;
}

export default function structureTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const state = stateOf(context.mount);
  const canRender = context.plugins.some((plugin) => plugin.name === 'render');
  // Collapsed nodes stay collapsed across snapshots, until the tab is taken down.
  const collapsed = new Set<string>();
  let status: string | undefined;
  let revealed: string | null = null;

  async function render (node: StructureNode, format: RenderFormat): Promise<void> {
    if (!node.id) return;
    const request: RenderRequest = node.kind === 'block' ? { block: node.id, format } : { container: node.id, format };
    try {
      const { omitted } = await context.call<Rendering>('render.draw', request);
      status = omitted.length ? `Saved, with ${omitted.join(', ')} left blank: ${omitted.length === 1 ? 'it sends' : 'they send'} no CORS headers.` : undefined;
    } catch (err) {
      status = `Couldn't render: ${(err as Error).message}`;
    }
    tab.redraw();
  }

  const appOf = (node: StructureNode) => panel.snapshot?.apps[node.app ?? 0];
  /** Whether the node is a query or control the app has, which Queries has a row for. */
  const declared = (node: StructureNode) => {
    if ((node.kind !== 'query' && node.kind !== 'control') || node.problems.length) return false;
    const app = appOf(node);
    const name = node.name ?? '';
    return !!app && (node.kind === 'query' ? name in app.queries : name in app.controls);
  };
  const showInQueries = (node: StructureNode) => state.reveal(panel, { entity: node.kind as 'query' | 'control', app: node.app ?? 0, name: node.name ?? '' });
  const showData = (node: StructureNode) => state.reveal(panel, { table: node.name ?? '', app: node.app ?? 0 });
  const select = (node: StructureNode) => {
    panel.select(node.key);
    panel.locate({ node: node.key });
  };

  function rows (nodes: StructureNode[], depth: number): HTMLElement {
    return h(doc, 'ul', { class: 'tree', role: depth === 0 ? 'tree' : 'group' }, nodes.map((node) => {
      const open = !collapsed.has(node.key);
      const selected = node.key === panel.selection;
      const id = idOf(node);
      return h(doc, 'li', { role: 'none' },
        h(doc, 'div', {
          class: `node${selected ? ' selected' : ''}${node.problems.length ? ' problem' : ''}`,
          role: 'treeitem',
          tabindex: '0',
          'aria-selected': String(selected),
          'aria-expanded': node.children.length ? String(open) : undefined,
          style: `padding-left:${4 + depth * 14}px`,
          title: node.problems.length ? node.problems.join('\n') : undefined,
          'data-key': `node:${node.key}`,
          'data-testid': `node-${node.key}`,
          onmouseenter: () => panel.highlight(node.key),
          onclick: () => select(node),
          onkeydown: (event: Event) => {
            const key = (event as KeyboardEvent).key;
            if (key === 'Enter') select(node);
            if (!node.children.length || (key === 'ArrowRight') === open || (key !== 'ArrowRight' && key !== 'ArrowLeft')) return;
            if (open) collapsed.add(node.key);
            else collapsed.delete(node.key);
            tab.redraw();
          },
        },
        node.children.length
          ? h(doc, 'button', {
            type: 'button',
            class: `row-action twist${open ? ' open' : ''}`,
            'aria-label': open ? 'Collapse' : 'Expand',
            tabindex: '-1',
            onclick: (event: Event) => {
              event.stopPropagation();
              if (open) collapsed.add(node.key);
              else collapsed.delete(node.key);
              tab.redraw();
            },
          }, icon(doc, 'chevron'))
          : h(doc, 'span', { class: 'twist-space' }),
        h(doc, 'span', { class: `kind ${node.kind}` }, node.kind),
        id && h(doc, 'span', { class: 'name' }, id),
        node.label && node.label !== id && h(doc, 'span', { class: 'label' }, node.label),
        node.problems.length > 0 && h(doc, 'span', { class: 'pill error', 'aria-label': 'Problems' }, '!'),
        h(doc, 'span', { class: 'actions' },
          declared(node) && h(doc, 'button', {
            type: 'button',
            class: 'row-action',
            title: 'Show in Queries',
            'aria-label': 'Show in Queries',
            'data-testid': 'show-in-queries',
            onclick: (event: Event) => {
              event.stopPropagation();
              showInQueries(node);
            },
          }, icon(doc, 'query')),
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
          copyIcon(doc, 'Copy handle', () => handleFor(node), { 'data-testid': 'copy-handle' }))),
        node.children.length > 0 && open && rows(node.children, depth + 1));
    }));
  }

  function detail (node: StructureNode): HTMLElement {
    const query = node.kind === 'query' && declared(node);
    return h(doc, 'section', { class: 'detail', 'data-testid': 'node-detail' },
      h(doc, 'h5', {}, node.kind),
      fields(doc, [
        [boxed(node) ? 'Id' : 'Name', (boxed(node) ? node.id : node.name) ?? ''],
        !!node.label && ['Label', node.label],
        !!node.app && ['App', `App ${node.app}`],
        ['Handle', h(doc, 'code', { class: 'wrap' }, handleFor(node))],
        node.children.length > 0 && ['Holds', plural(node.children.length, 'node')],
      ]),
      node.problems.map((problem) => h(doc, 'p', { class: 'error-text', role: 'alert' }, problem)),
      h(doc, 'div', { class: 'detail-actions' },
        declared(node) && link(doc, [icon(doc, 'query'), 'Show in Queries'], () => showInQueries(node), { 'data-testid': 'detail-show-in-queries' }),
        query && link(doc, [icon(doc, 'table'), 'Show data'], () => showData(node), { 'data-testid': 'view-data' }),
        copyIcon(doc, 'Copy the attribute', () => attributeFor(node)),
        canRender && renderable(node) && (['png', 'pdf'] as const).map((format) => h(doc, 'button', {
          type: 'button', class: 'tool', 'data-testid': `download-${format}`, onclick: () => { void render(node, format); },
        }, icon(doc, 'download'), format.toUpperCase()))),
      status && h(doc, 'p', { class: 'status', role: 'status' }, status));
  }

  const head = h(doc, 'div', { class: 'toolbar' });
  const tab = drawTab(root, context, () => {
    const structure = panel.snapshot?.structure;
    const selected = structure && panel.selection !== null ? findNode(structure.nodes, panel.selection) : undefined;
    const problems = structure ? countProblems(structure.nodes) : 0;
    replace(doc, head,
      h(doc, 'button', {
        type: 'button',
        class: 'tool',
        'aria-pressed': String(panel.picking),
        title: 'Click a part of the page to select it here. Esc stops.',
        'data-testid': 'pick',
        onclick: () => panel.pick(!panel.picking),
      }, icon(doc, 'pick'), 'Pick'),
      problems > 0 && h(doc, 'span', { class: 'pill error', title: 'Problems in the markup' }, plural(problems, 'problem')));
    // A node selected elsewhere (a pick, a link): its ancestors open, so it can be seen.
    if (structure && panel.selection !== null && panel.selection !== revealed) {
      ancestorsOf(structure.nodes, panel.selection)?.forEach((key) => collapsed.delete(key));
    }
    return [
      h(doc, 'div', { class: 'scroll', onmouseleave: () => panel.highlight(null) },
        !structure ? note(doc, 'Waiting for the Data App…', { 'data-testid': 'inspect-waiting' })
          : structure.nodes.length === 0 ? note(doc, [
            'Nothing is marked yet. Add ', h(doc, 'code', {}, 'data-anfra-container'), ', ', h(doc, 'code', {}, 'data-anfra-block'), ', ',
            h(doc, 'code', {}, 'data-anfra-query'), ' and ', h(doc, 'code', {}, 'data-anfra-control'),
            " to the page's elements and they show up here.",
          ], { 'data-testid': 'structure-none' })
            : rows(structure.nodes, 0)),
      selected && detail(selected),
    ];
  }, { head, split: true });

  const reveal = () => {
    if (panel.selection === null || panel.selection === revealed) return;
    const row = [...root.querySelectorAll<HTMLElement>('[data-key]')].find((element) => element.dataset.key === `node:${panel.selection}`);
    if (!row) return;
    revealed = panel.selection;
    row.scrollIntoView?.({ block: 'nearest' });
  };
  reveal();
  const stop = panel.subscribe(reveal);
  return () => {
    stop();
    tab.dispose();
    // Unmounting under the pointer, as a switch of tab does, leaves nothing highlighted.
    panel.highlight(null);
  };
}
