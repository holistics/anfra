/**
 * What `inspect`'s tabs draw with: a small element builder, the icons, the formatting, and the
 * stylesheet, all on the host's `--anfra-*` tokens. Plain DOM, so the tabs carry no framework into
 * a host that has its own.
 */
import type {
  Condition, EntityUsage, InspectedStructure, LocateKind, StructureNode,
} from '../../common/types';

type Child = Node | string | false | null | undefined;
type Children = Child | readonly Children[];
type Attributes = Record<string, string | number | boolean | undefined | ((event: Event) => void)>;

/** An element: `on*` attributes are listeners, `true` sets a bare attribute, false or undefined none. */
export function h (doc: Document, tag: string, attributes: Attributes = {}, ...children: Children[]): HTMLElement {
  const element = doc.createElement(tag);
  for (const [name, value] of Object.entries(attributes)) {
    if (typeof value === 'function') element.addEventListener(name.slice(2).toLowerCase(), value);
    else if (value === true) element.setAttribute(name, '');
    else if (value !== false && value !== undefined) element.setAttribute(name, String(value));
  }
  for (const child of (children as unknown[]).flat(Infinity) as Child[]) {
    if (child === false || child === null || child === undefined) continue;
    element.append(typeof child === 'string' ? doc.createTextNode(child) : child);
  }
  return element;
}

// Hand-drawn 24px stroke icons, as appserve draws them.
const PATHS = {
  close: 'M6 6l12 12 M18 6L6 18',
  copy: 'M9 9h11v11H9z M5 15H4V4h11v1',
  check: 'M5 12l5 5 9-10',
  table: 'M4 5h16v14H4z M4 10h16 M4 15h16 M10 10v9',
  download: 'M12 4v11 M7 10l5 5 5-5 M5 20h14',
  pick: 'M4 4l7 16 2-7 7-2z',
} as const;

export function icon (doc: Document, name: keyof typeof PATHS): SVGElement {
  const svg = doc.createElementNS('http://www.w3.org/2000/svg', 'svg');
  for (const [key, value] of Object.entries({
    class: 'icon', viewBox: '0 0 24 24', width: '14', height: '14', fill: 'none', stroke: 'currentColor',
    'stroke-width': '1.8', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true',
  })) svg.setAttribute(key, value);
  const path = doc.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', PATHS[name]);
  svg.append(path);
  return svg;
}

export function formatCondition (condition: Condition | undefined): string {
  if (!condition) return 'not applied yet';
  const parts = [condition.operator, condition.modifier, ...(condition.values ?? []).map(String)];
  return parts.filter((part) => part !== undefined && part !== '').join(' ');
}

export function formatTime (value: Date | string | undefined): string {
  if (!value) return '';
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleTimeString();
}

/** The attribute a marked element carries for an entity, as the definition writes it. */
export function entityAttribute (kind: LocateKind, app: number, name: string): string {
  return `data-anfra-${kind}="${app === 0 ? '' : `${app}/`}${name}"`;
}

/**
 * The handle an author copies for an agent: a Markdown-style link whose text is the label and
 * whose target is the attribute literal, `[River](data-anfra-block="river")`. With no label, the
 * attribute alone: an agent finds either in the file with one search.
 */
export function handleFor (node: StructureNode): string {
  const attribute = node.kind === 'container' || node.kind === 'block'
    ? `data-anfra-${node.kind}="${node.id ?? ''}"`
    : entityAttribute(node.kind, node.app ?? 0, node.name ?? '');
  return node.label ? `[${node.label}](${attribute})` : attribute;
}

/** How an entity is drawn, for its card: the snapshot's usage entry, or none. */
export function usageOf (structure: InspectedStructure | undefined, kind: LocateKind, app: number, name: string): EntityUsage | undefined {
  return structure?.usage[`${kind}:${app}/${name}`];
}

export function findNode (nodes: StructureNode[], key: string): StructureNode | undefined {
  for (const node of nodes) {
    if (node.key === key) return node;
    const inner = findNode(node.children, key);
    if (inner) return inner;
  }
  return undefined;
}

export function countProblems (nodes: StructureNode[]): number {
  return nodes.reduce((n, node) => n + node.problems.length + countProblems(node.children), 0);
}

