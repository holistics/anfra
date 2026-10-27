/**
 * A small tokenizer for AQL, SQL and the generated declaration code (JS, with AQL inside its
 * template literals). It returns plain tokens; the UI turns them into text, never into markup.
 */

export type Lang = 'aql' | 'sql' | 'js';
export type TokenKind = 'kw' | 'str' | 'num' | 'com' | 'fn';
export interface Token {
  text: string;
  kind?: TokenKind;
}

const words = (list: string) => new Set(list.split(' '));
const KEYWORDS: Record<Lang, Set<string>> = {
  aql: words(
    'explore dimensions measures sorts filters where having limit offset desc asc and or not in is null true false as with by order group',
  ),
  sql: words(
    'select from where group by order having limit offset join left right inner outer full cross on as and or not in is null like between case when then else end distinct union all with over partition asc desc true false cast count sum avg min max',
  ),
  js: words('const let var true false null undefined'),
};

const TOKEN =
  /(--[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|('(?:[^'\\]|\\.|'')*'|"(?:[^"\\]|\\.)*"|`[^`]*`)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_]\w*)(\s*\()?/g;

export function tokenize (text: string, lang: Lang): Token[] {
  const out: Token[] = [];
  let last = 0;
  const plain = (to: number) => {
    if (to > last) out.push({ text: text.slice(last, to) });
  };
  for (const m of text.matchAll(TOKEN)) {
    const i = m.index;
    let kind: TokenKind | undefined;
    let length = m[0].length;
    if (m[1]) kind = 'com';
    else if (m[2] && lang === 'js' && m[2].startsWith('`')) {
      // An AQL template literal: its contents are AQL.
      plain(i);
      out.push({ text: '`' }, ...tokenize(m[2].slice(1, -1), 'aql'), { text: '`' });
      last = i + length;
      continue;
    } else if (m[2]) kind = 'str';
    else if (m[3]) kind = 'num';
    else if (m[4] && m[5]) {
      kind = 'fn';
      length = m[4].length;
    } else if (m[4] && KEYWORDS[lang].has(m[4].toLowerCase())) kind = 'kw';
    if (!kind) continue;
    plain(i);
    out.push({ text: text.slice(i, i + length), kind });
    last = i + length;
  }
  plain(text.length);
  return out;
}
