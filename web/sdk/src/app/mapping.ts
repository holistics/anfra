/* eslint-disable max-classes-per-file -- Mapping and CrossFilter are the two kinds of one thing,
   a directed edge in the app's entity graph; splitting the file would hide that. */
import { ValidationError } from '../common/errors';
import type { ControlMappingJson, CrossFilterJson, FilterAggregation } from '../common/types';
import type { Control } from './controls';
import type { Query } from './query';

/** What an edge needs of its app: a way to take it out. */
export interface EdgeOwner {
  removeEdge (edge: Edge): void;
}

/** Throws for an edge already taken out of its app. */
function assertLive (edge: Edge, removed: boolean): void {
  if (removed) {
    throw new ValidationError(`Edge '${edge.id}' was removed from its app; map again to have it back.`, edge.from.name);
  }
}

/**
 * A directed edge from an interactive control to one field of one query.
 *
 * Always explicit: the field is never inferred from the control's own field, because "the control
 * happens to name a field this query also has" is exactly the coupling that breaks silently when
 * someone renames a dimension.
 */
export class Mapping {
  readonly kind = 'control' as const;

  readonly from: Control;

  readonly to: Query;

  readonly field: string;

  readonly aggregation?: FilterAggregation;

  private readonly owner?: EdgeOwner;

  private removed = false;

  constructor (from: Control, to: Query, field: string, aggregation?: FilterAggregation, owner?: EdgeOwner) {
    this.from = from;
    this.to = to;
    this.field = field;
    this.aggregation = aggregation;
    this.owner = owner;
  }

  /**
   * Takes this mapping out of its app: its query reruns without the control's condition at the
   * next `app.execute()`. An edge never changes: to change one, remove it and map again.
   */
  remove (): void {
    assertLive(this, this.removed);
    this.owner?.removeEdge(this);
  }

  /** @internal */
  markRemoved (): void {
    this.removed = true;
  }

  /**
   * Includes the field, unlike Dashboard as Code's `${from}.${to}` — that form collides the moment
   * one control maps onto one query on two fields.
   */
  get id (): string {
    return `${this.from.name}.${this.to.name}.${this.field}`;
  }

  toJSON (): ControlMappingJson {
    return {
      kind: this.kind,
      id: this.id,
      from: { kind: this.from.kind, name: this.from.name },
      to: { kind: 'query', name: this.to.name },
      field: this.field,
      ...(this.aggregation ? { aggregation: this.aggregation } : {}),
    };
  }
}

/**
 * A directed edge saying `from` may cross-filter `to`: when the reader selects rows in `from`, the
 * conditions derived from that selection apply to `to`.
 *
 * Carries no field. Which fields a selection conditions is decided at click time, from the rows the
 * author hands over — the same asymmetry that makes `CrossFilterInteraction` the one Dashboard as
 * Code interaction without a `field_path`. Two queries may cross-filter each other; only one
 * selection is live at a time, so a bidirectional pair is not a loop. See docs/adr/0005.
 */
export class CrossFilter {
  readonly kind = 'crossFilter' as const;

  readonly from: Query;

  readonly to: Query;

  private readonly owner?: EdgeOwner;

  private removed = false;

  constructor (from: Query, to: Query, owner?: EdgeOwner) {
    this.from = from;
    this.to = to;
    this.owner = owner;
  }

  /**
   * Takes this cross-filter out of its app: a selection in `from` stops reaching `to`, which
   * reruns without it at the next `app.execute()`. To change one, remove it and map again.
   */
  remove (): void {
    assertLive(this, this.removed);
    this.owner?.removeEdge(this);
  }

  /** @internal */
  markRemoved (): void {
    this.removed = true;
  }

  /** No field to disambiguate, and one pair may only be declared once, so the pair is the id. */
  get id (): string {
    return `${this.from.name}.${this.to.name}`;
  }

  toJSON (): CrossFilterJson {
    return {
      kind: this.kind,
      id: this.id,
      from: { kind: 'query', name: this.from.name },
      to: { kind: 'query', name: this.to.name },
    };
  }
}

/** Either kind of edge. `app.mappings` returns this; narrow on `kind`. */
export type Edge = Mapping | CrossFilter;
