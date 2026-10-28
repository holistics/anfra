<script setup lang="ts">
// The properties of what is selected: a picked element (ElementPanel, written back to its file),
// or a frame, section, note or arrow (on the board).
import { computed } from 'vue';
import {
  FRAME_SIZES, arrange, bounds, isShape, update,
  type Arrow, type BoardObject, type Frame, type Note, type Section as SectionShape, type Shape,
} from '../../model/index.ts';
import { duplicateSelection, wrapSelection } from '../commands.ts';
import { hasScenarios, scenarioOptions, useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';
import PropRow from '../ui/PropRow.vue';
import Section from '../ui/Section.vue';
import Segmented from '../ui/Segmented.vue';
import Select from '../ui/Select.vue';
import ElementPanel from './ElementPanel.vue';

const store = useStore();
const { state, board } = store;

const selected = computed<BoardObject[]>(() => {
  void board.value;
  return store.selected();
});
const one = computed(() => (selected.value.length === 1 ? selected.value[0] : undefined));
const picked = computed(() => state.picked);

// --- Board objects ---------------------------------------------------------------------------------

const set = <T extends BoardObject>(patch: Partial<T>) => {
  if (board.value && one.value) store.commit(update<T>(board.value, one.value.id, patch as never));
};
const num = (e: Event) => Math.round(Number((e.target as HTMLInputElement).value));
const str = (e: Event) => (e.target as HTMLInputElement).value;
const options = computed(() => scenarioOptions(store));
const shapesSelected = computed(() => selected.value.filter(isShape) as Shape[]);

const wrap = () => wrapSelection(store);
const copyShapes = () => duplicateSelection(store);

const frameOf = (o: BoardObject | undefined) => (o?.kind === 'frame' ? (o as Frame) : undefined);
const SIZES = FRAME_SIZES.map((s, i) => ({ value: i, label: s.name, title: `${s.width} × ${s.height}` }));
const sizeOf = (f: Frame) => FRAME_SIZES.findIndex((s) => s.width === f.width && s.height === f.height);
const setSize = (i: number) => set<Frame>({ width: FRAME_SIZES[i].width, height: FRAME_SIZES[i].height });
const routes = computed(() => [...state.settings.prototypes, ...state.settings.routes]);

const SHORTCUTS: [string, string[] | string][] = [
  ['Every command', ['⌘K']],
  ['More on what you click', ['Right-click']],
  ['Select, pan, use the app', ['V', 'H', 'P']],
  ['Comment', ['C']],
  ['Frame, section, note, arrow', ['F', 'S', 'N', 'A']],
  ['Pan', 'Space or scroll'],
  ['Zoom', '⌘ scroll'],
  ['Fit, zoom to selection', ['⇧1', '⇧2']],
  ['Edit text', 'Double-click'],
  ['Duplicate, delete', ['⌘D', '⌫']],
  ['Tidy up the flow', ['⌃⌥T']],
  ['Undo, redo', ['⌘Z', '⇧⌘Z']],
  ['Playing: next, back, stop', ['1', '←', 'Esc']],
];
const mcpCommand = computed(() => `claude mcp add --transport http ${state.settings.mcpName ?? 'anfra-canvas'} ${location.origin}${state.settings.mcp}`);
</script>

<template>
  <aside class="ac-panel ac-inspector" aria-label="Properties">
    <!-- A picked element -->
    <ElementPanel v-if="picked" :key="`${picked.frame}|${picked.info.ref.source ?? picked.info.ref.selector}|${picked.info.ref.index ?? 0}`" />

    <!-- A frame -->
    <template v-else-if="frameOf(one)">
      <Section title="Frame" icon="frame">
        <template #actions>
          <Button variant="ghost" icon="play" tip="Play the flow from this frame" @click="store.play(one!.id)">Play</Button>
        </template>
        <div class="ac-props">
          <PropRow label="Name">
            <Field :value="frameOf(one)!.name" aria-label="Name" @change="set<Frame>({ name: str($event) })" />
          </PropRow>
          <PropRow label="Route">
            <Field :value="frameOf(one)!.route" mono list="ac-routes-inspector" aria-label="Route" @change="set<Frame>({ route: str($event) })" />
          </PropRow>
          <PropRow v-if="hasScenarios(store)" label="Scenario">
            <Select :model-value="frameOf(one)!.scenario ?? 'default'" :options="options" label="Scenario" :min-width="264" @update:model-value="(v) => set<Frame>({ scenario: v })" />
          </PropRow>
        </div>
        <datalist id="ac-routes-inspector"><option v-for="r in routes" :key="r" :value="r" /></datalist>
        <template v-if="state.routes[one!.id] && state.routes[one!.id] !== frameOf(one)!.route">
          <p class="ac-callout ac-caption">Now at <span class="ac-mono">{{ state.routes[one!.id] }}</span></p>
          <div class="ac-row">
            <Button @click="set<Frame>({ route: state.routes[one!.id] })">Keep this route</Button>
          </div>
        </template>
      </Section>
      <Section title="Size">
        <Segmented :model-value="sizeOf(frameOf(one)!)" :options="SIZES" label="Size" fill @update:model-value="(i) => setSize(Number(i))" />
        <div class="ac-pair">
          <Field prefix="W" type="number" aria-label="Width" :value="frameOf(one)!.width" @change="set<Frame>({ width: num($event) })" />
          <Field prefix="H" type="number" aria-label="Height" :value="frameOf(one)!.height" @change="set<Frame>({ height: num($event) })" />
        </div>
      </Section>
    </template>

    <!-- A section, note or arrow -->
    <Section v-else-if="one?.kind === 'section'" title="Section" icon="section">
      <div class="ac-props">
        <PropRow label="Name">
          <Field :value="(one as SectionShape).name" aria-label="Name" @change="set<SectionShape>({ name: str($event) })" />
        </PropRow>
      </div>
      <p class="ac-caption">Moving a section moves what lies inside it</p>
    </Section>
    <Section v-else-if="one?.kind === 'note'" title="Note" icon="note">
      <Field :rows="6" aria-label="Note" :value="(one as Note).text" @change="set<Note>({ text: str($event) })" />
    </Section>
    <Section v-else-if="one?.kind === 'arrow'" title="Arrow" icon="arrow">
      <div class="ac-props">
        <PropRow label="Label">
          <Field :value="(one as Arrow).label ?? ''" placeholder="Invite, Save" aria-label="Label" @change="set<Arrow>({ label: str($event) || undefined })" />
        </PropRow>
      </div>
      <p class="ac-caption">What the person does to go this way. It names the way on when the flow is played</p>
    </Section>

    <!-- Several -->
    <Section v-else-if="shapesSelected.length > 1" :title="`${shapesSelected.length} selected`">
      <div class="ac-row tight">
        <Button icon="alignLeft" label="Align left edges" @click="board && store.commit(arrange(board, state.selection, 'left'))" />
        <Button icon="alignTop" label="Align top edges" @click="board && store.commit(arrange(board, state.selection, 'top'))" />
        <Button icon="row" label="Lay out in a row" @click="board && store.commit(arrange(board, state.selection, 'row'))" />
        <Button icon="column" label="Lay out in a column" @click="board && store.commit(arrange(board, state.selection, 'column'))" />
      </div>
      <div class="ac-row">
        <Button icon="section" @click="wrap">Put in a section</Button>
      </div>
    </Section>

    <!-- Nothing -->
    <template v-else>
      <Section title="Connect Claude" icon="plug">
        <p class="ac-caption">Claude reads the comments, changes the code they point at, and resolves them. Run this once in the repo:</p>
        <div class="ac-snippet">
          <pre>{{ mcpCommand }}</pre>
          <Button variant="ghost" icon="copy" label="Copy the command" @click="store.copy(mcpCommand, 'Command copied')" />
        </div>
        <p class="ac-caption">Then ask Claude to handle the open feedback on the canvas. Or copy the comments as Markdown and paste them anywhere</p>
      </Section>
      <Section title="Shortcuts">
        <dl class="ac-dl">
          <div v-for="[what, keys] in SHORTCUTS" :key="what">
            <dt>{{ what }}</dt>
            <dd v-if="typeof keys === 'string'">{{ keys }}</dd>
            <dd v-else><kbd v-for="k in keys" :key="k" class="ac-kbd">{{ k }}</kbd></dd>
          </div>
        </dl>
      </Section>
    </template>

    <Section v-if="shapesSelected.length && !picked">
      <div class="ac-row">
        <Button icon="copy" @click="copyShapes">Duplicate</Button>
        <Button icon="trash" @click="store.removeSelection()">Delete</Button>
      </div>
    </Section>
  </aside>
</template>
