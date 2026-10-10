/**
 * The frame bootstrap: runs inside a Data App's sandboxed frame, before any of the Data App
 * definition's own scripts. It reads what its host provisioned, gives the SDK a Backend that
 * forwards every call to the host over the bridge, and installs the SDK as the `Anfra` global.
 * The only code in a frame that knows about postMessage.
 */
import { PermissionError, QueryError, TransportError } from '../common/errors';
import { PROVISION_ELEMENT_ID } from '../common/bridge';
import type {
  BridgeError, BridgeMethod, FrameMessage, HostMessage, Provision,
} from '../common/bridge';
import type { Backend, BackendQueryResult } from '../common/types';
import { createSdk } from './sdk';
import { installSandbox } from './sandbox';
import { buildStructure, describeNode, type BuiltStructure } from './structure';
import { overlayFor, scrollTo } from './overlay';
import { startPick } from './pick';
import type { LocateTarget, StructureNode } from '../common/types';

function abortError (): Error {
  const err = new Error('Aborted');
  err.name = 'AbortError';
  return err;
}

// Errors cross the bridge as { name, message }; rebuild the SDK's class so an author can tell a
// permission failure from a broken query.
function toError (error: BridgeError | undefined): Error {
  switch (error?.name) {
    case 'PermissionError': return new PermissionError(error.message);
    case 'TransportError': return new TransportError(error.message);
    default: return new QueryError(error?.message || 'The query failed.');
  }
}

/** Provision the SDK in `win` from its host, and install it. Returns once `Anfra` exists. */
export function bootstrap (win: Window): void {
  const element = win.document.getElementById(PROVISION_ELEMENT_ID);
  if (!element?.textContent) throw new Error('anfra: this frame was not provisioned by a host.');
  const provision = JSON.parse(element.textContent) as Provision;
  const post = (message: FrameMessage) => win.parent.postMessage(message, provision.hostOrigin);

  const pending = new Map<number, { resolve: (v: unknown) => void, reject: (e: Error) => void }>();
  let nextId = 1;

  let inspecting = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  /* eslint-disable @typescript-eslint/no-use-before-define -- the instance and the inspection
     helpers exist by the time a message arrives */
  win.addEventListener('message', (event: MessageEvent) => {
    if (event.source !== win.parent) return;
    const message = event.data as HostMessage | undefined;
    switch (message?.type) {
      case 'anfra:response': {
        const call = pending.get(message.id);
        if (!call) return;
        pending.delete(message.id);
        if (message.ok) call.resolve(message.result);
        else call.reject(toError(message.error));
        return;
      }
      case 'anfra:inspect-watch':
        setInspecting(message.open);
        return;
      case 'anfra:highlight':
        highlight(message.node);
        return;
      case 'anfra:locate':
        locate(message.target);
        return;
      case 'anfra:pick-watch':
        setPicking(message.on);
        return;
      default:
    }
  });
  /* eslint-enable @typescript-eslint/no-use-before-define */

  function call<T> (method: BridgeMethod, request: unknown, signal: AbortSignal): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      if (signal.aborted) {
        reject(abortError());
        return;
      }
      const id = nextId++;
      pending.set(id, { resolve: resolve as (v: unknown) => void, reject });
      signal.addEventListener('abort', () => {
        if (!pending.delete(id)) return;
        post({ type: 'anfra:cancel', id });
        reject(abortError());
      }, { once: true });
      post({ type: 'anfra:request', id, method, request } as FrameMessage);
    });
  }

  const backend: Backend = {
    submitQuery: (request, signal) => call<BackendQueryResult>('submitQuery', request, signal).then((result) => {
      // Dates don't survive JSON; provenance's `executedAt` is one.
      if (result.debug?.executedAt) result.debug.executedAt = new Date(result.debug.executedAt);
      return result;
    }),
    fieldSuggestions: (request, signal) => call('fieldSuggestions', request, signal),
  };

  const instance = createSdk({ datasets: provision.datasets, user: provision.user, backend });

  // Inspection: while the host's inspect panel is open, post a snapshot of every app, and of the
  // page's structure, on each change. The instance is the only way to reach an app the author
  // built, so wrap createApp to subscribe to each one as it appears. Nothing here runs for a frame
  // nobody inspects: no observer, no structure, no overlay.
  const doc = win.document;
  let structure: BuiltStructure | undefined;
  let lastStructureJson = '';
  let observer: MutationObserver | undefined;
  let stopPick: (() => void) | undefined;

  function postSnapshot (): void {
    timer = undefined;
    if (!inspecting) return;
    try {
      structure = buildStructure(doc, instance.apps);
      lastStructureJson = JSON.stringify(structure.nodes);
      post({
        type: 'anfra:inspect',
        apps: instance.apps.map((app) => app.toInspectJSON()),
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
        if (JSON.stringify(buildStructure(doc, instance.apps).nodes) !== lastStructureJson) scheduleSnapshot();
      } catch { /* a broken document is the author's to see, not ours to throw over */ }
    });
    observer = watcher;
    watcher.observe(doc.documentElement, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['data-anfra-container', 'data-anfra-block', 'data-anfra-query', 'data-anfra-control', 'data-anfra-label'],
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
      overlayFor(doc).hide();
      structure = undefined;
      lastStructureJson = '';
    }
  }

  function findNode (nodes: StructureNode[], key: string): StructureNode | undefined {
    for (const node of nodes) {
      if (node.key === key) return node;
      const inner = findNode(node.children, key);
      if (inner) return inner;
    }
    return undefined;
  }
  function highlight (key: string | null): void {
    const overlay = overlayFor(doc);
    const node = key !== null && structure ? findNode(structure.nodes, key) : undefined;
    const element = key !== null ? structure?.elements.get(key) : undefined;
    if (!node || !element || !structure) { overlay.hide(); return; }
    overlay.show([element], describeNode(node, element, structure));
  }
  function locate (target: LocateTarget): void {
    if ('node' in target) {
      const element = structure?.elements.get(target.node);
      if (element) scrollTo(element);
      highlight(target.node);
    } else {
      instance.apps[target.app]?.locate(target.kind, target.name);
    }
    // The page may have changed since the last snapshot without the app noticing.
    scheduleSnapshot();
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
        if (key !== undefined) post({ type: 'anfra:picked', node: key });
      },
      onCancel: () => {
        setPicking(false);
        overlayFor(doc).hide();
        post({ type: 'anfra:picked', node: null });
      },
    });
  }

  const createApp = instance.createApp.bind(instance);
  instance.createApp = (declaration) => {
    const app = createApp(declaration);
    app.subscribe(scheduleSnapshot);
    scheduleSnapshot();
    return app;
  };

  installSandbox(instance, win as unknown as Record<string, unknown>);
  post({ type: 'anfra:ready' });
}
