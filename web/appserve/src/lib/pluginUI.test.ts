import type { Command, Tab } from '@holistics/anfra-sdk/host';
import {
  Crosshair, Download, Ellipsis, LayoutDashboard, PanelRight, Sheet, Table,
} from '@lucide/vue';
import { describe, expect, it } from 'vitest';
import {
  commandIcon, overflowMenus, panelCommandIcon, tabIcon,
} from './pluginUI';

const command = (id: string, more: Partial<Command> = {}): Command => ({
  id, title: id, run: { call: id }, ...more,
});
const tab = (id: string, icon?: string): Tab => ({ id, title: id, icon } as Tab);

describe('commandIcon', () => {
  it('draws the icon a command names', () => {
    expect(commandIcon(command('inspect.toggle', { icon: 'inspect' }))).toBe(PanelRight);
    expect(commandIcon(command('data.table', { icon: 'table' }))).toBe(Table);
  });

  it("draws the app's for an icon it does not know, or none", () => {
    expect(commandIcon(command('notes.share', { icon: 'share' }))).toBe(LayoutDashboard);
    expect(commandIcon(command('notes.open'))).toBe(LayoutDashboard);
    expect(commandIcon(command('odd', { icon: 'toString' }))).toBe(LayoutDashboard);
  });

  it("draws the panel's Pick, and the app's for others there", () => {
    expect(panelCommandIcon(command('panel.pick', { icon: 'pick' }))).toBe(Crosshair);
    expect(panelCommandIcon(command('inspect.toggle', { icon: 'inspect' }))).toBe(LayoutDashboard);
  });
});

describe('tabIcon', () => {
  it('draws the icon a tab names, and none for one it does not know', () => {
    expect(tabIcon(tab('data', 'table'))).toBe(Sheet);
    expect(tabIcon(tab('notes', 'notes'))).toBeUndefined();
    expect(tabIcon(tab('plain'))).toBeUndefined();
  });
});

describe('overflowMenus', () => {
  const png = command('render.png', { icon: 'download', placement: 'overflow' });
  const pdf = command('render.pdf', { icon: 'download', placement: 'overflow' });
  const odd = command('notes.share', { icon: 'share', placement: 'overflow' });
  const inspect = command('inspect.toggle', { icon: 'inspect', placement: 'primary' });

  it('puts overflow commands in a menu per icon, so the downloads are one Download menu', () => {
    expect(overflowMenus([inspect, png, pdf])).toEqual([
      { key: 'download', label: 'Download', icon: Download, commands: [png, pdf] },
    ]);
  });

  it('puts overflow commands with an icon it does not draw in More actions', () => {
    expect(overflowMenus([png, odd]).map((menu) => [menu.key, menu.label, menu.icon])).toEqual([
      ['download', 'Download', Download],
      ['more', 'More actions', Ellipsis],
    ]);
  });
});
