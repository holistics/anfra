<script setup lang="ts">
// The picked element, edited as a designer edits a layer: its component's props, its text, and
// its layout, spacing, size, type and fill. Every control writes the element's source (a prop is
// an attribute, a style is a Tailwind class from the theme's tokens) and the screen redraws from
// the file. Moving, duplicating and removing it write the file too; each is one undo step.
import { computed, ref, watch } from 'vue';
import { elementMarkdown } from '../../model/index.ts';
import type { PropSpec } from '../../vite/catalog.ts';
import type { Edit, SourceInfo } from '../../vite/source.ts';
import { choices, groups, read, sizeClass, sizeOf, write, type Group, type SizeMode } from '../classes.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';
import Select, { type SelectOption } from './Select.vue';

const store = useStore();
const { state, board } = store;
// The last element picked: the panel's watchers can run once more after the pick is cleared,
// before the panel is gone.
let last = state.picked!;
const picked = computed(() => (last = state.picked ?? last));
const ref_ = computed(() => picked.value.info.ref);
const G = groups(state.settings.tokens);
const C = choices(state.settings.tokens);

// --- Its source ------------------------------------------------------------------------------------

const src = ref<SourceInfo>();
const text = ref('');
const classes = ref('');
watch(() => [ref_.value.source, state.sources] as const, async ([loc]) => {
  if (!loc) {
    src.value = undefined;
    return;
  }
  const s = await store.source(loc);
  if (ref_.value.source !== loc) return;
  src.value = s;
  if (document.activeElement?.closest('.ac-element') == null) {
    text.value = s?.text ?? '';
    classes.value = s?.classes ?? '';
  }
}, { immediate: true });

const editable = computed(() => !!ref_.value.source && !!src.value);
const cls = computed(() => src.value?.classes ?? '');

async function change (edit: Edit, done = '') {
  const p = picked.value;
  const ok = await store.edit(p.frame, p.info.ref, edit, done);
  // Seen at once, before the screen redraws and the file is read again.
  if (ok && src.value && edit.kind === 'class') src.value = { ...src.value, classes: edit.to };
  return ok;
}

// --- Props -----------------------------------------------------------------------------------------

/** Attributes worth a control on plain elements and the components standing for them. */
const NATIVE: Record<string, PropSpec[]> = {
  button: [{ name: 'disabled', type: 'boolean' }],
  input: [
    { name: 'placeholder', type: 'string' },
    { name: 'type', type: 'enum', options: ['text', 'email', 'password', 'number', 'search', 'url', 'tel'] },
    { name: 'disabled', type: 'boolean' },
  ],
  textarea: [{ name: 'placeholder', type: 'string' }, { name: 'disabled', type: 'boolean' }],
  a: [{ name: 'href', type: 'string' }],
  img: [{ name: 'src', type: 'string' }, { name: 'alt', type: 'string' }],
};
const STANDS_FOR: Record<string, string> = { Button: 'button', Input: 'input', Textarea: 'textarea', Checkbox: 'button', Switch: 'button' };

const component = computed(() => state.settings.catalog.find((c) => c.name === ref_.value.component));
const props = computed<PropSpec[]>(() => {
  const own = component.value?.props ?? [];
  const tag = ref_.value.component ? STANDS_FOR[ref_.value.component] : src.value?.tag;
  const extra = (tag && NATIVE[tag]) || [];
  return [...own, ...extra.filter((e) => !own.some((o) => o.name === e.name))];
});

const kebab = (s: string) => s.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase();
const attr = (name: string) => src.value?.attrs.find((a) => a.name === name || kebab(a.name) === kebab(name));

const enumOptions = (p: PropSpec): SelectOption[] => [
  { value: '', label: p.default !== undefined ? `${p.default}` : 'Not set', hint: p.default !== undefined ? 'default' : undefined },
  ...(p.options ?? []).filter((o) => o !== p.default).map((o) => ({ value: o, label: o })),
];
const enumValue = (p: PropSpec) => {
  const v = attr(p.name)?.value;
  return typeof v === 'string' && v !== p.default ? v : '';
};
const boolValue = (p: PropSpec) => {
  const a = attr(p.name);
  return a ? a.value === true || a.value === '' || a.value === 'true' : p.default === true;
};
const setProp = (p: PropSpec, value: string | boolean | null) => change({ kind: 'attr', name: p.name, value: value === '' ? null : value });

// --- Text ------------------------------------------------------------------------------------------

async function saveText () {
  if (src.value?.text === undefined || text.value.trim() === src.value.text) return;
  await change({ kind: 'text', from: src.value.text, to: text.value });
}

// --- Design ----------------------------------------------------------------------------------------

const get = (g: Group) => read(cls.value, g) ?? '';
const set = (g: Group, value: string) => {
  const to = write(cls.value, g, value);
  if (to !== cls.value) void change({ kind: 'class', to });
};

