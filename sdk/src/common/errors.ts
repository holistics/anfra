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
