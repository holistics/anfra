<script setup lang="ts">
// A select in the app's look, for the canvas's own choices (the board, a frame's scenario): a
// 32px trigger like a field, and a floating list that can say what each choice means. The list is
// put on the canvas's root, so a scrolling panel can't clip it. Keyboard: arrows, Home and End,
// Enter or Space to choose, Escape to close, a letter to jump.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import Icon from './Icon.vue';

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
  /** Beside the label, quieter: a count, a key. */
  hint?: string;
}

const props = withDefaults(defineProps<{
  modelValue: string;
  options: SelectOption[];
  label?: string;
  placeholder?: string;
  /** `field` sits in a form; `ghost` sits in a bar, without a frame until hovered. */
  look?: 'field' | 'ghost';
  /** The list's width when the trigger is narrower. */
  minWidth?: number;
}>(), { look: 'field', minWidth: 0, label: undefined, placeholder: 'Choose' });

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
    <span class="ac-truncate" :class="{ placeholder: !current }">{{ current?.label ?? placeholder }}</span>
    <span v-if="current?.hint" class="ac-select-hint">{{ current.hint }}</span>
    <Icon name="chevron" :size="16" class="ac-select-chevron" />
  </button>
  <Teleport to=".ac">
    <div
      v-if="open && place"
      :id="id"
      ref="list"
      class="ac-select-list ac-popover ac-pop-in"
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
      <div
        v-for="(o, i) in options"
        :id="`${id}-${i}`"
        :key="o.value"
        class="ac-option"
        :class="{ active: i === active }"
        role="option"
        :aria-selected="o.value === modelValue"
        :data-index="i"
        @pointermove="active = i"
        @click="choose(i)"
      >
        <span class="ac-option-text">
          <span class="ac-option-label">
            <span class="ac-truncate">{{ o.label }}</span>
            <span v-if="o.hint" class="ac-select-hint">{{ o.hint }}</span>
          </span>
          <span v-if="o.description" class="ac-caption">{{ o.description }}</span>
        </span>
        <Icon v-if="o.value === modelValue" name="check" :size="16" class="ac-option-check" />
      </div>
      <p v-if="!options.length" class="ac-option-empty ac-caption">Nothing to choose</p>
    </div>
  </Teleport>
</template>

<style>
.ac-select {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  height: var(--ac-control);
  padding: 0 8px 0 10px;
  border: 1px solid var(--ac-line-field);
  border-radius: 10px;
  background: var(--ac-surface);
  color: var(--ac-ink);
  text-align: left;
  cursor: pointer;
  transition: border-color 160ms var(--ac-ease), background-color 160ms var(--ac-ease);
}
.ac-select > .ac-truncate { flex: 1; }
.ac-select .placeholder { color: var(--ac-ink-muted); }
.ac-select-chevron { flex-shrink: 0; color: var(--ac-icon-weak); transition: rotate 160ms var(--ac-ease); }
.ac-select[aria-expanded='true'] .ac-select-chevron { rotate: 180deg; }
.ac-select.ghost { width: auto; max-width: 320px; border-color: transparent; background: transparent; font-weight: 600; }
.ac-select.ghost[aria-expanded='true'] { background: var(--ac-active); }
@media (hover: hover) and (pointer: fine) {
  .ac-select.field:hover { border-color: var(--ac-line-hover); }
  .ac-select.ghost:hover { background: var(--ac-hover); }
}
.ac-select-hint { flex-shrink: 0; color: var(--ac-ink-muted); font-size: 12px; line-height: 16px; font-variant-numeric: tabular-nums; font-weight: 400; }

.ac-select-list {
  position: fixed;
  z-index: 30;
  display: grid;
  align-content: start;
  gap: 1px;
  padding: 4px;
  overflow: auto;
  overscroll-behavior: contain;
  outline: none;
  transform-origin: top;
}
.ac-select-list.up { transform-origin: bottom; }
.ac-option {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-height: var(--ac-control);
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
}
.ac-option.active { background: var(--ac-hover); }
.ac-option[aria-selected='true'] { background: var(--ac-selected); }
.ac-option-text { flex: 1; display: grid; gap: 2px; min-width: 0; }
.ac-option-label { display: flex; align-items: center; gap: 6px; min-width: 0; }
.ac-option-check { flex-shrink: 0; margin-top: 2px; color: var(--ac-violet-icon); }
.ac-option-empty { padding: 8px; }
</style>
