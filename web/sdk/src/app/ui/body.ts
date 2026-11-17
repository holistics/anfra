/**
 * The kernel's UI body: the menu, the modal and the popover plugins draw content UI with, in the
 * plugin layer, on the host's `--anfra-*` tokens. Sent to a frame the first time it opens one
 * (`anfra.body` for `anfra`), so a frame that never does carries none of it. The kernel hands it its
 * shadow root, and keeps one open at a time. Also the annotations badge, and the list its popover
 * shows (docs/designs/sdk-plugins.md, Annotations).
 */
import type { AnnotationSeverity } from '../../common/types';

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface MenuEntry {
  id: string;
  label: string;
  /** Why it can't run: shown, and the entry can't be picked. */
  disabled?: string;
}

/** An annotation as a reader is shown it: in a menu's head, or in the badge's popover. */
export interface AnnotationNote {
  label: string;
  detail?: string;
  severity?: AnnotationSeverity;
  /** The plugin that set it. */
  plugin: string;
}

export interface MenuOptions {
  /** Where the pointer was, in the viewport. */
  x: number;
  y: number;
  /** The annotations on the rows, listed before the entries, to read only. */
  notes?: AnnotationNote[];
  /** Entries in groups: a line between each. */
  groups: MenuEntry[][];
  onPick: (id: string) => void;
  onClose: () => void;
}

export interface ModalOptions {
  title: string;
  size?: 'sm' | 'md' | 'lg' | 'full';
  /** Draw the modal's content into its own shadow root; what it returns runs on close. */
  mount: (root: ShadowRoot) => void | (() => void);
  onClose: () => void;
}

/** Which side of its anchor a popover opens on. */
export type PopoverSide = 'right' | 'left' | 'bottom' | 'top';

export interface PopoverOptions {
  /** Where it points, now: read again on each scroll and resize; undefined once the anchor is gone. */
  anchor: () => Rect | undefined;
  /** The side it opens on when it fits there; else the opposite one, then any that fits. */
  side?: PopoverSide;
  mount: (root: ShadowRoot) => void | (() => void);
  /** `outside`: a click anywhere else closes it. Unless asked, only Escape and its close button do. */
  dismiss?: 'outside';
  /** false: no close button, for content that draws its own. */
  closeButton?: boolean;
  onClose: () => void;
}

/** One badge: on a query's marked element, for the annotations it doesn't draw. */
export interface Badge {
  /** Which query and element it is for (`app/query#n`). */
  id: string;
  /** The marked element: the badge sits inside its top-right corner. */
  element: Element;
  count: number;
  /** The highest severity of its annotations, if any has one. */
  severity?: AnnotationSeverity;
  /** What a screen reader says of it. */
  label: string;
  /** The reader clicked it: open its popover beside `anchor`. */
  onOpen: (anchor: () => Rect | undefined) => void;
}

/** An action on an annotation: one of its plugin's rows menu entries. */
export interface AnnotationAction {
  label: string;
  /** Why it can't run: shown, and it can't be picked. */
  disabled?: string;
  run?: () => unknown;
}

export interface UiBody {
  menu: (root: ShadowRoot, options: MenuOptions) => () => void;
  modal: (root: ShadowRoot, options: ModalOptions) => () => void;
  popover: (root: ShadowRoot, options: PopoverOptions) => () => void;
  /** Draw these badges in `root`, in place of those drawn before; none takes them all away. */
  badges: (root: ShadowRoot, badges: Badge[]) => void;
  /** Draw the badge's popover content: each annotation, and its actions; `onPick` runs the one picked. */
  annotationList: (root: ShadowRoot, items: (AnnotationNote & { actions: AnnotationAction[] })[], onPick: (action: AnnotationAction) => void) => void;
}

/** Each severity's colour, from the theme. */
const SEVERITY_COLORS = `
[data-severity="critical"] { --dot: var(--anfra-color-danger, oklch(0.501 0.17 27.2)); }
[data-severity="warning"] { --dot: var(--anfra-color-warning, oklch(0.501 0.1 70.3)); }
[data-severity="info"] { --dot: var(--anfra-color-accent, oklch(0.484 0.181 298.1)); }
.dot { display: inline-block; flex: none; width: 6px; height: 6px; border-radius: 50%; background: var(--dot, var(--anfra-color-muted, oklch(0.485 0.022 306.3))); }
`;

