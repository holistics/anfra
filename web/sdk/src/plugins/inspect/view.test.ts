import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { byTestId, inspect, type Harness } from './tabs/testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// jsdom has no PointerEvent: a MouseEvent of the same type stands in.
const over = (element: Element) => element.dispatchEvent(new MouseEvent('pointerover', { bubbles: true, composed: true }));
const out = (element: Element) => element.dispatchEvent(new MouseEvent('pointerout', { bubbles: true, composed: true }));
const tooltip = (root: ShadowRoot) => root.querySelector<HTMLElement>('.tooltip')!;

describe('tooltips', () => {
  it("shows an element's data-tip on hover after a moment, and hides it when the pointer leaves", async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    vi.useFakeTimers();
    const problems = root.querySelector('.toolbar .pill')!;
    over(problems);
    expect(tooltip(root).hidden).toBe(true);
    vi.advanceTimersByTime(300);
    expect(tooltip(root).hidden).toBe(false);
    expect(tooltip(root).textContent).toBe('Problems in the markup');
    expect(problems.getAttribute('aria-describedby')).toBe(tooltip(root).id);
    out(problems);
    expect(tooltip(root).hidden).toBe(true);
    expect(problems.hasAttribute('aria-describedby')).toBe(false);
  });

  it('shows the next one at once while the last has just gone, and hides on Escape', async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    vi.useFakeTimers();
    const query = byTestId(root, 'node-0.0')!.querySelector<HTMLElement>('[data-testid="show-in-queries"]')!;
    over(query);
    vi.advanceTimersByTime(300);
    out(query);
    over(root.querySelector('.toolbar .pill')!);
    expect(tooltip(root).textContent).toBe('Problems in the markup');
    root.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(tooltip(root).hidden).toBe(true);
  });

  it("shows a hint's rich content on a click too, and follows it across a redraw", async () => {
    harness = await inspect({ tab: 'inspect.overview' });
    const { root } = await harness.tab('inspect.overview');
    byTestId(root, 'status-trust')!.click();
    expect(byTestId(tooltip(root), 'trust-levels')).not.toBeNull();
    harness.post(harness.mounted.panel.snapshot!);
    expect(tooltip(root).hidden).toBe(false);
    expect(byTestId(root, 'status-trust')!.getAttribute('aria-describedby')).toBe(tooltip(root).id);
    byTestId(root, 'status-trust')!.click();
    expect(tooltip(root).hidden).toBe(true);
  });
});
