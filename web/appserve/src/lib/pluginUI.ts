// The lucide icon appserve draws for the icon name a plugin's command or tab gives.
import {
  ChevronsRight,
  Code,
  Copy,
  Crosshair,
  Database,
  Download,
  Ellipsis,
  GitFork,
  LayoutDashboard,
  ListTree,
  PanelRight,
  RefreshCw,
  Sheet,
  Table,
} from '@lucide/vue';
import type { Command, Tab } from '@holistics/anfra-sdk/host';
import type { Component } from 'vue';

const commandIcons: Record<string, Component> = {
  inspect: PanelRight,
  download: Download,
  table: Table,
  copy: Copy,
  reload: RefreshCw,
};
const known = (icons: Record<string, Component>, name?: string) => (name && Object.hasOwn(icons, name) ? icons[name] : undefined);

/** A command in the header or the palette; one with an icon appserve doesn't draw gets the app's. */
export const commandIcon = (command: Command): Component => known(commandIcons, command.icon) ?? LayoutDashboard;

/** A command in the panel's strip, before its tabs. */
export const panelCommandIcon = (command: Command): Component => known({ pick: Crosshair }, command.icon) ?? LayoutDashboard;

const tabIcons: Record<string, Component> = {
  overview: LayoutDashboard,
  table: Sheet,
  query: Database,
  lineage: GitFork,
  tree: ListTree,
  code: Code,
};
/** A panel tab's icon; a tab naming one appserve doesn't draw has none. */
export const tabIcon = (tab: Tab): Component | undefined => known(tabIcons, tab.icon);

/** The More tabs button's. */
export const moreTabsIcon = ChevronsRight;

export interface CommandMenu {
  key: string;
  label: string;
  icon: Component;
  commands: Command[];
}

/**
 * The header's menus: a plugin's `overflow` commands in a menu per icon appserve draws, so render's
 * downloads are one Download menu, and the rest in one More actions menu.
 */
export function overflowMenus(commands: readonly Command[]): CommandMenu[] {
  const menus = new Map<string, CommandMenu>();
  for (const command of commands.filter((each) => each.placement === 'overflow')) {
    const icon = known(commandIcons, command.icon);
    const key = icon ? command.icon! : 'more';
    if (!menus.has(key)) {
      menus.set(key, {
        key, label: key === 'download' ? 'Download' : 'More actions', icon: icon ?? Ellipsis, commands: [],
      });
    }
    menus.get(key)!.commands.push(command);
  }
  return [...menus.values()];
}
