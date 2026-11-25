# Spec: win-anfra-node

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-anfra-node`). Depends on: `win-sidecar-transport` (done). Uses `win-process-lifecycle`'s results (done). Repos: **holistics-core** (anfra-node) and **anfra** (how it stops anfra-node). Milestone **M2**: the sidecars ship. This module makes anfra-node's code right for Windows; its release build for Windows comes with the CI work, later.

## Objective

anfra-node already runs on Windows: M1 used it, built with Bun from the `anfra-node-v0.16.0` tag, and `validate`, `show` and `query` (with the cache) worked. One thing is wrong, and three are unchecked.

**What's wrong: anfra-node's stop logs a warning on Windows.** After every anfra command, anfra-node logs `watchdog.parent_death` ("Host process gone", WARN). On Windows, anfra stops anfra-node by closing its stdin (`internal/sidecar/stop_windows.go`), and anfra-node's watchdog takes stdin EOF to mean its host died (`src/rpc/watchdog.ts:45`). The stop itself is clean: `shutdown` closes the server and writes the compile cache (`src/main.ts:78-87`). On Linux, anfra sends SIGTERM, and anfra-node logs only `sidecar.shutdown` with reason `SIGTERM`. Decided with the user: **make Ctrl-Break anfra-node's normal stop on Windows**, as SIGTERM is on Linux:

1. **anfra-node** (`src/main.ts:89-90`) shuts down on `SIGBREAK`, as on `SIGINT` and `SIGTERM`. Go delivers Ctrl-Break to canal as `os.Interrupt`; Node delivers it to a program as `SIGBREAK`. Whether **Bun** delivers it on Windows is unknown, and is checked first.
2. **anfra** (`internal/sidecar/stop_windows.go`) sends Ctrl-Break to every sidecar, anfra-node included, and closes anfra-node's stdin only once it has exited (or been killed after the wait). Closing it earlier would still fire the watchdog's warning: `fire` logs before it calls `shutdown`, which ignores a second call (`watchdog.ts:42-46`, `main.ts:79`).

The watchdog stays as it is: stdin EOF still stops anfra-node when anfra dies without stopping it (`win-sidecar-transport` check 5), and it should still warn then.

**What's unchecked:**

3. **Bun 1.4.4.** anfra-node's release builds with Bun **1.4.4** (`anfra_node_build_release.yml`). The Windows checks (`win-sidecar-transport`) and M1 used **1.2.10**. Re-run, with a 1.4.4 build: anfra-node listens on a Unix socket at a `C:\` path and answers anfra (check 1); it stops on stdin EOF, after a clean stop, a crash and `taskkill /F` (check 5); and it starts over a stale socket file left by a crash (`src/rpc/server.ts:54-57`, the `fs.unlinkSync`).
4. **The `/repo` stand-in** (`src/amql-shell/source/selectors.ts:14`). The plan said it should be a real Windows path. It needs no change: without a repo folder, the code uses POSIX paths throughout (`const p = rootPath ? path : path.posix`), so `/repo` is never mixed with a Windows path. With a repo folder, it uses the platform's `path`, and turns the relative path into `/` form (`split(p.sep).join('/')`). Check it on Windows: `anfra validate` with path globs (`models/**`), and one escaping the repo (`..\x`), which must be refused.

**Users:** anyone running anfra on Windows (clean logs, a clean stop); `win-release`, which embeds the Windows anfra-node.

**Out of scope:**
- The `windows-x64` row in `anfra_node_build_release.yml`, and a Windows smoke test in holistics-core's CI: later, with the rest of the CI work (decided with the user).
- Signing: waits for the code-signing certificate.
- Bumping Bun.
- anfra's pin of anfra-node (`manifest.yml`): bumped once holistics-core releases an anfra-node with `SIGBREAK`. Until then, anfra's Windows stop must not send Ctrl-Break to an anfra-node without the handler (see Boundaries).

## Tech Stack

- anfra-node: TypeScript on Bun, in `holistics-core/apps/anfra-node` (version 0.16.0 at its last tag). Tests: vitest (`pnpm --filter @holistics/anfra-node test`). Bun **1.4.4**, as its release.
- anfra: Go 1.27, `golang.org/x/sys/windows`.
- Windows 11 x64, the host from the earlier modules (`C:\Users\nuita\wincheck`).

## Commands

```sh
# holistics-core: a branch from its latest master, in a worktree inside anfra's ignored tmp/
git -C ../holistics-core fetch origin
git -C ../holistics-core worktree add -b feat/anfra-node-windows-stop <anfra>/tmp/hc-win origin/master
cd <anfra>/tmp/hc-win
pnpm --filter "@holistics/anfra-node..." install
pnpm --filter "@holistics/anfra-node..." run build          # the workspace packages, as the release does
pnpm --filter @holistics/anfra-node test
BUN_COMPILE_TARGET=bun-windows-x64 bash apps/anfra-node/scripts/build-sidecar.sh <anfra>/tmp/anfra-node.exe   # Bun 1.4.4

# anfra: the stop change, one command at a time
GOOS=windows GOARCH=amd64 go vet ./internal/sidecar/
go test ./internal/sidecar/
GOOS=windows GOARCH=amd64 go test -c -o /mnt/c/Users/nuita/wincheck/sidecar.test.exe ./internal/sidecar/
GOOS=windows GOARCH=amd64 go build -o /mnt/c/Users/nuita/wincheck/anfra.exe ./cmd/anfra
```

```powershell
# On Windows: anfra with the new anfra-node
$env:ANFRA_NODE_BIN  = "C:\Users\nuita\wincheck\anfra-node.exe"
$env:ANFRA_CANAL_BIN = "C:\Users\nuita\wincheck\dll\anfra-canal.exe"
cd C:\Users\nuita\wincheck\demo
..\anfra.exe query -d ecommerce "orders | count(orders.id)" --cache.duration 10m
..\anfra.exe logs --level warn
```

## Project Structure

| Repo | File | Change |
|---|---|---|
| holistics-core | `apps/anfra-node/src/main.ts` | `process.on('SIGBREAK', …)` beside SIGINT and SIGTERM |
| holistics-core | `apps/anfra-node/src/*.test.ts` | A test that SIGBREAK shuts it down, if it can run under vitest; otherwise the Windows check below |
| anfra | `internal/sidecar/stop_windows.go` | Ctrl-Break to every sidecar; close stdin after it exits |
| anfra | `internal/sidecar/stop_windows_test.go` | A sidecar with a stdin pipe is stopped by Ctrl-Break, not EOF |
| anfra | `docs/designs/sidecars.md`, `docs/research/windows-sidecar-transport.md` | The stop, and the Bun 1.4.4 results |
| anfra | `WINDOWS-PLAN.md` | Where it stands; the module's result |

Check programs (the Bun 1.4.4 re-checks) are reused from the spike branches (`dev/wincheck/` in anfra's `spike/win-sidecar-transport` worktree), not committed to either repo's main branch.

## Code Style

Each repo's own conventions. anfra: [naming](docs/dev/naming.md), [readable prose](docs/designs/readable-prose.md), comments only for why. anfra-node: follow `main.ts` as it is:

```ts
process.on('SIGINT', () => { shutdown('SIGINT'); });
process.on('SIGTERM', () => { shutdown('SIGTERM'); });
// Windows has no SIGTERM: anfra stops it with Ctrl-Break, which arrives as SIGBREAK.
process.on('SIGBREAK', () => { shutdown('SIGBREAK'); });
```

## Testing Strategy

- **Bun delivers SIGBREAK on Windows** (first, the risk): a Bun 1.4.4 Windows build of anfra-node, started by anfra's `StartProcess`, receives Ctrl-Break and logs `sidecar.shutdown` with reason `SIGBREAK`, and no `watchdog.parent_death`.
- **anfra-node unit tests** pass on Linux (`pnpm --filter @holistics/anfra-node test`).
- **anfra:** `stop_windows_test.go` covers a sidecar with a stdin pipe that stops on Ctrl-Break (the test helper waits for `os.Interrupt`, as before); the existing stop tests still pass on Windows; Linux tests unchanged.
- **The Bun 1.4.4 re-checks** on Windows 11: socket at a `C:\` path (check 1), stdin EOF after a clean stop, a crash and `taskkill /F` (check 5), a stale socket file.
- **End to end on Windows 11,** with the new anfra-node and anfra: `validate` (with globs, and one escaping the repo), `show`, `query` with and without the cache; after each, no sidecar left, and `anfra logs --level warn` has no `watchdog.parent_death`; `taskkill /F` on anfra during a query still leaves none.

## Boundaries

- **Always:** keep the watchdog's warning for a host that really died; stop each sidecar one way only; run checks on Windows with the Bun version the release uses (1.4.4); keep the MotherDuck token and any credentials out of files and logs; run Go checks one command at a time.
- **Ask first:**
  - pushing a branch or opening a PR in holistics-core (another team's repo), and cutting an anfra-node release;
  - bumping anfra's `manifest.yml` pin to that release;
  - **landing anfra's Ctrl-Break stop for anfra-node before that pin bump:** with today's pinned anfra-node (no SIGBREAK handler), Ctrl-Break would end it at once, skipping its compile-cache write. Until the pin moves, anfra's change stays on its branch, unmerged;
  - bumping Bun, or changing the release workflow;
  - changing the watchdog.
- **Never:** commit to holistics-core's `master`; change anfra-node's behaviour on Linux or macOS; send Ctrl-Break and close stdin at once.

## Success Criteria

1. On Windows 11, a Bun 1.4.4 build of anfra-node stops on Ctrl-Break: it logs `sidecar.shutdown` with reason `SIGBREAK`, writes its compile cache, and exits with code 0.
2. On Windows, after any anfra command, `anfra logs --level warn` has no `watchdog.parent_death`; after `taskkill /F` on anfra, no anfra-node is left. (Corrected with the user: the Job Object ends anfra-node at once, so it logs nothing and doesn't write its compile cache. Accepted: it happens only when anfra is killed hard, and loses a log line and some cache work.)
3. The Bun 1.4.4 re-checks pass on Windows 11: socket at a `C:\` path, stdin EOF in all three cases, a stale socket file.
4. `anfra validate` with path globs works on Windows, and a path escaping the repo is refused; `selectors.ts` unchanged.
5. anfra-node's tests pass on Linux; anfra's `internal/sidecar` tests pass on Linux and Windows; on Linux and macOS, neither program's stop changes.
6. `docs/designs/sidecars.md` describes the new Windows stop; the findings doc records the Bun 1.4.4 results; `WINDOWS-PLAN.md` marks the module done (code), with the release row and smoke test left for the CI work.

## Open Questions

- If Bun 1.4.4 doesn't deliver SIGBREAK on Windows, the user chooses between: patching it another way (for example Bun's own Windows console handler, if one exists), lowering the stdin-EOF warning to INFO, or keeping the warning.
