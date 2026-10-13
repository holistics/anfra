/**
 * The renderer: draws part of a Data App's page, or all of it, onto a canvas, and makes a PNG or a
 * PDF of it. It runs in the frame, injected by the frame on its first rendering from code its host
 * carries (`anfra-sdk:renderer`), so frames that never render don't carry it.
 *
 * DOM to canvas, by html-to-image: the page is cloned with its computed styles into an SVG
 * `foreignObject`, which the browser draws. What it cannot read is lost, so what would be lost
 * silently is found first and reported: an image without CORS, left blank. A web font from a
 * stylesheet on another origin (Google Fonts) cannot be read in place either, so its faces are
 * fetched and embedded here.
 */
import { toCanvas } from 'html-to-image';
import { pdfOfJpeg } from './pdf';
import {
  RenderFailure, type DrawOptions, type Drawing, type Renderer,
} from './types';

/** A canvas side no browser refuses, and an area within all of theirs. */
const MAX_SIDE = 16384;
const MAX_AREA = 120_000_000;

/** The device pixels per CSS pixel to draw at: the author's, or the default lowered to fit. */
function ratioFor (width: number, height: number, asked: number | undefined): number {
  const fits = (r: number) => width * r <= MAX_SIDE && height * r <= MAX_SIDE && width * height * r * r <= MAX_AREA;
  const tooBig = (r: number) => new RenderFailure(
    `${width}×${height} px at pixelRatio ${r} is over a canvas's limit (${MAX_SIDE} px a side): render a smaller node, or a lower pixelRatio.`,
  );
  if (asked !== undefined) {
    if (!fits(asked)) throw tooBig(asked);
    return asked;
  }
  const best = Math.max(globalThis.devicePixelRatio || 1, 2);
  if (fits(best)) return best;
  const fitted = Math.floor(Math.min(MAX_SIDE / width, MAX_SIDE / height, Math.sqrt(MAX_AREA / (width * height))) * 100) / 100;
  if (fitted < 1) throw tooBig(1);
  return fitted;
}

function opaque (color: string): boolean {
  return !!color && color !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(color);
}

/**
 * The colour behind what is drawn: a node's own or its nearest ancestor's; for the whole app, the
 * page's (`<html>`'s, else `<body>`'s, which the browser paints across the page); else white.
 */
function backgroundOf (target: Element | null, doc: Document): string {
  const chain: Element[] = [];
  if (target) {
    for (let el: Element | null = target; el; el = el.parentElement) chain.push(el);
    chain.push(doc.body);
  } else {
    chain.push(doc.documentElement, doc.body);
  }
  return chain.map((el) => getComputedStyle(el).backgroundColor).find(opaque) ?? '#ffffff';
}

/** Every element `root` draws, skipped ones and their insides left out. */
function drawnElements (root: Element, skip: (element: Element) => boolean): Element[] {
  const out: Element[] = [];
  const walk = (el: Element) => {
    if (skip(el)) return;
    out.push(el);
    for (const child of el.children) walk(child);
  };
  walk(root);
  return out;
}

/**
 * The images the clone can't read, by URL. html-to-image fetches each image to inline it; one whose
 * server sends no CORS headers it draws as `imagePlaceholder`, with only a console warning, so
 * they are found here to be reported.
 */
async function unreadableImages (elements: Element[]): Promise<string[]> {
  const urls = new Set<string>();
  for (const el of elements) {
    if (el instanceof HTMLImageElement) {
      const url = el.currentSrc || el.src;
      if (url && !/^(data|blob):/.test(url)) urls.add(url);
    }
  }
  const readable = await Promise.all([...urls].map(async (url) => {
    try {
      const response = await fetch(url, { mode: 'cors', cache: 'force-cache' });
      return response.ok;
    } catch {
      return false;
    }
  }));
  return [...urls].filter((_, i) => !readable[i]);
}

/** What an image that can't be read is drawn as: a transparent pixel, its box kept, so nothing moves. */
const BLANK = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';

