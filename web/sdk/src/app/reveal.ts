/**
 * The definition's way to open the host's panel at a query or a control (`query.inspect()`,
 * `control.inspect()`): the kernel's `anfra.reveal` call, which the host answers with whether the
 * panel opened. Only on a reader's click, so an app can't open the panel on load or on each render
 * by mistake. That guards against slips and enforces nothing: the host's middleware decides.
 */
import { ValidationError } from '../common/errors';
import type { FrameKernel } from './kernel';
import { reportError } from './reporting';

/** What the frame asks the host to reveal. */
export interface RevealRequest {
  app: number;
  entity: 'query' | 'control';
  name: string;
  tab?: string;
}

const KIND = { query: 'Query', control: 'Control' } as const;

/** Whether the reader just interacted with the frame; true where the browser can't tell. */
function activated (win: Window): boolean {
  const activation = (win.navigator as Navigator & { userActivation?: { isActive: boolean } }).userActivation;
  return activation ? activation.isActive : true;
}

/** Give each app the frame's reveal; `kernel` is the kernel's own, named `anfra`. */
export function installReveal (kernel: FrameKernel): void {
  const reveal = async (request: RevealRequest): Promise<boolean> => {
    if (!activated(kernel.window)) {
      reportError('call.inspect', new ValidationError(
        `${KIND[request.entity]} '${request.name}' can't open the panel without a reader's click. Call inspect() from a click handler, such as a button's.`,
        request.name,
      ));
      return false;
    }
    try {
      return (await kernel.call<unknown>('anfra.reveal', request)) === true;
    } catch (err) {
      reportError('call.inspect', err);
      return false;
    }
  };
  kernel.onApp((app) => { app.revealer = reveal; });
}
