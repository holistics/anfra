<script setup lang="ts">
// What the sidebar holds: the repo's name, the search, the Data Apps tree and the foot. AppSidebar
// puts it in the sidebar; SidebarPeek puts the same component in a floating panel while the
// sidebar is collapsed, so the two never drift. The search is drawn as a field and opens the
// command palette, which finds Data Apps and commands alike.
import { SearchIcon } from '@lucide/vue';
import Logo from '@/components/Logo.vue';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import type { CatalogEntry } from '@/lib/catalog';
import type * as server from '@/lib/server';
import { IS_MAC } from '@/lib/shortcut';
import type { ThemeChoice } from '@/lib/theme';
import AppTree from './AppTree.vue';
import InlineCode from './InlineCode.vue';
import ShortcutKeys from './ShortcutKeys.vue';
import SidebarFoot from './SidebarFoot.vue';
import { sidebarGroupLabel } from './sidebar-menu';

defineProps<{
  repoName?: string;
  entries: CatalogEntry[];
  loaded: boolean;
  loadError?: string;
  selected?: string;
  collapsed: Set<string>;
  health: server.Health | 'unknown';
  version?: server.Version;
  themeChoice: ThemeChoice;
  theme: 'light' | 'dark';
}>();

const emit = defineEmits<{
  select: [path: string];
  toggle: [path: string];
  openPalette: [];
  setTheme: [choice: ThemeChoice];
  retry: [];
}>();

const notice = 'mx-2 text-caption';
const paletteShortcut = IS_MAC ? 'Meta+K' : 'Ctrl+K';
</script>

<template>
  <!-- Header, group and foot each pad 8px inside the sidebar's 8px. -->
  <div class="flex shrink-0 flex-col gap-2 p-2">
    <div class="flex h-8 items-center gap-1">
      <Logo class="mx-1.5 size-5 shrink-0 fill-(--anfra-color-icon-logo)" />
      <h1 class="min-w-0 flex-1 truncate px-1 text-label text-default" data-testid="brand">{{ repoName || 'anfra' }}</h1>
    </div>
    <!-- Drawn as the search field; the key hint sits 6px inside its edge, so its corner is the field's less 6px. -->
    <button
      type="button"
      class="flex h-8 w-full items-center gap-2 rounded-lg border border-strong bg-background pr-[5px] pl-2.5 text-left icon-weak outline-none! transition-[border-color] duration-160 ease-anfra-out hover:border-hover focus-visible:border-highlight-strong focus-visible:shadow-[inset_0_0_0_1px_var(--anfra-color-border-highlight-strong)]"
      aria-haspopup="dialog"
      aria-label="Search apps and commands"
      :aria-keyshortcuts="paletteShortcut.replace('Ctrl', 'Control')"
      data-testid="palette-open"
      @click="emit('openPalette')"
    >
      <SearchIcon class="size-4 shrink-0" />
      <span class="min-w-0 flex-1 truncate text-placeholder">Search</span>
      <ShortcutKeys :combo="paletteShortcut" class="[&>[data-slot=kbd]]:rounded-[calc(var(--radius-lg)-6px)]" />
    </button>
  </div>
  <div class="min-h-0 flex-1 overflow-y-auto">
    <div class="flex w-full min-w-0 flex-col p-2">
      <div :class="sidebarGroupLabel">Data Apps</div>
      <!-- The tree stays as the last answer left it; a failed read shows only when there is none. -->
      <AppTree
        v-if="entries.length"
        :entries="entries"
        :selected="selected"
        :collapsed="collapsed"
        @select="emit('select', $event)"
        @toggle="emit('toggle', $event)"
      />
      <div v-else-if="loadError" class="mx-2 flex flex-col items-start gap-1" data-testid="load-error">
        <p class="text-caption text-danger-default" role="alert">{{ loadError }}</p>
        <Button variant="link" size="xs" class="px-0" data-testid="retry" @click="emit('retry')">Try again</Button>
      </div>
      <div v-else-if="!loaded" role="status" class="flex flex-col gap-2 px-2 py-1" data-testid="loading-apps">
        <span class="sr-only">Loading the Data Apps</span>
        <Skeleton class="h-4 w-3/4" />
        <Skeleton class="h-4 w-1/2" />
        <Skeleton class="h-4 w-2/3" />
      </div>
      <p v-else :class="[notice, 'text-placeholder']" data-testid="empty-state">
        No Data Apps yet. Add a Data App definition, an <InlineCode>.html</InlineCode> file, to the repo's
        <InlineCode>apps/</InlineCode> directory.
      </p>
    </div>
  </div>
  <SidebarFoot
    :health="health"
    :version="version"
    :theme-choice="themeChoice"
    :theme="theme"
    @set-theme="emit('setTheme', $event)"
  />
</template>
