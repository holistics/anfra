/* eslint-disable max-classes-per-file -- Control and its subclasses are one mechanism;
   separating them would hide that they differ only in the operators they produce. */
import { Observable } from './observable';
import { ValidationError } from '../common/errors';
import { abortError } from '../common/endpoint';
import { tagCall } from '../common/runtimeErrors';
import { withChanges } from './changes';
import type { AppContext } from './internal';
import type { PluginMembers } from './pluginTypes';
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

  private initial: Condition;

  /** Set once `remove()` took it out of its app; every method then throws. */
  private removed = false;

  protected constructor (name: string, app: AppContext & Observable, initial: Condition) {
    // Parented to the app so one `app.subscribe` sees every entity's changes.
    super(app);
    this.name = name;
    this.app = app;
    this.initial = initial;
    this.pending = initial;
  }

  /**
   * Bring the elements marked `data-anfra-control` with this control's name into view. False when
   * none is on the page yet; never throws.
   */
  locate (): boolean {
    this.assertLive();
    return this.app.locate('control', this.name);
  }

  /** False once `remove()` took it out of its app. */
  get isRemoved (): boolean {
    return this.removed;
  }

  /**
   * Takes this control out of its app, with its mappings: the queries it was mapped to rerun
   * without its condition at the next `app.execute()`. Its name is free again, and every method
   * of it throws from then on.
   */
  remove (): void {
    this.assertLive();
    this.app.removeEntity(this);
  }

  /** @internal Throws once the control was removed. */
  assertLive (): void {
    if (this.removed) {
      throw new ValidationError(`Control '${this.name}' was removed from its app; it can't be used any more.`, this.name);
    }
  }

  /** @internal The app's half of `remove`: stop what is in flight. */
  markRemoved (): void {
    this.removed = true;
    this.dropInFlight();
  }

  /** @internal Drops work in flight that the control's current declaration no longer wants. */
  protected dropInFlight (): void {}

  /**
   * @internal A new declaration's default: `reset()` goes back to it. `restart` also sets the
   * reader's condition to it, for a control that now filters something else.
   */
  protected redefault (initial: Condition, restart: boolean): void {
    this.initial = initial;
    if (restart) this.pending = initial;
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
    this.assertLive();
    this.pending = condition;
    this.notify();
  }

  reset (): void {
    this.assertLive();
    this.setCondition(this.initial);
  }

  /** @internal Called by the executor once the queries reading this control have been submitted. */
  commit (): void {
    this.applied = this.pending;
  }

  abstract toJSON (): unknown;
}

/** What `filter.update` may change. Left out, each keeps its value; `undefined` clears it. */
export type FilterUpdate = Partial<FilterDeclaration & { dataset: string }>;

/** A filter's declaration, resolved: what it filters and how its values are typed. */
interface ResolvedFilter {
  type: FilterValueType;
  resolved?: ResolvedField;
  datasetName?: string;
}

export class Filter extends Control {
  readonly kind = 'filter' as const;

  private _declaration: FilterDeclaration;

  private resolution: ResolvedFilter;

  private readonly backend?: Backend;

  private loadedOptions?: ConditionValue[];

  /** The `loadOptions` calls in flight, so a removal or a new field can drop them. */
  private readonly loading = new Set<AbortController>();

  constructor (
    name: string,
    app: AppContext & Observable,
    declaration: FilterDeclaration,
    dataset?: DatasetIndex,
    backend?: Backend,
  ) {
    super(name, app, declaration.default ?? { operator: 'is', values: [] });
    this.backend = backend;
    this.resolution = Filter.resolve(name, declaration, dataset);
    this._declaration = declaration;
    this.assertValues(this.condition.values ?? []);
    this.assertValues(declaration.options ?? []);
  }

  private static resolve (name: string, declaration: FilterDeclaration, dataset?: DatasetIndex): ResolvedFilter {
    if (declaration.field) {
      if (!dataset) {
        throw new ValidationError(
          `Filter '${name}' names a field but no dataset. Pass \`dataset\` alongside \`field\`.`,
          name,
        );
      }
      const resolved = dataset.resolve(declaration.field, name);
      return { resolved, datasetName: dataset.descriptor.name, type: declaration.type ?? inferType(resolved.type) };
    }
    if (!declaration.type) {
      throw new ValidationError(
        `Filter '${name}' is a manual filter, so it needs a \`type\`.`,
        name,
      );
    }
    return { type: declaration.type };
  }

  /** What it was declared with, or last updated to. */
  get declaration (): FilterDeclaration {
    return this._declaration;
  }

  get type (): FilterValueType {
    return this.resolution.type;
  }

  setCondition (condition: Condition): void {
    this.assertLive();
    this.assertValues(condition.values ?? []);
    super.setCondition(condition);
  }

