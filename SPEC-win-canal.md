# Spec: win-canal

Part of [WINDOWS-PLAN.md](WINDOWS-PLAN.md) (module `win-canal`). Depends on: nothing for its build spike; `win-sidecar-transport` (done) for the rest. Repo: **canal** (`holistics/canal`, work from `origin/master`; the local checkout is stale).

## Objective

anfra embeds one canal binary, **`anfra-canal`**, which runs as canal-query (every SQL query) or canal-lake (the query cache): `anfra-canal query | lake | --version` (`internal/anfra-canal/anfracanal.go`). There's no Windows build of it today, and without one there's no Windows anfra.

This module delivers **`anfra-canal.exe` for windows-x64**, built from canal's code and shown to work on Windows.

**The build spike comes first and decides the plan's go/no-go:** canal is Go plus C code (DuckDB through holistics' `duckdb-go` fork, and the Oracle driver godror), built with cgo. Nobody has built it for Windows yet. If it needs more than about **5 ED** to build and run, stop and re-plan.

**What we already know:**
- canal's `go.mod` already pulls in DuckDB's Windows library: `github.com/duckdb/duckdb-go-bindings/lib/windows-amd64` v0.10506.0 (static, built with MinGW). The unknown is whether holistics' fork (`github.com/holistics/duckdb-go/v2 v2.10506.0-streaming.1`) and godror build with it.
- canal's sockets already work on Windows: its `lakeclient.Location` and `utils.Listen` passed unchanged (`win-sidecar-transport`, checks 2 and 3).
- canal already stops cleanly on `os.Interrupt` (`internal/query/start.go:56`, `internal/lake/start.go:38`), and Go delivers Windows' Ctrl-Break as `os.Interrupt`. So anfra's Windows stop (Ctrl-Break, in `win-process-lifecycle`) should need **no canal change**; this module verifies it.

**Users:** anfra's Windows release (`win-release` embeds the binary).

**Decided with the user:**
- The spike was planned as a **cross-compile in WSL**. It changed in T2, with the user, to a **native build on Windows**: Ubuntu's MinGW (GCC 13, MSVCRT) can't link DuckDB's Windows library, which needs MinGW-Builds GCC 14.2.0 with UCRT ([findings](docs/research/windows-canal-build.md)).
- **MotherDuck on Windows (decided 2026-11-24, after the build spike):** MotherDuck's DuckDB extension has no MinGW build, so the static MinGW build can't load it. A **DLL spike** builds canal against DuckDB's official Microsoft-built `duckdb.dll` (`duckdb_use_lib`) instead; if it passes, Windows uses the DLL build, otherwise the static build ships with MotherDuck documented as not on native Windows yet.
- **No stdin watchdog** in canal: on Windows, anfra's Job Object (`win-process-lifecycle`) kills canal when anfra dies.
- **CI is deferred:** the Windows release row and PR check in canal's GitHub Actions come back later. This spec builds and proves the binary by hand.
- Hardening the TCP fallback's bind address is **out of scope for now**.

**Out of scope:** canal's GitHub Actions (release row, PR check), windows-arm64, signing, the macOS/Linux minimum-OS drift fix, and anfra's side of starting and stopping canal (`win-process-lifecycle`).

## Tech Stack

