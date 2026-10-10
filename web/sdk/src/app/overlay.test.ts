import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import { FLASH_MS, OVERLAY_ID, overlayFor } from './overlay';
import { startPick } from './pick';

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); document.body.innerHTML = ''; });

describe('the overlay', () => {
  it('is one per document, hidden until shown, and draws a box per element with one label', () => {
    document.body.innerHTML = '<div id="a"></div><div id="b"></div>';
    const overlay = overlayFor(document);
    expect(overlayFor(document)).toBe(overlay);
    const root = document.getElementById(OVERLAY_ID)!;
    expect(root.style.display).toBe('none');
    overlay.show([document.getElementById('a')!, document.getElementById('b')!], {
      kind: 'query', name: 'revenue', note: '1 of 2', size: '0 × 0',
    });
    expect(root.style.display).toBe('block');
    expect(root.children).toHaveLength(3);
    const parts = [...root.lastElementChild!.children].map((part) => `${part.getAttribute('data-part')}=${part.textContent}`);
    expect(parts).toEqual(['kind=query', 'name=revenue', 'note=1 of 2', 'size=0 × 0']);
    overlay.hide();
    expect(root.style.display).toBe('none');
    expect(root.children).toHaveLength(0);
  });

  it('flashes for a moment, then hides itself', () => {
    document.body.innerHTML = '<div id="a"></div>';
    const overlay = overlayFor(document);
    overlay.flash([document.getElementById('a')!], { kind: 'block', name: 'a' });
    expect(overlay.visible).toBe(true);
    vi.advanceTimersByTime(FLASH_MS);
    expect(overlay.visible).toBe(false);
  });

  it('shows nothing for no elements', () => {
    const overlay = overlayFor(document);
    overlay.show([], { kind: 'block', name: 'x' });
    expect(overlay.visible).toBe(false);
  });
});

describe('pick mode', () => {
  it('reports the nearest marked element under the pointer, swallows the click, and stops on Escape', () => {
    document.body.innerHTML = '<div data-anfra-block="b"><span id="inner">x</span></div><p id="plain"></p>';
    const hovered: (Element | null)[] = [];
    const picked: Element[] = [];
    let cancelled = 0;
    const stop = startPick(document, {
      onHover: (el) => hovered.push(el),
      onPick: (el) => picked.push(el),
      onCancel: () => { cancelled += 1; },
    });
    const inner = document.getElementById('inner')!;
    const block = inner.parentElement!;
    inner.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }));
    inner.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }));
    document.getElementById('plain')!.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }));
    expect(hovered).toEqual([block, null]);

    const appClick = vi.fn();
    block.addEventListener('click', appClick);
    const click = new MouseEvent('click', { bubbles: true, cancelable: true });
    inner.dispatchEvent(click);
    expect(picked).toEqual([block]);
    expect(click.defaultPrevented).toBe(true);
    expect(appClick).not.toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(cancelled).toBe(1);

    stop();
    inner.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    expect(appClick).toHaveBeenCalledTimes(1);
  });
});
