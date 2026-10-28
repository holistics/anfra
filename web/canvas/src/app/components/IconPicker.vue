<script setup lang="ts">
// A prop that names one of the kit's icons: a trigger with the icon and its name, and a floating
// panel of every icon drawn small with a search, as the kit's own icon component draws it. The panel
// is put on the canvas's root, as a select's list is, so a scrolling panel can't clip it. Escape or a
// press anywhere else closes it.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useStore } from '../context.ts';
import Field from '../ui/Field.vue';
import Icon from '../ui/Icon.vue';
import { vTip } from '../ui/tooltip.ts';

defineProps<{ modelValue: string, label?: string }>();
const emit = defineEmits<{ 'update:modelValue': [string] }>();

const WIDTH = 288;
const store = useStore();
const trigger = ref<HTMLButtonElement>();
const panel = ref<HTMLElement>();
const open = ref(false);
const query = ref('');
const place = ref<{ left: number, top?: number, bottom?: number, maxHeight: number, up: boolean }>();
const names = computed(() => store.state.settings.icons?.names ?? []);
const shown = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? names.value.filter((n) => n.toLowerCase().includes(q)) : names.value;
});

function measure () {
  const r = trigger.value!.getBoundingClientRect();
  const below = window.innerHeight - r.bottom - 8;
  const above = r.top - 8;
  const up = below < 320 && above > below;
  place.value = {
    // Its right edge on the trigger's, so it opens into the board rather than off the screen
    left: Math.max(8, Math.min(r.right - WIDTH, window.innerWidth - WIDTH - 8)),
    up,
    maxHeight: Math.min(400, (up ? above : below) - 4),
    ...(up ? { bottom: window.innerHeight - r.top + 4 } : { top: r.bottom + 4 }),
  };
}

async function show () {
  measure();
  open.value = true;
  await nextTick();
  panel.value?.querySelector<HTMLInputElement>('input')?.focus({ preventScroll: true });
  panel.value?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' });
}

function hide (refocus = true) {
  if (!open.value) return;
  open.value = false;
  query.value = '';
  if (refocus) trigger.value?.focus({ preventScroll: true });
}

function choose (name: string) {
  emit('update:modelValue', name);
  hide();
}

function onKey (e: KeyboardEvent) {
  e.stopPropagation(); // the canvas's own shortcuts are letters too
  if (e.key === 'Escape') {
    e.preventDefault();
    hide();
  } else if (e.key === 'Enter' && e.target instanceof HTMLInputElement && shown.value.length) {
    e.preventDefault();
    choose(shown.value[0]);
  }
}

function onOutside (e: PointerEvent) {
  const t = e.target as Node;
  if (!panel.value?.contains(t) && !trigger.value?.contains(t)) hide(false);
}
const onResize = () => hide(false);
watch(open, (is) => {
  if (is) {
    window.addEventListener('pointerdown', onOutside, true);
    window.addEventListener('resize', onResize);
  } else {
    window.removeEventListener('pointerdown', onOutside, true);
    window.removeEventListener('resize', onResize);
  }
});
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onOutside, true);
  window.removeEventListener('resize', onResize);
});
</script>

<template>
  <button
    ref="trigger"
    type="button"
    class="ac-select"
    aria-haspopup="dialog"
    :aria-expanded="open"
    :aria-label="label"
    @click="open ? hide() : show()"
  >
    <span v-if="modelValue && store.previews" class="ac-icon-cell" aria-hidden="true" inert>
      <component :is="store.previews" :id="`icon:${modelValue}`" />
    </span>
    <span class="ac-truncate" :class="{ placeholder: !modelValue }">{{ modelValue || 'None' }}</span>
    <Icon name="chevron" :size="16" class="ac-select-chevron" />
  </button>
  <Teleport to=".ac">
    <div
      v-if="open && place"
      ref="panel"
      class="ac-icon-panel ac-popover form ac-pop-in"
      :class="{ up: place.up }"
      role="dialog"
      :aria-label="label ? `Choose ${label}` : 'Choose an icon'"
      :style="{
        left: `${place.left}px`,
        top: place.top !== undefined ? `${place.top}px` : undefined,
        bottom: place.bottom !== undefined ? `${place.bottom}px` : undefined,
        width: `${WIDTH}px`,
        maxHeight: `${place.maxHeight}px`,
      }"
      data-ui
      @keydown="onKey"
    >
      <Field v-model="query" icon="search" type="search" placeholder="Search icons" aria-label="Search icons" />
      <div class="ac-icon-grid" role="listbox" :aria-label="label">
        <button v-if="modelValue" v-tip="'No icon'" type="button" role="option" class="ac-icon-choice" :aria-selected="false" @click="choose('')">
          <Icon name="x" :size="16" />
        </button>
        <button
          v-for="n in shown"
          :key="n"
          v-tip="n"
          type="button"
          role="option"
          class="ac-icon-choice"
          :aria-selected="n === modelValue"
          @click="choose(n)"
        >
          <span class="ac-icon-cell" inert><component :is="store.previews" :id="`icon:${n}`" /></span>
        </button>
      </div>
      <p v-if="!shown.length" class="ac-caption">No icon matches</p>
      <p v-else class="ac-caption">{{ shown.length }} {{ shown.length === 1 ? 'icon' : 'icons' }}</p>
    </div>
  </Teleport>
</template>
