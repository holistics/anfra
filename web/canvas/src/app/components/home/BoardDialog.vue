<script setup lang="ts">
// A new board, or a board's details: its name, the project it is in, and who owns it.
import { computed, ref } from 'vue';
import type { BoardDetails } from '../../../vite/boards.ts';
import { useStore } from '../../context.ts';
import { people as everyone, projects as allProjects } from '../../home.ts';
import Avatar from '../../ui/Avatar.vue';
import Button from '../../ui/Button.vue';
import Field from '../../ui/Field.vue';

const props = defineProps<{ slug?: string, initial: BoardDetails }>();
const emit = defineEmits<{ close: [] }>();

const store = useStore();
const { state } = store;
const name = ref(props.initial.name ?? '');
const project = ref(props.initial.project ?? '');
const owners = ref<string[]>([...(props.initial.owners ?? [])]);
const known = computed(() => allProjects(state.boards, state.settings).map((p) => p.name));
const team = computed(() => everyone(state.boards, state.settings));
const listId = `ac-projects-${Math.random().toString(36).slice(2, 8)}`;

const slugOf = (n: string) => n.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 64);
const ready = computed(() => !!slugOf(name.value));

function toggle (id: string) {
  owners.value = owners.value.includes(id) ? owners.value.filter((o) => o !== id) : [...owners.value, id];
}

async function submit () {
  if (!ready.value) return;
  const details = { name: name.value.trim(), project: project.value.trim(), owners: owners.value };
  if (props.slug) await store.updateDetails(props.slug, details);
  else await store.create(slugOf(name.value), details.name, { project: details.project, owners: details.owners });
  emit('close');
}
</script>

<template>
  <div class="ac-command-scrim" data-ui @pointerdown.self="emit('close')">
    <form class="ac-board-dialog ac-popover form ac-pop-in" role="dialog" :aria-label="slug ? 'Board details' : 'New board'" @submit.prevent="submit" @keydown.esc="emit('close')">
      <p class="ac-heading">{{ slug ? 'Board details' : 'New board' }}</p>
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
        <span class="ac-caption">A board without one is in the folder most of its screens are in</span>
      </label>
      <div v-if="team.length" class="ac-board-field">
        <span class="ac-label">Owners</span>
        <div class="ac-owner-list" role="group" aria-label="Owners">
          <button v-for="p in team" :key="p.id" type="button" class="ac-owner" :aria-pressed="owners.includes(p.id)" @click="toggle(p.id)">
            <Avatar :name="p.name" :src="p.avatarUrl" :size="20" />
            <span class="ac-truncate">{{ p.name }}</span>
          </button>
        </div>
      </div>
      <div class="ac-row ac-board-actions">
        <span class="ac-spacer" />
        <Button variant="ghost" @click="emit('close')">Cancel</Button>
        <Button variant="primary" type="submit" :disabled="!ready">{{ slug ? 'Save' : 'Create board' }}</Button>
      </div>
    </form>
  </div>
</template>
