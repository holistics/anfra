<script setup lang="ts">
import {
  computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch,
} from 'vue';
import AppTree from './AppTree.vue';
import DataAppFrame from './DataAppFrame.vue';
import Icon from './Icon.vue';
import Logo from './Logo.vue';
import PluginPanel from './PluginPanel.vue';
import CommandPalette, { type PaletteEntry } from './CommandPalette.vue';
import type {
  DatasetDescriptor, Rendering, ThemeInput, User,
} from '@holistics/anfra-sdk/common';
import { designTheme } from '@holistics/anfra-sdk/common';
import type { Command, Host, MountedDataApp } from '@holistics/anfra-sdk/host';
import {
  allApps, filterEntries, findApp, folderPaths, pathFromLocation, urlFor, type CatalogEntry,
} from './catalog';
import * as server from './server';

const entries = ref<CatalogEntry[]>([]);
const loaded = ref(false);
const loadError = ref<string>();
const selectedPath = ref(pathFromLocation(window.location.pathname));
// Bumped to reload the running Data App: its file changed, or the Datasets it was provisioned with.
const revision = ref(0);

// The server's health (core.status), polled; and the repo's AML problems (core.validate).
const health = ref<server.Health | 'unknown'>('unknown');
const amlProblems = ref<server.AmlProblem[]>([]);
const datasetProblems = ref<server.AmlProblem[]>([]);
const contextProblems = ref<server.AmlProblem[]>([]);
const problems = computed(() => [...contextProblems.value, ...amlProblems.value, ...datasetProblems.value]);
const repoName = ref<string>();
const reader = ref<User>();
const watching = ref(false);
const allowOrigins = ref<string[]>([]);
// What each Data App is provisioned with; reloaded when the AML changes.
const datasets = ref<Record<string, DatasetDescriptor>>();
const frameError = ref<string>();
// The anfra serving, and a newer release when the server knows of one.
const version = ref<server.Version>();

// A release version reads as v0.4.2; a local build's ("dev") as itself.
const versionLabel = computed(() => {
  const current = version.value?.current;
  if (!current) return undefined;
  return /^\d/.test(current) ? `v${current}` : current;
});

const statusLabel = computed(() => {
  if (health.value === 'unreachable') return 'anfra server unreachable';
  if (health.value === 'up') return versionLabel.value ? `anfra ${versionLabel.value}` : 'anfra is running';
  if (health.value === 'down') return 'anfra is down';
  return 'Checking anfra…';
});

async function checkHealth (): Promise<void> {
  const was = health.value;
  health.value = await server.health();
  // Back after an outage: catch up on anything missed. A restart may also be onto a new version.
  if (was === 'unreachable' && health.value !== 'unreachable') {
    void refreshAll();
    void loadVersion();
  }
}

const updateHint = computed(() => (version.value?.latest
  ? `anfra ${version.value.latest} is available (you have ${version.value.current}). Run \`anfra update\`, then restart anfra serve.`
  : ''));

async function loadVersion (): Promise<void> {
  version.value = await server.version();
}

async function loadProblems (): Promise<void> {
  try {
    amlProblems.value = await server.problems();
  } catch { /* the status says why */ }
}

async function loadDatasets (): Promise<void> {
  try {
    const loaded = await server.datasets();
    datasets.value = loaded.datasets;
    datasetProblems.value = loaded.problems;
    frameError.value = undefined;
  } catch (err) {
    frameError.value = `Couldn't load the datasets: ${(err as Error).message}`;
  }
}

async function loadContext (): Promise<void> {
  try {
    const ctx = await server.context();
    repoName.value = ctx.repo.name;
    reader.value = ctx.reader;
    watching.value = ctx.watch;
    allowOrigins.value = ctx.allowOrigins;
    contextProblems.value = ctx.problems;
  } catch { /* the status says why */ }
}

async function refreshAll (): Promise<void> {
  await Promise.all([loadCatalog(), loadProblems(), loadDatasets(), loadContext()]);
  revision.value += 1;
}

function where (problem: server.AmlProblem): string {
  if (!problem.file) return '';
  return problem.line ? `${problem.file}:${problem.line}${problem.column ? `:${problem.column}` : ''}` : problem.file;
}

const COLLAPSED_KEY = 'anfra-appserve:collapsed-folders';
function loadCollapsed (): Set<string> {
  try {
    return new Set(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? '[]') as string[]);
  } catch {
    return new Set();
  }
}
// Folders start open; a reader collapses what they don't need, and it sticks.
const collapsed = ref(loadCollapsed());