/** An annotation's lines, in a menu or a list: its dot, label and detail. */
const NOTE_STYLES = `
.note { display: flex; gap: 8px; align-items: baseline; padding: 4px; }
.note > div { min-width: 0; }
.label { font-weight: 500; }
.detail { display: block; color: var(--anfra-color-fg-weak, oklch(0.392 0.021 304.3)); }
.plugin { display: block; font-size: 11px; color: var(--anfra-color-muted, oklch(0.485 0.022 306.3)); }
`;

const STYLES = `
:host { all: initial; }
* { box-sizing: border-box; }
/* A part's container: over the whole viewport in the top layer, letting the pointer through to the page. */
.part { position: fixed; inset: 0; width: auto; height: auto; margin: 0; padding: 0; border: 0; overflow: visible; background: none; pointer-events: none; }
.surface, .backdrop { pointer-events: auto; }
.surface { position: fixed; z-index: 1; color: var(--anfra-color-fg, oklch(0.201 0.022 301.1)); background: var(--anfra-color-bg, oklch(1 0 0));
  border: 1px solid var(--anfra-color-border, oklch(0.201 0.022 301.1 / 0.12)); border-radius: calc(var(--anfra-radius, 10px) + 4px); /* a surface: the controls' radius plus 4px, anfra-design's xl */
  box-shadow: 0 8px 24px oklch(0 0 0 / 18%), 0 1px 3px oklch(0 0 0 / 10%); font: 13px/1.45 var(--anfra-font, system-ui, sans-serif); }
.menu { min-width: 180px; max-width: 320px; padding: 4px; outline: none; }
.menu button { display: block; width: 100%; padding: 6px 10px; border: 0; border-radius: var(--anfra-radius, 10px); background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; }
/* One highlight, as a native menu has: the focused entry, which the pointer and the arrows both move. */
.menu button:focus { outline: none; background: color-mix(in srgb, var(--anfra-color-accent, oklch(0.484 0.181 298.1)) 14%, transparent); }
.menu button[aria-disabled="true"] { color: var(--anfra-color-muted, oklch(0.485 0.022 306.3)); cursor: default; }
.menu .reason { display: block; font-size: 11px; color: var(--anfra-color-muted, oklch(0.485 0.022 306.3)); }
.menu hr { margin: 4px 0; border: 0; border-top: 1px solid var(--anfra-color-border, oklch(0.201 0.022 301.1 / 0.12)); }
.backdrop { position: fixed; inset: 0; background: oklch(0 0 0 / 40%); }
.modal { display: flex; flex-direction: column; left: 50%; top: 50%; transform: translate(-50%, -50%); max-width: calc(100vw - 32px); max-height: calc(100vh - 32px); }
.modal.sm { width: 360px; } .modal.md { width: 560px; } .modal.lg { width: 860px; }
.modal.full { width: calc(100vw - 32px); height: calc(100vh - 32px); }
.modal header { display: flex; align-items: center; gap: 8px; padding: 10px 12px 10px 16px; border-bottom: 1px solid var(--anfra-color-border, oklch(0.201 0.022 301.1 / 0.12)); }
.modal h2 { flex: 1; margin: 0; font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.close { display: grid; place-items: center; width: 26px; height: 26px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--anfra-color-muted, oklch(0.485 0.022 306.3)); cursor: pointer; font: 18px/1 var(--anfra-font, system-ui, sans-serif); }
.close:hover { color: var(--anfra-color-fg, oklch(0.201 0.022 301.1)); background: color-mix(in srgb, var(--anfra-color-fg, oklch(0.201 0.022 301.1)) 7%, transparent); }
.content { flex: 1; min-height: 0; overflow: auto; }
.popover { max-width: min(360px, calc(100vw - 16px)); outline: none; }
.popover > .close { position: absolute; top: 4px; right: 4px; width: 22px; height: 22px; font-size: 16px; }
.popover.closable > .content { padding-right: 26px; }

.notes { margin: 0; padding: 4px 6px; list-style: none; }
${SEVERITY_COLORS}
${NOTE_STYLES}
`;

