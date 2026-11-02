<script setup lang="ts">
// The command palette: every command the shell offers, its own and the plugins', found by typing
// and run with Enter. What each entry does, and whether it can run, the shell decides.
import { computed, nextTick, ref, watch } from 'vue';
import { SearchIcon } from '@lucide/vue';

export interface PaletteEntry {
  id: string;
  title: string;
  group: string;
  // A key combination shown beside it.
  shortcut?: string;
  // Why it can't run now; it shows, greyed, and Enter skips it.
  disabled?: string;
  // On now, such as an open panel.
  pressed?: boolean;
}

const props = defineProps<{ entries: PaletteEntry[] }>();
const emit = defineEmits<{ run: [id: string], close: [] }>();

const query = ref('');
const active = ref(0);
const input = ref<HTMLInputElement>();
const list = ref<HTMLElement>();

// Every word typed is somewhere in the entry's title or group, in any order.
const shown = computed(() => {
  const words = query.value.toLowerCase().split(/\s+/).filter(Boolean);
  return props.entries.filter((entry) => {
    const text = `${entry.title} ${entry.group}`.toLowerCase();
    return words.every((word) => text.includes(word));
  });
});
const groups = computed(() => {
  const byGroup = new Map<string, { entry: PaletteEntry, index: number }[]>();
  shown.value.forEach((entry, index) => {
    if (!byGroup.has(entry.group)) byGroup.set(entry.group, []);
    byGroup.get(entry.group)!.push({ entry, index });
  });
  return [...byGroup];
});

watch(query, () => { active.value = Math.max(0, shown.value.findIndex((entry) => !entry.disabled)); });
void nextTick(() => input.value?.focus());

function move (step: number): void {
  const count = shown.value.length;
  if (!count) return;
  let next = active.value;
  for (let i = 0; i < count; i++) {
    next = (next + step + count) % count;
    if (!shown.value[next].disabled) break;
  }
  active.value = next;
  void nextTick(() => list.value?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }));
}

function run (entry: PaletteEntry | undefined): void {
  if (!entry || entry.disabled) return;
  emit('close');
  emit('run', entry.id);
}

function onKeydown (event: KeyboardEvent): void {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault();
    move(event.key === 'ArrowDown' ? 1 : -1);
  } else if (event.key === 'Enter') {
    event.preventDefault();
    run(shown.value[active.value]);
  } else if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    emit('close');
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-start justify-center bg-black/25 px-4 pt-[12vh] pb-4" @click.self="emit('close')">
    <div class="flex max-h-[min(440px,76vh)] w-[min(560px,100%)] flex-col overflow-hidden rounded-xl border border-default bg-background shadow-e2" role="dialog" aria-label="Command palette" data-testid="command-palette">
      <label class="flex shrink-0 items-center gap-2 border-b border-default px-3.5 text-placeholder">
        <SearchIcon class="size-4 shrink-0" />
        <input
          ref="input"
          v-model="query"
          type="text"
          class="h-[46px] flex-1 bg-transparent px-0.5 py-px text-[15px] text-default outline-none!"
          placeholder="Type a command"
          aria-label="Type a command"
          role="combobox"
          aria-controls="palette-list"
          aria-expanded="true"
          data-testid="palette-input"
          @keydown="onKeydown"
        >
      </label>
      <div id="palette-list" ref="list" class="min-h-0 flex-1 overflow-auto p-1" role="listbox">
        <p v-if="!shown.length" class="mx-2 my-4 text-placeholder">No commands match.</p>
        <section v-for="[group, items] in groups" :key="group" role="group" :aria-label="group">
          <h3 class="mx-2 mt-2 mb-1 text-[11px] font-semibold tracking-[0.04em] text-placeholder uppercase">{{ group }}</h3>
          <div
            v-for="{ entry, index } in items"
            :key="entry.id"
            class="flex cursor-pointer items-center gap-2 rounded-[calc(var(--radius-xl)-4px)] px-2.5 py-[7px] aria-disabled:cursor-default aria-disabled:text-placeholder aria-selected:bg-interactive-highlight-default"
            role="option"
            :aria-selected="index === active"
            :aria-disabled="!!entry.disabled"
            :title="entry.disabled"
            :data-testid="`palette-${entry.id}`"
            @pointermove="!entry.disabled && (active = index)"
            @click="run(entry)"
          >
            <span class="min-w-0 flex-1 truncate">{{ entry.title }}<span v-if="entry.pressed" class="ml-2 rounded-sm bg-interactive-highlight-default px-1.5 text-[11px] font-semibold text-highlight-default">on</span></span>
            <span v-if="entry.disabled" class="shrink-0 text-[12px]">{{ entry.disabled }}</span>
            <kbd v-else-if="entry.shortcut" class="shrink-0 rounded-sm border border-default px-1.5 py-px font-mono text-[11px] leading-[normal] text-placeholder">{{ entry.shortcut }}</kbd>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
