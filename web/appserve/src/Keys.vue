<script setup lang="ts">
// A key combination as shadcn's KbdGroup: one Kbd per key.
import { computed } from 'vue';
import { keyLabels } from './shortcut';

const props = defineProps<{ combo: string }>();
const keys = computed(() => keyLabels(props.combo));
</script>

<template>
  <span class="a-kbd-group" aria-hidden="true">
    <kbd v-for="(key, i) in keys" :key="i" class="a-kbd">{{ key }}</kbd>
  </span>
</template>
