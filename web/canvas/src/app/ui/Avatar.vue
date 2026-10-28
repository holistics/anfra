<script setup lang="ts">
// A person: their picture, else their initials on a tint picked from their name.
import { computed, ref } from 'vue';
import { initials } from '../home.ts';

const props = defineProps<{ name: string, src?: string, size?: number }>();
const broken = ref(false);
const hue = computed(() => [...props.name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7));
</script>

<template>
  <span
    class="ac-avatar"
    :style="{ width: `${size ?? 20}px`, height: `${size ?? 20}px`, fontSize: `${Math.round((size ?? 20) * 0.42)}px`, '--ac-avatar-hue': hue }"
    :title="name"
  >
    <img v-if="src && !broken" :src="src" alt="" @error="broken = true">
    <template v-else>{{ initials(name) }}</template>
  </span>
</template>
