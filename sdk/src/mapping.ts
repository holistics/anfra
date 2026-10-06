/* eslint-disable max-classes-per-file -- Mapping and CrossFilter are the two kinds of one thing,
   a directed edge in the app's entity graph; splitting the file would hide that. */
import type { ControlMappingJson, CrossFilterJson, FilterAggregation } from './types';
import type { Control } from './controls';
import type { Query } from './query';

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

  constructor (from: Control, to: Query, field: string, aggregation?: FilterAggregation) {
    this.from = from;
    this.to = to;
    this.field = field;
    this.aggregation = aggregation;
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

  constructor (from: Query, to: Query) {
    this.from = from;
    this.to = to;
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
