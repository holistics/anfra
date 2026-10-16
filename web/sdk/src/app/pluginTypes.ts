/**
 * The types of what plugins add to a Data App's frame: one registry, `AnfraPlugins`, from which the
 * members of `Anfra`, a query and a control are derived, each optional, since a plugin adds its
 * API only where its host installed it (docs/designs/sdk-plugins.md, Types). Types only: nothing here
 * runs.
 */
import type {
  ExportOptions, ExportResult, LineageOptions, LineageResult, LineageTarget, Rendering, RenderRequest,
} from '../common/types';

/**
 * What a plugin adds, by where: `anfra` is `Anfra.<plugin>`, `query` is `query.<plugin>` on every
 * query, `control` is `control.<plugin>` on every control. Any may be left out.
 */
export interface PluginApi {
  anfra?: unknown;
  query?: unknown;
  control?: unknown;
}

/**
 * Every plugin's API, by the plugin's name. anfra's own are here; a plugin outside the SDK adds
 * its entry from its own declarations:
 *
 * ```ts
 * declare module '@holistics/anfra-sdk/app' {
 *   interface AnfraPlugins { notes: { anfra: NotesApi, query: QueryNotes } }
 * }
 * ```
 */
export interface AnfraPlugins {
  /**
   * `query.export()`: the query's whole result as a file, saved for the reader. Every row, not only
   * the pages loaded, with what the result on screen was last run with: the mapped controls, the
   * selection, the sort, the timezone. A control the reader changed but has not applied is not in
   * it. Resolves once the file is made and its download has started, with its link and row count;
   * rejects with a `DataAppError` naming the query, and changes nothing on it either way. Offer it
   * only when `Anfra.user.permissions.canExportData` is true too.
   */
  export: { query: (options?: ExportOptions) => Promise<ExportResult> };
  /**
   * `Anfra.render()`: the whole app, or one block or container of its structure by id, as a PNG or
   * a PDF, saved for the reader. What is drawn is what the page shows now: `await app.execute()`
   * first for fresh results. Elements marked `data-anfra-render="skip"` are left out. Offer it only
   * when `Anfra.user.permissions.canExportData` is true too.
   */
  render: { anfra: (request: RenderRequest) => Promise<Rendering> };
  /**
   * `Anfra.lineage()`: fields traced through the repo's catalog, as a diagram to draw. Name each by
   * its parent and its name in it (`{ parent: 'ecommerce', child: 'revenue' }`). A field that
   * cannot be traced answers its `error`, and the others still answer.
   */
  lineage: { anfra: (targets: LineageTarget | readonly LineageTarget[], options?: LineageOptions) => Promise<LineageResult> };
  /** Adds nothing to the app: the host's inspect panel reads the app through it. */
  inspect: Record<never, never>;
}

/** A plugin's name: anfra's own and any a plugin's declarations added, for completion, or any other. */
export type PluginName = keyof AnfraPlugins | (string & {});

/** What the plugins add at `where`, each member optional and named after its plugin. */
export type PluginMembers<Where extends keyof PluginApi> = {
  readonly [K in keyof AnfraPlugins as AnfraPlugins[K] extends Record<Where, unknown> ? K : never]?:
    AnfraPlugins[K] extends Record<Where, infer Api> ? Api : never;
};
