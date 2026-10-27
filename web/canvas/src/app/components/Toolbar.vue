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
    <template v-for="(t, i) in TOOLS" :key="t.tool">
      <span v-if="i === 4" class="ac-toolbar-rule" aria-hidden="true" />
      <button
        class="ac-btn ghost icon"
        type="button"
        :aria-pressed="state.tool === t.tool"
        :title="`${t.label} (${t.key.toUpperCase()})`"
        :aria-label="t.label"
        @click="state.tool = t.tool"
      >
        <Icon :name="t.tool" />
      </button>
    </template>
    <span class="ac-toolbar-rule" aria-hidden="true" />
    <button class="ac-btn ghost icon" type="button" title="Tidy up: lay screens out from the flow (⌃⌥T)" aria-label="Tidy up" @click="store.tidyUp()">
      <Icon name="tidy" />
    </button>
  </div>
  <div class="ac-zoom ac-popover" data-ui role="group" aria-label="Zoom">
    <button class="ac-btn ghost icon" type="button" title="Zoom out (⌘−)" aria-label="Zoom out" @click="emit('zoom', 1 / 1.25)"><Icon name="minus" /></button>
    <span class="ac-zoom-value">{{ Math.round(state.view.zoom * 100) }}%</span>
    <button class="ac-btn ghost icon" type="button" title="Zoom in (⌘+)" aria-label="Zoom in" @click="emit('zoom', 1.25)"><Icon name="plus" /></button>
    <span class="ac-toolbar-rule" aria-hidden="true" />
    <button class="ac-btn ghost icon" type="button" title="Fit everything (⇧1)" aria-label="Fit everything" @click="fit"><Icon name="fit" /></button>
  </div>
</template>

<style>
.ac-toolbar, .ac-zoom {
  position: absolute;
  bottom: 16px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px;
}
.ac-toolbar { left: 50%; translate: -50% 0; }
.ac-zoom { right: 16px; }
.ac-toolbar-rule { width: 1px; height: 20px; margin: 0 4px; background: var(--ac-line); }
.ac-zoom-value { min-width: 44px; text-align: center; color: var(--ac-ink-weak); font-size: 12px; line-height: 16px; font-variant-numeric: tabular-nums; }
</style>