function setCollapsed (next: Set<string>): void {
  collapsed.value = next;
  try {
    window.localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...next]));
  } catch { /* storage unavailable: the choice just doesn't persist */ }
}

function toggleFolder (path: string): void {
  const next = new Set(collapsed.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  setCollapsed(next);
}

const narrowQuery = window.matchMedia('(max-width: 800px)');
const narrow = ref(narrowQuery.matches);
const sidebarOpen = ref(!narrow.value);
function onNarrowChange (): void {
  narrow.value = narrowQuery.matches;
  sidebarOpen.value = !narrow.value;
}

const search = ref('');
const searchInput = ref<HTMLInputElement>();
const filtered = computed(() => filterEntries(entries.value, search.value));
const searching = computed(() => search.value.trim() !== '');

const THEME_KEY = 'anfra-appserve:theme';
function systemTheme (): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}
function storedTheme (): 'light' | 'dark' {
  try {
    const saved = window.localStorage.getItem(THEME_KEY);
    if (saved === 'light' || saved === 'dark') return saved;
  } catch { /* storage unavailable: follow the system */ }
  return systemTheme();
}
const theme = ref(storedTheme());
watch(theme, (value) => { document.documentElement.dataset.theme = value; }, { immediate: true });
// The plugins' UI follows the shell: its scheme, and anfra-design's tokens as the SDK's.
const pluginTheme = computed<ThemeInput>(() => designTheme(theme.value, getComputedStyle(document.documentElement)));
function toggleTheme (): void {
  theme.value = theme.value === 'dark' ? 'light' : 'dark';
  try {
    window.localStorage.setItem(THEME_KEY, theme.value);
  } catch { /* the choice just doesn't persist */ }
}

const problemsOpen = ref(false);

// The host runtime and the open Data App's mount, for the plugins' commands and its panel. The
// panel belongs to one Data App: it closes when another is picked, and survives Reload.
const runtime = shallowRef<Host>();
const app = shallowRef<MountedDataApp>();
// Bumped on each change to the panel or the commands' state, which live in the SDK.
const panelVersion = ref(0);
let keptPanel: { tab?: string } | undefined;
let stopWatching: (() => void) | undefined;
function onAppMounted (host: Host, mounted: MountedDataApp): void {
  stopWatching?.();
  runtime.value = host;
  app.value = mounted;
  const changed = () => {
    panelVersion.value += 1;
    keptPanel = mounted.panel.open ? { tab: mounted.panel.tab } : undefined;
  };
  const stops = [mounted.panel.subscribe(changed), mounted.onCommandsChange(changed)];
  stopWatching = () => stops.forEach((stop) => stop());
  if (keptPanel) mounted.panel.show(keptPanel.tab);
  panelVersion.value += 1;
}
// The Data App went (Reload, another one picked): nothing runs on its mount until the next one is.
function onAppUnmounted (mounted: MountedDataApp): void {
  if (app.value !== mounted) return;
  stopWatching?.();
  stopWatching = undefined;
  app.value = undefined;
}
const panelOpen = computed(() => (panelVersion.value, app.value?.panel.open ?? false));
// The commands for the header: a plugin's `primary` ones as buttons, its `overflow` ones in a menu
// per icon, so render's downloads are one Download button; the palette has them all. Not those for
// the menu on the page (`target`), which the frame shows there, nor the panel's, in its strip.
const ICONS = ['inspect', 'download', 'table', 'copy', 'reload'] as const;
const chromeCommands = computed(() => runtime.value?.commands.filter((command) => !command.target) ?? []);
const toolbarCommands = computed(() => chromeCommands.value.filter((command) => command.placement !== 'overflow' && command.placement !== 'panel'));
const panelCommands = computed(() => chromeCommands.value.filter((command) => command.placement === 'panel'));
// An overflow command with no icon appserve draws goes in a More menu of its own.
const MENU_LABELS: Partial<Record<typeof ICONS[number], string>> = { download: 'Download' };
const overflowMenus = computed(() => {
  const menus = new Map<string, { icon: typeof ICONS[number] | 'more', label: string, commands: Command[] }>();
  for (const command of chromeCommands.value.filter((candidate) => candidate.placement === 'overflow')) {
    const icon = (ICONS as readonly string[]).includes(command.icon ?? '') ? command.icon as typeof ICONS[number] : 'more';
    if (!menus.has(icon)) menus.set(icon, { icon, label: (icon !== 'more' && MENU_LABELS[icon]) || 'More actions', commands: [] });
    menus.get(icon)!.commands.push(command);
  }
  return [...menus.values()];
});
const moreOpen = ref<string>();
function commandState (command: Command): { pressed?: boolean, disabled?: string } {
  void panelVersion.value;
  return app.value ? app.value.commandState(command.id) : { disabled: 'No Data App is open' };
}
const commandIcon = (command: Command) => (ICONS as readonly string[]).includes(command.icon ?? '') ? command.icon as typeof ICONS[number] : 'app';

