<script setup lang="ts">
import { bounds, shapes } from '../../model/index.ts';
import { useStore } from '../context.ts';
import { TOOLS } from '../store.ts';
import Icon from './Icon.vue';

const emit = defineEmits<{ zoom: [factor: number] }>();
const store = useStore();
const { state, board } = store;

const fit = () => board.value && store.zoomTo(bounds(shapes(board.value)));
</script>

<template>
  <div class="ac-toolbar ac-popover" data-ui role="toolbar" aria-label="Tools">
    <button
      v-for="t in TOOLS"
      :key="t.tool"
      class="ac-btn ghost icon"
      type="button"
      :aria-pressed="state.tool === t.tool"
      :title="`${t.label} (${t.key.toUpperCase()})`"
      :aria-label="t.label"
      @click="state.tool = t.tool"
    >
      <Icon :name="t.tool" />
    </button>
  </div>
  <div class="ac-zoom ac-popover" data-ui>
    <button class="ac-btn ghost icon small" type="button" title="Zoom out (⌘−)" aria-label="Zoom out" @click="emit('zoom', 1 / 1.25)"><Icon name="minus" /></button>
    <span class="ac-zoom-value">{{ Math.round(state.view.zoom * 100) }}%</span>
    <button class="ac-btn ghost icon small" type="button" title="Zoom in (⌘+)" aria-label="Zoom in" @click="emit('zoom', 1.25)"><Icon name="plus" /></button>
    <button class="ac-btn ghost icon small" type="button" title="Fit everything (⇧1)" aria-label="Fit everything" @click="fit"><Icon name="fit" /></button>
  </div>
</template>

<style>
.ac-toolbar {
  position: absolute;
  left: 50%;
  bottom: 16px;
  transform: translateX(-50%);
  display: flex;
  gap: 2px;
  padding: 4px;
  border-radius: 12px;
}
.ac-zoom {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px;
  border-radius: 12px;
}
.ac-zoom-value { min-width: 44px; text-align: center; font-size: 12px; font-variant-numeric: tabular-nums; color: var(--ac-ink-weak); }
</style>
