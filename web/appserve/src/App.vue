<script setup lang="ts">
import {
  computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch,
} from 'vue';
import {
  LayoutDashboard, Moon, PanelLeft, Sun,
} from '@lucide/vue';
import CommandPalette, { type PaletteEntry } from '@/components/apps/CommandPalette.vue';
import DataAppFrame from '@/components/apps/DataAppFrame.vue';
import PluginPanel from '@/components/apps/PluginPanel.vue';
import {
  AppHeader, AppSidebar, EmptyState, InlineCode, Scrim,
} from '@/components/shell';
import type {
  DatasetDescriptor, Rendering, ThemeInput, User,
} from '@holistics/anfra-sdk/common';
import { designTheme } from '@holistics/anfra-sdk/common';
import type { Command, Host, MountedDataApp } from '@holistics/anfra-sdk/host';
import {
  allApps, findApp, folderPaths, pathFromLocation, urlFor, type CatalogEntry,
} from '@/lib/catalog';
import { layerOpen } from '@/lib/layers';
import { commandIcon, overflowMenus } from '@/lib/pluginUI';
import * as server from '@/lib/server';

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
const appLogs = ref(true);
// What each Data App is provisioned with; reloaded when the AML changes.
const datasets = ref<Record<string, DatasetDescriptor>>();
const frameError = ref<string>();
// The anfra serving, and a newer release when the server knows of one.
const version = ref<server.Version>();

async function checkHealth (): Promise<void> {
  const was = health.value;
  health.value = await server.health();
  // Back after an outage: catch up on anything missed. A restart may also be onto a new version.
  if (was === 'unreachable' && health.value !== 'unreachable') {
    void refreshAll();
    void loadVersion();
  }
}

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
    appLogs.value = ctx.appLogs;
    contextProblems.value = ctx.problems;
  } catch { /* the status says why */ }
}

