<script setup lang="ts">
// A new board, or a board's details: its name and the project it is in. A new board can start
// blank or from a template: the template's folder is copied into the project, as the person's own
// prototype, and its screens are put on the board.
import { computed, onMounted, ref } from 'vue';
import type { TemplateSummary } from '../../../model/index.ts';
import type { BoardDetails } from '../../../vite/boards.ts';
import { useStore } from '../../context.ts';
import { projects as allProjects } from '../../home.ts';
import Button from '../../ui/Button.vue';
import Field from '../../ui/Field.vue';

const props = defineProps<{ slug?: string, initial: BoardDetails }>();
const emit = defineEmits<{ close: [] }>();

const store = useStore();
const { state } = store;
const name = ref(props.initial.name ?? '');
const project = ref(props.initial.project ?? '');
const known = computed(() => allProjects(state.boards, state.settings).map((p) => p.name));
const listId = `ac-projects-${Math.random().toString(36).slice(2, 8)}`;

const slugOf = (n: string) => n.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 64);
const ready = computed(() => !!slugOf(name.value));

// --- Starting from a template ---------------------------------------------------------------------

const templates = ref<TemplateSummary[]>([]);
/** The template picked, or '' for a blank board. */
const picked = ref('');
const template = computed(() => templates.value.find((t) => t.id === picked.value));
onMounted(async () => {
  if (!props.slug) templates.value = await store.templates();
});
/** Where the copy goes under the prototypes: the project's folder, then the board's. */
const folder = computed(() => [slugOf(project.value), slugOf(name.value)].filter(Boolean).join('/'));

function pick (id: string) {
  picked.value = id;
  if (!name.value.trim() && id) name.value = templates.value.find((t) => t.id === id)?.name ?? '';
}

async function submit () {
  if (!ready.value) return;
  const details = { name: name.value.trim(), project: project.value.trim() };
  if (props.slug) await store.updateDetails(props.slug, details);
  else if (template.value) await store.startFrom({ template: template.value.id, project: folder.value, board: slugOf(name.value), name: details.name });
  else await store.create(slugOf(name.value), details.name, { project: details.project });
  emit('close');
}
</script>

<template>
  <div class="ac-command-scrim" data-ui @pointerdown.self="emit('close')">
    <form class="ac-board-dialog ac-popover form ac-pop-in" :class="{ wide: templates.length }" role="dialog" :aria-label="slug ? 'Board details' : 'New board'" @submit.prevent="submit" @keydown.esc="emit('close')">
      <p class="ac-heading">{{ slug ? 'Board details' : 'New board' }}</p>
      <fieldset v-if="templates.length" class="ac-board-field ac-template-picker">
        <legend class="ac-label">Start from</legend>
        <ul class="ac-template-grid" role="listbox" aria-label="Start from">
          <li>
            <button type="button" class="ac-template-card" role="option" :aria-selected="!picked" @click="pick('')">
              <span class="ac-template-cover blank" aria-hidden="true">+</span>
              <span class="ac-template-name">Blank board</span>
              <span class="ac-caption">Add screens yourself</span>
            </button>
          </li>
          <li v-for="t in templates" :key="t.id">
            <button type="button" class="ac-template-card" role="option" :aria-selected="picked === t.id" @click="pick(t.id)">
              <span class="ac-template-cover" aria-hidden="true">
                <iframe :src="t.preview" :title="`${t.name}, preview`" loading="lazy" tabindex="-1" />
              </span>
              <span class="ac-template-name">{{ t.name }}</span>
              <span class="ac-caption">{{ t.area ? `${t.area} · ` : '' }}{{ t.screens.length }} screen{{ t.screens.length === 1 ? '' : 's' }}</span>
            </button>
          </li>
        </ul>
        <p v-if="template" class="ac-caption">{{ template.description }}</p>
      </fieldset>
      <label class="ac-board-field">
        <span class="ac-label">Name</span>
        <Field v-model="name" placeholder="Dataset list" autofocus />
      </label>
      <label class="ac-board-field">
        <span class="ac-label">Project</span>
        <Field v-model="project" placeholder="datasets" :list="listId" />
        <datalist :id="listId">
          <option v-for="p in known" :key="p" :value="p" />
        </datalist>
        <span v-if="template" class="ac-caption">Its screens are copied to <code class="ac-mono">src/prototypes/{{ folder || '…' }}/</code>, yours to change</span>
        <span v-else class="ac-caption">A board without one is in the folder most of its screens are in</span>
      </label>
      <div class="ac-row ac-board-actions">
        <span class="ac-spacer" />
        <Button variant="ghost" @click="emit('close')">Cancel</Button>
        <Button variant="primary" type="submit" :disabled="!ready">{{ slug ? 'Save' : template ? 'Create from template' : 'Create board' }}</Button>
      </div>
    </form>
  </div>
</template>
