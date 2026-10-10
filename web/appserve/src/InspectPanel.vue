<script setup lang="ts">
// The inspect panel: a devtool over the running Data App. Structure shows the page's semantic
// parts as a tree, with hover-highlight, click-to-locate, a pick mode and a detail pane; Data
// shows every app's queries, controls and selection.
import { computed, ref } from 'vue';
import Icon from './Icon.vue';
import StructureTree from './StructureTree.vue';
import EntityCard from './EntityCard.vue';
import {
  countProblems, findNode, formatCondition, handleFor, usageOf,
  type InspectedApp, type InspectedStructure, type LocateTarget,
} from './inspect';

// `apps` and `structure` are undefined until the frame has reported.
const props = defineProps<{
  apps?: InspectedApp[],
  structure?: InspectedStructure,
  selectedNode?: string,
  picking: boolean,
}>();
const emit = defineEmits<{
  close: [],
  highlight: [node: string | null],
  locate: [target: LocateTarget],
  select: [node: string | undefined],
  pick: [on: boolean],
}>();

const tab = ref<'structure' | 'data'>('structure');
const selected = computed(() => (props.structure && props.selectedNode ? findNode(props.structure.nodes, props.selectedNode) : undefined));
const problems = computed(() => (props.structure ? countProblems(props.structure.nodes) : 0));

const copied = ref(false);
async function copyHandle (): Promise<void> {
  if (!selected.value) return;
  try {
    await navigator.clipboard.writeText(handleFor(selected.value));
    copied.value = true;
    setTimeout(() => { copied.value = false; }, 1500);
  } catch { /* clipboard unavailable */ }
}

function select (key: string): void {
  emit('select', key);
  emit('locate', { node: key });
}

// A query or control node's entity, for its card in the detail pane.
const selectedEntity = computed(() => {
  const node = selected.value;
  if (!node || (node.kind !== 'query' && node.kind !== 'control') || node.problems.length) return undefined;
  const app = props.apps?.[node.app ?? 0];
  if (!app) return undefined;
  const name = node.name ?? '';
  return {
    app: node.app ?? 0,
    name,
    query: node.kind === 'query' ? app.queries[name] : undefined,
    control: node.kind === 'control' ? app.controls[name] : undefined,
    usage: usageOf(props.structure, node.kind, node.app ?? 0, name),
  };
});
</script>

