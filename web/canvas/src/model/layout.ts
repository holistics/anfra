// Tidy up: lay a board's screens out from its flow. The main path (from where the flow starts, on
// along the longest run of arrows) takes the first row, left to right. A branch off it starts in the
// column of the screen it branches from, a row below, and runs right from there; branches off the
// same screen sit side by side. A section is a band of rows, sized to hold its screens. Screens on
// no arrow go in a grid below, and notes go beside what they belong to.
import type { Board, Comment, Note, Point, Section } from './board.ts';
import { bounds, contents } from './ops.ts';

// Frame and section names, and arrow labels, keep their size on screen, so they take more of the
// world the further out the view is. The gaps leave them room at the zoom a whole flow is seen at.
export interface TidyOptions {
  /** Between columns: room for a connector's turns and its label. */
  gapX: number;
  /** Between rows, the frames' names above them included. */
  gapY: number;
  /** Inside a section, around its frames. */
  pad: number;
  /** Inside a section, above its frames: room for its name and theirs. */
  padTop: number;
  /** Between one section and the next. */
  gapSection: number;
  /** Columns in the grid of frames on no arrow. */
  gridColumns: number;
}

export const TIDY: TidyOptions = { gapX: 480, gapY: 480, pad: 120, padTop: 320, gapSection: 480, gridColumns: 4 };

type Edge = { from: string, to: string };

