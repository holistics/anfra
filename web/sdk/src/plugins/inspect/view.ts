/**
 * What `inspect`'s tabs draw with: a small element builder, the icons, the widgets every tab
 * shares, and the stylesheet, all on the host's `--anfra-*` tokens. Plain DOM, so the tabs carry
 * no framework into a host that has its own.
 */
import type { InspectedQuery } from '../../common/types';
import type { TabContext } from '../../host/contributions';
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
 * scroll of `.body` and `.detail`, which `<details data-key>` are open, and the focus on a
 * `[data-key]` element, so a request settling under the reader moves nothing.
 */
export function drawTab (
  root: ShadowRoot, context: TabContext, draw: () => Children,
  options: { head?: HTMLElement, split?: boolean, also?: readonly ((listener: () => void) => () => void)[] } = {},
): { redraw: () => void, dispose: () => void } {
  const doc = context.document;
  const body = h(doc, 'div', { class: options.split ? 'body split' : 'body' });
  root.append(h(doc, 'style', {}, STYLES), h(doc, 'div', { class: 'pane' }, options.head, body));
  const scrollers = () => [body, ...body.querySelectorAll<HTMLElement>('.detail, .scroll')];
  const redraw = () => {
    const scrolls = scrollers().map((element) => element.scrollTop);
    const focused = (root.activeElement as HTMLElement | null)?.closest?.<HTMLElement>('[data-key]')?.dataset.key;
    const focusInBody = !!root.activeElement && body.contains(root.activeElement);
    kept = new Set([
      ...[...body.querySelectorAll<HTMLDetailsElement>('details[data-key]')].filter((d) => d.open).map((d) => d.dataset.key!),
      ...[...body.querySelectorAll<HTMLElement>('.pop-trigger[aria-expanded="true"][data-key]')].map((trigger) => trigger.dataset.key!),
    ]);
    try {
      replace(doc, body, draw());
    } finally {
      kept = undefined;
    }
    scrollers().forEach((element, index) => { element.scrollTop = scrolls[index] ?? 0; });
    keepFlashes(body);
    if (focusInBody && focused !== undefined) {
      [...body.querySelectorAll<HTMLElement>('[data-key]')].find((element) => element.dataset.key === focused)?.focus({ preventScroll: true });
    }
  };
  redraw();
  const stops = [context.panel.subscribe(redraw), ...(options.also ?? []).map((subscribe) => subscribe(redraw))];
  return { redraw, dispose: () => stops.forEach((stop) => stop()) };
}

/** While a tab redraws: the keys of the collapsibles and popovers open before it, which stay open. */
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
  return copyButton(doc, 'row-action', text, { idle: icon(doc, 'copy'), done: icon(doc, 'check') }, { title: label, 'aria-label': label, ...attributes });
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

/**
 * A button that shows `content` under it until clicked again, or until focus leaves, the panel
 * scrolls or the window resizes. Given a `data-key`, it stays open across a redraw. The box is a
 * manual popover, so it sits in the top layer where the browser has one, and no scrolling box or
 * panel edge clips it; it is placed against the viewport, flipped above or shifted left to fit.
 */
export function popover (doc: Document, trigger: Children, content: () => Children, attributes: Attributes = {}): HTMLElement {
  const win = doc.defaultView!;
  const box = h(doc, 'span', { class: 'popover', role: 'tooltip', popover: 'manual', hidden: true });
  const place = () => {
    const anchor = button.getBoundingClientRect();
    const size = box.getBoundingClientRect();
    const room = { width: win.innerWidth, height: win.innerHeight };
    const below = anchor.bottom + POP_GAP;
    const top = below + size.height <= room.height - POP_MARGIN || anchor.top - POP_GAP - size.height < POP_MARGIN ? below : anchor.top - POP_GAP - size.height;
    const left = Math.max(POP_MARGIN, Math.min(anchor.left, room.width - POP_MARGIN - size.width));
    box.style.left = `${left}px`;
    box.style.top = `${top}px`;
  };
  const away = () => set(false);
  const set = (show: boolean) => {
    if (show === !box.hidden) return;
    const root = wrap.getRootNode();
    // A scroll inside the tab's shadow root doesn't reach the document, so both are listened to.
    for (const target of [win, root]) {
      if (show) target.addEventListener('scroll', away, true);
      else target.removeEventListener('scroll', away, true);
    }
    if (show) win.addEventListener('resize', away);
    else win.removeEventListener('resize', away);
    if (show) replace(doc, box, content());
    box.hidden = !show;
    if (show) box.showPopover?.();
    else box.hidePopover?.();
    button.setAttribute('aria-expanded', String(show));
    if (show) place();
  };
  const button = h(doc, 'button', {
    type: 'button',
    class: 'pop-trigger',
    'aria-expanded': 'false',
    ...attributes,
    onclick: (event: Event) => {
      event.stopPropagation();
      set(box.hidden);
    },
  }, trigger);
  const wrap = h(doc, 'span', { class: 'pop' }, button, box);
  wrap.addEventListener('focusout', (event) => {
    if (!wrap.contains((event as FocusEvent).relatedTarget as Node | null)) set(false);
  });
  const key = attributes['data-key'];
  // Reopened once it is in the page, so it can be placed.
  if (typeof key === 'string' && kept?.has(key)) queueMicrotask(() => { if (wrap.isConnected) set(true); });
  return wrap;
}

