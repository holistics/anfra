<script setup lang="ts">
// The right-click menu: the commands for what was clicked (an element of a screen, the selection,
// or the empty canvas), at the pointer. ↑ ↓ to move, ↵ to run, Esc or a click elsewhere to close.
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import type { Command, CommandGroup } from '../commands.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const props = defineProps<{ groups: CommandGroup[], at: { x: number, y: number } }>();
const emit = defineEmits<{ close: [] }>();
const { state } = useStore();

const el = ref<HTMLElement>();
const active = ref(-1);
const groups = computed(() => props.groups.filter((g) => g.items.length));
const flat = computed(() => groups.value.flatMap((g) => g.items));
const indexOf = (c: Command) => flat.value.indexOf(c);

// Kept inside the viewport: flipped to the pointer's other side where it would run out.
const place = ref({ left: `${props.at.x}px`, top: `${props.at.y}px` });
onMounted(async () => {
  await nextTick();
  const box = el.value!.getBoundingClientRect();
  const { width: W, height: H } = state.viewport;
  const left = props.at.x + box.width > W - 8 ? Math.max(8, props.at.x - box.width) : props.at.x;
  const top = props.at.y + box.height > H - 8 ? Math.max(8, H - 8 - box.height) : props.at.y;
  place.value = { left: `${left}px`, top: `${top}px` };
  el.value!.focus();
  window.addEventListener('pointerdown', outside, true);
  window.addEventListener('blur', close);
});
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', outside, true);
  window.removeEventListener('blur', close);
});

const close = () => emit('close');
const outside = (e: PointerEvent) => {
  if (!el.value?.contains(e.target as Node)) close();
};
function run (c: Command | undefined) {
  if (!c) return;
  close();
  void c.run();
}

function onKey (e: KeyboardEvent) {
  const n = flat.value.length;
  if (e.key === 'ArrowDown') active.value = (active.value + 1) % n;
  else if (e.key === 'ArrowUp') active.value = (active.value - 1 + n) % n;
  else if (e.key === 'Enter' || e.key === ' ') run(flat.value[active.value]);
  else if (e.key === 'Escape') close();
  else return;
  e.preventDefault();
  e.stopPropagation();
}
</script>

<template>
  <div
    ref="el"
    class="ac-menu ac-popover ac-pop-in"
    :style="place"
    role="menu"
    tabindex="-1"
    data-ui
    @keydown="onKey"
    @pointerdown.stop
    @contextmenu.prevent
  >
    <template v-for="(g, i) in groups" :key="i">
      <div v-if="i" class="ac-menu-rule" role="separator" />
      <div
        v-for="c in g.items"
        :key="c.id"
        class="ac-menu-item"
        :class="{ active: indexOf(c) === active, danger: c.danger }"
        role="menuitem"
        @pointermove="active = indexOf(c)"
        @pointerleave="active = -1"
        @click="run(c)"
      >
        <Icon v-if="c.icon" :name="c.icon" :size="16" />
        <span class="ac-menu-label">{{ c.label }}</span>
        <span v-if="c.keys" class="ac-menu-keys">{{ c.keys }}</span>
      </div>
    </template>
  </div>
</template>

<style>
.ac-menu { position: absolute; z-index: 20; display: grid; min-width: 220px; max-width: 320px; padding: 4px; transform-origin: top left; }
.ac .ac-menu:focus-visible { outline: none; }
</style>
