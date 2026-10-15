/**
 * What a plugin is, the host's side: a name, a version, the plugins it needs, a host half that
 * answers its calls, and the frame half's code. See docs/designs/plugins.md.
 */

/** Who made a call. Middleware and handlers can tell the definition's calls from a plugin's. */
export type Caller =
  /** The Data App's frame: its definition, or a plugin's frame half there. */
  | { kind: 'frame' }
  /** The host page's own code, through `Host.call` or `MountedDataApp.call`. */
  | { kind: 'host' }
  /** Another plugin's host half, by its name. */
  | { kind: 'plugin', plugin: string };

export interface CallOptions {
  signal?: AbortSignal;
}

/** One call, as its handler and each middleware see it. */
export interface CallContext {
  /** The call's full name, `<plugin>.<name>`. */
  readonly name: string;
  readonly caller: Caller;
  /** Aborts when the caller cancels. */
  readonly signal: AbortSignal;
  /** The frame the call is about: the one that made it, or the one the host named. */
  readonly mount?: MountContext;
  /** The middleware the call has passed through so far, by name, outermost first. */
  readonly trace: readonly string[];
}

/** What a plugin's handler gets: the call, and a way to make calls of its own as that plugin. */
export interface HandlerContext extends CallContext {
  /** Make another plugin's call, through the same pipeline, as this plugin, about the same frame. */
  call: <T = unknown>(name: string, request?: unknown, options?: CallOptions) => Promise<T>;
}

export type CallHandler = (request: any, context: HandlerContext) => unknown; // eslint-disable-line @typescript-eslint/no-explicit-any

export interface CallDefinition {
  handler: CallHandler;
  /** Whether a frame may make this call. Off unless true: a call is the host's until its plugin says. */
  frame?: boolean;
}

/**
 * Code around calls, chosen by name: `*`, `<plugin>.*`, or one call's full name. It may change the
 * request, answer in place of the plugin (not calling `next`), refuse the call (throwing), or change
 * the answer. Every call passes through it, whoever made it.
 */
export interface Middleware {
  /** Names the middleware in a call's trace, and orders it among middleware of the same stage. */
  name: string;
  match: string | readonly string[];
  /** Lower runs first (outermost). 0 unless given. Ties are ordered by name, never by registration. */
  stage?: number;
  handle: (request: unknown, context: CallContext, next: (request?: unknown) => Promise<unknown>) => unknown;
}

/** One mounted frame, as a plugin's host half sees it. */
export interface MountContext {
  readonly id: number;
  readonly frame: HTMLIFrameElement;
  /** The host page's document, where the frame is. */
  readonly document: Document;
  /** Make a call a frame half answers, in this frame. */
  call: <T = unknown>(name: string, request?: unknown, options?: CallOptions) => Promise<T>;
  /** Send this frame an event. */
  emit: (name: string, payload?: unknown) => void;
  /** Listen to this frame's events, by full name. Returns the function that stops listening. */
  on: (name: string, listener: (payload: any) => void) => () => void; // eslint-disable-line @typescript-eslint/no-explicit-any
  /** Each time the frame's document (re)loads and its kernel runs. */
  onReady: (listener: () => void) => () => void;
}

export interface HostHalf {
  /** The calls this plugin answers, by their name under the plugin (`submitQuery`, not `core.submitQuery`). */
  calls?: Record<string, CallHandler | CallDefinition>;
  /** Middleware this plugin puts around calls, its own or others'. */
  middleware?: readonly Middleware[];
  /** Each time a frame is mounted. What it returns runs when the frame is unmounted. */
  mount?: (mount: MountContext) => void | (() => void);
}

/** A frame half: code the host provisions into the frame, as script source (docs/designs/plugins.md). */
export interface FrameHalf {
  /** Runs before the definition, once per document: registers the plugin's API. */
  setup: string;
  /** Loaded on first use, through the kernel. */
  body?: string;
}

export interface Plugin {
  /** Lowercase letters, digits and dashes, starting with a letter. Names its calls, events and API. */
  name: string;
  version: string;
  /** Plugins this one needs; installing it without them fails. */
  requires?: readonly string[];
  host?: HostHalf;
  frame?: FrameHalf;
}

const NAME = /^[a-z][a-z0-9-]*$/;

/** Names reserved for the kernel: its own calls, and members of `Anfra` and its entities. */
export const RESERVED_NAMES: ReadonlySet<string> = new Set(['anfra', 'plugins']);

/** A plugin, checked: its name, and its calls' names. */
export function definePlugin<P extends Plugin> (plugin: P): P {
  if (!NAME.test(plugin?.name ?? '')) {
    throw new Error(`A plugin's name is lowercase letters, digits and dashes, starting with a letter: '${String(plugin?.name)}' is not.`);
  }
  if (RESERVED_NAMES.has(plugin.name)) throw new Error(`'${plugin.name}' is reserved for the SDK: name the plugin something else.`);
  for (const call of Object.keys(plugin.host?.calls ?? {})) {
    if (!/^[A-Za-z][A-Za-z0-9]*$/.test(call)) throw new Error(`Plugin '${plugin.name}': '${call}' is not a call name (letters and digits).`);
  }
  return plugin;
}
