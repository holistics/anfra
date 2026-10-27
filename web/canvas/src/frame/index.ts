// What runs in each frame, before the app: the Vite plugin puts it at the top of the app's page in
// dev. In a page that isn't a canvas frame it does nothing, so the app runs as always. In a frame,
// it answers the app's API calls from the frame's scenario, and gives the canvas an agent that
// picks, locates and edits elements.
import type { ElementRef } from '../model/board.ts';
import { answer, opOf, scenarioOps, type Scenarios } from '../model/scenario.ts';
import {
  COMPONENT_ATTR, SOURCE_ATTR, parseFrameName,
  type CanvasHost, type ElementInfo, type FrameAgent, type FrameEvent, type FrameMode,
} from './protocol.ts';

export interface FrameOptions {
  scenarios: Scenarios;
  /** Where the API is: requests below it are answered by the scenario. */
  apiBase?: string;
  /** Files whose elements are parts of a component, not what a person means to pick. */
  libraryDirs?: string[];
}

/** The scenario name that turns the mocks off: the frame talks to the real backend. */
export const LIVE = 'live';

export function installFrame (options: FrameOptions, win: Window = window): FrameAgent | undefined {
  quietTags(win);
  const named = parseFrameName(win.name);
  if (!named || win.parent === win) return undefined;
  if (named.scenario !== LIVE) installMocks(win, options.scenarios, named.scenario ?? 'default', options.apiBase ?? '/api');
  const agent = createAgent(win, named.id, options.libraryDirs ?? ['src/components/ui/']);
  return agent;
}

/**
 * A component whose root is a fragment can't take the source tags it is given, and Vue warns about
 * each one. Those warnings are about the canvas, not the app, so they are dropped; a warning that
 * names any other attribute is kept.
 */
function quietTags (win: Window): void {
  const { console } = win as Window & typeof globalThis;
  const warn = console.warn.bind(console);
  console.warn = (...args: unknown[]) => {
    const m = typeof args[0] === 'string' ? /Extraneous non-props attributes \(([^)]*)\)/.exec(args[0]) : null;
    if (m && m[1].split(',').every((a) => a.trim().startsWith('data-canvas-'))) return;
    warn(...args);
  };
}

// --- The API, from the scenario -------------------------------------------------------------------

function installMocks (win: Window, scenarios: Scenarios, scenario: string, base: string): void {
  const ops = scenarioOps(scenarios, scenario);
  const real = win.fetch.bind(win);
  win.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : undefined;
    const url = new URL(req ? req.url : String(input), win.location.href);
    const addressed = url.origin === win.location.origin ? opOf(url.pathname, base) : undefined;
    if (!addressed) return real(input, init);
    const text = init?.body != null ? String(init.body) : req ? await req.clone().text() : '';
    let body: unknown = {};
    try {
      body = text ? JSON.parse(text) : {};
    } catch { /* not JSON: the mock gets {} */ }
    const res = await answer(ops, { ...addressed, body });
    if (res.$status === 501) console.warn(`[anfra canvas] ${scenario}: no mock for ${addressed.op}`);
    if (res.$delay) await new Promise((r) => setTimeout(r, res.$delay));
    return new Response(res.$status === 204 ? null : JSON.stringify(res.$body ?? {}), {
      status: res.$status,
      headers: { 'Content-Type': 'application/json' },
    });
  };
}

// --- The agent -------------------------------------------------------------------------------------

