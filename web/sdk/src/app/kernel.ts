/**
 * The kernel, as a plugin's frame half sees it: the app model, the bridge under the plugin's name,
 * a place in the plugin layer, and the ways to add API for the author. One per plugin, made by the
 * bootstrap before the definition runs (docs/designs/plugins.md).
 */
import {
  DataAppError, PermissionError, QueryError, TransportError, ValidationError,
} from '../common/errors';
import type { Endpoint } from '../common/endpoint';
import type { App } from './app';
import type { Control } from './controls';
import type { Query } from './query';
import type { Sdk } from './sdk';
import { buildStructure, type BuiltStructure } from './structure';
import { layerRoot } from './layer';
import { overlayFor, type Overlay } from './overlay';
import { addExtension } from './extensions';

/** The kernel's own call: a plugin's frame body (answered by the host runtime). */
export const BODY_CALL = 'anfra.body';

/** Where a frame body leaves what it exports, for the moment it takes to read it. */
const BODY_GLOBAL = '__anfraBodyExports';

export const KERNEL_ERRORS = {
  DataAppError, ValidationError, QueryError, PermissionError, TransportError,
} as const;

export interface FrameKernel {
  /** The plugin's name: what its calls, events and API are named under. */
  readonly plugin: string;
  readonly window: Window;
  /** The SDK instance the definition gets as `Anfra`: its apps, datasets and user. */
  readonly sdk: Sdk;
  /** The SDK's error classes: reject with these, so an author can tell them apart. */
  readonly errors: typeof KERNEL_ERRORS;
  /** The overlay every part of the SDK shows nodes with. */
  readonly overlay: Overlay;
  /** The page's structure as it is now. */
  structure: () => BuiltStructure;
  /** Make a host half's call, by its full name (`core.submitQuery`). */
  call: <T = unknown>(name: string, request?: unknown, signal?: AbortSignal) => Promise<T>;
  /** Send the host one of this plugin's events, by its name under the plugin. */
  emit: (name: string, payload?: unknown) => void;
  /** Answer one of this plugin's calls made by the host, by its name under the plugin. */
  handle: (name: string, handler: (request: any, signal: AbortSignal) => unknown) => void; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** Listen to one of this plugin's events from the host, by its name under the plugin. */
  on: (name: string, listener: (payload: any) => void) => void; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** Each app the definition creates, and each already created. */
  onApp: (listener: (app: App) => void) => void;
  /** `Anfra.<plugin>`. */
  provide: (api: unknown) => void;
  /** `query.<plugin>` on every query, made once per query. */
  provideOnQuery: (factory: (query: Query) => unknown) => void;
  /** `control.<plugin>` on every control, made once per control. */
  provideOnControl: (factory: (control: Control) => unknown) => void;
  /** This plugin's shadow root in the plugin layer. */
  layer: () => ShadowRoot;
  /** The plugin's frame body, fetched and run on first use: what it exports. */
  body: <T = unknown>() => Promise<T>;
}

/** What every plugin's kernel shares: the frame, the SDK, and the bridge's two directions. */
export interface KernelShared {
  window: Window;
  sdk: Sdk;
  endpoint: Endpoint;
  /** Calls the host makes into the frame, by full name. */
  handlers: Map<string, (request: unknown, signal: AbortSignal) => unknown>;
  /** Events the host sends, by full name. */
  listeners: Map<string, Set<(payload: unknown) => void>>;
  /** Listeners for each app created. */
  appListeners: Set<(app: App) => void>;
}

/** Run a frame body's code and take what it exports (it assigns `__anfraBody`). */
function runBody (win: Window, code: string): unknown {
  const target = win as unknown as Record<string, unknown>;
  const script = win.document.createElement('script');
  script.textContent = `(function () { var __anfraBody;\n${code}\n;self[${JSON.stringify(BODY_GLOBAL)}] = __anfraBody; })();`;
  (win.document.head ?? win.document.documentElement).appendChild(script);
  script.remove();
  const exported = target[BODY_GLOBAL] as { default?: unknown } | undefined;
  delete target[BODY_GLOBAL];
  if (!exported) throw new ValidationError('The host sent a plugin body that did not run.');
  return exported.default ?? exported;
}

export function createKernel (plugin: string, shared: KernelShared, entities: { Query: { prototype: Query }, Control: { prototype: Control } }): FrameKernel {
  const { window: win, sdk, endpoint } = shared;
  let loading: Promise<unknown> | undefined;
  return {
    plugin,
    window: win,
    sdk,
    errors: KERNEL_ERRORS,
    // Made on first use: a frame nobody inspects or locates in draws no layer at all.
    get overlay () {
      return overlayFor(win.document);
    },
    structure: () => buildStructure(win.document, sdk.apps),
    call: (name, request, signal) => endpoint.call(name, request ?? null, signal),
    emit: (name, payload) => endpoint.emit(`${plugin}.${name}`, payload ?? null),
    handle (name, handler) {
      shared.handlers.set(`${plugin}.${name}`, handler);
    },
    on (name, listener) {
      const full = `${plugin}.${name}`;
      if (!shared.listeners.has(full)) shared.listeners.set(full, new Set());
      shared.listeners.get(full)!.add(listener);
    },
    onApp (listener) {
      shared.appListeners.add(listener);
      sdk.apps.forEach((app) => listener(app));
    },
    provide (api) {
      if (plugin in sdk) throw new ValidationError(`Plugin '${plugin}' can't add a member Anfra already has.`);
      Object.defineProperty(sdk, plugin, { value: api, enumerable: true });
    },
    provideOnQuery: (factory) => addExtension(sdk.extensions.query, entities.Query.prototype, 'query', plugin, factory),
    provideOnControl: (factory) => addExtension(sdk.extensions.control, entities.Control.prototype, 'control', plugin, factory),
    layer: () => layerRoot(win.document, plugin),
    body<T> () {
      loading ??= endpoint.call<string>(BODY_CALL, { plugin })
        .then((code) => runBody(win, code))
        .catch((err: unknown) => {
          loading = undefined;
          throw err;
        });
      return loading as Promise<T>;
    },
  };
}