<template>
  <aside class="inspect" aria-label="Inspect" data-testid="inspect-panel">
    <header class="inspect-header">
      <div class="inspect-tabs" role="tablist">
        <button type="button" role="tab" :aria-selected="tab === 'structure'" data-testid="tab-structure" @click="tab = 'structure'">
          Structure<span v-if="problems" class="pill error">{{ problems }}</span>
        </button>
        <button type="button" role="tab" :aria-selected="tab === 'data'" data-testid="tab-data" @click="tab = 'data'">Data</button>
      </div>
      <button type="button" class="icon-button" aria-label="Close inspect panel" @click="emit('close')">
        <Icon name="close" />
      </button>
    </header>

    <div v-if="tab === 'structure'" class="inspect-body" @mouseleave="emit('highlight', null)">
      <div class="toolbar">
        <button
          type="button"
          class="tool"
          :aria-pressed="picking"
          title="Click a part of the page to select it here. Esc stops."
          data-testid="pick"
          @click="emit('pick', !picking)"
        >
          Pick
        </button>
        <button type="button" class="tool" :disabled="!selected" data-testid="copy-handle" @click="copyHandle">
          {{ copied ? 'Copied' : 'Copy handle' }}
        </button>
      </div>
      <p v-if="!structure" class="inspect-note" data-testid="inspect-waiting">Waiting for the Data App…</p>
      <p v-else-if="structure.nodes.length === 0" class="inspect-note" data-testid="structure-none">
        Nothing is marked yet. Add <code>data-anfra-container</code>, <code>data-anfra-block</code>,
        <code>data-anfra-query</code> and <code>data-anfra-control</code> to the page's elements and they show up here.
      </p>
      <StructureTree
        v-else
        :nodes="structure.nodes"
        :selected="selectedNode"
        @hover="emit('highlight', $event)"
        @select="select"
      />

      <section v-if="selected" class="detail" data-testid="node-detail">
        <h5>{{ selected.kind }}</h5>
        <dl>
          <dt>{{ selected.kind === 'container' || selected.kind === 'block' ? 'Id' : 'Name' }}</dt>
          <dd>{{ selected.kind === 'container' || selected.kind === 'block' ? selected.id : selected.name }}</dd>
          <template v-if="selected.label"><dt>Label</dt><dd>{{ selected.label }}</dd></template>
          <dt>Handle</dt><dd><code class="wrap">{{ handleFor(selected) }}</code></dd>
          <template v-if="selected.children.length"><dt>Holds</dt><dd>{{ selected.children.length }} node{{ selected.children.length === 1 ? '' : 's' }}</dd></template>
        </dl>
        <p v-for="(problem, i) in selected.problems" :key="i" class="error-text" role="alert">{{ problem }}</p>
        <EntityCard v-if="selectedEntity" v-bind="selectedEntity" @locate="emit('locate', $event)" />
      </section>
    </div>

    <div v-else class="inspect-body">
      <p v-if="!apps" class="inspect-note" data-testid="inspect-waiting">Waiting for the Data App…</p>
      <p v-else-if="apps.length === 0" class="inspect-note" data-testid="inspect-none">
        This Data App hasn't created an SDK app.
      </p>
      <section v-for="(app, index) in apps" v-else :key="index" class="inspect-app" data-testid="inspect-app">
        <h4 v-if="apps.length > 1" class="app-title">{{ app.title || `App ${index + 1}` }}</h4>

        <h5>App</h5>
        <dl>
          <dt>Title</dt><dd>{{ app.title || '—' }}</dd>
          <dt>Timezone</dt><dd>{{ app.timezone || 'tenant default' }}</dd>
          <dt>Unapplied changes</dt>
          <dd><span class="pill" :class="{ warn: app.hasChanges }">{{ app.hasChanges ? 'yes' : 'no' }}</span></dd>
        </dl>

        <h5>Queries</h5>
        <p v-if="!Object.keys(app.queries).length" class="inspect-note">None.</p>
        <EntityCard
          v-for="(query, name) in app.queries"
          :key="name"
          :app="index"
          :name="String(name)"
          :query="query"
          :usage="usageOf(structure, 'query', index, String(name))"
          @locate="emit('locate', $event)"
        />

        <h5>Controls</h5>
        <p v-if="!Object.keys(app.controls).length" class="inspect-note">None.</p>
        <EntityCard
          v-for="(control, name) in app.controls"
          :key="name"
          :app="index"
          :name="String(name)"
          :control="control"
          :usage="usageOf(structure, 'control', index, String(name))"
          @locate="emit('locate', $event)"
        />

        <template v-if="app.selection || app.appliedSelection">
          <h5>Selection</h5>
          <article
            v-for="(selection, label) in { Current: app.selection, Applied: app.appliedSelection }"
            v-show="selection"
            :key="label"
            class="card"
          >
            <template v-if="selection">
              <header>
                <strong>{{ label }}</strong>
                <span v-if="selection.lossy" class="pill warn">lossy</span>
              </header>
              <dl>
                <dt>From</dt><dd>{{ selection.source }}</dd>
                <dt>Rows</dt><dd>{{ selection.rowCount }}</dd>
                <template v-if="selection.expression"><dt>Expression</dt><dd><code class="wrap">{{ selection.expression }}</code></dd></template>
                <template v-for="(condition, i) in selection.conditions" :key="i">
                  <dt>{{ condition.field }}</dt><dd>{{ formatCondition(condition) }}</dd>
                </template>
              </dl>
            </template>
          </article>
        </template>
      </section>
    </div>
  </aside>
</template>
