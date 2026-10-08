/**
 * The bridge: the postMessage protocol between a Data App's frame (`app`'s bootstrap) and the
 * page hosting it (`host`). The frame has an opaque origin and no access to the API; every call it
 * makes to its Backend crosses here, and the host answers it.
 */
import type {
  BackendQueryRequest, DatasetDescriptor, FieldSuggestionsRequest, InspectedApp, User,
} from './types';

/** The Backend methods a frame may call: the bridge serves these and nothing else. */
export type BridgeMethod = 'submitQuery' | 'fieldSuggestions';

/** What the host provisions a frame with, as JSON in `<script type="application/json" id="anfra-provision">`. */
export interface Provision {
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** The host page's origin: where the frame posts, so nothing else can read its messages. */
  hostOrigin: string;
}

export const PROVISION_ELEMENT_ID = 'anfra-provision';

/** Frame → host. */
export type FrameMessage =
  | { type: 'anfra:ready' }
  | { type: 'anfra:request', id: number, method: 'submitQuery', request: BackendQueryRequest }
  | { type: 'anfra:request', id: number, method: 'fieldSuggestions', request: FieldSuggestionsRequest }
  | { type: 'anfra:cancel', id: number }
  | { type: 'anfra:inspect', apps: InspectedApp[] };

/** An error as it crosses the bridge: the SDK's error class by name, and its message. */
export interface BridgeError {
  name: string;
  message: string;
}

/** Host → frame. */
export type HostMessage =
  | { type: 'anfra:response', id: number, ok: true, result: unknown }
  | { type: 'anfra:response', id: number, ok: false, error: BridgeError }
  | { type: 'anfra:inspect-watch', open: boolean };
