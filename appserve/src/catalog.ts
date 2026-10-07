// The Data App tree the appserve backend serves at /appserve/apps (internal/appserve's Entry).
export interface DataAppEntry {
  kind: 'app';
  path: string;
  label: string;
}

export interface FolderEntry {
  kind: 'folder';
  path: string;
  name: string;
  children: CatalogEntry[];
}

export type CatalogEntry = DataAppEntry | FolderEntry;

/** The Data App a URL path names (`sales/overview`), whatever the case of its `.html`. */
export function findApp (entries: CatalogEntry[], urlPath: string): DataAppEntry | undefined {
  for (const entry of entries) {
    if (entry.kind === 'app' && entry.path.replace(EXTENSION, '') === urlPath) return entry;
    if (entry.kind === 'folder') {
      const found = findApp(entry.children, urlPath);
      if (found) return found;
    }
  }
  return undefined;
}

const EXTENSION = /\.html$/i;

/** The Data App URL of a Data App path: `sales/overview.html` -> `/apps/sales/overview`. */
export function urlFor (path: string): string {
  return `/apps/${path.replace(EXTENSION, '').split('/').map(encodeURIComponent).join('/')}`;
}

/** The Data App path a pathname names, `/apps/sales/overview` -> `sales/overview`; undefined elsewhere. */
export function pathFromLocation (pathname: string): string | undefined {
  const match = /^\/apps\/(.+?)\/*$/.exec(pathname);
  return match ? decodeURIComponent(match[1]) : undefined;
}

/** Keep the apps whose label or path contains `query`, and the folders that lead to them. */
export function filterEntries (entries: CatalogEntry[], query: string): CatalogEntry[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return entries;
  const out: CatalogEntry[] = [];
  for (const entry of entries) {
    if (entry.kind === 'app') {
      if (entry.label.toLowerCase().includes(needle) || entry.path.toLowerCase().includes(needle)) out.push(entry);
    } else {
      const children = filterEntries(entry.children, query);
      if (children.length) out.push({ ...entry, children });
    }
  }
  return out;
}

/** Every folder path that leads to an app, `sales/deep/x.html` -> `sales`, `sales/deep`. */
export function folderPaths (appPath: string): string[] {
  const parts = appPath.split('/').slice(0, -1);
  return parts.map((_, i) => parts.slice(0, i + 1).join('/'));
}
