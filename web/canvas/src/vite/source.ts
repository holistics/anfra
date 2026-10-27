// Where elements are written, and the small edits the canvas makes there itself: an element's text,
// or its static classes. The compiler tags every element with its place in its file
// (`src/pages/Home.vue:12:5`), and an edit finds the element by that place again. Anything more
// than replacing one static text or one class list is the agent's to do.
import { relative } from 'node:path';
import { parse, type SFCDescriptor } from 'vue/compiler-sfc';
import type { AttributeNode, ElementNode, NodeTransform, TemplateChildNode, TextNode } from '@vue/compiler-core';
import { COMPONENT_ATTR, SOURCE_ATTR } from '../frame/protocol.ts';

// The compiler's node and element types are const enums, which have no values at run time.
const NodeTypes = { ELEMENT: 1, TEXT: 2, COMMENT: 3, ATTRIBUTE: 6, IF: 9, FOR: 11 } as const;
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

function elementAt (descriptor: SFCDescriptor, line: number, column: number): ElementNode {
  let found: ElementNode | undefined;
  const walk = (nodes: TemplateChildNode[]) => {
    for (const n of nodes) {
      if (found) return;
      if (n.type === NodeTypes.ELEMENT) {
        if (n.loc.start.line === line && n.loc.start.column === column) found = n;
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

export class EditError extends Error {}

export type Edit =
  | { kind: 'text', from: string, to: string }
  | { kind: 'class', to: string };

const escapeText = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/\{\{/g, '{&#123;');
const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');

/** The file with the edit made, or an EditError saying why the canvas can't make it. */
export function applyEdit (code: string, loc: SourceLoc, edit: Edit): string {
  const el = elementAt(descriptorOf(code, loc.file), loc.line, loc.column);
  if (edit.kind === 'text') {
    const children = el.children.filter((c) => !(c.type === NodeTypes.TEXT && !c.content.trim()) && c.type !== NodeTypes.COMMENT);
    const text = children[0];
    if (children.length !== 1 || text.type !== NodeTypes.TEXT) {
      throw new EditError(`<${el.tag}> has more than plain text in it (bindings, elements or slots), so its text is changed by the agent`);
    }
    if ((text as TextNode).content.trim() !== edit.from.trim()) {
      throw new EditError(`<${el.tag}>'s text in the file is not what the screen showed`);
    }
    const start = text.loc.start.offset + text.loc.source.indexOf(text.loc.source.trim());
    const end = start + text.loc.source.trim().length;
    return code.slice(0, start) + escapeText(edit.to.trim()) + code.slice(end);
  }
  const cls = el.props.find((p): p is AttributeNode => p.type === NodeTypes.ATTRIBUTE && p.name === 'class');
  const value = escapeAttr(edit.to.trim().replace(/\s+/g, ' '));
  if (cls?.value) return code.slice(0, cls.value.loc.start.offset) + `"${value}"` + code.slice(cls.value.loc.end.offset);
  if (cls) return code.slice(0, cls.loc.end.offset) + `="${value}"` + code.slice(cls.loc.end.offset);
  const afterTag = el.loc.start.offset + 1 + el.tag.length;
  return code.slice(0, afterTag) + ` class="${value}"` + code.slice(afterTag);
}

/** The element's source, for the inspector: its opening tag and static classes. */
export function describeSource (code: string, loc: SourceLoc): { tag: string, opening: string, classes?: string, text?: string } {
  const el = elementAt(descriptorOf(code, loc.file), loc.line, loc.column);
  const cls = el.props.find((p): p is AttributeNode => p.type === NodeTypes.ATTRIBUTE && p.name === 'class');
  const firstChild = el.children[0]?.loc.start.offset ?? el.loc.end.offset;
  const opening = code.slice(el.loc.start.offset, Math.min(firstChild, el.loc.start.offset + 600)).trim();
  const only = el.children.filter((c) => !(c.type === NodeTypes.TEXT && !c.content.trim()));
  return {
    tag: el.tag,
    opening,
    ...(cls?.value ? { classes: cls.value.content } : {}),
    ...(only.length === 1 && only[0].type === NodeTypes.TEXT ? { text: only[0].content.trim() } : {}),
  };
}
