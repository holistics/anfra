/**
 * The Shell's half of the Data App bridge. The frame's Backend posts `anfra:request` messages; this
 * answers each by calling the anfra server, and aborts the request on `anfra:cancel`. Only messages
 * from the given frame are served: a sandboxed frame has an opaque origin, so its window is what
 * identifies it.
 */

interface BridgeRequest {
  type: 'anfra:request';
  id: number;
  method: 'submitQuery' | 'fieldSuggestions';
  request: unknown;
}

interface BridgeCancel {
  type: 'anfra:cancel';
  id: number;
}

const ROUTES: Record<BridgeRequest['method'], string> = {
  submitQuery: '/_anfra/api/query',
  fieldSuggestions: '/_anfra/api/suggestions',
};

async function post (route: string, body: unknown, signal: AbortSignal): Promise<unknown> {
  const response = await fetch(route, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = (payload as { error?: { name?: string, message?: string } | string }).error;
    throw typeof error === 'object' && error
      ? Object.assign(new Error(error.message ?? 'The query failed.'), { name: error.name ?? 'QueryError' })
      : Object.assign(new Error(typeof error === 'string' ? error : `The server answered ${response.status}.`), { name: 'TransportError' });
  }
  return payload;
}

export interface BridgeHandle {
  /** Ask the frame to post (or stop posting) inspection snapshots; survives the frame reloading. */
  setInspecting: (open: boolean) => void;
  stop: () => void;
}

/** Serve a Data App frame's Backend calls, and relay its inspection snapshots, until `stop`. */
export function serveBridge (frame: HTMLIFrameElement, onInspect: (apps: unknown[]) => void): BridgeHandle {
  const inFlight = new Map<number, AbortController>();
  let inspecting = false;

  const reply = (message: unknown) => frame.contentWindow?.postMessage(message, '*');
  const sendWatch = () => reply({ type: 'anfra:inspect-watch', open: inspecting });

  const onMessage = (event: MessageEvent) => {
    if (event.source !== frame.contentWindow) return;
    const message = event.data as BridgeRequest | BridgeCancel | { type: 'anfra:ready' } | { type: 'anfra:inspect', apps: unknown[] } | undefined;

    // The frame's bootstrap is installed: tell it whether the Inspect panel is open.
    if (message?.type === 'anfra:ready') {
      if (inspecting) sendWatch();
      return;
    }
    if (message?.type === 'anfra:inspect') {
      if (inspecting && Array.isArray(message.apps)) onInspect(message.apps);
      return;
    }

    if (message?.type === 'anfra:cancel') {
      inFlight.get(message.id)?.abort();
      inFlight.delete(message.id);
      return;
    }
    if (message?.type !== 'anfra:request' || !(message.method in ROUTES)) return;

    const controller = new AbortController();
    inFlight.set(message.id, controller);
    post(ROUTES[message.method], message.request, controller.signal)
      .then((result) => reply({ type: 'anfra:response', id: message.id, ok: true, result }))
      .catch((err: Error) => {
        // A cancelled call already rejected in the frame; there is no one left to answer.
        if (err.name === 'AbortError') return;
        reply({
          type: 'anfra:response', id: message.id, ok: false, error: { name: err.name, message: err.message },
        });
      })
      .finally(() => inFlight.delete(message.id));
  };

  window.addEventListener('message', onMessage);
  return {
    setInspecting (open) {
      inspecting = open;
      sendWatch();
    },
    stop () {
      window.removeEventListener('message', onMessage);
      inFlight.forEach((controller) => controller.abort());
      inFlight.clear();
    },
  };
}
