import { describe, expect, it } from 'vitest';
import { DatasetIndex, resolveDataset } from './validation';
import { ValidationError } from './errors';
import { salesDataset } from './testSupport';

describe('DatasetIndex', () => {
  const index = new DatasetIndex(salesDataset);

  it('resolves a model field from a dotted reference', () => {
    expect(index.resolve('orders.amount', 'q')).toMatchObject({
      modelName: 'orders',
      fieldName: 'amount',
      isMetric: false,
      type: 'number',
    });
  });

  it('resolves a bare name as a dataset-level metric', () => {
    expect(index.resolve('aov', 'q')).toMatchObject({ fieldName: 'aov', isMetric: true });
  });

  it('suggests the near miss when a field name is mistyped', () => {
    expect(() => index.resolve('orders.amont', 'revenue')).toThrowError(
      /Unknown field 'orders.amont' on dataset 'sales'\. Did you mean 'orders\.amount'\?/,
    );
  });

  it('names the entity that caused the error, so an agent can locate it', () => {
    try {
      index.resolve('orders.nope', 'revenue');
      expect.unreachable();
    } catch (err) {
      expect(err).toBeInstanceOf(ValidationError);
      expect((err as ValidationError).entity).toBe('revenue');
    }
  });

  it('does not invent a suggestion when nothing is close', () => {
    expect(() => index.resolve('zzzzzzzzzz', 'q')).toThrowError(/^(?!.*Did you mean).*$/s);
  });
});

describe('resolveDataset', () => {
  const datasets = { sales: new DatasetIndex(salesDataset) };

  it('suggests a near-miss dataset uname', () => {
    expect(() => resolveDataset(datasets, 'sale', 'q')).toThrowError(
      /Unknown dataset 'sale'\. Did you mean 'sales'\?/,
    );
  });
});
