import { describe, expect, it } from 'vitest';
import { bareNames, exploreFilters, preambleOf } from './aqlText';

describe('preambleOf', () => {
  it('is what comes before the explore, and nothing when the explore comes first', () => {
    expect(preambleOf('explore { measures { n: count(orders.id) } }')).toBe('');
    expect(preambleOf('dimension orders.kind = case(when: orders.amount > 100, then: "Big", else: "Small");\nexplore { }'))
      .toBe('dimension orders.kind = case(when: orders.amount > 100, then: "Big", else: "Small");');
  });

  it('is not fooled by "explore {" in a string, a comment or a longer name', () => {
    expect(preambleOf('// explore { a }\ndimension orders.x = "explore {";\nexplore { }')).toBe('// explore { a }\ndimension orders.x = "explore {";');
    expect(preambleOf('my_explore { }')).toBe('');
  });
});

describe('exploreFilters', () => {
  it('is each condition of the explore\'s own filters, as written', () => {
    expect(exploreFilters(`
      explore {
        dimensions { status: orders.status }
        measures { n: count(orders.id) }
        filters {
          orders.status in ['paid', 'shipped'],
          orders.created_at matches @(last 2 years),
        }
        sorts { n DESC }
      }
    `)).toEqual(["orders.status in ['paid', 'shipped']", 'orders.created_at matches @(last 2 years)']);
  });

  it('takes comments out, and keeps a comma or brace inside a string or a call', () => {
    expect(exploreFilters(`explore {
      filters {
        // the paid ones, {really}
        orders.note == 'a, b }', // trailing
        coalesce(orders.amount, 0) > 1
      }
    }`)).toEqual(["orders.note == 'a, b }'", 'coalesce(orders.amount, 0) > 1']);
  });

  it('is empty without a filters block, and ignores one nested in another block', () => {
    expect(exploreFilters('explore { measures { n: count(orders.id) } }')).toEqual([]);
    expect(exploreFilters('explore { dimensions { filters { orders.a == 1 } } }')).toEqual([]);
    expect(exploreFilters('orders | count(orders.id)')).toEqual([]);
  });
});

describe('bareNames', () => {
  it('is the names that are neither a model, a field nor a function, outside strings', () => {
    expect([...bareNames("st == 'paid' and orders.status != \"x\" and sum(orders.amount) > n")]).toEqual(['st', 'and', 'n']);
    expect([...bareNames('orders.created_at matches @(last quarter)')]).toEqual(['matches']);
  });
});
