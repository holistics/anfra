import { App } from './app';
import type { Query } from './query';
import type { Control } from './controls';
import type { EntityExtensions } from './extensions';
import { DatasetIndex } from './validation';
import { ValidationError } from '../common/errors';
import type { PluginInfo } from '../common/bridge';
import type {
  AppDeclaration, Backend, DatasetDescriptor, SdkEnvironment, SdkFeatures, User,
} from '../common/types';
import type { PluginMembers, PluginName } from './pluginTypes';
import { DEFAULT_TOKENS, type Theme, type ThemeScheme, type ThemeTokens } from '../common/theme';
import { version } from '../../package.json';

/**
 * The host's theme, as `Anfra.theme`: its scheme and tokens, for a definition that wants to follow
 * its host's light or dark mode. Nothing is applied to the page unasked: read them, or use the
 * `--anfra-*` names in your own CSS after setting them yourself.
 */
export class HostTheme {
  private current: Theme = { scheme: 'light', tokens: DEFAULT_TOKENS.light };

  private readonly listeners = new Set<(theme: Theme) => void>();

  /** `light` or `dark`; `light` until the host says. */
  get scheme (): ThemeScheme {
    return this.current.scheme;
  }

  /** Every token by name (`color-bg`, `font`): each is the CSS custom property `--anfra-<name>`. */
  get tokens (): Readonly<ThemeTokens> {
    return this.current.tokens;
  }

  /** Each time the host's theme changes. Returns the function that stops listening. */
  subscribe (listener: (theme: Theme) => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  /** @internal The kernel's, on the host's word. */
  set (theme: Theme): void {
    this.current = theme;
    this.listeners.forEach((listener) => {
      try { listener(theme); } catch { /* one listener must not stop the others */ }
    });
  }
}

/** The plugins a host installed, as `Anfra.plugins`. */
export class InstalledPlugins {
  /** Each plugin's name and version, in the order the host installed them. */
  readonly list: readonly Readonly<PluginInfo>[];

  constructor (plugins: readonly PluginInfo[]) {
    this.list = Object.freeze(plugins.map(({ name, version }) => Object.freeze({ name, version })));
  }

  /**
   * Whether the host installed the plugin named `name`. To call a plugin's member, check the member
   * itself (`if (Anfra.render)`, `query.export?.()`): TypeScript narrows that, not this.
   */
  has (name: PluginName): boolean {
    return this.list.some((plugin) => plugin.name === name);
  }

  /** The installed plugin's version, or undefined. */
  version (name: PluginName): string | undefined {
    return this.list.find((plugin) => plugin.name === name)?.version;
  }
}

/**
 * What `Anfra.features` says: the kernel's gates, and whether each of anfra's optional plugins is
 * installed.
 *
 * @deprecated Check `Anfra.plugins.has(name)` instead. Kept so Data Apps written before plugins keep
 * working; removed in the next breaking release.
 */
export interface DeprecatedFeatures extends SdkFeatures {
  /** `Anfra.plugins.has('export')`. */
  readonly export: boolean;
  /** `Anfra.plugins.has('render')`. */
  readonly render: boolean;
  /** `Anfra.plugins.has('lineage')`. */
  readonly lineage: boolean;
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

  /** The host's theme: `light` or `dark`, and its tokens. */
  readonly theme = new HostTheme();

  /** The kernel's gates (`dateDrill`), as the environment gave them. */
  private readonly gates: Readonly<SdkFeatures>;

  private deprecatedFeatures?: Readonly<DeprecatedFeatures>;

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
    this.gates = Object.freeze({ ...environment.features });
    this.plugins = new InstalledPlugins(environment.plugins ?? []);
  }

  /**
   * Whether the host offers each of anfra's optional plugins, from before there were plugins. The
   * first read warns once, in the console.
   *
   * @deprecated Check `Anfra.plugins.has(name)` instead: `Anfra.plugins.has('export')` for
   * `Anfra.features.export`. Kept so Data Apps written before plugins keep working; removed in the
   * next breaking release.
   */
  get features (): Readonly<DeprecatedFeatures> {
    if (!this.deprecatedFeatures) {
      console.warn("anfra: Anfra.features is deprecated and will be removed. Check Anfra.plugins.has('export'), "
        + "Anfra.plugins.has('render') or Anfra.plugins.has('lineage') instead.");
      this.deprecatedFeatures = Object.freeze({
        ...this.gates,
        export: this.plugins.has('export'),
        render: this.plugins.has('render'),
        lineage: this.plugins.has('lineage'),
      });
    }
    return this.deprecatedFeatures;
  }

  createApp (declaration: AppDeclaration = {}): App {
    const app = new App(declaration, this.indices, this.backend, this.gates, this.extensions);
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

/** What the installed plugins add to `Anfra` (`AnfraPlugins`): each member is there only where its plugin is. */
// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging -- plugins' members, on the class they are added to
export interface Sdk extends PluginMembers<'anfra'> {}

export function createSdk (environment: SdkEnvironment): Sdk {
  return new Sdk(environment);
}
