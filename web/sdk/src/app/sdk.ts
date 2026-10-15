import { App } from './app';
import type { Query } from './query';
import type { Control } from './controls';
import type { EntityExtensions } from './extensions';
import { DatasetIndex } from './validation';
import { ValidationError } from '../common/errors';
import type { PluginInfo } from '../common/bridge';
import type {
  AppDeclaration, Backend, DatasetDescriptor, LineageOptions, LineageResult, LineageTarget, Rendering, RenderRequest,
  SdkEnvironment, SdkFeatures, User,
} from '../common/types';
import { version } from '../../package.json';

/** The plugins a host installed, as `Anfra.plugins`. */
export class InstalledPlugins {
  /** Each plugin's name and version, in the order the host installed them. */
  readonly list: readonly Readonly<PluginInfo>[];

  constructor (plugins: readonly PluginInfo[]) {
    this.list = Object.freeze(plugins.map(({ name, version }) => Object.freeze({ name, version })));
  }

  /** Whether the host installed the plugin named `name`. */
  has (name: string): boolean {
    return this.list.some((plugin) => plugin.name === name);
  }

  /** The installed plugin's version, or undefined. */
  version (name: string): string | undefined {
    return this.list.find((plugin) => plugin.name === name)?.version;
  }
}

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
   * The plugins the host installed. A plugin adds its API under its own name (`Anfra.lineage`,
   * `query.export`), and a plugin that isn't installed adds nothing: check `Anfra.plugins.has(name)`
   * before using an optional one.
   */
  readonly plugins: InstalledPlugins;

  private readonly features: Readonly<SdkFeatures>;

  /** @internal What plugins add to each query and control, by plugin name (the kernel's `provideOn*`). */
  readonly extensions: EntityExtensions<Query, Control> = { query: new Map(), control: new Map() };

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
    this.features = Object.freeze({ ...environment.features });
    this.plugins = new InstalledPlugins(environment.plugins ?? []);
  }

  createApp (declaration: AppDeclaration = {}): App {
    const app = new App(declaration, this.indices, this.backend, this.features, this.extensions);
    app.index = this._apps.length;
    this._apps.push(app);
    return app;
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

/**
 * What anfra's own plugins add to `Anfra`, each only where its host installed it: check
 * `Anfra.plugins.has(name)` first. Typed here so an author's editor knows them; a plugin outside the
 * SDK adds its own, untyped unless it says.
 */
// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging -- a plugin's member, on the class it is added to
export interface Sdk {
  /**
   * The `lineage` plugin: trace fields through the repo's catalog, as a diagram to draw. Name each
   * field by its parent and its name in it (`{ parent: 'ecommerce', child: 'revenue' }`). A field
   * that cannot be traced answers its `error`, and the others still answer.
   */
  readonly lineage?: (targets: LineageTarget | readonly LineageTarget[], options?: LineageOptions) => Promise<LineageResult>;
  /**
   * The `render` plugin: render the whole app, or one block or container of its structure by id,
   * as a PNG or a PDF, and have the host save it for the reader. What is drawn is what the page
   * shows now: `await app.execute()` first for fresh results. Elements marked
   * `data-anfra-render="skip"` are left out. Show its button only when
   * `Anfra.user.permissions.canExportData` is true too.
   */
  readonly render?: (request: RenderRequest) => Promise<Rendering>;
}

export function createSdk (environment: SdkEnvironment): Sdk {
  return new Sdk(environment);
}
