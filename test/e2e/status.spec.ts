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

const brokenAml = {
  __status: 'invalid',
  __data: {
    compileErrors: [{
      filePath: '/models/orders.model.aml', message: 'Unexpected token `}`', row: 7, col: 3,
    }],
    reports: [
      { filePath: 'datasets/ecommerce.dataset.aml', severity: 'error', message: 'Relationship refers to unknown field orders.nope' },
      { filePath: 'datasets/ecommerce.dataset.aml', severity: 'warning', message: 'Dataset has no description' },
    ],
  },
};

async function open (page: Page, anfra: Record<string, unknown> = {}) {
  folder = repo(db.port, {
    'apps/overview.html': dataAppHtml('Overview'),
    'models/orders.model.aml': 'Model orders { }\n',
  });
  server = await startServe({ repo: folder.dir, anfra });
  await page.goto(`${server.url}/`);
}

const badge = (page: Page) => page.getByTestId('aml-badge');
const banner = (page: Page) => page.getByTestId('aml-banner');
const status = (page: Page) => page.getByTestId('anfra-status');

test('shows no problems badge while the AML is valid', async ({ page }) => {
  await open(page);

  await expect(status(page)).toHaveAttribute('data-status', 'up');
  await expect(badge(page)).toHaveCount(0);
});

test('shows the AML\'s compile errors and error findings in a popover, but not warnings', async ({ page }) => {
  await open(page, { validate: brokenAml });

  await expect(badge(page)).toContainText('2 problems');
  await expect(banner(page)).toHaveCount(0);
  await badge(page).click();
  await expect(banner(page)).toContainText('2 problems');
  await expect(banner(page).locator('li')).toHaveText([
    'models/orders.model.aml:7:3 Unexpected token `}`',
    'datasets/ecommerce.dataset.aml Relationship refers to unknown field orders.nope',
  ]);

  await page.keyboard.press('Escape');
  await expect(banner(page)).toHaveCount(0);
});

test('updates the badge when the AML changes: appears when broken, clears when fixed', async ({ page }) => {
  await open(page);
  await expect(status(page)).toHaveAttribute('data-status', 'up');
  await expect(badge(page)).toHaveCount(0);

  server.setAnfra({ validate: brokenAml });
  folder.write('models/orders.model.aml', 'Model orders { \n');
  await expect(badge(page)).toContainText('2 problems');

  server.setAnfra({});
  folder.write('models/orders.model.aml', 'Model orders { }\n');
  await expect(badge(page)).toHaveCount(0);
});

test('turns the status dot red when anfra\'s sidecars don\'t answer', async ({ page }) => {
  await open(page);
  await expect(status(page)).toHaveAttribute('data-status', 'up');
  await expect(status(page)).toHaveText('anfra is running');

  server.setAnfra({ status: { __error: 'anfra-node is down' } });
  await page.reload();

  await expect(status(page)).toHaveAttribute('data-status', 'down');
  await expect(status(page)).toHaveText('anfra is down');
});

test('says so when the anfra server itself goes away', async ({ page }) => {
  await open(page);
  await expect(status(page)).toHaveAttribute('data-status', 'up');

  await server.stop();

  await expect(status(page)).toHaveAttribute('data-status', 'unreachable');
});