const GAP = 6;
const MARGIN = 8;

function viewport (win: Window): { width: number, height: number } {
  return { width: win.innerWidth, height: win.innerHeight };
}

/** Keep a box of this size, wanted at (x, y), inside the viewport: flipped above or left of `flipFrom` when it would spill. */
function place (win: Window, size: { width: number, height: number }, x: number, y: number, flipFrom?: { left: number, top: number }): { left: number, top: number } {
  const { width, height } = viewport(win);
  let left = x;
  let top = y;
  if (left + size.width > width - MARGIN) left = flipFrom ? flipFrom.left - size.width : width - MARGIN - size.width;
  if (top + size.height > height - MARGIN) top = flipFrom ? flipFrom.top - size.height : height - MARGIN - size.height;
  return { left: Math.max(MARGIN, left), top: Math.max(MARGIN, top) };
}

const OPPOSITE: Record<PopoverSide, PopoverSide> = {
  right: 'left', left: 'right', bottom: 'top', top: 'bottom',
};

/**
 * Where a box goes beside an anchor: on `side` if it fits there, else the opposite side, else
 * whichever side does, else `side`; then shifted along the anchor to stay in the viewport.
 */
function beside (win: Window, size: { width: number, height: number }, anchor: Rect, side: PopoverSide): { left: number, top: number } {
  const { width, height } = viewport(win);
  const at = (where: PopoverSide) => {
    if (where === 'right') return { left: anchor.x + anchor.width + GAP, top: anchor.y };
    if (where === 'left') return { left: anchor.x - size.width - GAP, top: anchor.y };
    if (where === 'bottom') return { left: anchor.x, top: anchor.y + anchor.height + GAP };
    return { left: anchor.x, top: anchor.y - size.height - GAP };
  };
  const fits = (where: PopoverSide) => {
    const { left, top } = at(where);
    return where === 'right' || where === 'left'
      ? left >= MARGIN && left + size.width <= width - MARGIN
      : top >= MARGIN && top + size.height <= height - MARGIN;
  };
  const order: PopoverSide[] = [side, OPPOSITE[side], ...(['right', 'bottom', 'left', 'top'] as const).filter((where) => where !== side && where !== OPPOSITE[side])];
  const { left, top } = at(order.find(fits) ?? side);
  const clamp = (value: number, extent: number, room: number) => Math.max(MARGIN, Math.min(value, room - MARGIN - extent));
  return { left: clamp(left, size.width, width), top: clamp(top, size.height, height) };
}

/**
 * A part of the UI: its stylesheet and a container in the kernel's root, removed by the returned
 * function. The container is a manual popover, shown at once, so it sits in the top layer: above
 * the author's own dialogs and popovers, and placed against the viewport whatever the page
 * transforms. Without the Popover API it is the plugin layer's top z-index that lifts it.
 */
function part (root: ShadowRoot): { container: HTMLElement, remove: () => void } {
  const doc = root.ownerDocument;
  const container = doc.createElement('div');
  container.className = 'part';
  container.setAttribute('popover', 'manual');
  const style = doc.createElement('style');
  style.textContent = STYLES;
  container.append(style);
  root.append(container);
  container.showPopover?.();
  return { container, remove: () => container.remove() };
}

/** An element with its own shadow root for a plugin's content, apart from the chrome's styles. */
function contentRoot (doc: Document, className: string): { element: HTMLElement, root: ShadowRoot } {
  const element = doc.createElement('div');
  element.className = className;
  return { element, root: element.attachShadow({ mode: 'open' }) };
}

