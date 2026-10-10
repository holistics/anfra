<script setup lang="ts">
// One query's or control's card: its state and what it is drawn by. Shown in the Data tab for
// every entity, and in the Structure tab for a selected query or control node.
import { ref } from 'vue';
import {
  entityAttribute, formatCondition, formatTime,
  type EntityUsage, type InspectedControl, type InspectedQuery, type LocateTarget,
} from './inspect';

const props = defineProps<{
  app: number,
  name: string,
  query?: InspectedQuery,
  control?: InspectedControl,
  usage?: EntityUsage,
}>();
const emit = defineEmits<{ locate: [target: LocateTarget] }>();

const kind = () => (props.query ? 'query' : 'control');
const copied = ref<string>();
async function copy (id: string, text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = id;
    setTimeout(() => { if (copied.value === id) copied.value = undefined; }, 1500);
  } catch { /* clipboard unavailable */ }
}
const controlKind = (control: InspectedControl) => (control.kind === 'dateDrill' ? 'Date drill' : 'Filter');
const drawn = () => (props.usage ? `${props.usage.markers} on the page${props.usage.blocks.length ? `, in ${props.usage.blocks.join(', ')}` : ''}` : 'not on the page');
</script>

<template>
  <article class="card" :data-testid="`inspect-${kind()}-${name}`">
    <header>
      <strong>{{ name }}</strong>
      <template v-if="query">
        <span class="pill" :class="query.state" data-testid="query-state">{{ query.state }}</span>
        <span v-if="query.isDirty" class="pill warn">dirty</span>
      </template>
      <template v-else-if="control">
        <span class="pill">{{ controlKind(control) }}</span>
        <span v-if="control.isDirty" class="pill warn">unapplied</span>
      </template>
      <button
        type="button"
        class="locate"
        :disabled="!usage"
        :title="usage ? 'Scroll to it on the page' : `Nothing on the page is marked ${entityAttribute(kind(), app, name)}`"
        data-testid="locate"
        @click="emit('locate', { app, kind: kind(), name })"
      >
        Locate
      </button>
    </header>
    <dl>
      <dt>Drawn</dt>
      <dd :data-testid="`${kind()}-usage`">
        {{ drawn() }}
        <button type="button" class="copy" :title="entityAttribute(kind(), app, name)" @click="copy('handle', entityAttribute(kind(), app, name))">
          {{ copied === 'handle' ? 'Copied' : 'Copy handle' }}
        </button>
      </dd>
      <template v-if="query">
        <dt>Rows</dt><dd>{{ query.rowCount }}<template v-if="query.selectedRowCount"> ({{ query.selectedRowCount }} selected)</template></dd>
        <template v-if="query.debug">
          <dt>Source</dt><dd>{{ query.debug.fromCache ? 'cache' : 'fresh run' }}<template v-if="query.debug.executedAt"> at {{ formatTime(query.debug.executedAt) }}</template></dd>
        </template>
      </template>
      <template v-else-if="control">
        <dt>Set to</dt><dd data-testid="control-condition">{{ formatCondition(control.condition) }}</dd>
        <dt>Applied</dt><dd data-testid="control-applied">{{ formatCondition(control.appliedCondition) }}</dd>
        <template v-if="control.options"><dt>Options</dt><dd>{{ control.options.join(', ') }}</dd></template>
      </template>
    </dl>
    <template v-if="query">
      <details class="signature">
        <summary>Signature</summary>
        <code class="wrap">{{ query.signature }}</code>
      </details>
      <p v-if="query.error" class="error-text" role="alert">{{ query.error.name }}: {{ query.error.message }}</p>
      <template v-for="field in (['executedAql', 'executedSql'] as const)" :key="field">
        <div v-if="query.debug?.[field]" class="code">
          <div class="code-head">
            <span>{{ field === 'executedAql' ? 'Executed AQL' : 'Executed SQL' }}</span>
            <button type="button" class="copy" :data-testid="`copy-${field}`" @click="copy(field, query.debug![field]!)">
              {{ copied === field ? 'Copied' : 'Copy' }}
            </button>
          </div>
          <pre>{{ query.debug[field] }}</pre>
        </div>
      </template>
    </template>
  </article>
</template>
