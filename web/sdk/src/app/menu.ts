/**
 * Content UI, the kernel's side: the menu on a marked block (a right-click) or on a chart's point
 * (`query.menu`), filled with plugins' frame items and the host's own; and the modal and popover a
 * plugin's frame half opens. What draws them is the kernel's UI body, sent on first use, so a frame
 * carries only the listener and the item list until someone opens one. One is open at a time.
 */
import type { MenuEntry, MenuTarget, MenuTargetKind, NodeTarget, RowsTarget } from '../common/menu';
import type { StructureNode } from '../common/types';
import type { FrameKernel } from './kernel';
import { layerRoot } from './layer';
import { ATTR } from './structure';
import type { Rect, UiBody } from './ui/body';

/** An entry a plugin's frame half adds to the menu, for one kind of target. */
export interface FrameMenuItem {
  /** Under the plugin's name once added (`underlying` is `explore.underlying`). */
  id: string;
  label: string;
  target: MenuTargetKind;
  /**
   * Whether it applies to this target; it shows when this is left out. Presentation only: frame code
   * shares a realm with the definition and can enforce nothing, so a rule that must hold belongs on
   * the host, in the calls the item makes.
   */
  when?: (target: MenuTarget) => boolean;
  run: (target: MenuTarget) => unknown;
}

export interface ModalInput {
  title: string;
  size?: 'sm' | 'md' | 'lg' | 'full';
  /** Draw the content into its own shadow root, on the host's `--anfra-*` tokens; what it returns runs on close. */
  mount: (root: ShadowRoot) => void | (() => void);
  onClose?: () => void;
}

export interface PopoverInput {
  /** A node (a target's, kept in view as the page scrolls), or a point in the viewport. */
  anchor: NodeTarget | { x: number, y: number };
  mount: (root: ShadowRoot) => void | (() => void);
  /** `outside`: a click anywhere else closes it. Unless asked, only Escape and its own close do. */
  dismiss?: 'outside';
  onClose?: () => void;
}

/** An open modal or popover. */
export interface Opened {
  close: () => void;
}

export interface ContentUi {
  addItem: (plugin: string, item: FrameMenuItem) => void;
  modal: (input: ModalInput) => Promise<Opened>;
  popover: (input: PopoverInput) => Promise<Opened>;
}

let rowsOpener: ((event: Event, target: RowsTarget) => boolean) | undefined;

/** `query.menu`'s way in: open the menu on a query's rows, for the event; whether one opens. */
export function openRowsMenu (event: Event, target: RowsTarget): boolean {
  return rowsOpener ? rowsOpener(event, target) : false;
}

const BLOCK = `[${ATTR.block}]`;
const OPTED_OUT = '[data-anfra-menu="off"]';

const bare = (node: StructureNode): StructureNode => ({ ...node, children: node.children.map((child) => ({ ...child, children: [] })) });

/** The node with this key, and the nodes it sits in, outermost first. */
function pathTo (nodes: StructureNode[], key: string, above: StructureNode[] = []): StructureNode[] | undefined {
  for (const node of nodes) {
    if (node.key === key) return [...above, node];
    const inner = pathTo(node.children, key, [...above, node]);
    if (inner) return inner;
  }
  return undefined;
}

const isElement = (value: unknown): value is Element => typeof (value as Element | null)?.closest === 'function';