/** Run a plugin's mount, keeping what it returns; a failure is logged, not thrown at the reader. */
function mountContent (win: Window, mount: (root: ShadowRoot) => void | (() => void), root: ShadowRoot): () => void {
  try {
    const cleanup = mount(root);
    return typeof cleanup === 'function' ? cleanup : () => {};
  } catch (err) {
    (win as Window & typeof globalThis).console?.error('anfra: a plugin failed to draw:', err);
    return () => {};
  }
}

/** One annotation's item: its severity's dot, its label, its detail, and its plugin when asked. */
function noteItem (doc: Document, note: AnnotationNote, withPlugin: boolean): HTMLLIElement {
  const item = doc.createElement('li');
  item.className = 'note';
  if (note.severity) item.dataset.severity = note.severity;
  const dot = doc.createElement('span');
  dot.className = 'dot';
  if (note.severity) dot.title = note.severity;
  const text = doc.createElement('div');
  const label = doc.createElement('span');
  label.className = 'label';
  label.textContent = note.label;
  text.append(label);
  if (note.detail) {
    const detail = doc.createElement('span');
    detail.className = 'detail';
    detail.textContent = note.detail;
    text.append(detail);
  }
  if (withPlugin) {
    const plugin = doc.createElement('span');
    plugin.className = 'plugin';
    plugin.textContent = note.plugin;
    text.append(plugin);
  }
  item.append(dot, text);
  return item;
}

function menu (root: ShadowRoot, options: MenuOptions): () => void {
  const doc = root.ownerDocument;
  const win = doc.defaultView!;
  const { container, remove } = part(root);
  const list = doc.createElement('div');
  list.className = 'surface menu';
  list.setAttribute('role', 'menu');
  const buttons: HTMLButtonElement[] = [];
  if (options.notes?.length) {
    const notes = doc.createElement('ul');
    notes.className = 'notes';
    notes.setAttribute('aria-label', 'Annotations');
    options.notes.forEach((note) => notes.append(noteItem(doc, note, false)));
    list.append(notes);
  }
  options.groups.filter((group) => group.length).forEach((group, index) => {
    if (index > 0 || options.notes?.length) list.append(doc.createElement('hr'));
    for (const entry of group) {
      const button = doc.createElement('button');
      button.type = 'button';
      button.setAttribute('role', 'menuitem');
      button.dataset.item = entry.id;
      button.textContent = entry.label;
      if (entry.disabled) {
        button.setAttribute('aria-disabled', 'true');
        button.title = entry.disabled;
        const reason = doc.createElement('span');
        reason.className = 'reason';
        reason.textContent = entry.disabled;
        button.append(reason);
      }
      button.addEventListener('click', () => {
        if (entry.disabled) return;
        close();
        options.onPick(entry.id);
      });
      buttons.push(button);
      list.append(button);
    }
  });
  container.append(list);
  const rect = list.getBoundingClientRect();
  const { left, top } = place(win, rect, options.x, options.y, { left: options.x, top: options.y });
  list.style.left = `${left}px`;
  list.style.top = `${top}px`;
  // The menu has focus, so its keys work, with no entry highlighted until the pointer or an arrow
  // picks one; a disabled entry never takes the highlight.
  list.tabIndex = -1;
  list.focus();
  const enabled = buttons.filter((button) => !button.hasAttribute('aria-disabled'));
  list.addEventListener('pointermove', (event) => {
    const button = event.composedPath().find((target): target is HTMLButtonElement => buttons.includes(target as HTMLButtonElement));
    if (button && enabled.includes(button)) {
      if (root.activeElement !== button) button.focus();
    } else if (root.activeElement !== list) {
      list.focus();
    }
  });
  list.addEventListener('pointerleave', () => list.focus());

  const onKey = (event: KeyboardEvent) => {
    if (event.key === 'Escape') { event.preventDefault(); close(); return; }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    event.preventDefault();
    if (!enabled.length) return;
    const at = enabled.indexOf(root.activeElement as HTMLButtonElement);
    const step = event.key === 'ArrowDown' ? 1 : -1;
    const next = at === -1 ? (step === 1 ? 0 : enabled.length - 1) : (at + step + enabled.length) % enabled.length;
    enabled[next].focus();
  };
  // A press outside the menu, a scroll, a resize or the window losing focus: the menu goes, as a
  // native one does.
  const onOutside = (event: Event) => { if (!event.composedPath().includes(list)) close(); };
  const onAway = () => close();
  const listeners: [EventTarget, string, EventListener, boolean][] = [
    [doc, 'keydown', onKey as EventListener, true],
    [doc, 'pointerdown', onOutside, true],
    [win, 'scroll', onAway, true],
    [win, 'resize', onAway, false],
    [win, 'blur', onAway, false],
  ];
  listeners.forEach(([target, type, listener, capture]) => target.addEventListener(type, listener, capture));
  let closed = false;
  function close (): void {
    if (closed) return;
    closed = true;
    listeners.forEach(([target, type, listener, capture]) => target.removeEventListener(type, listener, capture));
    remove();
    options.onClose();
  }
  return close;
}

