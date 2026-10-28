<script setup lang="ts">
// The picked element, edited as a designer edits a layer: its component's props, its text, and
// its layout, spacing, size, type and fill. Every control writes the element's source (a prop is
// an attribute, a style is a Tailwind class from the theme's tokens) and the screen redraws from
// the file. Moving, duplicating and removing it write the file too; each is one undo step.
import { computed, ref, watch } from 'vue';
import { elementMarkdown } from '../../model/index.ts';
import type { PropSpec } from '../../vite/catalog.ts';
import type { Edit, SourceInfo } from '../../vite/source.ts';
import { choices, groups, overrides, read, sizeClass, sizeOf, write, type Group, type SizeMode } from '../classes.ts';
import { useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';
import Icon from '../ui/Icon.vue';
import IconPicker from './IconPicker.vue';
import PropRow from '../ui/PropRow.vue';
import Section from '../ui/Section.vue';
import Segmented from '../ui/Segmented.vue';
import Select, { type SelectOption } from '../ui/Select.vue';
import Switch from '../ui/Switch.vue';
import { vTip } from '../ui/tooltip.ts';

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
/**
 * A design system's component is an instance: its look is its props, and the panel offers only how
 * it is placed in its parent. Classes on its tag that restyle its insides are flagged, to remove.
 */
const instance = computed(() => (component.value?.kit ? component.value : undefined));
const placement = computed(() => new Set(instance.value?.placement ?? []));
const overriding = computed(() => (instance.value ? overrides(cls.value, G) : []));
function removeOverrides () {
  const drop = new Set(overriding.value);
  void change({ kind: 'class', to: cls.value.split(/\s+/).filter((c) => c && !drop.has(c)).join(' ') });
}
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
const stringValue = (p: PropSpec) => {
  const v = attr(p.name)?.value;
  return typeof v === 'string' ? v : '';
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

const DISPLAYS = [
  { value: 'block', label: 'Block', title: 'Stacked as written' },
  { value: 'row', label: 'Row', title: 'Auto layout, side by side' },
  { value: 'column', label: 'Column', title: 'Auto layout, one under another' },
  { value: 'grid', label: 'Grid' },
] as const;

const ALIGN = [
  { value: '', icon: 'textLeft', label: 'Left', iconOnly: true },
  { value: 'text-center', icon: 'textCenter', label: 'Center', iconOnly: true },
  { value: 'text-right', icon: 'textRight', label: 'Right', iconOnly: true },
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
    <Section :title="title" :icon="ref_.component ? 'component' : 'box'">
      <template #actions>
        <Button variant="ghost" icon="parent" label="Select the parent" keys="⇧↵" @click="store.pickParent()" />
        <template v-if="editable">
          <Button variant="ghost" icon="arrowUp" label="Move up" keys="⌥↑" :disabled="src?.first" @click="change({ kind: 'move', by: -1 })" />
          <Button variant="ghost" icon="arrowDown" label="Move down" keys="⌥↓" :disabled="src?.last" @click="change({ kind: 'move', by: 1 })" />
          <Button variant="ghost" icon="duplicate" label="Duplicate" keys="⌘D" @click="change({ kind: 'duplicate' }, 'Duplicated')" />
          <Button variant="ghost" icon="trash" label="Delete" keys="⌫" @click="change({ kind: 'remove' }, `${title} removed`)" />
        </template>
      </template>
      <button v-if="ref_.source" class="ac-link ac-source" type="button" v-tip="'Open in your editor'" @click="openInEditor(ref_.source)">
        <span class="ac-link-text ac-mono ac-truncate">{{ where(ref_.source).file }}</span>
        <Icon name="external" :size="14" />
        <span class="ac-source-dir ac-caption ac-truncate"><bdi>{{ where(ref_.source).dir }}</bdi></span>
      </button>
      <p v-else class="ac-caption">Drawn at run time, not written in a template: only a comment can change it</p>
      <div class="ac-row">
        <Button icon="comment" @click="commentOn">Comment</Button>
        <Button variant="ghost" icon="copy" label="Copy as Markdown for Claude" @click="board && store.copy(elementMarkdown(board, ref_), 'Element copied as Markdown')" />
      </div>
    </Section>

    <template v-if="editable">
      <Section v-if="props.length || src?.text !== undefined" :title="component ? 'Properties' : 'Content'">
        <div class="ac-props">
          <PropRow v-if="src?.text !== undefined" label="Text">
            <Field v-model="text" aria-label="Text" @keydown.enter="saveText" @blur="saveText" />
          </PropRow>
          <PropRow v-for="p in props" :key="p.name" :label="p.name">
            <p v-if="attr(p.name)?.bound !== undefined" v-tip="attr(p.name)!.bound" class="ac-caption ac-truncate">Bound to <code class="ac-mono">{{ attr(p.name)!.bound }}</code></p>
            <Select v-else-if="p.type === 'enum'" :model-value="enumValue(p)" :options="enumOptions(p)" :label="p.name" @update:model-value="(v) => setProp(p, v)" />
            <Switch v-else-if="p.type === 'boolean'" :model-value="boolValue(p)" :label="p.name" @update:model-value="(v) => setProp(p, v || null)" />
            <IconPicker v-else-if="p.type === 'icon'" :model-value="stringValue(p)" :label="p.name" @update:model-value="(v) => setProp(p, v)" />
            <Field
              v-else
              :type="p.type === 'number' ? 'number' : 'text'"
              :value="stringValue(p)"
              :placeholder="p.default !== undefined ? String(p.default) : ''"
              :aria-label="p.name"
              @change="setProp(p, ($event.target as HTMLInputElement).value)"
            />
          </PropRow>
          <PropRow v-if="instance?.color === 'icon' && C.iconColor.length > 1" label="color" tip="An icon token: a class on its tag">
            <Select :model-value="get(G.iconColor)" :options="C.iconColor" label="Icon color" :min-width="240" @update:model-value="(v) => set(G.iconColor, v)" />
          </PropRow>
        </div>
      </Section>

      <Section v-if="instance" title="Placement">
        <template v-if="instance.classTarget === 'none'">
          <p class="ac-caption">{{ title }} takes no classes. To size or place it, put it in a div of its own.</p>
          <div class="ac-row">
            <Button icon="box" @click="change({ kind: 'wrap' }, 'Put in a div')">Put in a div</Button>
          </div>
        </template>
        <p v-else-if="!placement.size" class="ac-caption">{{ title }} opens from what it wraps: size and place that instead.</p>
        <div v-else class="ac-props">
          <PropRow v-if="placement.has('width')" label="Width">
            <div class="ac-pair">
              <Select :model-value="size('w').mode" :options="SIZE_MODES" label="Width" @update:model-value="(v) => setSize('w', v as SizeMode)" />
              <Field
                prefix="px"
                type="number"
                min="0"
                :value="size('w').mode === 'fixed' ? size('w').px : Math.round(picked.info.rect.width)"
                aria-label="Width in px"
                @change="setSize('w', 'fixed', Number(($event.target as HTMLInputElement).value))"
              />
            </div>
          </PropRow>
          <PropRow v-if="placement.has('grow')" label="In a row" tip="Its share of a row or column it is in">
            <Select :model-value="get(G.grow)" :options="C.grow" label="In a row" :min-width="240" @update:model-value="(v) => set(G.grow, v)" />
          </PropRow>
          <PropRow v-if="placement.has('align')" label="Align" tip="Its own alignment in its parent">
            <Select :model-value="get(G.self)" :options="C.self" label="Align itself" @update:model-value="(v) => set(G.self, v)" />
          </PropRow>
          <PropRow v-if="placement.has('margin')" label="Above" tip="Space above it, in px">
            <Select :model-value="get(G.marginTop)" :options="C.marginTop" label="Space above" @update:model-value="(v) => set(G.marginTop, v)" />
          </PropRow>
        </div>
        <p class="ac-caption">
          Its look is its properties. Spacing between it and what is beside it is its parent's:
          <button type="button" class="ac-link" @click="store.pickParent()">select the parent</button>
        </p>
        <template v-if="overriding.length">
          <p class="ac-callout warn ac-caption" role="note">
            {{ overriding.length }} {{ overriding.length === 1 ? 'class restyles' : 'classes restyle' }} {{ title }} over the design system:
            <code class="ac-mono">{{ overriding.join(' ') }}</code>
          </p>
          <div class="ac-row">
            <Button @click="removeOverrides">Remove {{ overriding.length === 1 ? 'it' : 'them' }}</Button>
          </div>
        </template>
      </Section>

      <template v-else>
      <Section title="Layout">
        <Segmented :model-value="display" :options="DISPLAYS" label="Layout" fill @update:model-value="(v) => setDisplay(v as Display)" />
        <p v-if="display === 'block' && computedDisplay && computedDisplay !== 'block'" class="ac-caption">Drawn as {{ computedDisplay }} by its component</p>
        <div class="ac-props">
          <template v-if="display !== 'block'">
            <PropRow label="Gap">
              <Select :model-value="get(G.gap)" :options="C.gap" label="Gap" @update:model-value="(v) => set(G.gap, v)" />
            </PropRow>
            <PropRow label="Align">
              <Select :model-value="get(G.items)" :options="C.items" label="Align items" @update:model-value="(v) => set(G.items, v)" />
            </PropRow>
            <PropRow label="Justify">
              <Select :model-value="get(G.justify)" :options="C.justify" label="Justify" @update:model-value="(v) => set(G.justify, v)" />
            </PropRow>
          </template>
          <PropRow label="Padding x" tip="Padding left and right, in px">
            <Select :model-value="padding('px')" :options="C.px" label="Padding left and right" @update:model-value="(v) => setPadding('px', v)" />
          </PropRow>
          <PropRow label="Padding y" tip="Padding top and bottom, in px">
            <Select :model-value="padding('py')" :options="C.py" label="Padding top and bottom" @update:model-value="(v) => setPadding('py', v)" />
          </PropRow>
        </div>
      </Section>

      <Section title="Size">
        <div class="ac-props">
          <PropRow v-for="axis in (['w', 'h'] as const)" :key="axis" :label="axis === 'w' ? 'Width' : 'Height'">
            <div class="ac-pair">
              <Select :model-value="size(axis).mode" :options="SIZE_MODES" :label="axis === 'w' ? 'Width' : 'Height'" @update:model-value="(v) => setSize(axis, v as SizeMode)" />
              <Field
                prefix="px"
                type="number"
                min="0"
                :value="size(axis).mode === 'fixed' ? size(axis).px : Math.round(axis === 'w' ? picked.info.rect.width : picked.info.rect.height)"
                :aria-label="axis === 'w' ? 'Width in px' : 'Height in px'"
                @change="setSize(axis, 'fixed', Number(($event.target as HTMLInputElement).value))"
              />
            </div>
          </PropRow>
        </div>
      </Section>

      <Section title="Text">
        <div class="ac-props">
          <PropRow label="Style">
            <Select :model-value="get(G.type)" :options="C.type" label="Text style" @update:model-value="(v) => set(G.type, v)" />
          </PropRow>
          <PropRow label="Color">
            <Select :model-value="get(G.textColor)" :options="C.textColor" label="Text color" :min-width="240" @update:model-value="(v) => set(G.textColor, v)" />
          </PropRow>
          <PropRow label="Align">
            <Segmented :model-value="get(G.textAlign)" :options="ALIGN" label="Text align" fill @update:model-value="(v) => set(G.textAlign, String(v))" />
          </PropRow>
        </div>
      </Section>

      <Section title="Fill and border">
        <div class="ac-props">
          <PropRow label="Fill">
            <Select :model-value="get(G.background)" :options="C.background" label="Fill" :min-width="260" @update:model-value="(v) => set(G.background, v)" />
          </PropRow>
          <PropRow label="Border">
            <div class="ac-row">
              <Switch :model-value="border" label="Border" @update:model-value="(on) => set(G.border, on ? 'border' : '')" />
              <Select v-if="border" :model-value="get(G.borderColor)" :options="C.borderColor" label="Border color" :min-width="240" @update:model-value="(v) => set(G.borderColor, v)" />
            </div>
          </PropRow>
          <PropRow label="Radius">
            <Select :model-value="get(G.radius)" :options="C.radius" label="Radius" @update:model-value="(v) => set(G.radius, v)" />
          </PropRow>
        </div>
      </Section>

      </template>

      <Section :title="instance ? 'Raw classes' : 'Classes'" collapsible>
        <Field v-model="classes" :rows="3" mono aria-label="Classes" @keydown.meta.enter="change({ kind: 'class', to: classes })" />
        <div class="ac-row">
          <span class="ac-caption ac-spacer">{{ instance ? 'On a design system component, classes beyond placement restyle it: leave its look to its properties' : 'Every static class, hover: and md: ones too' }}</span>
          <Button :disabled="classes === cls" @click="change({ kind: 'class', to: classes })">Apply</Button>
        </div>
      </Section>
    </template>

    <Section title="Computed" collapsible>
      <dl class="ac-dl values">
        <div v-for="(v, k) in picked.info.style" :key="k">
          <dt>{{ k }}</dt>
          <dd>{{ v }}</dd>
        </div>
      </dl>
    </Section>
  </div>
</template>
