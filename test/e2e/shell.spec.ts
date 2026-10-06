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

async function open (page: Page, hash = '') {
  folder = repo(db.port, {
    'apps/overview.html': dataAppHtml('Sales Overview'),
    'apps/sales/by-region.html': dataAppHtml('Revenue by Region'),
    'apps/sales/deep/funnel.html': dataAppHtml('Funnel'),
  });
  server = await startServe({ repo: folder.dir });
  await page.goto(`${server.url}/${hash}`);
}

const tree = (page: Page) => page.getByRole('navigation', { name: 'Data Apps' });
const appLink = (page: Page, path: string) => tree(page).locator(`[data-app="${path}"]`);
const search = (page: Page) => page.getByTestId('search');

test('names the sidebar after the Repo', async ({ page }) => {
  await open(page);
  await expect(page.getByTestId('brand')).toHaveText(folder.dir.split('/').pop() as string);
});

test('filters the tree by label or path, expanding folders, and restores it when cleared', async ({ page }) => {
  await open(page);
  await tree(page).locator('[data-folder="sales"]').click();
  await expect(appLink(page, 'sales/by-region.html')).toHaveCount(0);

  await search(page).fill('funnel');
  await expect(appLink(page, 'sales/deep/funnel.html')).toBeVisible();
  await expect(appLink(page, 'overview.html')).toHaveCount(0);

  await search(page).fill('nothing-like-this');
  await expect(page.getByTestId('no-matches')).toBeVisible();

  await search(page).press('Escape');
  await expect(search(page)).toHaveValue('');
  await expect(appLink(page, 'overview.html')).toBeVisible();
  // The reader's collapsed folder is still collapsed.
  await expect(appLink(page, 'sales/by-region.html')).toHaveCount(0);
});

test('focuses the search with /', async ({ page }) => {
  await open(page);
  await page.locator('body').press('/');
  await expect(search(page)).toBeFocused();
});

test('hides and shows the sidebar', async ({ page }) => {
  await open(page);
  await expect(appLink(page, 'overview.html')).toBeVisible();

  await page.getByTestId('sidebar-toggle').click();
  await expect(appLink(page, 'overview.html')).toBeHidden();

  await page.getByTestId('sidebar-toggle').click();
  await expect(appLink(page, 'overview.html')).toBeVisible();
});

test('shows the path as a breadcrumb whose folders reveal themselves in the tree', async ({ page }) => {
  await open(page, 'sales/deep/funnel');
  await expect(page.getByTestId('viewer-title')).toHaveText('Funnel');
  await expect(page.getByTestId('crumb')).toHaveText(['sales', 'deep']);

  await tree(page).locator('[data-folder="sales/deep"]').click();
  await expect(appLink(page, 'sales/deep/funnel.html')).toHaveCount(0);

  await page.locator('[data-crumb="sales/deep"]').click();
  await expect(appLink(page, 'sales/deep/funnel.html')).toBeVisible();
});

test('shows a loading bar until the Data App has loaded, and reloads it on demand', async ({ page }) => {
  await open(page, 'overview');
  await expect(page.getByTestId('data-app-frame')).toBeVisible();
  await expect(page.getByTestId('loading')).toHaveCount(0);

  await page.getByTestId('reload').click();
  await expect(page.getByTestId('loading')).toHaveCount(0);
  await expect(page.getByTestId('data-app-frame')).toBeVisible();
});

test('collapses the sidebar into an overlay on narrow screens', async ({ page }) => {
  await page.setViewportSize({ width: 600, height: 800 });
  await open(page);
  await expect(appLink(page, 'overview.html')).toBeHidden();

  await page.getByTestId('sidebar-toggle').click();
  await appLink(page, 'overview.html').click();
  await expect(page.getByTestId('viewer-title')).toHaveText('Sales Overview');
  await expect(appLink(page, 'overview.html')).toBeHidden();
});

test('keeps the search box inside the sidebar', async ({ page }) => {
  await open(page);
  const box = (await search(page).locator('xpath=..').boundingBox())!;
  const sidebar = (await tree(page).boundingBox())!;
  expect(box.x + box.width).toBeLessThanOrEqual(sidebar.x + sidebar.width - 8);
});

test('switches between light and dark, and remembers the choice', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await open(page);
  const html = page.locator('html');
  await expect(html).toHaveAttribute('data-theme', 'light');

  await page.getByTestId('theme-toggle').click();
  await expect(html).toHaveAttribute('data-theme', 'dark');

  await page.reload();
  await expect(html).toHaveAttribute('data-theme', 'dark');
});
