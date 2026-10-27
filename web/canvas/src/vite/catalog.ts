// What the canvas knows about the app's components and design tokens, read from their source: each
// component's props (with the options of a variant, from its cva call), whether it takes a slot,
// where it is imported from, and the markup it is placed with. The tokens are the theme's colors,
// type roles and radii, which the inspector offers in place of raw classes.
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { babelParse, parse } from 'vue/compiler-sfc';

export interface PropSpec {
  name: string;
  type: 'enum' | 'boolean' | 'string' | 'number';
  options?: string[];
  default?: string | number | boolean;
}

export interface ComponentSpec {
  /** As it is written in a template: `Button`. */
  name: string;
  /** Its folder under the components folder: `button`. */
  folder: string;
  /** The import path for it: `@/components/ui/button`. */
  from: string;
  props: PropSpec[];
  slot: boolean;
  /** The folder's own component (Button in button/), the one the palette offers. */
  main: boolean;
}

export interface PaletteItem {
  /** The folder. */
  id: string;
  name: string;
  markup: string;
  imports: { name: string, from: string }[];
}

/** A theme token: its utility suffix (`weak` for `text-weak`) and its CSS value, for a swatch. */
export interface Token { name: string, value: string }

export interface Tokens {
  textColor: Token[];
  background: Token[];
  borderColor: Token[];
  color: Token[];
  type: Token[];
  radius: Token[];
}

/* eslint-disable @typescript-eslint/no-explicit-any -- the Babel AST, walked loosely */
type Node = { type: string, [key: string]: any };

const SKIP_PROPS = new Set(['class', 'style', 'as', 'asChild']);

