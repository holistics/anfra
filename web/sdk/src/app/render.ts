/**
 * Renderings, the frame's side: which element a request names, the file's name, the renderer's
 * code (asked of the host once, then injected), and saving through the host, since the frame can't
 * download. Drawing is the renderer's (`src/render`), which the frame script does not carry.
 */
import { ValidationError } from '../common/errors';
import type { SaveFileRequest } from '../common/bridge';
import type { Rendering, RenderRequest } from '../common/types';
import { RENDERER_GLOBAL, type Renderer } from '../render/types';
import { OVERLAY_ID } from './overlay';
import { ATTR } from './structure';
import type { App } from './app';

export const SKIP_ATTRIBUTE = 'data-anfra-render';

const EXTENSIONS: Record<RenderRequest['format'], string> = { png: 'png', pdf: 'pdf' };

/** How long a host's rendering waits for running queries to settle before drawing anyway. */
export const SETTLE_MS = 10_000;

export interface RenderDeps {
  /** Asks the host: the renderer's code, or to save a file. */
  ask: {
    renderer: () => Promise<string>;
    saveFile: (request: SaveFileRequest) => Promise<unknown>;
  };
  apps: () => readonly App[];
}

/** A file name the host can save: the author's, or one made from the app and the node. */
function filenameFor (request: RenderRequest, doc: Document, apps: readonly App[], element: Element | null): string {
  const ext = EXTENSIONS[request.format];
  let name = request.filename?.trim();
  if (!name) {
    const title = apps.find((app) => app.title)?.title || doc.title.trim() || 'Data App';
    const node = element && (element.getAttribute(ATTR.label) || request.block || request.container);
    name = node ? `${title} - ${node}` : title;
  }
  // A download names one file: no path, nothing unprintable.
  name = name.replace(/[/\\]/g, '-').replace(/\p{Cc}/gu, '').trim() || 'Data App';
  return name.toLowerCase().endsWith(`.${ext}`) ? name : `${name}.${ext}`;
}

/** The element a request names, or null for the whole app; refuses what names nothing. */
function targetOf (request: RenderRequest, doc: Document): { element: Element | null, entity?: string } {
  if (request.block !== undefined && request.container !== undefined) {
    throw new ValidationError('Render a block or a container, not both.', request.block);
  }
  if (request.format !== 'png' && request.format !== 'pdf') {
    throw new ValidationError(`Can't render as '${String(request.format)}': the formats are 'png' and 'pdf'.`);
  }
  const { pixelRatio } = request;
  if (pixelRatio !== undefined && !(Number.isFinite(pixelRatio) && pixelRatio > 0)) {
    throw new ValidationError(`pixelRatio ${String(pixelRatio)} is not a positive number.`);
  }
  const kind = request.block !== undefined ? 'block' : request.container !== undefined ? 'container' : undefined;
  if (!kind) return { element: null };
  const id = (request.block ?? request.container) as string;
  // Compared, not spliced into a selector: an id is the author's free text.
  const element = [...doc.querySelectorAll(`[${ATTR[kind]}]`)].find((el) => el.getAttribute(ATTR[kind]) === id);
  if (!element) throw new ValidationError(`No element is marked ${ATTR[kind]}="${id}" on the page.`, id);
  return { element, entity: id };
}

/** Never drawn: the SDK's overlay, and what the author marked to skip. */
function skip (element: Element): boolean {
  return element.id === OVERLAY_ID || element.getAttribute(SKIP_ATTRIBUTE) === 'skip';
}

/**
 * `Anfra.render()`: draw the request's element, or the whole app, and have the host save it. Runs at
 * once; `settle` is the host's way in, which waits for the page first.
 */
export function createRender (win: Window, deps: RenderDeps): (request: RenderRequest) => Promise<Rendering> {
  let loading: Promise<Renderer> | undefined;
  const load = (): Promise<Renderer> => {
    const ready = (win as unknown as Record<string, Renderer | undefined>)[RENDERER_GLOBAL];
    if (ready) return Promise.resolve(ready);
    loading ??= deps.ask.renderer().then((code) => {
      const script = win.document.createElement('script');
      script.textContent = code;
      (win.document.head ?? win.document.documentElement).appendChild(script);
      script.remove();
      const installed = (win as unknown as Record<string, Renderer | undefined>)[RENDERER_GLOBAL];
      if (!installed) throw new ValidationError('The host sent a renderer that did not install.');
      return installed;
    }).catch((err: unknown) => {
      loading = undefined;
      throw err;
    });
    return loading;
  };

  return async (request) => {
    const doc = win.document;
    const { element, entity } = targetOf(request ?? {} as RenderRequest, doc);
    const filename = filenameFor(request, doc, deps.apps(), element);
    try {
      const renderer = await load();
      const drawing = await renderer.draw(element, { format: request.format, pixelRatio: request.pixelRatio, skip });
      await deps.ask.saveFile({ blob: drawing.blob, filename });
      return { filename, format: request.format, ...drawing };
    } catch (err) {
      if (err instanceof ValidationError && err.entity) throw err;
      throw new ValidationError((err as Error)?.message || 'The rendering failed.', entity);
    }
  };
}

/** Until no query of any app is running, or `SETTLE_MS` has passed; then one frame, to paint. */
export async function settle (win: Window, apps: () => readonly App[], timeout = SETTLE_MS): Promise<void> {
  const busy = () => apps().some((app) => Object.values(app.queries).some((query) => query.state === 'executing'));
  const deadline = Date.now() + timeout;
  while (busy() && Date.now() < deadline) {
    await new Promise((resolve) => { win.setTimeout(resolve, 100); });
  }
  await new Promise((resolve) => { win.requestAnimationFrame(() => resolve(undefined)); });
}
