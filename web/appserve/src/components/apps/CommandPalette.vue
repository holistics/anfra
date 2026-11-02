<script setup lang="ts">
// Cmd/Ctrl+K: run one of the shell's commands or the plugins', or open another Data App. What each
// entry does, and whether it can run, the shell decides.
import { computed, type Component } from 'vue';
import {
  CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList,
} from '@/components/ui/command';
import { ShortcutKeys } from '@/components/shell';
import { Badge } from '@/components/ui/badge';

export interface PaletteEntry {
  id: string;
  title: string;
  group: string;
  icon?: Component;
  shortcut?: string;
  /** Why it can't run now: it shows, greyed, and can't be chosen. */
  disabled?: string;
  /** On now, such as an open panel. */
  pressed?: boolean;
}

const open = defineModel<boolean>('open', { required: true });
const props = defineProps<{ entries: PaletteEntry[] }>();
const emit = defineEmits<{ run: [id: string] }>();

const groups = computed(() => {
  const byGroup = new Map<string, PaletteEntry[]>();
  for (const entry of props.entries) {
    if (!byGroup.has(entry.group)) byGroup.set(entry.group, []);
    byGroup.get(entry.group)!.push(entry);
  }
  return [...byGroup];
});

function choose (entry: PaletteEntry): void {
  open.value = false;
  emit('run', entry.id);
}
</script>

<template>
  <CommandDialog
    v-model:open="open"
    title="Command palette"
    description="Run a command, or open a Data App"
    class="top-[12vh] flex max-h-[min(440px,76vh)] w-[min(560px,calc(100%-2rem))] max-w-none flex-col sm:max-w-none"
  >
    <CommandInput placeholder="Search apps and commands" data-testid="palette-input" />
    <CommandList data-testid="command-palette">
      <CommandEmpty>No commands match.</CommandEmpty>
      <CommandGroup v-for="[group, items] in groups" :key="group" :heading="group">
        <CommandItem
          v-for="entry in items"
          :key="entry.id"
          :value="entry.id"
          :filter-value="`${entry.title} ${entry.group}`"
          :disabled="!!entry.disabled"
          :title="entry.disabled"
          :data-testid="`palette-${entry.id}`"
          @select="choose(entry)"
        >
          <component :is="entry.icon" v-if="entry.icon" />
          <span class="min-w-0 flex-1 truncate">{{ entry.title }}</span>
          <Badge v-if="entry.pressed" variant="secondary">On</Badge>
          <span v-if="entry.disabled" class="shrink-0 text-caption text-placeholder">{{ entry.disabled }}</span>
          <ShortcutKeys v-else-if="entry.shortcut" class="ml-auto" :combo="entry.shortcut" />
        </CommandItem>
      </CommandGroup>
    </CommandList>
  </CommandDialog>
</template>
