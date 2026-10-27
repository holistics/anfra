/**
 * An app's declaration written back out as the JS that would declare it, to paste to an agent or
 * diff against the source.
 */
import type { EntityRef, InspectedApp } from '../../../common/types';
import { controlMappings, crossFilters } from './checks';

/**
 * Strips the indentation the code was written with. The first line is usually trimmed already
 * (`aql: \`explore {`), so the common indent comes from the lines after it.
 */
export function dedent (text: string): string {
  const lines = text.replace(/\t/g, '  ').trim().split('\n');
  const indentOf = (line: string) => line.length - line.trimStart().length;
  const rest = lines.slice(1).filter((line) => line.trim());
  const indent = rest.length ? Math.min(...rest.map(indentOf)) : 0;
  return lines
    .map((line, i) => (i ? line.slice(Math.min(indent, indentOf(line))) : line).trimEnd())
    .join('\n');
}

const json = (value: unknown) => JSON.stringify(value);

/** Text as it can sit in a template literal: its backticks, `${` and backslashes escaped. */
const templateText = (text: string) => text.replace(/[`\\]|\$\{/g, (match) => `\\${match}`);

const RESERVED = new Set(
  'break case catch class const continue debugger default delete do else enum export extends false finally for function if import in instanceof let new null return static super switch this throw true try typeof var void while with yield await'.split(
    ' ',
  ),
);

/** A name as JS can use it as a variable: `top-products` → `topProducts`. */
function identifier (name: string): string {
  const id = name.replace(/[^\w$]+(.)?/g, (_, next: string | undefined) => next?.toUpperCase() ?? '');
  return /^[A-Za-z_$]/.test(id) && !RESERVED.has(id) ? id : `_${id}`;
}

/**
 * Each entity's variable, keyed `<kind>:<name>`: its name made an identifier, numbered where two
 * would be the same. Queries, filters and date drills share one scope, as they would in the source.
 */
function variables (refs: EntityRef[]): Map<string, string> {
  const taken = new Set<string>();
  const byRef = new Map<string, string>();
  for (const { kind, name } of refs) {
    const base = identifier(name);
    let variable = base;
    for (let n = 2; taken.has(variable); n++) variable = `${base}${n}`;
    taken.add(variable);
    byRef.set(`${kind}:${name}`, variable);
  }
  return byRef;
}

export function declarationCode (app: InspectedApp): string {
  const { queries, filters, dateDrills } = app.declaration;
  const blocks: string[][] = [[], [], [], []];
  const names = variables([
    ...Object.keys(queries).map((name) => ({ kind: 'query' as const, name })),
    ...Object.keys(filters).map((name) => ({ kind: 'filter' as const, name })),
    ...Object.keys(dateDrills).map((name) => ({ kind: 'dateDrill' as const, name })),
  ]);
  // A mapping to an entity the app doesn't declare still reads as its name.
  const variable = ({ kind, name }: EntityRef) => names.get(`${kind}:${name}`) ?? identifier(name);

  for (const [name, query] of Object.entries(queries)) {
    const aql = templateText(dedent(query.aql))
      .split('\n')
      .map((line) => (line ? `    ${line}` : line))
      .join('\n');
    const pageSize = query.pageSize ? `\n  pageSize: ${query.pageSize},` : '';
    blocks[0]?.push(
      `const ${variable({ kind: 'query', name })} = app.createQuery(${json(name)}, {\n  dataset: ${json(query.dataset)},${pageSize}\n  aql: \`\n${aql}\n  \`,\n});`,
    );
  }
  for (const [name, filter] of Object.entries(filters)) {
    blocks[1]?.push(
      `const ${variable({ kind: 'filter', name })} = app.createFilter(${json(name)}, ${json(filter)});`,
    );
  }
  for (const [name, drill] of Object.entries(dateDrills)) {
    blocks[1]?.push(
      `const ${variable({ kind: 'dateDrill', name })} = app.createDateDrill(${json(name)}, ${json(drill)});`,
    );
  }
  for (const m of controlMappings(app)) {
    const options = m.aggregation ? { field: m.field, aggregation: m.aggregation } : { field: m.field };
    blocks[2]?.push(`app.mapControl(${variable(m.from)}, ${variable(m.to)}, ${json(options)});`);
  }
  for (const c of crossFilters(app)) {
    blocks[3]?.push(`app.mapCrossFilter(${variable(c.from)}, ${variable(c.to)});`);
  }
  return blocks
    .filter((lines) => lines.length)
    .map((lines) => lines.join('\n'))
    .join('\n\n');
}