// A command's failure shows for a few seconds, out of the page's way; so does what a rendering
// left out, so a blank in the saved picture isn't a surprise.
const toast = ref<{ text: string, error: boolean }>();
let toastTimer: ReturnType<typeof setTimeout> | undefined;
function showToast (text: string, error: boolean): void {
  toast.value = { text, error };
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toast.value = undefined; }, 8000);
}
async function runCommand (command: Command): Promise<void> {
  moreOpen.value = undefined;
  if (!app.value) return;
  try {
    const result = await app.value.run(command.id);
    if ('call' in command.run && command.run.call === 'render.download') {
      const { omitted } = result as Rendering;
      if (omitted.length) {
        showToast(`Saved, with ${omitted.length === 1 ? 'an image' : `${omitted.length} images`} left blank: `
          + `${omitted.join(', ')} ${omitted.length === 1 ? 'sends' : 'send'} no CORS headers.`, false);
      }
    }
  } catch (err) {
    showToast(`Couldn't run ${command.title}: ${(err as Error).message}`, true);
  }
}

const WIDTH_KEY = 'anfra-appserve:inspect-width';
const MIN_WIDTH = 280;
function loadWidth (): number {
  try {
    const saved = Number(window.localStorage.getItem(WIDTH_KEY));
    if (saved >= MIN_WIDTH) return saved;
  } catch { /* use the default */ }
  return 380;
}
const inspectWidth = ref(loadWidth());
const dragging = ref(false);

function startResize (event: PointerEvent): void {
  const handle = event.currentTarget as HTMLElement;
  handle.setPointerCapture(event.pointerId);
  dragging.value = true;
  const body = handle.closest('.viewer-body')?.getBoundingClientRect();
  const onMove = (move: PointerEvent): void => {
    if (!body) return;
    const max = Math.max(MIN_WIDTH, body.width - 240);
    inspectWidth.value = Math.round(Math.min(max, Math.max(MIN_WIDTH, body.right - move.clientX)));
  };
  const onUp = (): void => {
    dragging.value = false;
    handle.removeEventListener('pointermove', onMove);
    handle.removeEventListener('pointerup', onUp);
    try {
      window.localStorage.setItem(WIDTH_KEY, String(inspectWidth.value));
    } catch { /* the width just doesn't persist */ }
  };
  handle.addEventListener('pointermove', onMove);
  handle.addEventListener('pointerup', onUp);
}

const loading = ref(false);

const selected = computed(() => (selectedPath.value ? findApp(entries.value, selectedPath.value) : undefined));

async function loadCatalog (): Promise<void> {
  try {
    entries.value = await server.catalog();
    loadError.value = undefined;
  } catch (err) {
    loadError.value = `Couldn't load the Data Apps: ${(err as Error).message}`;
  } finally {
    loaded.value = true;
  }
}

function select (path: string): void {
  window.history.pushState(null, '', urlFor(path));
  selectedPath.value = pathFromLocation(window.location.pathname);
  if (narrow.value) sidebarOpen.value = false;
}

function reload (): void {
  revision.value += 1;
}

// Breadcrumb folders: open the sidebar, expand the folder and bring it into view.
async function revealFolder (path: string): Promise<void> {
  sidebarOpen.value = true;
  search.value = '';
  const next = new Set(collapsed.value);
  next.delete(path);
  setCollapsed(next);
  await nextTick();
  document.querySelector(`[data-folder="${CSS.escape(path)}"]`)?.scrollIntoView({ block: 'nearest' });
}

const crumbs = computed(() => {
  const path = selected.value?.path ?? selectedPath.value;
  return path ? folderPaths(path).map((p) => ({ path: p, name: p.split('/').pop() as string })) : [];
});
const title = computed(() => selected.value?.label ?? selectedPath.value?.split('/').pop() ?? 'Data Apps');

