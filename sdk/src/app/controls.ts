/* eslint-disable max-classes-per-file -- Control and its subclasses are one mechanism;
   separating them would hide that they differ only in the operators they produce. */
import { Observable } from './observable';
import { ValidationError } from '../common/errors';
import type { AppContext } from './internal';
import type { DatasetIndex, ResolvedField } from './validation';
import type {
  Backend,
  Condition,
  ConditionValue,
  DateDrillDeclaration,
  DateGrain,
  EntityKind,
  FilterDeclaration,
  FilterValueType,
} from '../common/types';

const DATE_GRAINS: DateGrain[] = ['year', 'quarter', 'month', 'week', 'day', 'hour', 'minute'];

function assertGrain (entity: string, grain: DateGrain): void {
  if (!DATE_GRAINS.includes(grain)) {
    throw new ValidationError(
      `Invalid grain '${grain}'. Expected one of: ${DATE_GRAINS.join(', ')}.`,
      entity,
    );
  }
}

function drillCondition (entity: string, grain?: DateGrain): Condition {
  if (!grain) return { operator: 'none' };
  assertGrain(entity, grain);
  return { operator: 'transform_date_drill', values: [`datetrunc ${grain}`] };
}

function inferType (fieldType: string): FilterValueType {
  const type = fieldType.toLowerCase();
  if (type.includes('date') || type.includes('time')) return 'date';
  if (type.includes('int') || type.includes('number') || type.includes('float') || type.includes('decimal')) return 'number';
  if (type.includes('bool')) return 'boolean';
  return 'string';
}

function sameCondition (a: Condition | undefined, b: Condition | undefined): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
}

/**
 * Base for the interactive controls. They are one mechanism — a named condition that
 * maps onto queries — and differ only in the operators they produce. They are separate classes
 * because a single `Filter` accepting all 25 operators would be untypeable in any useful way, and
 * nobody should have to remember that a date drill is `transform_date_drill` with
 * `values: ['datetrunc month']`.
 */
export abstract class Control extends Observable {
  readonly name: string;

  abstract readonly kind: EntityKind;

  protected readonly app: AppContext & Observable;

  private pending: Condition;

  private applied?: Condition;

  private readonly initial: Condition;

  protected constructor (name: string, app: AppContext & Observable, initial: Condition) {
    // Parented to the app so one `app.subscribe` sees every entity's changes.
    super(app);
    this.name = name;
    this.app = app;
    this.initial = initial;
    this.pending = initial;
  }

  /** The reader's current value, which `execute()` will commit. */
  get condition (): Condition {
    return this.pending;
  }

  /** The value this control's mapped queries last ran with. Undefined before the first execution. */
  get appliedCondition (): Condition | undefined {
    return this.applied;
  }

  /**
   * False before the first execution: with nothing applied there is nothing to differ from, so a
   * control set from a URL at startup is not an unapplied change.
   */
  get isDirty (): boolean {
    return this.applied !== undefined && !sameCondition(this.pending, this.applied);
  }

  setCondition (condition: Condition): void {
    this.pending = condition;
    this.notify();
  }

  reset (): void {
    this.setCondition(this.initial);
  }

  /** @internal Called by the executor once the queries reading this control have been submitted. */
  commit (): void {
    this.applied = this.pending;
  }

  abstract toJSON (): unknown;
}

export class Filter extends Control {
  readonly kind = 'filter' as const;

  readonly declaration: FilterDeclaration;

  readonly type: FilterValueType;

  private readonly resolved?: ResolvedField;

  private readonly datasetName?: string;

  private readonly backend?: Backend;

  private loadedOptions?: ConditionValue[];

  constructor (
    name: string,
    app: AppContext & Observable,
    declaration: FilterDeclaration,
    dataset?: DatasetIndex,
    backend?: Backend,
  ) {
    super(name, app, declaration.default ?? { operator: 'is', values: [] });
    this.declaration = declaration;
    this.backend = backend;

    if (declaration.field) {
      if (!dataset) {
        throw new ValidationError(
          `Filter '${name}' names a field but no dataset. Pass \`dataset\` alongside \`field\`.`,
          name,
        );
      }
      this.resolved = dataset.resolve(declaration.field, name);
      this.datasetName = dataset.descriptor.name;
      this.type = declaration.type ?? inferType(this.resolved.type);
    } else {
      if (!declaration.type) {
        throw new ValidationError(
          `Filter '${name}' is a manual filter, so it needs a \`type\`.`,
          name,
        );
      }
      this.type = declaration.type;
    }
  }

  /** True for a field-backed filter, false for a manual one. The two are separate axes from `type`. */
  get isFieldBacked (): boolean {
    return this.resolved !== undefined;
  }

  /** A manual filter's static list, or whatever the last `loadOptions()` returned. */
  get options (): ConditionValue[] {
    return this.loadedOptions ?? this.declaration.options ?? [];
  }

  /**
   * Fetches option values for a field-backed filter.
   *
   * Lazy rather than fetched up front: building a distinct-values query for every filter is slow,
   * and readers often touch none of them. The backend decides which values this reader may see.
   */
  async loadOptions (search?: string, signal: AbortSignal = new AbortController().signal): Promise<ConditionValue[]> {
    // A metric has no column of values to suggest from.
    const model = this.resolved?.modelName;
    if (!this.resolved || !model || !this.backend || !this.datasetName) return this.options;

    const options = await this.backend.fieldSuggestions(
      {
        dataset: this.datasetName,
        model,
        field: this.resolved.fieldName,
        q: search ?? '',
      },
      signal,
    );

    this.loadedOptions = [...(options ?? [])];
    this.notify();
    return this.loadedOptions;
  }

  toJSON (): FilterDeclaration {
    return this.declaration;
  }
}

export class DateDrillControl extends Control {
  readonly kind = 'dateDrill' as const;

  readonly declaration: DateDrillDeclaration;

  constructor (name: string, app: AppContext & Observable, declaration: DateDrillDeclaration = {}) {
    super(name, app, drillCondition(name, declaration.default));
    this.declaration = declaration;
  }

  setGrain (grain: DateGrain): void {
    this.setCondition(drillCondition(this.name, grain));
  }

  toJSON (): DateDrillDeclaration {
    return this.declaration;
  }
}
