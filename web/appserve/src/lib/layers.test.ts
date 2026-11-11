import { afterEach, expect, it } from 'vitest';
import { h, nextTick } from 'vue';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip';
import { RepoProblems } from '@/components/shell';
import { mount, unmountAll } from '@/test/mount';
import { layerOpen } from './layers';

afterEach(unmountAll);

it('sees an open menu, which takes Escape before the shell', async () => {
  expect(layerOpen()).toBe(false);
  const menu = await mount(DropdownMenu, { defaultOpen: true }, () => [
    h(DropdownMenuTrigger, null, () => 'Download'),
    h(DropdownMenuContent, null, () => h(DropdownMenuItem, null, () => 'Download as PNG')),
  ]);
  await nextTick();
  expect(layerOpen()).toBe(true);
  menu.unmount();
  expect(layerOpen()).toBe(false);
});

it('leaves Escape to the shell while only a tooltip shows', async () => {
  // jsdom has no ResizeObserver, which the tooltip's arrow measures with.
  globalThis.ResizeObserver ??= class { observe () {} unobserve () {} disconnect () {} } as unknown as typeof ResizeObserver;
  await mount(TooltipProvider, {}, () => h(Tooltip, { open: true }, {
    default: () => [h(TooltipTrigger, null, { default: () => 'Theme' }), h(TooltipContent, null, { default: () => 'Theme' })],
  }));
  await nextTick();
  expect(document.querySelector('[data-slot="tooltip-content"]')).not.toBeNull();
  expect(layerOpen()).toBe(false);
});

it("sees the repo's problems while their list is open", async () => {
  const { host } = await mount(RepoProblems, { problems: [{ message: 'Unknown field' }] });
  expect(layerOpen()).toBe(false);
  host.querySelector<HTMLButtonElement>('[data-testid="aml-badge"]')!.click();
  await nextTick();
  expect(layerOpen()).toBe(true);
});
