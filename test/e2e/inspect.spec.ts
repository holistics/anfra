import { test, expect, type Page } from '@playwright/test';
import {
  dataAppHtml, repo, fakeDatabase, startServe, type Repo, type Server,
} from '../serve';

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

const AQL = `explore {
  dimensions { country: customers.country }
  measures { revenue: revenue }
}`;

const revenueApp = `<!doctype html>
<html><head><meta charset="utf-8"><title>Revenue</title></head>
<body>
  <p id="state"></p>
  <script>
    const app = Anfra.createApp({ title: 'Revenue' });
    const query = app.createQuery('revenue', { dataset: 'ecommerce', aql: ${JSON.stringify(AQL)} });
    const country = app.createFilter('country', { field: 'customers.country', dataset: 'ecommerce' });
    app.mapControl(country, query, { field: 'customers.country' });
    country.setCondition({ operator: 'is', values: ['Vietnam'] });
    app.subscribe(() => { document.getElementById('state').textContent = query.state; });
    app.execute().then(() => country.setCondition({ operator: 'is', values: ['Japan'] }));
  </script>
</body></html>
`;

async function open (page: Page, files: Record<string, string>, hash = 'revenue') {
  folder = repo(db.port, files);
  server = await startServe({ repo: folder.dir });
  await page.goto(`${server.url}/${hash}`);
}

const panel = (page: Page) => page.getByTestId('inspect-panel');
const toggle = (page: Page) => page.getByTestId('inspect');

test('shows the running app\'s queries and controls, read from the SDK', async ({ page }) => {
  await open(page, { 'apps/revenue.html': revenueApp });
  await expect(panel(page)).toHaveCount(0);

  await toggle(page).click();
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'true');

  const query = page.getByTestId('inspect-query-revenue');
  await expect(query.getByTestId('query-state')).toHaveText('success');
  await expect(query).toContainText('2'); // rows
  await expect(query).toContainText('Executed AQL');

  const control = page.getByTestId('inspect-control-country');
  await expect(control.getByTestId('control-condition')).toHaveText('is Japan');
  await expect(control.getByTestId('control-applied')).toHaveText('is Vietnam');
  await expect(control).toContainText('unapplied');
});

test('copies the executed AQL', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await open(page, { 'apps/revenue.html': revenueApp });
  await toggle(page).click();

  await page.getByTestId('copy-executedAql').click();
  await expect(page.getByTestId('copy-executedAql')).toHaveText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toContain('explore');
});

test('says so when the Data App never creates an SDK app', async ({ page }) => {
  await open(page, { 'apps/plain.html': dataAppHtml('Plain') }, 'plain');
  await toggle(page).click();
  await expect(page.getByTestId('inspect-none')).toBeVisible();
});

test('stays open across Reload, closes when another app is picked, and closes on Escape', async ({ page }) => {
  await open(page, { 'apps/revenue.html': revenueApp, 'apps/plain.html': dataAppHtml('Plain') });
  await toggle(page).click();
  await expect(page.getByTestId('inspect-query-revenue')).toBeVisible();

  await page.getByTestId('reload').click();
  await expect(panel(page)).toBeVisible();
  await expect(page.getByTestId('inspect-query-revenue')).toBeVisible();

  await page.locator('[data-app="plain.html"]').click();
  await expect(panel(page)).toHaveCount(0);
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'false');

  await toggle(page).click();
  await expect(panel(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(panel(page)).toHaveCount(0);
});

test('disables Inspect until an app is selected', async ({ page }) => {
  await open(page, { 'apps/revenue.html': revenueApp }, '');
  await expect(toggle(page)).toBeDisabled();
});

test('remembers the width the panel was dragged to', async ({ page }) => {
  await open(page, { 'apps/revenue.html': revenueApp });
  await toggle(page).click();
  const dock = page.locator('.inspect-dock');
  const before = (await dock.boundingBox())!;

  const handle = (await page.locator('.resize-handle').boundingBox())!;
  await page.mouse.move(handle.x + 3, handle.y + 200);
  await page.mouse.down();
  await page.mouse.move(handle.x - 97, handle.y + 200, { steps: 5 });
  await page.mouse.up();
  const after = (await dock.boundingBox())!;
  expect(after.width).toBeGreaterThan(before.width + 80);

  await page.reload();
  await toggle(page).click();
  expect(Math.round((await dock.boundingBox())!.width)).toBe(Math.round(after.width));
});
