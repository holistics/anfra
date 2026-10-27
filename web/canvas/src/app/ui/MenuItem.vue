<script setup lang="ts">
// A row of a menu, a select's list or the command palette: an icon or a color, the label with a
// quiet hint beside it and what it means under it, then other words it is found by, its shortcut,
// and a check where it is the current choice. `active` is the row the keyboard or the pointer is on.
import Icon, { type IconName } from './Icon.vue';

defineProps<{
  label: string;
  icon?: IconName;
  swatch?: string;
  hint?: string;
  description?: string;
  also?: string;
  keys?: string;
  active?: boolean;
  checked?: boolean;
  danger?: boolean;
}>();
</script>

<template>
  <div class="ac-item" :class="{ active, checked, danger }">
    <Icon v-if="icon" :name="icon" :size="16" />
    <span class="ac-item-text">
      <span class="ac-item-label">
        <span v-if="swatch" class="ac-swatch" :style="{ background: swatch }" />
        <span class="ac-truncate">{{ label }}</span>
        <span v-if="hint" class="ac-item-hint">{{ hint }}</span>
      </span>
      <span v-if="description" class="ac-caption">{{ description }}</span>
    </span>
    <span v-if="also" class="ac-item-also ac-mono">{{ also }}</span>
    <span v-if="keys" class="ac-item-keys">{{ keys }}</span>
    <Icon v-if="checked" name="check" :size="16" class="ac-item-check" />
  </div>
</template>