watch(() => (selected.value ? `${selected.value.path}#${revision.value}` : ''), (key) => {
  loading.value = key !== '';
}, { immediate: true });
watch(selectedPath, () => {
  keptPanel = undefined;
  app.value?.panel.hide();
  frameError.value = undefined;
});

// The command palette: the shell's own commands, the plugins', and every Data App to open.
const paletteOpen = ref(false);
const IS_MAC = /Mac|iPhone|iPad/.test(navigator.platform);
const PALETTE_SHORTCUT = IS_MAC ? '⌘K' : 'Ctrl+K';
const paletteEntries = computed<PaletteEntry[]>(() => {
  const noApp = selected.value ? undefined : 'Open a Data App first';
  const plugins = chromeCommands.value.map((command) => {
    const state = commandState(command);
    return {
      id: `command:${command.id}`, title: command.title, group: 'Data App', shortcut: command.shortcut, disabled: noApp ?? state.disabled, pressed: state.pressed,
    };
  });
  return [
    ...plugins,
    { id: 'reload', title: 'Reload Data App', group: 'Data App', disabled: noApp },
    { id: 'sidebar', title: sidebarOpen.value ? 'Hide sidebar' : 'Show sidebar', group: 'View' },
    { id: 'theme', title: theme.value === 'dark' ? 'Switch to light mode' : 'Switch to dark mode', group: 'View' },
    ...allApps(entries.value).map((entry) => ({
      id: `app:${entry.path}`, title: entry.label, group: 'Open a Data App', disabled: entry.path === selected.value?.path ? 'Open now' : undefined,
    })),
  ];
});
function runPaletteEntry (id: string): void {
  if (id.startsWith('command:')) {
    const command = runtime.value?.commands.find((candidate) => `command:${candidate.id}` === id);
    if (command) void runCommand(command);
  } else if (id.startsWith('app:')) {
    select(id.slice('app:'.length));
  } else if (id === 'reload') {
    reload();
  } else if (id === 'sidebar') {
    sidebarOpen.value = !sidebarOpen.value;
  } else if (id === 'theme') {
    toggleTheme();
  }
}

function onKeydown (event: KeyboardEvent): void {
  if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'k') {
    event.preventDefault();
    paletteOpen.value = !paletteOpen.value;
    return;
  }
  if (paletteOpen.value) return;
  // The plugins' shortcuts first (Alt+Shift+I for Inspect); the frame forwards its own.
  if (app.value?.handleKeydown(event)) return;
  const target = event.target as HTMLElement | null;
  const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
  if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey) {
    event.preventDefault();
    sidebarOpen.value = true;
    void nextTick(() => searchInput.value?.focus());
  }
  if (event.key === 'Escape') {
    problemsOpen.value = false;
    // Picking, Escape only ends the pick, as it does with focus in the frame; again, it closes the panel.
    if (app.value?.panel.picking) app.value.panel.pick(false);
    else app.value?.panel.hide();
  }
}

function onSearchKeydown (event: KeyboardEvent): void {
  if (event.key !== 'Escape') return;
  if (search.value) search.value = '';
  else (event.target as HTMLElement).blur();
}

function onDocumentClick (event: MouseEvent): void {
  if (!(event.target as HTMLElement).closest('.problems')) problemsOpen.value = false;
  if (!(event.target as HTMLElement).closest('.more')) moreOpen.value = undefined;
}

function onPopState (): void {
  selectedPath.value = pathFromLocation(window.location.pathname);
}

// Live reload, when the server has it on: a changed Data App reloads if it is the one open; a
// changed AML re-validates, and reloads the datasets and the open Data App.
let events: EventSource | undefined;

function onServerEvent (event: MessageEvent): void {
  const message = JSON.parse(event.data) as server.RepoEvent;
  if (message.type === 'apps') {
    void loadCatalog();
    if (selected.value && message.paths.includes(selected.value.path)) revision.value += 1;
  }
  if (message.type === 'aml') {
    void loadProblems();
    void loadDatasets().then(() => { revision.value += 1; });
  }
}

let healthTimer: ReturnType<typeof setInterval> | undefined;

