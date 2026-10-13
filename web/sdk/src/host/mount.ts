import type { Provision } from '../common/bridge';
import type {
  Backend, DatasetDescriptor, User,
} from '../common/types';
import { serveBridge, type BridgeCallbacks, type BridgeHandle } from './bridge';
import { builtFrameScript } from './frameScript';
import { builtRenderer } from './rendererScript';
import { provisionDocument } from './provision';

export interface MountOptions extends BridgeCallbacks {
  /** Where the frame goes: it is appended to this element. */
  container: HTMLElement;
  /** The Data App definition: its HTML, as its author wrote it. */
  definition: string;
  /** Where the definition's relative URLs resolve. */
  baseHref: string;
  datasets: Record<string, DatasetDescriptor>;
  user: User;
  /** What answers the frame's queries: `coreApiBackend` from `@holistics/anfra-sdk/api`, on any host. */
  backend: Backend;
  /** The frame script; the one built into this package unless given (tests). */
  frameScript?: string;
  /**
   * Whether the frame may render itself as a PNG or PDF (`Anfra.render()`, `render()` here). Off
   * unless turned on: rendering has the host save a file the frame made, so a host opts in to it
   * (docs/designs/data-apps.md, Renderings).
   */
  render?: boolean;
  /** The renderer's script; the one built into this package unless given (tests). */
  renderer?: string;
  title?: string;
}

export interface MountedDataApp extends Pick<BridgeHandle, 'setInspecting' | 'highlight' | 'locate' | 'setPicking' | 'render'> {
  frame: HTMLIFrameElement;
  /** Stop answering the frame, and remove it. */
  unmount: () => void;
}

/**
 * Mount a Data App: a sandboxed frame (scripts only, an opaque origin, so its own requests reach no
 * API) whose document is the definition with the SDK provisioned ahead of it, answered over the
 * bridge by `backend`. The bridge is served before the document loads, so no call is missed.
 */
export function mountDataApp (options: MountOptions): MountedDataApp {
  const frameScript = options.frameScript ?? builtFrameScript();
  if (!frameScript) throw new Error('@holistics/anfra-sdk/host: built without its frame script; build the package (pnpm build).');

  const doc = options.container.ownerDocument;
  const frame = doc.createElement('iframe');
  frame.setAttribute('sandbox', 'allow-scripts');
  frame.title = options.title ?? 'Data App';
  options.container.appendChild(frame);

  const renderer = options.renderer ?? builtRenderer();
  const render = options.render === true && !!renderer;
  const bridge = serveBridge(
    frame,
    options.backend,
    { onInspect: options.onInspect, onPick: options.onPick },
    { render, renderer },
  );
  const provision: Provision = {
    datasets: options.datasets,
    user: options.user,
    features: { export: typeof options.backend.exportQuery === 'function', render },
    hostOrigin: doc.defaultView?.location.origin ?? '*',
  };
  frame.srcdoc = provisionDocument(options.definition, { baseHref: options.baseHref, provision, frameScript });

  return {
    frame,
    setInspecting: bridge.setInspecting,
    highlight: bridge.highlight,
    locate: bridge.locate,
    setPicking: bridge.setPicking,
    render: bridge.render,
    unmount () {
      bridge.stop();
      frame.remove();
    },
  };
}
