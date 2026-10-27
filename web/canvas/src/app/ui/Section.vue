<script setup lang="ts">
// A section of a panel: a heading with an icon and actions at its end, then its content, 16px in
// and a hairline from the one before. `collapsible` makes the heading open and close it.
import Icon, { type IconName } from './Icon.vue';

defineProps<{ title?: string, icon?: IconName, collapsible?: boolean, open?: boolean }>();
</script>

<template>
  <details v-if="collapsible" class="ac-section" :open="open">
    <summary class="ac-section-head ac-heading"><Icon name="chevronRight" :size="16" /> {{ title }}</summary>
    <slot />
  </details>
  <section v-else class="ac-section">
    <div v-if="title || icon || $slots.actions" class="ac-section-head">
      <Icon v-if="icon" :name="icon" />
      <span class="ac-heading ac-truncate"><slot name="title">{{ title }}</slot></span>
      <template v-if="$slots.actions">
        <span class="ac-spacer" />
        <div class="ac-row tight"><slot name="actions" /></div>
      </template>
    </div>
    <slot />
  </section>
</template>
