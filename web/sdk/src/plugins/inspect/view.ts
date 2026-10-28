/**
 * What `inspect`'s tabs draw with: a small element builder, the icons, the widgets every tab
 * shares, and the stylesheet, all on the host's `--anfra-*` tokens. Plain DOM, so the tabs carry
 * no framework into a host that has its own.
 */
import type { ColumnMeta, InspectedQuery } from '../../common/types';
import type { TabContext } from '../../host/contributions';
import type { Panel } from '../../host/panel';
import { LINEAGE_TAB } from './model/types';
import type { QueryInput } from './model/inputs';
import { tokenize, type Lang } from './model/highlight';
import { plural } from './model/format';
import { LEVELS, TRUST_LEVELS, type Trust } from './model/trust';

export type Child = Node | string | false | null | undefined;
export type Children = Child | readonly Children[];
type Attributes = Record<string, string | number | boolean | undefined | ((event: Event) => void)>;

/** An element: `on*` attributes are listeners, `true` sets a bare attribute, false or undefined none. */
export function h (doc: Document, tag: string, attributes: Attributes = {}, ...children: Children[]): HTMLElement {
  const element = doc.createElement(tag);
  for (const [name, value] of Object.entries(attributes)) {
    if (typeof value === 'function') element.addEventListener(name.slice(2).toLowerCase(), value);
    else if (value === true) element.setAttribute(name, '');
    else if (value !== false && value !== undefined) element.setAttribute(name, String(value));
  }
  for (const child of (children as unknown[]).flat(Infinity) as Child[]) {
    if (child === false || child === null || child === undefined) continue;
    element.append(typeof child === 'string' ? doc.createTextNode(child) : child);
  }
  return element;
}

/** Replace an element's children, with `h`'s children: false, null and undefined left out. */
export function replace (doc: Document, element: Element, ...children: Children[]): void {
  element.replaceChildren(...h(doc, 'div', {}, ...children).childNodes);
}

// Hand-drawn 24px stroke icons, as appserve draws them.
const PATHS = {
  close: 'M6 6l12 12 M18 6L6 18',
  copy: 'M9 9h11v11H9z M5 15H4V4h11v1',
  check: 'M5 12l5 5 9-10',
  table: 'M4 5h16v14H4z M4 10h16 M4 15h16 M10 10v9',
  download: 'M12 4v11 M7 10l5 5 5-5 M5 20h14',
  pick: 'M4 4l7 16 2-7 7-2z',
  chevron: 'M9 6l6 6-6 6',
  filter: 'M4 6h16 M7 12h10 M10 18h4',
  info: 'M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18z M12 11v6 M12 7.5v.5',
  locate: 'M12 3v4 M12 17v4 M3 12h4 M17 12h4 M12 8a4 4 0 1 0 0 8a4 4 0 1 0 0-8z',
  query: 'M5 6c0-1.7 3.1-3 7-3s7 1.3 7 3-3.1 3-7 3-7-1.3-7-3z M5 6v12c0 1.7 3.1 3 7 3s7-1.3 7-3V6 M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3',
  tree: 'M5 4v16 M5 8h6 M5 16h6 M13 6h6v4h-6z M13 14h6v4h-6z',
  search: 'M11 4a7 7 0 1 0 0 14a7 7 0 1 0 0-14z M16 16l4 4',
  // Lineage: a card per kind, and the canvas's tools.
  lineage: 'M9 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M3 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M15 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M18 9v2c0 .6-.4 1-1 1H7c-.6 0-1-.4-1-1V9 M12 12v3',
  control: 'M4 6h9 M17 6h3 M15 4v4 M4 12h3 M11 12h9 M9 10v4 M4 18h11 M19 18h1 M17 16v4',
  block: 'M4 4h7v7H4z M13 4h7v7h-7z M4 13h7v7H4z M13 13h7v7h-7z',
  metric: 'M18 5H6l6 7-6 7h12',
  dimension: 'M3 17L17 3l4 4L7 21z M7 13l2 2 M10 10l2 2 M13 7l2 2',
  dataset: 'M4 8l8-4 8 4-8 4z M4 12l8 4 8-4 M4 16l8 4 8-4',
  model: 'M4 7l8-4 8 4v10l-8 4-8-4z M4 7l8 4 8-4 M12 11v10',
  focus: 'M4 8V4h4 M16 4h4v4 M20 16v4h-4 M8 20H4v-4 M12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6z',
  expand: 'M14 4h6v6 M10 20H4v-6 M20 4l-7 7 M4 20l7-7',
  help: 'M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18z M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6v.6 M12 17v.5',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z M12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6z',
  hidden: 'M3 3l18 18 M10.6 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3 3.9 M6.6 6.6A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 5.4-1.6 M9.9 9.9a3 3 0 0 0 4.2 4.2',
  plus: 'M12 5v14 M5 12h14',
  minus: 'M5 12h14',
  fit: 'M4 9V4h5 M15 4h5v5 M20 15v5h-5 M9 20H4v-5',
  code: 'M16 18l6-6-6-6 M8 6l-6 6 6 6',
} as const;

export type IconName = keyof typeof PATHS;

export function icon (doc: Document, name: IconName): SVGElement {
  const svg = doc.createElementNS('http://www.w3.org/2000/svg', 'svg');
  for (const [key, value] of Object.entries({
    class: 'icon', viewBox: '0 0 24 24', width: '14', height: '14', fill: 'none', stroke: 'currentColor',
    'stroke-width': '1.8', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true',
  })) svg.setAttribute(key, value);
  const path = doc.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', PATHS[name]);
  svg.append(path);
  return svg;
}

