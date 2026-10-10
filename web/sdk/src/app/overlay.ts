/**
 * The overlay: how a frame shows a node on its page, for the host's inspect panel and for the
 * definition's own `locate()`. A tinted box over each marked element and a label naming the node,
 * drawn above everything and ignoring the pointer. In the frame's document because the host cannot
 * reach it; the only UI the SDK draws, and only while someone is inspecting.
 */
export const OVERLAY_ID = 'anfra-overlay';

/** How long a flash (the author's `locate()`) stays. */
export const FLASH_MS = 1500;

const BOX_STYLE = 'position:fixed;box-sizing:border-box;background:rgba(37,99,235,0.22);outline:1px solid #2563eb;outline-offset:-1px;';
const LABEL_STYLE = 'position:fixed;max-width:60vw;display:flex;gap:6px;align-items:baseline;padding:3px 8px;border-radius:4px;'
  + 'background:#0f172a;color:#f8fafc;font:12px/1.4 system-ui,sans-serif;white-space:nowrap;box-shadow:0 2px 8px rgba(0,0,0,0.35);';
const PART_STYLE: Record<keyof OverlayLabel, string> = {
  kind: 'color:#94a3b8;',
  name: 'color:#93c5fd;font-weight:700;',
  label: 'color:#f8fafc;',
  note: 'color:#94a3b8;',
  size: 'color:#94a3b8;margin-left:10px;',
};

/**
 * What the label says, in parts the overlay sets apart: the kind, the id or name, the author's
 * label, a note such as "1 of 3", and the element's size.
 */
export interface OverlayLabel {
  kind: string;
  name: string;
  label?: string;
  note?: string;
  size?: string;
}

export class Overlay {
  private readonly root: HTMLElement;

  private shown: Element[] = [];

  private text: OverlayLabel = { kind: '', name: '' };

  private timer: ReturnType<typeof setTimeout> | undefined;

  private readonly reposition = () => this.draw();

  constructor (private readonly doc: Document) {
    const existing = doc.getElementById(OVERLAY_ID);
    if (existing) {
      this.root = existing;
    } else {
      this.root = doc.createElement('div');
      this.root.id = OVERLAY_ID;
      this.root.setAttribute('aria-hidden', 'true');
      this.root.setAttribute('style', 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;display:none;');
      (doc.body ?? doc.documentElement).appendChild(this.root);
    }
  }

  /** Show the overlay on these elements, labelled, until `hide()` or the next `show()`. */
  show (elements: readonly Element[], text: OverlayLabel): void {
    this.clearTimer();
    this.shown = [...elements];
    this.text = text;
    if (!this.shown.length) { this.hide(); return; }
    this.root.style.display = 'block';
    this.draw();
    this.doc.addEventListener('scroll', this.reposition, { capture: true, passive: true });
    this.doc.defaultView?.addEventListener('resize', this.reposition);
  }

  /** Show for a moment, then hide. */
  flash (elements: readonly Element[], text: OverlayLabel, ms = FLASH_MS): void {
    this.show(elements, text);
    this.timer = setTimeout(() => this.hide(), ms);
  }

  hide (): void {
    this.clearTimer();
    this.shown = [];
    this.root.style.display = 'none';
    this.root.replaceChildren();
    this.doc.removeEventListener('scroll', this.reposition, { capture: true });
    this.doc.defaultView?.removeEventListener('resize', this.reposition);
  }

  get visible (): boolean {
    return this.shown.length > 0;
  }

  private clearTimer (): void {
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = undefined;
  }

  private draw (): void {
    const boxes = this.shown.map((element) => {
      const r = element.getBoundingClientRect();
      const box = this.doc.createElement('div');
      box.setAttribute('style', `${BOX_STYLE}left:${r.left}px;top:${r.top}px;width:${r.width}px;height:${r.height}px;`);
      return box;
    });
    const first = this.shown[0].getBoundingClientRect();
    const label = this.doc.createElement('div');
    label.setAttribute('data-testid', 'anfra-overlay-label');
    const viewportHeight = this.doc.defaultView?.innerHeight ?? 0;
    // Above the element when there is room, else just inside its top edge.
    const top = first.top >= 26 ? first.top - 24 : Math.max(4, first.top + 4);
    const left = Math.max(4, Math.min(first.left, (this.doc.defaultView?.innerWidth ?? 0) - 24));
    label.setAttribute('style', `${LABEL_STYLE}left:${left}px;top:${Math.min(top, viewportHeight - 24)}px;`);
    for (const part of ['kind', 'name', 'label', 'note', 'size'] as const) {
      const value = this.text[part];
      if (!value) continue;
      const span = this.doc.createElement('span');
      span.setAttribute('style', PART_STYLE[part]);
      span.setAttribute('data-part', part);
      span.textContent = value;
      label.appendChild(span);
    }
    this.root.replaceChildren(...boxes, label);
  }
}

const overlays = new WeakMap<Document, Overlay>();

/** The one overlay of a document. */
export function overlayFor (doc: Document): Overlay {
  let overlay = overlays.get(doc);
  if (!overlay) {
    overlay = new Overlay(doc);
    overlays.set(doc, overlay);
  }
  return overlay;
}

/** Scroll the first element into view, if the environment can. */
export function scrollTo (element: Element): void {
  if (typeof element.scrollIntoView === 'function') {
    element.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
  }
}
