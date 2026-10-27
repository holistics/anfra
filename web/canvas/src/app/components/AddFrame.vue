<script setup lang="ts">
// Putting a screen on the canvas: which route, under which scenario, at what size.
import { computed, onMounted, ref } from 'vue';
import { FRAME_SIZES, newId, type Frame, type Point } from '../../model/index.ts';
import { scenarioOptions, useStore } from '../context.ts';
import Select from './Select.vue';

const props = defineProps<{ at: Point, screen: Point }>();
const emit = defineEmits<{ add: [Frame], cancel: [] }>();
const store = useStore();
const { state } = store;

const route = ref(state.settings.prototypes[0] ?? state.settings.routes[0] ?? '/');
const scenario = ref('default');
const size = ref(1);
const input = ref<HTMLInputElement>();
onMounted(() => input.value?.select());

const options = computed(() => scenarioOptions(store));
const suggestions = computed(() => [...state.settings.prototypes, ...state.settings.routes]);
const style = computed(() => ({
  left: `${Math.min(props.screen.x, state.viewport.width - 336)}px`,
  top: `${Math.min(props.screen.y, state.viewport.height - 300)}px`,
}));

function add () {
  const s = FRAME_SIZES[size.value];
  const name = route.value.split('/').filter(Boolean).pop() ?? route.value;
  emit('add', {
    kind: 'frame', id: newId('frame'), name, route: route.value.trim() || '/', scenario: scenario.value,
    x: Math.round(props.at.x), y: Math.round(props.at.y), width: s.width, height: s.height,
  });
}
</script>

<template>
  <form class="ac-add ac-popover ac-pop-in" :style="style" data-ui @submit.prevent="add" @pointerdown.stop @keydown.esc="emit('cancel')">
    <p class="ac-heading">Add a screen</p>
    <label class="ac-stack">
      <span class="ac-label">Route</span>
      <input ref="input" v-model="route" class="ac-field mono" list="ac-routes" placeholder="/o/acme/settings/users" />
      <datalist id="ac-routes">
        <option v-for="r in suggestions" :key="r" :value="r" />
      </datalist>
    </label>
    <div class="ac-stack">
      <span class="ac-label">Scenario</span>
      <Select v-model="scenario" :options="options" label="Scenario" />
    </div>
    <div class="ac-stack">
      <span class="ac-label">Size</span>
      <div class="ac-seg fill" role="group" aria-label="Size">
        <button v-for="(s, i) in FRAME_SIZES" :key="s.name" type="button" :aria-pressed="size === i" :title="`${s.width} × ${s.height}`" @click="size = i">{{ s.name }}</button>
      </div>
    </div>
    <div class="ac-row ac-actions">
      <button class="ac-btn ghost" type="button" @click="emit('cancel')">Cancel</button>
      <button class="ac-btn primary" type="submit">Add screen</button>
    </div>
  </form>
</template>

<style>
.ac-add { position: absolute; width: 320px; padding: 16px; display: grid; gap: 16px; z-index: 5; transform-origin: top left; }
.ac-actions { justify-content: flex-end; }
</style>
