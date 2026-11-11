<script setup lang="ts">
// The sidebar: its content (SidebarPanel, the slot) beside the viewer, at the width the reader
// dragged it to. Collapsed, it slides to nothing; on a narrow window it floats over the viewer.
defineProps<{ open: boolean, narrow: boolean }>();
</script>

<template>
  <nav
    class="flex min-w-0 flex-col overflow-hidden p-2 text-sidebar-foreground duration-180 ease-[ease]"
    :class="[
      narrow ? 'fixed inset-y-0 left-0 z-20 w-64 bg-sidebar shadow-e2 transition-[translate,visibility]' : 'w-(--sidebar-width) transition-[visibility]',
      !open && (narrow ? 'invisible -translate-x-full' : 'invisible'),
    ]"
    aria-label="Data Apps"
    :inert="!open"
  >
    <slot />
  </nav>
</template>
