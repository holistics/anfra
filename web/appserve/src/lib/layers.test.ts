import { afterEach, expect, it } from 'vitest';
import { h, nextTick } from 'vue';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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

it("sees the repo's problems while their list is open", async () => {
  const { host } = await mount(RepoProblems, { problems: [{ message: 'Unknown field' }] });
  expect(layerOpen()).toBe(false);
  host.querySelector<HTMLButtonElement>('[data-testid="aml-badge"]')!.click();
  await nextTick();
  expect(layerOpen()).toBe(true);
});
