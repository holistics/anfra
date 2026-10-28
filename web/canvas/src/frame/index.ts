// What runs in each frame, before the app: the Vite plugin puts it at the top of the app's page in
// dev. In a page that isn't a canvas frame it does nothing, so the app runs as always. In a frame,
// it answers the app's API calls from the frame's scenario, and gives the canvas an agent that
// picks, locates and edits elements.
import type { ElementRef, Rect } from '../model/board.ts';
import { snapshot } from './snapshot.ts';
import { answer, opOf, scenarioOps, type Scenarios } from '../model/scenario.ts';
import {
  COMPONENT_ATTR, SOURCE_ATTR, parseFrameName,
  type CanvasHost, type ElementInfo, type FrameAgent, type FrameEvent, type FrameMode, type LayerNode,
} from './protocol.ts';

/** Elements that can't hold others, and ones that hold text, not boxes: a drop goes beside them. */
const LEAF = /^(INPUT|IMG|BR|HR|TEXTAREA|SELECT|SVG|PATH|BUTTON|A|LABEL|SPAN|P|H[1-6]|CODE|STRONG|EM|SMALL)$/i;

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
    const start = target instanceof Element ? target : target instanceof Node ? target.parentElement : null;
    let el = start;
    while (el && el !== doc.body && el !== doc.documentElement && inLibrary(el)) el = el.parentElement;
    if (el && el !== doc.body && el !== doc.documentElement) return el;
    return start ? overlayOf(start) : undefined;
  };

  /**
   * What a component teleported to the body draws (a modal's dialog, a popover's box, a tooltip)
   * has no element written in the app above it, so it stands for the component that drew it, found
   * on Vue's instances: a pick on a modal's title picks the modal where the screen uses it. The
   * element it stands as is the overlay's own box, not a backdrop that covers the whole screen.
   */
  const overlays = new Map<Element, { source: string, component?: string }>();
  const overlayOf = (start: Element): Element | undefined => {
    let inst = (start as Element & { __vueParentComponent?: VueInstance }).__vueParentComponent;
    while (inst) {
      const src = inst.vnode?.props?.[SOURCE_ATTR];
      if (typeof src === 'string' && !libraryDirs.some((d) => src.startsWith(d))) {
        let box = start;
        for (let a = start.parentElement; a && a !== doc.body && a !== doc.documentElement; a = a.parentElement) {
          const r = a.getBoundingClientRect();
          if (r.width >= win.innerWidth - 1 && r.height >= win.innerHeight - 1) break;
          box = a;
        }
        const cmp = inst.vnode?.props?.[COMPONENT_ATTR];
        for (const e of overlays.keys()) if (!e.isConnected) overlays.delete(e);
        overlays.set(box, { source: src, component: typeof cmp === 'string' ? cmp : undefined });
        return box;
      }
      inst = inst.parent ?? undefined;
    }
    return undefined;
  };

  /**
   * Every place that draws this element, innermost first. One element can stand for several
   * components: `<Button as-child><RouterLink>` draws one <a>, whose tag says RouterLink; the
   * Button is found on the component instances whose root it is.
   */
  const placesOf = (el: Element): { source: string, component?: string }[] => {
    const overlay = overlays.get(el);
    if (overlay) return [overlay];
    const own = el.getAttribute(SOURCE_ATTR);
    const out = own ? [{ source: own, component: el.getAttribute(COMPONENT_ATTR) ?? undefined }] : [];
    let inst = (el as Element & { __vueParentComponent?: VueInstance }).__vueParentComponent;
    while (inst && inst.vnode?.el === el) {
      const src = inst.vnode.props?.[SOURCE_ATTR];
      const cmp = inst.vnode.props?.[COMPONENT_ATTR];
      if (typeof src === 'string' && !out.some((p) => p.source === src) && !libraryDirs.some((d) => src.startsWith(d))) {
        out.push({ source: src, component: typeof cmp === 'string' ? cmp : undefined });
      }
      inst = inst.parent ?? undefined;
    }
    return out;
  };

  /** The elements a place draws: tagged with it, or drawn by a component used there. */
  const drawnBy = (source: string): Element[] => {
    const tagged = [...doc.querySelectorAll(`[${SOURCE_ATTR}="${quoted(source)}"]`)];
    if (tagged.length) return tagged;
    const drawn = [...doc.querySelectorAll(`[${SOURCE_ATTR}]`)].filter((e) => placesOf(e).some((p) => p.source === source));
    if (drawn.length) return drawn;
    return [...overlays].filter(([e, p]) => e.isConnected && p.source === source).map(([e]) => e);
  };

  const refOf = (el: Element, place = overlays.get(el)): ElementRef => {
    const source = place?.source ?? el.getAttribute(SOURCE_ATTR) ?? undefined;
    const component = place ? place.component : el.getAttribute(COMPONENT_ATTR) ?? undefined;
    const same = source ? drawnBy(source) : [];
    const index = same.length > 1 ? same.indexOf(el) : undefined;
    const text = (el as HTMLElement).innerText ?? el.textContent ?? '';
    return {
      frame: id,
      tag: el.tagName.toLowerCase(),
      ...(source ? { source } : {}),
      ...(index !== undefined && index > 0 ? { index } : {}),
      ...(component ? { component } : {}),
      ...(text.trim() ? { text: text.trim().replace(/\s+/g, ' ').slice(0, 120) } : {}),
      selector: selectorOf(el),
    };
  };

  const describe = (el: Element, place?: { source: string, component?: string }): ElementInfo => {
    const r = el.getBoundingClientRect();
    const cs = win.getComputedStyle(el);
    const text = (el as HTMLElement).innerText ?? el.textContent ?? '';
    const ref = refOf(el, place);
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
      container: !LEAF.test(el.tagName) && !ref.component && (el.children.length > 0 || !text.trim()),
    };
  };

  const tree = (): LayerNode[] => {
    let budget = 3000;
    const nodes = (parent: Element): LayerNode[] => [...parent.children].flatMap((c): LayerNode[] => {
      if (budget-- <= 0 || /^(SCRIPT|STYLE|TEMPLATE|NOSCRIPT)$/.test(c.tagName)) return [];
      const children = nodes(c);
      if (!c.hasAttribute(SOURCE_ATTR) || inLibrary(c)) return children;
      const own = [...c.childNodes].filter((n) => n.nodeType === Node.TEXT_NODE).map((n) => n.textContent ?? '').join(' ').trim().replace(/\s+/g, ' ');
      const component = c.getAttribute(COMPONENT_ATTR);
      const text = own || (component && !children.length ? ((c as HTMLElement).innerText ?? '').trim().replace(/\s+/g, ' ') : '');
      return [{
        ref: refOf(c),
        label: component ?? c.tagName.toLowerCase(),
        ...(text ? { text: text.slice(0, 48) } : {}),
        component: !!component,
        children,
      }];
    });
    return nodes(doc.body);
  };

  const resolve = (ref: ElementRef): Element | undefined => {
    if (ref.source) {
      const all = drawnBy(ref.source);
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
  // A right click opens the canvas's menu for the element under it, not the browser's.
  const onMenu = (e: MouseEvent) => {
    if (!picking()) return;
    swallow(e);
    const el = pickable(e.target);
    if (el) emit({ type: 'menu', info: describe(el), point: { x: e.clientX, y: e.clientY } });
  };
  const onWheel = (e: WheelEvent) => {
    if (mode === 'interact') return;
    e.preventDefault();
    emit({ type: 'wheel', deltaX: e.deltaX, deltaY: e.deltaY, zoom: e.ctrlKey || e.metaKey, x: e.clientX, y: e.clientY });
  };
  // Using the app, it keeps its keys; only an Escape it didn't use goes to the canvas (to stop
  // playing a flow). That one is heard after the app's own handlers, in the bubbling phase.
  const onUnusedEscape = (e: KeyboardEvent) => {
    if (mode === 'interact' && e.key === 'Escape' && !e.defaultPrevented) emit({ type: 'key', key: e.key, meta: false, shift: false });
  };
  win.addEventListener('keydown', onUnusedEscape);
  const onKey = (e: KeyboardEvent) => {
    if (mode === 'interact' || editing) return;
    const field = e.target instanceof HTMLElement && (e.target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName));
    if (field) return;
    e.preventDefault();
    emit({ type: 'key', key: e.key, meta: e.metaKey || e.ctrlKey, shift: e.shiftKey, alt: e.altKey });
  };

  win.addEventListener('pointermove', onMove, true);
  doc.addEventListener('pointerleave', onLeave);
  for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'submit']) win.addEventListener(type, swallow, true);
  win.addEventListener('click', onClick, true);
  win.addEventListener('dblclick', onDouble, true);
  win.addEventListener('contextmenu', onMenu, true);
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
    parent (ref) {
      const el = resolve(ref);
      if (!el) return undefined;
      // A component drawing the same element comes first (the Button around a RouterLink).
      const places = placesOf(el);
      const outer = places[places.findIndex((p) => p.source === ref.source) + 1];
      if (outer) return describe(el, outer);
      const own = new Set(places.map((p) => p.source));
      let at = el.parentElement;
      while (at && at !== doc.body && (inLibrary(at) || own.has(at.getAttribute(SOURCE_ATTR) ?? ''))) at = at.parentElement;
      return at && at !== doc.body ? describe(at) : undefined;
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
    tree,
    snapshot: () => snapshot(doc),
    rects (sources) {
      const out: Rect[] = [];
      for (const el of new Set(sources.flatMap(drawnBy))) {
        const r = el.getBoundingClientRect();
        if (r.width && r.height) out.push({ x: r.x, y: r.y, width: r.width, height: r.height });
      }
      return out;
    },
    shows: (file) => !!doc.querySelector(`[${SOURCE_ATTR}^="${quoted(`${file}:`)}"]`),
  };

  const attach = () => host()?.attach(id, agent);
  attach();
  win.addEventListener('pagehide', () => host()?.detach(id, agent));
  return agent;
}

/** What the agent reads of a Vue component instance: its vnode's props, its element, its parent. */
interface VueInstance {
  vnode?: { el?: unknown, props?: Record<string, unknown> | null };
  parent?: VueInstance | null;
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
