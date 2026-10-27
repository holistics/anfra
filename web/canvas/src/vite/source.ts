// Where elements are written, and the edits the canvas makes there itself: one element's text,
// classes or attributes, or the element itself inserted, moved, duplicated or removed. The compiler
// tags every element with its place in its file (`src/pages/Home.vue:12:5`), and an edit finds the
// element by that place again. Anything wider (a flow, several screens, logic) is the agent's.
import { relative } from 'node:path';
import { babelParse, parse, type SFCDescriptor } from 'vue/compiler-sfc';
import type { AttributeNode, DirectiveNode, ElementNode, NodeTransform, TemplateChildNode, TextNode } from '@vue/compiler-core';
import { COMPONENT_ATTR, SOURCE_ATTR } from '../frame/protocol.ts';

// The compiler's node and element types are const enums, which have no values at run time.
const NodeTypes = { ELEMENT: 1, TEXT: 2, COMMENT: 3, SIMPLE_EXPRESSION: 4, ATTRIBUTE: 6, DIRECTIVE: 7, IF: 9, FOR: 11 } as const;
const ElementTypes = { COMPONENT: 1, SLOT: 2, TEMPLATE: 3 } as const;

export interface SourceLoc {
  file: string;
  line: number;
  column: number;
}

export function parseLoc (value: string): SourceLoc | undefined {
  const m = /^(.+):(\d+):(\d+)$/.exec(value);
  return m ? { file: m[1], line: Number(m[2]), column: Number(m[3]) } : undefined;
}

// Not tagged: what renders no element of its own, and RouterView, whose attributes would land on
// every page it shows and say where the layout put it rather than where the page is.
const SKIP = new Set([
  'template', 'slot', 'Teleport', 'KeepAlive', 'Transition', 'TransitionGroup', 'Suspense', 'RouterView', 'router-view',
]);

/**
 * The compiler transform that tags elements. A component's tag falls through to its root element,
 * where it replaces the root's own, so an element drawn by a component points at the place the
 * component is used, which is what a person editing a screen means.
 */
export function sourceTransform (root: string): NodeTransform {
  return (node, context) => {
    if (node.type !== NodeTypes.ELEMENT || SKIP.has(node.tag) || node.tagType === ElementTypes.TEMPLATE || node.tagType === ElementTypes.SLOT) return;
    if (!context.filename || context.filename.includes('/node_modules/')) return;
    const file = relative(root, context.filename);
    if (file.startsWith('..')) return;
    const { line, column } = node.loc.start;
    node.props.push(attribute(SOURCE_ATTR, `${file}:${line}:${column}`, node));
    if (node.tagType === ElementTypes.COMPONENT) node.props.push(attribute(COMPONENT_ATTR, node.tag, node));
  };
}

function attribute (name: string, content: string, node: ElementNode): AttributeNode {
  return {
    type: NodeTypes.ATTRIBUTE,
    name,
    nameLoc: node.loc,
    value: { type: NodeTypes.TEXT, content, loc: node.loc },
    loc: node.loc,
  };
}

// --- Finding an element again ---------------------------------------------------------------------

function descriptorOf (code: string, file: string): SFCDescriptor {
  const { descriptor, errors } = parse(code, { filename: file });
  if (errors.length) throw new EditError(`${file} doesn't parse: ${String(errors[0])}`);
  if (!descriptor.template?.ast) throw new EditError(`${file} has no template`);
  return descriptor;
}

interface Found {
  el: ElementNode;
  /** The element's siblings, itself among them, as written: text and comments left out. */
  siblings: TemplateChildNode[];
}

function find (descriptor: SFCDescriptor, line: number, column: number): Found {
  let found: Found | undefined;
  const walk = (nodes: TemplateChildNode[]) => {
    for (const n of nodes) {
      if (found) return;
      if (n.type === NodeTypes.ELEMENT) {
        if (n.loc.start.line === line && n.loc.start.column === column) found = { el: n, siblings: written(nodes) };
        else walk(n.children);
      } else if (n.type === NodeTypes.IF) {
        for (const b of n.branches) walk(b.children);
      } else if (n.type === NodeTypes.FOR) {
        walk(n.children);
      }
    }
  };
  walk(descriptor.template!.ast!.children);
  if (!found) throw new EditError('the element is no longer where it was: the file changed since the screen was drawn');
  return found;
}