// A column's icon: Σ for a metric, its semantic type's for a dimension. Drawn as the kit's icons are.
export const TYPE_PATHS: Record<string, string> = {
  metric: 'M17 5H7l6 7-6 7h10',
  text: 'M5 7V5h14v2 M12 5v14 M9 19h6',
  number: 'M5 9h14 M5 15h14 M10 4L8 20 M16 4l-2 16',
  date: 'M4 6h16v14H4z M4 10h16 M8 3v4 M16 3v4',
  datetime: 'M4 6h16v14H4z M4 10h16 M8 3v4 M16 3v4 M12 13v3h3',
  truefalse: 'M8 7h8a5 5 0 0 1 0 10H8A5 5 0 0 1 8 7z M8 10a2 2 0 1 0 0 4a2 2 0 1 0 0-4z',
  bit: 'M6 5v6 M12 5h4v6h-4z M6 13h4v6H6z M16 13v6',
  json: 'M9 4c-2 0-3 1-3 3v2c0 1.5-1 3-2 3 1 0 2 1.5 2 3v2c0 2 1 3 3 3 M15 4c2 0 3 1 3 3v2c0 1.5 1 3 2 3-1 0-2 1.5-2 3v2c0 2-1 3-3 3',
  unknown: 'M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18z M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.7.3-1 .9-1 1.7 M12 17v.5',
};

export function columnIcon (doc: Document, column: Pick<ColumnMeta, 'isMeasure' | 'semantic_type'>): SVGElement {
  const svg = icon(doc, 'info');
  svg.querySelector('path')!.setAttribute('d', column.isMeasure ? TYPE_PATHS.metric! : TYPE_PATHS[column.semantic_type] ?? TYPE_PATHS.unknown!);
  svg.setAttribute('width', '12');
  svg.setAttribute('height', '12');
  return svg;
}

