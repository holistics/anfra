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

  win.addEventListener('message', (event: MessageEvent) => {
    if (event.source !== win.parent) return;
    const message = event.data as HostMessage | undefined;
    if (message?.type === 'anfra:response') {
      const call = pending.get(message.id);
      if (!call) return;
      pending.delete(message.id);
      if (message.ok) call.resolve(message.result);
      else call.reject(toError(message.error));
    } else if (message?.type === 'anfra:inspect-watch') {
      inspecting = message.open;
      // eslint-disable-next-line @typescript-eslint/no-use-before-define -- the instance exists by the time a message arrives
      if (inspecting) postSnapshot();
    }
  });

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

  // Inspection: while the host's inspect panel is open, post a snapshot of every app on each
  // change. The instance is the only way to reach an app the author built, so wrap createApp to
  // subscribe to each one as it appears.
  function postSnapshot (): void {
    timer = undefined;
    if (!inspecting) return;
    try {
      post({ type: 'anfra:inspect', apps: instance.apps.map((app) => app.toInspectJSON()) });
    } catch {
      // A snapshot that will not clone must not break the author's app.
    }
  }
  function scheduleSnapshot (): void {
    if (inspecting && timer === undefined) timer = setTimeout(postSnapshot, 50);
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
