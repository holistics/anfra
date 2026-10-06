import * as fs from 'node:fs';
import * as path from 'node:path';
import { test, expect, type Page } from '@playwright/test';
import {
  dataAppHtml, repo, fakeDatabase, startServe, type Repo, type Server,
} from '../serve';
import { catalog } from '../fixtures';

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

/** A Data App that shows a message and the label of its Dataset, as provisioned. */
const labelApp = (message: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Live</title></head>
<body><p id="message">${message}</p><p id="label"></p>
<script>document.getElementById('label').textContent = Anfra.datasets.ecommerce.label;</script>
</body></html>
`;

async function open (page: Page, files: Record<string, string>, hash = '') {
  folder = repo(db.port, files);
  server = await startServe({ repo: folder.dir });
  await page.goto(`${server.url}/${hash}`);
}

const tree = (page: Page) => page.getByRole('navigation', { name: 'Data Apps' });
const frame = (page: Page) => page.frameLocator('[data-testid="data-app-frame"]');

test('adds a new Data App to the tree when its file appears', async ({ page }) => {
  await open(page, { 'apps/first.html': dataAppHtml('First') });
  await expect(tree(page).locator('[data-app="first.html"]')).toBeVisible();

  folder.write('apps/sales/second.html', dataAppHtml('Second'));

  await expect(tree(page).locator('[data-app="sales/second.html"]')).toHaveText('Second');
});

test('drops a Data App from the tree when its file goes, and relabels one whose title changes', async ({ page }) => {
  await open(page, {
    'apps/first.html': dataAppHtml('First'),
    'apps/second.html': dataAppHtml('Second'),
  });
  await expect(tree(page).locator('[data-app]')).toHaveCount(2);

  fs.rmSync(path.join(folder.dir, 'apps/second.html'));
  folder.write('apps/first.html', dataAppHtml('First, renamed'));

  await expect(tree(page).locator('[data-app]')).toHaveCount(1);
  await expect(tree(page).locator('[data-app="first.html"]')).toHaveText('First, renamed');
});

test('reloads the running Data App when its file is saved', async ({ page }) => {
  await open(page, { 'apps/live.html': labelApp('before') }, 'live');
  await expect(frame(page).locator('#message')).toHaveText('before');

  folder.write('apps/live.html', labelApp('after'));

  await expect(frame(page).locator('#message')).toHaveText('after');
});

test('leaves the running Data App alone when another one changes', async ({ page }) => {
  await open(page, {
    'apps/live.html': labelApp('running'),
    'apps/other.html': dataAppHtml('Other'),
  }, 'live');
  await expect(frame(page).locator('#message')).toHaveText('running');
  await frame(page).locator('#message').evaluate((el) => { el.textContent = 'untouched'; });

  folder.write('apps/other.html', dataAppHtml('Other, edited'));

  await expect(tree(page).locator('[data-app="other.html"]')).toHaveText('Other, edited');
  await expect(frame(page).locator('#message')).toHaveText('untouched');
});

test('rebuilds the Datasets and reloads the running Data App when AML changes', async ({ page }) => {
  await open(page, {
    'apps/live.html': labelApp('running'),
    'datasets/ecommerce.dataset.aml': 'Dataset ecommerce { label: "E-commerce" }\n',
  }, 'live');
  await expect(frame(page).locator('#label')).toHaveText('E-commerce');
  const ingestsBefore = server.anfraCalls('ingest').length;

  // What anfra's catalog answers once the AML is re-read.
  server.setAnfra({
    'search:type:aml.dataset': {
      ...catalog['search:type:aml.dataset'],
      entities: [{ type: 'aml.dataset', source: 'aml', properties: { name: 'ecommerce', fqn: 'ecommerce', label: 'Shop' } }],
    },
  });
  folder.write('datasets/ecommerce.dataset.aml', 'Dataset ecommerce { label: "Shop" }\n');

  await expect(frame(page).locator('#label')).toHaveText('Shop');
  expect(server.anfraCalls('ingest').length).toBeGreaterThan(ingestsBefore);
});
