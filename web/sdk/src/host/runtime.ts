/**
 * The host runtime: the plugins a page installed, the middleware around their calls, and the
 * frames mounted into it. One per page; every call, from a frame or from a plugin, is dispatched
 * through its one pipeline (docs/designs/sdk-plugins.md).
 */
import { DataAppError, forbidden, invalidRequest } from '../common/errors';
import type { BridgeMessage, PluginInfo, Provision } from '../common/bridge';
import { createEndpoint, type Endpoint } from '../common/endpoint';
import {
  checkConsoleEvent, checkErrorEvent, createBudget, describeError, type AppErrorEvent,
} from '../common/runtimeErrors';
import type { DatasetDescriptor, User } from '../common/types';
import type {
  CallContext, CallDefinition, Caller, CallOptions, EventCheck, HandlerCallOptions, HandlerContext, Middleware, MountContext, Plugin,
} from './plugin';
import { checkPlugin } from './plugin';
import { builtFrameScript, builtUiBody } from './frameScript';
import { checkTarget, type MenuEntry, type MenuTarget } from '../common/menu';
import { definitionStack, provisionFrame, type DefinitionLines } from './provision';
import { contentPolicy } from './policy';
import { createPanel, type Panel } from './panel';
import { applyTheme, resolveTheme, type Theme, type ThemeInput } from '../common/theme';
import { eventShortcut, isTyping, normalizeShortcut } from '../common/shortcut';
import {
  commandState, installContributions, type Command, type CommandContribution, type CommandState, type Contribution, type Tab,
  type TabContribution,
} from './contributions';

/** The kernel's own call: a plugin's frame body, asked for on its first use. */
export const BODY_CALL = 'anfra.body';

export interface HostOptions {
  /** The plugins this page's Data Apps get. `core` is required: the app model runs its queries with it. */
  plugins: readonly Plugin[];
  /** Middleware around calls, beside what plugins bring. */
  middleware?: readonly Middleware[];
  /** The frame script; the one built into this package unless given (tests). */
  frameScript?: string;
  /** The kernel's UI body, for the frame's menu, modal and popover; the one built in unless given (tests). */
  uiBody?: string;
  /**
   * Origins a Data App may load scripts, styles, fonts and images from, and fetch them from, beside
   * the policy's own CDNs: each `https://<host>[:<port>]`, nothing after it. Never a way to the
   * network at large (docs/designs/security.md).
   */
  allowOrigins?: readonly string[];
  /** The host's own commands, beside the plugins': ids as given, unique among all. */
  commands?: readonly CommandContribution[];
  /** The host's own tabs, beside the plugins': ids as given, unique among all. */
  tabs?: readonly TabContribution[];
  /**
   * Which contributions, the plugins' and the host's own, the host shows: what it returns false for
   * is left out everywhere (`host.commands`, `host.tabs`, the panel). A plugin's calls stay, so a host
   * can draw its own UI on them instead.
   */
  contributions?: (contribution: Contribution) => boolean;
  /** The theme plugin UI follows: tabs, and frame halves in the plugin layer. Light defaults unless given. */
  theme?: ThemeInput;
  /**
   * Every mounted Data App's runtime errors: a query that failed, the app's uncaught errors, a
   * listener's exception, and a call this host failed and hid from the frame. Each mount's also
   * reach `mounted.on('anfra.error')`. `console.error` unless given; when given, the SDK writes
   * nothing to the page's console, not even how many errors a mount sent over its budget.
   */
  onError?: (event: AppErrorEvent) => void;
  /**
   * Whether frames report their `console.error` / `console.warn` lines, as `anfra.console` on each
   * mount. They can hold the app's data. Unless given, on in a development build of the host's page
   * and off in a production one (`process.env.NODE_ENV`).
   */
  captureConsole?: boolean;
}

declare const process: { env: { NODE_ENV?: string } };

/** The default for `captureConsole`: off only where the host's bundle says it is a production build. */
export function developmentBuild (): boolean {
  try {
    // Written out in full, so the host's bundler replaces it.
    return process.env.NODE_ENV !== 'production';
  } catch {
    return true; // no bundler, no process: nothing says it is a production build
  }
}

/**
 * How many runtime errors a mount takes in a minute, and console lines apart from them: the frame
 * holds itself to as many, but it runs anyone's code, so the host holds it to them too.
 */
const EVENTS_PER_MINUTE = 120;

