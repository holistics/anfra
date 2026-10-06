import { spawn, type ChildProcess } from 'node:child_process';
import * as fs from 'node:fs';
import * as net from 'node:net';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defaultResponses } from './fixtures';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
/** anfra built with the e2e fake (`-tags apps_e2e`) by test/global-setup.ts. */
export const ANFRA_E2E_BIN = path.join(repoRoot, 'bin', 'anfra-e2e');

async function freePort (): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address() as net.AddressInfo;
      server.close(() => resolve(port));
    });
  });
}

/** A TCP listener standing in for a Data Source's database, so the startup check passes. */
export async function fakeDatabase (): Promise<{ port: number, close: () => Promise<void> }> {
  const server = net.createServer((socket) => socket.end());
  await new Promise<void>((resolve) => { server.listen(0, '127.0.0.1', resolve); });
  const { port } = server.address() as net.AddressInfo;
  return { port, close: () => new Promise((resolve) => { server.close(() => resolve()); }) };
}

export interface Repo {
  dir: string;
  /** Write a file under the Repo, creating its directories. */
  write: (rel: string, content: string) => void;
  remove: () => void;
}

/** A throwaway Repo whose one Data Source points at `dbPort`. `files` are relative to it. */
export function repo (dbPort: number, files: Record<string, string> = {}): Repo {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'anfra-server-folder-'));
  const write = (rel: string, content: string) => {
    const file = path.join(dir, rel);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, content);
  };
  write('.anfra/context_sources.yml', 'sources:\n  - name: aml\n    type: aml\n    path: ..\n');
  write('.anfra/data_sources.yml', [
    'data_sources:',
    '  demo_pg:',
    '    type: postgresql',
    '    connection: { host: 127.0.0.1, port: ' + dbPort + ', user: u, password: p, dbname: d }',
    '',
  ].join('\n'));
  Object.entries(files).forEach(([rel, content]) => write(rel, content));
  return { dir, write, remove: () => fs.rmSync(dir, { recursive: true, force: true }) };
}

/** A minimal Data App file. */
export const dataAppHtml = (title?: string) => `<!doctype html>
<html><head><meta charset="utf-8">${title === undefined ? '' : `<title>${title}</title>`}</head>
<body><p>${title ?? 'untitled'}</p></body></html>
`;

export interface Server {
  url: string;
  process: ChildProcess;
  output: () => string;
  /** The events the fake anfra logged: its start, and each command. */
  anfraLog: () => Record<string, unknown>[];
  /** The args of each `command` the fake ran. */
  anfraCalls: (command: string) => Record<string, unknown>[];
  /** Change what the fake answers from now on (on top of `defaultResponses`). */
  setAnfra: (answers: Record<string, unknown>) => void;
  /** Resolves with the exit code once the server process exits. */
  exited: Promise<number | null>;
  stop: () => Promise<void>;
}

export interface ServeOptions {
  repo: string;
  env?: Record<string, string>;
  /** Fake anfra answers on top of `defaultResponses`, keyed as internal/apps/dispatch/fake.go describes. */
  anfra?: Record<string, unknown>;
}

function launch ({ repo: folder, env = {}, anfra = {} }: ServeOptions, port: number) {
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'anfra-e2e-'));
  const responses = path.join(scratch, 'responses.json');
  const log = path.join(scratch, 'anfra.jsonl');
  fs.writeFileSync(responses, JSON.stringify({ ...defaultResponses, ...anfra }));

  // `anfra serve --apps` in the repo, as a user runs it. HOME is scratch, so anfra's per-repo
  // state and logs land there rather than in the real ~/.anfra.
  const child = spawn(ANFRA_E2E_BIN, ['serve', '--apps', '--port', String(port)], {
    cwd: folder,
    env: {
      ...process.env,
      HOME: scratch,
      ANFRA_NO_UPDATE_NOTIFIER: '1',
      ANFRA_E2E_RESPONSES: responses,
      ANFRA_E2E_LOG: log,
      ...env,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let output = '';
  child.stdout?.on('data', (chunk: Buffer) => { output += chunk.toString(); });
  child.stderr?.on('data', (chunk: Buffer) => { output += chunk.toString(); });
  const exited = new Promise<number | null>((resolve) => { child.on('exit', (code) => resolve(code)); });
  const anfraLog = () => fakeLog(env.ANFRA_E2E_LOG ?? log);
  const anfraCalls = (command: string) => anfraLog()
    .filter((e) => e.event === 'call' && e.command === command)
    .map((e) => (e.args ?? {}) as Record<string, unknown>);
  const setAnfra = (answers: Record<string, unknown>) => {
    fs.writeFileSync(responses, JSON.stringify({ ...defaultResponses, ...answers }));
  };
  return {
    child, output: () => output, exited, anfraLog, anfraCalls, setAnfra,
  };
}

/** Start `anfra serve --apps` in the repo, with the fake anfra, and wait until it serves Data Apps. */
export async function startServe (options: ServeOptions): Promise<Server> {
  const port = await freePort();
  const {
    child, output, exited, anfraLog, anfraCalls, setAnfra,
  } = launch(options, port);
  const url = `http://127.0.0.1:${port}`;

  const deadline = Date.now() + 30_000;
  let gone = false;
  void exited.then(() => { gone = true; });
  while (Date.now() < deadline) {
    if (gone) throw new Error(`The server exited during startup:\n${output()}`);
    if (output().includes('Data Apps at http://')) {
      return {
        url,
        process: child,
        output,
        anfraLog,
        anfraCalls,
        setAnfra,
        exited,
        stop: async () => {
          if (!gone) child.kill('SIGTERM');
          await exited;
        },
      };
    }
    await new Promise((resolve) => { setTimeout(resolve, 100); });
  }
  child.kill('SIGKILL');
  throw new Error(`The server didn't start within 30s:\n${output()}`);
}

/** Start the server expecting it to refuse, and return its exit code and output. */
export async function startServeExpectingFailure (options: ServeOptions): Promise<{ code: number | null, output: string }> {
  const { output, exited } = launch(options, await freePort());
  const code = await Promise.race([
    exited,
    new Promise<null>((_, reject) => { setTimeout(() => reject(new Error(`The server didn't exit:\n${output()}`)), 30_000); }),
  ]);
  return { code, output: output() };
}

/** The events the fake anfra logged. */
export function fakeLog (file: string): Record<string, unknown>[] {
  if (!fs.existsSync(file)) return [];
  return fs.readFileSync(file, 'utf-8').split('\n').filter(Boolean).map((line) => JSON.parse(line));
}
