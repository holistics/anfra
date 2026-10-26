<script setup lang="ts">
// The command palette: every command the shell offers, its own and the plugins', found by typing
// and run with Enter. What each entry does, and whether it can run, the shell decides.
import { computed, nextTick, ref, watch } from 'vue';
import Icon from './Icon.vue';

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
  <div class="palette-backdrop" @click.self="emit('close')">
    <div class="palette" role="dialog" aria-label="Command palette" data-testid="command-palette">
      <label class="palette-search">
        <Icon name="search" />
        <input
          ref="input"
          v-model="query"
          type="text"
          placeholder="Type a command"
          aria-label="Type a command"
          role="combobox"
          aria-controls="palette-list"
          aria-expanded="true"
          data-testid="palette-input"
          @keydown="onKeydown"
        >
      </label>
      <div id="palette-list" ref="list" class="palette-list" role="listbox">
        <p v-if="!shown.length" class="palette-empty">No commands match.</p>
        <section v-for="[group, items] in groups" :key="group" role="group" :aria-label="group">
          <h3>{{ group }}</h3>
          <div
            v-for="{ entry, index } in items"
            :key="entry.id"
            class="palette-entry"
            role="option"
            :aria-selected="index === active"
            :aria-disabled="!!entry.disabled"
            :title="entry.disabled"
            :data-testid="`palette-${entry.id}`"
            @pointermove="!entry.disabled && (active = index)"
            @click="run(entry)"
          >
            <span class="palette-title">{{ entry.title }}<span v-if="entry.pressed" class="palette-on">on</span></span>
            <span v-if="entry.disabled" class="palette-hint">{{ entry.disabled }}</span>
            <kbd v-else-if="entry.shortcut">{{ entry.shortcut }}</kbd>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
