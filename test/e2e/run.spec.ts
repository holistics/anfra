import { test, expect, type Page } from '@playwright/test';
import {
  repo, fakeDatabase, startServe, type Repo, type Server,
} from '../serve';
import { revenueByCountry } from '../fixtures';

let db: Awaited<ReturnType<typeof fakeDatabase>>;
let folder: Repo;
let server: Server;

test.beforeEach(async () => {
  db = await fakeDatabase();
});

test.afterEach(async () => {
  await server?.stop();
  folder?.remove();
  await db.close();
});

const AQL = 'explore { dimensions { country: customers.country } measures { revenue: revenue } }';

/** A Data App that runs one query and renders its state, rows and a few environment facts. */
const revenueApp = (extra = '') => `<!doctype html>
<html><head><meta charset="utf-8"><title>Revenue</title></head>
<body>
  <p id="state"></p>
  <ul id="rows"></ul>
  <pre id="env"></pre>
  <script>
    const app = Anfra.createApp({ title: 'Revenue' });
    const query = app.createQuery('revenue', { dataset: 'ecommerce', aql: ${JSON.stringify(AQL)} });
    app.subscribe(() => {
      document.getElementById('state').textContent = query.state === 'error'
        ? query.error.name + ': ' + query.error.message
        : query.state;
      document.getElementById('rows').replaceChildren(...(query.result?.rows ?? []).map((row) => {
        const li = document.createElement('li');
        li.textContent = row.country + ' ' + row.revenue;
        return li;
      }));
    });
    document.getElementById('env').textContent = JSON.stringify({
      user: Anfra.user.name,
      datasets: Object.keys(Anfra.datasets),
      models: Anfra.datasets.ecommerce.data_models.map((m) => m.name),
      metrics: Anfra.datasets.ecommerce.metrics.map((m) => m.name),
      countryType: Anfra.datasets.ecommerce.data_models
        .find((m) => m.name === 'customers').fields.find((f) => f.name === 'country').type,
    });
    ${extra}
    app.execute();
  </script>
</body></html>
`;

async function openApp (page: Page, html: string, anfra: Record<string, unknown> = {}) {
  folder = repo(db.port, { 'apps/revenue.html': html });
  server = await startServe({ repo: folder.dir, anfra });
  await page.goto(`${server.url}/revenue`);
  return page.frameLocator('[data-testid="data-app-frame"]');
}

test('runs a Data App in a sandboxed frame, with rows from anfra', async ({ page }) => {
  const app = await openApp(page, revenueApp());

  await expect(page.getByTestId('data-app-frame')).toHaveAttribute('sandbox', 'allow-scripts');
  await expect(app.locator('#state')).toHaveText('success');
  await expect(app.locator('#rows li')).toHaveText(['Vietnam 1200.5', 'Japan 980']);
});

test('provisions the SDK before author code, with every Dataset and the local user', async ({ page }) => {
  const app = await openApp(page, revenueApp());

  await expect(app.locator('#env')).not.toBeEmpty();
  const env = JSON.parse(await app.locator('#env').textContent() ?? '{}');
  expect(env).toEqual({
    user: 'Local Developer',
    datasets: ['ecommerce'],
    models: ['customers', 'orders'],
    metrics: ['revenue'],
    countryType: 'text',
  });
});

test('sends anfra the AQL unchanged, with the Query Input as structured data and no paging', async ({ page }) => {
  const app = await openApp(page, revenueApp());
  await expect(app.locator('#state')).toHaveText('success');

  const [call] = server.anfraCalls('query');
  expect(call).toEqual({
    dataset: 'ecommerce',
    aql: AQL,
    input: {
      filters: [], conditions: [], sorts: [], dateDrills: [],
    },
  });
});

test('pages a query that declares a page size', async ({ page }) => {
  const app = await openApp(page, revenueApp().replace(
    `aql: ${JSON.stringify(AQL)} }`,
    `aql: ${JSON.stringify(AQL)}, pageSize: 50 }`,
  ));
  await expect(app.locator('#state')).toHaveText('success');

  expect(server.anfraCalls('query').at(-1)).toMatchObject({ page: 1, 'page-size': 50 });
});

test('forwards a reader\'s filters, sort, date drill and timezone to anfra', async ({ page }) => {
  const html = revenueApp().replace(
    "const app = Anfra.createApp({ title: 'Revenue' });",
    "const app = Anfra.createApp({ title: 'Revenue', timezone: 'Asia/Tokyo' });",
  ).replace('app.execute();\n  </script>', `
    const country = app.createFilter('country', { dataset: 'ecommerce', field: 'customers.country' });
    const grain = app.createDateDrill('grain', { default: 'quarter' });
    app.mapControl(country, query, { field: 'customers.country' });
    app.mapControl(grain, query, { field: 'orders.ordered_at' });
    country.setCondition({ operator: 'is', values: ['Vietnam'] });
    query.setSort([{ field: 'revenue', direction: 'desc' }]);
    app.execute();
  </script>`);
  const app = await openApp(page, html);
  await expect(app.locator('#state')).toHaveText('success');

  expect(server.anfraCalls('query').at(-1)).toEqual({
    dataset: 'ecommerce',
    aql: AQL,
    input: {
      filters: [{ field: 'customers.country', operator: 'is', values: ['Vietnam'] }],
      conditions: [],
      sorts: [{ field: 'revenue', direction: 'desc' }],
      dateDrills: [{ field: 'orders.ordered_at', grain: 'quarter' }],
    },
    timezone: 'Asia/Tokyo',
  });
});

