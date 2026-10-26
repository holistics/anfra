import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import { flush, testFrame } from './testFrame';
import type { FrameKernel } from './kernel';
import type { PluginSetup } from './bootstrap';
import ui from './ui/body';

// Each test's frame listens on the test's one window: take its listeners away after it, as a
// frame's go with its document.
let added: [string, EventListenerOrEventListenerObject][] = [];
beforeEach(() => {
  const add = window.addEventListener.bind(window);
  vi.spyOn(window, 'addEventListener').mockImplementation((type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions) => {
    added.push([type, listener]);
    add(type, listener, options);
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  added.forEach(([type, listener]) => window.removeEventListener(type, listener));
  added = [];
  document.body.innerHTML = '';
  document.documentElement.style.overflow = '';
});

/** A frame with a block, a plugin whose setup the test gives, and the kernel's UI body. */
function frame (setup?: (kernel: FrameKernel) => void, answers: Record<string, (request: any) => unknown> = {}) { // eslint-disable-line @typescript-eslint/no-explicit-any
  document.body.innerHTML = '<p id="outside">text</p>'
    + '<section data-anfra-block="trend" data-anfra-label="Trend"><span id="inside">chart</span></section>'
    + '<section data-anfra-block="map" data-anfra-menu="off"><span id="opted-out">map</span></section>';
  const setups: PluginSetup[] = setup ? [['notes', setup]] : [];
  return testFrame({ setups, bodies: { anfra: ui }, answers });
}

const rightClick = (id: string, init: MouseEventInit = {}) => {
  const event = new MouseEvent('contextmenu', {
    bubbles: true, cancelable: true, clientX: 20, clientY: 30, ...init,
  });
  document.getElementById(id)!.dispatchEvent(event);
  return event;
};

const menuRoot = () => document.querySelector('[data-anfra-layer] [data-part="anfra-ui"]')?.shadowRoot;
const entries = () => [...(menuRoot()?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])];
const settle = async () => { await flush(); await flush(); };

describe('the menu on a block', () => {
  it('leaves right-click alone while nothing offers a menu', () => {
    frame();
    expect(rightClick('inside').defaultPrevented).toBe(false);
  });

  it('opens on a marked block with the frame\'s items, and runs one with the node', async () => {
    const run = vi.fn();
    frame((kernel) => kernel.menuItem({ id: 'note', label: 'Add a note', target: 'node', run }));
    expect(rightClick('outside').defaultPrevented).toBe(false);
    expect(rightClick('inside').defaultPrevented).toBe(true);
    await settle();
    expect(entries().map((entry) => entry.textContent)).toEqual(['Add a note']);
    entries()[0].click();
    await settle();
    expect(run).toHaveBeenCalledWith(expect.objectContaining({ kind: 'node', node: expect.objectContaining({ kind: 'block', id: 'trend', label: 'Trend' }) }));
    expect(entries()).toEqual([]);
  });

  it('gives the page, Shift and an opted-out block their own right-click', () => {
    frame((kernel) => kernel.menuItem({ id: 'note', label: 'Add a note', target: 'node', run: () => {} }));
    document.getElementById('inside')!.addEventListener('contextmenu', (event) => event.preventDefault(), { once: true });
    rightClick('inside');
    expect(rightClick('inside', { shiftKey: true }).defaultPrevented).toBe(false);
    expect(rightClick('opted-out').defaultPrevented).toBe(false);
  });

  it('adds the host\'s items, which run on the host, and shows why one can\'t', async () => {
    const f = frame(undefined, {
      'anfra.menu': () => [{ id: 'ask', label: 'Ask AI' }, { id: 'share', label: 'Share', disabled: 'Disabled by admin' }],
      'anfra.run': () => null,
    });
    f.emit('anfra.menu', { kinds: ['node'] });
    await flush();
    expect(rightClick('inside').defaultPrevented).toBe(true);
    await settle();
    expect(entries().map((entry) => entry.getAttribute('aria-disabled'))).toEqual([null, 'true']);
    entries()[1].click();
    entries()[0].click();
    await settle();
    expect(f.calls.map((call) => call.name)).toEqual(['anfra.body', 'anfra.menu', 'anfra.run']);
    expect(f.calls[2].request).toMatchObject({ id: 'ask', target: { kind: 'node', node: { id: 'trend' } } });
  });

  it('keeps an item away from targets its `when` refuses', async () => {
    frame((kernel) => kernel.menuItem({
      id: 'note', label: 'Add a note', target: 'node', when: (target) => target.kind === 'node' && target.node.id === 'other', run: () => {},
    }));
    rightClick('inside');
    await settle();
    expect(entries()).toEqual([]);
  });
});

describe('the menu on a query\'s rows', () => {
  it('opens from query.menu with the rows and the block they are in, rows first', async () => {
    const run = vi.fn();
    const f = frame((kernel) => {
      kernel.menuItem({ id: 'underlying', label: 'View underlying data', target: 'rows', run });
      kernel.menuItem({ id: 'note', label: 'Add a note', target: 'node', run: () => {} });
    });
    const query = f.anfra.createApp().createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    document.getElementById('inside')!.addEventListener('contextmenu', (e) => { expect(query.menu(e, { region: 'APAC' })).toBe(true); }, { once: true });
    document.getElementById('inside')!.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    await settle();
    expect(entries().map((entry) => entry.textContent)).toEqual(['View underlying data', 'Add a note']);
    entries()[0].click();
    await settle();
    expect(run).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'rows', app: 0, query: 'revenue', rows: [{ region: 'APAC' }], node: expect.objectContaining({ kind: 'node' }),
    }));
  });

  it('opens nothing when no item is for rows', () => {
    const f = frame();
    const query = f.anfra.createApp().createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    expect(query.menu(new MouseEvent('contextmenu', { cancelable: true }), [])).toBe(false);
  });
});

