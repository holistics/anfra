/**
 * The plugin layer: where whatever the SDK draws on a Data App's page goes (the overlay, a plugin's
 * modal), above the definition's own page and apart from it. One element at the end of the
 * document, holding one shadow root per part (the kernel's, each plugin's), so the page's styles
 * don't reach in and theirs don't reach out. A rendering never draws it.
 */
import { applyTheme, type Theme } from '../common/theme';

export const LAYER_ATTRIBUTE = 'data-anfra-layer';

/** The host's theme, per document, for the layer whenever it is made. */
const themes = new WeakMap<Document, Theme>();

const LAYER_STYLE = 'position:absolute;top:0;left:0;width:0;height:0;overflow:visible;z-index:2147483647;';

/** The layer's element, made on first use. */
function layerOf (doc: Document): HTMLElement {
  const existing = doc.querySelector(`[${LAYER_ATTRIBUTE}]`);
  if (existing instanceof (doc.defaultView?.HTMLElement ?? HTMLElement)) return existing as HTMLElement;
  const layer = doc.createElement('div');
  layer.setAttribute(LAYER_ATTRIBUTE, '');
  layer.setAttribute('style', LAYER_STYLE);
  const theme = themes.get(doc);
  if (theme) applyTheme(layer.style, theme);
  (doc.body ?? doc.documentElement).appendChild(layer);
  return layer;
}

/** The shadow root `part` draws in (`anfra` for the kernel's overlay, a plugin's name for its own). */
export function layerRoot (doc: Document, part: string): ShadowRoot {
  const layer = layerOf(doc);
  const host = [...layer.children].find((child) => child.getAttribute('data-part') === part);
  if (host?.shadowRoot) return host.shadowRoot;
  const element = doc.createElement('div');
  element.setAttribute('data-part', part);
  layer.appendChild(element);
  return element.attachShadow({ mode: 'open' });
}

/**
 * Give the layer the host's theme: its tokens as custom properties, which every part's shadow root
 * inherits. Applied now if the layer exists, else when it is made, so a theme draws no layer.
 */
export function themeLayer (doc: Document, theme: Theme): void {
  themes.set(doc, theme);
  const existing = doc.querySelector(`[${LAYER_ATTRIBUTE}]`);
  if (existing) applyTheme((existing as HTMLElement).style, theme);
}

/** Whether `element` is the layer, which a rendering leaves out. */
export function isLayer (element: Element): boolean {
  return element.hasAttribute(LAYER_ATTRIBUTE);
}
