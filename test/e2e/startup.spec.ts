import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { test, expect } from '@playwright/test';
import {
  repo, fakeDatabase, startServe, startServeExpectingFailure, fakeLog,
} from '../serve';

test('serves the repo it runs in, and stops on SIGTERM', async () => {
  const db = await fakeDatabase();
  const folder = repo(db.port);
  const log = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'fake-log-')), 'anfra.jsonl');

  const server = await startServe({ repo: folder.dir, env: { ANFRA_E2E_LOG: log } });
  try {
    const started = fakeLog(log).find((event) => event.event === 'start') as { cwd: string };
    expect(fs.realpathSync(started.cwd)).toBe(fs.realpathSync(folder.dir));

    const status = await fetch(`${server.url}/_anfra/api/status`).then((r) => r.json());
    expect(status).toMatchObject({ anfra: 'up', problems: [] });

    await server.stop();
    expect(await server.exited).toBe(0);
  } finally {
    await server.stop();
    folder.remove();
    await db.close();
  }
});

test('refuses to start, clearly, when a Data Source\'s database is unreachable', async () => {
  const db = await fakeDatabase();
  const deadPort = db.port;
  await db.close(); // nothing listens there any more
  const folder = repo(deadPort);
  try {
    const { code, output } = await startServeExpectingFailure({ repo: folder.dir });
    expect(code).not.toBe(0);
    expect(output).toContain('Can\'t reach the database');
    expect(output).toContain(`demo_pg (127.0.0.1:${deadPort})`);
  } finally {
    folder.remove();
  }
});

test('refuses to start, clearly, when the repo has no context source config', async () => {
  const db = await fakeDatabase();
  const folder = repo(db.port);
  fs.rmSync(path.join(folder.dir, '.anfra', 'context_sources.yml'));
  try {
    const { code, output } = await startServeExpectingFailure({ repo: folder.dir });
    expect(code).not.toBe(0);
    expect(output).toContain('context_sources.yml');
  } finally {
    folder.remove();
    await db.close();
  }
});
