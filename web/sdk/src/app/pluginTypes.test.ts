import { describe, expectTypeOf, it } from 'vitest';
import type { ExportResult, Rendering } from '../common/types';
import type { Control } from './controls';
import type { Query } from './query';
import type { Sdk } from './sdk';

// Types only: `pnpm typecheck` is what checks these.
describe('the plugin registry', () => {
  it('adds each plugin\'s members, optional, where it said', () => {
    expectTypeOf<Sdk['render']>().toEqualTypeOf<((request: Parameters<NonNullable<Sdk['render']>>[0]) => Promise<Rendering>) | undefined>();
    expectTypeOf<Query['export']>().toMatchTypeOf<((...args: never[]) => Promise<ExportResult>) | undefined>();
    expectTypeOf<Sdk>().not.toHaveProperty('export');
    expectTypeOf<Sdk>().not.toHaveProperty('inspect');
    expectTypeOf<Query>().not.toHaveProperty('render');
    expectTypeOf<Control>().not.toHaveProperty('export');
  });

  it('narrows a member when the member itself is checked', () => {
    const narrowed = (anfra: Sdk, query: Query) => {
      if (anfra.render) expectTypeOf(anfra.render).toBeFunction();
      if (query.export) expectTypeOf(query.export).toBeFunction();
    };
    expectTypeOf(narrowed).toBeFunction();
  });

  it('completes plugin names, and takes any other', () => {
    expectTypeOf<Sdk['plugins']['has']>().parameter(0).toMatchTypeOf<string>();
    expectTypeOf<'export'>().toMatchTypeOf<Parameters<Sdk['plugins']['has']>[0]>();
    expectTypeOf<'notes'>().toMatchTypeOf<Parameters<Sdk['plugins']['has']>[0]>();
  });
});