type Display = 'block' | 'row' | 'column' | 'grid';
const display = computed<Display>(() => {
  const d = get(G.display);
  if (d === 'grid' || d === 'inline-grid') return 'grid';
  if (d === 'flex' || d === 'inline-flex') return get(G.direction).startsWith('flex-col') ? 'column' : 'row';
  return 'block';
});
const computedDisplay = computed(() => picked.value.info.style.display);
function setDisplay (to: Display) {
  let c = write(cls.value, G.direction, '');
  if (to === 'block') {
    c = write(c, G.display, '');
    for (const g of [G.gap, G.items, G.justify]) c = write(c, g, '');
  } else {
    c = write(c, G.display, to === 'grid' ? 'grid' : get(G.display) === 'inline-flex' ? 'inline-flex' : 'flex');
    if (to === 'column') c = write(c, G.direction, 'flex-col');
  }
  if (c !== cls.value) void change({ kind: 'class', to: c });
}

/** Padding on one axis: a `p-*` is split into its two axes first, so the other keeps its value. */
const padding = (axis: 'px' | 'py') => get(G[axis]) || (get(G.padding) ? `${axis}-${get(G.padding).slice(2)}` : '');
function setPadding (axis: 'px' | 'py', value: string) {
  let c = cls.value;
  const all = read(c, G.padding);
  if (all) {
    const n = all.slice(2);
    c = write(c, G.padding, '');
    c = write(c, G.px, `px-${n}`);
    c = write(c, G.py, `py-${n}`);
  }
  c = write(c, G[axis], value);
  if (c !== cls.value) void change({ kind: 'class', to: c });
}

const SIZE_MODES: SelectOption[] = [
  { value: 'auto', label: 'Auto' },
  { value: 'fill', label: 'Fill', description: 'As wide as its container' },
  { value: 'hug', label: 'Hug', description: 'As wide as its content' },
  { value: 'fixed', label: 'Fixed' },
];
const size = (axis: 'w' | 'h') => sizeOf(read(cls.value, axis === 'w' ? G.width : G.height), axis);
function setSize (axis: 'w' | 'h', mode: SizeMode, px?: number) {
  const now = Math.round(axis === 'w' ? picked.value.info.rect.width : picked.value.info.rect.height);
  set(axis === 'w' ? G.width : G.height, sizeClass(axis, mode, px ?? (size(axis).px || now)));
}

const ALIGN = [
  { value: '', icon: 'textLeft', label: 'Left' },
  { value: 'text-center', icon: 'textCenter', label: 'Center' },
  { value: 'text-right', icon: 'textRight', label: 'Right' },
] as const;

const border = computed(() => !!get(G.border));

// --- Actions ---------------------------------------------------------------------------------------

const commentOn = () => store.commentOn(picked.value.frame, picked.value.info);

const where = (loc: string) => {
  const i = loc.lastIndexOf('/');
  return { file: loc.slice(i + 1), dir: i > 0 ? loc.slice(0, i) : '' };
};
const openInEditor = (loc: string) => fetch(`/__open-in-editor?file=${encodeURIComponent(loc)}`);
const title = computed(() => ref_.value.component ?? ref_.value.tag);
</script>