  /**
   * Edits this filter in place. A new label, options or default keep the reader's condition, and
   * `reset()` goes to the new default. A new field, dataset or type is checked as `createFilter`
   * checks it, and the condition goes back to the new default: the old values filtered something
   * else. Its mappings stay as they are: each mapping's own field decides what its condition applies to.
   */
  update (changes: FilterUpdate): void {
    this.assertLive();
    const declaration = withChanges<FilterDeclaration & { dataset?: string }>(this._declaration, changes);
    const previous = this._declaration as FilterDeclaration & { dataset?: string };
    const redefined = (['field', 'dataset', 'type'] as const).some((key) => declaration[key] !== previous[key]);
    const resolution = redefined
      ? Filter.resolve(this.name, declaration, this.app.filterDataset(this.name, declaration))
      : this.resolution;
    const initial = declaration.default ?? { operator: 'is', values: [] };
    const values = (condition: Condition) => condition.values ?? [];
    this.assertValuesOf(resolution.type, values(initial));
    this.assertValuesOf(resolution.type, declaration.options ?? []);
    if (!redefined) this.assertValuesOf(resolution.type, values(this.condition));

    this._declaration = declaration;
    this.resolution = resolution;
    this.redefault(initial, redefined);
    if (redefined) {
      this.loadedOptions = undefined;
      this.dropInFlight();
    }
    this.notify();
  }

  protected dropInFlight (): void {
    this.loading.forEach((controller) => controller.abort());
    this.loading.clear();
  }

  /**
   * A number filter's values are strings, as query results give them, which keep every digit. A JS
   * number may already have lost some (`Number("9007199254740993")` is ...992), and nothing after
   * can tell, so the core API refuses one; this says so where the app hands it over.
   */
  private assertValues (values: readonly unknown[]): void {
    this.assertValuesOf(this.type, values);
  }

  private assertValuesOf (type: FilterValueType, values: readonly unknown[]): void {
    if (type !== 'number') return;
    const number = values.find((value) => typeof value === 'number');
    if (number === undefined) return;
    throw new ValidationError(
      `Filter '${this.name}' filters numbers: give its values as strings, as query results give them `
      + `(${JSON.stringify(String(number))}, not ${String(number)}), so none has lost a digit.`,
      this.name,
    );
  }

  /** True for a field-backed filter, false for a manual one. The two are separate axes from `type`. */
  get isFieldBacked (): boolean {
    return this.resolution.resolved !== undefined;
  }

  /** A manual filter's static list, or whatever the last `loadOptions()` returned. */
  get options (): ConditionValue[] {
    return this.loadedOptions ?? this._declaration.options ?? [];
  }

  /**
   * Fetches option values for a field-backed filter.
   *
   * Lazy rather than fetched up front: building a distinct-values query for every filter is slow,
   * and readers often touch none of them. The backend decides which values this reader may see.
   * Rejects with an `AbortError` when the filter is removed, or its field changes, before the list
   * arrives: the list is dropped.
   */
  async loadOptions (search?: string, signal: AbortSignal = new AbortController().signal): Promise<ConditionValue[]> {
    this.assertLive();
    // A metric has no column of values to suggest from.
    const { resolved, datasetName } = this.resolution;
    const model = resolved?.modelName;
    if (!resolved || !model || !this.backend || !datasetName) return this.options;

    const controller = new AbortController();
    const follow = () => controller.abort();
    if (signal.aborted) controller.abort();
    else signal.addEventListener('abort', follow, { once: true });
    this.loading.add(controller);
    try {
      const options = await this.backend.fieldSuggestions(
        {
          dataset: datasetName,
          model,
          field: resolved.fieldName,
          q: search ?? '',
        },
        controller.signal,
      ).catch((err: unknown) => {
        // Left unhandled, it is reported as `call.load_options`.
        tagCall(err, 'load_options');
        throw err;
      });
      // Removed, or now about another field: this list is not its own.
      if (controller.signal.aborted && !this.loading.has(controller)) throw abortError();

      this.loadedOptions = [...(options ?? [])];
      this.notify();
      return this.loadedOptions;
    } finally {
      this.loading.delete(controller);
      signal.removeEventListener('abort', follow);
    }
  }

  toJSON (): FilterDeclaration {
    return this._declaration;
  }
}

export class DateDrillControl extends Control {
  readonly kind = 'dateDrill' as const;

  private _declaration: DateDrillDeclaration;

  constructor (name: string, app: AppContext & Observable, declaration: DateDrillDeclaration = {}) {
    super(name, app, drillCondition(name, declaration.default));
    this._declaration = declaration;
  }

  /** What it was declared with, or last updated to. */
  get declaration (): DateDrillDeclaration {
    return this._declaration;
  }

  /** Edits its label or default. The reader's grain stays; `reset()` goes to the new default. */
  update (changes: DateDrillDeclaration): void {
    this.assertLive();
    const declaration = withChanges(this._declaration, changes);
    const initial = drillCondition(this.name, declaration.default);
    this._declaration = declaration;
    this.redefault(initial, false);
    this.notify();
  }

  setGrain (grain: DateGrain): void {
    this.assertLive();
    this.setCondition(drillCondition(this.name, grain));
  }

  toJSON (): DateDrillDeclaration {
    return this._declaration;
  }
}

/** What the installed plugins add to every control (`AnfraPlugins`): each member is there only where its plugin is. */
// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging -- plugins' members, on the class they are added to
export interface Control extends PluginMembers<'control'> {}
