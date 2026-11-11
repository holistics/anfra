import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { h, nextTick, type Component } from 'vue';
import { TooltipProvider } from '@/components/ui/tooltip';
import type { CatalogEntry } from '@/lib/catalog';
import { MAX_WIDTH } from '@/lib/sidebar';
import { mount as mountBare, unmountAll } from '@/test/mount';
import AppTree from './AppTree.vue';
import SidebarFoot, { themeMenuOpen } from './SidebarFoot.vue';
import SidebarPanel from './SidebarPanel.vue';
import SidebarPeek from './SidebarPeek.vue';
import SidebarResizer from './SidebarResizer.vue';

afterEach(unmountAll);

beforeAll(() => {
  // jsdom has no pointer capture.
  HTMLElement.prototype.setPointerCapture ??= () => {};
});

/** Mounts inside a TooltipProvider, as App mounts the shell. */
const mount = (component: Component, props: Record<string, unknown> = {}, slot?: () => unknown) => mountBare(
  TooltipProvider,
  {},
  () => h(component, props, slot ? { default: slot } : undefined),
);

const app = (path: string): CatalogEntry => ({ kind: 'app', path, label: path.split('/').pop()! });
const folder = (path: string, children: CatalogEntry[]): CatalogEntry => ({
  kind: 'folder', path, name: path.split('/').pop()!, children,
});

const pointer = (type: string, init: MouseEventInit = {}) => new MouseEvent(type, { bubbles: true, button: 0, ...init });

describe('AppTree', () => {
  const entries = [folder('finance', [app('finance/revenue.html')]), app('sales.html')];

  it('marks a Data App with the app window icon, as Cloud does', async () => {
    const { host } = await mount(AppTree, { entries, collapsed: new Set() });
    const icon = host.querySelector('[data-app="sales.html"] svg')!;
    expect(icon.getAttribute('class')).toContain('lucide-app-window-mac');
  });

  it("swaps a folder's icon for its chevron under the pointer, with none trailing the name", async () => {
    const { host } = await mount(AppTree, { entries, collapsed: new Set() });
    const row = host.querySelector('[data-folder="finance"]')!;
    const [icon, chevron] = row.querySelectorAll('span svg');
    expect(icon.getAttribute('class')).toContain('group-hover/folder:invisible');
    expect(chevron.hasAttribute('data-chevron')).toBe(true);
    expect(chevron.getAttribute('class')).toContain('invisible');
    expect(row.lastElementChild!.tagName).toBe('SPAN');
    expect(row.lastElementChild!.textContent).toBe('finance');
  });

  it('marks the current Data App as the page, with the neutral fill and no violet icon', async () => {
    const { host } = await mount(AppTree, { entries, collapsed: new Set(), selected: 'finance/revenue.html' });
    const row = host.querySelector('[data-app="finance/revenue.html"]')!;
    expect(row.getAttribute('aria-current')).toBe('page');
    expect(row.hasAttribute('data-active')).toBe(true);
    expect(row.className).toContain('data-active:bg-active');
    expect(row.className).not.toContain('icon-highlight');
    expect(host.querySelector('[data-app="sales.html"]')!.hasAttribute('data-active')).toBe(false);
  });

  it('opens and closes a folder in place', async () => {
    const onToggle = vi.fn();
    const { host } = await mount(AppTree, { entries, collapsed: new Set(['finance']), onToggle });
    expect(host.querySelector('[data-app="finance/revenue.html"]')).toBeNull();
    host.querySelector<HTMLButtonElement>('[data-folder="finance"]')!.click();
    expect(onToggle).toHaveBeenCalledWith('finance');
  });
});