const logError = (event: AppErrorEvent) => {
  // eslint-disable-next-line no-console -- a host that says nothing else gets its page's console
  console.error(`anfra: a Data App's ${event.kind} error${event.entity ? ` in '${event.entity}'` : ''}: ${event.message}`, event);
};

export interface MountOptions {
  /** Where the frame goes: it is appended to this element. */
  container: HTMLElement;
  /** The Data App definition: its HTML, as its author wrote it. */
  definition: string;
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
  /** This Data App's panel: the host's devtools over it, with the tabs plugins add. */
  panel: Panel;
  /** How a command, by full id, shows for this Data App now. */
  commandState: (id: string) => CommandState;
  /**
   * Run a command, by full id, for this Data App. Refused while its state says it is disabled. A
   * command for the page's content (`target`) runs from the frame's menu, with its target.
   */
  run: (id: string) => Promise<unknown>;
  /** Each time commands' state may have changed: a plugin said so, or the panel changed. */
  onCommandsChange: (listener: () => void) => () => void;
  /**
   * Run the command whose shortcut a key event on the host's own page makes, if any: whether one
   * did. Keys pressed while focus is in the frame are forwarded and run without this.
   */
  handleKeydown: (event: KeyboardEvent) => boolean;
  /** Stop answering the frame, and remove it. */
  unmount: () => void;
}

