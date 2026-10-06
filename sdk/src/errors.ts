/* eslint-disable max-classes-per-file -- one closed family of sibling errors; splitting them
   across five files hides the taxonomy this module exists to define. */

/**
 * Four error classes and no more. An error taxonomy is a public API: every class added here is one
 * more thing a consumer will `instanceof` against forever.
 */

export abstract class DataAppError extends Error {
  /** The query, control or dataset that caused this. */
  readonly entity?: string;

  constructor (message: string, entity?: string) {
    super(message);
    this.name = new.target.name;
    this.entity = entity;
    // Restore the prototype chain, which is lost when subclassing Error under ES5 downlevelling.
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Thrown synchronously at declaration time. This is the primary feedback channel for an agent
 * authoring an app: it has no REPL, so the thrown error is the whole loop. Messages must carry the
 * offending value and, where we can compute one, a suggestion.
 */
export class ValidationError extends DataAppError {}

/** Stored on the query, never thrown — one failed query must not blank the rest. */
export class QueryError extends DataAppError {
  readonly cause?: unknown;

  constructor (message: string, entity?: string, cause?: unknown) {
    super(message, entity);
    this.cause = cause;
  }
}

/** Separate from QueryError so an app can render "you can't see this" differently from "this broke". */
export class PermissionError extends DataAppError {}

export class TransportError extends DataAppError {
  readonly status?: number;

  constructor (message: string, status?: number, entity?: string) {
    super(message, entity);
    this.status = status;
  }
}

/**
 * Levenshtein distance, capped: only used to suggest a near-miss field name, so exact cost past a
 * couple of edits is irrelevant.
 */
function editDistance (a: string, b: string): number {
  const rows = a.length + 1;
  const cols = b.length + 1;
  let prev = Array.from({ length: cols }, (_, j) => j);

  for (let i = 1; i < rows; i++) {
    const curr = [i];
    for (let j = 1; j < cols; j++) {
      curr[j] = Math.min(
        prev[j] + 1,
        curr[j - 1] + 1,
        prev[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1),
      );
    }
    prev = curr;
  }

  return prev[cols - 1];
}

/** Returns the closest candidate within a plausible typo distance, or undefined. */
export function suggest (value: string, candidates: string[]): string | undefined {
  let best: string | undefined;
  let bestDistance = Infinity;

  candidates.forEach((candidate) => {
    const distance = editDistance(value.toLowerCase(), candidate.toLowerCase());
    if (distance < bestDistance) {
      bestDistance = distance;
      best = candidate;
    }
  });

  const threshold = Math.max(2, Math.floor(value.length / 3));
  return bestDistance <= threshold ? best : undefined;
}

export function withSuggestion (message: string, value: string, candidates: string[]): string {
  const hint = suggest(value, candidates);
  return hint ? `${message} Did you mean '${hint}'?` : message;
}