describe('SidebarPanel', () => {
  const props = {
    repoName: 'shop',
    entries: [app('sales'), app('churn')],
    loaded: true,
    collapsed: new Set<string>(),
    health: 'up',
    version: { current: '0.16.0' },
    themeChoice: 'system',
    theme: 'light',
  };

  it('names the repo, and lists every Data App', async () => {
    const { host } = await mount(SidebarPanel, props);
    expect(host.querySelector('[data-testid="brand"]')!.textContent).toBe('shop');
    expect([...host.querySelectorAll('[data-app]')].map((row) => row.getAttribute('data-app'))).toEqual(['sales', 'churn']);
  });

  it('opens the palette from the search', async () => {
    const onOpenPalette = vi.fn();
    const { host } = await mount(SidebarPanel, { ...props, onOpenPalette });
    const search = host.querySelector<HTMLButtonElement>('[data-testid="palette-open"]')!;
    expect(search.getAttribute('aria-haspopup')).toBe('dialog');
    search.click();
    expect(onOpenPalette).toHaveBeenCalledOnce();
  });

  it('shows a skeleton until the Data Apps are read', async () => {
    const { host } = await mount(SidebarPanel, { ...props, entries: [], loaded: false });
    expect(host.querySelector('[data-testid="loading-apps"]')!.getAttribute('role')).toBe('status');
    expect(host.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(3);
    expect(host.querySelector('[data-testid="empty-state"]')).toBeNull();
  });

  it('says a read failed when there is no tree, and tries again', async () => {
    const onRetry = vi.fn();
    const { host } = await mount(SidebarPanel, {
      ...props, entries: [], loadError: "Couldn't load the Data Apps: offline", onRetry,
    });
    expect(host.querySelector('[role="alert"]')!.textContent).toContain('offline');
    host.querySelector<HTMLButtonElement>('[data-testid="retry"]')!.click();
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it('keeps the tree a later read left, when that read fails', async () => {
    const { host } = await mount(SidebarPanel, { ...props, loadError: "Couldn't load the Data Apps: offline" });
    expect(host.querySelectorAll('[data-app]')).toHaveLength(2);
    expect(host.querySelector('[data-testid="load-error"]')).toBeNull();
  });
});

describe('SidebarFoot', () => {
  const props = {
    health: 'up', version: { current: '0.16.0' }, themeChoice: 'system', theme: 'light',
  };

  it('says which anfra is running, with a dot and in text', async () => {
    const { host } = await mount(SidebarFoot, props);
    const status = host.querySelector('[data-testid="anfra-status"]')!;
    expect(status.getAttribute('role')).toBe('status');
    expect(status.getAttribute('data-status')).toBe('up');
    expect(status.textContent).toContain('anfra v0.16.0');
    expect(host.querySelector('[data-testid="status-dot"]')!.className).toContain('bg-success-default');
    expect(host.querySelector('[data-testid="alpha-badge"]')!.textContent).toBe('Alpha');
  });

  it('says when anfra is not answering', async () => {
    const { host } = await mount(SidebarFoot, { ...props, health: 'unreachable' });
    expect(host.querySelector('[data-testid="anfra-status"]')!.textContent).toContain('anfra server unreachable');
    expect(host.querySelector('[data-testid="status-dot"]')!.className).toContain('bg-danger-default');
  });

  it('offers Light, Dark and System, the choice in effect checked', async () => {
    const onSetTheme = vi.fn();
    const { host } = await mount(SidebarFoot, { ...props, themeChoice: 'dark', theme: 'dark', onSetTheme });
    const button = host.querySelector<HTMLButtonElement>('[data-testid="theme-menu"]')!;
    expect(button.getAttribute('aria-label')).toBe('Theme');
    button.click();
    await nextTick();
    await nextTick();
    const options = [...document.querySelectorAll('[data-theme-option]')];
    expect(options.map((option) => option.textContent!.trim())).toEqual(['Light', 'Dark', 'System']);
    expect(options[1].getAttribute('aria-checked')).toBe('true');
    (options[2] as HTMLElement).click();
    await nextTick();
    expect(onSetTheme).toHaveBeenCalledWith('system');
  });

  it('stops counting its menu as open when it goes while open, as when the peek closes under it', async () => {
    const { host, unmount } = await mount(SidebarFoot, props);
    host.querySelector<HTMLButtonElement>('[data-testid="theme-menu"]')!.click();
    await nextTick();
    expect(themeMenuOpen.value).toBe(true);
    unmount();
    expect(themeMenuOpen.value).toBe(false);
  });
});

describe('SidebarResizer', () => {
  it('follows a drag of its edge, within bounds, and gives the width it ends at', async () => {
    const onPress = vi.fn();
    const onResize = vi.fn();
    const onRelease = vi.fn();
    const onCollapse = vi.fn();
    const { host } = await mount(SidebarResizer, {
      width: 256, onPress, onResize, onRelease, onCollapse,
    });
    const rail = host.querySelector<HTMLButtonElement>('[data-testid="sidebar-rail"]')!;
    expect(rail.style.left).toBe('256px');
    rail.dispatchEvent(pointer('pointerdown', { clientX: 256 }));
    rail.dispatchEvent(pointer('pointermove', { clientX: 300 }));
    rail.dispatchEvent(pointer('pointermove', { clientX: 1000 }));
    rail.dispatchEvent(pointer('pointerup', { clientX: 1000 }));
    rail.click();
    expect(onPress).toHaveBeenCalledOnce();
    expect(onResize.mock.calls.map(([width]) => width)).toEqual([300, MAX_WIDTH]);
    expect(onRelease).toHaveBeenCalledWith(MAX_WIDTH);
    expect(onCollapse).not.toHaveBeenCalled();
  });

  it('ignores a press with another button', async () => {
    const onPress = vi.fn();
    const onResize = vi.fn();
    const { host } = await mount(SidebarResizer, { width: 256, onPress, onResize });
    const rail = host.querySelector<HTMLButtonElement>('[data-testid="sidebar-rail"]')!;
    rail.dispatchEvent(pointer('pointerdown', { button: 2, clientX: 256 }));
    rail.dispatchEvent(pointer('pointermove', { clientX: 400 }));
    expect(onPress).not.toHaveBeenCalled();
    expect(onResize).not.toHaveBeenCalled();
  });

  it('collapses the sidebar on a click', async () => {
    const onRelease = vi.fn();
    const onCollapse = vi.fn();
    const { host } = await mount(SidebarResizer, { width: 256, onRelease, onCollapse });
    const rail = host.querySelector<HTMLButtonElement>('[data-testid="sidebar-rail"]')!;
    rail.dispatchEvent(pointer('pointerdown', { clientX: 256 }));
    rail.dispatchEvent(pointer('pointermove', { clientX: 257 }));
    rail.dispatchEvent(pointer('pointerup', { clientX: 257 }));
    rail.click();
    expect(onRelease).toHaveBeenCalledWith(undefined);
    expect(onCollapse).toHaveBeenCalledOnce();
  });
});

describe('SidebarPeek', () => {
  const content = () => h('p', { 'data-testid': 'peek-content' }, 'the sidebar');

  it('toggles the sidebar, and names its shortcut', async () => {
    const onToggleSidebar = vi.fn();
    const { host } = await mount(SidebarPeek, {
      open: true, narrow: false, width: 256, onToggleSidebar,
    }, content);
    const toggle = host.querySelector<HTMLButtonElement>('[data-testid="sidebar-toggle"]')!;
    expect(toggle.getAttribute('aria-label')).toBe('Hide sidebar');
    expect(toggle.getAttribute('aria-keyshortcuts')).toMatch(/^(Meta|Control)\+\\$/);
    toggle.click();
    expect(onToggleSidebar).toHaveBeenCalledOnce();
  });

  it("floats the sidebar's content from the toggle while the sidebar is collapsed", async () => {
    const { host } = await mount(SidebarPeek, { open: false, narrow: false, width: 300 }, content);
    host.querySelector('[data-testid="sidebar-toggle"]')!.dispatchEvent(new MouseEvent('pointerenter'));
    await nextTick();
    await nextTick();
    const peek = document.querySelector<HTMLElement>('[data-testid="sidebar-peek"]')!;
    expect(peek.style.width).toBe('300px');
    expect(peek.querySelector('[data-testid="peek-content"]')).not.toBeNull();
  });

  it('closes 150 ms after the pointer leaves, but stays while the theme menu is open', async () => {
    vi.useFakeTimers();
    try {
      const { host } = await mount(SidebarPeek, { open: false, narrow: false, width: 256 }, content);
      const toggle = host.querySelector('[data-testid="sidebar-toggle"]')!;
      const anchor = toggle.parentElement!;
      const peek = () => document.querySelector('[data-testid="sidebar-peek"]');
      toggle.dispatchEvent(new MouseEvent('pointerenter'));
      await nextTick();
      themeMenuOpen.value = true;
      anchor.dispatchEvent(new MouseEvent('pointerleave'));
      vi.advanceTimersByTime(200);
      await nextTick();
      expect(peek()?.getAttribute('data-state')).toBe('open');
      themeMenuOpen.value = false; // the menu closes, the pointer away: the peek follows
      await nextTick();
      vi.advanceTimersByTime(200);
      await nextTick();
      expect(peek()?.getAttribute('data-state') ?? 'closed').toBe('closed');
    } finally {
      themeMenuOpen.value = false;
      vi.useRealTimers();
    }
  });

  it('does not peek at an open sidebar, nor on a narrow window', async () => {
    for (const props of [{ open: true, narrow: false }, { open: false, narrow: true }]) {
      const { host } = await mount(SidebarPeek, { ...props, width: 256 }, content);
      host.querySelector('[data-testid="sidebar-toggle"]')!.dispatchEvent(new MouseEvent('pointerenter'));
      await nextTick();
      expect(document.querySelector('[data-testid="sidebar-peek"]')).toBeNull();
      unmountAll();
    }
  });
});