/** Copy text from the host page; whether it worked. */
export async function copyText (doc: Document, text: string): Promise<boolean> {
  try {
    await doc.defaultView?.navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/**
 * Draw a tab: the stylesheet, a `head` kept as it is (a search box keeps its focus and text), and
 * `draw`'s body again on each change to the panel or to what `also` watches. A redraw keeps the
 * scroll of `.body` and `.detail`, which `<details data-key>` are open, the focus on a
 * `[data-key]` element and a tooltip shown on one, so a request settling under the reader moves
 * nothing.
 */
export function drawTab (
  root: ShadowRoot, context: TabContext, draw: () => Children,
  options: { head?: HTMLElement, split?: boolean, also?: readonly ((listener: () => void) => () => void)[] } = {},
): { redraw: () => void, dispose: () => void } {
  const doc = context.document;
  const body = h(doc, 'div', { class: options.split ? 'body split' : 'body' });
  root.append(h(doc, 'style', {}, STYLES), h(doc, 'div', { class: 'pane' }, options.head, body));
  const tooltip = tooltips(root);
  const scrollers = () => [body, ...body.querySelectorAll<HTMLElement>('.detail, .scroll')];
  const redraw = () => {
    const scrolls = scrollers().map((element) => element.scrollTop);
    const focused = (root.activeElement as HTMLElement | null)?.closest?.<HTMLElement>('[data-key]')?.dataset.key;
    const focusInBody = !!root.activeElement && body.contains(root.activeElement);
    kept = new Set([...body.querySelectorAll<HTMLDetailsElement>('details[data-key]')].filter((d) => d.open).map((d) => d.dataset.key!));
    try {
      replace(doc, body, draw());
    } finally {
      kept = undefined;
    }
    scrollers().forEach((element, index) => { element.scrollTop = scrolls[index] ?? 0; });
    keepFlashes(body);
    tooltip.redrawn();
    if (focusInBody && focused !== undefined) {
      [...body.querySelectorAll<HTMLElement>('[data-key]')].find((element) => element.dataset.key === focused)?.focus({ preventScroll: true });
    }
  };
  redraw();
  const stops = [context.panel.subscribe(redraw), ...(options.also ?? []).map((subscribe) => subscribe(redraw))];
  return {
    redraw,
    dispose: () => {
      stops.forEach((stop) => stop());
      tooltip.dispose();
    },
  };
}

/** While a tab redraws: the keys of the collapsibles open before it, which stay open. */
let kept: Set<string> | undefined;

/** A button whose label says "Copied" for a moment after it copies. */
export function copyButton (doc: Document, className: string, text: () => string, labels: { idle: Child, done: Child }, attributes: Record<string, string> = {}): HTMLElement {
  const button = h(doc, 'button', { type: 'button', class: className, ...attributes }, labels.idle);
  button.addEventListener('click', async (event) => {
    event.stopPropagation();
    if (!await copyText(doc, text())) return;
    replace(doc, button, labels.done);
    setTimeout(() => replace(doc, button, labels.idle), 1500);
  });
  return button;
}

/** A small icon button that copies, with its label as tooltip. */
export function copyIcon (doc: Document, label: string, text: () => string, attributes: Record<string, string> = {}): HTMLElement {
  return copyButton(doc, 'row-action', text, { idle: icon(doc, 'copy'), done: icon(doc, 'check') }, { 'data-tip': label, 'aria-label': label, ...attributes });
}

/** A link-styled button: the tabs' cross-links. */
export function link (doc: Document, label: Children, onclick: () => void, attributes: Attributes = {}): HTMLElement {
  return h(doc, 'button', {
    type: 'button',
    class: 'link',
    ...attributes,
    onclick: (event: Event) => {
      event.stopPropagation();
      onclick();
    },
  }, label);
}

/**
 * Show a query in Lineage: Lineage follows the selection, so this selects the query's first marker
 * (`node`). A query drawn nowhere on the page has none, and the button is disabled, saying why. It
 * sits in a row's summary, so its click neither opens nor closes the row.
 */
export function lineageButton (doc: Document, panel: Panel, node: string | undefined): HTMLElement {
  const button = h(doc, 'button', {
    type: 'button',
    class: 'row-action',
    'aria-label': 'Show in Lineage',
    'data-tip': node === undefined ? undefined : 'Show in Lineage',
    'data-testid': 'show-in-lineage',
    disabled: node === undefined,
    onclick: (event: Event) => {
      event.preventDefault();
      event.stopPropagation();
      if (node !== undefined) panel.show(LINEAGE_TAB, node);
    },
  }, icon(doc, 'lineage'));
  // A disabled button may get no pointer events, so what wraps it says why.
  return node === undefined ? h(doc, 'span', { class: 'row-action-wrap', 'data-tip': "Not on the page, so it can't be selected in Lineage" }, button) : button;
}

/**
 * A section that opens and closes (`<details data-key>`). It stays open across a redraw; `open`
 * opens it all the same. Its body is drawn when it opens, so a closed section costs nothing.
 * `onToggle` may hear the same state twice, so it sets rather than flips.
 */
export function collapsible (doc: Document, key: string, summary: Children, body: () => Children, options: { open?: boolean, onToggle?: (open: boolean) => void, class?: string } = {}): HTMLElement {
  const open = options.open || !!kept?.has(key);
  const details = h(doc, 'details', { class: `collapsible ${options.class ?? ''}`, 'data-key': key, open },
    h(doc, 'summary', { 'data-key': `${key}:summary` }, icon(doc, 'chevron'), summary)) as HTMLDetailsElement;
  let drawn = false;
  const fill = () => {
    if (!details.open || drawn) return;
    drawn = true;
    details.append(h(doc, 'div', { class: 'collapsible-body' }, body()));
  };
  fill();
  details.addEventListener('toggle', () => {
    fill();
    options.onToggle?.(details.open);
  });
  return details;
}

/** Rich tooltips, by the element they are on; an element's `data-tip` is a plain one. */
const tips = new WeakMap<Element, () => Children>();

/** Give an element a tooltip of its own content, shown as `data-tip`'s is. */
export function tip<T extends Element> (element: T, content: () => Children): T {
  tips.set(element, content);
  return element;
}

/** A button that shows `content` as its tooltip, on hover and focus as any other, and on a click or tap too. */
export function popover (doc: Document, trigger: Children, content: () => Children, attributes: Attributes = {}): HTMLElement {
  const button = h(doc, 'button', {
    type: 'button',
    class: 'pop-trigger',
    ...attributes,
    onclick: (event: Event) => {
      event.stopPropagation();
      controllers.get(button.getRootNode())?.toggle(button);
    },
  }, trigger);
  return tip(button, content);
}

const TIP_DELAY = 300;
/** Moving from one tooltip to the next within this long shows the next at once. */
const TIP_WARM = 400;
const TIP_GAP = 4;
const TIP_MARGIN = 8;

export interface Tooltips {
  toggle: (element: HTMLElement) => void;
  /** After a redraw: follow the element shown on to its new self by `data-key`, or hide. */
  redrawn: () => void;
  dispose: () => void;
}

const controllers = new WeakMap<Node, Tooltips>();

/**
 * One tooltip for a tab: shown for the `[data-tip]` or `tip()` element under the pointer after a
 * moment, or at once for one focused by keyboard; hidden when the pointer or focus leaves, on
 * Escape, a press, a scroll or a resize. It is a manual popover, in the top layer where the browser
 * has one, so no scrolling box or panel edge clips it, and placed under its element, or above it,
 * inside the window.
 */
export function tooltips (root: ShadowRoot): Tooltips {
  const doc = root.ownerDocument;
  const win = doc.defaultView!;
  const box = h(doc, 'div', { class: 'tooltip', role: 'tooltip', id: 'anfra-tooltip', popover: 'manual', hidden: true });
  root.append(box);
  let anchor: HTMLElement | undefined;
  /** The element waiting out the delay, and its timer. */
  let pending: HTMLElement | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let hiddenAt = 0;

  const target = (event: Event) => event.composedPath().find((node): node is HTMLElement => node instanceof win.HTMLElement && (node.hasAttribute('data-tip') || tips.has(node)));
  const place = () => {
    if (!anchor) return;
    const at = anchor.getBoundingClientRect();
    const size = box.getBoundingClientRect();
    const below = at.bottom + TIP_GAP;
    const above = at.top - TIP_GAP - size.height;
    const fitsBelow = below + size.height <= win.innerHeight - TIP_MARGIN;
    // Below, else above, else as far down as it stays in the window.
    const top = fitsBelow ? below : above >= TIP_MARGIN ? above : Math.max(TIP_MARGIN, win.innerHeight - TIP_MARGIN - size.height);
    box.style.left = `${Math.max(TIP_MARGIN, Math.min(at.left, win.innerWidth - TIP_MARGIN - size.width))}px`;
    box.style.top = `${top}px`;
  };
  const show = (element: HTMLElement) => {
    clearTimeout(timer);
    pending = undefined;
    const rich = tips.get(element);
    const text = element.dataset.tip;
    if (!rich && !text) return;
    anchor?.removeAttribute('aria-describedby');
    anchor = element;
    replace(doc, box, rich ? rich() : text);
    box.classList.toggle('rich', !!rich);
    box.hidden = false;
    if (!box.matches(':popover-open')) box.showPopover?.();
    element.setAttribute('aria-describedby', box.id);
    place();
  };
  const hide = () => {
    clearTimeout(timer);
    pending = undefined;
    if (!anchor) return;
    anchor.removeAttribute('aria-describedby');
    anchor = undefined;
    box.hidden = true;
    box.hidePopover?.();
    hiddenAt = Date.now();
  };
  const later = (element: HTMLElement) => {
    clearTimeout(timer);
    if (anchor || Date.now() - hiddenAt < TIP_WARM) {
      show(element);
      return;
    }
    pending = element;
    timer = setTimeout(() => show(element), TIP_DELAY);
  };

  const listeners: [EventTarget, string, (event: Event) => void, boolean][] = [
    [root, 'pointerover', (event) => {
      const element = target(event);
      if (element && element !== anchor && element !== pending) later(element);
    }, false],
    [root, 'pointerout', (event) => {
      const current = anchor ?? pending;
      const next = (event as PointerEvent).relatedTarget as Node | null;
      if (current && !(next && current.contains(next))) hide();
    }, false],
    [root, 'focusin', (event) => {
      const element = target(event);
      if (element?.matches(':focus-visible')) later(element);
    }, false],
    [root, 'focusout', () => hide(), false],
    [root, 'pointerdown', () => hide(), true],
    [root, 'keydown', (event) => { if ((event as KeyboardEvent).key === 'Escape') hide(); }, false],
    // A scroll inside the tab's shadow root doesn't reach the document, so both are listened to.
    [root, 'scroll', () => hide(), true],
    [win, 'scroll', () => hide(), true],
    [win, 'resize', () => hide(), false],
  ];
  listeners.forEach(([on, name, listener, capture]) => on.addEventListener(name, listener, capture));
  const controller: Tooltips = {
    toggle: (element) => {
      if (anchor === element) hide();
      else show(element);
    },
    redrawn: () => {
      if (!anchor || anchor.isConnected) return;
      const key = anchor.dataset.key;
      const next = key === undefined ? undefined : [...root.querySelectorAll<HTMLElement>('[data-key]')].find((element) => element.dataset.key === key);
      if (next) show(next);
      else hide();
    },
    dispose: () => {
      hide();
      listeners.forEach(([on, name, listener, capture]) => on.removeEventListener(name, listener, capture));
      controllers.delete(root);
      box.remove();
    },
  };
  controllers.set(root, controller);
  return controller;
}

/** The trust meter: one rising bar per level, filled up to the one reached. */
export function trustBars (doc: Document, trust: Trust, size: 'sm' | 'md' | 'lg' = 'md'): HTMLElement {
  const [base, rise, width] = size === 'sm' ? [4, 8, 3] : size === 'lg' ? [20, 28, 12] : [12, 16, 8];
  return h(doc, 'span', { class: `bars ${size}`, 'aria-hidden': 'true' }, LEVELS.map((level) => h(doc, 'span', {
    class: level <= trust.level ? `bar fill-${trust.tone}` : 'bar',
    style: `height:${base + ((level - 1) * rise) / (LEVELS.length - 1)}px;width:${width}px`,
  })));
}

/** A query's trust as small bars, with its level and name as tooltip. */
export function trustIndicator (doc: Document, trust: Trust): HTMLElement {
  const label = `Level ${trust.level} · ${TRUST_LEVELS[trust.level].name}`;
  return h(doc, 'span', { class: 'indicator trust', role: 'img', 'aria-label': `Trust level ${trust.level}`, 'data-tip': label }, trustBars(doc, trust, 'sm'));
}

/** A dot for where a query stands: idle, executing, success or error. */
export function stateDot (doc: Document, state: InspectedQuery['state']): HTMLElement {
  return h(doc, 'span', { class: `dot ${state}`, role: 'img', 'aria-label': `State: ${state}`, 'data-tip': state });
}

/** How many filters narrow a query now, each listed in the tooltip; an empty slot with none, so the icons after it stay in line. */
export function filterIndicator (doc: Document, active: readonly QueryInput[]): HTMLElement {
  if (active.length === 0) return h(doc, 'span', { class: 'indicator filters', 'aria-hidden': 'true' });
  const lines = active.map((input) => {
    const source = input.kind === 'declared' ? 'query' : input.self ? 'itself' : input.source;
    return `${input.field} · ${source}${input.value !== '—' ? `\n  ${input.value}` : ''}${input.pending ? `\n  pending: ${input.pending}` : ''}`;
  });
  return h(doc, 'span', {
    class: 'indicator filters',
    'data-tip': `${plural(active.length, 'filter')} applied\n${lines.join('\n')}`,
    'aria-label': `${plural(active.length, 'filter')} applied`,
  }, String(active.length), icon(doc, 'filter'));
}

/** Code, with AQL, SQL or JS highlighted as text spans, never markup. */
export function highlighted (doc: Document, code: string, lang: Lang | undefined): HTMLElement {
  const tokens = lang ? tokenize(code, lang) : [{ text: code }];
  return h(doc, 'pre', { class: 'code-text scroll' }, h(doc, 'code', {}, tokens.map((token) => (token.kind ? h(doc, 'span', { class: `tok-${token.kind}` }, token.text) : token.text))));
}

/**
 * A value written as `JSON.stringify(value, null, 2)` writes it, as highlighted spans: the text
 * is the same, character for character. `aql` strings are shown across their lines.
 */
function json (doc: Document, value: unknown, indent: string, key: string | undefined): Children {
  const data = (value as { toJSON?: () => unknown } | null)?.toJSON?.() ?? value;
  if (typeof data === 'string') return key === 'aql' && data.includes('\n') ? aql(doc, data, `${indent}  `) : h(doc, 'span', { class: 'tok-str' }, JSON.stringify(data));
  if (typeof data === 'number' || typeof data === 'boolean' || data === null) {
    return h(doc, 'span', { class: typeof data === 'number' ? 'tok-num' : 'tok-kw' }, JSON.stringify(data));
  }
  const inner = `${indent}  `;
  if (Array.isArray(data)) {
    if (!data.length) return '[]';
    // As JSON.stringify does: a value it can't write is null in an array.
    return ['[\n', data.map((item, i) => [inner, json(doc, item === undefined || typeof item === 'function' ? null : item, inner, undefined), i < data.length - 1 ? ',\n' : '\n']), `${indent}]`];
  }
  const entries = Object.entries(data as Record<string, unknown>).filter(([, item]) => item !== undefined && typeof item !== 'function');
  if (!entries.length) return '{}';
  return ['{\n', entries.map(([name, item], i) => [
    inner, h(doc, 'span', { class: 'tok-key' }, JSON.stringify(name)), ': ', json(doc, item, inner, name), i < entries.length - 1 ? ',\n' : '\n',
  ]), `${indent}}`];
}

/** An AQL string as JSON, highlighted as AQL, its lines shown under `indent` without the indent they share. */
function aql (doc: Document, text: string, indent: string): Children {
  const lines = text.split('\n');
  // As `dedent` reads it: the first line is usually trimmed already (`aql: \`explore {`).
  const rest = lines.slice(1).filter((line) => line.trim());
  const shared = rest.length ? Math.min(...rest.map((line) => line.length - line.trimStart().length)) : 0;
  const escape = (part: string) => JSON.stringify(part).slice(1, -1);
  return h(doc, 'span', { class: 'tok-str' }, '"', lines.map((line, i) => {
    const cut = i === 0 ? 0 : Math.min(shared, line.length - line.trimStart().length);
    return [
      cut > 0 && h(doc, 'span', { class: 'json-dedent' }, escape(line.slice(0, cut))),
      tokenize(line.slice(cut), 'aql').map((token) => (token.kind ? h(doc, 'span', { class: `tok-${token.kind}` }, escape(token.text)) : escape(token.text))),
      i < lines.length - 1 && [h(doc, 'span', { class: 'json-nl' }, '\\n'), h(doc, 'span', { class: 'json-br', 'data-indent': indent })],
    ];
  }), '"');
}

/** A value as `JSON.stringify(value, null, 2)` writes it, highlighted, with its `aql` strings across their lines. */
export function jsonView (doc: Document, value: unknown): HTMLElement {
  return h(doc, 'pre', { class: 'code-text scroll' }, h(doc, 'code', {}, json(doc, value, '', undefined)));
}

/** One piece of code a code block shows: its label, its code and language, and why it is empty when it is. */
export interface CodeSource {
  label: string;
  code: string | undefined;
  lang?: Lang;
  /** The value `code` is the JSON of: shown as `jsonView` shows it. */
  json?: unknown;
  empty: string;
}

/**
 * Several pieces of code, one at a time: tabs, a copy button, and the code. Opens on `tab`, or the
 * first that has code; `onTab` hears the reader's choice, to keep it.
 */
export function codeBlock<T extends string> (doc: Document, sources: Record<T, CodeSource>, tab: T, onTab: (tab: T) => void = () => {}): HTMLElement {
  const tabs = Object.keys(sources) as T[];
  let shown = sources[tab]?.code ? tab : (tabs.find((t) => sources[t].code) ?? tab);
  const block = h(doc, 'div', { class: 'code' });
  const draw = () => {
    const { label, code, lang, json: value, empty } = sources[shown];
    const body = !code ? h(doc, 'p', { class: 'code-empty' }, empty) : value !== undefined ? jsonView(doc, value) : highlighted(doc, code, lang);
    const copy = code && copyIcon(doc, `Copy the ${label}`, () => code);
    replace(doc, block,
      tabs.length > 1
        ? h(doc, 'div', { class: 'code-head', role: 'tablist' },
          tabs.map((t) => {
            const button = h(doc, 'button', {
              type: 'button',
              role: 'tab',
              class: 'code-tab',
              'aria-selected': String(t === shown),
              disabled: !sources[t].code,
              onclick: () => {
                shown = t;
                onTab(t);
                draw();
              },
            }, sources[t].label);
            // A disabled button may get no pointer events, so what wraps it says why.
            return sources[t].code ? button : h(doc, 'span', { class: 'code-tab-wrap', 'data-tip': sources[t].empty }, button);
          }),
          h(doc, 'span', { class: 'spacer' }), copy)
        : copy && h(doc, 'div', { class: 'code-corner' }, copy),
      body);
  };
  draw();
  return block;
}

/** Rows in a small scrolling table, headed by `columns`; each cell as text. */
export function table (doc: Document, columns: readonly Children[], rows: readonly (readonly unknown[])[], attributes: Attributes = {}): HTMLElement {
  return h(doc, 'div', { class: 'table-wrap', ...attributes },
    h(doc, 'table', {},
      h(doc, 'thead', {}, h(doc, 'tr', {}, columns.map((column) => h(doc, 'th', {}, column)))),
      h(doc, 'tbody', {}, rows.map((row) => h(doc, 'tr', {}, row.map((cell) => h(doc, 'td', {}, cellText(cell))))))));
}

/** A result value as text: null as an empty cell, an object as JSON. */
export function cellText (value: unknown): string {
  if (value === null || value === undefined) return '';
  if (value instanceof Date) return value.toISOString();
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

/** A labelled list of terms: `[term, value]` pairs, a false pair left out. */
export function fields (doc: Document, pairs: readonly (readonly [Children, Children] | false | undefined)[], attributes: Attributes = {}): HTMLElement {
  return h(doc, 'dl', attributes, pairs.map((pair) => pair && [h(doc, 'dt', {}, pair[0]), h(doc, 'dd', {}, pair[1])]));
}

/** A note in the tab's body, for an empty or waiting state. */
export const note = (doc: Document, text: Children, attributes: Attributes = {}) => h(doc, 'p', { class: 'note', ...attributes }, text);

const FLASH_MS = 1200;
/** Per tab body: when each `[data-key]` element was flashed, so a redraw in the meantime carries the flash on. */
const flashes = new WeakMap<Element, Map<string, number>>();

/** Flash an element once, after scrolling it into view: where a link landed. */
export function flash (element: HTMLElement): void {
  element.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' });
  element.classList.remove('flash');
  element.style.animationDelay = '';
  void element.offsetWidth;
  element.classList.add('flash');
  const key = element.dataset.key;
  const body = element.closest('.body');
  if (key === undefined || !body) return;
  if (!flashes.has(body)) flashes.set(body, new Map());
  flashes.get(body)!.set(key, Date.now());
}

function keepFlashes (body: HTMLElement): void {
  const now = Date.now();
  const keys = flashes.get(body);
  for (const [key, at] of keys ?? []) {
    if (now - at >= FLASH_MS) { keys!.delete(key); continue; }
    const element = [...body.querySelectorAll<HTMLElement>('[data-key]')].find((candidate) => candidate.dataset.key === key);
    if (!element) continue;
    // The same flash, picked up where it was.
    element.style.animationDelay = `-${now - at}ms`;
    element.classList.add('flash');
  }
}

/** The tabs' stylesheet, on the host's tokens. Fixed colours only for meaning (states, tones, kinds), each with a dark variant. */
export const STYLES = `
:host { display: block; height: 100%; color: var(--anfra-color-fg); background: var(--anfra-color-bg); font: 13px/1.45 var(--anfra-font); }
* { box-sizing: border-box; }
[hidden] { display: none !important; }
button { font: inherit; color: inherit; cursor: pointer; }
button:disabled { cursor: default; }
:focus-visible { outline: 2px solid var(--anfra-color-accent); outline-offset: 1px; }
.pane { height: 100%; display: flex; flex-direction: column; min-height: 0;
  --hover: color-mix(in srgb, var(--anfra-color-fg) 7%, transparent);
  --subtle: color-mix(in srgb, var(--anfra-color-fg) 4%, transparent);
  --accent-bg: color-mix(in srgb, var(--anfra-color-accent) 14%, transparent);
  --danger-bg: color-mix(in srgb, var(--anfra-color-danger) 14%, transparent);
  --ok: light-dark(#059669, #34d399); --warn: light-dark(#d97706, #fbbf24); --run: light-dark(#0284c7, #38bdf8);
  --warn-bg: color-mix(in srgb, var(--warn) 16%, transparent); --ok-bg: color-mix(in srgb, var(--ok) 16%, transparent); }
.body { flex: 1; min-height: 0; overflow: auto; padding: 4px 14px 16px; }
.body.split { display: flex; flex-direction: column; overflow: hidden; padding: 0; }
.body.split > .scroll { flex: 1; min-height: 0; overflow: auto; padding: 4px 14px 16px; }
.note { color: var(--anfra-color-muted); margin: 12px 0; }
.muted { color: var(--anfra-color-muted); }
code, pre { font-family: var(--anfra-font-mono); }
h4 { margin: 16px 0 0; font-size: 13px; }
h5 { margin: 16px 0 6px; font-size: 13px; font-weight: 500; color: var(--anfra-color-fg); display: flex; align-items: center; gap: 6px; }
dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 4px 12px; margin: 0; }
dt { color: var(--anfra-color-muted); }
dd { margin: 0; overflow-wrap: anywhere; min-width: 0; }
.icon { flex: none; }
.card { margin-bottom: 8px; padding: 10px 12px; border-radius: 8px; background: var(--subtle); }
.card header { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.flash { animation: flash 1.2s ease-out; }
@keyframes flash { from { box-shadow: inset 0 0 0 2px var(--anfra-color-accent); background: var(--accent-bg); } }
.pill { flex: none; padding: 1px 8px; border-radius: 999px; background: var(--hover); color: var(--anfra-color-muted); font-size: 12px; font-weight: 500; white-space: nowrap; }
.pill.warn { background: var(--warn-bg); color: var(--warn); }
.pill.success, .pill.ok { background: var(--ok-bg); color: var(--ok); }
.pill.error { background: var(--danger-bg); color: var(--anfra-color-danger); }
.pill.accent { background: var(--accent-bg); color: var(--anfra-color-accent); }
.tone-ok { color: var(--ok); } .tone-warn { color: var(--warn); } .tone-error { color: var(--anfra-color-danger); }
code.wrap { overflow-wrap: anywhere; }
.error-text { margin: 6px 0 0; color: var(--anfra-color-danger); overflow-wrap: anywhere; }
.link { border: 0; border-radius: 4px; background: none; padding: 2px 6px; color: var(--anfra-color-accent); font-size: 12px; font-weight: 500; display: inline-flex; align-items: center; gap: 4px; }
.link:hover:not(:disabled) { background: var(--hover); }
.link:disabled { color: var(--anfra-color-muted); }
.toolbar { flex: none; display: flex; align-items: center; gap: 6px; padding: 8px 14px; border-bottom: 1px solid var(--anfra-color-border); }
.tool { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: none; padding: 3px 10px; font-size: 12px; }
.tool:hover:not(:disabled) { background: var(--hover); }
.tool[aria-pressed="true"] { border-color: var(--anfra-color-accent); color: var(--anfra-color-accent); background: var(--accent-bg); }
.search { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; padding: 3px 8px; border: 1px solid var(--anfra-color-border); border-radius: 6px; color: var(--anfra-color-muted); }
.search input { flex: 1; min-width: 0; border: 0; outline: 0; background: none; color: var(--anfra-color-fg); font: inherit; font-size: 12px; }
.segments { flex: none; display: inline-flex; gap: 2px; padding: 2px; border-radius: 8px; background: var(--hover); }
.segments button { border: 0; border-radius: 6px; background: none; padding: 2px 8px; font-size: 12px; font-weight: 500; color: var(--anfra-color-muted); white-space: nowrap; }
.segments button:hover { color: var(--anfra-color-fg); }
/* The selected one is raised, in ink: violet is for links, selection and focus. */
.segments button[aria-pressed="true"] { background: var(--anfra-color-bg); color: var(--anfra-color-fg); box-shadow: 0 0 0 1px var(--anfra-color-border), 0 1px 2px rgb(0 0 0 / 0.08); }
.row-action { flex: none; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--anfra-color-muted); }
.row-action:hover:not(:disabled) { background: var(--hover); color: var(--anfra-color-accent); }
.row-action:disabled { opacity: 0.4; cursor: default; }
.row-action-wrap { flex: none; display: inline-flex; }
.spacer { flex: 1; }
/* Collapsibles */
.collapsible > summary { list-style: none; display: flex; align-items: center; gap: 6px; padding: 4px 0; cursor: pointer; user-select: none; }
.collapsible > summary::-webkit-details-marker { display: none; }
.collapsible > summary > .icon { transition: transform 0.15s; color: var(--anfra-color-muted); }
.collapsible[open] > summary > .icon { transform: rotate(90deg); }
.collapsible-body { padding: 2px 0 8px 20px; min-width: 0; }
.section > summary { font-size: 13px; font-weight: 500; margin-top: 12px; }
.section > .collapsible-body { padding-left: 0; }
/* Rows that open, as tiles: the chevron at the end, the body inside the tile. A tile in a tile is on the panel's ground. */
.row { border-radius: 8px; background: var(--subtle); }
.row + .row { margin-top: 6px; }
.row > summary { min-height: 40px; padding: 6px 8px 6px 12px; border-radius: 8px; min-width: 0; }
.row > summary:hover { background: var(--hover); }
.row > summary > .icon { order: 99; margin-left: 4px; color: var(--anfra-color-muted); }
.row > summary .name { font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.row > .collapsible-body { padding: 2px 12px 12px; }
.row .row { background: var(--anfra-color-bg); box-shadow: inset 0 0 0 1px var(--anfra-color-border); }
.row.flash > summary, .card.flash, .row.flash { animation: flash 1.2s ease-out; }
.sub > summary { font-size: 13px; font-weight: 500; }
.sub > .collapsible-body { padding-left: 20px; }
.indicator { flex: none; display: inline-flex; align-items: center; justify-content: flex-end; gap: 3px; color: var(--anfra-color-muted); font-size: 12px; font-variant-numeric: tabular-nums; }
.indicator.filters { width: 32px; }
.indicator.trust { width: 20px; height: 12px; align-items: flex-end; }
.bars { display: inline-flex; align-items: flex-end; gap: 2px; }
.bars.sm { gap: 1px; } .bars.lg { gap: 4px; }
.bar { display: inline-block; border-radius: 2px; background: color-mix(in srgb, var(--anfra-color-muted) 30%, transparent); }
.bar.fill-ok { background: var(--ok); } .bar.fill-warn { background: var(--warn); } .bar.fill-error { background: var(--anfra-color-danger); }
.dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: color-mix(in srgb, var(--anfra-color-muted) 45%, transparent); }
.dot.success { background: var(--ok); } .dot.error { background: var(--anfra-color-danger); } .dot.ok { background: var(--ok); } .dot.aborted { background: var(--warn); }
.dot.executing, .dot.pending { background: var(--run); animation: pulse 1.2s ease-in-out infinite; }
@keyframes pulse { 50% { opacity: 0.35; } }
/* Popovers */
.pop-trigger { border: 0; background: none; padding: 0; display: inline-flex; align-items: center; gap: 4px; color: inherit; }
/* Plain tooltips are one line of text or a few, with their line breaks; rich ones lay out their own. */
.tooltip { position: fixed; inset: auto; margin: 0; z-index: 10; width: max-content; max-width: 280px; padding: 5px 8px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: var(--anfra-color-bg); color: var(--anfra-color-fg); box-shadow: 0 6px 20px rgb(0 0 0 / 0.15); font: 12px/1.45 var(--anfra-font); white-space: pre-line; overflow-wrap: anywhere; pointer-events: none; }
.tooltip.rich { max-width: 340px; padding: 8px 10px; white-space: normal; }
/* Code */
.code { margin-top: 6px; border: 1px solid var(--anfra-color-border); border-radius: 6px; overflow: hidden; position: relative; }
.code-head { display: flex; align-items: center; gap: 2px; padding: 0 4px; border-bottom: 1px solid var(--anfra-color-border); background: var(--subtle); }
.code-tab { border: 0; border-bottom: 2px solid transparent; background: none; padding: 5px 6px; font-size: 12px; color: var(--anfra-color-muted); }
.code-tab[aria-selected="true"] { color: var(--anfra-color-fg); border-bottom-color: var(--anfra-color-accent); }
.code-tab:disabled { opacity: 0.5; }
.code-tab-wrap { display: inline-flex; }
.code-corner { position: absolute; top: 2px; right: 2px; }
.code-text { margin: 0; max-height: 240px; overflow: auto; padding: 8px 10px; font-size: 12px; line-height: 1.5; background: var(--subtle); }
.code-empty { margin: 0; padding: 8px 10px; font-size: 12px; color: var(--anfra-color-muted); background: var(--subtle); }
.tok-kw { color: light-dark(#6d28d9, #c4b5fd); font-weight: 600; } .tok-str { color: light-dark(#047857, #6ee7b7); }
.tok-key { color: light-dark(#0f766e, #5eead4); }
/* An AQL string, shown across its lines: the text stays the JSON, escapes and all, so a copy of
   it is too; the line breaks and the indent under its key are generated, which a copy leaves out. */
.json-nl { opacity: 0.45; }
.json-br::after { content: "\\A" attr(data-indent); white-space: pre; }
/* The indent every line of the AQL shares: kept in the text, so the copy is exact, but not shown. */
.json-dedent { font-size: 0; }
.tok-num { color: light-dark(#b45309, #fcd34d); } .tok-com { color: var(--anfra-color-muted); font-style: italic; } .tok-fn { color: light-dark(#1d4ed8, #93c5fd); }
/* Tables */
.table-wrap { overflow: auto; max-height: 320px; border: 1px solid var(--anfra-color-border); border-radius: 6px; }
table { border-collapse: collapse; font-size: 12px; width: max-content; min-width: 100%; }
th, td { padding: 3px 8px; text-align: left; border-bottom: 1px solid var(--anfra-color-border); white-space: nowrap; max-width: 280px; overflow: hidden; text-overflow: ellipsis; }
th { position: sticky; top: 0; background: var(--anfra-color-bg); font-weight: 500; z-index: 1; }
tbody tr:last-child td { border-bottom: 0; }
/* Structure */
.tree { list-style: none; margin: 0; padding: 0; }
.node { display: flex; align-items: center; gap: 6px; padding: 4px 8px; border-radius: 6px; font-size: 13px; cursor: pointer; white-space: nowrap; overflow: hidden; }
.node:hover { background: var(--hover); }
.twist .icon { transition: transform 0.15s; } .twist.open .icon { transform: rotate(90deg); }
.node .twist { width: 16px; height: 16px; } .twist-space { flex: none; width: 16px; }
.node.selected { background: var(--accent-bg); }
.name { font-weight: 500; overflow: hidden; text-overflow: ellipsis; }
.node .label, .node .app { color: var(--anfra-color-muted); overflow: hidden; text-overflow: ellipsis; }
.node.problem .name { color: var(--anfra-color-danger); }
.kind { flex: none; padding: 0 6px; border-radius: 4px; font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
.kind.container { background: color-mix(in srgb, light-dark(#7c3aed, #a78bfa) 18%, transparent); color: light-dark(#7c3aed, #a78bfa); }
.kind.block { background: color-mix(in srgb, light-dark(#2563eb, #93c5fd) 18%, transparent); color: light-dark(#2563eb, #93c5fd); }
.kind.query { background: var(--ok-bg); color: var(--ok); }
.kind.control { background: var(--warn-bg); color: var(--warn); }
/* The actions keep their place and sit in the row's height, so showing them moves nothing. */
.actions { flex: none; display: flex; gap: 2px; margin: -4px 0 -4px auto; visibility: hidden; }
.node:hover .actions, .node.selected .actions, .node:focus-within .actions { visibility: visible; }
.detail { flex: none; max-height: 45%; overflow: auto; padding: 8px 14px 16px; border-top: 1px solid var(--anfra-color-border); }
.detail h5 { margin-top: 4px; }
.detail .card { margin-top: 16px; }
.detail-actions { display: flex; gap: 6px; margin-top: 8px; }
.status { margin: 8px 0 0; color: var(--anfra-color-muted); }
`;
