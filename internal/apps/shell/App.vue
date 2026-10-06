<script setup lang="ts">
import {
  computed, nextTick, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import AppTree from './AppTree.vue';
import DataAppFrame from './DataAppFrame.vue';
import Icon from './Icon.vue';
import InspectPanel from './InspectPanel.vue';
import type { InspectedApp } from './inspect';
import {
  filterEntries, findApp, folderPaths, pathFromLocation, urlFor, type CatalogEntry,
} from './catalog';

const entries = ref<CatalogEntry[]>([]);
const loaded = ref(false);
const loadError = ref<string>();
const selectedPath = ref(pathFromLocation(window.location.pathname));
// Bumped to reload the running Data App: its file changed, or the Datasets it was provisioned with.
const revision = ref(0);

interface AmlProblem { file?: string, line?: number, column?: number, message: string }
// anfra's health, and whether the Shell can still hear the demo server at all.
const anfraStatus = ref<'up' | 'down' | 'unknown'>('unknown');
const serverReachable = ref(true);
const problems = ref<AmlProblem[]>([]);
const dataFolder = ref<string>();

const statusLabel = computed(() => {
  if (!serverReachable.value) return 'Demo server unreachable';
  if (anfraStatus.value === 'up') return 'anfra is running';
  if (anfraStatus.value === 'down') return 'anfra is down';
  return 'Checking anfra…';
});

async function loadStatus (): Promise<void> {
  try {
    const status = await fetch('/_anfra/api/status').then((r) => r.json()) as { anfra: 'up' | 'down', problems: AmlProblem[], dataFolder?: string };
    anfraStatus.value = status.anfra;
    problems.value = status.problems ?? [];
    dataFolder.value = status.dataFolder;
  } catch {
    anfraStatus.value = 'unknown';
  }
}

function where (problem: AmlProblem): string {
  if (!problem.file) return '';
  return problem.line ? `${problem.file}:${problem.line}${problem.column ? `:${problem.column}` : ''}` : problem.file;
}

const COLLAPSED_KEY = 'anfra-demo:collapsed-folders';
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

const THEME_KEY = 'anfra-demo:theme';
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
function toggleTheme (): void {
  theme.value = theme.value === 'dark' ? 'light' : 'dark';
  try {
    window.localStorage.setItem(THEME_KEY, theme.value);
  } catch { /* the choice just doesn't persist */ }
}

const problemsOpen = ref(false);

// The Inspect panel belongs to one Data App: it closes when another is picked, and survives Reload.
const inspectOpen = ref(false);
const inspected = ref<InspectedApp[]>();
const WIDTH_KEY = 'anfra-demo:inspect-width';
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
    const response = await fetch('/_anfra/api/apps');
    if (!response.ok) throw new Error(`The server answered ${response.status}.`);
    entries.value = await response.json();
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
  inspected.value = undefined;
}, { immediate: true });
watch(selectedPath, () => {
  inspectOpen.value = false;
});

function onKeydown (event: KeyboardEvent): void {
  const target = event.target as HTMLElement | null;
  const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
  if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey) {
    event.preventDefault();
    sidebarOpen.value = true;
    void nextTick(() => searchInput.value?.focus());
  }
  if (event.key === 'Escape') {
    problemsOpen.value = false;
    inspectOpen.value = false;
  }
}

function onSearchKeydown (event: KeyboardEvent): void {
  if (event.key !== 'Escape') return;
  if (search.value) search.value = '';
  else (event.target as HTMLElement).blur();
}

function onDocumentClick (event: MouseEvent): void {
  if (!(event.target as HTMLElement).closest('.problems')) problemsOpen.value = false;
}

function onPopState (): void {
  selectedPath.value = pathFromLocation(window.location.pathname);
}

let events: EventSource | undefined;

