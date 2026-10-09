import { PROVISION_ELEMENT_ID, type Provision } from '../common/bridge';

/** JSON safe inside a <script>: nothing in it can end the element or open a comment. */
function scriptJSON (value: unknown): string {
  return JSON.stringify(value)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029');
}

/** Code that cannot close its own <script> early. */
function scriptCode (code: string): string {
  return code.replace(/<\/script/gi, '<\\/script');
}

function escapeAttribute (value: string): string {
  return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
}

export interface ProvisionOptions {
  /** Where the definition's relative URLs resolve: the directory it was read from. */
  baseHref: string;
  provision: Provision;
  /** The frame script (`@holistics/anfra-sdk/app`'s IIFE), run before any of the definition's own. */
  frameScript: string;
}

const charsetRe = /<meta[^>]*charset[^>]*>/i;
const headRe = /<head[^>]*>/i;

/**
 * A Data App's frame document: its definition, with its base URL, the provision data and the frame
 * script added at the top of its <head> (after <meta charset> when there is one, so the encoding is
 * still declared first), so `Anfra` exists before the definition's first script runs. The
 * definition itself never mentions the SDK.
 */
export function provisionDocument (definition: string, options: ProvisionOptions): string {
  const injected = [
    `<base href="${escapeAttribute(options.baseHref)}">`,
    `<script type="application/json" id="${PROVISION_ELEMENT_ID}">${scriptJSON(options.provision)}</script>`,
    `<script>${scriptCode(options.frameScript)}</script>`,
  ].join('\n');
  for (const re of [charsetRe, headRe]) {
    const match = re.exec(definition);
    if (match) {
      const at = match.index + match[0].length;
      return `${definition.slice(0, at)}\n${injected}\n${definition.slice(at)}`;
    }
  }
  return `${injected}\n${definition}`;
}