const written = (nodes: TemplateChildNode[]) => nodes.filter((c) => !(c.type === NodeTypes.TEXT && !c.content.trim()) && c.type !== NodeTypes.COMMENT);

export class EditError extends Error {}

export type Edit =
  | { kind: 'text', from: string, to: string }
  | { kind: 'class', to: string }
  /** A static attribute or prop: a string value, `true` for a bare boolean, `null` (or false) to remove it. */
  | { kind: 'attr', name: string, value: string | boolean | null }
  | { kind: 'remove' }
  | { kind: 'duplicate' }
  | { kind: 'move', by: -1 | 1 }
  /** New markup beside or inside the element, with the imports it needs in the script. */
  | { kind: 'insert', where: Where, markup: string, imports?: Import[] };

export type Where = 'before' | 'after' | 'inside';
export interface Import { name: string, from: string }

export interface Edited {
  code: string;
  /** Where the element now starts, when it moved or is new: what the canvas picks next. */
  at?: SourceLoc;
}

const escapeText = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/\{\{/g, '{&#123;');
const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
const kebab = (s: string) => s.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase();

/** The file with the edit made, or an EditError saying why the canvas can't make it. */
export function applyEdit (code: string, loc: SourceLoc, edit: Edit): string {
  return editSource(code, loc, edit).code;
}

export function editSource (code: string, loc: SourceLoc, edit: Edit): Edited {
  const descriptor = descriptorOf(code, loc.file);
  const { el, siblings } = find(descriptor, loc.line, loc.column);
  const start = el.loc.start.offset;
  const end = el.loc.end.offset;
  const indent = indentAt(code, start);
  const placed = (c: string, offset: number): Edited => ({ code: c, at: { file: loc.file, ...position(c, offset) } });

  switch (edit.kind) {
    case 'text': {
      const children = written(el.children);
      const text = children[0];
      if (children.length !== 1 || text.type !== NodeTypes.TEXT) {
        throw new EditError(`<${el.tag}> has more than plain text in it (bindings, elements or slots), so its text is changed by the agent`);
      }
      if ((text as TextNode).content.trim() !== edit.from.trim()) {
        throw new EditError(`<${el.tag}>'s text in the file is not what the screen showed`);
      }
      const from = text.loc.start.offset + text.loc.source.indexOf(text.loc.source.trim());
      const to = from + text.loc.source.trim().length;
      return { code: code.slice(0, from) + escapeText(edit.to.trim()) + code.slice(to) };
    }
    case 'class': {
      const value = edit.to.trim().replace(/\s+/g, ' ');
      return { code: setAttribute(code, el, 'class', value || null) };
    }
    case 'attr':
      return { code: setAttribute(code, el, edit.name, edit.value === false ? null : edit.value) };
    case 'remove': {
      const [from, to] = wholeLines(code, start, end);
      return { code: code.slice(0, from) + code.slice(to) };
    }
    case 'duplicate': {
      const copy = `\n${indent}${code.slice(start, end)}`;
      return placed(code.slice(0, end) + copy + code.slice(end), end + 1 + indent.length);
    }
    case 'move': {
      const i = siblings.indexOf(el);
      const other = siblings[i + edit.by];
      if (!other) throw new EditError(`<${el.tag}> is already the ${edit.by < 0 ? 'first' : 'last'} in its parent`);
      const [a, b] = edit.by < 0 ? [other, el] : [el, other];
      const aSrc = code.slice(a.loc.start.offset, a.loc.end.offset);
      const bSrc = code.slice(b.loc.start.offset, b.loc.end.offset);
      const between = code.slice(a.loc.end.offset, b.loc.start.offset);
      const out = code.slice(0, a.loc.start.offset) + bSrc + between + aSrc + code.slice(b.loc.end.offset);
      return placed(out, edit.by < 0 ? a.loc.start.offset : a.loc.start.offset + bSrc.length + between.length);
    }
    case 'insert': {
      const unit = indentUnit(code);
      const lines = (markup: string, pad: string) => markup.trim().split('\n').map((l, n) => (n ? pad + l : l)).join('\n');
      let out: string;
      let at: number;
      if (edit.where === 'before') {
        out = code.slice(0, start) + lines(edit.markup, indent) + `\n${indent}` + code.slice(start);
        at = start;
      } else if (edit.where === 'after') {
        out = code.slice(0, end) + `\n${indent}` + lines(edit.markup, indent) + code.slice(end);
        at = end + 1 + indent.length;
      } else {
        const inner = indent + unit;
        const markup = lines(edit.markup, inner);
        if (el.isSelfClosing) {
          const close = code.lastIndexOf('/>', end);
          out = code.slice(0, close).trimEnd() + `>\n${inner}${markup}\n${indent}</${el.tag}>` + code.slice(end);
          at = code.slice(0, close).trimEnd().length + 2 + inner.length;
        } else {
          const kids = el.children.filter((c) => c.type !== NodeTypes.TEXT || c.content.trim());
          const last = kids.at(-1);
          const closeAt = end - `</${el.tag}>`.length;
          if (last) {
            out = code.slice(0, last.loc.end.offset) + `\n${inner}${markup}` + code.slice(last.loc.end.offset);
            at = last.loc.end.offset + 1 + inner.length;
          } else {
            const open = code.slice(0, closeAt).trimEnd();
            out = open + `\n${inner}${markup}\n${indent}` + code.slice(closeAt);
            at = open.length + 1 + inner.length;
          }
        }
      }
      const withImports = addImports(out, loc.file, edit.imports ?? []);
      return placed(withImports.code, withImports.at <= at ? at + withImports.length : at);
    }
  }
}

/** The attribute set (or removed, for null) on the element's opening tag. A bound one is the agent's. */
function setAttribute (code: string, el: ElementNode, name: string, value: string | true | null): string {
  const same = (n: string) => n === name || kebab(n) === kebab(name);
  const bound = el.props.find((p): p is DirectiveNode => p.type === NodeTypes.DIRECTIVE && p.name === 'bind'
    && p.arg?.type === NodeTypes.SIMPLE_EXPRESSION && same(p.arg.content));
  if (bound && name !== 'class') {
    const exp = bound.exp?.type === NodeTypes.SIMPLE_EXPRESSION ? bound.exp.content : '';
    throw new EditError(`${name} is bound to \`${exp}\` in the file, so the agent changes it`);
  }
  const attr = el.props.find((p): p is AttributeNode => p.type === NodeTypes.ATTRIBUTE && same(p.name));
  const text = value === null ? '' : value === true ? name : `${name}="${escapeAttr(value)}"`;
  if (attr) {
    if (value !== null) return code.slice(0, attr.loc.start.offset) + text + code.slice(attr.loc.end.offset);
    let from = attr.loc.start.offset;
    while (from > 0 && /\s/.test(code[from - 1])) from--;
    return code.slice(0, from) + code.slice(attr.loc.end.offset);
  }
  if (value === null) return code;
  const afterTag = el.loc.start.offset + 1 + el.tag.length;
  return code.slice(0, afterTag) + ` ${text}` + code.slice(afterTag);
}

/** Imports added to the script setup (made if there is none), skipping names already imported. */
function addImports (code: string, file: string, imports: Import[]): { code: string, at: number, length: number } {
  const needed = imports.filter((i, n) => imports.findIndex((j) => j.name === i.name) === n);
  if (!needed.length) return { code, at: Infinity, length: 0 };
  const { descriptor } = parse(code, { filename: file });
  const script = descriptor.scriptSetup;
  if (!script) {
    const block = `<script setup lang="ts">\n${needed.map((i) => `import { ${i.name} } from '${i.from}';`).join('\n')}\n</script>\n\n`;
    return { code: block + code, at: 0, length: block.length };
  }
  const body = script.content;
  const ast = babelParse(body, { sourceType: 'module', plugins: script.lang === 'ts' ? ['typescript'] : [] });
  const decls = ast.program.body.filter((n) => n.type === 'ImportDeclaration');
  const known = new Set(decls.flatMap((d) => d.specifiers.map((s) => s.local.name)));
  const missing = needed.filter((i) => !known.has(i.name));
  if (!missing.length) return { code, at: Infinity, length: 0 };
  const quote = /from\s+"/.test(body) && !/from\s+'/.test(body) ? '"' : '\'';
  const semi = decls.length && !decls.every((d) => body[d.end! - 1] === ';') ? '' : ';';
  const lines = missing.map((i) => `import { ${i.name} } from ${quote}${i.from}${quote}${semi}`).join('\n');
  const last = decls.at(-1);
  const inBody = last ? last.end! : body.match(/^\s*/)![0].length;
  const text = last ? `\n${lines}` : `${lines}\n`;
  const at = script.loc.start.offset + inBody;
  return { code: code.slice(0, at) + text + code.slice(at), at, length: text.length };
}

/** The whitespace before an offset on its line, when only whitespace is before it there. */
function indentAt (code: string, offset: number): string {
  const lineStart = code.lastIndexOf('\n', offset - 1) + 1;
  const before = code.slice(lineStart, offset);
  return /^\s*$/.test(before) ? before : '';
}

/** A range grown to whole lines when nothing else is on them, so removing it leaves no blank line. */
function wholeLines (code: string, start: number, end: number): [number, number] {
  const lineStart = code.lastIndexOf('\n', start - 1) + 1;
  const lineEnd = code.indexOf('\n', end);
  const ownLine = /^\s*$/.test(code.slice(lineStart, start)) && /^\s*$/.test(code.slice(end, lineEnd === -1 ? code.length : lineEnd));
  return ownLine ? [lineStart, lineEnd === -1 ? code.length : lineEnd + 1] : [start, end];
}

function indentUnit (code: string): string {
  const m = /\n([ \t]+)</.exec(code);
  return m?.[1].startsWith('\t') ? '\t' : '  ';
}

function position (code: string, offset: number): { line: number, column: number } {
  const before = code.slice(0, offset);
  const line = before.split('\n').length;
  return { line, column: offset - before.lastIndexOf('\n') };
}

export interface SourceAttr {
  name: string;
  /** The static value; true for a bare boolean attribute. */
  value?: string | true;
  /** The expression, when it is bound. */
  bound?: string;
}

export interface SourceInfo {
  tag: string;
  opening: string;
  classes?: string;
  text?: string;
  attrs: SourceAttr[];
  /** Elements written inside it: something can be dropped in. */
  children: number;
  /** Its place among its siblings, to move it up or down. */
  first: boolean;
  last: boolean;
}

/** The element's source, for the inspector: its opening tag, static classes, text and attributes. */
export function describeSource (code: string, loc: SourceLoc): SourceInfo {
  const { el, siblings } = find(descriptorOf(code, loc.file), loc.line, loc.column);
  const cls = el.props.find((p): p is AttributeNode => p.type === NodeTypes.ATTRIBUTE && p.name === 'class');
  const firstChild = el.children[0]?.loc.start.offset ?? el.loc.end.offset;
  const opening = code.slice(el.loc.start.offset, Math.min(firstChild, el.loc.start.offset + 600)).trim();
  const only = written(el.children);
  const attrs: SourceAttr[] = el.props.flatMap((p): SourceAttr[] => {
    if (p.type === NodeTypes.ATTRIBUTE) return [{ name: p.name, value: p.value ? p.value.content : true }];
    if (p.name === 'bind' && p.arg?.type === NodeTypes.SIMPLE_EXPRESSION) {
      return [{ name: p.arg.content, bound: p.exp?.type === NodeTypes.SIMPLE_EXPRESSION ? p.exp.content : '' }];
    }
    return [];
  });
  return {
    tag: el.tag,
    opening,
    ...(cls?.value ? { classes: cls.value.content } : {}),
    ...(only.length === 1 && only[0].type === NodeTypes.TEXT ? { text: only[0].content.trim() } : {}),
    attrs,
    children: only.filter((c) => c.type === NodeTypes.ELEMENT).length,
    first: siblings[0] === el,
    last: siblings.at(-1) === el,
  };
}
