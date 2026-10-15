/**
 * `inspect`'s frame half: while the host watches, post a snapshot of every app, and of the page's
 * structure, on each change; highlight, locate and pick on the host's word. Nothing here runs for a
 * frame nobody inspects: no observer, no structure, no overlay.
 */
import type { FrameKernel } from '../../app/kernel';
import { ATTR, describeNode, type BuiltStructure } from '../../app/structure';
import { scrollTo } from '../../app/overlay';
import type { LocateTarget, StructureNode } from '../../common/types';
import { startPick } from './pick';

function findNode (nodes: StructureNode[], key: string): StructureNode | undefined {
  for (const node of nodes) {
    if (node.key === key) return node;
    const inner = findNode(node.children, key);
    if (inner) return inner;
  }
  return undefined;
}

export default function setup (kernel: FrameKernel): void {
  const win = kernel.window;
  const doc = win.document;
  let inspecting = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let structure: BuiltStructure | undefined;
  let lastStructureJson = '';
  let observer: MutationObserver | undefined;
  let stopPick: (() => void) | undefined;

  function postSnapshot (): void {
    timer = undefined;
    if (!inspecting) return;
    try {
      structure = kernel.structure();
      lastStructureJson = JSON.stringify(structure.nodes);
      kernel.emit('snapshot', {
        apps: kernel.sdk.apps.map((app) => app.toInspectJSON()),
        structure: { nodes: structure.nodes, usage: structure.usage },
      });
    } catch {
      // A snapshot that will not clone must not break the author's app.
    }
  }
  function scheduleSnapshot (): void {
    if (inspecting && timer === undefined) timer = setTimeout(postSnapshot, 50);
  }
  // The page can change without the app noticing (a table drawn on the first result, a framework
  // re-rendering). Watch only what the structure reads: elements coming and going, and the
  // structure attributes. Canvas redraws, styles and tooltips never fire it.
  function observeStructure (): void {
    const Observer = (win as Window & typeof globalThis).MutationObserver;
    if (observer || typeof Observer !== 'function') return;
    const watcher = new Observer(() => {
      if (!inspecting || timer !== undefined) return;
      try {
        if (JSON.stringify(kernel.structure().nodes) !== lastStructureJson) scheduleSnapshot();
      } catch { /* a broken document is the author's to see, not ours to throw over */ }
    });
    observer = watcher;
    watcher.observe(doc.documentElement, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: Object.values(ATTR),
    });
  }
  function highlight (key: string | null): void {
    const node = key !== null && structure ? findNode(structure.nodes, key) : undefined;
    const element = key !== null ? structure?.elements.get(key) : undefined;
    if (!node || !element || !structure) { kernel.overlay.hide(); return; }
    kernel.overlay.show([element], describeNode(node, element, structure));
  }
  function setPicking (on: boolean): void {
    if (!on) {
      stopPick?.();
      stopPick = undefined;
      return;
    }
    if (stopPick) return;
    const keyOf = (element: Element): string | undefined => structure?.keysOf.get(element)?.[0];
    stopPick = startPick(doc, {
      onHover: (element) => highlight(element ? keyOf(element) ?? null : null),
      onPick: (element) => {
        const key = keyOf(element);
        if (key !== undefined) kernel.emit('picked', { node: key });
      },
      onCancel: () => {
        setPicking(false);
        kernel.overlay.hide();
        kernel.emit('picked', { node: null });
      },
    });
  }
  function setInspecting (open: boolean): void {
    inspecting = open;
    if (open) {
      observeStructure();
      postSnapshot();
    } else {
      observer?.disconnect();
      observer = undefined;
      setPicking(false);
      kernel.overlay.hide();
      structure = undefined;
      lastStructureJson = '';
    }
  }
  function locate (target: LocateTarget): void {
    if ('node' in target) {
      const element = structure?.elements.get(target.node);
      if (element) scrollTo(element);
      highlight(target.node);
    } else {
      kernel.sdk.apps[target.app]?.locate(target.kind, target.name);
    }
    // The page may have changed since the last snapshot without the app noticing.
    scheduleSnapshot();
  }

  kernel.on('watch', (open: boolean) => setInspecting(open === true));
  kernel.on('highlight', (node: string | null) => highlight(node));
  kernel.on('locate', (target: LocateTarget) => locate(target));
  kernel.on('pick', (on: boolean) => setPicking(on === true));
  kernel.onApp((app) => {
    app.subscribe(scheduleSnapshot);
    scheduleSnapshot();
  });
}
