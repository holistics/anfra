// Two people (a designer on the canvas, an agent over MCP) change one board at once. A save that
// started from an older board is merged onto the newer one, object by object: what each side
// added, changed or removed since the board they both started from is kept, and where both
// changed the same object, the later save wins for that object.
import { COLLECTIONS, type Board, type BoardObject } from './board.ts';

const index = (list: BoardObject[]) => new Map(list.map((o) => [o.id, o]));
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/** `mine` and `theirs` both started from `base`; answers `theirs` with what `mine` changed on top. */
export function merge (base: Board, mine: Board, theirs: Board): Board {
  const out: Board = { ...theirs, name: mine.name !== base.name ? mine.name : theirs.name };
  // The board's details: mine where I changed them, else theirs.
  const project = mine.project !== base.project ? mine.project : theirs.project;
  if (project === undefined) delete out.project;
  else out.project = project;
  for (const key of COLLECTIONS) {
    const was = index(base[key] as BoardObject[]);
    const now = index(mine[key] as BoardObject[]);
    const result = index(theirs[key] as BoardObject[]);
    for (const [id, o] of now) {
      const before = was.get(id);
      if (!before || !same(before, o)) result.set(id, o); // added or changed by me
    }
    for (const id of was.keys()) {
      if (!now.has(id)) result.delete(id); // removed by me
    }
    // Keep their order, then mine for what only I added.
    const order = [...(theirs[key] as BoardObject[]).map((o) => o.id), ...(mine[key] as BoardObject[]).map((o) => o.id)];
    (out[key] as BoardObject[]) = [...new Set(order)].filter((id) => result.has(id)).map((id) => result.get(id)!);
  }
  return out;
}
