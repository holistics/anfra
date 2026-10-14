import { TransportError, ValidationError } from '../common/errors';
import type {
  BridgeError, FrameMessage, HostMessage, SaveFileRequest,
} from '../common/bridge';
import type {
  Backend, BackendExportRequest, ExportResult, InspectedApp, InspectedStructure, LocateTarget, Rendering, RenderRequest,
} from '../common/types';

export interface BridgeHandle {
  /** Ask the frame to post (or stop posting) inspection snapshots; survives the frame reloading. */
  setInspecting: (open: boolean) => void;
  /** Show a node of the last snapshot's structure on the page, or nothing (null). */
  highlight: (node: string | null) => void;
  /**
   * Scroll a node, or an entity wherever it is drawn, into view and hold the highlight on it. Fire
   * and forget: the frame answers nothing, and does nothing for what is not on the page.
   */
  locate: (target: LocateTarget) => void;
  /** Start or stop pick mode in the frame: pointing outlines marked elements, a click reports one. */
  setPicking: (on: boolean) => void;
  /**
   * Render the app, or one block or container, once its running queries settle, and save the file
   * for the reader. Rejects with a `ValidationError` when the frame can't, or rendering is off.
   */
  render: (request: RenderRequest) => Promise<Rendering>;
  stop: () => void;
}

export interface BridgeOptions {
  /** Whether the frame may render: off unless true, and off, its asks are refused and `render` rejects. */
  render?: boolean;
  /** The renderer's code (`anfra-sdk:renderer`), sent to the frame on its first rendering. */
  renderer?: string;
}

const RENDERING_OFF = 'This host does not render Data Apps.';

/** What a frame may save: a rendering, told by its first bytes, never by what the frame says it is. */
const PICTURES = [
  { ext: 'png', type: 'image/png', signature: [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a] },
  { ext: 'pdf', type: 'application/pdf', signature: [0x25, 0x50, 0x44, 0x46, 0x2d] },
] as const;

/** Saves a frame may start: one at a time, and this many in this window, so an app can't flood the reader. */
const SAVES_PER_WINDOW = 5;
const SAVE_WINDOW_MS = 10_000;

/**
 * The name a picture is saved under: the frame's, without what a file system or a reader would
 * misread (control characters, path separators), ending in the extension its bytes call for. A
 * name that pretends otherwise (`report.pdf.exe`) keeps its words and gets the real extension.
 */
