/**
 * The host runtime: the plugins a page installed, the middleware around their calls, and the
 * frames mounted into it. One per page; every call, from a frame or from a plugin, is dispatched
 * through its one pipeline (docs/designs/plugins.md).
 */
import { PermissionError, TransportError, ValidationError } from '../common/errors';
import type { BridgeMessage, PluginInfo, Provision } from '../common/bridge';
import { createEndpoint, type Endpoint } from '../common/endpoint';
import type { DatasetDescriptor, User } from '../common/types';
import type {
  CallContext, CallDefinition, Caller, CallOptions, EventCheck, HandlerCallOptions, HandlerContext, Middleware, MountContext, Plugin,
} from './plugin';
import { checkPlugin } from './plugin';
import { builtFrameScript } from './frameScript';
import { provisionDocument } from './provision';

/** The kernel's own call: a plugin's frame body, asked for on its first use. */
export const BODY_CALL = 'anfra.body';

export interface HostOptions {
  /** The plugins this page's Data Apps get. `core` is required: the app model runs its queries with it. */
  plugins: readonly Plugin[];
  /** Middleware around calls, beside what plugins bring. */
  middleware?: readonly Middleware[];
  /** The frame script; the one built into this package unless given (tests). */
  frameScript?: string;
}

export interface MountOptions {
  /** Where the frame goes: it is appended to this element. */
  container: HTMLElement;
  /** The Data App definition: its HTML, as its author wrote it. */
  definition: string;
  /** Where the definition's relative URLs resolve. */
  baseHref: string;
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  title?: string;
}

export interface MountedDataApp {
  frame: HTMLIFrameElement;
  /**
   * Make a call about this frame: a host half's (through the pipeline), or, when no host half
   * answers the name, a frame half's, in the frame.
   */
  call: <T = unknown>(name: string, request?: unknown, options?: CallOptions) => Promise<T>;
  /** Send the frame an event. */
  emit: (name: string, payload?: unknown) => void;
  /** Listen to the frame's events, by full name. Returns the function that stops listening. */
  on: (name: string, listener: (payload: any) => void) => () => void; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** Stop answering the frame, and remove it. */
  unmount: () => void;
}

export interface Host {
  /** The installed plugins, in install order. */
  readonly plugins: readonly PluginInfo[];
  /** Make a call as the host page, about no frame. */
  call: <T = unknown>(name: string, request?: unknown, options?: CallOptions) => Promise<T>;
  /**
   * Mount a Data App: a sandboxed frame (scripts only, an opaque origin, so its own requests reach
   * no API) whose document is the definition with the SDK and the plugins' frame halves provisioned
   * ahead of it. The bridge is served before the document loads, so no call is missed.
   */
  mount: (options: MountOptions) => MountedDataApp;
}

interface Answer {
  plugin: string;
  definition: CallDefinition;
}

function matches (pattern: string, name: string): boolean {
  if (pattern === '*') return true;
  if (pattern.endsWith('.*')) return name.startsWith(pattern.slice(0, -1));
  return pattern === name;
}

/** Check what is installed, and index the calls. */
function install (plugins: readonly Plugin[]): Map<string, Answer> {
  const names = new Set<string>();
  for (const plugin of plugins) {
    checkPlugin(plugin);
    if (names.has(plugin.name)) throw new Error(`Plugin '${plugin.name}' is installed twice.`);
    names.add(plugin.name);
  }
  if (!names.has('core')) {
    throw new Error("A host needs the 'core' plugin, which runs a Data App's queries: install corePlugin(backend).");
  }
  for (const plugin of plugins) {
    const missing = (plugin.requires ?? []).filter((name) => !names.has(name));
    if (missing.length) {
      throw new Error(`Plugin '${plugin.name}' needs ${missing.map((name) => `'${name}'`).join(', ')}, which ${missing.length > 1 ? 'are' : 'is'} not installed.`);
    }
  }
  const answers = new Map<string, Answer>();
  for (const plugin of plugins) {
    for (const [call, definition] of Object.entries(plugin.host?.calls ?? {})) {
      answers.set(`${plugin.name}.${call}`, {
        plugin: plugin.name,
        definition: typeof definition === 'function' ? { handler: definition } : definition,
      });
    }
  }
  return answers;
}

