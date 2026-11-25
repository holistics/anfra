# Sidecar transport on Windows

> Status: done. All five checks passed; decision: Unix sockets, no fallback. Spec: [SPEC-win-sidecar-transport.md](../../SPEC-win-sidecar-transport.md). Plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md), module `win-sidecar-transport`.

anfra talks to its sidecars over Unix sockets (AF_UNIX): a socket file in a private folder (`internal/sidecar/socket.go`). This doc answers one question: **on Windows, can anfra, anfra-node and canal keep talking that way, with no change to how they talk?**

| From → to | How | Code |
|---|---|---|
| anfra (Go) → anfra-node (Bun) | HTTP JSON-RPC; anfra-node listens with `node:http` | anfra `internal/sidecar/anfranode/client.go` `NewClientUnix`; holistics-core `apps/anfra-node/src/rpc/server.ts` `startServer` |
| anfra (Go) → canal-query (Go) | HTTP | anfra `internal/sidecar/canalquery/canalquery.go`; canal `internal/shared/utils/listen.go` `Listen` |
| canal-query (Go) → canal-lake (Go) | Arrow Flight (gRPC), address `"unix:" + CANAL_LAKE_SOCKET` | canal `internal/query/lakeclient/lakeclient.go` `Location` |

Reading Bun's source suggests yes: on Windows, Bun makes a real AF_UNIX socket for a normal file path, and a named pipe only for `\\.\pipe\…` names ([Background: Bun's source](#background-buns-source)). The checks below confirm it on a real machine.

## Background: Bun's source

anfra-node is a Bun program (built with Bun 1.2.10). Node.js on Windows always uses **named pipes** for `listen(path)`, but **Bun is different**. Bun's source code shows that on Windows, `listen(path)` (and `Bun.listen({ unix: path })`) checks the path:

| The path looks like | What Bun creates |
|---|---|
| `\\.\pipe\name` or `\\?\pipe\name` | a Windows **named pipe** |
| anything else, e.g. `C:\Users\me\AppData\Local\Temp\anfra-x\anfra-node.sock` | a **real Unix socket (AF_UNIX)**, using Windows' own support (`afunix.h`, Windows 10 version 1803 and later) |

Evidence, at tag `bun-v1.2.10`:
- [`src/bun.js/api/bun/socket.zig:642`](https://github.com/oven-sh/bun/blob/bun-v1.2.10/src/bun.js/api/bun/socket.zig): *"we check if the path is a named pipe otherwise we try to connect using AF_UNIX"*. `normalizePipeName` returns a value only for `\\.\pipe\` and `\\?\pipe\`.
- `packages/bun-usockets/src/bsd.c:970`: `#include <afunix.h>` on Windows, then `bsd_create_socket(AF_UNIX, SOCK_STREAM, …)`.
- The current `main` branch still works this way ([`src/runtime/socket/Listener.rs:224`](https://github.com/oven-sh/bun/blob/main/src/runtime/socket/Listener.rs), [`bsd.cpp:1408`](https://github.com/oven-sh/bun/blob/main/packages/bun-usockets/src/bsd.cpp)).
- `node:net`'s `server.listen(path)` calls `Bun.listen({ unix: path })` ([`src/js/node/net.ts`](https://github.com/oven-sh/bun/blob/main/src/js/node/net.ts)), so anfra-node's `src/rpc/server.ts` gets the same behaviour.

**What this means:**
- anfra's Go client already dials `net.Dial("unix", path)` (`internal/sidecar/anfranode/client.go`). Go has supported AF_UNIX on Windows since Go 1.12, so it should connect with **no transport change**. Go to canal-query, and canal-query to canal-lake, are Go to Go and work the same way.
- **Avoid named pipes.** Bun's named-pipe code has known crashes on Windows (e.g. [Claude Code's Chrome host](https://claudeissues.com/issue/22416-chrome-native-host-crashes-on-windows-bun-1-3-5-panic-on-named-pipe-creation)). The AF_UNIX path doesn't go through that code.

These findings come from reading source code, not from running anything on Windows. The checks below confirm them.

**One gap:** anfra-node listens with **`node:http`** (`http.createServer().listen(path)`), not `node:net`. In Bun, `node:http` runs on `Bun.serve`, a different code path from `Bun.listen`. The research above covers `Bun.listen`. Check 1 tests `node:http` directly.

## Machine

- Windows edition and build: Windows 11, 10.0.26200.9974 (25H2), x64. A developer laptop, not a clean install; the checks ran as native Windows programs, launched from WSL.
- AF_UNIX driver: the `afunix` service is Running.
- Bun version anfra-node was built with: 1.2.10, cross-compiled from Linux (`bun-windows-x64`).
- Commits the checks were built from:
  - anfra: `19119ef`; check programs on `spike/win-sidecar-transport` (`dev/wincheck/`).
  - holistics-core: `af32df7a9` (anfra-node 0.13.1). Its socket, startup and watchdog code (`src/rpc/server.ts`, `src/main.ts`, `src/rpc/watchdog.ts`) and `scripts/build-sidecar.sh` are the same as the released `anfra-node-v0.16.0`.
  - canal: `bdda8ef` (`origin/master`); check programs on its `spike/win-sidecar-transport` (`dev/wincheck/`).
- Date: 2026-11-23

## Check 1: anfra → anfra-node over `node:http`

`anfra-node.exe --socket=<file path>`, then a Go client with the same dialer as `NewClientUnix` calls `GET /health` and one JSON-RPC method. This decides the transport: `node:http` in Bun runs on `Bun.serve`, a different code path from the `Bun.listen` the source reading covered.

- What ran: `dialnode.exe` (`dev/wincheck/dialnode`) started `anfra-node.exe --socket=C:\Users\nuita\AppData\Local\Temp\anfra-check1\anfra-node.sock` (62 bytes) with its stdin held open, then dialed with `net.Dial("unix", …)`. The same program first passed on Linux, against a Linux anfra-node.
- Result: **pass.**
  ```
  GET /health: {"status":"ok"}
  JSON-RPC ping: {"jsonrpc":"2.0","id":1,"result":{"pong":true}}
  socket file: mode=Srw-rw-rw-, ModeSocket=true
  ```
  anfra-node logged `sidecar.listening` and one `rpc.request`, and no anfra-node process was left after `dialnode` closed its stdin and stopped it.
- What it means:
  - Bun's `node:http` makes a **real AF_UNIX socket** for a normal file path on Windows, not a named pipe: the file exists, and Go reports it as a socket. anfra's Go client reaches it with no change.
  - On Windows, Go's `os.Lstat` reports `ModeSocket` for a socket file. That bears on check 3 (canal's stale-socket check relies on it).
  - Found along the way: started with a closed stdin (PowerShell's `Start-Process`), anfra-node listened, then logged `watchdog.parent_death` ("host stdin closed (EOF)") 10 ms later and exited. So its stdin watchdog does notice EOF on Windows, which bears on check 5. anfra must start it with stdin held open, as it does today (`PipeStdin`).

## Check 2: canal-query → canal-lake, gRPC on a `C:\` path

A Flight server on a socket in `%TEMP%` (canal's `Listen`), called once through canal's `lakeclient.New()` with `CANAL_LAKE_SOCKET` set. Does grpc-go accept `unix:C:\…`?

- What ran: `flightunix.exe` (canal, `dev/wincheck/flightunix`) listens with canal's own `utils.Listen` on `%TEMP%\canal-check-<random>\canal-lake.sock`, serves a minimal Arrow Flight service there, sets `CANAL_LAKE_SOCKET`, and calls `ListFlights` through `lakeclient.New()`, the client canal-query uses. canal-lake itself isn't started: it needs DuckDB, and this check is about the address only. Built with `CGO_ENABLED=0`; the same program first passed on Linux.
- Result: **pass,** with the address exactly as canal builds it today:
  ```
  address (lakeclient.Location): unix:C:\Users\nuita\AppData\Local\Temp\canal-check-2212768340\canal-lake.sock
  ListFlights answered: TotalRecords = 42
  PASS
  ```
- What it means: grpc-go (v1.83.2, with arrow-go's Flight client v18.5.1) accepts `unix:` followed by a Windows path, and connects over AF_UNIX. `lakeclient.Location` needs **no change** (T7 skipped), and the `unix:///C:/…` form the program would have tried next wasn't needed.

## Check 3: canal `Listen` on a leftover socket

canal's own `TestListenOnStaleOrLiveSocket` and `TestListenKeepsNonSocketFile` (`internal/shared/utils/listen_test.go`), built for Windows with `go test -c`. Also: does `os.Lstat` report `ModeSocket` for a socket file on Windows?

- What ran: `utils.test.exe`, canal's tests for `utils.Listen` built for Windows with `GOOS=windows CGO_ENABLED=0 go test -c ./internal/shared/utils`, run with `-test.run TestListen`. The same tests first passed on Linux.
  - `TestListenOnStaleOrLiveSocket` leaves a socket file behind from a listener that's gone (as a crashed canal-query or canal-lake would), checks `Listen` replaces it, then checks a second `Listen` on a live socket is refused ("in use").
  - `TestListenKeepsNonSocketFile` puts an ordinary file at the path and checks `Listen` refuses and leaves it alone.
- Result: **pass,** both tests:
  ```
  --- PASS: TestListenOnStaleOrLiveSocket (0.00s)
  --- PASS: TestListenKeepsNonSocketFile (0.00s)
  PASS
  ```
- What it means:
  - `Listen`'s stale-socket check (`fi.Mode()&os.ModeSocket != 0`, `listen.go:21`) works on Windows: Go reports a leftover AF_UNIX file as a socket there, as check 1 also saw for the socket anfra-node made. So canal restarts cleanly after a crash, and still never deletes a file that isn't a socket.
  - `Listen` needs **no change** (T8 skipped).

## Check 4: socket path length

The path `SocketPath` builds (`os.MkdirTemp("", "anfra-")` + `canal-query.sock`), for a user name of 20+ characters. AF_UNIX caps a path at about 108 bytes.

- What ran: `sockpath.exe` (`dev/wincheck/sockpath`) builds the path the way `SocketPath` does, listens on it and dials it. Run with the real `%TEMP%`, then with `TEMP` pointed at folders as long as `C:\Users\<name>\AppData\Local\Temp` (28 characters plus the name) for longer names, and at folders with accented names. The same program first passed on Linux with a short temp folder, and failed there with a long one, so it does detect the limit.
- Result: **pass for realistic user names.**

  | Temp folder | Socket path | Result |
  |---|---|---|
  | Real: `C:\Users\nuita\AppData\Local\Temp` | 67 bytes | pass |
  | As long as a 20-character user name | 82 bytes | pass |
  | As long as a 32-character name | 94 bytes | pass |
  | As long as a 40-character name | 102 bytes | pass |
  | As long as a 48-character name | 110 bytes | **fail**: `bind: invalid argument` |
  | Accented names (`Nguyễn Văn Ánh`, `Nguyễn Thị Phương Thảo`) | 78, 90 bytes | pass |

- What it means:
  - The limit is between 102 and 110 bytes (AF_UNIX allows 108, including a closing zero). With anfra's longest socket name, `canal-query.sock`, a profile folder name of up to about **45 characters** fits.
  - Windows local account names are at most 20 characters, and a profile folder that adds a domain (`name.DOMAIN`, a domain name is at most 15 characters) stays under about 40. So realistic profiles fit, with a few characters to spare.
  - The length counts bytes: accented letters take two or three each, so a long name made mostly of them gets closer to the limit.
  - **No fix needed** (T9 skipped). If it's ever hit, the error is `bind: invalid argument` from the sidecar, and the fix in T9 (a shorter base folder on Windows) is small.

## Check 5: anfra-node's stdin watchdog

`anfra-node.exe` started with stdin as a pipe, which is then closed. Does it exit? (`src/rpc/watchdog.ts` `fdIsPipe` must see a Windows anonymous pipe as a pipe.) Recorded for `win-anfra-node`; not fixed here.

- What ran: `stdinwatch.exe` (`dev/wincheck/stdinwatch`) starts `anfra-node.exe` with stdin as a pipe, as anfra does (`PipeStdin`), waits until it answers `/health` over its socket, then ends the host one of three ways. The same program first passed on Linux.
- Result: **pass, in all three cases:**

  | How the host ends | anfra-node |
  |---|---|
  | **close:** the host closes the pipe (anfra's clean stop) | Exited at once (0.0 s) |
  | **exit:** the host exits without stopping anfra-node (a crash) | Gone; none left running 4 s later |
  | **kill:** the host is killed with `taskkill /F` while anfra-node runs | Gone 0.1 s after the kill; it logged `watchdog.parent_death`, reason "host stdin closed (EOF)" |

- What it means:
  - On Windows, Bun's `fdIsPipe(0)` sees anfra's stdin pipe as a pipe, so the stdin-EOF watchdog is armed, and it fires whenever the host goes, cleanly or not. **anfra-node can't be orphaned** as long as anfra starts it with stdin as a pipe, as it does today.
  - The watchdog's other check, the parent-PID poll, never fires on Windows (no reparenting), but it doesn't misfire either, so it needs no change; stdin EOF covers it.
  - For `win-anfra-node`: the watchdog needs no Windows work. Only `SIGBREAK` handling remains there (for anfra's Ctrl-Break stop).
  - For `win-process-lifecycle`: closing stdin is enough to stop anfra-node cleanly on Windows. A Job Object is still needed for canal-query and canal-lake, which have no stdin watchdog.

## Decision

**Unix sockets (AF_UNIX) on Windows, as on macOS and Linux. No fallback transport.**

Why:
- All five checks passed on Windows 11 25H2, with the code as it is today in all three repos: anfra → anfra-node over `node:http` on Bun (check 1), canal-query → canal-lake over gRPC with a `unix:C:\…` address (check 2), canal's restart over a leftover socket file (check 3), socket path length for realistic user names (check 4), and anfra-node stopping when anfra goes, even after `taskkill /F` (check 5).
- No fix was needed in any repo (T7–T9 skipped), so nothing changes on macOS or Linux either.
- Named pipes would have meant Windows-only code in all three repos, and Bun's named-pipe code has known crashes on Windows; neither is needed now.

What it hands on:
- `win-process-lifecycle`: closing anfra-node's stdin stops it cleanly (check 5); a Job Object is still needed for canal-query and canal-lake, which have no stdin watchdog. Socket paths have a few characters to spare for long profile folder names (check 4).
- `win-anfra-node`: the watchdog needs no change; only `SIGBREAK` handling remains (check 5).
- `win-canal`: `lakeclient.Location` and `utils.Listen` need no change (checks 2 and 3).

Not covered here: anfra itself on Windows (it doesn't compile there yet; these checks copied its socket code), and canal built with DuckDB (these checks built canal's Go code without cgo). Those are `win-process-lifecycle` and `win-canal`'s build spike.

## Later: stopping canal with Ctrl-Break, with and without a console

Checked for `win-process-lifecycle` (task T3). Windows has no SIGTERM, so anfra stops canal with Ctrl-Break, which Go delivers as `os.Interrupt`. Ctrl-Break only reaches processes on the sender's console. The question: does it reach canal when canal has a hidden console of its own (`CREATE_NO_WINDOW`, so no window opens), or when anfra has no console at all (started by an agent, or detached)?

- What ran: a small check program in anfra's ignored `tmp/` starts the DLL build of `anfra-canal.exe query`, waits for `/health`, sends Ctrl-Break, and waits 5 s. "Clean" means canal logged "shutting down", exited with code 0, and removed its socket. It ran once in a console, and once re-started with `DETACHED_PROCESS` (no console).
- Result:

  | anfra | canal started with | How Ctrl-Break is sent | canal |
  |---|---|---|---|
  | has a console | `CREATE_NEW_PROCESS_GROUP` (shares anfra's console) | directly | **clean**, at once |
  | has a console | `+ CREATE_NO_WINDOW` (its own hidden console) | directly | not reached; killed after 5 s |
  | has a console | `+ CREATE_NO_WINDOW` | anfra attaches to canal's console, sends, leaves | **clean**, at once |
  | no console | `CREATE_NEW_PROCESS_GROUP` | directly | fails: "The handle is invalid"; killed |
  | no console | `+ CREATE_NO_WINDOW` | directly | fails the same way; killed |
  | no console | `+ CREATE_NO_WINDOW` | anfra attaches to canal's console, sends, leaves | **clean**, at once |

- What it means, and what `internal/sidecar` now does:
  - **anfra has a console:** sidecars share it (`CREATE_NEW_PROCESS_GROUP` only), so no window opens and Ctrl-Break is sent directly. Attaching would mean anfra leaving its own console, so it isn't used here.
  - **anfra has no console:** sidecars get a hidden one (`CREATE_NO_WINDOW`, or Windows would open a window), and anfra stops one by attaching to its console (`AttachConsole(pid)`), sending Ctrl-Break, and leaving (`FreeConsole`). One stop attaches at a time.
  - Whether anfra has a console is read once, with `GetConsoleCP`. `GetConsoleWindow` isn't used: it answered "none" when run from WSL, though Ctrl-Break worked (that console has no window).
  - No fallback is needed: canal stops cleanly in every case. Killing after the wait stays, for a sidecar that doesn't stop.
  - For `win-process-lifecycle` T8: `anfra serve` started by the console with `DETACHED_PROCESS` has no console, so the console can't reach it with Ctrl-Break this way. Starting it with `CREATE_NO_WINDOW` instead gives it a hidden console to attach to.

## Later: anfra-node with Bun 1.4.4

Checked for `win-anfra-node` (tasks T1 and T3). anfra-node's release builds with Bun 1.4.4; the checks above used 1.2.10. Rebuilt from holistics-core `78cfd8c49` (anfra-node 0.17.1) with Bun 1.4.4, cross-compiled from Linux (`bun-windows-x64`), with the `SIGBREAK` handler added.

| Check | Result |
|---|---|
| Check 1: anfra's Go client reaches anfra-node on a Unix socket at a `C:\` path (`dialnode`) | **pass** |
| Check 5, close: the host closes stdin | **pass**: exits at once, reason "host stdin closed (EOF)" |
| Check 5, exit: the host exits without stopping it | **pass**: gone; logs `watchdog.parent_death`, then shuts down |
| Check 5, kill: `taskkill /F` on the host | **pass**: gone 1 s after |
| Stale socket: anfra-node killed with `taskkill /F`, its socket file left, a new one started on the same path | **pass**: the new one listens (`server.ts` removes the stale file) |
| Ctrl-Break, with the `SIGBREAK` handler | **pass**: logs "Shutting down", reason `SIGBREAK`, exit code 0, no `watchdog.parent_death` |
| Ctrl-Break, without the handler (anfra-node as released) | Ended at once, exit code `0xC000013A`; its shutdown doesn't run |

So Bun 1.4.4 changes nothing for anfra-node on Windows, and Bun delivers Ctrl-Break to `process.on('SIGBREAK')`: anfra can stop anfra-node with Ctrl-Break, as it stops canal, once anfra-node has the handler.
