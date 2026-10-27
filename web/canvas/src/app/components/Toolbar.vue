<script setup lang="ts">
import { bounds, shapes } from '../../model/index.ts';
import { useStore } from '../context.ts';
import { TOOLS } from '../store.ts';
import Button from '../ui/Button.vue';

const emit = defineEmits<{ zoom: [factor: number] }>();
const store = useStore();
const { state, board } = store;

const fit = () => board.value && store.zoomTo(bounds(shapes(board.value)));
</script>

<template>
  <div class="ac-toolbar ac-bar ac-popover bar" data-ui role="toolbar" aria-label="Tools">
    <template v-for="(t, i) in TOOLS" :key="t.tool">
      <span v-if="i === 4" class="ac-rule-v" aria-hidden="true" />
      <Button
        variant="ghost"
        :icon="t.tool"
        :label="t.label"
        :keys="t.key.toUpperCase()"
        :aria-pressed="state.tool === t.tool"
        @click="state.tool = t.tool"
      />
    </template>
    <span class="ac-rule-v" aria-hidden="true" />
    <Button variant="ghost" icon="tidy" label="Tidy up" tip="Tidy up: lay screens out from the flow" keys="⌃⌥T" @click="store.tidyUp()" />
  </div>
  <div class="ac-zoom ac-bar ac-popover bar" data-ui role="group" aria-label="Zoom">
    <Button variant="ghost" icon="minus" label="Zoom out" keys="⌘−" @click="emit('zoom', 1 / 1.25)" />
    <span class="ac-zoom-value">{{ Math.round(state.view.zoom * 100) }}%</span>
    <Button variant="ghost" icon="plus" label="Zoom in" keys="⌘+" @click="emit('zoom', 1.25)" />
    <span class="ac-rule-v" aria-hidden="true" />
    <Button variant="ghost" icon="fit" label="Fit everything" keys="⇧1" @click="fit" />
  </div>
</template>