export async function readCatalog (dir: string, importBase: string): Promise<ComponentSpec[]> {
  let folders: string[];
  try {
    folders = (await readdir(dir, { withFileTypes: true })).filter((d) => d.isDirectory()).map((d) => d.name).sort();
  } catch {
    return [];
  }
  const specs = await Promise.all(folders.map(async (folder) => {
    const index = await readFile(join(dir, folder, 'index.ts'), 'utf8').catch(() => '');
    const variants = cvaVariants(index);
    const exported = [...index.matchAll(/default\s+as\s+(\w+)\s*\}\s*from\s*['"]\.\/([\w-]+)\.vue['"]/g)].map((m) => ({ name: m[1], file: m[2] }));
    const mainName = exported.find((e) => e.name.toLowerCase() === folder.replace(/-/g, ''))?.name ?? exported[0]?.name;
    return Promise.all(exported.map(async ({ name, file }): Promise<ComponentSpec> => {
      const code = await readFile(join(dir, folder, `${file}.vue`), 'utf8').catch(() => '');
      const { props, slot } = readComponent(code, variants);
      return { name, folder, from: `${importBase}/${folder}`, props, slot, main: name === mainName };
    }));
  }));
  return specs.flat();
}

/** The palette: each folder's main component, placed with the app's snippet for it or a plain one. */
export function palette (catalog: ComponentSpec[], snippets: Record<string, string | false> = {}): PaletteItem[] {
  const byName = new Map(catalog.map((c) => [c.name, c]));
  return catalog.filter((c) => c.main && snippets[c.folder] !== false).map((c) => {
    const markup = (snippets[c.folder] as string | undefined) ?? (c.slot ? `<${c.name}>${words(c.name)}</${c.name}>` : `<${c.name} />`);
    const used = [...new Set([...markup.matchAll(/<([A-Z][\w]*)/g)].map((m) => m[1]))];
    const imports = used.flatMap((n) => (byName.has(n) ? [{ name: n, from: byName.get(n)!.from }] : []));
    return { id: c.folder, name: words(c.name), markup, imports };
  });
}

const words = (name: string) => name.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, (c) => c.toUpperCase());

/** The variants of each cva call in a module: `{ variant: ['default', 'outline', …] }`, with defaults. */
function cvaVariants (code: string): Map<string, { options: string[], default?: string }> {
  const out = new Map<string, { options: string[], default?: string }>();
  if (!code.includes('cva(')) return out;
  let ast: Node;
  try {
    ast = babelParse(code, { sourceType: 'module', plugins: ['typescript'] }) as unknown as Node;
  } catch {
    return out;
  }
  walk(ast, (n) => {
    if (n.type !== 'CallExpression' || n.callee?.name !== 'cva' || n.arguments[1]?.type !== 'ObjectExpression') return;
    const config = n.arguments[1];
    const defaults = objectProp(config, 'defaultVariants');
    for (const v of objectProp(config, 'variants')?.properties ?? []) {
      const key = keyOf(v);
      if (!key || v.value?.type !== 'ObjectExpression' || out.has(key)) continue;
      const d = defaults && objectProp(defaults, key);
      out.set(key, {
        options: v.value.properties.map(keyOf).filter(Boolean),
        ...(d?.type === 'StringLiteral' ? { default: d.value } : {}),
      });
    }
  });
  return out;
}

/** A component's props, from `defineProps<…>()` (or its runtime object), and whether it has a slot. */
function readComponent (code: string, variants: Map<string, { options: string[], default?: string }>): { props: PropSpec[], slot: boolean } {
  if (!code) return { props: [], slot: false };
  const { descriptor } = parse(code);
  const slot = /<slot[\s/>]/.test(descriptor.template?.content ?? '');
  const script = descriptor.scriptSetup;
  if (!script) return { props: [], slot };
  let ast: Node;
  try {
    ast = babelParse(script.content, { sourceType: 'module', plugins: ['typescript'] }) as unknown as Node;
  } catch {
    return { props: [], slot };
  }
  const declared = new Map<string, Node>();
  walk(ast, (n) => {
    if ((n.type === 'TSInterfaceDeclaration' || n.type === 'TSTypeAliasDeclaration') && n.id?.name) declared.set(n.id.name, n);
  });
  let call: Node | undefined;
  let defaults: Node | undefined;
  walk(ast, (n) => {
    if (n.type !== 'CallExpression' || call) return;
    if (n.callee?.name === 'withDefaults' && n.arguments[0]?.callee?.name === 'defineProps') {
      call = n.arguments[0];
      defaults = n.arguments[1];
    } else if (n.callee?.name === 'defineProps') {
      call = n;
    }
  });
  if (!call) return { props: [], slot };

  const members: Node[] = [];
  const collect = (t: Node | undefined) => {
    if (!t) return;
    if (t.type === 'TSTypeLiteral') members.push(...t.members);
    else if (t.type === 'TSIntersectionType') t.types.forEach(collect);
    else if (t.type === 'TSTypeReference' && declared.has(t.typeName?.name)) {
      const d = declared.get(t.typeName.name)!;
      if (d.type === 'TSInterfaceDeclaration') members.push(...d.body.body);
      else collect(d.typeAnnotation);
    }
  };
  const typeArg = (call.typeParameters ?? call.typeArguments)?.params?.[0];
  collect(typeArg);

  const props: PropSpec[] = [];
  for (const m of members) {
    const name = keyOf(m);
    if (!name || SKIP_PROPS.has(name) || m.type !== 'TSPropertySignature') continue;
    const spec = propType(m.typeAnnotation?.typeAnnotation, variants);
    if (!spec) continue;
    const d = defaults ? objectProp(defaults, name) : undefined;
    const value = d && literal(d);
    props.push({ name, ...spec, ...(value !== undefined ? { default: value } : {}) });
  }
  // Runtime props: `defineProps({ size: String, open: { type: Boolean, default: false } })`.
  if (!typeArg && call.arguments[0]?.type === 'ObjectExpression') {
    for (const p of call.arguments[0].properties) {
      const name = keyOf(p);
      if (!name || SKIP_PROPS.has(name)) continue;
      const ctor = p.value?.type === 'Identifier' ? p.value.name : objectProp(p.value, 'type')?.name;
      const type = ctor === 'Boolean' ? 'boolean' : ctor === 'Number' ? 'number' : ctor === 'String' ? 'string' : undefined;
      const d = p.value?.type === 'ObjectExpression' ? objectProp(p.value, 'default') : undefined;
      const value = d && literal(d);
      if (type) props.push({ name, type, ...(value !== undefined ? { default: value } : {}) });
    }
  }
  return { props, slot };
}

function propType (t: Node | undefined, variants: Map<string, { options: string[], default?: string }>): Omit<PropSpec, 'name'> | undefined {
  if (!t) return undefined;
  switch (t.type) {
    case 'TSBooleanKeyword': return { type: 'boolean' };
    case 'TSStringKeyword': return { type: 'string' };
    case 'TSNumberKeyword': return { type: 'number' };
    case 'TSIndexedAccessType': {
      const key = t.indexType?.literal?.value;
      const v = typeof key === 'string' ? variants.get(key) : undefined;
      return v ? { type: 'enum', options: v.options, ...(v.default ? { default: v.default } : {}) } : undefined;
    }
    case 'TSUnionType': {
      const parts = t.types.filter((x: Node) => x.type !== 'TSUndefinedKeyword' && x.type !== 'TSNullKeyword');
      if (parts.length && parts.every((x: Node) => x.type === 'TSLiteralType' && x.literal.type === 'StringLiteral')) {
        return { type: 'enum', options: parts.map((x: Node) => x.literal.value) };
      }
      if (parts.length === 1) return propType(parts[0], variants);
      if (parts.some((x: Node) => x.type === 'TSBooleanKeyword')) return { type: 'boolean' };
      return undefined;
    }
    default: return undefined;
  }
}

function literal (n: Node): string | number | boolean | undefined {
  return n.type === 'StringLiteral' || n.type === 'NumericLiteral' || n.type === 'BooleanLiteral' ? n.value : undefined;
}

const keyOf = (p: Node): string => (p?.key?.type === 'Identifier' ? p.key.name : p?.key?.type === 'StringLiteral' ? p.key.value : '');

const objectProp = (o: Node | undefined, key: string): Node | undefined =>
  o?.type === 'ObjectExpression' ? o.properties.find((p: Node) => keyOf(p) === key)?.value : undefined;

function walk (n: Node, visit: (n: Node) => void) {
  if (!n || typeof n.type !== 'string') return;
  visit(n);
  for (const k of Object.keys(n)) {
    if (k === 'loc' || k === 'start' || k === 'end' || k === 'extra') continue;
    const v = n[k];
    if (Array.isArray(v)) v.forEach((c) => c && typeof c === 'object' && walk(c, visit));
    else if (v && typeof v === 'object' && typeof v.type === 'string') walk(v, visit);
  }
}
/* eslint-enable @typescript-eslint/no-explicit-any */

/**
 * The theme's tokens, from the app's stylesheets: Tailwind's `@theme` variables, each a utility
 * (`--text-color-weak` is `text-weak`, `--text-title` is `text-title`, `--radius-lg` is `rounded-lg`).
 */
export function readTokens (css: string): Tokens {
  const t: Tokens = { textColor: [], background: [], borderColor: [], color: [], type: [], radius: [] };
  const add = (list: Token[], name: string, value: string) => list.some((x) => x.name === name) || list.push({ name, value: value.trim() });
  for (const m of css.matchAll(/(?:^|[\s;{])--([a-z][a-z0-9-]*)\s*:\s*([^;]+);/g)) {
    const [, name, value] = m;
    if (value.trim() === 'initial' || name.includes('--')) continue;
    let k: RegExpExecArray | null;
    if ((k = /^text-color-(.+)$/.exec(name))) add(t.textColor, k[1], value);
    else if ((k = /^background-color-(.+)$/.exec(name))) add(t.background, k[1], value);
    else if ((k = /^border-color-(.+)$/.exec(name))) add(t.borderColor, k[1], value);
    else if ((k = /^color-(.+)$/.exec(name))) add(t.color, k[1], value);
    else if ((k = /^text-(.+)$/.exec(name))) add(t.type, k[1], value);
    else if ((k = /^radius-(.+)$/.exec(name))) add(t.radius, k[1], value);
  }
  return t;
}
