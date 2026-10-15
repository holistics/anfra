/**
 * The contract between a frame and the renderer: `render`'s frame body (`anfra-sdk:body/render`),
 * which the frame runs on its first rendering.
 */
import type { RenderFormat } from '../common/types';

export interface DrawOptions {
  format: RenderFormat;
  /** The author's, or undefined for the default: the screen's, at least 2, lowered to fit a canvas. */
  pixelRatio?: number;
  /** Elements never drawn (the SDK's overlay, `data-anfra-render="skip"`), by predicate. */
  skip: (element: Element) => boolean;
}

export interface Drawing {
  blob: Blob;
  /** In CSS pixels. */
  width: number;
  height: number;
  /** Images left blank, since their servers send no CORS headers, by URL. */
  omitted: string[];
}

export interface Renderer {
  /** Draw `target`, or the whole document when null. Rejects with a `RenderFailure` an author can act on. */
  draw (target: Element | null, options: DrawOptions): Promise<Drawing>;
}

/** Why a rendering failed, in words an author can act on. The frame makes it a `ValidationError`. */
export class RenderFailure extends Error {
  constructor (message: string) {
    super(message);
    this.name = 'RenderFailure';
  }
}