/** Copy text from the host page; whether it worked. */
export async function copyText (doc: Document, text: string): Promise<boolean> {
  try {
    await doc.defaultView?.navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** The tabs' stylesheet, on the host's tokens. */
export const STYLES = `
:host { display: block; height: 100%; color: var(--anfra-color-fg); background: var(--anfra-color-bg); font: 13px/1.45 var(--anfra-font); }
* { box-sizing: border-box; }
button { font: inherit; cursor: pointer; }
.pane { height: 100%; display: flex; flex-direction: column; min-height: 0;
  --hover: color-mix(in srgb, var(--anfra-color-fg) 7%, transparent);
  --accent-bg: color-mix(in srgb, var(--anfra-color-accent) 14%, transparent);
  --danger-bg: color-mix(in srgb, var(--anfra-color-danger) 14%, transparent);
  --ok: #16a34a; }
.body { flex: 1; min-height: 0; overflow: auto; padding: 4px 14px 16px; }
.note { color: var(--anfra-color-muted); margin: 12px 0; }
code, pre { font-family: var(--anfra-font-mono); }
h4 { margin: 16px 0 0; font-size: 14px; }
h5 { margin: 16px 0 6px; font-size: 11px; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; color: var(--anfra-color-muted); }
dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 4px 12px; margin: 0; }
dt { color: var(--anfra-color-muted); }
dd { margin: 0; overflow-wrap: anywhere; }
.card { margin-bottom: 8px; padding: 10px 12px; border: 1px solid var(--anfra-color-border); border-radius: var(--anfra-radius); }
.card header { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.card.flash { animation: flash 1.5s ease-out; }
@keyframes flash { from { border-color: var(--anfra-color-accent); box-shadow: 0 0 0 3px var(--accent-bg); } }
.pill { padding: 1px 8px; border-radius: 10px; background: var(--hover); color: var(--anfra-color-muted); font-size: 11px; font-weight: 600; }
.pill.warn { background: var(--accent-bg); color: var(--anfra-color-accent); }
.pill.success { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok); }
.pill.error { background: var(--danger-bg); color: var(--anfra-color-danger); }
.signature { margin-top: 8px; color: var(--anfra-color-muted); }
.signature summary { cursor: pointer; }
.signature code { display: block; margin-top: 4px; }
code.wrap { overflow-wrap: anywhere; }
.error-text { margin: 8px 0 0; color: var(--anfra-color-danger); }
.code { margin-top: 8px; border: 1px solid var(--anfra-color-border); border-radius: 6px; overflow: hidden; }
.code-head { display: flex; align-items: center; justify-content: space-between; padding: 4px 8px; background: var(--hover); color: var(--anfra-color-muted); font-size: 11px; }
.code pre { margin: 0; max-height: 220px; overflow: auto; padding: 8px; font-size: 12px; line-height: 1.5; }
.link { border: 0; border-radius: 4px; background: none; padding: 2px 6px; color: var(--anfra-color-accent); font-size: 11px; }
.link:hover:not(:disabled) { background: var(--hover); }
.link:disabled { color: var(--anfra-color-muted); cursor: default; }
.locate { margin-left: auto; }
.toolbar { flex: none; display: flex; gap: 6px; padding: 8px 14px; border-bottom: 1px solid var(--anfra-color-border); }
.tool { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--anfra-color-border); border-radius: 6px; background: none; padding: 3px 10px; color: var(--anfra-color-fg); font-size: 12px; }
.tool:hover:not(:disabled) { background: var(--hover); }
.tool[aria-pressed="true"] { border-color: var(--anfra-color-accent); color: var(--anfra-color-accent); background: var(--accent-bg); }
.tree { list-style: none; margin: 0; padding: 0; }
.node { display: flex; align-items: center; gap: 6px; padding: 3px 8px; border-radius: 4px; font-size: 12px; cursor: pointer; white-space: nowrap; overflow: hidden; }
.node:hover { background: var(--hover); }
.node.selected { background: var(--accent-bg); }
.name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
.label, .app { color: var(--anfra-color-muted); overflow: hidden; text-overflow: ellipsis; }
.node.problem .name { color: var(--anfra-color-danger); }
.kind { flex: none; padding: 0 6px; border-radius: 4px; font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
.kind.container { background: color-mix(in srgb, #7c3aed 18%, transparent); color: #7c3aed; }
.kind.block { background: color-mix(in srgb, #2563eb 18%, transparent); color: #2563eb; }
.kind.query { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok); }
.kind.control { background: color-mix(in srgb, #d97706 18%, transparent); color: #d97706; }
.node .pill { flex: none; }
/* The actions keep their place and sit in the row's height, so showing them moves nothing. */
.actions { flex: none; display: flex; gap: 2px; margin: -4px 0 -4px auto; visibility: hidden; }
.node:hover .actions, .node.selected .actions, .node:focus-within .actions { visibility: visible; }
.row-action { display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--anfra-color-muted); }
.row-action:hover { background: var(--hover); color: var(--anfra-color-accent); }
.detail { flex: none; max-height: 45%; overflow: auto; padding: 8px 14px 16px; border-top: 1px solid var(--anfra-color-border); }
.detail h5 { margin-top: 4px; }
.detail .card { margin-top: 16px; }
.detail-actions { display: flex; gap: 6px; margin-top: 8px; }
.status { margin: 8px 0 0; color: var(--anfra-color-muted); }
`;
