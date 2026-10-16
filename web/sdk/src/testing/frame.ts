/**
 * A frame for testing an SDK plugin's frame half: the real kernel and plugin setups, on the test's
 * DOM (jsdom, happy-dom), talking over the real bridge to a stand-in host whose answers the test
 * gives. See docs/usage/sdk-plugins.md, Testing.
 */
import { createEndpoint } from '../common/endpoint';
import { PROVISION_ELEMENT_ID, type BridgeMessage, type PluginInfo, type Provision } from '../common/bridge';
import type { DatasetDescriptor, User } from '../common/types';
import { bootstrap, type PluginSetup } from '../app/bootstrap';
import type { Sdk } from '../app/sdk';

/** The reader a test frame is provisioned with unless given. */
export const TEST_READER: User = {
  id: 1,
  name: 'Test Reader',
  email: 'reader@example.com',
  role: 'analyst',
  timezone: 'UTC',
  permissions: { canViewGeneratedSql: true, canExportData: true },
};

export interface TestFrameOptions {
  /** The plugins' setups, by name, in install order: `[['notes', setup]]`, `setup` the frame half's default export. */
  setups?: readonly PluginSetup[];
  /** What the frame is provisioned with; none unless given. */
  datasets?: Record<string, DatasetDescriptor>;
  /** The reader; `TEST_READER` unless given. */
  user?: User;
  /** The host's answer to each frame call, by full name. A call without one fails as nothing answers it. */
  answers?: Record<string, (request: any, signal: AbortSignal) => unknown>; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** What a plugin's frame body exports, by plugin: jsdom runs no injected script, so the test gives it. */
  bodies?: Record<string, unknown>;
  /** `Anfra.plugins`; the setups' names unless given. */
  plugins?: PluginInfo[];
}

export interface TestFrame {
  win: Window;
  /** `Anfra`, with whatever the plugins added. */
  anfra: Sdk & Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** The frame's calls, in order. */
  calls: { name: string, request: unknown }[];
  /** The frame's events, in order. */
  events: { name: string, payload: unknown }[];
  /** How often the frame said it was ready. */
  ready: () => number;
  /** The host making a call into the frame. */
  call: <T = unknown>(name: string, request?: unknown) => Promise<T>;
  /** The host sending the frame an event. */
  emit: (name: string, payload?: unknown) => void;
}

/** Until queued messages and the work they start have run. */
export const flush = () => new Promise((resolve) => { setTimeout(resolve, 0); });

export function testFrame (options: TestFrameOptions = {}): TestFrame {
  const setups = options.setups ?? [];
  const provision: Provision = {
    datasets: options.datasets ?? {},
    user: options.user ?? TEST_READER,
    plugins: options.plugins ?? setups.map(([name]) => ({ name, version: 'test' })),
    hostOrigin: 'http://host.test',
  };
  document.getElementById(PROVISION_ELEMENT_ID)?.remove();
  const element = document.createElement('script');
  element.type = 'application/json';
  element.id = PROVISION_ELEMENT_ID;
  element.textContent = JSON.stringify(provision);
  document.head.appendChild(element);

  const frameListeners: ((event: MessageEvent) => void)[] = [];
  const calls: TestFrame['calls'] = [];
  const events: TestFrame['events'] = [];
  let readyCount = 0;

  // A window that is the test's document, with a parent that is the stand-in host.
  const win = Object.create(window) as Window & Record<string, unknown>;
  const toFrame = (message: BridgeMessage) => {
    queueMicrotask(() => frameListeners.forEach((listener) => listener({ source: parent, data: message } as unknown as MessageEvent)));
  };
  const host = createEndpoint(toFrame, {
    answer: async (name, request, signal) => {
      calls.push({ name, request });
      if (name === 'anfra.body') {
        const plugin = (request as { plugin: string }).plugin;
        if (!options.bodies || !(plugin in options.bodies)) throw new Error(`No body for '${plugin}'.`);
        win.__anfraBodyExports = { default: options.bodies[plugin] };
        return '';
      }
      const answer = options.answers?.[name];
      if (!answer) throw new Error(`Nothing answers '${name}'.`);
      return answer(request, signal);
    },
    event: (name, payload) => { events.push({ name, payload }); },
  });
  const parent = {
    postMessage (message: BridgeMessage) {
      if (message.type === 'anfra:ready') { readyCount += 1; return; }
      queueMicrotask(() => host.receive(message));
    },
  };
  Object.defineProperties(win, {
    parent: { value: parent },
    document: { value: document },
    // Called on the stand-in, which jsdom's own would refuse as not a window.
    setTimeout: { value: window.setTimeout.bind(window) },
    clearTimeout: { value: window.clearTimeout.bind(window) },
    requestAnimationFrame: { value: (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0) },
    addEventListener: {
      value: (type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') frameListeners.push(listener);
        else window.addEventListener(type, listener as EventListener);
      },
    },
  });

  bootstrap(win, setups);
  return {
    win,
    anfra: win.Anfra as TestFrame['anfra'],
    calls,
    events,
    ready: () => readyCount,
    call: (name, request) => host.call(name, request ?? null),
    emit: (name, payload) => host.emit(name, payload ?? null),
  };
}