<template>
  <div class="ac-element">
    <div class="ac-section">
      <div class="ac-section-head">
        <Icon :name="ref_.component ? 'component' : 'box'" class="ac-inspector-icon" />
        <span class="ac-heading ac-truncate">{{ title }}</span>
        <span class="ac-spacer" />
        <div class="ac-row ac-row-tight">
          <button class="ac-btn ghost icon" type="button" title="Select the parent (⇧↵)" aria-label="Select the parent" @click="store.pickParent()"><Icon name="parent" /></button>
          <template v-if="editable">
          <button class="ac-btn ghost icon" type="button" title="Move up (⌥↑)" aria-label="Move up" :disabled="src?.first" @click="change({ kind: 'move', by: -1 })"><Icon name="arrowUp" /></button>
          <button class="ac-btn ghost icon" type="button" title="Move down (⌥↓)" aria-label="Move down" :disabled="src?.last" @click="change({ kind: 'move', by: 1 })"><Icon name="arrowDown" /></button>
          <button class="ac-btn ghost icon" type="button" title="Duplicate (⌘D)" aria-label="Duplicate" @click="change({ kind: 'duplicate' }, 'Duplicated')"><Icon name="duplicate" /></button>
          <button class="ac-btn ghost icon" type="button" title="Delete (⌫)" aria-label="Delete" @click="change({ kind: 'remove' }, `${title} removed`)"><Icon name="trash" /></button>
          </template>
        </div>
      </div>
      <button v-if="ref_.source" class="ac-source" type="button" :title="`Open ${ref_.source} in your editor`" @click="openInEditor(ref_.source)">
        <span class="ac-source-file ac-mono ac-truncate">{{ where(ref_.source).file }}</span>
        <Icon name="external" :size="14" />
        <span class="ac-source-dir ac-caption ac-truncate"><bdi>{{ where(ref_.source).dir }}</bdi></span>
      </button>
      <p v-else class="ac-caption">Drawn at run time, not written in a template: only a comment can change it</p>
      <div class="ac-row">
        <button class="ac-btn" type="button" @click="commentOn"><Icon name="comment" /> Comment</button>
        <button class="ac-btn ghost icon" type="button" title="Copy as Markdown for Claude" aria-label="Copy as Markdown" @click="board && store.copy(elementMarkdown(board, ref_), 'Element copied as Markdown')"><Icon name="copy" /></button>
      </div>
    </div>

    <template v-if="editable">
      <div v-if="props.length || src?.text !== undefined" class="ac-section">
        <p class="ac-heading">{{ component ? 'Properties' : 'Content' }}</p>
        <div class="ac-grid">
          <template v-if="src?.text !== undefined">
            <label class="ac-grid-label" for="ac-el-text">Text</label>
            <input id="ac-el-text" v-model="text" class="ac-field" @keydown.enter="saveText" @blur="saveText" />
          </template>
          <template v-for="p in props" :key="p.name">
            <span class="ac-grid-label ac-truncate" :title="p.name">{{ p.name }}</span>
            <p v-if="attr(p.name)?.bound !== undefined" class="ac-caption ac-bound" :title="attr(p.name)!.bound">Bound to <code class="ac-mono">{{ attr(p.name)!.bound }}</code></p>
            <Select v-else-if="p.type === 'enum'" :model-value="enumValue(p)" :options="enumOptions(p)" :label="p.name" @update:model-value="(v) => setProp(p, v)" />
            <button v-else-if="p.type === 'boolean'" class="ac-switch" type="button" role="switch" :aria-checked="boolValue(p)" :aria-label="p.name" @click="setProp(p, !boolValue(p) || null)" />
            <input
              v-else
              class="ac-field"
              :type="p.type === 'number' ? 'number' : 'text'"
              :value="typeof attr(p.name)?.value === 'string' ? attr(p.name)!.value : ''"
              :placeholder="p.default !== undefined ? String(p.default) : ''"
              :aria-label="p.name"
              @change="setProp(p, ($event.target as HTMLInputElement).value)"
            />
          </template>
        </div>
      </div>

      <div class="ac-section">
        <p class="ac-heading">Layout</p>
        <div class="ac-seg fill" role="group" aria-label="Layout">
          <button type="button" :aria-pressed="display === 'block'" title="Stacked as written" @click="setDisplay('block')">Block</button>
          <button type="button" :aria-pressed="display === 'row'" title="Auto layout, side by side" @click="setDisplay('row')">Row</button>
          <button type="button" :aria-pressed="display === 'column'" title="Auto layout, one under another" @click="setDisplay('column')">Column</button>
          <button type="button" :aria-pressed="display === 'grid'" @click="setDisplay('grid')">Grid</button>
        </div>
        <p v-if="display === 'block' && computedDisplay && computedDisplay !== 'block'" class="ac-caption">Drawn as {{ computedDisplay }} by its component</p>
        <div v-if="display !== 'block'" class="ac-grid">
          <span class="ac-grid-label">Gap</span>
          <Select :model-value="get(G.gap)" :options="C.gap" label="Gap" @update:model-value="(v) => set(G.gap, v)" />
          <span class="ac-grid-label">Align</span>
          <Select :model-value="get(G.items)" :options="C.items" label="Align items" @update:model-value="(v) => set(G.items, v)" />
          <span class="ac-grid-label">Justify</span>
          <Select :model-value="get(G.justify)" :options="C.justify" label="Justify" @update:model-value="(v) => set(G.justify, v)" />
        </div>
        <div class="ac-grid">
          <span class="ac-grid-label" title="Padding left and right, in px">Padding x</span>
          <Select :model-value="padding('px')" :options="C.px" label="Padding left and right" @update:model-value="(v) => setPadding('px', v)" />
          <span class="ac-grid-label" title="Padding top and bottom, in px">Padding y</span>
          <Select :model-value="padding('py')" :options="C.py" label="Padding top and bottom" @update:model-value="(v) => setPadding('py', v)" />
        </div>
      </div>

      <div class="ac-section">
        <p class="ac-heading">Size</p>
        <div class="ac-grid">
          <template v-for="axis in (['w', 'h'] as const)" :key="axis">
            <span class="ac-grid-label">{{ axis === 'w' ? 'Width' : 'Height' }}</span>
            <div class="ac-pair">
              <Select :model-value="size(axis).mode" :options="SIZE_MODES" :label="axis === 'w' ? 'Width' : 'Height'" @update:model-value="(v) => setSize(axis, v as SizeMode)" />
              <label class="ac-affix">
                <span>px</span>
                <input
                  class="ac-field"
                  type="number"
                  min="0"
                  :value="size(axis).mode === 'fixed' ? size(axis).px : Math.round(axis === 'w' ? picked.info.rect.width : picked.info.rect.height)"
                  :aria-label="axis === 'w' ? 'Width in px' : 'Height in px'"
                  @change="setSize(axis, 'fixed', Number(($event.target as HTMLInputElement).value))"
                />
              </label>
            </div>
          </template>
        </div>
      </div>

      <div class="ac-section">
        <p class="ac-heading">Text</p>
        <div class="ac-grid">
          <span class="ac-grid-label">Style</span>
          <Select :model-value="get(G.type)" :options="C.type" label="Text style" @update:model-value="(v) => set(G.type, v)" />
          <span class="ac-grid-label">Color</span>
          <Select :model-value="get(G.textColor)" :options="C.textColor" label="Text color" :min-width="240" @update:model-value="(v) => set(G.textColor, v)" />
          <span class="ac-grid-label">Align</span>
          <div class="ac-seg fill" role="group" aria-label="Text align">
            <button v-for="a in ALIGN" :key="a.label" type="button" :aria-pressed="get(G.textAlign) === a.value || (!a.value && !get(G.textAlign))" :title="a.label" :aria-label="a.label" @click="set(G.textAlign, a.value)">
              <Icon :name="a.icon" :size="14" />
            </button>
          </div>
        </div>
      </div>

      <div class="ac-section">
        <p class="ac-heading">Fill and border</p>
        <div class="ac-grid">
          <span class="ac-grid-label">Fill</span>
          <Select :model-value="get(G.background)" :options="C.background" label="Fill" :min-width="260" @update:model-value="(v) => set(G.background, v)" />
          <span class="ac-grid-label">Border</span>
          <div class="ac-row">
            <button class="ac-switch" type="button" role="switch" :aria-checked="border" aria-label="Border" @click="set(G.border, border ? '' : 'border')" />
            <Select v-if="border" :model-value="get(G.borderColor)" :options="C.borderColor" label="Border color" :min-width="240" @update:model-value="(v) => set(G.borderColor, v)" />
          </div>
          <span class="ac-grid-label">Radius</span>
          <Select :model-value="get(G.radius)" :options="C.radius" label="Radius" @update:model-value="(v) => set(G.radius, v)" />
        </div>
      </div>

      <details class="ac-section ac-more">
        <summary class="ac-heading"><Icon name="chevronRight" :size="16" /> Classes</summary>
        <textarea v-model="classes" class="ac-field mono" rows="3" aria-label="Classes" @keydown.meta.enter="change({ kind: 'class', to: classes })" />
        <div class="ac-row">
          <span class="ac-caption ac-spacer">Every static class, hover: and md: ones too</span>
          <button class="ac-btn" type="button" :disabled="classes === cls" @click="change({ kind: 'class', to: classes })">Apply</button>
        </div>
      </details>
    </template>

    <details class="ac-section ac-more">
      <summary class="ac-heading"><Icon name="chevronRight" :size="16" /> Computed</summary>
      <dl class="ac-props">
        <template v-for="(v, k) in picked.info.style" :key="k">
          <dt>{{ k }}</dt>
          <dd class="ac-mono">{{ v }}</dd>
        </template>
      </dl>
    </details>
  </div>
