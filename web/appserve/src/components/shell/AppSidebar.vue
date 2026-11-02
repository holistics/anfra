<script setup lang="ts">
// As anfra Cloud's inset sidebar.
import {
  CommandIcon, Moon, SearchIcon, Sun,
} from '@lucide/vue';
import { computed, ref } from 'vue';
import Logo from '@/components/Logo.vue';
import { Button } from '@/components/ui/button';
import { Kbd } from '@/components/ui/kbd';
import { StatusPill } from '@/components/ui/status-pill';
import { filterEntries, type CatalogEntry } from '@/lib/catalog';
import type * as server from '@/lib/server';
import AppTree from './AppTree.vue';
import InlineCode from './InlineCode.vue';
import { sidebarGroupLabel } from './sidebar-menu';

const props = defineProps<{
  open: boolean;
  narrow: boolean;
  repoName?: string;
  entries: CatalogEntry[];
  loaded: boolean;
  loadError?: string;
  selected?: string;
  collapsed: Set<string>;
  health: server.Health | 'unknown';
  version?: server.Version;
  theme: 'light' | 'dark';
  paletteShortcut: string;
}>();

const emit = defineEmits<{
  select: [path: string];
  toggle: [path: string];
  openPalette: [];
  toggleTheme: [];
}>();

const search = ref('');
const searchInput = ref<HTMLInputElement>();
const filtered = computed(() => filterEntries(props.entries, search.value));
const searching = computed(() => search.value.trim() !== '');

function onSearchKeydown (event: KeyboardEvent): void {
  if (event.key !== 'Escape') return;
  if (search.value) search.value = '';
  else (event.target as HTMLElement).blur();
}

// A release version reads as v0.4.2; a local build's ("dev") as itself.
const versionLabel = computed(() => {
  const current = props.version?.current;
  if (!current) return undefined;
  return /^\d/.test(current) ? `v${current}` : current;
});

const statusLabel = computed(() => {
  if (props.health === 'unreachable') return 'anfra server unreachable';
  if (props.health === 'up') return versionLabel.value ? `anfra ${versionLabel.value}` : 'anfra is running';
  if (props.health === 'down') return 'anfra is down';
  return 'Checking anfra…';
});

const updateHint = computed(() => (props.version?.latest
  ? `anfra ${props.version.latest} is available (you have ${props.version.current}). Run \`anfra update\`, then restart anfra serve.`
  : ''));

const notice = 'mx-2 text-caption';

defineExpose({
  focusSearch: () => searchInput.value?.focus(),
  clearSearch: () => { search.value = ''; },
});
</script>

<template>
  <!-- Header, group and footer each pad 8px inside the sidebar's 8px. Narrow, it floats over the viewer. -->
  <nav
    class="flex w-64 min-w-0 flex-col overflow-hidden p-2 text-sidebar-foreground duration-180 ease-[ease]"
    :class="[
      narrow ? 'fixed inset-y-0 left-0 z-20 bg-sidebar shadow-e2 transition-[translate,visibility]' : 'transition-[visibility]',
      !open && (narrow ? 'invisible -translate-x-full' : 'invisible'),
    ]"
    aria-label="Data Apps"
    :inert="!open"
  >
    <div class="flex shrink-0 flex-col gap-2 p-2">
      <div class="flex h-8 items-center gap-1">
        <Logo class="mx-1.5 size-5 shrink-0 fill-(--anfra-color-icon-logo)" />
        <h1 class="min-w-0 flex-1 truncate px-1 text-label text-default" data-testid="brand">{{ repoName || 'anfra' }}</h1>
        <Button
          variant="ghost"
          size="icon-sm"
          :aria-label="`Command palette (${paletteShortcut})`"
          :title="`Command palette (${paletteShortcut})`"
          data-testid="palette-open"
          @click="emit('openPalette')"
        >
          <CommandIcon />
        </Button>
      </div>
      <!-- The key hint sits 6px inside the field's edge, so its corner is the field's less 6px. -->
      <label
        class="flex h-8 items-center gap-2 rounded-lg border border-strong bg-background pr-[5px] pl-2.5 icon-weak transition-[border-color] duration-160 ease-anfra-out hover:border-hover focus-within:border-highlight-strong focus-within:shadow-[inset_0_0_0_1px_var(--anfra-color-border-highlight-strong)]"
      >
        <SearchIcon class="size-4 shrink-0" />
        <input
          ref="searchInput"
          v-model="search"
          type="search"
          class="min-w-0 flex-1 appearance-none bg-transparent px-0.5 py-px text-default outline-none! placeholder:text-placeholder [&::-webkit-search-cancel-button]:appearance-none"
          placeholder="Search apps"
          aria-label="Search Data Apps"
          data-testid="search"
          @keydown="onSearchKeydown"
        >
        <Kbd v-if="!search" class="rounded-[calc(var(--radius-lg)-6px)]" aria-hidden="true">/</Kbd>
      </label>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="flex w-full min-w-0 flex-col p-2">
        <div :class="sidebarGroupLabel">Data Apps</div>
        <p v-if="loadError" :class="[notice, 'text-danger-default']" role="alert">{{ loadError }}</p>
        <p v-else-if="loaded && entries.length === 0" :class="[notice, 'text-placeholder']" data-testid="empty-state">
          No Data Apps yet. Add a Data App definition, an <InlineCode>.html</InlineCode> file, to the repo's
          <InlineCode>apps/</InlineCode> directory.
        </p>
        <p v-else-if="searching && filtered.length === 0" :class="[notice, 'text-placeholder']" data-testid="no-matches">No matches</p>
        <AppTree
          v-else
          :entries="filtered"
          :selected="selected"
          :collapsed="collapsed"
          :force-open="searching"
          @select="emit('select', $event)"
          @toggle="emit('toggle', $event)"
        />
      </div>
    </div>
    <div class="flex shrink-0 items-center justify-between gap-2 p-2">
      <div class="flex min-w-0 items-center gap-1.5">
        <StatusPill
          class="min-w-0 overflow-hidden text-ellipsis"
          :tone="health === 'up' ? 'success' : health === 'unknown' ? 'neutral' : 'danger'"
          data-testid="anfra-status"
          :data-status="health"
          :title="health === 'up' && version ? `anfra is running on version ${version.current}` : undefined"
          role="status"
        >
          {{ statusLabel }}
        </StatusPill>
        <StatusPill
          v-if="version?.latest && health !== 'unreachable'"
          class="cursor-help"
          tone="highlight"
          data-testid="update-badge"
          role="note"
          :aria-label="updateHint"
          :title="updateHint"
        >
          Update
        </StatusPill>
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        :aria-label="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
        :title="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
        data-testid="theme-toggle"
        @click="emit('toggleTheme')"
      >
        <Sun v-if="theme === 'dark'" />
        <Moon v-else />
      </Button>
    </div>
  </nav>
</template>
