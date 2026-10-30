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
  inspect: 'M3 7V5a2 2 0 0 1 2-2h2 M17 3h2a2 2 0 0 1 2 2v2 M21 17v2a2 2 0 0 1-2 2h-2 M7 21H5a2 2 0 0 1-2-2v-2 M9 12a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M16 16l-1.9-1.9',
  brand: 'M4 18l5-12 3 7 2-4 6 9',
  update: 'M12 19V5 M6 11l6-6 6 6',
  copy: 'M9 9h11v11H9z M5 15H4V4h11v1',
  check: 'M5 12l5 5 9-10',
  table: 'M4 5h16v14H4z M4 10h16 M4 15h16 M10 10v9',
  download: 'M12 4v11 M7 10l5 5 5-5 M5 20h14',
  more: 'M6 12h.01 M12 12h.01 M18 12h.01',
  overflow: 'M6 7l5 5-5 5 M13 7l5 5-5 5',
  command: 'M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z',
  // The panel's tabs, by the icon each names (`table` is the Data tab's sheet).
  overview: 'M4 3h5a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-5a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z M15 3h5a1 1 0 0 1 1 1v3a1 1 0 0 1-1 1h-5a1 1 0 0 1-1-1v-3a1 1 0 0 1 1-1z M15 12h5a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-5a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z M4 16h5a1 1 0 0 1 1 1v3a1 1 0 0 1-1 1h-5a1 1 0 0 1-1-1v-3a1 1 0 0 1 1-1z',
  sheet: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-14a2 2 0 0 1-2-2v-14a2 2 0 0 1 2-2z M3 9h18 M3 15h18 M9 9v12 M15 9v12',
  database: 'M3 5a9 3 0 1 0 18 0a9 3 0 1 0-18 0 M3 5v14a9 3 0 0 0 18 0V5 M3 12a9 3 0 0 0 18 0',
  lineage: 'M9 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M3 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M15 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0 M18 9v2c0 .6-.4 1-1 1H7c-.6 0-1-.4-1-1V9 M12 12v3',
  tree: 'M21 12h-8 M21 6H8 M21 18h-8 M3 6v4c0 1.1.9 2 2 2h3 M3 10v6c0 1.1.9 2 2 2h3',
  code: 'M16 18l6-6-6-6 M8 6l-6 6 6 6',
  // The panel's commands.
  pick: 'M4 4l7 16 2-7 7-2z',
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
