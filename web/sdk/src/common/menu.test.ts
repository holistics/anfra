import { describe, expect, it } from 'vitest';
import { checkTarget } from './menu';

const rows = {
  kind: 'rows', app: 0, query: 'revenue', rows: [{ region: 'APAC' }], fields: ['region'], conditions: [],
};

describe('checkTarget', () => {
  it('keeps a rows target\'s point as the frame sent it', () => {
    expect(checkTarget({ ...rows, at: { x: 20, y: 30.5 } })).toEqual({ ...rows, at: { x: 20, y: 30.5 } });
    expect(checkTarget(rows)).toEqual(rows);
  });

  it('refuses a rows target whose point isn\'t one', () => {
    for (const at of [null, 'here', { x: 1 }, { x: '1', y: 2 }, { x: Number.NaN, y: 2 }, [1, 2]]) {
      expect(checkTarget({ ...rows, at })).toBeUndefined();
    }
  });

  it('drops what a point carries besides its coordinates', () => {
    expect(checkTarget({ ...rows, at: { x: 1, y: 2, note: 'extra' } })).toEqual({ ...rows, at: { x: 1, y: 2 } });
  });
});
