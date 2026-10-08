<script setup lang="ts">
// Hand-drawn 24px stroke icons, so the Shell needs no icon library.
const PATHS = {
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  app: 'M4 4h6v6H4z M14 4h6v6h-6z M4 14h6v6H4z M14 14h6v6h-6z',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14z M20 20l-3.5-3.5',
  chevron: 'M9 6l6 6-6 6',
  sidebar: 'M4 5h16v14H4z M9 5v14',
  reload: 'M20 12a8 8 0 1 1-2.3-5.7 M20 4v4h-4',
  alert: 'M12 4l9 16H3z M12 10v4 M12 17.5v.01',
  close: 'M6 6l12 12 M18 6L6 18',
  sun: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z M12 2v2 M12 20v2 M2 12h2 M20 12h2 M4.9 4.9l1.4 1.4 M17.7 17.7l1.4 1.4 M4.9 19.1l1.4-1.4 M17.7 6.3l1.4-1.4',
  moon: 'M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z',
  inspect: 'M8 8l-4 4 4 4 M16 8l4 4-4 4 M13.5 6l-3 12',
  brand: 'M4 18l5-12 3 7 2-4 6 9',
  update: 'M12 19V5 M6 11l6-6 6 6',
} as const;

defineProps<{ name: keyof typeof PATHS }>();
</script>

<template>
  <svg
    class="icon"
    viewBox="0 0 24 24"
    width="16"
    height="16"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="PATHS[name]" />
  </svg>
</template>
