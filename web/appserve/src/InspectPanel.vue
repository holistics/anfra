<script setup lang="ts">
import { ref } from 'vue';
import Icon from './Icon.vue';
import {
  formatCondition, formatTime, type InspectedApp, type InspectedControl,
} from './inspect';

// `apps` is undefined until the frame has reported.
defineProps<{ apps?: InspectedApp[] }>();
const emit = defineEmits<{ close: [] }>();

const copied = ref<string>();
async function copy (id: string, text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = id;
    setTimeout(() => { if (copied.value === id) copied.value = undefined; }, 1500);
  } catch { /* clipboard unavailable */ }
}

const controlKind = (control: InspectedControl) => (control.kind === 'dateDrill' ? 'Date drill' : 'Filter');
</script>

<template>
  <aside class="inspect" aria-label="Inspect" data-testid="inspect-panel">
    <header class="inspect-header">
      <h3>Inspect</h3>
      <button type="button" class="icon-button" aria-label="Close inspect panel" @click="emit('close')">
        <Icon name="close" />
      </button>
    </header>
    <div class="inspect-body">
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
        <article v-for="(query, name) in app.queries" :key="name" class="card" :data-testid="`inspect-query-${name}`">
          <header>
            <strong>{{ name }}</strong>
            <span class="pill" :class="query.state" data-testid="query-state">{{ query.state }}</span>
            <span v-if="query.isDirty" class="pill warn">dirty</span>
          </header>
          <dl>
            <dt>Rows</dt><dd>{{ query.rowCount }}<template v-if="query.selectedRowCount"> ({{ query.selectedRowCount }} selected)</template></dd>
            <template v-if="query.debug">
              <dt>Source</dt><dd>{{ query.debug.fromCache ? 'cache' : 'fresh run' }}<template v-if="query.debug.executedAt"> at {{ formatTime(query.debug.executedAt) }}</template></dd>
            </template>
          </dl>
          <details class="signature">
            <summary>Signature</summary>
            <code class="wrap">{{ query.signature }}</code>
          </details>
          <p v-if="query.error" class="error-text" role="alert">{{ query.error.name }}: {{ query.error.message }}</p>
          <template v-for="kind in (['executedAql', 'executedSql'] as const)" :key="kind">
            <div v-if="query.debug?.[kind]" class="code">
              <div class="code-head">
                <span>{{ kind === 'executedAql' ? 'Executed AQL' : 'Executed SQL' }}</span>
                <button
                  type="button"
                  class="copy"
                  :data-testid="`copy-${kind}`"
                  @click="copy(`${index}-${name}-${kind}`, query.debug[kind]!)"
                >
                  {{ copied === `${index}-${name}-${kind}` ? 'Copied' : 'Copy' }}
                </button>
              </div>
              <pre>{{ query.debug[kind] }}</pre>
            </div>
          </template>
        </article>

        <h5>Controls</h5>
        <p v-if="!Object.keys(app.controls).length" class="inspect-note">None.</p>
        <article v-for="(control, name) in app.controls" :key="name" class="card" :data-testid="`inspect-control-${name}`">
          <header>
            <strong>{{ name }}</strong>
            <span class="pill">{{ controlKind(control) }}</span>
            <span v-if="control.isDirty" class="pill warn">unapplied</span>
          </header>
          <dl>
            <dt>Set to</dt><dd data-testid="control-condition">{{ formatCondition(control.condition) }}</dd>
            <dt>Applied</dt><dd data-testid="control-applied">{{ formatCondition(control.appliedCondition) }}</dd>
            <template v-if="control.options"><dt>Options</dt><dd>{{ control.options.join(', ') }}</dd></template>
          </dl>
        </article>

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