/** Start content UI in a frame; `kernel` is the kernel's own, named `anfra`. */
export function installContentUi (kernel: FrameKernel): ContentUi {
  const win = kernel.window;
  const doc = win.document;
  const items: (FrameMenuItem & { full: string })[] = [];
  let hostKinds = new Set<MenuTargetKind>();
  let listening = false;
  let current: (() => void) | undefined;

  const ui = () => kernel.body<UiBody>();
  const root = () => layerRoot(doc, 'anfra-ui');
  const offered = (kind: MenuTargetKind) => hostKinds.has(kind) || items.some((item) => item.target === kind);
  const closeCurrent = () => {
    const close = current;
    current = undefined;
    close?.();
  };

  /** The marked block an event happened in, unless it or a block around it opted out. */
  function blockOf (target: EventTarget | null): Element | undefined {
    const element = isElement(target) ? target.closest(BLOCK) : null;
    return element && !element.closest(OPTED_OUT) ? element : undefined;
  }

  function nodeTarget (element: Element): NodeTarget | undefined {
    const structure = kernel.structure();
    const key = structure.keysOf.get(element)?.[0];
    const path = key !== undefined ? pathTo(structure.nodes, key) : undefined;
    if (!path) return undefined;
    const { x, y, width, height } = element.getBoundingClientRect();
    return {
      kind: 'node', node: bare(path[path.length - 1]), ancestors: path.slice(0, -1).map((node) => ({ ...node, children: [] })), rect: { x, y, width, height },
    };
  }

  /** The entries for one target: the frame's items, then the host's. */
  async function entriesFor (target: MenuTarget): Promise<{ entries: MenuEntry[], run: Map<string, () => unknown> }> {
    const run = new Map<string, () => unknown>();
    const entries: MenuEntry[] = [];
    for (const item of items) {
      if (item.target !== target.kind) continue;
      let applies = true;
      try { applies = item.when ? item.when(target) !== false : true; } catch { applies = false; }
      if (!applies) continue;
      const key = `frame:${item.full}:${target.kind}`;
      entries.push({ id: key, label: item.label });
      run.set(key, () => item.run(target));
    }
    if (hostKinds.has(target.kind)) {
      try {
        const answered = await kernel.call<MenuEntry[]>('anfra.menu', target);
        for (const entry of Array.isArray(answered) ? answered : []) {
          if (typeof entry?.id !== 'string' || typeof entry.label !== 'string') continue;
          const key = `host:${entry.id}:${target.kind}`;
          entries.push({ id: key, label: entry.label, ...(typeof entry.disabled === 'string' ? { disabled: entry.disabled } : {}) });
          run.set(key, () => kernel.call('anfra.run', { id: entry.id, target }));
        }
      } catch { /* the host's entries are left out; the frame's still show */ }
    }
    return { entries, run };
  }

  /** Open the menu on these targets (the rows first, then their node), for the event. */
  function open (event: Event, targets: MenuTarget[]): boolean {
    const relevant = targets.filter((target) => offered(target.kind));
    if (!relevant.length) return false;
    event.preventDefault();
    const point = event as Partial<MouseEvent>;
    const x = typeof point.clientX === 'number' ? point.clientX : 0;
    const y = typeof point.clientY === 'number' ? point.clientY : 0;
    void (async () => {
      const [body, ...found] = await Promise.all([ui(), ...relevant.map(entriesFor)]);
      const groups = found.map((group) => group.entries).filter((group) => group.length);
      if (!groups.length) return;
      closeCurrent();
      const close = body.menu(root(), {
        x,
        y,
        groups,
        onPick: (key) => {
          const action = found.map((group) => group.run.get(key)).find(Boolean);
          Promise.resolve().then(action).catch((err: unknown) => (win as Window & typeof globalThis).console?.error('anfra: a menu item failed:', err));
        },
        onClose: () => { if (current === close) current = undefined; },
      });
      current = close;
    })().catch((err: unknown) => (win as Window & typeof globalThis).console?.error('anfra: the menu failed to open:', err));
    return true;
  }

  // The right-click on a block: only once something offers a menu, and never over the page's own
  // handler (it prevented the default) or Shift, which keeps the browser's menu.
  function listen (): void {
    if (listening || !(offered('node') || offered('rows'))) return;
    listening = true;
    win.addEventListener('contextmenu', (event: Event) => {
      if (event.defaultPrevented || (event as MouseEvent).shiftKey) return;
      const element = blockOf(event.target);
      const target = element && nodeTarget(element);
      if (target) open(event, [target]);
    });
  }

  rowsOpener = (event, target) => {
    if (event.defaultPrevented || (event as MouseEvent).shiftKey) return false;
    const element = blockOf(event.target);
    const node = element ? nodeTarget(element) : undefined;
    const rows: RowsTarget = node ? { ...target, node } : target;
    return open(event, node ? [rows, node] : [rows]);
  };

  kernel.on('menu', (payload: unknown) => {
    const kinds = (payload as { kinds?: unknown } | null)?.kinds;
    hostKinds = new Set((Array.isArray(kinds) ? kinds : []).filter((kind): kind is MenuTargetKind => kind === 'node' || kind === 'rows'));
    listen();
  });

  async function show<T extends { onClose?: () => void }> (input: T, draw: (body: UiBody, onClose: () => void) => () => void): Promise<Opened> {
    const body = await ui();
    closeCurrent();
    let close: () => void = () => {};
    const onClose = () => {
      if (current === close) current = undefined;
      try { input.onClose?.(); } catch { /* the plugin's to see */ }
    };
    close = draw(body, onClose);
    current = close;
    return { close };
  }

  return {
    addItem (plugin, item) {
      items.push({ ...item, full: `${plugin}.${item.id}` });
      listen();
    },
    modal: (input) => show(input, (body, onClose) => body.modal(root(), {
      title: input.title, size: input.size, mount: input.mount, onClose,
    })),
    popover: (input) => show(input, (body, onClose) => {
      const { anchor } = input;
      let rect: () => Rect | undefined;
      if ('kind' in anchor) {
        const element = kernel.structure().elements.get(anchor.node.key);
        rect = () => {
          if (!element?.isConnected) return undefined;
          const { x, y, width, height } = element.getBoundingClientRect();
          return { x, y, width, height };
        };
      } else {
        rect = () => ({ x: anchor.x, y: anchor.y, width: 0, height: 0 });
      }
      return body.popover(root(), {
        anchor: rect, mount: input.mount, dismiss: input.dismiss, onClose,
      });
    }),
  };
}
