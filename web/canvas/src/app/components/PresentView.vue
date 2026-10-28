<script setup lang="ts">
// Presenting a prototype: one frame's screen, full screen and fully usable, as it runs on its own.
// Its links go where the app sends them, so a prototype that links its screens runs end to end.
// The bar above switches its scenario and follows the board's arrows to the next screen, as
// playing the board does; Escape, or the board's name, goes back to the board.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { Frame } from '../../model/index.ts';
import { useStore } from '../context.ts';
import { PRESENT_FRAME } from '../store.ts';
import Button from '../ui/Button.vue';
import Icon from '../ui/Icon.vue';
import Segmented from '../ui/Segmented.vue';
import Select, { type SelectOption } from '../ui/Select.vue';

const store = useStore();
const { state, board } = store;

const frameOf = (id: string): Frame | undefined => board.value?.frames.find((f) => f.id === id);
const frame = computed(() => frameOf(state.present?.frame ?? ''));
const scenario = computed(() => state.present?.scenario ?? frame.value?.scenario ?? 'default');
/** The frames presented so far, to go back along. */
const path = ref<string[]>([]);
watch(() => state.present?.frame, (id) => {
  if (!id) return;
  const i = path.value.lastIndexOf(id);
  path.value = i >= 0 ? path.value.slice(0, i + 1) : [...path.value, id];
}, { immediate: true });

const ways = computed(() => (board.value?.arrows ?? [])
  .filter((a) => a.from === frame.value?.id && frameOf(a.to))
  .map((a, i) => ({ id: a.id, to: a.to, n: i + 1, label: a.label || frameOf(a.to)!.name })));

const scenarios = computed<SelectOption[]>(() => Object.entries(state.settings.scenarios).map(([value, s]) => ({
  value, label: s.label ?? value, description: s.description,
})));

const go = (to: string) => {
  location.hash = `${state.slug}/present/${to}`;
};
const back = () => {
  const prev = path.value.at(-2);
  if (prev) go(prev);
};
const close = () => {
  location.hash = state.slug;
};
const setScenario = (s: string) => {
  if (state.present) state.present = { ...state.present, scenario: s };
};
// Where the screen is now, which its own links may have changed, to open in a tab of its own.
const route = computed(() => state.routes[PRESENT_FRAME] ?? frame.value?.route ?? '/');
const tabUrl = computed(() => {
  const url = new URL(route.value, location.origin);
  url.searchParams.set('scenario', scenario.value);
  return `${url.pathname}${url.search}${url.hash}`;
});

// Fit the screen to the window, or show it at its own size and scroll.
const fit = ref<'fit' | 'actual'>('fit');
const stage = ref<HTMLElement>();
const room = ref({ width: 0, height: 0 });
let observer: ResizeObserver | undefined;
onMounted(() => {
  observer = new ResizeObserver(([e]) => {
    room.value = { width: e.contentRect.width, height: e.contentRect.height };
  });
  if (stage.value) observer.observe(stage.value);
});
onBeforeUnmount(() => observer?.disconnect());
const scale = computed(() => {
  const f = frame.value;
  if (!f || fit.value === 'actual' || !room.value.width) return 1;
  return Math.min(1, (room.value.width - 48) / f.width, (room.value.height - 48) / f.height);
});

/** Keys while presenting, from the canvas or from the screen (only an Escape it didn't use). */
function key (k: string): boolean {
  if (k === 'Escape') close();
  else if (k === 'ArrowLeft' && path.value.length > 1) back();
  else if (/^[1-9]$/.test(k) && ways.value[Number(k) - 1]) go(ways.value[Number(k) - 1].to);
  else return false;
  return true;
}
defineExpose({ key });
</script>

<template>
  <div class="ac-present" data-ui role="dialog" :aria-label="frame ? `Presenting ${frame.name}` : 'Presenting'">
    <header class="ac-present-bar">
      <Button variant="ghost" icon="back" label="Back to the board" keys="Esc" @click="close" />
      <div class="ac-present-title">
        <span class="ac-caption ac-truncate">{{ board?.name }}</span>
        <span class="ac-heading ac-truncate">{{ frame?.name ?? 'A removed frame' }}</span>
      </div>
      <Select
        v-if="scenarios.length > 1"
        :model-value="scenario"
        :options="scenarios"
        label="Scenario"
        look="ghost"
        :min-width="220"
        @update:model-value="setScenario"
      />
      <span class="ac-spacer" />
      <Button variant="ghost" icon="back" label="Previous screen" keys="←" :disabled="path.length < 2" @click="back" />
      <template v-if="ways.length">
        <Button v-for="w in ways" :key="w.id" :tip="`To ${frameOf(w.to)?.name}`" :keys="String(w.n)" @click="go(w.to)">
          <span class="ac-truncate">{{ w.label }}</span>
          <Icon name="arrowRight" />
        </Button>
      </template>
      <span v-else class="ac-caption">No arrow leaves this screen</span>
      <Segmented
        v-model="fit"
        :options="[{ value: 'fit', label: 'Fit' }, { value: 'actual', label: '100%' }]"
        label="Size"
      />
      <a class="ac-btn ghost" :href="tabUrl" target="_blank" rel="noopener">Open in a tab <Icon name="external" :size="14" /></a>
    </header>
    <div ref="stage" class="ac-present-stage" :class="fit">
      <div
        v-if="frame"
        class="ac-present-screen"
        :style="{ width: `${frame.width * scale}px`, height: `${frame.height * scale}px` }"
      >
        <iframe
          :key="`${frame.id}|${scenario}`"
          :src="frame.route"
          :name="`anfra-canvas|${PRESENT_FRAME}|${scenario}`"
          :title="frame.name"
          :style="{ width: `${frame.width}px`, height: `${frame.height}px`, transform: `scale(${scale})` }"
        />
      </div>
      <p v-else class="ac-caption">This frame is no longer on the board</p>
    </div>
  </div>
</template>
