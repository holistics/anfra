<script setup lang="ts">
import { LayoutDashboard } from '@lucide/vue';
</script>

<template>
  <div data-slot="empty-state" class="flex flex-1 flex-col items-center justify-center gap-3 text-placeholder">
    <LayoutDashboard class="size-10 rounded-lg bg-sidebar p-2.5" />
    <p><slot /></p>
  </div>
</template>
