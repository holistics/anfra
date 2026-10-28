// The inspector's design controls, over an element's Tailwind classes: each control owns a group
// of utilities (gap-*, px-*, text-<role>, bg-<token>, …), reads the one the element has and
// replaces it. Classes with a variant (hover:, md:) are left alone; the classes field shows them.
import type { Tokens } from '../vite/catalog.ts';
import type { SelectOption } from './ui/Select.vue';

export interface Group {
  /** Whether a class (with no variant) is one of this group's. */
  test: (c: string) => boolean;
}

const base = (c: string) => !c.includes(':');
const split = (classes: string) => classes.split(/\s+/).filter(Boolean);

/** The group's class the element has, if any. */
export function read (classes: string, group: Group): string | undefined {
  return split(classes).find((c) => base(c) && group.test(c));
}

/** The classes with the group's class replaced by `value` (or removed, for ''). */
export function write (classes: string, group: Group, value: string): string {
  const list = split(classes);
  const at = list.findIndex((c) => base(c) && group.test(c));
  const rest = list.filter((c) => !(base(c) && group.test(c)));
  if (value) rest.splice(at < 0 ? rest.length : Math.min(at, rest.length), 0, value);
  return rest.join(' ');
}

const re = (pattern: string) => {
  const r = new RegExp(`^(?:${pattern})$`);
  return { test: (c: string) => r.test(c) };
};
const names = (list: { name: string }[]) => list.map((t) => t.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|') || '(?!)';

/** Tailwind's spacing steps, as the app uses them, labelled in pixels. */
const STEPS = ['0', '0.5', '1', '1.5', '2', '2.5', '3', '4', '5', '6', '8', '10', '12', '16'];
const steps = (prefix: string): SelectOption[] => [
  { value: '', label: 'None' },
  ...STEPS.map((s) => ({ value: `${prefix}-${s}`, label: `${Number(s) * 4}`, hint: s })),
];

export function groups (tokens: Tokens) {
  return {
    display: re('block|inline-block|inline|flex|inline-flex|grid|inline-grid|hidden|contents'),
    direction: re('flex-(row|col)(-reverse)?'),
    wrap: re('flex-(wrap|nowrap|wrap-reverse)'),
    gap: re('gap-[\\w.[\\]-]+'),
    items: re('items-(start|end|center|baseline|stretch)'),
    justify: re('justify-(start|end|center|between|around|evenly|stretch)'),
    padding: re('p-[\\w.[\\]-]+'),
    px: re('px-[\\w.[\\]-]+'),
    py: re('py-[\\w.[\\]-]+'),
    width: re('w-[\\w./[\\]-]+'),
    height: re('h-[\\w./[\\]-]+'),
    type: re(`text-(${names(tokens.type)})`),
    textColor: re(`text-(${names([...tokens.textColor, ...tokens.color])})`),
    textAlign: re('text-(left|center|right|justify|start|end)'),
    iconColor: re(`icon-(${names(tokens.iconColor)})`),
    background: re(`bg-(${names([...tokens.background, ...tokens.color])}|transparent|white|black)`),
    border: re('border|border-[0-9]+'),
    borderColor: re(`border-(${names([...tokens.borderColor, ...tokens.color])})`),
    radius: re(`rounded(-(none|full|${names(tokens.radius)}))?`),
    // How an element sits in its parent: its share of a row, its own alignment, space above it.
    grow: re('flex-1|flex-auto|flex-none|grow|grow-0|shrink-0'),
    self: re('self-(auto|start|end|center|stretch|baseline)'),
    marginTop: re('mt-[\\w.[\\]-]+'),
  };
}

/** What a design system's component draws itself: classes on its tag that change these fight it. */
const INSIDES = ['display', 'direction', 'wrap', 'gap', 'items', 'justify', 'padding', 'px', 'py', 'type', 'textColor', 'textAlign', 'background', 'border', 'borderColor', 'radius'] as const;

/** Any fill, type, spacing inside, border, radius or shadow, theme token or not (`bg-red-500`). */
const RESTYLES = /^-?(bg|text|font|leading|tracking|rounded|shadow|border|ring|p[xytrbl]?|gap|space-[xy]|items|justify|content|place-[a-z]+|grid-cols|grid-rows|flex-(row|col|wrap))(-|$)/;

/** The classes on a component's tag that restyle its insides, rather than place it. */
export function overrides (classes: string, g: Groups): string[] {
  return split(classes).filter((c) => base(c) && !g.grow.test(c) && (INSIDES.some((k) => g[k].test(c)) || RESTYLES.test(c)));
}

export type Groups = ReturnType<typeof groups>;

/** The choices each select control offers, from the theme's tokens. */
export function choices (tokens: Tokens) {
  const color = (prefix: string, list: { name: string, value: string }[]): SelectOption[] => [
    { value: '', label: 'None' },
    ...list.map((t) => ({ value: `${prefix}-${t.name}`, label: t.name, swatch: t.value })),
  ];
  return {
    gap: steps('gap'),
    px: steps('px'),
    py: steps('py'),
    items: [
      { value: '', label: 'Default' },
      ...['start', 'center', 'end', 'stretch', 'baseline'].map((v) => ({ value: `items-${v}`, label: v })),
    ],
    justify: [
      { value: '', label: 'Default' },
      ...['start', 'center', 'end', 'between'].map((v) => ({ value: `justify-${v}`, label: v === 'between' ? 'space between' : v })),
    ],
    type: [{ value: '', label: 'Inherit' }, ...tokens.type.map((t) => ({ value: `text-${t.name}`, label: t.name, hint: px(t.value) }))],
    textColor: color('text', tokens.textColor.length ? tokens.textColor : tokens.color),
    background: color('bg', tokens.background.length ? tokens.background : tokens.color),
    borderColor: color('border', tokens.borderColor.length ? tokens.borderColor : tokens.color),
    iconColor: [
      { value: '', label: 'Inherit', description: 'The color of the text around it' },
      ...tokens.iconColor.map((t) => ({ value: `icon-${t.name}`, label: t.name, swatch: t.value })),
    ],
    grow: [
      { value: '', label: 'Its own size' },
      { value: 'flex-1', label: 'Fill the rest', description: 'Takes the room left in its row or column' },
      { value: 'shrink-0', label: 'Never shrink', description: 'Keeps its size when the row is tight' },
    ],
    self: [
      { value: '', label: 'As its parent says' },
      ...['start', 'center', 'end', 'stretch'].map((v) => ({ value: `self-${v}`, label: v })),
    ],
    marginTop: steps('mt'),
    radius: [
      { value: '', label: 'None' },
      ...tokens.radius.map((t) => ({ value: `rounded-${t.name}`, label: t.name, hint: px(t.value) })),
      { value: 'rounded-full', label: 'full' },
    ],
  };
}

/** A CSS length as px, for a hint: `1.25rem` is `20px`. */
function px (value: string): string {
  const v = value.replace(/\s*\/\*.*$/, '').trim();
  const rem = /^([\d.]+)rem$/.exec(v);
  return rem ? `${Number(rem[1]) * 16}px` : v;
}

/** A size control's mode: from the width or height class. */
export type SizeMode = 'auto' | 'fill' | 'hug' | 'fixed';

export function sizeOf (cls: string | undefined, axis: 'w' | 'h'): { mode: SizeMode, px: number } {
  if (!cls) return { mode: 'auto', px: 0 };
  if (cls === `${axis}-full`) return { mode: 'fill', px: 0 };
  if (cls === `${axis}-fit`) return { mode: 'hug', px: 0 };
  const arbitrary = new RegExp(`^${axis}-\\[(\\d+(?:\\.\\d+)?)px\\]$`).exec(cls);
  if (arbitrary) return { mode: 'fixed', px: Number(arbitrary[1]) };
  const step = new RegExp(`^${axis}-(\\d+(?:\\.\\d+)?)$`).exec(cls);
  if (step) return { mode: 'fixed', px: Number(step[1]) * 4 };
  return { mode: 'fixed', px: 0 };
}

export function sizeClass (axis: 'w' | 'h', mode: SizeMode, px: number): string {
  if (mode === 'fill') return `${axis}-full`;
  if (mode === 'hug') return `${axis}-fit`;
  if (mode === 'fixed') return Number.isInteger(px / 4) ? `${axis}-${px / 4}` : `${axis}-[${px}px]`;
  return '';
}
