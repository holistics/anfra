import type {
  BridgeError, FrameMessage, HostMessage,
} from '../common/bridge';
import type {
  Backend, InspectedApp, InspectedStructure, LocateTarget,
} from '../common/types';

export interface BridgeHandle {
  /** Ask the frame to post (or stop posting) inspection snapshots; survives the frame reloading. */
  setInspecting: (open: boolean) => void;
  /** Show a node of the last snapshot's structure on the page, or nothing (null). */
  highlight: (node: string | null) => void;
  /**
   * Scroll a node, or an entity wherever it is drawn, into view and hold the highlight on it. Fire
   * and forget: the frame answers nothing, and does nothing for what is not on the page.
   */
  locate: (target: LocateTarget) => void;
  /** Start or stop pick mode in the frame: pointing outlines marked elements, a click reports one. */
  setPicking: (on: boolean) => void;
  stop: () => void;
}

export interface BridgeCallbacks {
  /** The frame's inspection snapshot: its apps and its structure. */
  onInspect?: (apps: InspectedApp[], structure: InspectedStructure) => void;
  /** In pick mode, the reader chose a node (its key), or left the mode (null). */
  onPick?: (node: string | null) => void;
}

function toBridgeError (err: unknown): BridgeError {
  const e = err as Error | undefined;
  return { name: e?.name ?? 'QueryError', message: e?.message ?? 'The query failed.' };
}

/**
 * The bridge, the host's side: answer a Data App frame's Backend calls with `backend`, abort one on
 * the frame's cancel, and relay its inspection snapshots, until `stop`. Only messages from this
 * frame are served (a sandboxed frame has an opaque origin, so its window is what identifies it),
 * and only the Backend's own methods: nothing else of the API is reachable from a frame.
 */
export function serveBridge (
  frame: HTMLIFrameElement,
  backend: Backend,
  callbacks: BridgeCallbacks = {},
): BridgeHandle {
  const inFlight = new Map<number, AbortController>();
  let inspecting = false;
  const win = frame.ownerDocument.defaultView ?? window;

  // An opaque-origin frame can only be addressed with '*'; its window identifies it.
  const reply = (message: HostMessage) => frame.contentWindow?.postMessage(message, '*');
  const sendWatch = () => reply({ type: 'anfra:inspect-watch', open: inspecting });

  const onMessage = (event: MessageEvent) => {
    if (event.source !== frame.contentWindow) return;
    const message = event.data as FrameMessage | undefined;
    switch (message?.type) {
      case 'anfra:ready':
        if (inspecting) sendWatch();
        return;
      case 'anfra:inspect':
        if (inspecting && Array.isArray(message.apps)) {
          callbacks.onInspect?.(message.apps, message.structure ?? { nodes: [], usage: {} });
        }
        return;
      case 'anfra:picked':
        if (inspecting) callbacks.onPick?.(message.node);
        return;
      case 'anfra:cancel':
        inFlight.get(message.id)?.abort();
        inFlight.delete(message.id);
        return;
      case 'anfra:request': {
        const { id } = message;
        const controller = new AbortController();
        inFlight.set(id, controller);
        const result = message.method === 'submitQuery'
          ? backend.submitQuery(message.request, controller.signal)
          : message.method === 'fieldSuggestions'
            ? backend.fieldSuggestions(message.request, controller.signal)
            : Promise.reject(new Error(`No such Backend method: ${String((message as { method: unknown }).method)}`));
        result
          .then((value) => reply({ type: 'anfra:response', id, ok: true, result: value }))
          .catch((err: unknown) => {
            // A cancelled call already rejected in the frame; there is no one left to answer.
            if ((err as Error | undefined)?.name === 'AbortError') return;
            reply({ type: 'anfra:response', id, ok: false, error: toBridgeError(err) });
          })
          .finally(() => inFlight.delete(id));
        return;
      }
      default:
    }
  };

  win.addEventListener('message', onMessage);
  return {
    setInspecting (open) {
      inspecting = open;
      sendWatch();
    },
    highlight (node) {
      reply({ type: 'anfra:highlight', node });
    },
    locate (target) {
      reply({ type: 'anfra:locate', target });
    },
    setPicking (on) {
      reply({ type: 'anfra:pick-watch', on });
    },
    stop () {
      win.removeEventListener('message', onMessage);
      inFlight.forEach((controller) => controller.abort());
      inFlight.clear();
    },
  };
}