function modal (root: ShadowRoot, options: ModalOptions): () => void {
  const doc = root.ownerDocument;
  const win = doc.defaultView!;
  const { container, remove } = part(root);
  const backdrop = doc.createElement('div');
  backdrop.className = 'backdrop';
  const dialog = doc.createElement('section');
  dialog.className = `surface modal ${options.size ?? 'md'}`;
  dialog.setAttribute('role', 'dialog');
  dialog.setAttribute('aria-modal', 'true');
  const header = doc.createElement('header');
  const title = doc.createElement('h2');
  title.id = 'anfra-modal-title';
  title.textContent = options.title;
  dialog.setAttribute('aria-labelledby', title.id);
  const closeButton = doc.createElement('button');
  closeButton.type = 'button';
  closeButton.className = 'close';
  closeButton.setAttribute('aria-label', 'Close');
  closeButton.textContent = '×';
  closeButton.addEventListener('click', () => close());
  header.append(title, closeButton);
  const content = contentRoot(doc, 'content');
  dialog.append(header, content.element);
  container.append(backdrop, dialog);

  // The page under the modal can't be reached, by pointer, keyboard or scroll, while it is open:
  // everything in the body but the plugin layer is inert, and the page doesn't scroll.
  const layer = root.host.parentElement;
  const realm = win as Window & typeof globalThis;
  const inerted: Element[] = [];
  const inert = (elements: Iterable<Node>) => {
    for (const element of elements) {
      if (!(element instanceof realm.Element) || element === layer || element.hasAttribute('inert')) continue;
      element.setAttribute('inert', '');
      inerted.push(element);
    }
  };
  inert(doc.body.children);
  // What the page adds while the modal is open, a toast say, can't be reached either.
  const observer = new realm.MutationObserver((records) => records.forEach((record) => inert(record.addedNodes)));
  observer.observe(doc.body, { childList: true });
  // The page stops scrolling, and keeps its scrollbar's room so hiding the bar moves nothing.
  const html = doc.documentElement;
  const saved = { overflow: html.style.overflow, gutter: html.style.getPropertyValue('scrollbar-gutter') };
  if (win.innerWidth > html.clientWidth) html.style.setProperty('scrollbar-gutter', 'stable');
  html.style.overflow = 'hidden';
  const unmount = mountContent(win, options.mount, content.root);
  closeButton.focus();

  const onKey = (event: KeyboardEvent) => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  };
  doc.addEventListener('keydown', onKey, true);
  let closed = false;
  function close (): void {
    if (closed) return;
    closed = true;
    doc.removeEventListener('keydown', onKey, true);
    try { unmount(); } catch { /* it is going anyway */ }
    observer.disconnect();
    inerted.forEach((element) => element.removeAttribute('inert'));
    html.style.overflow = saved.overflow;
    if (saved.gutter) html.style.setProperty('scrollbar-gutter', saved.gutter);
    else html.style.removeProperty('scrollbar-gutter');
    remove();
    options.onClose();
  }
  return close;
}

