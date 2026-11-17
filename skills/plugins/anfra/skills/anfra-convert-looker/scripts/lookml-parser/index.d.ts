/** Types for the part of this lookml-parser copy that the plugin uses. Upstream ships no types. */

export interface ParseFilesOptions {
  input: readonly { path: string, content: string }[];
  modelAssembly?: boolean;
  extensions?: boolean;
  strings?: boolean;
  positions?: boolean;
  console?: {
    log?: (...args: unknown[]) => void;
    warn?: (...args: unknown[]) => void;
    error?: (...args: unknown[]) => void;
  };
}

export function parseFiles (options: ParseFilesOptions): Promise<Record<string, unknown>>;
