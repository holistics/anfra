import { App } from './app';
import { DatasetIndex } from './validation';
import { ValidationError } from '../common/errors';
import type {
  AppDeclaration, Backend, DatasetDescriptor, SdkEnvironment, SdkFeatures, User,
} from '../common/types';

export const VERSION = '0.1.0';

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

  private readonly features: SdkFeatures;

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
  }

  createApp (declaration: AppDeclaration = {}): App {
    const app = new App(declaration, this.indices, this.backend, this.features);
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

export function createSdk (environment: SdkEnvironment): Sdk {
  return new Sdk(environment);
}
