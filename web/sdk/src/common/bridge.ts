/**
 * The bridge: the postMessage protocol between a Data App's frame (`app`'s bootstrap) and the
 * page hosting it (`host`). The frame has an opaque origin and no access to the API; every call it
 * makes to its Backend crosses here, and the host answers it.
 */
import type {
  BackendExportRequest, BackendLineageRequest, BackendQueryRequest, DatasetDescriptor, FieldSuggestionsRequest, InspectedApp,
  InspectedStructure, LocateTarget, Rendering, RenderRequest, SdkFeatures, User,
} from './types';

/**
 * What a frame may ask its host: the Backend's methods, and the two a rendering needs (the
 * renderer's code, and saving a file). The bridge serves these and nothing else.
 */
export type BridgeMethod = 'submitQuery' | 'fieldSuggestions' | 'exportQuery' | 'lineage' | 'renderer' | 'saveFile';

/** A file the frame made, for the host to save for the reader: the frame cannot download. */
export interface SaveFileRequest {
  blob: Blob;
  filename: string;
}

/** What the host provisions a frame with, as JSON in `<script type="application/json" id="anfra-provision">`. */
export interface Provision {
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** What the host can do beyond queries: exports, rendering. Absent from a host older than this field. */
  features?: SdkFeatures;
  /** The host page's origin: where the frame posts, so nothing else can read its messages. */
  hostOrigin: string;
}

export const PROVISION_ELEMENT_ID = 'anfra-provision';

/** Frame → host. */
export type FrameMessage =
  | { type: 'anfra:ready' }
  | { type: 'anfra:request', id: number, method: 'submitQuery', request: BackendQueryRequest }
  | { type: 'anfra:request', id: number, method: 'fieldSuggestions', request: FieldSuggestionsRequest }
  | { type: 'anfra:request', id: number, method: 'lineage', request: BackendLineageRequest }
  /** The host makes the file, starts saving it for the reader, then answers its link. */
  | { type: 'anfra:request', id: number, method: 'exportQuery', request: BackendExportRequest }
  /** The renderer's code, asked for on the frame's first rendering; answered as a string. */
  | { type: 'anfra:request', id: number, method: 'renderer', request: null }
  | { type: 'anfra:request', id: number, method: 'saveFile', request: SaveFileRequest }
  /** The answer to the host's `anfra:render`, by its id. */
  | { type: 'anfra:rendered', id: number, ok: true, result: Rendering }
  | { type: 'anfra:rendered', id: number, ok: false, error: BridgeError }
  | { type: 'anfra:cancel', id: number }
  | { type: 'anfra:inspect', apps: InspectedApp[], structure: InspectedStructure }
  /** In pick mode, the reader chose a node, or left the mode (null, on Escape). */
  | { type: 'anfra:picked', node: string | null };

/** An error as it crosses the bridge: the SDK's error class by name, and its message. */
export interface BridgeError {
  name: string;
  message: string;
}

/** Host → frame. */
export type HostMessage =
  | { type: 'anfra:response', id: number, ok: true, result: unknown }
  | { type: 'anfra:response', id: number, ok: false, error: BridgeError }
  | { type: 'anfra:inspect-watch', open: boolean }
  /** Show a node on the page (its key in the last snapshot), or nothing. The frame answers nothing. */
  | { type: 'anfra:highlight', node: string | null }
  /** Scroll a node or an entity into view and hold the highlight on it. */
  | { type: 'anfra:locate', target: LocateTarget }
  /** Start or stop pick mode. */
  | { type: 'anfra:pick-watch', on: boolean }
  /** Render the app or one node, once its queries settle, and save it; the frame answers `anfra:rendered`. */
  | { type: 'anfra:render', id: number, request: RenderRequest };
