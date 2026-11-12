import { Download } from '@lucide/vue';
import { afterEach, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { mount, unmountAll } from '@/test/mount';
import CommandPalette, { type PaletteEntry } from './CommandPalette.vue';

// reka scrolls the highlighted item into view; jsdom has no scrollIntoView.
Element.prototype.scrollIntoView ??= () => {};

afterEach(unmountAll);

const entries: PaletteEntry[] = [
  { id: 'command:render.png', title: 'Download PNG', group: 'Data App', icon: Download, shortcut: 'Alt+Shift+P' },
  { id: 'command:core.refresh', title: 'Refresh', group: 'Data App', disabled: 'Open a Data App first' },
  { id: 'command:inspect.toggle', title: 'Inspect', group: 'Data App', detail: 'Opens the panel' },
  { id: 'app:sales', title: 'Sales', group: 'Open a Data App' },
];

const item = (id: string) => document.querySelector<HTMLElement>(`[data-testid="palette-${id}"]`);

it('lists the entries by group, with what keeps one from running', async () => {
  await mount(CommandPalette, { open: true, entries });
  const headings = [...document.querySelectorAll('[data-slot="command-group-heading"]')].map((h) => h.textContent?.trim());
  expect(headings).toEqual(['Data App', 'Open a Data App']);
  expect(item('command:core.refresh')!.hasAttribute('data-disabled')).toBe(true);
  expect(item('command:core.refresh')!.className).toContain('data-disabled:text-disabled');
  expect(item('command:core.refresh')!.textContent).toContain('Open a Data App first');
  expect(item('command:inspect.toggle')!.title).toBe('Opens the panel');
  expect(item('command:render.png')!.querySelectorAll('[data-slot="kbd"]')).toHaveLength(3);
});

it('closes, then runs the entry chosen', async () => {
  const onRun = vi.fn();
  const onUpdateOpen = vi.fn();
  await mount(CommandPalette, { open: true, entries, onRun, 'onUpdate:open': onUpdateOpen });
  item('app:sales')!.click();
  await nextTick();
  expect(onUpdateOpen).toHaveBeenCalledWith(false);
  expect(onRun).toHaveBeenCalledWith('app:sales');
});