const POP_GAP = 4;
const POP_MARGIN = 8;

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
  return h(doc, 'span', { class: 'indicator trust', role: 'img', 'aria-label': `Trust level ${trust.level}`, title: label }, trustBars(doc, trust, 'sm'));
}

/** A dot for where a query stands: idle, executing, success or error. */
export function stateDot (doc: Document, state: InspectedQuery['state']): HTMLElement {
  return h(doc, 'span', { class: `dot ${state}`, role: 'img', 'aria-label': `State: ${state}`, title: state });
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
    title: `${plural(active.length, 'filter')} applied\n${lines.join('\n')}`,
    'aria-label': `${plural(active.length, 'filter')} applied`,
  }, String(active.length), icon(doc, 'filter'));
}

/** Code, with AQL, SQL or JS highlighted as text spans, never markup. */
export function highlighted (doc: Document, code: string, lang: Lang | undefined): HTMLElement {
  const tokens = lang ? tokenize(code, lang) : [{ text: code }];
  return h(doc, 'pre', { class: 'code-text scroll' }, h(doc, 'code', {}, tokens.map((token) => (token.kind ? h(doc, 'span', { class: `tok-${token.kind}` }, token.text) : token.text))));
}

/** One piece of code a code block shows: its label, its code and language, and why it is empty when it is. */
export interface CodeSource {
  label: string;
  code: string | undefined;
  lang?: Lang;
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
    const { label, code, lang, empty } = sources[shown];
    const body = code ? highlighted(doc, code, lang) : h(doc, 'p', { class: 'code-empty' }, empty);
    const copy = code && copyIcon(doc, `Copy the ${label}`, () => code);
    replace(doc, block,
      tabs.length > 1
        ? h(doc, 'div', { class: 'code-head', role: 'tablist' },
          tabs.map((t) => h(doc, 'button', {
            type: 'button',
            role: 'tab',
            class: 'code-tab',
            'aria-selected': String(t === shown),
            disabled: !sources[t].code,
            title: sources[t].code ? undefined : sources[t].empty,
            onclick: () => {
              shown = t;
              onTab(t);
              draw();
            },
          }, sources[t].label)),
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
h4 { margin: 16px 0 0; font-size: 14px; }
h5 { margin: 16px 0 6px; font-size: 11px; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; color: var(--anfra-color-muted); display: flex; align-items: center; gap: 6px; }
dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 4px 12px; margin: 0; }
dt { color: var(--anfra-color-muted); }
dd { margin: 0; overflow-wrap: anywhere; min-width: 0; }
.icon { flex: none; }
.card { margin-bottom: 8px; padding: 10px 12px; border: 1px solid var(--anfra-color-border); border-radius: var(--anfra-radius); }
.card header { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.flash { animation: flash 1.2s ease-out; }
@keyframes flash { from { box-shadow: inset 0 0 0 2px var(--anfra-color-accent); background: var(--accent-bg); } }
.pill { flex: none; padding: 1px 8px; border-radius: 10px; background: var(--hover); color: var(--anfra-color-muted); font-size: 11px; font-weight: 600; white-space: nowrap; }
.pill.warn { background: var(--warn-bg); color: var(--warn); }
.pill.success, .pill.ok { background: var(--ok-bg); color: var(--ok); }
.pill.error { background: var(--danger-bg); color: var(--anfra-color-danger); }
.pill.accent { background: var(--accent-bg); color: var(--anfra-color-accent); }
.tone-ok { color: var(--ok); } .tone-warn { color: var(--warn); } .tone-error { color: var(--anfra-color-danger); }
code.wrap { overflow-wrap: anywhere; }
.error-text { margin: 6px 0 0; color: var(--anfra-color-danger); overflow-wrap: anywhere; }
.link { border: 0; border-radius: 4px; background: none; padding: 2px 6px; color: var(--anfra-color-accent); font-size: 11px; display: inline-flex; align-items: center; gap: 4px; }
.link:hover:not(:disabled) { background: var(--hover); }
.link:disabled { color: var(--anfra-color-muted); }
.toolbar { flex: none; display: flex; align-items: center; gap: 6px; padding: 8px 14px; border-bottom: 1px solid var(--anfra-color-border); }
.tool { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: none; padding: 3px 10px; font-size: 12px; }
.tool:hover:not(:disabled) { background: var(--hover); }
.tool[aria-pressed="true"] { border-color: var(--anfra-color-accent); color: var(--anfra-color-accent); background: var(--accent-bg); }
.search { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; padding: 3px 8px; border: 1px solid var(--anfra-color-border); border-radius: 6px; color: var(--anfra-color-muted); }
.search input { flex: 1; min-width: 0; border: 0; outline: 0; background: none; color: var(--anfra-color-fg); font: inherit; font-size: 12px; }
.segments { flex: none; display: inline-flex; border: 1px solid var(--anfra-color-border); border-radius: 6px; overflow: hidden; }
.segments button { border: 0; background: none; padding: 3px 8px; font-size: 12px; }
.segments button + button { border-left: 1px solid var(--anfra-color-border); }
.segments button[aria-pressed="true"] { background: var(--accent-bg); color: var(--anfra-color-accent); }
.row-action { flex: none; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--anfra-color-muted); }
.row-action:hover { background: var(--hover); color: var(--anfra-color-accent); }
.spacer { flex: 1; }
/* Collapsibles */
.collapsible > summary { list-style: none; display: flex; align-items: center; gap: 6px; padding: 4px 0; cursor: pointer; user-select: none; }
.collapsible > summary::-webkit-details-marker { display: none; }
.collapsible > summary > .icon { transition: transform 0.15s; color: var(--anfra-color-muted); }
.collapsible[open] > summary > .icon { transform: rotate(90deg); }
.collapsible-body { padding: 2px 0 8px 20px; min-width: 0; }
.section > summary { font-size: 11px; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; color: var(--anfra-color-muted); margin-top: 10px; }
/* One-line rows that open */
.row { border-bottom: 1px solid var(--anfra-color-border); }
.row > summary { padding: 6px 4px; border-radius: 4px; min-width: 0; }
.row > summary:hover { background: var(--hover); }
.row > summary .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.row .collapsible-body { padding: 4px 4px 12px 20px; }
.row.flash > summary, .card.flash { animation: flash 1.2s ease-out; }
.sub > summary { font-size: 12px; font-weight: 600; color: var(--anfra-color-muted); }
.indicator { flex: none; display: inline-flex; align-items: center; justify-content: flex-end; gap: 3px; color: var(--anfra-color-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
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
.pop { position: relative; display: inline-flex; }
.pop-trigger { border: 0; background: none; padding: 0; display: inline-flex; align-items: center; gap: 4px; color: inherit; }
.popover { position: fixed; inset: auto; margin: 0; z-index: 10; width: max-content; max-width: 280px; padding: 8px 10px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: var(--anfra-color-bg); color: var(--anfra-color-fg); box-shadow: 0 6px 20px rgb(0 0 0 / 0.15); font-size: 12px; font-weight: normal; text-transform: none; letter-spacing: normal; white-space: normal; }
/* Code */
.code { margin-top: 6px; border: 1px solid var(--anfra-color-border); border-radius: 6px; overflow: hidden; position: relative; }
.code-head { display: flex; align-items: center; gap: 2px; padding: 0 4px; border-bottom: 1px solid var(--anfra-color-border); background: var(--subtle); }
.code-tab { border: 0; border-bottom: 2px solid transparent; background: none; padding: 5px 6px; font-size: 11px; color: var(--anfra-color-muted); }
.code-tab[aria-selected="true"] { color: var(--anfra-color-fg); border-bottom-color: var(--anfra-color-accent); }
.code-tab:disabled { opacity: 0.5; }
.code-corner { position: absolute; top: 2px; right: 2px; }
.code-text { margin: 0; max-height: 240px; overflow: auto; padding: 8px 10px; font-size: 12px; line-height: 1.5; background: var(--subtle); }
.code-empty { margin: 0; padding: 8px 10px; font-size: 12px; color: var(--anfra-color-muted); background: var(--subtle); }
.tok-kw { color: light-dark(#6d28d9, #c4b5fd); font-weight: 600; } .tok-str { color: light-dark(#047857, #6ee7b7); }
.tok-num { color: light-dark(#b45309, #fcd34d); } .tok-com { color: var(--anfra-color-muted); font-style: italic; } .tok-fn { color: light-dark(#1d4ed8, #93c5fd); }
/* Tables */
.table-wrap { overflow: auto; max-height: 320px; border: 1px solid var(--anfra-color-border); border-radius: 6px; }
table { border-collapse: collapse; font-size: 12px; width: max-content; min-width: 100%; }
th, td { padding: 3px 8px; text-align: left; border-bottom: 1px solid var(--anfra-color-border); white-space: nowrap; max-width: 280px; overflow: hidden; text-overflow: ellipsis; }
th { position: sticky; top: 0; background: var(--anfra-color-bg); font-weight: 600; z-index: 1; }
tbody tr:last-child td { border-bottom: 0; }
/* Structure */
.tree { list-style: none; margin: 0; padding: 0; }
.node { display: flex; align-items: center; gap: 6px; padding: 3px 8px; border-radius: 4px; font-size: 12px; cursor: pointer; white-space: nowrap; overflow: hidden; }
.node:hover { background: var(--hover); }
.twist .icon { transition: transform 0.15s; } .twist.open .icon { transform: rotate(90deg); }
.node .twist { width: 16px; height: 16px; } .twist-space { flex: none; width: 16px; }
.node.selected { background: var(--accent-bg); }
.name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
.label, .app { color: var(--anfra-color-muted); overflow: hidden; text-overflow: ellipsis; }
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