export function createHost (options: HostOptions): Host {
  const plugins = [...options.plugins];
  const answers = install(plugins);
  const middleware = [
    ...(options.middleware ?? []),
    ...plugins.flatMap((plugin) => plugin.host?.middleware ?? []),
  ].sort((a, b) => (a.stage ?? 0) - (b.stage ?? 0) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  const bodies = new Map(plugins.flatMap((plugin) => (plugin.frame?.body ? [[plugin.name, plugin.frame.body] as const] : [])));
  // The kernel's own call: a plugin's frame body. A call like any other, through the middleware.
  answers.set(BODY_CALL, {
    plugin: 'anfra',
    definition: {
      frame: true,
      handler: (request: unknown) => {
        const plugin = (request as { plugin?: unknown } | null)?.plugin;
        const body = typeof plugin === 'string' ? bodies.get(plugin) : undefined;
        if (body === undefined) throw new ValidationError(`No plugin '${String(plugin)}' with a frame body is installed.`);
        return body;
      },
    },
  });
  // The events a frame may send, by full name, each with its plugin's check.
  const events = new Map<string, EventCheck>(plugins.flatMap((plugin) => Object.entries(plugin.host?.events ?? {})
    .map(([name, check]) => [`${plugin.name}.${name}`, check] as const)));
  const infos: PluginInfo[] = plugins.map(({ name, version }) => ({ name, version }));
  let nextMount = 1;

  /**
   * Run a call through the middleware that match it, then its handler. Resolves to the answer;
   * `undefined` for a name no host half answers, which the caller decides about.
   */
  function dispatch (name: string, request: unknown, callers: readonly Caller[], mount: MountContext | undefined, signal: AbortSignal, asOrigin = false): Promise<unknown> {
    const answer = answers.get(name);
    if (!answer) return Promise.reject(new TransportError(`Nothing on this host answers '${name}'.`));
    const caller = callers[callers.length - 1];
    const origin = callers[0];
    // The frame itself, or, when asked, a plugin calling for a chain the frame started.
    if ((caller.kind === 'frame' || (asOrigin && origin.kind === 'frame')) && answer.definition.frame !== true) {
      return Promise.reject(new PermissionError(`'${name}' can't be called from a Data App.`));
    }
    const chain = middleware.filter((m) => (typeof m.match === 'string' ? [m.match] : m.match).some((p) => matches(p, name)));
    const trace: string[] = [];
    const context: CallContext = {
      name, caller, chain: callers, origin, signal, mount, trace,
    };
    const run = (index: number, current: unknown): Promise<unknown> => {
      if (signal.aborted) return Promise.reject(signal.reason ?? new Error('Aborted'));
      const layer = chain[index];
      if (!layer) {
        const handlerContext: HandlerContext = {
          ...context,
          call: (inner, innerRequest, innerOptions) => call(inner, innerRequest, [...callers, { kind: 'plugin', plugin: answer.plugin }], mount, innerOptions) as never,
        };
        return Promise.resolve().then(() => answer.definition.handler(current, handlerContext));
      }
      trace.push(layer.name);
      let called = false;
      return Promise.resolve().then(() => layer.handle(current, context, (next) => {
        if (called) return Promise.reject(new Error(`Middleware '${layer.name}' called next() twice.`));
        called = true;
        return run(index + 1, next === undefined ? current : next);
      }));
    };
    return run(0, request);
  }

  /** A call the host or a plugin makes: a host half's, or a frame half's in `mount`'s frame. */
  function call (name: string, request: unknown, callers: readonly Caller[], mount: MountContext | undefined, callOptions: HandlerCallOptions = {}): Promise<unknown> {
    const signal = callOptions.signal ?? new AbortController().signal;
    if (!answers.has(name) && mount) return mount.call(name, request, { signal });
    return dispatch(name, request, callers, mount, signal, callOptions.asOrigin === true);
  }

  return {
    plugins: infos,
    call: (name, request, callOptions) => call(name, request, [{ kind: 'host' }], undefined, callOptions) as never,
    mount (mountOptions) {
      const frameScript = options.frameScript ?? builtFrameScript();
      if (!frameScript) throw new Error('@holistics/anfra-sdk/host: built without its frame script; build the package (pnpm build).');

      const doc = mountOptions.container.ownerDocument;
      const win = doc.defaultView ?? window;
      const frame = doc.createElement('iframe');
      frame.setAttribute('sandbox', 'allow-scripts');
      frame.title = mountOptions.title ?? 'Data App';
      mountOptions.container.appendChild(frame);

      const listeners = new Map<string, Set<(payload: unknown) => void>>();
      const readyListeners = new Set<() => void>();
      const listen = <T>(set: Set<T>, listener: T) => {
        set.add(listener);
        return () => { set.delete(listener); };
      };

      // An opaque-origin frame can only be addressed with '*'; its window identifies it.
      const post = (message: BridgeMessage) => frame.contentWindow?.postMessage(message, '*');
      // eslint-disable-next-line prefer-const -- the context and the endpoint refer to each other
      let endpoint: Endpoint;
      const context: MountContext = {
        id: nextMount++,
        frame,
        document: doc,
        call: (name, request, callOptions) => endpoint.call(name, request, callOptions?.signal),
        emit: (name, payload) => endpoint.emit(name, payload),
        on (name, listener) {
          if (!listeners.has(name)) listeners.set(name, new Set());
          return listen(listeners.get(name)!, listener);
        },
        onReady: (listener) => listen(readyListeners, listener),
      };
      endpoint = createEndpoint(post, {
        answer: (name, request, signal) => dispatch(name, request, [{ kind: 'frame' }], context, signal),
        event: (name, payload) => {
          // Only what a plugin declared, as its check let it through: the frame is untrusted.
          const check = events.get(name);
          if (!check || !listeners.has(name)) return;
          let checked: unknown;
          try { checked = check(payload, context); } catch { return; }
          if (checked === undefined) return;
          listeners.get(name)?.forEach((listener) => {
            try { listener(checked); } catch { /* one listener must not stop the others */ }
          });
        },
      });

      const onMessage = (event: MessageEvent) => {
        if (event.source !== frame.contentWindow) return;
        const message = event.data as BridgeMessage | undefined;
        if (message?.type === 'anfra:ready') {
          readyListeners.forEach((listener) => listener());
          return;
        }
        if (message) endpoint.receive(message);
      };
      win.addEventListener('message', onMessage);

      const disposers = plugins.flatMap((plugin) => {
        const dispose = plugin.host?.mount?.(context);
        return typeof dispose === 'function' ? [dispose] : [];
      });

      const provision: Provision = {
        datasets: mountOptions.datasets,
        user: mountOptions.user,
        plugins: infos,
        hostOrigin: win.location.origin,
      };
      frame.srcdoc = provisionDocument(mountOptions.definition, {
        baseHref: mountOptions.baseHref,
        provision,
        frameScript,
        setups: plugins.flatMap((plugin) => (plugin.frame ? [{ name: plugin.name, setup: plugin.frame.setup }] : [])),
      });

      return {
        frame,
        call: (name, request, callOptions) => call(name, request, [{ kind: 'host' }], context, callOptions) as never,
        emit: context.emit,
        on: context.on,
        unmount () {
          win.removeEventListener('message', onMessage);
          endpoint.close(new ValidationError('The Data App was unmounted before the call finished.'));
          disposers.forEach((dispose) => dispose());
          listeners.clear();
          readyListeners.clear();
          frame.remove();
        },
      };
    },
  };
}