function createAgent (win: Window, id: string, libraryDirs: string[]): FrameAgent {
  const doc = win.document;
  let mode: FrameMode = 'idle';
  let hovered: Element | undefined;
  let editing = false;

  const host = (): CanvasHost | undefined => {
    try {
      return win.parent.__anfraCanvasHost;
    } catch {
      return undefined;
    }
  };
  const emit = (event: FrameEvent) => host()?.event(id, event);

  const inLibrary = (el: Element) => {
    const src = el.getAttribute(SOURCE_ATTR);
    return !src || libraryDirs.some((d) => src.startsWith(d));
  };

  /** The element a person means: the nearest one written in the app, not inside a component. */
  const pickable = (target: EventTarget | null): Element | undefined => {
    let el = target instanceof Element ? target : target instanceof Node ? target.parentElement : null;
    while (el && el !== doc.body && el !== doc.documentElement && inLibrary(el)) el = el.parentElement;
    return el && el !== doc.body && el !== doc.documentElement ? el : undefined;
  };

  const describe = (el: Element): ElementInfo => {
    const r = el.getBoundingClientRect();
    const source = el.getAttribute(SOURCE_ATTR) ?? undefined;
    const same = source ? [...doc.querySelectorAll(`[${SOURCE_ATTR}="${quoted(source)}"]`)] : [];
    const index = same.length > 1 ? same.indexOf(el) : undefined;
    const cs = win.getComputedStyle(el);
    const text = (el as HTMLElement).innerText ?? el.textContent ?? '';
    const ref: ElementRef = {
      frame: id,
      tag: el.tagName.toLowerCase(),
      ...(source ? { source } : {}),
      ...(index !== undefined && index > 0 ? { index } : {}),
      ...(el.getAttribute(COMPONENT_ATTR) ? { component: el.getAttribute(COMPONENT_ATTR)! } : {}),
      ...(text.trim() ? { text: text.trim().replace(/\s+/g, ' ').slice(0, 120) } : {}),
      selector: selectorOf(el),
    };
    return {
      ref,
      rect: { x: r.x, y: r.y, width: r.width, height: r.height },
      textOnly: el.childNodes.length > 0 && [...el.childNodes].every((n) => n.nodeType === Node.TEXT_NODE) && !!text.trim(),
      classes: typeof el.className === 'string' ? el.className : '',
      style: {
        width: `${Math.round(r.width)}`,
        height: `${Math.round(r.height)}`,
        font: `${cs.fontSize} / ${cs.lineHeight}, ${cs.fontWeight}`,
        color: cs.color,
        background: cs.backgroundColor,
        padding: cs.padding,
        radius: cs.borderRadius,
      },
    };
  };

  const resolve = (ref: ElementRef): Element | undefined => {
    if (ref.source) {
      const all = doc.querySelectorAll(`[${SOURCE_ATTR}="${quoted(ref.source)}"]`);
      const el = all[ref.index ?? 0] ?? all[0];
      if (el) return el;
    }
    if (ref.selector) {
      try {
        return doc.querySelector(ref.selector) ?? undefined;
      } catch { /* a selector from an older screen */ }
    }
    return undefined;
  };

  const picking = () => (mode === 'inspect' || mode === 'comment') && !editing;

  const onMove = (e: PointerEvent) => {
    if (!picking()) return;
    const el = pickable(e.target);
    if (el === hovered) return;
    hovered = el;
    emit({ type: 'hover', info: el ? describe(el) : undefined });
  };
  const onLeave = () => {
    hovered = undefined;
    emit({ type: 'hover' });
  };
  const swallow = (e: Event) => {
    if (!picking()) return;
    e.preventDefault();
    e.stopPropagation();
  };
  const onClick = (e: MouseEvent) => {
    if (!picking()) return;
    swallow(e);
    const el = pickable(e.target);
    if (el) emit({ type: 'pick', info: describe(el), shift: e.shiftKey, point: { x: e.clientX, y: e.clientY } });
  };
  const onDouble = (e: MouseEvent) => {
    if (mode !== 'inspect' || editing) return;
    swallow(e);
    const el = pickable(e.target);
    if (el) emit({ type: 'activate', info: describe(el) });
  };
  const onWheel = (e: WheelEvent) => {
    if (mode === 'interact') return;
    e.preventDefault();
    emit({ type: 'wheel', deltaX: e.deltaX, deltaY: e.deltaY, zoom: e.ctrlKey || e.metaKey, x: e.clientX, y: e.clientY });
  };
  const onKey = (e: KeyboardEvent) => {
    if (mode === 'interact' || editing) return;
    const field = e.target instanceof HTMLElement && (e.target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName));
    if (field) return;
    e.preventDefault();
    emit({ type: 'key', key: e.key, meta: e.metaKey || e.ctrlKey, shift: e.shiftKey });
  };

  win.addEventListener('pointermove', onMove, true);
  doc.addEventListener('pointerleave', onLeave);
  for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'submit']) win.addEventListener(type, swallow, true);
  win.addEventListener('click', onClick, true);
  win.addEventListener('dblclick', onDouble, true);
  win.addEventListener('wheel', onWheel, { capture: true, passive: false });
  win.addEventListener('keydown', onKey, true);

  // The canvas redraws its outlines when the screen moves under them.
  let queued = false;
  const layout = () => {
    if (queued) return;
    queued = true;
    win.requestAnimationFrame(() => {
      queued = false;
      emit({ type: 'layout' });
    });
  };
  win.addEventListener('scroll', layout, true);
  win.addEventListener('resize', layout);
  new MutationObserver(layout).observe(doc, { subtree: true, childList: true, attributes: true, characterData: true });

  // The route, as the app navigates.
  const route = () => win.location.pathname + win.location.search.replace(/[?&]?__[a-z]+=[^&]*/g, '') + win.location.hash;
  let last = route();
  const routed = () => {
    const now = route();
    if (now === last) return;
    last = now;
    emit({ type: 'route', route: now });
  };
  for (const method of ['pushState', 'replaceState'] as const) {
    const original = win.history[method].bind(win.history);
    win.history[method] = (...args: Parameters<History['pushState']>) => {
      original(...args);
      routed();
    };
  }
  win.addEventListener('popstate', routed);

  const agent: FrameAgent = {
    setMode (next) {
      mode = next;
      if (!picking() && hovered) onLeave();
    },
    locate (ref) {
      const el = resolve(ref);
      return el ? describe(el) : undefined;
    },
    pickAt (x, y) {
      const el = pickable(doc.elementFromPoint(x, y));
      return el ? describe(el) : undefined;
    },
    editText (ref) {
      const el = resolve(ref) as HTMLElement | undefined;
      if (!el) return Promise.resolve(undefined);
      editing = true;
      const from = el.textContent ?? '';
      el.contentEditable = 'plaintext-only';
      el.focus();
      const range = doc.createRange();
      range.selectNodeContents(el);
      win.getSelection()?.removeAllRanges();
      win.getSelection()?.addRange(range);
      return new Promise((done) => {
        const finish = (keep: boolean) => {
          el.removeEventListener('keydown', onEditKey, true);
          el.removeEventListener('blur', onBlur);
          el.removeAttribute('contenteditable');
          editing = false;
          const to = el.textContent ?? '';
          if (!keep) el.textContent = from;
          done(keep && to.trim() !== from.trim() ? { from: from.trim(), to: to.trim() } : undefined);
        };
        const onEditKey = (e: KeyboardEvent) => {
          e.stopPropagation();
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            finish(true);
          } else if (e.key === 'Escape') {
            e.preventDefault();
            finish(false);
          }
        };
        const onBlur = () => finish(true);
        el.addEventListener('keydown', onEditKey, true);
        el.addEventListener('blur', onBlur);
      });
    },
    route,
  };

  const attach = () => host()?.attach(id, agent);
  attach();
  win.addEventListener('pagehide', () => host()?.detach(id, agent));
  return agent;
}

/** A value for a double-quoted attribute selector. */
const quoted = (value: string) => value.replace(/["\\]/g, '\\$&');

/** A short CSS path to an element, used when it has no source (markup the app built at run time). */
function selectorOf (el: Element): string {
  const parts: string[] = [];
  let at: Element | null = el;
  while (at && at.tagName !== 'BODY' && parts.length < 5) {
    if (at.id) {
      parts.unshift(`#${typeof CSS !== 'undefined' ? CSS.escape(at.id) : at.id.replace(/[^\w-]/g, '\\$&')}`);
      break;
    }
    const tag = at.tagName.toLowerCase();
    const parent: Element | null = at.parentElement;
    const siblings = parent ? [...parent.children].filter((c) => c.tagName === at!.tagName) : [];
    parts.unshift(siblings.length > 1 ? `${tag}:nth-of-type(${siblings.indexOf(at) + 1})` : tag);
    at = parent;
  }
  return parts.join(' > ');
}