export function pictureFilename (filename: unknown, ext: string): string {
  const name = (typeof filename === 'string' ? filename : '')
    .replace(/[\u0000-\u001f\u007f-\u009f\u200e\u200f\u202a-\u202e\u2066-\u2069]/g, '')
    .replace(/[\\/:*?"<>|]/g, '-')
    .trim()
    .replace(/^[.\s]+|[.\s]+$/g, '')
    .slice(0, 200) || 'Rendering';
  return name.toLowerCase().endsWith(`.${ext}`) ? name : `${name}.${ext}`;
}

/**
 * Save a file the frame made, from the host's page: the frame has no `allow-downloads`, and this
 * gives back only what a rendering needs. The file must be a PNG or a PDF by its bytes; it is saved
 * as that type, under a name ending in that extension. The object URL is kept a moment, since the
 * browser reads it after the click returns.
 */
async function saveBlob (doc: Document, request: SaveFileRequest): Promise<void> {
  const blob = request?.blob;
  if (!(blob instanceof Blob)) throw new ValidationError('A rendering must be saved as a file.');
  const head = new Uint8Array(await blob.slice(0, 8).arrayBuffer());
  const picture = PICTURES.find(({ signature }) => signature.every((byte, i) => head[i] === byte));
  if (!picture) throw new ValidationError('Only a PNG or a PDF rendering can be saved.');

  const url = URL.createObjectURL(new Blob([blob], { type: picture.type }));
  const a = doc.createElement('a');
  a.href = url;
  a.download = pictureFilename(request.filename, picture.ext);
  a.rel = 'noopener';
  a.style.display = 'none';
  doc.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

export interface BridgeCallbacks {
  /** The frame's inspection snapshot: its apps and its structure. */
  onInspect?: (apps: InspectedApp[], structure: InspectedStructure) => void;
  /** In pick mode, the reader chose a node (its key), or left the mode (null). */
  onPick?: (node: string | null) => void;
}

function toBridgeError (err: unknown): BridgeError {
  const e = err as Error | undefined;
  return { name: e?.name ?? 'QueryError', message: e?.message ?? 'The query failed.' };
}

/**
 * Save an export's file for the reader, from the host's page: a frame without `allow-downloads`
 * cannot, and following a link is how a browser writes a file to disk without holding it. A link
 * to another origin (a presigned URL) ignores `download`; its server's `Content-Disposition` names
 * the file instead.
 */
function saveFile (doc: Document, link: ExportResult): void {
  const a = doc.createElement('a');
  a.href = link.url;
  a.download = link.filename;
  a.rel = 'noopener';
  a.style.display = 'none';
  doc.body.appendChild(a);
  a.click();
  a.remove();
}

/** Make the export, then start saving it, then answer the frame with its link. */
async function exportAndSave (
  backend: Backend,
  doc: Document,
  request: BackendExportRequest,
  signal: AbortSignal,
): Promise<ExportResult> {
  if (!backend.exportQuery) throw new TransportError('This host cannot make exports.');
  const link = await backend.exportQuery(request, signal);
  saveFile(doc, link);
  return link;
}

/**
 * The bridge, the host's side: answer a Data App frame's Backend calls with `backend`, abort one on
 * the frame's cancel, and relay its inspection snapshots, until `stop`. Only messages from this
 * frame are served (a sandboxed frame has an opaque origin, so its window is what identifies it),
 * and only the Backend's own methods: nothing else of the API is reachable from a frame.
 */
export function serveBridge (
  frame: HTMLIFrameElement,
  backend: Backend,
  callbacks: BridgeCallbacks = {},
  options: BridgeOptions = {},
): BridgeHandle {
  const inFlight = new Map<number, AbortController>();
  const renderings = new Map<number, { resolve: (r: Rendering) => void, reject: (e: Error) => void }>();
  let nextRender = 1;
  let saving = false;
  const saves: number[] = [];
  const rendering = options.render === true && !!options.renderer;
  let inspecting = false;
  const win = frame.ownerDocument.defaultView ?? window;

  // An opaque-origin frame can only be addressed with '*'; its window identifies it.
  const reply = (message: HostMessage) => frame.contentWindow?.postMessage(message, '*');
  const sendWatch = () => reply({ type: 'anfra:inspect-watch', open: inspecting });

  /** Save a rendering the frame made: one at a time, and only so many in a window. */
  const save = async (request: SaveFileRequest): Promise<null> => {
    const now = Date.now();
    while (saves.length && now - saves[0] >= SAVE_WINDOW_MS) saves.shift();
    if (saving || saves.length >= SAVES_PER_WINDOW) {
      throw new ValidationError('Too many files saved at once: try again in a few seconds.');
    }
    saving = true;
    saves.push(now);
    try {
      await saveBlob(frame.ownerDocument, request);
    } finally {
      saving = false;
    }
    return null;
  };

  /** Answer one of the frame's asks: a Backend method, or one of the two a rendering needs. */
  const answer = async (message: Extract<FrameMessage, { type: 'anfra:request' }>, signal: AbortSignal): Promise<unknown> => {
    switch (message.method) {
      case 'submitQuery':
        return backend.submitQuery(message.request, signal);
      case 'fieldSuggestions':
        return backend.fieldSuggestions(message.request, signal);
      case 'exportQuery':
        return exportAndSave(backend, frame.ownerDocument, message.request, signal);
      case 'lineage':
        if (!backend.lineage) throw new TransportError('This host cannot trace lineage.');
        return backend.lineage(message.request, signal);
      case 'renderer':
        if (!rendering) throw new ValidationError(RENDERING_OFF);
        return options.renderer;
      case 'saveFile':
        if (!rendering) throw new ValidationError(RENDERING_OFF);
        return save(message.request);
      default:
        throw new Error(`No such Backend method: ${String((message as { method: unknown }).method)}`);
    }
  };

  const onMessage = (event: MessageEvent) => {
    if (event.source !== frame.contentWindow) return;
    const message = event.data as FrameMessage | undefined;
    switch (message?.type) {
      case 'anfra:ready':
        if (inspecting) sendWatch();
        return;
      case 'anfra:inspect':
        if (inspecting && Array.isArray(message.apps)) {
          callbacks.onInspect?.(message.apps, message.structure ?? { nodes: [], usage: {} });
        }
        return;
      case 'anfra:picked':
        if (inspecting) callbacks.onPick?.(message.node);
        return;
      case 'anfra:rendered': {
        const pending = renderings.get(message.id);
        if (!pending) return;
        renderings.delete(message.id);
        if (message.ok) pending.resolve(message.result);
        else pending.reject(new ValidationError(message.error.message));
        return;
      }
      case 'anfra:cancel':
        inFlight.get(message.id)?.abort();
        inFlight.delete(message.id);
        return;
      case 'anfra:request': {
        const { id } = message;
        const controller = new AbortController();
        inFlight.set(id, controller);
        answer(message, controller.signal)
          .then((value) => reply({ type: 'anfra:response', id, ok: true, result: value }))
          .catch((err: unknown) => {
            // A cancelled call already rejected in the frame; there is no one left to answer.
            if ((err as Error | undefined)?.name === 'AbortError') return;
            reply({ type: 'anfra:response', id, ok: false, error: toBridgeError(err) });
          })
          .finally(() => inFlight.delete(id));
        return;
      }
      default:
    }
  };

  win.addEventListener('message', onMessage);
  return {
    setInspecting (open) {
      inspecting = open;
      sendWatch();
    },
    highlight (node) {
      reply({ type: 'anfra:highlight', node });
    },
    locate (target) {
      reply({ type: 'anfra:locate', target });
    },
    setPicking (on) {
      reply({ type: 'anfra:pick-watch', on });
    },
    render (request) {
      if (!rendering) return Promise.reject(new ValidationError(RENDERING_OFF));
      const id = nextRender++;
      return new Promise<Rendering>((resolve, reject) => {
        renderings.set(id, { resolve, reject });
        reply({ type: 'anfra:render', id, request });
      });
    },
    stop () {
      win.removeEventListener('message', onMessage);
      inFlight.forEach((controller) => controller.abort());
      inFlight.clear();
      renderings.forEach(({ reject }) => reject(new ValidationError('The Data App was unmounted before its rendering finished.')));
      renderings.clear();
    },
  };
}
