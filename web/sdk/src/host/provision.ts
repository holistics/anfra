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
  /** The frame's content policy (`contentPolicy`), set before anything of the definition's. */
  contentPolicy: string;
  provision: Provision;
  /** The frame script (`@holistics/anfra-sdk/app`'s kernel, as `__anfraKernel`), run before any of the definition's own. */
  frameScript: string;
  /** The installed plugins' frame setups (each assigning `__anfraPlugin`), run after the kernel, in this order. */
  setups?: readonly { name: string, setup: string }[];
}

/**
 * The one script a frame starts with: the kernel, then each plugin's setup, then the bootstrap,
 * all inside one function so that neither the kernel nor a setup is left on the frame's window,
 * where the definition would find it.
 */
export function composeFrameScript (frameScript: string, setups: readonly { name: string, setup: string }[] = []): string {
  const plugins = setups.map(({ name, setup }) => [
    '(function () {',
    'var __anfraPlugin;',
    setup,
    `__setups.push([${JSON.stringify(name)}, __anfraPlugin && __anfraPlugin.default]);`,
    '})();',
  ].join('\n'));
  return [
    '(function () {',
    'var __anfraKernel;',
    frameScript,
    'var __setups = [];',
    ...plugins,
    '__anfraKernel.bootstrap(window, __setups);',
    '})();',
  ].join('\n');
}

const charsetRe = /<meta[^>]*charset[^>]*>/i;
const headRe = /<head(?:\s[^>]*)?>/i;
// What may come before the policy without being parsed first: whitespace, comments and the
// doctype, which only those may precede (anything else would put the frame in quirks mode).
const preambleRe = /^(?:\s|<!--[\s\S]*?-->)*<!doctype[^>]*>/i;

/**
 * A Data App's frame document: its definition, with its content policy as the first element the
 * parser meets, so it governs every script, style and fetch of the definition's, even one written
 * ahead of its `<meta charset>`; then the provision data and the frame script (the kernel and the
 * plugins' setups) at the top of its <head> (after <meta charset> when there is one, so the
 * encoding is still declared first), so `Anfra` exists before the definition's first script runs.
 * The definition itself never mentions the SDK.
 */
export function provisionDocument (definition: string, options: ProvisionOptions): string {
  const policy = `<meta http-equiv="Content-Security-Policy" content="${escapeAttribute(options.contentPolicy)}">`;
  const injected = [
    `<script type="application/json" id="${PROVISION_ELEMENT_ID}">${scriptJSON(options.provision)}</script>`,
    `<script>${scriptCode(composeFrameScript(options.frameScript, options.setups))}</script>`,
  ].join('\n');
  const preamble = preambleRe.exec(definition)?.[0] ?? '';
  const rest = definition.slice(preamble.length);
  for (const re of [charsetRe, headRe]) {
    const match = re.exec(rest);
    if (match) {
      const at = match.index + match[0].length;
      return `${preamble}${policy}\n${rest.slice(0, at)}\n${injected}\n${rest.slice(at)}`;
    }
  }
  return `${preamble}${policy}\n${injected}\n${rest}`;
}