function popover (root: ShadowRoot, options: PopoverOptions): () => void {
  const doc = root.ownerDocument;
  const win = doc.defaultView!;
  const { container, remove } = part(root);
  const box = doc.createElement('section');
  box.className = 'surface popover';
  box.setAttribute('role', 'dialog');
  // Focusable, and focused, so Escape reaches it at once, whatever had focus before.
  box.tabIndex = -1;
  const content = contentRoot(doc, 'content');
  box.append(content.element);
  if (options.closeButton !== false) {
    box.classList.add('closable');
    const closeButton = doc.createElement('button');
    closeButton.type = 'button';
    closeButton.className = 'close';
    closeButton.setAttribute('aria-label', 'Close');
    closeButton.textContent = '×';
    closeButton.addEventListener('click', () => close());
    box.append(closeButton);
  }
  container.append(box);
  const unmount = mountContent(win, options.mount, content.root);
  // Unless the content took focus itself (a text field, say).
  if (!content.root.activeElement) box.focus();

  // Beside the anchor, on its side (right unless asked) or the next that fits; following it as the
  // page scrolls, at most once a frame.
  let pending = false;
  const position = () => {
    pending = false;
    const anchor = options.anchor();
    if (!anchor) { close(); return; }
    const size = box.getBoundingClientRect();
    const { left, top } = beside(win, size, anchor, options.side ?? 'right');
    box.style.left = `${left}px`;
    box.style.top = `${top}px`;
  };
  const schedule = () => {
    if (pending) return;
    pending = true;
    win.requestAnimationFrame(position);
  };
  // Before the first placing, which closes it when its anchor is already gone.
  let closed = false;
  position();
  if (closed) return close;

  const onKey = (event: KeyboardEvent) => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  };
  const onOutside = (event: Event) => { if (!event.composedPath().includes(box)) close(); };
  doc.addEventListener('keydown', onKey, true);
  win.addEventListener('scroll', schedule, true);
  win.addEventListener('resize', schedule);
  if (options.dismiss === 'outside') doc.addEventListener('pointerdown', onOutside, true);
  function close (): void {
    if (closed) return;
    closed = true;
    doc.removeEventListener('keydown', onKey, true);
    win.removeEventListener('scroll', schedule, true);
    win.removeEventListener('resize', schedule);
    doc.removeEventListener('pointerdown', onOutside, true);
    try { unmount(); } catch { /* it is going anyway */ }
    remove();
    options.onClose();
  }
  return close;
}

const BADGE_STYLES = `
:host { all: initial; }
.badges { position: fixed; inset: 0; pointer-events: none; }
.badge { position: fixed; display: inline-flex; align-items: center; gap: 4px; height: 20px; padding: 0 6px; pointer-events: auto; cursor: pointer;
  color: var(--anfra-color-fg, oklch(0.201 0.022 301.1)); background: var(--anfra-color-bg, oklch(1 0 0));
  border: 1px solid var(--anfra-color-border-strong, oklch(0.201 0.022 301.1 / 0.24)); border-radius: 9999px;
  font: 500 11px/1 var(--anfra-font, system-ui, sans-serif); font-variant-numeric: tabular-nums; }
.badge:hover { background: var(--anfra-color-hover, oklch(0.201 0.022 301.1 / 0.05)); }
.badge:focus-visible { outline: 2px solid var(--anfra-color-accent, oklch(0.484 0.181 298.1)); outline-offset: 1px; }
.badge[hidden] { display: none; }
${SEVERITY_COLORS}
`;

/** How far a badge sits inside its element's top-right corner. */
const INSET = 6;

/** Each badges root's listeners and what it shows, so a redraw replaces them. */
const drawn = new WeakMap<ShadowRoot, () => void>();

