import { App } from './app';
import { DatasetIndex } from './validation';
import { TransportError, ValidationError } from '../common/errors';
import type {
  AppDeclaration, Backend, DatasetDescriptor, LineageOptions, LineageResult, LineageTarget, Rendering, RenderRequest,
  SdkEnvironment, SdkFeatures, User,
} from '../common/types';
import { version } from '../../package.json';

/** The package's version: anfra's own, as each release publishes the SDK with it. */
export const VERSION: string = version;

/**
 * A provisioned SDK instance: the environment, plus the one factory an author needs.
 *
 * Authors never construct this. The provisioner builds it and exposes it to the sandbox
 * as `Anfra`, so author and agent code contains no setup at all — which matters most for the
 * agent, which cannot write boilerplate about a backend it has never seen. See docs/adr/0003.
 */
export class Sdk {
  readonly version = VERSION;

  private readonly _apps: App[] = [];

  /**
   * Readable on purpose: an agent can check a field name against the real schema before writing
   * it. This is the useful half of runtime introspection *for an author*, which is why there is
   * still no `describe()` on this surface. Describing a running app is a separate thing, addressed
   * to the host rather than the author — see `App.toInspectJSON` and docs/adr/0007.
   */
  readonly datasets: Readonly<Record<string, DatasetDescriptor>>;

  /**
   * Who the app is running as. Provisioned like `datasets` rather than fetched, so author code
   * needs no await and no null check to greet someone or hide a control they cannot use.
   */
  readonly user: Readonly<User>;

  private readonly indices: Record<string, DatasetIndex>;

  private readonly backend: Backend;

  /**
   * What the host can do beyond queries. `export` says whether `query.export()` can work here, and
   * `render` whether `render()` can: show either's button only when it and
   * `user.permissions.canExportData` are both true.
   */
  readonly features: Readonly<SdkFeatures>;

  private readonly renderHook?: (request: RenderRequest) => Promise<Rendering>;

  constructor (environment: SdkEnvironment) {
    if (!environment?.datasets) {
      throw new ValidationError(
        'No datasets provided. The host must call createSdk({ datasets }) before an app can be declared.',
      );
    }

    if (!environment.user) {
      throw new ValidationError(
        'No user provided. The host must call createSdk({ user }) before an app can be declared.',
      );
    }

    if (!environment.backend) {
      throw new ValidationError(
        'No backend provided. The host must call createSdk({ backend }) before an app can run a query.',
      );
    }

    this.datasets = Object.freeze({ ...environment.datasets });
    // Frozen one level deeper than `datasets` because `permissions` is the half author code is
    // most likely to try to "fix" when a control is hidden.
    this.user = Object.freeze({
      ...environment.user,
      permissions: Object.freeze({ ...environment.user.permissions }),
    });
    this.indices = Object.fromEntries(
      Object.entries(environment.datasets).map(([uname, descriptor]) => [
        uname,
        new DatasetIndex(descriptor),
      ]),
    );
    this.backend = environment.backend;
    this.renderHook = environment.render;
    this.features = Object.freeze({
      ...environment.features,
      // Known from the Backend itself when the host does not say. In a frame the bridge's Backend
      // has every method, so there it is the host's provision that says.
      export: environment.features?.export ?? typeof environment.backend.exportQuery === 'function',
      render: environment.features?.render ?? typeof environment.render === 'function',
    });
  }

  createApp (declaration: AppDeclaration = {}): App {
    const app = new App(declaration, this.indices, this.backend, this.features);
    app.index = this._apps.length;
    this._apps.push(app);
    return app;
  }

  /**
   * Render the whole app, or one block or container of its structure by id, as a PNG or a PDF, and
   * have the host save it for the reader. What is drawn is what the page shows now: `await
   * app.execute()` first for fresh results. Elements marked `data-anfra-render="skip"` are left out.
   * Rejects with a `ValidationError` naming what to fix.
   */
  render (request: RenderRequest): Promise<Rendering> {
    if (!this.renderHook || this.features.render === false) {
      return Promise.reject(new ValidationError('This host does not render Data Apps: check Anfra.features.render first.'));
    }
    return this.renderHook(request);
  }

  /**
   * Trace fields through the repo's catalog: what each one is built from (the fields, models and
   * sources upstream of it), as a diagram to draw. Name each field by its parent and its name in
   * it: `{ parent: 'orders', child: 'amount' }` for a model's field, `{ parent: 'ecommerce', child:
   * 'revenue' }` for a dataset's metric. The catalog is the one the host last ingested. A field
   * that cannot be traced answers its `error`, and the others still answer; a host that cannot
   * trace lineage rejects with a `TransportError`.
   */
  lineage (targets: LineageTarget | readonly LineageTarget[], options: LineageOptions = {}): Promise<LineageResult> {
    const list: LineageTarget[] = Array.isArray(targets) ? [...targets] : [targets as LineageTarget];
    const named = (s: unknown) => typeof s === 'string' && s.trim() !== '';
    if (list.length === 0 || list.some((t) => typeof t !== 'object' || t === null || !named(t.parent) || !named(t.child))) {
      return Promise.reject(new ValidationError(
        "Anfra.lineage() an array of targets, each by its parent and its name in it, such as { parent: 'ecommerce', child: 'revenue' }.",
      ));
    }
    if (!this.backend.lineage) return Promise.reject(new TransportError('This host cannot trace lineage.'));
    const { limit, offset, signal = new AbortController().signal } = options;
    return this.backend.lineage({
      targets: list.map(({ parent, child }) => ({ parent, child })),
      ...(limit !== undefined ? { limit } : {}),
      ...(offset !== undefined ? { offset } : {}),
    }, signal);
  }

  /**
   * Every app this instance has built, for a host rendering an inspection panel.
   *
   * Not addressed to author code, which already holds whatever it created — this exists because the
   * bootstrap does not, and cannot otherwise reach an app at all. Holding them means they outlive
   * the author's own reference; the frame is discarded and rebuilt on every Run, which is what
   * bounds it. See docs/adr/0007.
   */
  get apps (): readonly App[] {
    return this._apps;
  }
}

export function createSdk (environment: SdkEnvironment): Sdk {
  return new Sdk(environment);
}
