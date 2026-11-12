/**
 * The bridge: the postMessage protocol between a Data App's frame (`app`'s bootstrap) and the
 * page hosting it (`host`). The frame has an opaque origin and no access to the API: everything it
 * reaches crosses here, as a plugin's calls and events (docs/designs/sdk-plugins.md).
 *
 * Both sides speak the same messages. A call is answered by its `id`, which each side counts on
 * its own: a result answers a call the other side made.
 */
import type { DatasetDescriptor, QueryStamp, User } from './types';

/** A plugin a host installed, as a frame sees it (`Anfra.plugins`). */
export interface PluginInfo {
  name: string;
  version: string;
}

/** What the host provisions a frame with, as JSON in `<script type="application/json" id="anfra-provision">`. */
export interface Provision {
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** Every plugin the host installed, whether or not it has a frame half. */
  plugins: PluginInfo[];
  /** The host page's origin: where the frame posts, so nothing else can read its messages. */
  hostOrigin: string;
  /** Whether the frame reports its `console.error` / `console.warn` lines (`anfra.console`). */
  captureConsole?: boolean;
}

export const PROVISION_ELEMENT_ID = 'anfra-provision';

/**
 * An error as it crosses the bridge: the SDK's error in the API's shape, only the fields that shape
 * names (docs/designs/errors/data-apps.md).
 */
export interface BridgeError {
  /** `ValidationError` for a declaration's error; `DataAppError` for any other. */
  name: string;
  code: string;
  scope: string;
  message: string;
  status?: number;
  details?: unknown;
  context?: { step?: string, text: string }[];
  requestId?: string;
}

export type BridgeMessage =
  /** Frame → host: the frame's kernel is running (once per load of its document). */
  | { type: 'anfra:ready' }
  /** A plugin call, `<plugin>.<name>`, answered by an `anfra:result` with the same id. */
  | { type: 'anfra:call', id: number, name: string, request: unknown, stamp?: QueryStamp }
  | { type: 'anfra:result', id: number, ok: true, result: unknown }
  | { type: 'anfra:result', id: number, ok: false, error: BridgeError }
  /** The caller gave up on its call: stop it. Nothing answers. */
  | { type: 'anfra:cancel', id: number }
  /** A bridge event, `<plugin>.<name>`. Nothing answers. */
  | { type: 'anfra:event', name: string, payload: unknown };

/** Frame → host. */
export type FrameMessage = BridgeMessage;

/** Host → frame. */
export type HostMessage = Exclude<BridgeMessage, { type: 'anfra:ready' }>;