test('hands the Data App the Executed AQL and column metadata anfra returned', async ({ page }) => {
  const app = await openApp(page, revenueApp(`
    app.subscribe(() => {
      if (query.state !== 'success') return;
      document.getElementById('env').dataset.result = JSON.stringify({
        executedAql: query.result.debug.executedAql,
        executedAtIsDate: query.result.debug.executedAt instanceof Date,
        columns: query.result.columns,
      });
    });
  `));
  await expect(app.locator('#env[data-result]')).toHaveCount(1);

  const result = JSON.parse(await app.locator('#env').getAttribute('data-result') ?? '{}');
  expect(result).toEqual({
    executedAql: revenueByCountry.aql,
    executedAtIsDate: true,
    columns: revenueByCountry.columns,
  });
});

test('surfaces a failed anfra query as a QueryError in the Data App', async ({ page }) => {
  const app = await openApp(page, revenueApp(), {
    query: { __error: 'filters[0].field: Unknown field `customers.nope` in this dataset.' },
  });

  await expect(app.locator('#state')).toHaveText('QueryError: filters[0].field: Unknown field `customers.nope` in this dataset.');
});

test('surfaces AQL that does not type-check as a QueryError with anfra\'s diagnostics', async ({ page }) => {
  const app = await openApp(page, revenueApp(), {
    query: { __status: 'invalid', __data: { diagnostics: [{ message: 'Unknown field customers.nope', line: 1, column: 30 }] } },
  });

  await expect(app.locator('#state')).toHaveText('QueryError: line 1:30: Unknown field customers.nope');
});

test('cancels the anfra call when the Data App aborts a query', async ({ page }) => {
  const app = await openApp(page, revenueApp('setTimeout(() => query.abort(), 300);'), {
    query: { __delayMs: 3000, __data: revenueByCountry },
  });

  await expect.poll(() => server.anfraLog().some((e) => e.event === 'aborted' && e.command === 'query'), { timeout: 5000 })
    .toBe(true);
  // The aborted run never lands.
  await expect(app.locator('#rows li')).toHaveCount(0);
});

test('never lets a Data App file reach outside apps/', async ({ request }) => {
  folder = repo(db.port, { 'apps/revenue.html': revenueApp() });
  server = await startServe({ repo: folder.dir });

  expect((await request.get(`${server.url}/_anfra/data-apps/..%2F.anfra%2Fdata_sources.yml`)).status()).toBe(404);
  expect((await request.get(`${server.url}/_anfra/data-apps/missing.html`)).status()).toBe(404);
});

/** A Data App with one field-backed filter, which loads its options and renders them. */
const suggestionsApp = (field: string, search: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Suggestions</title></head>
<body>
  <ul id="options"></ul><p id="error"></p>
  <script>
    const app = Anfra.createApp();
    const filter = app.createFilter('f', { dataset: 'ecommerce', field: ${JSON.stringify(field)} });
    filter.loadOptions(${JSON.stringify(search)})
      .then((options) => document.getElementById('options').replaceChildren(...options.map((value) => {
        const li = document.createElement('li');
        li.textContent = String(value);
        return li;
      })))
      .catch((err) => { document.getElementById('error').textContent = err.name + ': ' + err.message; });
  </script>
</body></html>
`;

const countryValues = {
  ...revenueByCountry,
  columns: [{
    name: 'value', fieldName: 'country', modelId: 'customers', label: 'Country', adhoc: false, isMeasure: false,
  }],
  result: { fields: ['value'], records: [['Japan'], ['Vietnam'], [null]] },
};

test('offers a field-backed filter the field\'s values, narrowed by what the reader typed', async ({ page }) => {
  folder = repo(db.port, { 'apps/suggest.html': suggestionsApp('customers.country', 'an') });
  server = await startServe({ repo: folder.dir, anfra: { query: countryValues } });
  await page.goto(`${server.url}/suggest`);
  const app = page.frameLocator('[data-testid="data-app-frame"]');

  await expect(app.locator('#options li')).toHaveText(['Japan', 'Vietnam']);
  expect(server.anfraCalls('query').at(-1)).toEqual({
    dataset: 'ecommerce',
    aql: 'explore { dimensions { value: customers.country } }',
    input: {
      filters: [{ field: 'customers.country', operator: 'contains', values: ['an'] }],
      conditions: [],
      sorts: [{ field: 'value', direction: 'asc' }],
      dateDrills: [],
    },
    page: 1,
    'page-size': 100,
  });
});

test('does not narrow a non-text field by search text', async ({ page }) => {
  folder = repo(db.port, { 'apps/suggest.html': suggestionsApp('orders.id', '12') });
  server = await startServe({ repo: folder.dir, anfra: { query: { ...countryValues, result: { fields: ['value'], records: [[12], [120]] } } } });
  await page.goto(`${server.url}/suggest`);
  const app = page.frameLocator('[data-testid="data-app-frame"]');

  await expect(app.locator('#options li')).toHaveText(['12', '120']);
  expect((server.anfraCalls('query').at(-1)?.input as { filters: unknown[] }).filters).toEqual([]);
});

test('refuses suggestions for a field the Dataset does not define', async ({ request }) => {
  folder = repo(db.port, {});
  server = await startServe({ repo: folder.dir });

  const response = await request.post(`${server.url}/_anfra/api/suggestions`, {
    data: {
      dataset: 'ecommerce', model: 'customers', field: 'country } } explore { dimensions { x: orders.id', q: '',
    },
  });
  expect(response.status()).toBe(422);
  expect(server.anfraCalls('query')).toHaveLength(0);
});

