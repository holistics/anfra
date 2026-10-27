<script setup lang="ts">
// The Changes tab: what changed in the app's files, read from git. Since the last commit, with a
// box to commit it (on a new branch when this is the default branch; nothing is pushed), or all
// this branch holds that the default branch doesn't, with its commits.
import { computed, reactive, ref, watch } from 'vue';
import type { CommitInfo, FileDiff } from '../../vite/changes.ts';
import { useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';
import Icon from '../ui/Icon.vue';
import Segmented, { type SegmentedOption } from '../ui/Segmented.vue';
import Switch from '../ui/Switch.vue';
import { vTip } from '../ui/tooltip.ts';
import FileChange from './FileChange.vue';

const store = useStore();
const { state } = store;
const c = computed(() => state.changes);
const onBranch = computed(() => !!c.value && !c.value.needsBranch);

const SCOPES = computed<SegmentedOption<typeof state.changesScope>[]>(() => [
  { value: 'head', label: 'Not committed', count: c.value?.pending.length },
  { value: 'branch', label: 'This branch' },
]);

// --- Committing ---------------------------------------------------------------------------------

/** The start a repo's checks want (`feat(ui):`), shown before the message and not typed. */
const prefix = computed(() => state.settings.commitPrefix.trim());
const message = ref('');
const branch = ref('');
const error = ref('');
const busy = ref(false);
const count = computed(() => c.value?.files.length ?? 0);
const ready = computed(() => !busy.value && !!message.value.trim() && (!c.value?.needsBranch || !!branch.value.trim()));

async function commit () {
  if (!ready.value) return;
  let text = message.value.trim();
  // After a conventional prefix the subject starts in lower case, as the checks want.
  if (prefix.value && /^[A-Z][a-z]/.test(text)) text = text[0].toLowerCase() + text.slice(1);
  busy.value = true;
  error.value = (await store.commitChanges(prefix.value ? `${prefix.value} ${text}` : text, c.value?.needsBranch ? branch.value.trim() : undefined)) ?? '';
  busy.value = false;
  if (!error.value) {
    message.value = '';
    branch.value = '';
  }
}

// --- The branch's commits -----------------------------------------------------------------------

const opened = reactive<Record<string, FileDiff[] | null>>({});
async function toggle (k: CommitInfo) {
  if (k.sha in opened) {
    delete opened[k.sha];
    return;
  }
  opened[k.sha] = null;
  opened[k.sha] = await store.readCommit(k.sha);
}
watch(() => c.value?.head, () => {
  for (const sha of Object.keys(opened)) delete opened[sha];
});
</script>

<template>
  <div class="ac-list" role="tabpanel">
    <p v-if="!c" class="ac-caption ac-list-empty">Reading the changes</p>
    <div v-else-if="!c.repo" class="ac-list-empty">
      <p class="ac-heading">Not in a git repository</p>
      <p class="ac-caption">The canvas reads what changed from git. Run git init in the app's folder to start one</p>
    </div>
    <template v-else>
      <div class="ac-changes-head">
        <div class="ac-row">
          <Icon name="branch" />
          <span v-tip="'The branch you are on'" class="ac-changes-branch ac-truncate">{{ c.branch }}</span>
          <span v-if="onBranch" class="ac-caption ac-truncate">from {{ c.defaultBranch }}</span>
          <span class="ac-spacer" />
          <Button variant="ghost" icon="reload" label="Read the changes again" @click="store.loadChanges()" />
        </div>
        <Segmented v-if="onBranch" v-model="state.changesScope" :options="SCOPES" label="Which changes" fill />
        <div v-if="Object.keys(c.marks).length" class="ac-row">
          <span class="ac-caption">Outline them on the screens</span>
          <span class="ac-spacer" />
          <Switch v-model="state.outlineChanges" label="Outline the changed elements on the screens" />
        </div>
      </div>

      <div v-if="!c.files.length" class="ac-list-empty">
        <p class="ac-heading">{{ c.scope === 'head' ? 'Nothing changed' : 'No changes on this branch' }}</p>
        <p class="ac-caption">
          {{ c.scope === 'head'
            ? 'Edits you make on the canvas, and changes Claude makes to the code, show here as they happen'
            : `This branch has nothing ${c.defaultBranch} doesn't have yet` }}
        </p>
      </div>
      <ul v-else class="ac-files">
        <li v-for="f in c.files" :key="f.path"><FileChange :file="f" /></li>
      </ul>

      <form v-if="c.scope === 'head' && count" class="ac-commit" @submit.prevent="commit">
        <p class="ac-heading">Commit {{ count }} {{ count === 1 ? 'file' : 'files' }}</p>
        <label v-if="c.needsBranch" class="ac-stack">
          <span class="ac-caption">You're on {{ c.branch }}, so this work goes on a new branch</span>
          <Field v-model="branch" mono placeholder="design/invite-copy" aria-label="The new branch's name" />
        </label>
        <Field v-model="message" :prefix="prefix || undefined" placeholder="what changed, in a line" aria-label="What changed" />
        <p v-if="error" class="ac-callout ac-commit-error ac-mono" role="alert">{{ error }}</p>
        <Button variant="primary" type="submit" icon="commit" :disabled="!ready">{{ busy ? 'Committing' : 'Commit' }}</Button>
        <p class="ac-caption">Saved in git on this computer. Nothing is pushed</p>
      </form>

      <template v-if="c.scope === 'branch' && c.commits.length">
        <p class="ac-changes-sub ac-heading">{{ c.commits.length }} {{ c.commits.length === 1 ? 'commit' : 'commits' }}</p>
        <ul class="ac-files">
          <li v-for="k in c.commits" :key="k.sha">
            <button class="ac-file-head" type="button" :aria-expanded="k.sha in opened" @click="toggle(k)">
              <Icon name="chevronRight" :size="14" />
              <span class="ac-file-text">
                <span class="ac-file-name"><span class="ac-truncate">{{ k.subject }}</span></span>
                <span class="ac-caption ac-truncate">{{ k.short }} · {{ k.author }} · {{ k.when }}</span>
              </span>
            </button>
            <p v-if="opened[k.sha] === null" class="ac-caption ac-changes-nested">Reading the commit</p>
            <ul v-else-if="opened[k.sha]" class="ac-files ac-changes-nested">
              <li v-for="f in opened[k.sha]" :key="f.path"><FileChange :file="f" /></li>
            </ul>
          </li>
        </ul>
      </template>
    </template>
  </div>
</template>
