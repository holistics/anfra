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
// doctype, which only those may precede (anything else would put the frame in quirks mode), then
// an opening <html> and <head> with nothing between them, so the policy goes inside them and both
// keep their attributes. Anything else (a <script> ahead of <head>, no <head> at all) and the
// policy goes before it: the parser then opens <html> and <head> itself, merges a later <html>'s
// attributes and drops a later <head>'s, which is the price of no script running ahead of it.
// Each part matches only what the parser ends where the pattern does: a comment closes at `-->` or
// `--!>`, and `<!-->` or `<!--->` is a whole one; a `>` in a quoted attribute value doesn't end the
// tag, while a quote is a value's only after `=` (elsewhere it is part of a name). A doctype ends at
// any `>`, quoted or not. A name and an unquoted value run to their end, so the definition, which is
// untrusted, can't make the pattern backtrack through every way of splitting them.
const gap = '(?:\\s|<!--(?!-?>)[\\s\\S]*?--!?>)*';
const attribute = `[^\\s/>][^\\s/>=]*(?![^\\s/>=])(?:\\s*=\\s*(?:"[^"]*"|'[^']*'|[^\\s>"'][^\\s>]*(?![^\\s>]))|(?!\\s*=))`;
const attributes = `(?:\\s(?:[\\s/]|${attribute})*)?`;
const preambleRe = new RegExp(`^${gap}(?:<!doctype[^>]*>${gap})?(?:<html${attributes}>${gap})?(?:<head${attributes}>)?`, 'i');

/**
 * A Data App's frame document: its definition, with its content policy as the first element the
 * parser meets, so it governs every script, style and fetch of the definition's, even one written
 * ahead of its `<meta charset>`; then the provision data and the frame script (the kernel and the
 * plugins' setups) at the top of its <head> (after <meta charset> when there is one, so the
 * encoding is still declared first), so `Anfra` exists before the definition's first script runs.
 * The definition itself never mentions the SDK.
 */
export function provisionDocument (definition: string, options: ProvisionOptions): string {
  return provisionFrame(definition, options).html;
}

/**
 * Where the definition's text sits in its frame document, so a line the browser names there
 * (`about:srcdoc:147:48` in a stack) can be named in the definition: each run of the definition's
 * text, from the document's line `from` to `to`, starting at the definition's `line` and `column`
 * (0-based, on that first line only; the document's runs all start a line).
 */
export interface DefinitionLines {
  runs: readonly { from: number, to: number, line: number, column: number }[];
}

/** The frame document (`provisionDocument`), and where the definition's lines are in it. */
export function provisionFrame (definition: string, options: ProvisionOptions): { html: string, lines: DefinitionLines } {
  const policy = `<meta http-equiv="Content-Security-Policy" content="${escapeAttribute(options.contentPolicy)}">`;
  const injected = [
    `<script type="application/json" id="${PROVISION_ELEMENT_ID}">${scriptJSON(options.provision)}</script>`,
    `<script>${scriptCode(composeFrameScript(options.frameScript, options.setups))}</script>`,
  ].join('\n');
  const preamble = preambleRe.exec(definition)?.[0] ?? '';
  const rest = definition.slice(preamble.length);
  // Each piece of the document, and for the definition's, where it starts in the definition.
  const pieces: { text: string, from?: number }[] = [{ text: preamble, from: 0 }, { text: `${policy}\n` }];
  const match = charsetRe.exec(rest) ?? headRe.exec(rest);
  if (match) {
    const at = match.index + match[0].length;
    pieces.push({ text: rest.slice(0, at), from: preamble.length }, { text: `\n${injected}\n` }, { text: rest.slice(at), from: preamble.length + at });
  } else {
    pieces.push({ text: `${injected}\n` }, { text: rest, from: preamble.length });
  }
  return { html: pieces.map((piece) => piece.text).join(''), lines: linesOf(definition, pieces) };
}

const newlines = (text: string) => text.split('\n').length - 1;

function linesOf (definition: string, pieces: readonly { text: string, from?: number }[]): DefinitionLines {
  const runs: DefinitionLines['runs'][number][] = [];
  let line = 1;
  for (const { text, from } of pieces) {
    if (from !== undefined && text) {
      const before = definition.slice(0, from);
      runs.push({
        from: line, to: line + newlines(text), line: newlines(before) + 1, column: from - (before.lastIndexOf('\n') + 1),
      });
    }
    line += newlines(text);
  }
  return { runs };
}

/** The definition's line and column for the document's (1-based columns, as a stack has them), or undefined for the SDK's. */
export function definitionPosition (lines: DefinitionLines, line: number, column: number): { line: number, column: number } | undefined {
  const run = lines.runs.find((candidate) => candidate.from <= line && line <= candidate.to);
  if (!run) return undefined;
  return { line: run.line + line - run.from, column: line === run.from ? column + run.column : column };
}

/**
 * A stack from the frame with its document's lines named in the definition: `definition:107:48`
 * for the definition's own, `anfra-sdk` for the SDK's, which the host put in the document.
 */
export function definitionStack (stack: string, lines: DefinitionLines): string {
  return stack.replace(/about:srcdoc:(\d+):(\d+)/g, (_, line: string, column: string) => {
    const at = definitionPosition(lines, Number(line), Number(column));
    return at ? `definition:${at.line}:${at.column}` : 'anfra-sdk';
  });
}
