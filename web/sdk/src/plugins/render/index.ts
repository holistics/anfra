/**
 * `render`: a picture (PNG or PDF) of a Data App or one node of its structure, drawn in the frame
 * (`Anfra.render()`, or the host's `render.draw`) and saved for the reader from the host's page.
 * The renderer is the frame body: frames that never render don't carry it.
 */
import { ValidationError } from '../../common/errors';
import { definePlugin, type MountContext, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/render';
import body from 'anfra-sdk:body/render';
import type { SaveFileRequest } from './types';

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
export async function saveBlob (doc: Document, request: SaveFileRequest): Promise<void> {
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

/** Saves started in each frame, for the limit. */
interface SaveState {
  saving: boolean;
  saves: number[];
}

export interface RenderPluginOptions {
  /** The renderer's script; the one built into this package unless given (tests). */
  renderer?: string;
}

/**
 * Installing it is the host's opt-in to rendering: it has the host save a file the frame made
 * (docs/designs/data-apps.md, Renderings).
 */
export function renderPlugin (options: RenderPluginOptions = {}): Plugin {
  const renderer = options.renderer ?? (body || undefined);
  const states = new WeakMap<MountContext, SaveState>();
  return definePlugin({
    name: 'render',
    version: PLUGIN_VERSION,
    host: {
      mount (mount) {
        states.set(mount, { saving: false, saves: [] });
      },
      calls: {
        /** Save a rendering the frame made: one at a time, and only so many in a window. */
        save: {
          frame: true,
          async handler (request: SaveFileRequest, { mount }) {
            const state = mount && states.get(mount);
            if (!mount || !state) throw new ValidationError('A rendering is saved for a frame.');
            const now = Date.now();
            while (state.saves.length && now - state.saves[0] >= SAVE_WINDOW_MS) state.saves.shift();
            if (state.saving || state.saves.length >= SAVES_PER_WINDOW) {
              throw new ValidationError('Too many files saved at once: try again in a few seconds.');
            }
            state.saving = true;
            state.saves.push(now);
            mount.commandsChanged();
            try {
              await saveBlob(mount.document, request);
            } finally {
              state.saving = false;
              mount.commandsChanged();
            }
            return null;
          },
        },
      },
      // The whole page as a picture, for the host's menus and palette. What a run answers (the
      // rendering, with the images left blank) is the host's to tell the reader.
      commands: (['png', 'pdf'] as const).map((format) => ({
        id: format,
        title: `Download as ${format.toUpperCase()}`,
        icon: 'download',
        placement: 'overflow' as const,
        run: { call: 'render.draw', request: { format } },
        state: ({ mount }: { mount: MountContext }) => (states.get(mount)?.saving ? { disabled: 'Saving a picture…' } : {}),
      })),
    },
    frame: { setup, ...(renderer ? { body: renderer } : {}) },
  });
}
