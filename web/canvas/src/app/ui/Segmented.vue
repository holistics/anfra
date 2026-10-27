<script setup lang="ts" generic="T extends string | number">
// One of a few, side by side: options (a layout, a size) or, with `tabs`, the tabs of a panel. An
// option can be an icon alone, named by its label; it can carry a count. `fill` stretches it to
// the width it has, each option as wide as the others.
import Icon, { type IconName } from './Icon.vue';
import { vTip } from './tooltip.ts';

export interface SegmentedOption<T = string | number> {
  value: T;
  label: string;
  icon?: IconName;
  /** Only the icon shows; the label names it. */
  iconOnly?: boolean;
  title?: string;
  count?: number;
}

defineProps<{
  modelValue: T;
  options: readonly SegmentedOption<T>[];
  label: string;
  fill?: boolean;
  tabs?: boolean;
}>();
const emit = defineEmits<{ 'update:modelValue': [T] }>();
</script>

<template>
  <div class="ac-seg" :class="{ fill }" :role="tabs ? 'tablist' : 'group'" :aria-label="label">
    <button
      v-for="o in options"
      :key="o.value"
      type="button"
      :role="tabs ? 'tab' : undefined"
      :aria-selected="tabs ? o.value === modelValue : undefined"
      :aria-pressed="tabs ? undefined : o.value === modelValue"
      :aria-label="o.iconOnly ? o.label : undefined"
      v-tip="o.title ?? (o.iconOnly ? o.label : undefined)"
      @click="emit('update:modelValue', o.value)"
    >
      <Icon v-if="o.icon" :name="o.icon" :size="14" />
      <template v-if="!o.iconOnly">{{ o.label }}</template>
      <span v-if="o.count" class="ac-count">{{ o.count }}</span>
    </button>
  </div>
</template>
