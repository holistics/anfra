<script setup lang="ts">
// The command palette (⌘K): every command on the canvas, the screens of this board and the other
// boards to go to, the app's components to insert, and its routes to add as screens. Type to find
// one; ↑ ↓ to move, ↵ to run, Esc to close.
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import {
  addCommands, boardCommands, componentCommands, elementCommands, goCommands, routeCommands, score, selectionCommands, viewCommands,
  type Command, type CommandGroup,
} from '../commands.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const store = useStore();
const { state } = store;

const query = ref('');
const active = ref(0);
const input = ref<HTMLInputElement>();
const list = ref<HTMLElement>();
onMounted(() => input.value?.focus());

const picked = computed(() => state.picked && (state.picked.info.ref.component ?? state.picked.info.ref.tag));

const groups = computed<CommandGroup[]>(() => {
  const q = query.value;
  const all: CommandGroup[] = state.palette === 'components'
    ? [{ name: picked.value ? `Insert after ${picked.value}` : 'Place on a screen', items: componentCommands(store) }]
    : [
        { name: picked.value ?? '', items: elementCommands(store) },
        { name: 'Selection', items: state.picked ? [] : selectionCommands(store) },
        { name: 'Board', items: [...boardCommands(store), ...addCommands(store)] },
        ...goCommands(store),
        { name: 'View', items: viewCommands(store) },
        // Long lists only once something is typed.
        { name: 'Insert a component', items: q.trim() ? componentCommands(store) : [] },
        { name: 'Add a screen of a route', items: q.trim() ? routeCommands(store) : [] },
      ];
  return all
    .map((g) => ({
      name: g.name,
      items: g.items
        .map((c) => ({ c, s: score(c, q) }))
        .filter((x) => x.s > 0)
        .sort((x, y) => y.s - x.s)
        .slice(0, q.trim() ? 8 : 50)
        .map((x) => x.c),
    }))
    .filter((g) => g.items.length);
});
const flat = computed(() => groups.value.flatMap((g) => g.items));
const indexOf = (c: Command) => flat.value.indexOf(c);

watch(query, () => (active.value = 0));
watch(active, async () => {
  await nextTick();
  list.value?.querySelector('.ac-menu-item.active')?.scrollIntoView({ block: 'nearest' });
});

const close = () => (state.palette = '');
function run (c: Command | undefined) {
  if (!c) return;
  close();
  void c.run();
}

function onKey (e: KeyboardEvent) {
  const n = flat.value.length;
  if (e.key === 'ArrowDown') active.value = n ? (active.value + 1) % n : 0;
  else if (e.key === 'ArrowUp') active.value = n ? (active.value - 1 + n) % n : 0;
  else if (e.key === 'Enter') run(flat.value[active.value]);
  else if (e.key === 'Escape') close();
  else return;
  e.preventDefault();
  e.stopPropagation();
}
</script>

<template>
  <div class="ac-palette-scrim" data-ui @pointerdown.self="close">
    <div class="ac-palette ac-popover ac-pop-in" role="dialog" aria-label="Command palette" @keydown="onKey">
      <label class="ac-palette-search">
        <Icon name="search" />
        <input
          ref="input"
          v-model="query"
          :placeholder="state.palette === 'components' ? 'Find a component' : 'Find a command, screen, board or component'"
          aria-label="Search"
          role="combobox"
          aria-expanded="true"
          aria-controls="ac-palette-list"
          :aria-activedescendant="flat[active] ? `ac-cmd-${active}` : undefined"
          autocomplete="off"
          spellcheck="false"
        />
        <span class="ac-kbd">Esc</span>
      </label>
      <div id="ac-palette-list" ref="list" class="ac-palette-list" role="listbox">
        <template v-for="g in groups" :key="g.name">
          <p v-if="g.name" class="ac-palette-group">{{ g.name }}</p>
          <div
            v-for="c in g.items"
            :id="`ac-cmd-${indexOf(c)}`"
            :key="c.id"
            class="ac-menu-item"
            :class="{ active: indexOf(c) === active, danger: c.danger }"
            role="option"
            :aria-selected="indexOf(c) === active"
            @pointermove="active = indexOf(c)"
            @click="run(c)"
          >
            <Icon v-if="c.icon" :name="c.icon" :size="16" />
            <span class="ac-menu-label">{{ c.label }}</span>
            <span v-if="c.id.startsWith('go.')" class="ac-menu-also ac-mono">{{ c.also }}</span>
            <span v-if="c.keys" class="ac-menu-keys">{{ c.keys }}</span>
          </div>
        </template>
        <p v-if="!flat.length" class="ac-palette-empty ac-caption">Nothing matches “{{ query }}”</p>
      </div>
    </div>
  </div>
</template>

<style>
.ac-palette-scrim { position: fixed; inset: 0; z-index: 40; display: grid; justify-items: center; align-items: start; padding: 96px 16px 16px; }
.ac-palette { display: grid; grid-template-rows: auto minmax(0, 1fr); width: min(560px, 100%); max-height: min(480px, calc(100vh - 128px)); overflow: hidden; }
.ac-palette-search { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--ac-line-weak); color: var(--ac-icon-weak); }
.ac-palette-search input { flex: 1; min-width: 0; height: var(--ac-control); border: 0; background: transparent; color: var(--ac-ink); font-size: 14px; outline: none; }
.ac-palette-search input::placeholder { color: var(--ac-ink-muted); }
.ac .ac-palette-search input:focus-visible { outline: none; }
.ac-palette-list { overflow: auto; overscroll-behavior: contain; padding: 4px; }
.ac-palette-group { padding: 8px 8px 4px; color: var(--ac-ink-muted); font-size: 12px; line-height: 16px; font-weight: 500; }
.ac-palette-empty { padding: 16px 8px; text-align: center; }
</style>