describe('the modal and the popover', () => {
  it('draws a modal over an inert page, closed by Escape', async () => {
    let kernel!: FrameKernel;
    frame((k) => { kernel = k; });
    const onClose = vi.fn();
    await kernel.modal({ title: 'Rows', mount: (root) => { root.innerHTML = '<table></table>'; }, onClose });
    const dialog = menuRoot()!.querySelector('[role="dialog"]')!;
    expect(dialog.querySelector('h2')?.textContent).toBe('Rows');
    expect(dialog.querySelector<HTMLElement>('.content')!.shadowRoot!.innerHTML).toBe('<table></table>');
    expect(document.getElementById('outside')!.hasAttribute('inert')).toBe(true);
    expect(document.documentElement.style.overflow).toBe('hidden');
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onClose).toHaveBeenCalled();
    expect(menuRoot()!.querySelector('[role="dialog"]')).toBeNull();
    expect(document.getElementById('outside')!.hasAttribute('inert')).toBe(false);
  });

  it('keeps one open at a time: a modal closes a popover', async () => {
    let kernel!: FrameKernel;
    frame((k) => { kernel = k; });
    const closed = vi.fn();
    await kernel.popover({ anchor: { x: 10, y: 10 }, mount: (root) => { root.textContent = 'card'; }, onClose: closed });
    expect(menuRoot()!.querySelector('.popover')).not.toBeNull();
    await kernel.modal({ title: 'Rows', mount: () => {} });
    expect(closed).toHaveBeenCalled();
    expect(menuRoot()!.querySelector('.popover')).toBeNull();
  });

  it('closes a popover on an outside press only when asked', async () => {
    let kernel!: FrameKernel;
    frame((k) => { kernel = k; });
    const kept = await kernel.popover({ anchor: { x: 0, y: 0 }, mount: () => {} });
    document.getElementById('outside')!.dispatchEvent(new Event('pointerdown', { bubbles: true, composed: true }));
    expect(menuRoot()!.querySelector('.popover')).not.toBeNull();
    kept.close();
    await kernel.popover({ anchor: { x: 0, y: 0 }, mount: () => {}, dismiss: 'outside' });
    document.getElementById('outside')!.dispatchEvent(new Event('pointerdown', { bubbles: true, composed: true }));
    expect(menuRoot()!.querySelector('.popover')).toBeNull();
  });
});