- canal: Go 1.26 (`go.mod`), cgo, build tag `duckdb_arrow`.
- DuckDB: `duckdb-go-bindings` v0.10506.0 (prebuilt static libraries per platform), holistics fork `duckdb-go` v2.10506.0-streaming.1.
- Oracle: `github.com/godror/godror` v0.50.0 (cgo, bundles ODPI-C; loads Oracle's client libraries only at run time).
- Toolchain: MinGW-w64 cross-compiler in WSL (`x86_64-w64-mingw32-gcc`, `-g++`).

## Commands

```sh
# In WSL, in a canal worktree (spike/win-sidecar-transport, or a win-canal branch)
sudo apt-get install -y gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64     # ask first: system packages

GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
  CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  go build -tags=duckdb_arrow -o anfra-canal.exe ./app/anfra-canal

# canal's own DuckDB tests, built for Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  go test -c -tags=duckdb_arrow -o connector.test.exe ./internal/query/connector
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  go test -c -tags=duckdb_arrow -o lake.test.exe ./internal/lake
```

```powershell
# On Windows (copied to C:\Users\<you>\wincheck)
.\anfra-canal.exe --version
.\connector.test.exe -test.run 'TestDuckdb' -test.v
.\lake.test.exe -test.v
```

## Project Structure

| What | Where | Committed? |
|---|---|---|
| Spike results: what built, what failed and why, on which toolchain | `docs/research/windows-canal-build.md` in anfra, listed in `docs/README.md` | Yes (anfra) |
| Spike-only helpers (a script with the build flags, an end-to-end check program) | canal `spike/win-sidecar-transport` branch, `dev/wincheck/` | No, never merged |
| Any code fix the build needs (e.g. a file that doesn't compile on Windows) | Next to the code it fixes, as `_windows.go` / `_unix.go` build-tagged files | Yes (canal) |
| Oracle note | canal's README or docs, wherever its data-source requirements are written | Yes (canal) |

## Code Style

canal's own conventions. Windows-only code goes in a build-tagged file next to the Unix one, never a `runtime.GOOS` branch in the middle of a function. Comments say only why:

```go
//go:build windows

package connector

// Oracle's client libraries are found at run time, as on other OSes; ...
```

## Testing Strategy

All on Windows 11 x64 (the WSL host), with binaries cross-compiled in WSL:
1. `anfra-canal.exe --version` runs.
2. canal's own DuckDB tests pass: `TestDuckdbSuite` and `TestDuckdbConnectorGating` (`internal/query/connector`), and canal-lake's tests (`internal/lake`), which load the `json` and `icu` extensions (`internal/lake/duckdb_pool.go:72-75`). That covers DuckDB, its Arrow streaming (the holistics fork's change) and extension download for `windows_amd64`.
3. **End to end:** `anfra-canal.exe query` started on a Unix socket in `%TEMP%`, as anfra starts it, runs `SELECT 1` on a DuckDB data source through its HTTP API, and answers the row. Ctrl-Break stops it cleanly (it exits, and its socket file is removed).
4. **The DLL build (for MotherDuck):** `anfra-canal.exe` built with `duckdb_use_lib` against DuckDB's official `duckdb.dll` of the same version as the Go bindings passes checks 2 and 3 again (with `duckdb.dll` beside the `.exe`), loads MotherDuck's `windows_amd64` extension, and, with a MotherDuck token the user sets in the environment, answers a query on MotherDuck.
5. **godror:** it must compile and link. Running Oracle queries needs Oracle's client libraries and an Oracle database, which stays out of this module; it's documented instead.
6. **No change on Linux/macOS:** if the build needs a code change in canal, canal's existing tests still pass on Linux (`go test ./...` for the touched packages).

## Boundaries

- **Always:**
  - Record every build attempt and its error in the findings, including the ones that fail.
  - Keep Linux and macOS builds exactly as they are.
  - Run Go builds and tests one at a time.
  - Keep the MotherDuck token out of the conversation, files and logs: the user sets it in the environment; programs read it, never print it.
- **Ask first:**
  - Installing the MinGW packages in WSL (`sudo apt-get`).
  - Pushing any branch or opening a PR in holistics/canal.
  - Changing holistics' `duckdb-go` fork or bumping DuckDB, `duckdb-go-bindings` or godror.
  - Dropping a data source driver from the Windows build (a Go build tag) if it can't build.
  - Touching canal's GitHub Actions (deferred).
- **Never:**
  - Merge the spike helpers.
  - Commit before you ask.
  - Change canal's behaviour on Linux or macOS to make Windows easier.
  - Disable a failing test to make a Windows run pass.

## Success Criteria

1. **Go/no-go evidence:** `docs/research/windows-canal-build.md` says whether `anfra-canal.exe` builds and runs on Windows, how long it took, and every problem met. If the build needs more than about 5 ED, this is where the plan stops.
2. On Windows 11 x64: `anfra-canal.exe --version` runs; canal's DuckDB connector tests and canal-lake's tests pass; the end-to-end `SELECT 1` over a Unix socket answers; Ctrl-Break stops it cleanly.
3. Any code change canal needed is in build-tagged files, and canal's existing tests still pass on Linux.
4. The Oracle requirement on Windows (Oracle's client libraries, as on other OSes) is documented.
5. **MotherDuck on Windows is decided on evidence:** the DLL build either passes check 4 (and becomes the Windows build), or the findings say why not, and MotherDuck is documented as not on native Windows yet.

## Open Questions

1. Is a MotherDuck account and token available for the DLL spike? Without one, it proves everything except the live MotherDuck connection.

## Plan updates this spec implies

To make in `WINDOWS-PLAN.md` once this spec is approved:
- `win-canal`'s "Graceful stop" row becomes a check only (canal already handles `os.Interrupt`, and there's no stdin watchdog).
- The "Sockets" row (TCP fallback bind) is dropped for now.
- The build spike runs as a WSL cross-compile.
- The release row and the canal PR check move to a later CI step.