function badges (root: ShadowRoot, list: Badge[]): void {
  drawn.get(root)?.();
  drawn.delete(root);
  root.replaceChildren();
  if (!list.length) return;
  const doc = root.ownerDocument;
  const win = doc.defaultView!;
  const realm = win as Window & typeof globalThis;
  const style = doc.createElement('style');
  style.textContent = BADGE_STYLES;
  const container = doc.createElement('div');
  container.className = 'badges';
  const placed = list.map((badge) => {
    const button = doc.createElement('button');
    button.type = 'button';
    button.className = 'badge';
    button.dataset.for = badge.id;
    if (badge.severity) button.dataset.severity = badge.severity;
    button.setAttribute('aria-label', badge.label);
    button.title = badge.label;
    const dot = doc.createElement('span');
    dot.className = 'dot';
    dot.setAttribute('aria-hidden', 'true');
    button.append(dot, String(badge.count));
    const anchor = (): Rect | undefined => {
      if (!button.isConnected) return undefined;
      const { x, y, width, height } = button.getBoundingClientRect();
      return { x, y, width, height };
    };
    button.addEventListener('click', () => badge.onOpen(anchor));
    container.append(button);
    return { badge, button };
  });
  root.append(style, container);

  // Inside each element's top-right corner, following it as the page scrolls or it resizes, at most
  // once a frame. Hidden while the element is gone or has no box.
  let pending = false;
  const position = () => {
    pending = false;
    for (const { badge, button } of placed) {
      const rect = badge.element.isConnected ? badge.element.getBoundingClientRect() : undefined;
      button.hidden = !rect || (rect.width === 0 && rect.height === 0);
      if (!rect || button.hidden) continue;
      const width = button.getBoundingClientRect().width;
      button.style.left = `${rect.x + rect.width - width - INSET}px`;
      button.style.top = `${rect.y + INSET}px`;
    }
  };
  const schedule = () => {
    if (pending) return;
    pending = true;
    win.requestAnimationFrame(position);
  };
  position();
  win.addEventListener('scroll', schedule, true);
  win.addEventListener('resize', schedule);
  const observer = typeof realm.ResizeObserver === 'function' ? new realm.ResizeObserver(schedule) : undefined;
  placed.forEach(({ badge }) => observer?.observe(badge.element));
  drawn.set(root, () => {
    win.removeEventListener('scroll', schedule, true);
    win.removeEventListener('resize', schedule);
    observer?.disconnect();
  });
}

const LIST_STYLES = `
:host { display: block; font: 13px/1.45 var(--anfra-font, system-ui, sans-serif); color: var(--anfra-color-fg, oklch(0.201 0.022 301.1)); }
ul { margin: 0; padding: 8px; list-style: none; max-height: 60vh; overflow: auto; }
li + li { border-top: 1px solid var(--anfra-color-border, oklch(0.201 0.022 301.1 / 0.12)); }
.note { padding: 6px 4px; }
.actions { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.actions button { padding: 2px 8px; border: 1px solid var(--anfra-color-border, oklch(0.201 0.022 301.1 / 0.12)); border-radius: var(--anfra-radius, 10px);
  background: none; color: inherit; font: inherit; font-size: 12px; cursor: pointer; }
.actions button:hover { background: var(--anfra-color-hover, oklch(0.201 0.022 301.1 / 0.05)); }
.actions button[aria-disabled="true"] { color: var(--anfra-color-muted, oklch(0.485 0.022 306.3)); cursor: default; }
${NOTE_STYLES}
${SEVERITY_COLORS}
`;

function annotationList (root: ShadowRoot, items: (AnnotationNote & { actions: AnnotationAction[] })[], onPick: (action: AnnotationAction) => void): void {
  const doc = root.ownerDocument;
  const style = doc.createElement('style');
  style.textContent = LIST_STYLES;
  const list = doc.createElement('ul');
  list.setAttribute('aria-label', 'Annotations');
  for (const entry of items) {
    const item = noteItem(doc, entry, true);
    if (entry.actions.length) {
      const actions = doc.createElement('div');
      actions.className = 'actions';
      for (const action of entry.actions) {
        const button = doc.createElement('button');
        button.type = 'button';
        button.textContent = action.label;
        if (action.disabled) {
          button.setAttribute('aria-disabled', 'true');
          button.title = action.disabled;
        }
        button.addEventListener('click', () => { if (!action.disabled) onPick(action); });
        actions.append(button);
      }
      item.lastElementChild!.append(actions);
    }
    list.append(item);
  }
  root.replaceChildren(style, list);
}

const body: UiBody = {
  menu, modal, popover, badges, annotationList,
};
export default body;
