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

async function open (page: Page, files: Record<string, string>, hash = '') {
  folder = repo(db.port, files);
  server = await startServe({ repo: folder.dir });
  await page.goto(`${server.url}/${hash}`);
}

const tree = (page: Page) => page.getByRole('navigation', { name: 'Data Apps' });
const appLink = (page: Page, path: string) => tree(page).locator(`[data-app="${path}"]`);

test('lists every Data App under apps/, labelled by its <title>', async ({ page }) => {
  await open(page, {
    'apps/overview.html': dataAppHtml('Sales Overview'),
    'apps/sales/by-region.html': dataAppHtml('Revenue by Region'),
    'apps/sales/deep/funnel.html': dataAppHtml('Funnel &amp; Drop-off'),
    'apps/untitled.html': dataAppHtml(),
  });

  await expect(appLink(page, 'overview.html')).toHaveText('Sales Overview');
  await expect(appLink(page, 'sales/by-region.html')).toHaveText('Revenue by Region');
  await expect(appLink(page, 'sales/deep/funnel.html')).toHaveText('Funnel & Drop-off');
  // No <title>: fall back to the file name.
  await expect(appLink(page, 'untitled.html')).toHaveText('untitled.html');
});

test('shows nested folders as a tree that collapses and expands', async ({ page }) => {
  await open(page, {
    'apps/sales/by-region.html': dataAppHtml('Revenue by Region'),
    'apps/sales/deep/funnel.html': dataAppHtml('Funnel'),
  });

  const sales = tree(page).locator('[data-folder="sales"]');
  await expect(sales).toHaveAttribute('aria-expanded', 'true');
  await expect(tree(page).locator('[data-folder="sales/deep"]')).toBeVisible();

  await sales.click();
  await expect(sales).toHaveAttribute('aria-expanded', 'false');
  await expect(appLink(page, 'sales/by-region.html')).toHaveCount(0);
  await expect(appLink(page, 'sales/deep/funnel.html')).toHaveCount(0);

  await sales.click();
  await expect(appLink(page, 'sales/deep/funnel.html')).toBeVisible();
});

test('hides folders that hold no Data Apps, and files that are not HTML', async ({ page }) => {
  await open(page, {
    'apps/overview.html': dataAppHtml('Overview'),
    'apps/empty/notes.txt': 'not a data app',
    'apps/empty/deeper/also-empty/.keep': '',
    'apps/readme.md': '# not a data app',
  });

  await expect(appLink(page, 'overview.html')).toBeVisible();
  await expect(tree(page).locator('[data-folder]')).toHaveCount(0);
  await expect(tree(page).locator('[data-app]')).toHaveCount(1);
});

test('shows an empty state when there are no Data Apps', async ({ page }) => {
  await open(page, {});

  await expect(page.getByTestId('empty-state')).toContainText('No Data Apps yet');
});

test('puts the selected Data App in the URL path, and restores it on reload', async ({ page }) => {
  await open(page, {
    'apps/overview.html': dataAppHtml('Sales Overview'),
    'apps/sales/by-region.html': dataAppHtml('Revenue by Region'),
  });

  await appLink(page, 'sales/by-region.html').click();
  await expect(page).toHaveURL(/\/sales\/by-region$/);
  await expect(page.getByTestId('viewer-title')).toHaveText('Revenue by Region');
  await expect(appLink(page, 'sales/by-region.html')).toHaveAttribute('aria-current', 'page');

  await page.reload();
  await expect(page.getByTestId('viewer-title')).toHaveText('Revenue by Region');
  await expect(appLink(page, 'sales/by-region.html')).toHaveAttribute('aria-current', 'page');
});

test('opens the Data App a shared link names', async ({ page }) => {
  await open(page, {
    'apps/overview.html': dataAppHtml('Sales Overview'),
    'apps/sales/by-region.html': dataAppHtml('Revenue by Region'),
  }, 'overview');

  await expect(page.getByTestId('viewer-title')).toHaveText('Sales Overview');
});

test('says so when a link names a Data App that does not exist', async ({ page }) => {
  await open(page, { 'apps/overview.html': dataAppHtml('Sales Overview') }, 'gone');

  await expect(page.getByTestId('not-found')).toContainText('gone');
});

test('has no URL for a folder, nor for a Data App with its .html', async ({ page }) => {
  await open(page, { 'apps/sales/by-region.html': dataAppHtml('Revenue by Region') }, 'sales');
  await expect(page.getByTestId('not-found')).toContainText('sales');

  await page.goto(`${server.url}/sales/by-region.html`);
  await expect(page.getByTestId('not-found')).toBeVisible();
});

test('leaves out a Data App whose URL would be inside the reserved namespace', async ({ page, request }) => {
  await open(page, {
    'apps/overview.html': dataAppHtml('Sales Overview'),
    'apps/_anfra/shadow.html': dataAppHtml('Shadow'),
    'apps/_anfra.html': dataAppHtml('Shadow'),
  });

  await expect(appLink(page, 'overview.html')).toBeVisible();
  expect(await (await request.get(`${server.url}/_anfra/api/apps`)).json()).toHaveLength(1);
  expect((await request.get(`${server.url}/_anfra/nothing`)).status()).toBe(404);
});
