/**
 * One side of the bridge: makes calls and matches their results, answers the other side's calls
 * (aborting one on its cancel), and passes events on. The host and the frame each run one, over
 * their own `post`; neither knows postMessage beyond that.
 */
import {
  PermissionError, QueryError, TransportError, ValidationError,
} from './errors';
import type { BridgeError, BridgeMessage } from './bridge';

export function abortError (): Error {
  const err = new Error('Aborted');
  err.name = 'AbortError';
  return err;
}

export function isAbortError (err: unknown): boolean {
  return (err as Error | undefined)?.name === 'AbortError';
}

export function toBridgeError (err: unknown): BridgeError {
  const e = err as Error | undefined;
  return { name: e?.name || 'QueryError', message: e?.message || 'The call failed.' };
}

/**
 * Errors cross the bridge as { name, message }; rebuild the SDK's class so an author can tell a
 * permission failure from a broken query. Any other name is the SDK's `QueryError`.
 */
export function fromBridgeError (error: BridgeError | undefined): Error {
  switch (error?.name) {
    case 'PermissionError': return new PermissionError(error.message);
    case 'TransportError': return new TransportError(error.message);
    case 'ValidationError': return new ValidationError(error.message);
    default: return new QueryError(error?.message || 'The call failed.');
  }
}

export interface EndpointHandlers {
  /** Answer one of the other side's calls; `signal` aborts when it cancels. */
  answer: (name: string, request: unknown, signal: AbortSignal) => Promise<unknown>;
  /** One of the other side's events. */
  event: (name: string, payload: unknown) => void;
}

export interface Endpoint {
  call: <T = unknown>(name: string, request: unknown, signal?: AbortSignal) => Promise<T>;
  emit: (name: string, payload: unknown) => void;
  /** A message from the other side. False when it is not one of the bridge's. */
  receive: (message: BridgeMessage) => boolean;
  /** Abort every call either way: the other side is gone. Pending calls reject with `reason`. */
  close: (reason: Error) => void;
}

export function createEndpoint (post: (message: BridgeMessage) => void, handlers: EndpointHandlers): Endpoint {
  const pending = new Map<number, { resolve: (value: unknown) => void, reject: (err: Error) => void }>();
  const inFlight = new Map<number, AbortController>();
  let nextId = 1;

  const answer = (id: number, name: string, request: unknown) => {
    const controller = new AbortController();
    inFlight.set(id, controller);
    Promise.resolve()
      .then(() => handlers.answer(name, request, controller.signal))
      .then((result) => {
        if (!controller.signal.aborted) post({ type: 'anfra:result', id, ok: true, result });
      })
      .catch((err: unknown) => {
        // A cancelled call already rejected on the other side; there is no one left to answer.
        if (controller.signal.aborted || isAbortError(err)) return;
        post({ type: 'anfra:result', id, ok: false, error: toBridgeError(err) });
      })
      .finally(() => {
        if (inFlight.get(id) === controller) inFlight.delete(id);
      });
  };

  return {
    call (name, request, signal) {
      return new Promise((resolve, reject) => {
        if (signal?.aborted) {
          reject(abortError());
          return;
        }
        const id = nextId++;
        pending.set(id, { resolve: resolve as (value: unknown) => void, reject });
        signal?.addEventListener('abort', () => {
          if (!pending.delete(id)) return;
          post({ type: 'anfra:cancel', id });
          reject(abortError());
        }, { once: true });
        post({ type: 'anfra:call', id, name, request });
      });
    },
    emit (name, payload) {
      post({ type: 'anfra:event', name, payload });
    },
    receive (message) {
      switch (message?.type) {
        case 'anfra:call':
          answer(message.id, message.name, message.request);
          return true;
        case 'anfra:result': {
          const call = pending.get(message.id);
          if (!call) return true;
          pending.delete(message.id);
          if (message.ok) call.resolve(message.result);
          else call.reject(fromBridgeError(message.error));
          return true;
        }
        case 'anfra:cancel':
          inFlight.get(message.id)?.abort();
          inFlight.delete(message.id);
          return true;
        case 'anfra:event':
          handlers.event(message.name, message.payload);
          return true;
        default:
          return false;
      }
    },
    close (reason) {
      inFlight.forEach((controller) => controller.abort());
      inFlight.clear();
      pending.forEach(({ reject }) => reject(reason));
      pending.clear();
    },
  };
}