export function tidy (board: Board, o: TidyOptions = TIDY): Board {
  const frames = board.frames;
  if (!frames.length) return board;
  const ids = new Set(frames.map((f) => f.id));
  const edges = board.arrows.filter((a) => ids.has(a.from) && ids.has(a.to) && a.from !== a.to);
  const order = [...frames].sort((p, q) => p.y - q.y || p.x - q.x);
  const rank = new Map(order.map((f, i) => [f.id, i]));
  const byReading = (p: string, q: string) => rank.get(p)! - rank.get(q)!;

  // The arrows that go forward: all but those back up a cycle.
  const out = new Map<string, string[]>();
  for (const e of edges) out.set(e.from, [...(out.get(e.from) ?? []), e.to]);
  const into = new Set(edges.map((e) => e.to));
  const starts = order.filter((f) => !into.has(f.id) && out.has(f.id)).map((f) => f.id);
  const forward: Edge[] = [];
  const seen = new Set<string>();
  const onPath = new Set<string>();
  const visit = (id: string) => {
    seen.add(id);
    onPath.add(id);
    for (const to of out.get(id) ?? []) {
      if (onPath.has(to)) continue;
      forward.push({ from: id, to });
      if (!seen.has(to)) visit(to);
    }
    onPath.delete(id);
  };
  for (const id of [...starts, ...order.map((f) => f.id)]) if (!seen.has(id) && out.has(id)) visit(id);
  const next = (id: string) => forward.filter((e) => e.from === id).map((e) => e.to);
  const before = (id: string) => forward.filter((e) => e.to === id).map((e) => e.from);

  // How many screens lie ahead of each, along the longest way: the main path follows the most.
  const ahead = new Map<string, number>();
  for (const id of topo(order.map((f) => f.id), forward).reverse()) {
    ahead.set(id, Math.max(0, ...next(id).map((to) => ahead.get(to)! + 1)));
  }
  const chainFrom = (head: string, left: Set<string>): string[] => {
    const chain: string[] = [];
    for (let at: string | undefined = head; at !== undefined && left.has(at);) {
      chain.push(at);
      left.delete(at);
      at = next(at).filter((to) => left.has(to)).sort((p, q) => ahead.get(q)! - ahead.get(p)! || byReading(p, q))[0];
    }
    return chain;
  };

  // Chains and columns: the main path first, then each branch from the screen it leaves.
  const linked = new Set(edges.flatMap((e) => [e.from, e.to]));
  const left = new Set(order.map((f) => f.id).filter((id) => linked.has(id)));
  const col = new Map<string, number>();
  const chains: { ids: string[], from?: string }[] = [];
  while (left.size) {
    const branches = [...left]
      .map((id) => ({ id, from: before(id).filter((p) => col.has(p)).sort((p, q) => col.get(p)! - col.get(q)!)[0] }))
      .filter((b) => b.from !== undefined)
      .sort((p, q) => col.get(p.from!)! - col.get(q.from!)! || byReading(p.id, q.id));
    const fresh = starts.filter((id) => left.has(id)).sort((p, q) => ahead.get(q)! - ahead.get(p)! || byReading(p, q));
    const head = branches[0]?.id ?? fresh[0] ?? [...left].sort(byReading)[0];
    const at = branches[0] ? col.get(branches[0].from!)! : 0;
    const ids = chainFrom(head, left);
    ids.forEach((id, i) => col.set(id, at + i));
    chains.push({ ids, from: branches[0]?.from });
  }

  // Bands: each section with the frames in it, top to bottom, then the frames in none.
  const sections = [...board.sections].sort((p, q) => p.y - q.y || p.x - q.x);
  const sectionOf = new Map<string, Section>();
  for (const s of sections) for (const c of contents(board, s)) if (!sectionOf.has(c.id)) sectionOf.set(c.id, s);
  const bands: { section?: Section, frames: string[] }[] = sections.map((section) => ({ section, frames: [] }));
  const loose: string[] = [];
  for (const f of order) {
    const s = sectionOf.get(f.id);
    if (s) bands[sections.indexOf(s)].frames.push(f.id);
    else loose.push(f.id);
  }
  if (loose.length) bands.push({ frames: loose });

  // Rows: in each band, each chain (its part in the band) goes in the first row below the screen
  // it leaves where its columns are free, moved right as far as it must; a flow of its own starts
  // a row. Then the grid.
  const row = new Map<string, number>();
  const widest = Math.max(o.gridColumns, ...[...col.values()].map((c) => c + 1));
  for (const band of bands) {
    const mine = new Set(band.frames);
    const taken = new Set<string>();
    let used = 0;
    const put = (id: string, r: number, c: number) => {
      row.set(id, r);
      col.set(id, c);
      taken.add(`${r}|${c}`);
      used = Math.max(used, r + 1);
    };
    for (const chain of chains) {
      const part = chain.ids.filter((id) => mine.has(id));
      if (!part.length) continue;
      const from = chain.from && mine.has(chain.from) ? row.get(chain.from)! + 1 : 0;
      for (let r = chain.from ? from : used; ; r++) {
        const shift = [0, 1, 2, 3, 4, 5, 6, 7].find((k) => part.every((id) => !taken.has(`${r}|${col.get(id)! + k}`)));
        if (shift === undefined) continue;
        const cols = part.map((id) => col.get(id)! + shift);
        part.forEach((id, i) => put(id, r, cols[i]));
        break;
      }
    }
    const alone = band.frames.filter((id) => !linked.has(id));
    const top = used;
    alone.forEach((id, i) => put(id, top + Math.floor(i / widest), i % widest));
  }

  // Sizes: each column as wide as its widest frame, each band's rows as tall as their tallest.
  const byId = new Map(frames.map((f) => [f.id, f]));
  const colWidth = new Map<number, number>();
  for (const f of frames) colWidth.set(col.get(f.id)!, Math.max(colWidth.get(col.get(f.id)!) ?? 0, f.width));
  const colX = new Map<number, number>();
  let x = 0;
  for (const c of [...colWidth.keys()].sort((p, q) => p - q)) {
    colX.set(c, x);
    x += colWidth.get(c)! + o.gapX;
  }

  const all = bounds([...board.frames, ...board.sections, ...board.notes])!;
  const origin: Point = { x: all.x + (sections.length ? o.pad : 0), y: all.y + (sections.length ? o.padTop : 0) };
  const placed = new Map<string, Point>();
  const sized = new Map<string, Section>();
  const notesIn = (s: Section) => contents(board, s).filter((c): c is Note => c.kind === 'note' && !attachedTo(board, c.id));
  const notePlaced = new Map<string, Point>();
  let y = origin.y;
  for (const band of bands) {
    if (!band.frames.length) continue;
    const heights = new Map<number, number>();
    for (const id of band.frames) heights.set(row.get(id)!, Math.max(heights.get(row.get(id)!) ?? 0, byId.get(id)!.height));
    const rowY = new Map<number, number>();
    let ry = y;
    for (const r of [...heights.keys()].sort((p, q) => p - q)) {
      rowY.set(r, ry);
      ry += heights.get(r)! + o.gapY;
    }
    for (const id of band.frames) placed.set(id, { x: origin.x + colX.get(col.get(id)!)!, y: rowY.get(row.get(id)!)! });
    const box = bounds(band.frames.map((id) => ({ ...byId.get(id)!, ...placed.get(id)! })))!;
    if (!band.section) {
      y = box.y + box.height + o.gapSection;
      continue;
    }
    // The section's own notes stack to the right of its frames, inside it.
    let ny = box.y;
    let right = box.x + box.width;
    for (const n of notesIn(band.section)) {
      notePlaced.set(n.id, { x: box.x + box.width + o.gapX / 2, y: ny });
      ny += n.height + 40;
      right = Math.max(right, box.x + box.width + o.gapX / 2 + n.width);
    }
    sized.set(band.section.id, {
      ...band.section,
      x: box.x - o.pad,
      y: box.y - o.padTop,
      width: right - box.x + o.pad * 2,
      height: Math.max(box.height, ny - 40 - box.y) + o.padTop + o.pad,
    });
    y = box.y + box.height + o.pad + o.gapSection + o.padTop;
  }

  // Notes: one on an arrow moves with its frame; the rest stack to the right of everything.
  const delta = new Map<string, Point>();
  for (const f of frames) delta.set(f.id, { x: placed.get(f.id)!.x - f.x, y: placed.get(f.id)!.y - f.y });
  const edge = Math.max(...frames.map((f) => placed.get(f.id)!.x + f.width), ...[...sized.values()].map((s) => s.x + s.width));
  let free = origin.y;
  const notes = board.notes.map((n): Note => {
    const frame = attachedTo(board, n.id);
    const d = frame && delta.get(frame);
    if (d) return { ...n, x: n.x + d.x, y: n.y + d.y };
    const at = notePlaced.get(n.id);
    if (at) return { ...n, ...at };
    const spot = { ...n, x: edge + o.gapX / 2, y: free };
    free += n.height + 40;
    return spot;
  });
  for (const n of notes) {
    const was = board.notes.find((m) => m.id === n.id)!;
    delta.set(n.id, { x: n.x - was.x, y: n.y - was.y });
  }
  for (const s of board.sections) {
    const to = sized.get(s.id);
    if (to) delta.set(s.id, { x: to.x - s.x, y: to.y - s.y });
  }

  // Comments pinned to the canvas move with the shape they were pinned on.
  const under = (p: Point) => [...board.frames, ...board.notes, ...sections]
    .find((r) => p.x >= r.x && p.y >= r.y && p.x <= r.x + r.width && p.y <= r.y + r.height);
  const comments = board.comments.map((c): Comment => {
    if (c.anchor.type !== 'canvas') return c;
    const d = delta.get(under(c.anchor)?.id ?? '');
    return d ? { ...c, anchor: { ...c.anchor, x: c.anchor.x + d.x, y: c.anchor.y + d.y } } : c;
  });

  return {
    ...board,
    frames: frames.map((f) => ({ ...f, ...placed.get(f.id)! })),
    sections: board.sections.map((s) => sized.get(s.id) ?? s),
    notes,
    comments,
  };
}

/** The frame a note points at or is pointed at from, if any. */
function attachedTo (board: Board, note: string): string | undefined {
  const frames = new Set(board.frames.map((f) => f.id));
  for (const a of board.arrows) {
    if (a.from === note && frames.has(a.to)) return a.to;
    if (a.to === note && frames.has(a.from)) return a.from;
  }
  return undefined;
}

/** The ids in an order where every edge goes forward (the edges have no cycle). */
function topo (ids: string[], edges: Edge[]): string[] {
  const indeg = new Map(ids.map((id) => [id, 0]));
  for (const e of edges) indeg.set(e.to, (indeg.get(e.to) ?? 0) + 1);
  const queue = ids.filter((id) => !indeg.get(id));
  const out: string[] = [];
  while (queue.length) {
    const id = queue.shift()!;
    out.push(id);
    for (const e of edges) {
      if (e.from !== id) continue;
      indeg.set(e.to, indeg.get(e.to)! - 1);
      if (!indeg.get(e.to)) queue.push(e.to);
    }
  }
  return [...out, ...ids.filter((id) => !out.includes(id))];
}
