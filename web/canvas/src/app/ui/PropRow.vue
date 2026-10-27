<script setup lang="ts">
// One property: its name in a 72px column, its control beside it. Rows go in a `.ac-props` group.
// `top` keeps the name at the top, beside a control taller than one line. Its tooltip says `tip`, or
// the whole name where the column cuts it.
import { vTip } from './tooltip.ts';

defineProps<{ label: string, tip?: string, top?: boolean }>();
</script>

<template>
  <div class="ac-prop" :class="{ top }">
    <span v-tip="tip ?? (label.length > 11 ? label : undefined)" class="ac-prop-label">{{ label }}</span>
    <slot />
  </div>
</template>
