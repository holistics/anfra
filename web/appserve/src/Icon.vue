<script setup lang="ts">
// anfra's lucide icons, from their path data in @holistics/anfra-design, so the Shell needs no icon library.
import { ICONS, type IconName } from '@holistics/anfra-design/icons';

defineProps<{ name: IconName }>();
</script>

<template>
  <svg
    class="icon"
    viewBox="0 0 24 24"
    width="16"
    height="16"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="ICONS[name]" />
  </svg>
</template>
