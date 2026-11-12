import { afterEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { mount, unmountAll } from '@/test/mount';
import type { CatalogEntry } from '@/lib/catalog';
import type { AmlProblem } from '@/lib/server';
import {
  AppHeader, AppSidebar, EmptyState, InlineCode, LoadingBar, RepoProblems, Scrim, ShortcutKeys,
} from '.';

afterEach(unmountAll);

const app = (path: string): CatalogEntry => ({
  kind: 'app', path, name: path.split('/').pop()!, label: path.split('/').pop()!,
} as CatalogEntry);

describe('Scrim', () => {
  it('covers the window behind what floats, and takes the click that closes it', async () => {
    const onClick = vi.fn();
    const { host } = await mount(Scrim, { onClick });
    const scrim = host.querySelector<HTMLElement>('[data-slot="scrim"]')!;
    expect(scrim.className).toContain('fixed inset-0');
    scrim.click();
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe('EmptyState', () => {
  it('says why there is no Data App, beside the app icon', async () => {
    const { host } = await mount(EmptyState, {}, () => 'Pick a Data App on the left.');
    expect(host.querySelector('svg')).not.toBeNull();
    expect(host.textContent).toContain('Pick a Data App on the left.');
  });
});

describe('InlineCode', () => {
  it('sets a name written in code in the mono role', async () => {
    const { host } = await mount(InlineCode, {}, () => 'apps/');
    const code = host.querySelector('code')!;
    expect(code.textContent).toBe('apps/');
    expect(code.className).toContain('font-mono');
  });
});

describe('LoadingBar', () => {
  it('is a progress bar, labelled by whoever shows it', async () => {
    const { host } = await mount(LoadingBar, { 'aria-label': 'Loading Data App' });
    const bar = host.querySelector('[role="progressbar"]')!;
    expect(bar.getAttribute('aria-label')).toBe('Loading Data App');
  });
});

describe('RepoProblems', () => {
  const problems: AmlProblem[] = [
    { message: 'Unknown field', file: 'models/orders.aml', line: 3, column: 7 } as AmlProblem,
    { message: 'No data source' } as AmlProblem,
  ];

  it('counts the problems, and lists them with where each is when opened', async () => {
    const { host } = await mount(RepoProblems, { problems });
    const badge = host.querySelector<HTMLButtonElement>('[data-testid="aml-badge"]')!;
    expect(badge.textContent).toContain('2 problems');
    expect(host.querySelector('[data-testid="aml-banner"]')).toBeNull();
    badge.click();
    await nextTick();
    const banner = host.querySelector('[data-testid="aml-banner"]')!;
    expect(banner.textContent).toContain('models/orders.aml:3:7');
    expect(banner.textContent).toContain('No data source');
    expect(badge.getAttribute('aria-expanded')).toBe('true');
  });

  it('closes on Escape', async () => {
    const { host } = await mount(RepoProblems, { problems });
    host.querySelector<HTMLButtonElement>('[data-testid="aml-badge"]')!.click();
    await nextTick();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(host.querySelector('[data-testid="aml-banner"]')).toBeNull();
  });
});

describe('AppSidebar', () => {
  const props = {
    open: true,
    narrow: false,
    repoName: 'shop',
    entries: [app('sales'), app('churn')],
    loaded: true,
    collapsed: new Set<string>(),
    health: 'up',
    version: { current: '0.16.0' },
    theme: 'light',
  };

  it('names the repo, and says which anfra is running', async () => {
    const { host } = await mount(AppSidebar, props);
    expect(host.querySelector('[data-testid="brand"]')!.textContent).toBe('shop');
    const status = host.querySelector('[data-testid="anfra-status"]')!;
    expect(status.textContent).toContain('anfra v0.16.0');
    expect(status.getAttribute('data-tone')).toBe('success');
  });

  it('lists every Data App', async () => {
    const { host } = await mount(AppSidebar, props);
    expect([...host.querySelectorAll('[data-app]')].map((row) => row.getAttribute('data-app'))).toEqual(['sales', 'churn']);
  });

  it('marks the open Data App as the current page', async () => {
    const { host } = await mount(AppSidebar, { ...props, selected: 'sales' });
    const row = host.querySelector('[data-app="sales"]')!;
    expect(row.getAttribute('aria-current')).toBe('page');
    expect(row.hasAttribute('data-active')).toBe(true);
  });

  it('searches in the palette, and asks for the other theme', async () => {
    const onOpenPalette = vi.fn();
    const onToggleTheme = vi.fn();
    const { host } = await mount(AppSidebar, { ...props, onOpenPalette, onToggleTheme });
    const search = host.querySelector<HTMLButtonElement>('[data-testid="palette-open"]')!;
    expect(search.getAttribute('aria-haspopup')).toBe('dialog');
    search.click();
    host.querySelector<HTMLButtonElement>('[data-testid="theme-toggle"]')!.click();
    expect(onOpenPalette).toHaveBeenCalledOnce();
    expect(onToggleTheme).toHaveBeenCalledOnce();
  });
});

describe('ShortcutKeys', () => {
  it('draws a combination as one key each', async () => {
    const { host } = await mount(ShortcutKeys, { combo: 'Alt+Shift+I' });
    const keys = [...host.querySelectorAll('[data-slot="kbd"]')].map((key) => key.textContent);
    expect(keys).toHaveLength(3);
    expect(keys.at(-1)).toBe('I');
  });
});

describe('AppHeader', () => {
  const inspect = { id: 'inspect.toggle', title: 'Inspect', icon: 'inspect', run: { call: 'inspect.toggle' } };
  const props = {
    sidebarOpen: true,
    hasApp: true,
    title: 'Sales',
    crumbs: [{ path: 'finance', name: 'finance' }],
    problems: [],
    loading: false,
    commands: [inspect],
    menus: [],
    commandState: () => ({ pressed: true }),
  };

  it('shows the open Data App and the folders it is in', async () => {
    const onRevealFolder = vi.fn();
    const { host } = await mount(AppHeader, { ...props, onRevealFolder });
    expect(host.querySelector('[data-testid="viewer-title"]')!.textContent).toBe('Sales');
    host.querySelector<HTMLButtonElement>('[data-crumb="finance"]')!.click();
    expect(onRevealFolder).toHaveBeenCalledWith('finance');
  });

  it('runs a command, and shows a toggled one pressed', async () => {
    const onRun = vi.fn();
    const { host } = await mount(AppHeader, { ...props, onRun });
    const button = host.querySelector<HTMLButtonElement>('[data-testid="command-inspect.toggle"]')!;
    expect(button.getAttribute('aria-pressed')).toBe('true');
    button.click();
    expect(onRun).toHaveBeenCalledWith(inspect);
  });

  it('runs nothing with no Data App open', async () => {
    const { host } = await mount(AppHeader, { ...props, hasApp: false });
    expect(host.querySelector<HTMLButtonElement>('[data-testid="command-inspect.toggle"]')!.disabled).toBe(true);
  });

  it('has no Reload: Refresh is a command like any other', async () => {
    const refresh = {
      id: 'core.refresh', title: 'Refresh', icon: 'refresh', placement: 'primary' as const, run: { call: 'core.refresh' },
    };
    const { host } = await mount(AppHeader, {
      ...props, commands: [refresh, inspect], commandState: () => ({}),
    });
    expect(host.querySelector('[data-testid="reload"]')).toBeNull();
    const button = host.querySelector<HTMLButtonElement>('[data-testid="command-core.refresh"]')!;
    expect(button.disabled).toBe(false);
    expect(button.title).toBe('Refresh');
    expect(button.querySelector('svg')!.getAttribute('class')).toContain('refresh-cw');
  });
});
