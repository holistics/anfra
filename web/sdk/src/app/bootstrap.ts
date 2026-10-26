/**
 * The frame bootstrap: runs inside a Data App's sandboxed frame, before any of the Data App
 * definition's own scripts. It reads what its host provisioned, gives the app model a Backend that
 * makes `core`'s calls over the bridge, runs each plugin's setup, and installs the SDK as the
 * `Anfra` global. The only code in a frame that knows about postMessage.
 */
import { PROVISION_ELEMENT_ID } from '../common/bridge';
import type { BridgeMessage, Provision } from '../common/bridge';
import { createEndpoint } from '../common/endpoint';
import { TransportError } from '../common/errors';
import type { Backend, BackendQueryResult } from '../common/types';
import type { App } from './app';
import { Control } from './controls';
import { installInspection } from './inspection';
import { createKernel, type FrameKernel, type KernelShared } from './kernel';
import { Query } from './query';
import { createSdk } from './sdk';
import { installSandbox } from './sandbox';

/** A plugin's frame setup, as the host's frame script hands it over: its name and its function. */
export type PluginSetup = readonly [name: string, setup: ((kernel: FrameKernel) => void) | undefined];

/** Provision the SDK in `win` from its host, run the plugins' setups, and install it. Returns once `Anfra` exists. */
export function bootstrap (win: Window, setups: readonly PluginSetup[] = []): void {
  const element = win.document.getElementById(PROVISION_ELEMENT_ID);
  if (!element?.textContent) throw new Error('anfra: this frame was not provisioned by a host.');
  const provision = JSON.parse(element.textContent) as Provision;
  const post = (message: BridgeMessage) => win.parent.postMessage(message, provision.hostOrigin);

  const handlers: KernelShared['handlers'] = new Map();
  const listeners: KernelShared['listeners'] = new Map();
  const endpoint = createEndpoint(post, {
    answer: async (name, request, signal) => {
      const handler = handlers.get(name);
      if (!handler) throw new TransportError(`Nothing in this frame answers '${name}'.`);
      return handler(request, signal);
    },
    event: (name, payload) => {
      listeners.get(name)?.forEach((listener) => {
        try { listener(payload); } catch { /* one plugin's listener must not stop another's */ }
      });
    },
  });
  // The host is the parent, or a window of the host's origin: a host that mounts into a nested
  // document of its own posts from its page's window, and a browser names the window whose code
  // posted, never the frame's parent. An opaque host's origin names no one.
  const fromHost = (event: MessageEvent) => event.source === win.parent
    || (!!provision.hostOrigin && provision.hostOrigin !== 'null' && event.origin === provision.hostOrigin);
  win.addEventListener('message', (event: MessageEvent) => {
    if (!fromHost(event)) return;
    endpoint.receive(event.data as BridgeMessage);
  });

  // The app model's Backend: `core`'s calls, which every host answers.
  const backend: Backend = {
    submitQuery: (request, signal) => endpoint.call<BackendQueryResult>('core.submitQuery', request, signal).then((result) => {
      // Dates don't survive JSON; provenance's `executedAt` is one.
      if (result.debug?.executedAt) result.debug.executedAt = new Date(result.debug.executedAt);
      return result;
    }),
    fieldSuggestions: (request, signal) => endpoint.call('core.fieldSuggestions', request, signal),
  };

  const instance = createSdk({
    datasets: provision.datasets,
    user: provision.user,
    backend,
    plugins: provision.plugins ?? [],
  });

  // Lifecycle: tell the plugins of each app the definition creates.
  const appListeners = new Set<(app: App) => void>();
  const createApp = instance.createApp.bind(instance);
  instance.createApp = (declaration) => {
    const app = createApp(declaration);
    appListeners.forEach((listener) => {
      try { listener(app); } catch { /* a plugin's failure must not break the author's app */ }
    });
    return app;
  };

  const shared: KernelShared = {
    window: win, sdk: instance, endpoint, handlers, listeners, appListeners,
  };
  // The kernel's own part: what every host's panel needs of the frame, whatever plugins it installs.
  installInspection(createKernel('anfra', shared, { Query, Control }));
  for (const [name, setup] of setups) {
    if (typeof setup !== 'function') continue;
    try {
      setup(createKernel(name, shared, { Query, Control }));
    } catch (err) {
      // One broken plugin leaves the others, and the app, running.
      (win as Window & typeof globalThis).console?.error(`anfra: plugin '${name}' failed to set up:`, err);
    }
  }

  installSandbox(instance, win as unknown as Record<string, unknown>);
  post({ type: 'anfra:ready' });
}