function onServerEvent (event: MessageEvent): void {
  const message = JSON.parse(event.data) as { type: string, paths?: string[], problems?: AmlProblem[], status?: 'up' | 'down' };
  if (message.type === 'validation') problems.value = message.problems ?? [];
  if (message.type === 'anfra' && message.status) anfraStatus.value = message.status;
  if (message.type === 'apps') {
    void loadCatalog();
    if (selected.value && message.paths?.includes(selected.value.path)) revision.value += 1;
  }
  if (message.type === 'datasets') revision.value += 1;
}

onMounted(() => {
  window.addEventListener('popstate', onPopState);
  window.addEventListener('keydown', onKeydown);
  document.addEventListener('click', onDocumentClick);
  narrowQuery.addEventListener('change', onNarrowChange);
  void loadCatalog();
  void loadStatus();
  events = new EventSource('/_anfra/api/events');
  events.addEventListener('message', onServerEvent);
  events.addEventListener('open', () => {
    // Reconnected after the server came back: catch up on anything missed.
    if (!serverReachable.value) {
      serverReachable.value = true;
      void loadStatus();
      void loadCatalog();
    }
  });
  events.addEventListener('error', () => { serverReachable.value = false; });
});
onBeforeUnmount(() => {
  window.removeEventListener('popstate', onPopState);
  window.removeEventListener('keydown', onKeydown);
  document.removeEventListener('click', onDocumentClick);
  narrowQuery.removeEventListener('change', onNarrowChange);
  events?.close();
});
</script>

<template>
  <div class="shell" :class="{ 'sidebar-closed': !sidebarOpen, narrow }">
    <div v-if="narrow && sidebarOpen" class="backdrop" @click="sidebarOpen = false" />
    <nav class="sidebar" aria-label="Data Apps" :inert="!sidebarOpen">
      <div class="brand">
        <span class="logo"><Icon name="brand" /></span>
        <h1 data-testid="brand">{{ dataFolder || 'anfra demo' }}</h1>
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
          No Data Apps yet. Add an <code>.html</code> file to the Data Folder's <code>apps/</code>
          directory.
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
        <p
          class="status"
          data-testid="anfra-status"
          :data-status="serverReachable ? anfraStatus : 'unreachable'"
          role="status"
        >
          <span class="dot" aria-hidden="true" />{{ statusLabel }}
        </p>
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
              <strong>The Data Folder's AML has {{ problems.length === 1 ? 'a problem' : `${problems.length} problems` }}.</strong>
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
            :class="{ active: inspectOpen }"
            aria-label="Inspect Data App"
            title="Inspect Data App"
            data-testid="inspect"
            :aria-pressed="inspectOpen"
            :disabled="!selected"
            @click="inspectOpen = !inspectOpen"
          >
            <Icon name="inspect" />
          </button>
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
        </div>
        <div v-if="loading" class="progress" role="progressbar" aria-label="Loading Data App" data-testid="loading" />
      </header>
      <div class="viewer-body" :class="{ dragging }">
        <DataAppFrame
          v-if="selected"
          :key="`${selected.path}#${revision}`"
          :path="selected.path"
          :inspecting="inspectOpen"
          @load="loading = false"
          @inspect="inspected = $event as InspectedApp[]"
        />
        <div v-else-if="selectedPath && loaded" class="empty-state" data-testid="not-found">
          <Icon name="app" />
          <p>No Data App at <code>{{ selectedPath }}</code>.</p>
        </div>
        <div v-else class="empty-state">
          <Icon name="app" />
          <p>Pick a Data App on the left.</p>
        </div>
        <template v-if="inspectOpen && selected">
          <div v-if="narrow" class="backdrop" @click="inspectOpen = false" />
          <div class="inspect-dock" :style="{ width: `${inspectWidth}px` }">
            <div class="resize-handle" role="separator" aria-orientation="vertical" @pointerdown.prevent="startResize" />
            <InspectPanel :apps="inspected" @close="inspectOpen = false" />
          </div>
        </template>
      </div>
    </main>
  </div>
</template>
