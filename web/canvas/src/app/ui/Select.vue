<script setup lang="ts">
// A select for the canvas's own choices (the board, a frame's scenario, a token): a trigger framed
// like a field, and a floating list of items that can say what each choice means. The list is put
// on the canvas's root, so a scrolling panel can't clip it. Keyboard: arrows, Home and End, Enter or
// Space to choose, Escape to close, a letter to jump.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import Icon from './Icon.vue';
import MenuItem from './MenuItem.vue';

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
  /** Beside the label, quieter: a count, a key. */
  hint?: string;
  /** A color shown before the label: a CSS color. */
  swatch?: string;
}

const props = withDefaults(defineProps<{
  modelValue: string;
  options: SelectOption[];
  label?: string;
  placeholder?: string;
  /** `field` sits in a form; `ghost` sits in a bar, without a frame until hovered. */
  look?: 'field' | 'ghost';
  /** The list's width when the trigger is narrower: 200px unless said. */
  minWidth?: number;
}>(), { look: 'field', minWidth: 200, label: undefined, placeholder: 'Choose' });

const emit = defineEmits<{ 'update:modelValue': [string] }>();

const id = `ac-select-${Math.random().toString(36).slice(2, 8)}`;
const trigger = ref<HTMLButtonElement>();
const list = ref<HTMLElement>();
const open = ref(false);
const active = ref(-1);
const place = ref<{ left: number, top?: number, bottom?: number, width: number, maxHeight: number, up: boolean }>();

const current = computed(() => props.options.find((o) => o.value === props.modelValue));

function measure () {
  const r = trigger.value!.getBoundingClientRect();
  const below = window.innerHeight - r.bottom - 8;
  const above = r.top - 8;
  const up = below < 240 && above > below;
  const width = Math.max(r.width, props.minWidth);
  place.value = {
    left: Math.min(r.left, window.innerWidth - width - 8),
    width,
    up,
    maxHeight: Math.min(320, (up ? above : below) - 4),
    ...(up ? { bottom: window.innerHeight - r.top + 4 } : { top: r.bottom + 4 }),
  };
}

async function show () {
  if (open.value) return;
  measure();
  active.value = Math.max(0, props.options.findIndex((o) => o.value === props.modelValue));
  open.value = true;
  await nextTick();
  list.value?.focus({ preventScroll: true });
  scrollToActive();
}

function hide (refocus = true) {
  if (!open.value) return;
  open.value = false;
  if (refocus) trigger.value?.focus({ preventScroll: true });
}

function choose (i: number) {
  const o = props.options[i];
  if (o) emit('update:modelValue', o.value);
  hide();
}

function scrollToActive () {
  list.value?.querySelector<HTMLElement>(`[data-index="${active.value}"]`)?.scrollIntoView({ block: 'nearest' });
}

function move (to: number) {
  const n = props.options.length;
  if (!n) return;
  active.value = (to + n) % n;
  scrollToActive();
}

function onTriggerKey (e: KeyboardEvent) {
  if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
    e.preventDefault();
    void show();
  }
}

function onListKey (e: KeyboardEvent) {
  e.stopPropagation(); // the canvas's own shortcuts are letters too
  const keys: Record<string, () => void> = {
    ArrowDown: () => move(active.value + 1),
    ArrowUp: () => move(active.value - 1),
    Home: () => move(0),
    End: () => move(props.options.length - 1),
    Enter: () => choose(active.value),
    ' ': () => choose(active.value),
    Escape: () => hide(),
    Tab: () => hide(false),
  };
  const run = keys[e.key];
  if (run) {
    if (e.key !== 'Tab') e.preventDefault();
    run();
    return;
  }
  if (e.key.length === 1 && !e.metaKey && !e.ctrlKey) {
    const k = e.key.toLowerCase();
    const n = props.options.length;
    for (let step = 1; step <= n; step++) {
      const i = (active.value + step) % n;
      if (props.options[i].label.toLowerCase().startsWith(k)) return move(i);
    }
  }
}

// Close on a press anywhere else, and when the page around it moves.
function onOutside (e: PointerEvent) {
  const t = e.target as Node;
  if (!list.value?.contains(t) && !trigger.value?.contains(t)) hide(false);
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
    :class="look"
    aria-haspopup="listbox"
    :aria-expanded="open"
    :aria-controls="open ? id : undefined"
    :aria-label="label"
    @click="open ? hide() : show()"
    @keydown="onTriggerKey"
  >
    <span v-if="current?.swatch" class="ac-swatch" :style="{ background: current.swatch }" />
    <span class="ac-truncate" :class="{ placeholder: !current }">{{ current?.label ?? placeholder }}</span>
    <span v-if="current?.hint" class="ac-item-hint">{{ current.hint }}</span>
    <Icon name="chevron" :size="16" class="ac-select-chevron" />
  </button>
  <Teleport to=".ac">
    <div
      v-if="open && place"
      :id="id"
      ref="list"
      class="ac-listbox ac-popover ac-pop-in"
      :class="{ up: place.up }"
      role="listbox"
      tabindex="-1"
      :aria-label="label"
      :aria-activedescendant="active >= 0 ? `${id}-${active}` : undefined"
      :style="{
        left: `${place.left}px`,
        top: place.top !== undefined ? `${place.top}px` : undefined,
        bottom: place.bottom !== undefined ? `${place.bottom}px` : undefined,
        width: `${place.width}px`,
        maxHeight: `${place.maxHeight}px`,
      }"
      data-ui
      @keydown="onListKey"
    >
      <MenuItem
        v-for="(o, i) in options"
        :id="`${id}-${i}`"
        :key="o.value"
        :label="o.label"
        :swatch="o.swatch"
        :hint="o.hint"
        :description="o.description"
        :active="i === active"
        :checked="o.value === modelValue"
        role="option"
        :aria-selected="o.value === modelValue"
        :data-index="i"
        @pointermove="active = i"
        @click="choose(i)"
      />
      <p v-if="!options.length" class="ac-item-empty ac-caption">Nothing to choose</p>
    </div>
  </Teleport>
</template>
