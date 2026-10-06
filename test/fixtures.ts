/**
 * Canned anfra answers for the fake: a small `ecommerce` Dataset in anfra's catalog shape, and a
 * query result. Tests override any key.
 */

const entity = (type: string, properties: Record<string, unknown>) => ({ type, source: 'aml', properties });
const found = (entities: unknown[]) => ({ query: '', total: entities.length, entities });

export const catalog = {
  'search:type:aml.dataset': found([
    entity('aml.dataset', { name: 'ecommerce', fqn: 'ecommerce', label: 'E-commerce' }),
  ]),
  'search:type:aml.dataset_model_alias': found([
    entity('aml.dataset_model_alias', { dataset_fqn: 'ecommerce', model_fqn: 'customers' }),
    entity('aml.dataset_model_alias', { dataset_fqn: 'ecommerce', model_fqn: 'orders' }),
  ]),
  'search:type:aml.model': found([
    entity('aml.model', { fqn: 'customers', name: 'customers', label: 'Customers' }),
    entity('aml.model', { fqn: 'orders', name: 'orders', label: 'Orders' }),
  ]),
  'search:type:aml.dimension': found([
    entity('aml.dimension', { parent_fqn: 'customers', name: 'country', label: 'Country', data_type: 'text' }),
    entity('aml.dimension', { parent_fqn: 'customers', name: 'name', label: 'Name', data_type: 'text' }),
    entity('aml.dimension', { parent_fqn: 'orders', name: 'id', label: 'ID', data_type: 'number' }),
    entity('aml.dimension', { parent_fqn: 'orders', name: 'ordered_at', label: 'Ordered At', data_type: 'date' }),
  ]),
  'search:type:aml.measure': found([]),
  'search:type:aml.metric': found([
    entity('aml.metric', { dataset_name: 'ecommerce', name: 'revenue', label: 'Revenue', data_type: 'number' }),
  ]),
};

/** What `anfra query` answers for a two-row revenue-by-country query. */
export const revenueByCountry = {
  sql: 'SELECT "customers"."country" AS "country", ... FROM public.customers',
  aql: "explore {\n  dimensions { country: customers.country }\n  measures { revenue: revenue }\n}",
  columns: [
    {
      name: 'country', fieldName: 'country', modelId: 'customers', label: 'Country', adhoc: false, isMeasure: false,
    },
    {
      name: 'revenue', fieldName: 'revenue', label: 'Revenue', adhoc: false, isMeasure: true,
    },
  ],
  result: {
    fields: ['country', 'revenue'],
    records: [['Vietnam', 1200.5], ['Japan', 980]],
  },
};

export const defaultResponses = {
  ingest: 'ingest complete',
  validate: { compileErrors: [], reports: [] },
  ...catalog,
  query: revenueByCountry,
};
