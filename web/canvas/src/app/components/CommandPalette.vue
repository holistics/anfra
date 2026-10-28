<script setup lang="ts">
// The command palette (⌘K): every command on the canvas, the screens of this board and the other
// boards to go to, the app's components to insert, and its routes to add as screens. Type to find
// one; ↑ ↓ to move, ↵ to run, Esc to close. The tabs above the list show one kind alone, all of it
// (Tab and Shift+Tab move between them).
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import {
  addCommands, boardCommands, componentCommands, elementCommands, goCommands, routeCommands, score, selectionCommands, viewCommands,
  type Command, type CommandGroup,
} from '../commands.ts';
import { useStore } from '../context.ts';
import Field from '../ui/Field.vue';
import MenuItem from '../ui/MenuItem.vue';
import Segmented from '../ui/Segmented.vue';

const store = useStore();
const { state } = store;

const query = ref('');
const active = ref(0);
const input = ref<InstanceType<typeof Field>>();
const list = ref<HTMLElement>();
onMounted(() => input.value?.focus());

const picked = computed(() => state.picked && (state.picked.info.ref.component ?? state.picked.info.ref.tag));

type Kind = 'all' | 'commands' | 'screens' | 'boards' | 'components' | 'routes';
const KINDS: { value: Kind, label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'commands', label: 'Commands' },
  { value: 'screens', label: 'Screens' },
  { value: 'boards', label: 'Boards' },
  { value: 'components', label: 'Components' },
  { value: 'routes', label: 'Routes' },
];
const kind = ref<Kind>('all');

/** Every group, each of a kind, with the commands the query finds in it, best first. */
const found = computed<(CommandGroup & { kind: Kind })[]>(() => {
  const q = query.value;
  const [screens, boards] = goCommands(store);
  const all: (CommandGroup & { kind: Kind })[] = state.palette === 'components'
    ? [{ kind: 'components', name: picked.value ? `Insert after ${picked.value}` : 'Place on a screen', items: componentCommands(store) }]
    : [
        { kind: 'commands', name: picked.value ?? '', items: elementCommands(store) },
        { kind: 'commands', name: 'Selection', items: state.picked ? [] : selectionCommands(store) },
        { kind: 'commands', name: 'Board', items: [...boardCommands(store), ...addCommands(store)] },
        { kind: 'screens', ...screens },
        { kind: 'boards', ...boards },
        { kind: 'commands', name: 'View', items: viewCommands(store) },
        { kind: 'components', name: 'Insert a component', items: componentCommands(store) },
        { kind: 'routes', name: 'Add a screen of a route', items: routeCommands(store) },
      ];
  return all.map((g) => ({
    ...g,
    items: g.items
      .map((c) => ({ c, s: score(c, q) }))
      .filter((x) => x.s > 0)
      .sort((x, y) => y.s - x.s)
      .map((x) => x.c),
  }));
});

const kinds = computed(() => KINDS.map((k) => ({
  ...k,
  count: k.value === 'all' ? undefined : found.value.filter((g) => g.kind === k.value).reduce((n, g) => n + g.items.length, 0),
})));

const groups = computed<CommandGroup[]>(() => {
  const typed = !!query.value.trim();
  return found.value
    .filter((g) => kind.value === 'all'
      // Everything at once: the long lists only once something is typed, and the first few of each.
      ? !(g.kind === 'components' && state.palette !== 'components' && !typed) && !(g.kind === 'routes' && !typed)
      : g.kind === kind.value)
    .map((g) => ({ name: g.name, items: kind.value === 'all' ? g.items.slice(0, typed ? 8 : 50) : g.items }))
    .filter((g) => g.items.length);
});
const flat = computed(() => groups.value.flatMap((g) => g.items));
const indexOf = (c: Command) => flat.value.indexOf(c);

watch(query, () => (active.value = 0));
// A tab picked with the pointer: typing goes on in the search.
watch(kind, () => {
  active.value = 0;
  input.value?.focus();
});
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
  if (e.key === 'Tab' && state.palette !== 'components') {
    const i = KINDS.findIndex((k) => k.value === kind.value);
    kind.value = KINDS[(i + (e.shiftKey ? -1 : 1) + KINDS.length) % KINDS.length].value;
  } else if (e.key === 'ArrowDown') active.value = n ? (active.value + 1) % n : 0;
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
      <div v-if="state.palette !== 'components'" class="ac-command-kinds">
        <Segmented v-model="kind" :options="kinds" label="Show" tabs />
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
