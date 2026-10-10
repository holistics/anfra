import type { Provision } from '../common/bridge';
import type {
  Backend, DatasetDescriptor, User,
} from '../common/types';
import { serveBridge, type BridgeCallbacks, type BridgeHandle } from './bridge';
import { builtFrameScript } from './frameScript';
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
  title?: string;
}

export interface MountedDataApp extends Pick<BridgeHandle, 'setInspecting' | 'highlight' | 'locate' | 'setPicking'> {
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

  const bridge = serveBridge(frame, options.backend, { onInspect: options.onInspect, onPick: options.onPick });
  const provision: Provision = {
    datasets: options.datasets,
    user: options.user,
    hostOrigin: doc.defaultView?.location.origin ?? '*',
  };
  frame.srcdoc = provisionDocument(options.definition, { baseHref: options.baseHref, provision, frameScript });

  return {
    frame,
    setInspecting: bridge.setInspecting,
    highlight: bridge.highlight,
    locate: bridge.locate,
    setPicking: bridge.setPicking,
    unmount () {
      bridge.stop();
      frame.remove();
    },
  };
}