async function refreshAll (): Promise<void> {
  await Promise.all([loadCatalog(), loadProblems(), loadDatasets(), loadContext()]);
  revision.value += 1;
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

// The host runtime and the open Data App's mount, for the plugins' commands and its panel. The
// panel belongs to one Data App: it closes when another is picked, and survives a live reload.
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
// The Data App went (a live reload, another one picked): nothing runs on its mount until the next one is.
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
const chromeCommands = computed(() => runtime.value?.commands.filter((command) => !command.target) ?? []);
const toolbarCommands = computed(() => chromeCommands.value.filter((command) => command.placement !== 'overflow' && command.placement !== 'panel'));
const panelCommands = computed(() => chromeCommands.value.filter((command) => command.placement === 'panel'));
const menus = computed(() => overflowMenus(chromeCommands.value));
function commandState (command: Command): { pressed?: boolean, disabled?: string } {
  void panelVersion.value;
  return app.value ? app.value.commandState(command.id) : { disabled: 'No Data App is open' };
}

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
const viewerBody = ref<HTMLElement>();

function startResize (event: PointerEvent): void {
  const handle = event.currentTarget as HTMLElement;
  handle.setPointerCapture(event.pointerId);
  dragging.value = true;
  const body = viewerBody.value?.getBoundingClientRect();
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

// Breadcrumb folders: open the sidebar, expand the folder and bring it into view.
async function revealFolder (path: string): Promise<void> {
  sidebarOpen.value = true;
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
const paletteEntries = computed<PaletteEntry[]>(() => {
  const noApp = selected.value ? undefined : 'Open a Data App first';
  const plugins = chromeCommands.value.map((command): PaletteEntry => {
    const state = commandState(command);
    return {
      id: `command:${command.id}`, title: command.title, group: 'Data App', icon: commandIcon(command), shortcut: command.shortcut, disabled: noApp ?? state.disabled, pressed: state.pressed,
    };
  });
  return [
    ...plugins,
    { id: 'sidebar', title: sidebarOpen.value ? 'Hide sidebar' : 'Show sidebar', group: 'View', icon: PanelLeft },
    { id: 'theme', title: theme.value === 'dark' ? 'Switch to light mode' : 'Switch to dark mode', group: 'View', icon: theme.value === 'dark' ? Sun : Moon },
    ...allApps(entries.value).map((entry): PaletteEntry => ({
      id: `app:${entry.path}`, title: entry.label, group: 'Open a Data App', icon: LayoutDashboard, disabled: entry.path === selected.value?.path ? 'Already open' : undefined,
    })),
  ];
});
function runPaletteEntry (id: string): void {
  if (id.startsWith('command:')) {
    const command = runtime.value?.commands.find((candidate) => `command:${candidate.id}` === id);
    if (command) void runCommand(command);
  } else if (id.startsWith('app:')) {
    select(id.slice('app:'.length));
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
    paletteOpen.value = true;
  }
  if (event.key === 'Escape' && !layerOpen()) {
    // Picking, Escape only ends the pick, as it does with focus in the frame; again, it closes the panel.
    if (app.value?.panel.picking) app.value.panel.pick(false);
    else app.value?.panel.hide();
  }
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
  narrowQuery.removeEventListener('change', onNarrowChange);
  events?.close();
  clearInterval(healthTimer);
  stopWatching?.();
});
</script>

<template>
  <!-- The sidebar beside the viewer, 256px; closed, it slides to nothing. Narrow, the sidebar and the
       panel float over the viewer instead, over a scrim. -->
  <div
    class="grid h-screen transition-[grid-template-columns] duration-180 ease-[ease]"
    :class="narrow ? 'grid-cols-[minmax(0,1fr)]' : sidebarOpen ? 'grid-cols-[256px_minmax(0,1fr)]' : 'grid-cols-[0_minmax(0,1fr)]'"
  >
    <Scrim v-if="narrow && sidebarOpen" @click="sidebarOpen = false" />
    <AppSidebar
      :open="sidebarOpen"
      :narrow="narrow"
      :repo-name="repoName"
      :entries="entries"
      :loaded="loaded"
      :load-error="loadError"
      :selected="selected?.path"
      :collapsed="collapsed"
      :health="health"
      :version="version"
      :theme="theme"
      @select="select"
      @toggle="toggleFolder"
      @open-palette="paletteOpen = true"
      @toggle-theme="toggleTheme"
    />

    <main
      class="relative my-2 mr-2 flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border border-default bg-background"
      :class="narrow || !sidebarOpen ? 'ml-2' : 'ml-0'"
      data-testid="viewer"
    >
      <AppHeader
        :sidebar-open="sidebarOpen"
        :has-app="!!selected"
        :title="title"
        :crumbs="crumbs"
        :problems="problems"
        :loading="loading"
        :commands="toolbarCommands"
        :menus="menus"
        :command-state="commandState"
        @toggle-sidebar="sidebarOpen = !sidebarOpen"
        @reveal-folder="revealFolder"
        @run="runCommand"
      />
      <div
        ref="viewerBody"
        class="relative flex min-h-0 flex-1"
        :class="dragging && 'cursor-col-resize select-none'"
        :data-pressing="dragging || undefined"
      >
        <p
          v-if="toast"
          class="fixed right-4 bottom-4 z-20 max-w-[min(480px,90vw)] rounded-[8px] border px-3.5 py-2.5 shadow-e2"
          :class="toast.error ? 'border-transparent bg-danger-secondary-default text-danger-default' : 'border-default bg-background'"
          :role="toast.error ? 'alert' : 'status'"
          data-testid="render-toast"
        >
          {{ toast.text }}
        </p>
        <p v-if="selected && frameError" class="m-2 text-danger-default" role="alert" data-testid="frame-error">{{ frameError }}</p>
        <DataAppFrame
          v-else-if="selected && datasets && reader"
          :key="`${selected.path}#${revision}`"
          :path="selected.path"
          :datasets="datasets"
          :reader="reader"
          :allow-origins="allowOrigins"
          :app-logs="appLogs"
          :theme="pluginTheme"
          @load="loading = false"
          @mounted="onAppMounted"
          @unmounted="onAppUnmounted"
          @error="frameError = $event"
        />
        <EmptyState v-else-if="selectedPath && loaded" data-testid="not-found">
          No Data App at <InlineCode>{{ selectedPath }}</InlineCode>.
        </EmptyState>
        <EmptyState v-else>Pick a Data App on the left.</EmptyState>
        <template v-if="panelOpen && selected && app && runtime">
          <Scrim v-if="narrow" @click="app.panel.hide()" />
          <div
            class="flex shrink-0 border-l border-default bg-background"
            :class="narrow ? 'fixed inset-y-0 right-0 z-20 w-[min(380px,100vw)] shadow-e2' : 'relative'"
            :style="narrow ? undefined : { width: `${inspectWidth}px` }"
          >
            <div
              v-if="!narrow"
              class="absolute inset-y-0 -left-[3px] z-2 w-1.5 cursor-col-resize touch-none hover:bg-interactive-highlight-default"
              role="separator"
              aria-orientation="vertical"
              @pointerdown.prevent="startResize"
            />
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
    <CommandPalette v-model:open="paletteOpen" :entries="paletteEntries" @run="runPaletteEntry" />
  </div>
</template>
