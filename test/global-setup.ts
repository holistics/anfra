import { execFileSync } from 'node:child_process';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

/**
 * Build what the tests run: the Anfra SDK bundle and the Shell (both embedded into the binary),
 * then anfra itself with the e2e fake (`-tags apps_e2e`), which answers commands from canned
 * responses instead of running sidecars. Needs `go` on PATH.
 */
export default function globalSetup (): void {
  const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const run = (cmd: string, args: string[]) => execFileSync(cmd, args, { cwd: repoRoot, stdio: 'inherit' });
  run('pnpm', ['build:apps']);
  run('go', ['build', '-tags', 'apps_e2e', '-o', 'bin/anfra-e2e', './cmd/anfra']);
}