export interface Host {
  /** The installed plugins, in install order. */
  readonly plugins: readonly PluginInfo[];
  /** Every command to show: the plugins', then the host's own, as the filter kept them. */
  readonly commands: readonly Command[];
  /** Every tab for a mount's panel, in order, as the filter kept them. */
  readonly tabs: readonly Tab[];
  /** The theme plugin UI follows now, every token resolved. */
  readonly theme: Theme;
  /** Change the theme: every tab and every mounted frame's plugin layer follow it. */
  setTheme: (theme: ThemeInput) => void;
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
  const onError = options.onError ?? logError;
  // A host that says where its errors go has said where the SDK's notes go: none on its page.
  const quiet = options.onError !== undefined;
  const captureConsole = options.captureConsole ?? developmentBuild();
  const policy = contentPolicy(options.allowOrigins);
  const middleware = [
    ...(options.middleware ?? []),
    ...plugins.flatMap((plugin) => plugin.host?.middleware ?? []),
  ].sort((a, b) => (a.stage ?? 0) - (b.stage ?? 0) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  const bodies = new Map(plugins.flatMap((plugin) => (plugin.frame?.body ? [[plugin.name, plugin.frame.body] as const] : [])));
  const ui = options.uiBody ?? builtUiBody();
  if (ui) bodies.set('anfra', ui);
  // The kernel's own call: a plugin's frame body. A call like any other, through the middleware.
  answers.set(BODY_CALL, {
    plugin: 'anfra',
    definition: {
      frame: true,
      handler: (request: unknown) => {
        const plugin = (request as { plugin?: unknown } | null)?.plugin;
        const body = typeof plugin === 'string' ? bodies.get(plugin) : undefined;
        if (body === undefined) throw invalidRequest(`No plugin '${String(plugin)}' with a frame body is installed.`);
        return body;
      },
    },
  });
  // The kernel's menu calls, answered for the mount the frame is: its host commands for a target,
  // and running one the reader picked. Calls like any other, so middleware sees them.
  const menus = new WeakMap<MountContext, { entries: (target: MenuTarget) => MenuEntry[], run: (id: string, target: MenuTarget) => Promise<unknown> }>();
  const menuOf = (mount: MountContext | undefined) => {
    const menu = mount && menus.get(mount);
    if (!menu) throw invalidRequest('The menu is for a mounted Data App.');
    return menu;
  };
  const targetOf = (request: unknown): MenuTarget => {
    const target = checkTarget(request);
    if (!target) throw invalidRequest('Not a menu target.');
    return target;
  };
  answers.set('anfra.menu', {
    plugin: 'anfra',
    definition: { frame: true, handler: (request: unknown, { mount }) => menuOf(mount).entries(targetOf(request)) },
  });
  answers.set('anfra.run', {
    plugin: 'anfra',
    definition: {
      frame: true,
      handler: (request: unknown, { mount }) => {
        const id = (request as { id?: unknown } | null)?.id;
        if (typeof id !== 'string') throw invalidRequest('Name the command to run.');
        return menuOf(mount).run(id, targetOf((request as { target?: unknown }).target));
      },
    },
  });
  // The events a frame may send, by full name, each with its plugin's check.
  const events = new Map<string, EventCheck>(plugins.flatMap((plugin) => Object.entries(plugin.host?.events ?? {})
    .map(([name, check]) => [`${plugin.name}.${name}`, check] as const)));
  const infos: PluginInfo[] = plugins.map(({ name, version }) => ({ name, version }));
  const { commands, tabs } = installContributions(plugins, options, options.contributions);
  let theme = resolveTheme(options.theme);
  const themeListeners = new Set<(next: Theme) => void>();
  const tabIds = tabs.map((tab) => tab.id);
  const selectionTabIds = tabs.filter((tab) => tab.selection).map((tab) => tab.id);
  const pickTabIds = tabs.filter((tab) => tab.selection === 'pick').map((tab) => tab.id);
  // Which kinds of target the host's own menu commands are for: the frame listens only when asked.
  const menuKinds = [...new Set(commands.flatMap((command) => (command.target ? [command.target] : [])))];
  // The chrome's commands by shortcut: a menu command runs only with its target, so has none.
  const byShortcut = new Map<string, string>();
  for (const command of commands) {
    if (command.shortcut === undefined) continue;
    const shortcut = normalizeShortcut(command.shortcut);
    if (!shortcut) throw new Error(`Command '${command.id}' has a shortcut that isn't one: '${command.shortcut}'.`);
    if (command.target) throw new Error(`Command '${command.id}' is for the menu on the page, which has no shortcuts.`);
    const other = byShortcut.get(shortcut);
    if (other) throw new Error(`Commands '${other}' and '${command.id}' have the same shortcut, '${command.shortcut}'.`);
    byShortcut.set(shortcut, command.id);
  }
  let nextMount = 1;

  /**
   * Run a call through the middleware that match it, then its handler. Resolves to the answer;
   * `undefined` for a name no host half answers, which the caller decides about.
   */
  function dispatch (name: string, request: unknown, callers: readonly Caller[], mount: MountContext | undefined, signal: AbortSignal, asOrigin = false): Promise<unknown> {
    const answer = answers.get(name);
    if (!answer) return Promise.reject(invalidRequest(`Nothing on this host answers '${name}'.`));
    const caller = callers[callers.length - 1];
    const origin = callers[0];
    // The frame itself, or, when asked, a plugin calling for a chain the frame started.
    if ((caller.kind === 'frame' || (asOrigin && origin.kind === 'frame')) && answer.definition.frame !== true) {
      return Promise.reject(forbidden(`'${name}' can't be called from a Data App.`));
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

  /** Who a contribution's calls come from: the host, through its plugin if it has one. */
  const callersOf = (plugin: string | undefined): Caller[] => (plugin ? [{ kind: 'host' }, { kind: 'plugin', plugin }] : [{ kind: 'host' }]);

  return {
    plugins: infos,
    commands,
    tabs,
    get theme () { return theme; },
    setTheme (input) {
      theme = resolveTheme(input);
      themeListeners.forEach((listener) => {
        try { listener(theme); } catch { /* one listener must not stop the others */ }
      });
    },
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
      const commandListeners = new Set<() => void>();
      const commandsChanged = () => commandListeners.forEach((listener) => {
        try { listener(); } catch { /* one listener must not stop the others */ }
      });
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
        user: mountOptions.user,
        datasets: mountOptions.datasets,
        call: (name, request, callOptions) => endpoint.call(name, request, callOptions?.signal),
        emit: (name, payload) => endpoint.emit(name, payload),
        on (name, listener) {
          if (!listeners.has(name)) listeners.set(name, new Set());
          return listen(listeners.get(name)!, listener);
        },
        onReady: (listener) => listen(readyListeners, listener),
        commandsChanged,
      };
      // Set once the frame's document is made, before the frame can send anything.
      let definitionLines: DefinitionLines = { runs: [] };
      const mountInfo = { id: context.id, ...(mountOptions.title !== undefined ? { title: mountOptions.title } : {}) };
      const dropped = (what: string) => (count: number) => {
        if (quiet) return;
        // eslint-disable-next-line no-console -- what is over the budget is said only here
        console.warn(`anfra: ${count} ${what} of a Data App (mount ${mountInfo.id}) over the ${EVENTS_PER_MINUTE} a minute a host takes were dropped.`);
      };
      const errorBudget = createBudget(EVENTS_PER_MINUTE, dropped('runtime errors'));
      const consoleBudget = createBudget(EVENTS_PER_MINUTE, dropped('console lines'));
      /** A runtime error of this mount: to the host's callback, and to this mount's listeners. */
      const reportError = (event: AppErrorEvent) => {
        if (!errorBudget.take()) return;
        try { onError(event); } catch { /* the host's sink failing must not stop the listeners */ }
        deliver('anfra.error', event);
      };
      const deliver = (name: string, event: unknown) => listeners.get(name)?.forEach((listener) => {
        try { listener(event); } catch { /* one listener must not stop the others */ }
      });
      endpoint = createEndpoint(post, {
        answer: (name, request, signal) => dispatch(name, request, [{ kind: 'frame' }], context, signal),
        hidden: (name, err) => {
          const { message, stack, at } = describeError('hidden', err);
          reportError({
            kind: 'hidden',
            code: 'call_failed',
            scope: 'server',
            message,
            ...(stack !== undefined ? { stack } : {}),
            call: name,
            at,
            mount: mountInfo.id,
            ...(mountInfo.title !== undefined ? { title: mountInfo.title } : {}),
          });
        },
        event: (name, payload) => {
          // The kernel's runtime errors and console lines: the frame's word, checked.
          if (name === 'anfra.error') {
            const event = checkErrorEvent(payload, mountInfo);
            // The frame's stack names its document's lines: the author reads the definition's.
            if (event?.stack) event.stack = definitionStack(event.stack, definitionLines);
            if (event) reportError(event);
            return;
          }
          if (name === 'anfra.console') {
            const event = captureConsole ? checkConsoleEvent(payload, mountInfo) : undefined;
            if (event && consoleBudget.take()) deliver('anfra.console', event);
            return;
          }
          // The panel's own, from the kernel.
          if (name === 'anfra.snapshot') { panelState.snapshot(payload); return; }
          if (name === 'anfra.picked') { panelState.picked(payload); return; }
          if (name === 'anfra.shortcut') { runShortcut(payload, 'key'); return; }
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

      const panelState = createPanel(context, {
        tabs: () => tabIds,
        selectionTabs: () => selectionTabIds,
        pickTabs: () => pickTabIds,
        mountTab (panel, id, element) {
          const tab = tabs.find((candidate) => candidate.id === id);
          if (!tab) throw new Error(`No tab '${id}' is installed.`);
          const root = element.shadowRoot ?? element.attachShadow({ mode: 'open' });
          applyTheme(element.style, theme);
          const themed = (next: Theme) => applyTheme(element.style, next);
          themeListeners.add(themed);
          let disposed = false;
          let cleanup: void | (() => void);
          const done = (result: void | (() => void)) => {
            if (disposed) { if (typeof result === 'function') result(); return; }
            cleanup = result;
          };
          try {
            const result = tab.mount(root, {
              tab: id,
              panel,
              mount: context,
              plugins: infos,
              document: doc,
              get theme () { return theme; },
              call: (name, request, callOptions) => call(name, request, callersOf(tab.plugin), context, callOptions) as never,
            });
            if (result instanceof Promise) result.then(done, (err: unknown) => win.console?.error(`anfra: tab '${id}' failed to draw:`, err));
            else done(result);
          } catch (err) {
            win.console?.error(`anfra: tab '${id}' failed to draw:`, err);
          }
          return () => {
            if (disposed) return;
            disposed = true;
            themeListeners.delete(themed);
            try { if (typeof cleanup === 'function') cleanup(); } catch { /* the tab is going anyway */ }
            root.replaceChildren();
          };
        },
      });
      panelState.panel.subscribe(commandsChanged);
      const sendTheme = (next: Theme) => context.emit('anfra.theme', next);
      themeListeners.add(sendTheme);
      const stateContext = { mount: context, panel: panelState.panel };
      /** Run the command a shortcut names, for a key in the frame or the host's page: whether one did. */
      const runShortcut = (shortcut: unknown, from: 'host' | 'key' = 'host'): boolean => {
        const id = typeof shortcut === 'string' ? byShortcut.get(shortcut) : undefined;
        if (!id) return false;
        runCommand(id, undefined, from).catch(() => { /* a disabled command says why in its state, not here */ });
        return true;
      };
      const commandOf = (id: string) => {
        const command = commands.find((candidate) => candidate.id === id);
        if (!command) throw new Error(`No command '${id}' is installed.`);
        return command;
      };

      const onMessage = (event: MessageEvent) => {
        if (event.source !== frame.contentWindow) return;
        const message = event.data as BridgeMessage | undefined;
        if (message?.type === 'anfra:ready') {
          context.emit('anfra.theme', theme);
          if (menuKinds.length) context.emit('anfra.menu', { kinds: menuKinds });
          if (byShortcut.size) context.emit('anfra.shortcuts', [...byShortcut.keys()]);
          panelState.ready();
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
        captureConsole,
      };
      const provisioned = provisionFrame(mountOptions.definition, {
        contentPolicy: policy,
        provision,
        frameScript,
        setups: plugins.flatMap((plugin) => (plugin.frame ? [{ name: plugin.name, setup: plugin.frame.setup }] : [])),
      });
      definitionLines = provisioned.lines;
      frame.srcdoc = provisioned.html;

      /**
       * Run a command for this frame, unless its state says it can't run; one for content, with its
       * target. Asked by the host, the reader in the frame's menu, or a key the frame forwarded: the
       * frame may make that one up, so its call is held to what the frame may call itself.
       */
      const runCommand = async (id: string, target?: MenuTarget, from: 'host' | 'menu' | 'key' = 'host'): Promise<unknown> => {
        const command = commandOf(id);
        if (command.target && target?.kind !== command.target) throw invalidRequest(`'${id}' runs from the menu on the page, with a ${command.target} target.`);
        const { disabled, pressed } = commandState(command, stateContext, target);
        if (disabled) throw forbidden(disabled);
        if ('tab' in command.run) {
          const node = target ? (target.kind === 'node' ? target : target.node)?.node.key : undefined;
          const selected = node ?? command.run.target;
          if (selected !== undefined) panelState.panel.show(command.run.tab, selected);
          // A toggle: pressed, it closes the panel, whichever of its tabs shows.
          else if (pressed) panelState.panel.hide();
          else panelState.panel.show(command.run.tab);
          return null;
        }
        if ('panel' in command.run) {
          panelState.panel.pick(!panelState.panel.picking);
          return null;
        }
        const request = target ? { ...(command.run.request as object | undefined ?? {}), target } : command.run.request ?? null;
        const callers: Caller[] = from === 'host' ? callersOf(command.plugin) : [{ kind: 'frame' }, ...callersOf(command.plugin)];
        return call(command.run.call, request, callers, context, { asOrigin: from === 'key' });
      };
      menus.set(context, {
        entries: (target) => menuFor(target),
        // Picked in the frame: the command's call keeps the frame as its origin.
        run: (id, target) => {
          if (!commandOf(id).target) return Promise.reject(forbidden(`'${id}' isn't a menu command.`));
          return runCommand(id, target, 'menu');
        },
      });
      /** The host's menu commands that apply to a target, as the frame's menu shows them. */
      const menuFor = (target: MenuTarget): MenuEntry[] => commands.flatMap((command) => {
        if (command.target !== target.kind) return [];
        try {
          if (command.when && !command.when(target, stateContext)) return [];
        } catch {
          return [];
        }
        const { disabled } = commandState(command, stateContext, target);
        return [{ id: command.id, label: command.title, ...(disabled ? { disabled } : {}) }];
      });

      return {
        frame,
        call: (name, request, callOptions) => call(name, request, [{ kind: 'host' }], context, callOptions) as never,
        emit: context.emit,
        on: context.on,
        panel: panelState.panel,
        commandState: (id) => commandState(commandOf(id), stateContext),
        run: (id) => runCommand(id),
        onCommandsChange: (listener) => listen(commandListeners, listener),
        handleKeydown (event) {
          if (event.defaultPrevented || event.repeat || isTyping(event.target)) return false;
          if (!runShortcut(eventShortcut(event))) return false;
          event.preventDefault();
          return true;
        },
        unmount () {
          themeListeners.delete(sendTheme);
          commandListeners.clear();
          panelState.dispose();
          errorBudget.dispose();
          consoleBudget.dispose();
          win.removeEventListener('message', onMessage);
          endpoint.close(new DataAppError({ code: 'call_failed', scope: 'server', message: 'The Data App was unmounted before the call finished.' }));
          disposers.forEach((dispose) => dispose());
          listeners.clear();
          readyListeners.clear();
          frame.remove();
        },
      };
    },
  };
}