onMounted(async () => {
  window.addEventListener('popstate', onPopState);
  window.addEventListener('keydown', onKeydown);
  document.addEventListener('click', onDocumentClick);
  narrowQuery.addEventListener('change', onNarrowChange);
  void loadCatalog();
  void loadProblems();
  void loadDatasets();
  void checkHealth();
  void loadVersion();
  healthTimer = setInterval(() => { void checkHealth(); }, 10_000);
  await loadContext();
  if (watching.value) {
    events = new EventSource('/appserve/events');
    events.addEventListener('message', onServerEvent);
  }
});
onBeforeUnmount(() => {
  window.removeEventListener('popstate', onPopState);
  window.removeEventListener('keydown', onKeydown);
  document.removeEventListener('click', onDocumentClick);
  narrowQuery.removeEventListener('change', onNarrowChange);
  events?.close();
  clearInterval(healthTimer);
  stopWatching?.();
});
</script>

<template>
  <div class="shell" :class="{ 'sidebar-closed': !sidebarOpen, narrow }">
    <div v-if="narrow && sidebarOpen" class="backdrop" @click="sidebarOpen = false" />
    <nav class="sidebar" aria-label="Data Apps" :inert="!sidebarOpen">
      <div class="brand">
        <Logo />
        <h1 data-testid="brand">{{ repoName || 'anfra' }}</h1>
        <button
          type="button"
          class="icon-button palette-open"
          :aria-label="`Command palette (${PALETTE_SHORTCUT})`"
          :title="`Command palette (${PALETTE_SHORTCUT})`"
          data-testid="palette-open"
          @click="paletteOpen = true"
        >
          <Icon name="command" />
        </button>
      </div>
      <label class="search">
        <Icon name="search" />
        <input
          ref="searchInput"
          v-model="search"
          type="search"
          placeholder="Search apps"
          aria-label="Search Data Apps"
          data-testid="search"
          @keydown="onSearchKeydown"
        >
        <kbd v-if="!search" aria-hidden="true">/</kbd>
      </label>
      <div class="tree-scroll">
        <p v-if="loadError" class="notice error" role="alert">{{ loadError }}</p>
        <p v-else-if="loaded && entries.length === 0" class="notice empty" data-testid="empty-state">
          No Data Apps yet. Add a Data App definition, an <code>.html</code> file, to the repo's
          <code>apps/</code> directory.
        </p>
        <p v-else-if="searching && filtered.length === 0" class="notice" data-testid="no-matches">No matches</p>
        <AppTree
          v-else
          :entries="filtered"
          :selected="selected?.path"
          :collapsed="collapsed"
          :force-open="searching"
          @select="select"
          @toggle="toggleFolder"
        />
      </div>
      <div class="sidebar-footer">
        <div class="footer-status">
          <p
            class="status"
            data-testid="anfra-status"
            :data-status="health"
            :title="health === 'up' && version ? `anfra is running on version ${version.current}` : undefined"
            role="status"
          >
            <span class="dot" aria-hidden="true" />{{ statusLabel }}
          </p>
          <span
            v-if="version?.latest && health !== 'unreachable'"
            class="update-badge"
            data-testid="update-badge"
            role="note"
            :aria-label="updateHint"
            :title="updateHint"
          >
            <Icon name="update" />Update
          </span>
        </div>
        <button
          type="button"
          class="icon-button"
          :aria-label="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
          :title="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
          data-testid="theme-toggle"
          @click="toggleTheme"
        >
          <Icon :name="theme === 'dark' ? 'sun' : 'moon'" />
        </button>
      </div>
    </nav>

    <main class="viewer" data-testid="viewer">
      <header class="viewer-header">
        <button
          type="button"
          class="icon-button"
          :aria-label="sidebarOpen ? 'Hide sidebar' : 'Show sidebar'"
          :aria-expanded="sidebarOpen"
          title="Toggle sidebar"
          data-testid="sidebar-toggle"
          @click="sidebarOpen = !sidebarOpen"
        >
          <Icon name="sidebar" />
        </button>
        <nav class="breadcrumb" aria-label="Breadcrumb">
          <template v-for="crumb in crumbs" :key="crumb.path">
            <button type="button" class="crumb" data-testid="crumb" :data-crumb="crumb.path" @click="revealFolder(crumb.path)">
              {{ crumb.name }}
            </button>
            <Icon name="chevron" class="sep" />
          </template>
          <h2 class="current" data-testid="viewer-title">{{ title }}</h2>
        </nav>
        <div class="actions">
          <div v-if="problems.length" class="problems">
            <button
              type="button"
              class="badge"
              data-testid="aml-badge"
              :aria-expanded="problemsOpen"
              @click="problemsOpen = !problemsOpen"
            >
              <Icon name="alert" />{{ problems.length === 1 ? '1 problem' : `${problems.length} problems` }}
            </button>
            <section v-if="problemsOpen" class="popover" role="alert" data-testid="aml-banner">
              <strong>The repo has {{ problems.length === 1 ? 'a problem' : `${problems.length} problems` }}.</strong>
              <span class="hint">Queries may fail until it's fixed.</span>
              <ul>
                <li v-for="(problem, i) in problems" :key="i">
                  <code v-if="where(problem)">{{ where(problem) }}</code> {{ problem.message }}
                </li>
              </ul>
            </section>
          </div>
          <button
            type="button"
            class="icon-button"
            aria-label="Reload Data App"
            title="Reload Data App"
            data-testid="reload"
            :disabled="!selected"
            @click="reload"
          >
            <Icon name="reload" />
          </button>
          <button
            v-for="command in toolbarCommands"
            :key="command.id"
            type="button"
            class="icon-button"
            :class="{ active: commandState(command).pressed }"
            :aria-label="command.title"
            :title="commandState(command).disabled ?? (command.shortcut ? `${command.title} (${command.shortcut})` : command.title)"
            :data-testid="`command-${command.id}`"
            :aria-pressed="commandState(command).pressed ?? false"
            :disabled="!selected || !!commandState(command).disabled"
            @click="runCommand(command)"
          >
            <Icon :name="commandIcon(command)" />
          </button>
          <div v-for="menu in overflowMenus" :key="menu.icon" class="more">
            <button
              type="button"
              class="icon-button"
              :aria-label="menu.label"
              :title="menu.label"
              :data-testid="`more-${menu.icon}`"
              aria-haspopup="menu"
              :aria-expanded="moreOpen === menu.icon"
              :disabled="!selected"
              @click="moreOpen = moreOpen === menu.icon ? undefined : menu.icon"
            >
              <Icon :name="menu.icon" />
            </button>
            <div v-if="moreOpen === menu.icon" class="menu" role="menu" :data-testid="`more-${menu.icon}-menu`">
              <button
                v-for="command in menu.commands"
                :key="command.id"
                type="button"
                role="menuitem"
                :title="commandState(command).disabled"
                :disabled="!!commandState(command).disabled"
                :data-testid="`command-${command.id}`"
                @click="runCommand(command)"
              >
                {{ command.title }}
              </button>
            </div>
          </div>
        </div>
        <div v-if="loading" class="progress" role="progressbar" aria-label="Loading Data App" data-testid="loading" />
      </header>
      <div class="viewer-body" :class="{ dragging }">
        <p v-if="toast" class="toast" :class="{ error: toast.error }" :role="toast.error ? 'alert' : 'status'" data-testid="render-toast">{{ toast.text }}</p>
        <p v-if="selected && frameError" class="notice error" role="alert" data-testid="frame-error">{{ frameError }}</p>
        <DataAppFrame
          v-else-if="selected && datasets && reader"
          :key="`${selected.path}#${revision}`"
          :path="selected.path"
          :datasets="datasets"
          :reader="reader"
          :allow-origins="allowOrigins"
          :theme="pluginTheme"
          @load="loading = false"
          @mounted="onAppMounted"
          @unmounted="onAppUnmounted"
          @error="frameError = $event"
        />
        <div v-else-if="selectedPath && loaded" class="empty-state" data-testid="not-found">
          <Icon name="app" />
          <p>No Data App at <code>{{ selectedPath }}</code>.</p>
        </div>
        <div v-else class="empty-state">
          <Icon name="app" />
          <p>Pick a Data App on the left.</p>
        </div>
        <template v-if="panelOpen && selected && app && runtime">
          <div v-if="narrow" class="backdrop" @click="app.panel.hide()" />
          <div class="inspect-dock" :style="{ width: `${inspectWidth}px` }">
            <div class="resize-handle" role="separator" aria-orientation="vertical" @pointerdown.prevent="startResize" />
            <PluginPanel
              :panel="app.panel"
              :tabs="runtime.tabs"
              :commands="panelCommands"
              :command-state="commandState"
              :version="panelVersion"
              @run="runCommand"
            />
          </div>
        </template>
      </div>
    </main>
    <CommandPalette v-if="paletteOpen" :entries="paletteEntries" @run="runPaletteEntry" @close="paletteOpen = false" />
  </div>
</template>