</template>

<style>
.ac-grid { display: grid; grid-template-columns: 72px minmax(0, 1fr); align-items: center; gap: 8px; }
.ac-grid-label { color: var(--ac-ink-weak); font-size: 12px; line-height: 16px; }
.ac-grid-note { margin-top: -4px; padding-left: 80px; }
.ac-bound { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-switch {
  position: relative;
  flex-shrink: 0;
  width: 32px;
  height: 18px;
  margin: 7px 0;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: var(--ac-line-strong);
  cursor: pointer;
  transition: background-color 160ms var(--ac-ease);
}
.ac-switch::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 999px;
  background: var(--ac-surface);
  box-shadow: var(--ac-e1);
  transition: translate 160ms var(--ac-ease);
}
.ac-switch[aria-checked='true'] { background: var(--ac-violet); }
.ac-switch[aria-checked='true']::after { translate: 14px 0; }
.ac-more > summary { display: flex; align-items: center; gap: 6px; list-style: none; cursor: pointer; }
.ac-more > summary::-webkit-details-marker { display: none; }
.ac-more > summary > svg { color: var(--ac-icon-weak); transition: rotate 160ms var(--ac-ease); }
.ac-more[open] > summary > svg { rotate: 90deg; }
.ac-more:not([open]) { gap: 0; }
</style>