function dataUrlOf (blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

/** The font families `elements` are drawn in, lowercased, unquoted. */
function familiesOf (elements: Element[]): Set<string> {
  const families = new Set<string>();
  for (const el of elements) {
    for (const family of getComputedStyle(el).fontFamily.split(',')) {
      families.add(family.trim().replace(/^["']|["']$/g, '').toLowerCase());
    }
  }
  return families;
}

/**
 * The `@font-face` rules of the fonts `elements` use, their files inlined as data URLs. A
 * stylesheet on another origin can't be read in place, so it is fetched (a font service allows
 * CORS); a face that can't be fetched is left out, and the text falls back to another font.
 */
async function embedFonts (doc: Document, elements: Element[]): Promise<string> {
  const used = familiesOf(elements);
  const faces: { css: string, base: string }[] = [];
  await Promise.all([...doc.styleSheets].map(async (sheet) => {
    const base = sheet.href ?? doc.baseURI;
    try {
      for (const rule of sheet.cssRules) {
        if (rule instanceof CSSFontFaceRule) faces.push({ css: rule.cssText, base });
      }
    } catch {
      if (!sheet.href) return;
      try {
        const text = await (await fetch(sheet.href, { mode: 'cors', cache: 'force-cache' })).text();
        for (const css of text.match(/@font-face\s*{[^}]*}/g) ?? []) faces.push({ css, base });
      } catch { /* a stylesheet we can't fetch: its fonts fall back */ }
    }
  }));

  const embedded = await Promise.all(faces.map(async ({ css, base }) => {
    const family = /font-family:\s*["']?([^;"']+)/.exec(css)?.[1]?.trim().toLowerCase();
    if (!family || !used.has(family)) return '';
    let out = css;
    for (const [whole, , url] of css.matchAll(/url\((["']?)([^"')]+)\1\)/g)) {
      if (url.startsWith('data:')) continue;
      try {
        const response = await fetch(new URL(url, base).href, { mode: 'cors', cache: 'force-cache' });
        out = out.replace(whole, `url(${await dataUrlOf(await response.blob())})`);
      } catch {
        return '';
      }
    }
    return out;
  }));
  return embedded.filter(Boolean).join('\n');
}

function toBlob (canvas: HTMLCanvasElement, type: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    try {
      canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new RenderFailure('The browser made no image of the canvas.'))), type, quality);
    } catch {
      // A canvas drawn from something read without CORS (an author's own canvas, a font) is tainted.
      reject(new RenderFailure(
        'The page holds something read from another server without CORS (a canvas or a font), so the browser refuses to make an image of it. '
        + 'Load it with CORS, or mark it data-anfra-render="skip".',
      ));
    }
  });
}

async function draw (target: Element | null, options: DrawOptions): Promise<Drawing> {
  const doc = document;
  // The whole app is the body, at the document's full scroll size.
  const root = target ?? doc.body;
  const rect = root.getBoundingClientRect();
  const width = Math.ceil(target ? rect.width : doc.documentElement.scrollWidth);
  const height = Math.ceil(target ? rect.height : doc.documentElement.scrollHeight);
  if (!width || !height) throw new RenderFailure('There is nothing to draw: it has no size (hidden, or empty).');
  const pixelRatio = ratioFor(width, height, options.pixelRatio);

  const elements = drawnElements(root, options.skip);
  const omitted = await unreadableImages(elements);
  const fontEmbedCSS = await embedFonts(doc, elements);

  const canvas = await toCanvas(root as HTMLElement, {
    width,
    height,
    pixelRatio,
    skipAutoScale: true,
    backgroundColor: backgroundOf(target, doc),
    fontEmbedCSS,
    imagePlaceholder: BLANK,
    filter: (node) => !(node instanceof Element && options.skip(node)),
    // The clone's root is given the size it is drawn at, so it must count its padding and border
    // in it, or it overflows the picture. A node drops its margin, which is outside it; the body
    // keeps its own, so its content sits where it does on the page, at the body's own width.
    style: target
      ? { boxSizing: 'border-box', margin: '0' }
      : { boxSizing: 'border-box', width: `${rect.width}px`, height: 'auto' },
  });

  if (options.format === 'png') return { blob: await toBlob(canvas, 'image/png'), width, height, omitted };
  const jpeg = new Uint8Array(await (await toBlob(canvas, 'image/jpeg', 0.92)).arrayBuffer());
  return { blob: pdfOfJpeg(jpeg, canvas, { width, height }), width, height, omitted };
}

export const renderer: Renderer = { draw };
