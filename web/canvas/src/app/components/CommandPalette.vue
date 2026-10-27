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
import Field from '../ui/Field.vue';
import MenuItem from '../ui/MenuItem.vue';

const store = useStore();
const { state } = store;

const query = ref('');
const active = ref(0);
const input = ref<InstanceType<typeof Field>>();
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
  list.value?.querySelector('.ac-item.active')?.scrollIntoView({ block: 'nearest' });
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
  <div class="ac-command-scrim" data-ui @pointerdown.self="close">
    <div class="ac-command ac-popover ac-pop-in" role="dialog" aria-label="Command palette" @keydown="onKey">
      <div class="ac-command-search">
        <Field
          ref="input"
          v-model="query"
          icon="search"
          bare
          :placeholder="state.palette === 'components' ? 'Find a component' : 'Find a command, screen, board or component'"
          aria-label="Search"
          role="combobox"
          aria-expanded="true"
          aria-controls="ac-command-list"
          :aria-activedescendant="flat[active] ? `ac-cmd-${active}` : undefined"
          autocomplete="off"
          spellcheck="false"
        />
        <span class="ac-kbd">Esc</span>
      </div>
      <div id="ac-command-list" ref="list" class="ac-command-list" role="listbox">
        <template v-for="g in groups" :key="g.name">
          <p v-if="g.name" class="ac-item-group">{{ g.name }}</p>
          <MenuItem
            v-for="c in g.items"
            :id="`ac-cmd-${indexOf(c)}`"
            :key="c.id"
            :label="c.label"
            :icon="c.icon"
            :also="c.id.startsWith('go.') ? c.also : undefined"
            :keys="c.keys"
            :active="indexOf(c) === active"
            :danger="c.danger"
            role="option"
            :aria-selected="indexOf(c) === active"
            @pointermove="active = indexOf(c)"
            @click="run(c)"
          />
        </template>
        <p v-if="!flat.length" class="ac-item-empty ac-caption">Nothing matches “{{ query }}”</p>
      </div>
    </div>
  </div>
</template>
