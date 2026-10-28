// The home page's reading of the boards: the project each is in, what a search finds, and the boards
// opened last. Pure, beside the recents kept in this browser.
import type { BoardSummary } from '../vite/boards.ts';
import type { HomeView, Settings } from './store.ts';

/** A screen's project: the folder it is in under the prototypes' route (`/p/datasets/List` is in `datasets`). */
export function screenProject (route: string, prefix: string): string | undefined {
  const path = route.split(/[?#]/)[0];
  if (!prefix || !path.startsWith(`${prefix}/`)) return undefined;
  const parts = path.slice(prefix.length + 1).split('/');
  return parts.length > 1 ? parts[0] : undefined;
}

/** A board's project: its own, else the folder most of its screens are in. */
export function boardProject (b: BoardSummary, prefix: string): string | undefined {
  if (b.project) return b.project;
  const counts = new Map<string, number>();
  for (const r of b.routes ?? []) {
    const p = screenProject(r, prefix);
    if (p) counts.set(p, (counts.get(p) ?? 0) + 1);
  }
  return [...counts].sort((a, z) => z[1] - a[1] || a[0].localeCompare(z[0]))[0]?.[0];
}

export function projects (boards: BoardSummary[], settings: Settings): { name: string, boards: number, screens: number }[] {
  const prefix = settings.prototypeRoute ?? '';
  const names = new Set<string>();
  for (const b of boards) {
    const p = boardProject(b, prefix);
    if (p) names.add(p);
  }
  for (const r of settings.prototypes) {
    const p = screenProject(r, prefix);
    if (p) names.add(p);
  }
  return [...names].sort((a, z) => a.localeCompare(z)).map((name) => ({
    name,
    boards: boards.filter((b) => boardProject(b, prefix) === name).length,
    screens: settings.prototypes.filter((r) => screenProject(r, prefix) === name).length,
  }));
}

/** The boards a view lists, found by a search when there is one, newest or by name. */
export function listed (boards: BoardSummary[], settings: Settings, view: HomeView, query: string, order: 'updated' | 'name', recent: string[]): BoardSummary[] {
  const prefix = settings.prototypeRoute ?? '';
  const q = query.trim().toLowerCase();
  let out = boards;
  if (q) {
    out = boards.filter((b) => [
      b.name, b.slug, boardProject(b, prefix) ?? '', ...(b.screens ?? []), ...(b.routes ?? []),
    ].some((t) => t.toLowerCase().includes(q)));
  } else if (view.kind === 'project') out = boards.filter((b) => boardProject(b, prefix) === view.id);
  else if (view.kind === 'recent') {
    return recent.map((slug) => boards.find((b) => b.slug === slug)).filter((b): b is BoardSummary => !!b);
  }
  return [...out].sort(order === 'name' ? (a, z) => a.name.localeCompare(z.name) : (a, z) => (z.updated ?? 0) - (a.updated ?? 0));
}

/** The prototypes a view lists: a project's, or those a search finds. */
export function screens (settings: Settings, view: HomeView, query: string): string[] {
  const q = query.trim().toLowerCase();
  if (q) return settings.prototypes.filter((r) => r.toLowerCase().includes(q));
  if (view.kind === 'project') return settings.prototypes.filter((r) => screenProject(r, settings.prototypeRoute ?? '') === view.id);
  return [];
}

/** "2 hours ago", "Yesterday", "12 Mar". */
export function ago (ms: number, now = Date.now()): string {
  const s = Math.max(0, (now - ms) / 1000);
  if (s < 60) return 'Just now';
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} ${Math.floor(s / 3600) === 1 ? 'hour' : 'hours'} ago`;
  if (s < 172800) return 'Yesterday';
  if (s < 604800) return `${Math.floor(s / 86400)} days ago`;
  return new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
}

/** Where a view lives in the page's address: `#@project/datasets`, `#@recent`. */
export function viewHash (view: HomeView): string {
  return view.kind === 'all' ? '' : view.id ? `@${view.kind}/${encodeURIComponent(view.id)}` : `@${view.kind}`;
}

export function parseView (hash: string): HomeView | undefined {
  if (!hash) return { kind: 'all' };
  const m = /^@(recent|project)(?:\/(.+))?$/.exec(hash);
  if (!m) return undefined;
  return { kind: m[1] as HomeView['kind'], ...(m[2] ? { id: decodeURIComponent(m[2]) } : {}) };
}

// The boards opened last, in this browser.
const RECENT = 'canvas:recent';

export function recentBoards (): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT) ?? '[]');
    return Array.isArray(v) ? v.filter((s): s is string => typeof s === 'string') : [];
  } catch {
    return [];
  }
}

export function opened (slug: string) {
  try {
    localStorage.setItem(RECENT, JSON.stringify([slug, ...recentBoards().filter((s) => s !== slug)].slice(0, 24)));
  } catch { /* storage off: no recents */ }
}
