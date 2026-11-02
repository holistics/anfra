<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue';
import { Kbd, KbdGroup } from '@/components/ui/kbd';
import { keyLabels } from '@/lib/shortcut';

const props = defineProps<{ combo: string, class?: HTMLAttributes['class'] }>();
const keys = computed(() => keyLabels(props.combo));
</script>

<template>
  <KbdGroup :class="props.class" aria-hidden="true">
    <Kbd v-for="(key, i) in keys" :key="i">{{ key }}</Kbd>
  </KbdGroup>
</template>
