/**
 * The little of a query's AQL text the plugin reads: what it declares before its `explore`, and
 * the conditions of the explore's own `filters { }`. Not a parser (the SDK parses no AQL, docs/adr/0001):
 * a scan that knows strings, `//` comments, `@(...)` dates and bracket depth, enough to find those
 * blocks and split a block at its commas. What it can't read, it leaves out.
 */

const IDENTIFIER = /[A-Za-z_][A-Za-z0-9_]*/y;

/** Where the code resumes after a string, comment or date literal starting at `i`; `i` when none does. */
function skipLiteral (text: string, i: number): number {
  const c = text[i];
  if (c === '/' && text[i + 1] === '/') {
    const end = text.indexOf('\n', i);
    return end < 0 ? text.length : end;
  }
  if (c === '"' || c === "'" || c === '`') {
    for (let j = i + 1; j < text.length; j += 1) {
      if (text[j] === '\\') j += 1;
      else if (text[j] === c) return j + 1;
    }
    return text.length;
  }
  if (c === '@' && text[i + 1] === '(') {
    const end = text.indexOf(')', i);
    return end < 0 ? text.length : end + 1;
  }
  return i;
}

const OPENERS: Record<string, string> = { '(': ')', '[': ']', '{': '}' };

/** The index just past the bracket matching the one at `open`; undefined when it isn't closed. */
function closing (text: string, open: number): number | undefined {
  let depth = 0;
  for (let i = open; i < text.length;) {
    const next = skipLiteral(text, i);
    if (next !== i) { i = next; continue; }
    if (text[i] in OPENERS) depth += 1;
    else if (text[i] === ')' || text[i] === ']' || text[i] === '}') {
      depth -= 1;
      if (depth === 0) return i + 1;
    }
    i += 1;
  }
  return undefined;
}

interface Block {
  /** Where its name starts. */
  start: number;
  /** Its `{`. */
  open: number;
  /** Just past its `}`. */
  end: number;
}

/** The `<name> { ... }` block at the top level of `text[from, to)`, if there is one. */
function blockNamed (text: string, name: string, from = 0, to = text.length): Block | undefined {
  for (let i = from; i < to;) {
    const next = skipLiteral(text, i);
    if (next !== i) { i = next; continue; }
    if (text[i] in OPENERS) {
      i = closing(text, i) ?? to;
      continue;
    }
    IDENTIFIER.lastIndex = i;
    const word = IDENTIFIER.exec(text);
    if (!word) { i += 1; continue; }
    const after = i + word[0].length;
    const open = text.slice(after, to).search(/\S/) + after;
    // Only a whole word: `my_explore {` is not `explore {`.
    const whole = i === 0 || !/[A-Za-z0-9_.]/.test(text[i - 1]);
    if (whole && word[0] === name && text[open] === '{') {
      const end = closing(text, open);
      return end === undefined ? undefined : { start: i, open, end };
    }
    i = after;
  }
  return undefined;
}

/**
 * What the query declares before its `explore` (a query-local `dimension orders.kind = ...;`): kept,
 * since the point's conditions may name what it declares.
 */
export function preambleOf (aql: string): string {
  const explore = blockNamed(aql, 'explore');
  return explore ? aql.slice(0, explore.start).trim() : '';
}

/** The text with its comments taken out, and its parts split at the top level's commas. */
function splitTopLevel (text: string): string[] {
  const parts: string[] = [];
  let current = '';
  for (let i = 0; i < text.length;) {
    const next = skipLiteral(text, i);
    if (next !== i) {
      if (!text.startsWith('//', i)) current += text.slice(i, next);
      i = next;
      continue;
    }
    if (text[i] in OPENERS) {
      const end = closing(text, i) ?? text.length;
      current += text.slice(i, end);
      i = end;
      continue;
    }
    if (text[i] === ',') {
      parts.push(current);
      current = '';
    } else {
      current += text[i];
    }
    i += 1;
  }
  parts.push(current);
  return parts.map((part) => part.trim()).filter(Boolean);
}

/**
 * The bare names a condition reads, outside its literals: not a model (`orders.`), a field
 * (`.status`) or a function (`sum(`). An explore's alias is one; so are words like `and`.
 */
export function bareNames (condition: string): Set<string> {
  const names = new Set<string>();
  for (let i = 0; i < condition.length;) {
    const next = skipLiteral(condition, i);
    if (next !== i) { i = next; continue; }
    IDENTIFIER.lastIndex = i;
    const word = IDENTIFIER.exec(condition);
    if (!word || (i > 0 && /[A-Za-z0-9_]/.test(condition[i - 1]))) { i += 1; continue; }
    const after = i + word[0].length;
    const following = condition.slice(after).match(/^\s*(\S)/)?.[1];
    if (condition[i - 1] !== '.' && following !== '.' && following !== '(') names.add(word[0]);
    i = after;
  }
  return names;
}

/**
 * The conditions of the explore's own `filters { }`, each as written, its comments taken out;
 * empty when it has none.
 */
export function exploreFilters (aql: string): string[] {
  const explore = blockNamed(aql, 'explore');
  if (!explore) return [];
  const filters = blockNamed(aql, 'filters', explore.open + 1, explore.end - 1);
  return filters ? splitTopLevel(aql.slice(filters.open + 1, filters.end - 1)) : [];
}
