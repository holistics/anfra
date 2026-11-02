<template>
  <!-- Along the bottom edge of its positioned parent. -->
  <div
    data-slot="loading-bar"
    class="absolute inset-x-0 -bottom-px h-0.5 overflow-hidden before:absolute before:inset-y-0 before:left-0 before:w-2/5 before:animate-[loading-bar-sweep_1.1s_ease-in-out_infinite] before:bg-primary before:content-['']"
    role="progressbar"
  />
</template>

<style>
@keyframes loading-bar-sweep {
  from { transform: translateX(-100%); }
  to { transform: translateX(250%); }
}
</style>
