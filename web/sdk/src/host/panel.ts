/**
 * The panel: one per mounted Data App, the host's devtools over it. The runtime owns its state (open
 * or not, the tab it shows, the node selected, the frame's last snapshot, pick mode) and the frame
 * side it needs (`anfra.*`), so every host's panel has them whatever plugins it installs. The host
 * draws the panel's chrome itself; plugins add its tabs (docs/designs/sdk-plugins.md, The panel).
 */
import type { InspectedApp, InspectedStructure, LocateTarget } from '../common/types';
import type { MountContext } from './plugin';

/** What the frame says while the panel is open: every app's state, and the page's structure. No rows. */
export interface Snapshot {
  apps: InspectedApp[];
  structure: InspectedStructure;
}

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

/** A structure node, and every one under it, of the shape the tabs read. */
const isNodeTree = (value: unknown): boolean => isObject(value) && typeof value.key === 'string' && typeof value.kind === 'string'
  && Array.isArray(value.problems) && Array.isArray(value.children) && value.children.every(isNodeTree);

/**
 * A snapshot as the frame posts it, or undefined when it isn't one: the frame is the definition's
 * realm, so a panel gets only something of the snapshot's shape, never what a script made up.
 */
export function checkSnapshot (payload: unknown): Snapshot | undefined {
  if (!isObject(payload) || !Array.isArray(payload.apps) || !payload.apps.every(isObject)) return undefined;
  const structure = payload.structure ?? { nodes: [], usage: {} };
  if (!isObject(structure) || !Array.isArray(structure.nodes) || !structure.nodes.every(isNodeTree) || !isObject(structure.usage ?? {})) return undefined;
  return {
    apps: payload.apps as unknown as InspectedApp[],
    structure: { nodes: structure.nodes, usage: structure.usage ?? {} } as InspectedStructure,
  };
}

/** A pick as the frame posts it: a node's key, or null for none; undefined when it isn't one. */
export function checkPicked (payload: unknown): { node: string | null } | undefined {
  if (!isObject(payload) || !(typeof payload.node === 'string' || payload.node === null)) return undefined;
  return { node: payload.node };
}

/** One mounted Data App's panel. Read its state, change it, and subscribe to hear of each change. */
export interface Panel {
  /** Whether the panel is open. The frame posts snapshots only while it is. */
  readonly open: boolean;
  /** The tab it shows, by full id; undefined until one is chosen. */
  readonly tab: string | undefined;
  /** The node every tab is about, by its key in the snapshot's structure; null for none. */
  readonly selection: string | null;
  /** The frame's last snapshot while open; undefined until it reports, and once closed. */
  readonly snapshot: Snapshot | undefined;
  /** Whether pick mode is on: pointing at the page outlines marked elements, and a click selects one. */
  readonly picking: boolean;
  /** Open the panel, at `tab` if given (else the last one), with `target` selected if given. */
  show: (tab?: string, target?: string | null) => void;
  /** Close the panel: ends pick mode and clears the selection. */
  hide: () => void;
  /** Open the panel at `tab`, or close it when it is already open there (or anywhere, with no tab). */
  toggle: (tab?: string) => void;
  /** Select a node, by key, or none. */
  select: (node: string | null) => void;
  /** Show a node on the page, by key, or nothing. */
  highlight: (node: string | null) => void;
  /** Scroll a node, or an entity wherever it is drawn, into view, and hold the highlight on it. */
  locate: (target: LocateTarget) => void;
  /** Turn pick mode on or off. A pick selects the node and ends the mode; Escape in the frame ends it. */
  pick: (on: boolean) => void;
  /** Each change to any of the above. Returns the function that stops listening. */
  subscribe: (listener: () => void) => () => void;
  /**
   * Draw a tab, by full id, into `element` (in its shadow root, made if it has none), where the host
   * placed it in its chrome. Returns the function that takes the tab down again.
   */
  mountTab: (id: string, element: HTMLElement) => () => void;
}

/** The runtime's side of a panel: the panel, and what feeds it from the frame. */
export interface PanelState {
  panel: Panel;
  /** A frame's `anfra.snapshot`, unchecked. */
  snapshot: (payload: unknown) => void;
  /** A frame's `anfra.picked`, unchecked. */
  picked: (payload: unknown) => void;
  /** The frame's document (re)loaded: tell it again what it forgot. */
  ready: () => void;
  dispose: () => void;
}

export interface PanelOptions {
  /** The installed tabs' ids, in order. */
  tabs: () => readonly string[];
  /** Draw a tab into an element; the runtime's, since a tab needs its contribution and its calls. */
  mountTab: (panel: Panel, id: string, element: HTMLElement) => () => void;
}

export function createPanel (mount: MountContext, options: PanelOptions): PanelState {
  let open = false;
  let tab: string | undefined;
  let selection: string | null = null;
  let snapshot: Snapshot | undefined;
  let picking = false;
  const listeners = new Set<() => void>();
  const changed = () => listeners.forEach((listener) => {
    try { listener(); } catch { /* one listener must not stop the others */ }
  });

  const setPicking = (on: boolean) => {
    if (picking === on) return;
    picking = on;
    mount.emit('anfra.pick', on);
  };
  const setOpen = (value: boolean) => {
    if (open === value) return;
    open = value;
    mount.emit('anfra.watch', value);
    if (!value) {
      setPicking(false);
      selection = null;
      snapshot = undefined;
    }
  };
  // A tab the host knows of, or the first one, so an open panel always shows something it has.
  const known = (id: string | undefined) => {
    const tabs = options.tabs();
    return id !== undefined && tabs.includes(id) ? id : tab !== undefined && tabs.includes(tab) ? tab : tabs[0];
  };

  const panel: Panel = {
    get open () { return open; },
    get tab () { return tab; },
    get selection () { return selection; },
    get snapshot () { return snapshot; },
    get picking () { return picking; },
    show (next, target) {
      setOpen(true);
      tab = known(next);
      if (target !== undefined) {
        selection = target;
        if (target !== null) mount.emit('anfra.locate', { node: target });
      }
      changed();
    },
    hide () {
      if (!open) return;
      setOpen(false);
      mount.emit('anfra.highlight', null);
      changed();
    },
    toggle (next) {
      if (open && (next === undefined || next === tab)) panel.hide();
      else panel.show(next);
    },
    select (node) {
      selection = node;
      setPicking(false);
      changed();
    },
    highlight (node) {
      mount.emit('anfra.highlight', node);
    },
    locate (target) {
      mount.emit('anfra.locate', target);
    },
    pick (on) {
      if (on && !open) {
        setOpen(true);
        tab = known(tab);
      }
      setPicking(on === true);
      changed();
    },
    subscribe (listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    mountTab: (id, element) => options.mountTab(panel, id, element),
  };

  return {
    panel,
    snapshot (payload) {
      if (!open) return;
      const checked = checkSnapshot(payload);
      if (!checked) return;
      snapshot = checked;
      changed();
    },
    picked (payload) {
      if (!open) return;
      const checked = checkPicked(payload);
      if (!checked) return;
      // A pick selects the node and ends pick mode; Escape only ends it (the frame has left it
      // already). Either way the outline the pick left goes.
      setPicking(false);
      if (checked.node !== null) selection = checked.node;
      mount.emit('anfra.highlight', null);
      changed();
    },
    ready () {
      if (open) mount.emit('anfra.watch', true);
      if (picking) mount.emit('anfra.pick', true);
    },
    dispose () {
      listeners.clear();
    },
  };
}
