/**
 * Contributions: the UI a plugin, or the host itself, adds to the host page, as data each host
 * renders in its own chrome. A command is something to run (from the host's action bar, its command
 * palette, a shortcut); a tab is a view in the mount's panel. Plugins never name a host's layout:
 * where each one goes, and how it looks, is the host's (docs/designs/sdk-plugins.md, Plugin UI).
 */
import type { PluginInfo } from '../common/bridge';
import type { CallOptions, MountContext } from './plugin';
import type { Panel } from './panel';
import type { Theme } from '../common/theme';
import type { MenuTarget, MenuTargetKind } from '../common/menu';

/** What running a command does: make a call, or open the panel at a tab (toggling it). */
export type CommandRun =
  /** A call by its full name, made as the command's plugin (or the host), about the mount. */
  | { call: string, request?: unknown }
  /** Open the panel at this tab, by its full id, with `target` (a node's key) selected if given. */
  | { tab: string, target?: string | null };

/** How a command shows right now. */
export interface CommandState {
  /** Shown as on, such as a panel's command while the panel is open. */
  pressed?: boolean;
  /** Shown as off, with this reason (a tooltip): a command that can't run always says why. */
  disabled?: string;
}

/** What a command's `state` gets: the mount it is about, and its panel. */
export interface CommandStateContext {
  readonly mount: MountContext;
  readonly panel: Panel;
}

export interface CommandContribution {
  /** Under its plugin's name once installed (`toggle` is `inspect.toggle`); a host's own, as given. */
  id: string;
  /** Its label, and its entry in a command palette. */
  title: string;
  /** A short name for an icon (`inspect`, `download`, `comment`), which the host maps to its own. */
  icon?: string;
  /** A hint for the host: `primary` for its action bar, `overflow` for a menu. A host may ignore it. */
  placement?: 'primary' | 'overflow';
  /** A key combination, `Alt+Shift+I`: modifiers (`Ctrl`, `Alt`, `Shift`, `Meta`) then a key. */
  shortcut?: string;
  run: CommandRun;
  /**
   * Set, the command is for the menu on the page's content instead of the host's chrome: on a marked
   * block (`node`) or on a query's rows (`rows`, from the definition's `query.menu`). It runs with
   * that target: a call gets it as `target` in its request, a tab is opened with the node selected.
   */
  target?: MenuTargetKind;
  /**
   * For a command with a `target`: whether it applies to this one. The target comes from the frame
   * and is checked only for its shape: decide on it, never trust it.
   */
  when?: (target: MenuTarget, context: CommandStateContext) => boolean;
  /**
   * How it shows now, read again on each change the plugin signals (`MountContext.commandsChanged`)
   * and on each change to the panel. Unless given, a command that opens a tab is pressed while the
   * panel shows one of its plugin's tabs (or that tab, for the host's own).
   */
  state?: (context: CommandStateContext, target?: MenuTarget) => CommandState;
}

/** What a tab gets: its panel, its calls, and the page around it. */
export interface TabContext {
  /** The tab's full id. */
  readonly tab: string;
  /** The panel it is in: the node selected, the frame's snapshot, highlight, locate, pick. */
  readonly panel: Panel;
  /** The mount the panel is for: a plugin keys what it keeps about the frame by it. */
  readonly mount: MountContext;
  /** The plugins the host installed. */
  readonly plugins: readonly PluginInfo[];
  /** The host page's document. */
  readonly document: Document;
  /**
   * The host's theme now. Its tokens are also set as `--anfra-*` custom properties on the tab's
   * element, which its shadow root inherits, and follow each change: style with `var(--anfra-color-bg)`.
   */
  readonly theme: Theme;
  /** Make a call about this mount, through the pipeline, as the tab's plugin (or the host). */
  call: <T = unknown>(name: string, request?: unknown, options?: CallOptions) => Promise<T>;
}

export interface TabContribution {
  /** Under its plugin's name once installed (`structure` is `inspect.structure`); a host's own, as given. */
  id: string;
  title: string;
  /** A short name for an icon, which the host maps to its own. */
  icon?: string;
  /** Lower comes first; ties keep install order (the host's own last). */
  order?: number;
  /**
   * Draw the tab into `root`, a shadow root the host owns, with any framework or none: the host's
   * styles don't reach in, and the tab's don't reach out. What it returns runs when the tab is
   * taken down. Load what is heavy from here, so a host that never shows the tab never loads it.
   */
  mount: (root: ShadowRoot, context: TabContext) => void | (() => void) | Promise<void | (() => void)>;
}

/** An installed command: its full id, and the plugin it came from (none for the host's own). */
export interface Command extends CommandContribution {
  readonly plugin?: string;
}

/** An installed tab: its full id, and the plugin it came from (none for the host's own). */
export interface Tab extends TabContribution {
  readonly plugin?: string;
}

export type Contribution = { kind: 'command', command: Command } | { kind: 'tab', tab: Tab };

/** Every plugin's contributions, then the host's own, under full ids, as the filter keeps them. */
export function installContributions (
  plugins: readonly { name: string, host?: { commands?: readonly CommandContribution[], tabs?: readonly TabContribution[] } }[],
  own: { commands?: readonly CommandContribution[], tabs?: readonly TabContribution[] },
  keep: ((contribution: Contribution) => boolean) | undefined,
): { commands: Command[], tabs: Tab[] } {
  const commands: Command[] = [];
  const tabs: Tab[] = [];
  const ids = new Set<string>();
  const unique = (kind: string, id: string) => {
    if (typeof id !== 'string' || id === '') throw new Error(`A ${kind} needs an id.`);
    if (ids.has(id)) throw new Error(`Two contributions are named '${id}'.`);
    ids.add(id);
  };
  for (const plugin of plugins) {
    for (const command of plugin.host?.commands ?? []) {
      const id = `${plugin.name}.${command.id}`;
      unique('command', id);
      commands.push({ ...command, id, plugin: plugin.name });
    }
    for (const tab of plugin.host?.tabs ?? []) {
      const id = `${plugin.name}.${tab.id}`;
      unique('tab', id);
      tabs.push({ ...tab, id, plugin: plugin.name });
    }
  }
  for (const command of own.commands ?? []) {
    unique('command', command.id);
    commands.push({ ...command });
  }
  for (const tab of own.tabs ?? []) {
    unique('tab', tab.id);
    tabs.push({ ...tab });
  }
  const order = new Map(tabs.map((tab, index) => [tab, index]));
  tabs.sort((a, b) => (a.order ?? 0) - (b.order ?? 0) || order.get(a)! - order.get(b)!);
  return {
    commands: keep ? commands.filter((command) => keep({ kind: 'command', command })) : commands,
    tabs: keep ? tabs.filter((tab) => keep({ kind: 'tab', tab })) : tabs,
  };
}

/** How a command shows for a mount: its own `state`, or, for one that opens a tab, the panel's. */
export function commandState (command: Command, context: CommandStateContext, target?: MenuTarget): CommandState {
  if (command.state) {
    try {
      return command.state(context, target) ?? {};
    } catch {
      return { disabled: 'This command failed to report its state.' };
    }
  }
  if ('tab' in command.run) {
    const { panel } = context;
    const shown = panel.tab;
    const pressed = panel.open && shown !== undefined
      && (command.plugin ? shown.startsWith(`${command.plugin}.`) : shown === command.run.tab);
    return { pressed };
  }
  return {};
}
