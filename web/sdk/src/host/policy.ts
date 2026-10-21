/**
 * The content policy a Data App's frame runs under: libraries from a short list of CDNs, data only
 * over the bridge. The sandbox keeps the frame away from the page around it; this keeps it away
 * from the network, so an app, or a library it loads, cannot send what its reader can query to a
 * server of its choosing (docs/designs/security.md).
 */

/**
 * Where an app may load scripts from, and styles and fonts beside them: two CDNs, each one more
 * server trusted to send the files an app asked for. jsDelivr serves npm as published (hashable),
 * cdnjs a curated set. A host adds others with `allowOrigins`.
 */
export const SCRIPT_ORIGINS: readonly string[] = [
  'https://cdnjs.cloudflare.com',
  'https://cdn.jsdelivr.net',
];

const STYLE_ORIGINS = [...SCRIPT_ORIGINS, 'https://fonts.googleapis.com'];
const FONT_ORIGINS = [...SCRIPT_ORIGINS, 'https://fonts.gstatic.com'];

/**
 * An origin a host may add: `https://` with a host and nothing after it, as `URL` writes it, so it
 * cannot widen a directive (a scheme, a wildcard) or end one (a space, a `;`).
 */
export function checkOrigin (origin: unknown): string {
  let url: URL | undefined;
  try { url = typeof origin === 'string' ? new URL(origin) : undefined; } catch { /* reported below */ }
  if (!url || url.protocol !== 'https:' || url.origin !== origin || url.host.includes('*')) {
    throw new Error(`An allowed origin is https://<host>[:<port>], with no path, wildcard or trailing slash: got ${JSON.stringify(origin)}.`);
  }
  return origin;
}

/**
 * The policy, with a host's own origins added to every source an app loads from, and to what it
 * may fetch: the render plugin fetches the images, stylesheets and fonts it embeds in a picture.
 * Nothing else is reachable. `'self'` is absent: the frame's origin is opaque, so it matches nothing.
 */
export function contentPolicy (allowOrigins: readonly string[] = []): string {
  const extra = allowOrigins.map(checkOrigin);
  const list = (...origins: readonly string[][]) => [...new Set([...origins.flat(), ...extra])].join(' ');
  return [
    "default-src 'none'",
    `script-src ${list([...SCRIPT_ORIGINS])} 'unsafe-inline' 'unsafe-eval'`,
    `style-src ${list(STYLE_ORIGINS)} 'unsafe-inline'`,
    `font-src ${list(FONT_ORIGINS)} data:`,
    'img-src https: data: blob:',
    `connect-src ${list(STYLE_ORIGINS, FONT_ORIGINS)} data: blob:`,
    "frame-src 'none'",
    "form-action 'none'",
    "base-uri 'none'",
  ].join('; ');
}
