# canal on Windows: the build spike

> Status: **done** (2026-11-24). `anfra-canal.exe` builds and works on Windows with no canal code change, in ~1.65 ED; the Windows build links DuckDB's official `duckdb.dll`, for MotherDuck. See the [Conclusion](#conclusion). Spec: [SPEC-win-canal.md](../../SPEC-win-canal.md). Plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md), module `win-canal`.

anfra embeds one canal binary, `anfra-canal`, which runs as canal-query (every SQL query) and canal-lake (the query cache). It's Go plus C code built with cgo: DuckDB (through holistics' `duckdb-go` fork and the prebuilt `duckdb-go-bindings` libraries), Arrow's C data interface, and the Oracle driver godror. This doc answers: **does `anfra-canal.exe` build for windows-x64 and work there, and how much effort does it take?**

It decides the plan's go/no-go: if canal needs more than about **5 engineer-days** to build and run on Windows, the plan stops and is re-planned.

## Machine and toolchain

- Build, attempt 1: WSL (Ubuntu 24.04), cross-compiling.
  - Go 1.27.3.
  - MinGW-w64 from Ubuntu's packages: `gcc-mingw-w64-x86-64` and `g++-mingw-w64-x86-64` 13.2.0-6ubuntu1+26.1, `mingw-w64-x86-64-dev` 11.0.1, binutils 2.41.90. Two variants: `x86_64-w64-mingw32-gcc` (the default, Windows' thread model, "13-win32") and `x86_64-w64-mingw32-gcc-posix` ("13-posix").
- Build, attempt 2 (works): natively on the Windows host, driven from WSL through PowerShell.
  - Go 1.27.3 for Windows (`go1.27.3.windows-amd64.zip`, checksum matches go.dev's).
  - MinGW-Builds GCC 14.2.0, posix threads, SEH, UCRT (`x86_64-14.2.0-release-posix-seh-ucrt-rt_v12-rev0.7z` from `niXman/mingw-builds-binaries`): the exact toolchain DuckDB's Windows library was built with.
  - Both unpacked with Windows' `tar` into `C:\Users\nuita\wincheck\toolchain`: no installer, no admin.
- Run: Windows 11 25H2 (10.0.26200), x64, the WSL host.
- canal: `bdda8ef` (`origin/master`), worktree on `spike/win-sidecar-transport`. DuckDB: `duckdb-go-bindings` v0.10506.0 with its `windows-amd64` library, holistics fork `duckdb-go` v2.10506.0-streaming.1. godror v0.50.0.

## Build (`anfra-canal.exe`)

- Command (attempt 1): `GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc-posix CXX=x86_64-w64-mingw32-g++-posix go build -tags=duckdb_arrow -o anfra-canal.exe ./app/anfra-canal`
- Attempts, errors and fixes:
  1. **Ubuntu's MinGW GCC 13 (posix): compiles, fails to link.** Every Go and C package compiled, including DuckDB's cgo wrapper, Arrow and godror. The final link (83 s) failed on 5 undefined symbols:
     ```
     undefined reference to `__stdio_common_vswprintf'
     undefined reference to `__stdio_common_vsnprintf_s'
     undefined reference to `__imp___tzname'
     undefined reference to `__imp___timezone'
     undefined reference to `std::basic_streambuf<char, std::char_traits<char> >::seekpos(std::fpos<_Mbstatet>, std::_Ios_Openmode)'
     ```
     **Cause: a toolchain mismatch, not canal's code.** DuckDB's Windows library (`duckdb-go-bindings/lib/windows-amd64`) was built with `GCC: (x86_64-posix-seh-rev0, Built by MinGW-Builds project) 14.2.0` (read from the library), against Windows' **UCRT** C runtime: the four `__stdio_common_*` / `__tzname` / `__timezone` symbols exist only in UCRT. Ubuntu's MinGW is GCC 13 against the older **MSVCRT**, and its C++ library is a version behind (the `std::basic_streambuf` symbol). No linker flag fixes that: it needs a matching toolchain, GCC 14 with UCRT.
  2. **MinGW-Builds GCC 14.2.0 UCRT, natively on Windows: builds.** canal's source (`git archive` of `bdda8ef`) copied to `C:\Users\nuita\wincheck\canal-src`, then built with `dev/wincheck/build-windows.ps1` (canal spike branch): `CGO_ENABLED=1 CC=gcc CXX=g++ go build -buildvcs=false -tags=duckdb_arrow -o anfra-canal.exe ./app/anfra-canal`. 5.1 minutes, including downloading every Go module into a fresh cache. **No change to canal's code, no extra linker flags.**
- Windows DLLs it needs (`x86_64-w64-mingw32-objdump -p anfra-canal.exe | grep 'DLL Name'`): only Windows' own: `KERNEL32.dll`, `WS2_32.dll`, `RstrtMgr.DLL`, and the `api-ms-win-crt-*` set (UCRT, part of Windows 10 and 11). No `libstdc++-6.dll`, `libgcc_s_seh-1.dll` or `libwinpthread-1.dll`: the Go bindings already link MinGW's runtimes statically (`--static`). So anfra can embed and unpack it as one file.
- Result: **builds.** `anfra-canal.exe` is a 231 MB PE32+ x86-64 console program (not stripped). `anfra-canal.exe --version` on Windows prints `1.1.1`, `canal-query 2.28.0`, `canal-lake 2.10.0`.
- What it means:
  - canal's code needs **no change** to build for Windows: holistics' DuckDB fork, Arrow's cgo code and godror all compile and link.
  - **The toolchain must match DuckDB's**, GCC 14.2.0 from MinGW-Builds with UCRT, not just "a MinGW". That decides the later CI step: a Windows runner with this MinGW-Builds release, or a Linux runner with a matching UCRT cross-compiler. A DuckDB upgrade may move to a newer toolchain, so the build should read which one the library was made with (`strings libduckdb_static.a | grep 'GCC: '`), as this spike did.
  - The spec's "cross-compile in WSL" changed, with the user, to "build natively on Windows".

## canal's DuckDB tests on Windows (`internal/query/connector`)

- What ran: canal's own DuckDB tests, as canal's CI runs them (`.github/workflows/test.yml`, job `test_query_duckdb`). They need a running **canal-lake** on port 1324 (the query cache: every query's result is written to it), and `CI=true`, `QUERY_PREVIEW_ENABLED=true` (`internal/query/.env.ci`). Without canal-lake they fail on Linux too (`initialize cache writer: … dial tcp 127.0.0.1:1324: connection refused`), so that's test setup, not Windows.
  - Linux first: `go test -tags=duckdb_arrow -run TestDuckdb ./internal/query/connector`, with canal-lake running.
  - Windows: `go test -c -buildvcs=false -tags=duckdb_arrow ./internal/query/connector` built natively (0.7 minutes), run from `internal\query\connector` with `-test.run TestDuckdb -test.v`, while **`anfra-canal.exe lake`** (the T2 binary) served the cache on port 1324.
- Result: **pass, 12 of 12** on Windows, the same as Linux:
  ```
  --- PASS: TestDuckdbConnectorGating (0.00s)
  --- PASS: TestDuckdbStatementTimeoutFetch (0.09s)
  --- PASS: TestDuckdbArrowStreamingFailurePartWay (0.10s)
  --- PASS: TestDuckdbArrowStreamingCancelInterruptsFetch (2.00s)
  --- PASS: TestDuckdbSuite (5.14s)      (its 7 subtests pass too)
  PASS
  ```
- What it means:
  - DuckDB works on Windows through canal's connector: types, statement timeouts, and the **Arrow streaming that holistics' `duckdb-go` fork adds**, including a stream cancelled or failing part way.
  - **canal-lake runs on Windows too:** `anfra-canal.exe lake` started and stored every test query's result. (Over TCP here; over a Unix socket in the end-to-end check.)

## canal-lake's tests on Windows (`internal/lake`)

- What ran: canal-lake's own tests, as canal's CI runs them (job `test_lake`: `go test -tags=duckdb_arrow ./...` in `internal/lake`; only `h_canal/internal/lake` has tests). Nothing needs to be running. Linux first, then `go test -c` built natively on Windows and run from `internal\lake` with `-test.v`.
- Where the `json` and `icu` extensions went, and whether they loaded: `C:\Users\nuita\.duckdb\extensions\v1.5.6\windows_amd64_mingw\` (`json` 53 MB, `icu` 63 MB). DuckDB downloaded them the first time canal-lake started (in T3, when `anfra-canal.exe lake` ran), and canal-lake loads them at start (`internal/lake/duckdb_pool.go:72-75`). Note the platform: **`windows_amd64_mingw`**, DuckDB's separate extension builds for MinGW-built programs.
- Result: **pass, identical to Linux: 46 pass, 1 skip** (`TestFlightServerSuite/TestReadStreams`, which skips itself on every OS).
- What it means: canal-lake (the query cache, on DuckDB, Arrow Flight and Parquet) works on Windows as on Linux.

### Found here: MotherDuck's extension has no MinGW build

canal loads one more DuckDB extension for **MotherDuck** data sources (`internal/query/connector/motherduck_*`). DuckDB's extension repository has it for `windows_amd64` (DuckDB built with Microsoft's compiler, MSVC), but **not for `windows_amd64_mingw`**:

| `extensions.duckdb.org/<version>/<platform>/motherduck.duckdb_extension.gz` | `windows_amd64_mingw` | `windows_amd64` (MSVC) |
|---|---|---|
| v1.5.6 (canal's DuckDB) | 404 | 200 |
| v1.4.3, v1.3.2, v1.2.2 | 404 | 200 |

`json` and `icu` exist for both. A MinGW-built canal can't load an MSVC-built extension (their C++ libraries don't mix). So **MotherDuck data sources won't work on native Windows** with this build; local DuckDB files do. Not tested end to end (that needs a MotherDuck account and token); the missing download is enough to know it fails.

Not a blocker for the go/no-go, but a feature gap to decide on (see Go / no-go).

## End to end: canal-query over a Unix socket, then Ctrl-Break

- What ran: `canalquery.exe` (canal spike branch, `dev/wincheck/canalquery`) starts `anfra-canal.exe query` the way anfra does (`internal/sidecar/canalquery/canalquery.go`: `SOCKET` on a Unix socket in `%TEMP%`, `SKIP_HOLISTICS_DB=1`, `ENABLE_DUCKDB_CONNECTOR=1`, `EXPOSE_ERROR_CAUSES=1`, no canal-lake), in a process group of its own (`CREATE_NEW_PROCESS_GROUP`). It waits for `/health`, sends `POST /query` as anfra's `canalquery.Client.Stream` does (streamed result, header with Arrow types, cache skipped) for `select 1` on an in-memory DuckDB and for a query on `anfra-demo-data`'s `ecommerce.duckdb` (read-only), then sends **Ctrl-Break** (`GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT)`). The same program first passed on Linux, with SIGINT.
- Result: **pass,** the same answers as on Linux:
  ```
  query: select 1 as one
    header: {"__holistics_header__":true,"fields":["one"],"types":["INTEGER"],"arrow_types":[{"type":"int","bit_width":32,...}]}
    row: ["1"]
  query: select status, count(*) as orders from public.orders group by 1 order by 1
    row: ["cancelled","306"]
    row: ["completed","893"]
    row: ["shipped","301"]
  stopped after 0.0s
  socket removed
  PASS
  ```
  canal-query's own log ends with "Canal Query is ready", then "shutting down Canal Query": Ctrl-Break reached it as `os.Interrupt` and it took its normal shutdown. No `anfra-canal` process was left.
- What it means:
  - anfra's whole path to canal works on Windows: start on a Unix socket, health check, a streamed DuckDB query in anfra's request format, and a clean stop.
  - **Ctrl-Break needs no canal change**, as the spec expected. For `win-process-lifecycle`: start canal with `CREATE_NEW_PROCESS_GROUP` and stop it with `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`, as this program does.
  - canal-query's log file is UTF-16 when its stderr is redirected by PowerShell; that's PowerShell's redirection, not canal. anfra reads canal's stderr through a pipe, so it gets canal's own bytes.

## The DLL build, for MotherDuck (T9–T10)

MotherDuck's extension has no MinGW build (T4), so the static MinGW build can't load it. The other way to get DuckDB into a Go program on Windows is to **link DuckDB's official Microsoft-built `duckdb.dll` through its C API**, which works across compilers. That DuckDB is `windows_amd64` and loads MSVC extensions, MotherDuck's included.

### Build (T9)

- DuckDB's DLL: `libduckdb-windows-amd64.zip` from DuckDB **v1.5.6**, the version `duckdb-go-bindings` v0.10506.0 pins. sha256 `44cf59583f9951d2cb09b1bf115a63ecb2d8901e363903029d86c7d8683fe96a`, matching the one GitHub publishes. It holds `duckdb.dll`, `duckdb.lib` and the headers.
- The pinned bindings support it: with the build tag `duckdb_use_lib` they link `-lduckdb` (`cgo_dynamic.go`), from the folder given in `CGO_LDFLAGS=-L…`.
- Command: `dev/wincheck/build-windows-dll.ps1` (canal spike branch): the same toolchain as the static build, plus `CGO_LDFLAGS=-L<duckdb-dll>`, `go build -buildvcs=false "-tags=duckdb_arrow,duckdb_use_lib" ./app/anfra-canal`, then `duckdb.dll` copied beside the `.exe`. (In PowerShell the `-tags` argument must be quoted: a bare comma there is a list separator.)
- Result: **builds** in 1.0 minute, **with no change to canal's code**; `--version` prints `1.1.1`, canal-query 2.28.0, canal-lake 2.10.0.
- Files and the DLLs they need:

  | File | Size | Imports |
  |---|---|---|
  | `anfra-canal.exe` | 164 MB | `duckdb.dll`, `KERNEL32.dll`, UCRT's `api-ms-win-crt-*` |
  | `duckdb.dll` | 37 MB | `KERNEL32.dll`, `WS2_32.dll`, `RstrtMgr.DLL` only |
  | (static build, for comparison: `anfra-canal.exe`) | 231 MB | Windows' own DLLs only |

  `duckdb.dll` links Microsoft's C++ runtime into itself, so it needs **no Visual C++ Redistributable**: only Windows' own DLLs. Together the two files are 201 MB, a little smaller than the static build.

### Checks and MotherDuck (T10)

The same checks as the static build, with the DLL build (`anfra-canal.exe` and the test binaries built with `duckdb_use_lib`, each with `duckdb.dll` beside it):

| Check | Static (MinGW) build | DLL build |
|---|---|---|
| canal's DuckDB tests (T3), with canal-lake running | 12 of 12 pass | **12 of 12 pass** (canal-lake also the DLL build) |
| canal-lake's tests (T4) | 46 pass, 1 skip | **46 pass, 1 skip** |
| End to end (T5): Unix socket, `select 1`, the demo query, Ctrl-Break | pass | **pass**: the same rows, stopped in 0.0 s, socket removed |
| DuckDB's platform | `windows_amd64_mingw` | `windows_amd64` |
| `INSTALL motherduck` / `LOAD motherduck` | **fails:** `HTTP 404 … windows_amd64_mingw/motherduck.duckdb_extension.gz` | **ok:** loaded from `C:\Users\nuita\.duckdb\extensions\v1.5.6\windows_amd64\motherduck.duckdb_extension` |

The MotherDuck row comes from `mdext.exe` (canal spike branch, `dev/wincheck/mdext`), which runs `INSTALL` and `LOAD motherduck` through canal's own DuckDB driver, built each way.

- **A live MotherDuck query (tested later the same day, with the user's token):** `canalquery.exe` (now with MotherDuck queries when `TEST_MOTHERDUCK_TOKEN` is set) started each build as canal-query and sent two **read-only** queries with `{"dbtype": "motherduck", "dbconfig": {"token": …, "dbname": "my_db"}}`, the way anfra would:

  | Build | `select current_database(), 1` and a list of tables |
  |---|---|
  | **DLL build** | **Answered:** `["my_db","1"]`, then 5 tables from `information_schema` (e.g. `ecommerce.cities`); stopped cleanly on Ctrl-Break |
  | Static build | **Failed, clearly:** `load motherduck extension: HTTP Error: Failed to download extension "motherduck" at URL "…/v1.5.6/windows_amd64_mingw/motherduck.duckdb_extension.gz" (HTTP 404)` |

  The token was passed only in that one process's environment, never printed or written; canal-query's logs were checked afterwards and don't contain it. canal's own `TestMotherduck` tests weren't run: the token is read-write, and those tests may create objects in the account.
- What it means:
  - The DLL build is **as good as the static build** on every check, and **adds MotherDuck**: its extension installs and loads on Windows.
  - Holistics' `duckdb-go` fork works through DuckDB's C API with the official DLL, Arrow streaming included (the DuckDB tests cover it).
  - The cost: anfra ships `duckdb.dll` next to `anfra-canal.exe`, and its version must match the Go bindings on every DuckDB upgrade (here v1.5.6 for v0.10506.0).

### Can the DLL build be cross-compiled from Linux? (tested: no)

The DLL build links only DuckDB's C functions, so it might not need DuckDB's exact toolchain, and a Linux CI runner could build it. Tried in WSL with Ubuntu's MinGW (GCC 13, MSVCRT), the toolchain that couldn't link the static build in T2:

- It **builds** (43 s); `anfra-canal.exe` imports `duckdb.dll`, `KERNEL32.dll` and `msvcrt.dll`.
- It **crashes on its first query:** canal-query starts and answers `/health`, then dies while answering `select 1`, with no error in its log and the stream cut off.

The natively built DLL build (MinGW-Builds GCC 14.2.0, UCRT) answers the same query. So mixing MSVCRT in canal with the Microsoft runtime inside `duckdb.dll` breaks at run time even though it links. **The DLL build must also be built with a UCRT MinGW:** a Windows runner with MinGW-Builds, as here, or a UCRT cross-compiler; not Ubuntu's MinGW.

### If the user supplies `duckdb.dll` (tested)

Could anfra ship only `anfra-canal.exe` (the DLL build) and leave `duckdb.dll` for users to download? Tried with the DLL build of canal (built for DuckDB v1.5.6):

| `duckdb.dll` | Result |
|---|---|
| None anywhere Windows looks | **canal can't start**, not even `--version`: exit `0xC0000135` (DLL not found). Every query would fail, not only MotherDuck. |
| v1.5.6, in a folder the user put on `PATH` | Works |
| v1.4.3 (older) | **canal can't start:** `0xC0000139` (entry point not found): canal needs functions DuckDB 1.4.3 doesn't have |
| v2.0.1 (newer) | **canal starts, with no error, on the wrong DuckDB** (`v2.0.1`); then `LOAD motherduck` fails: "MotherDuck extension is not available for this DuckDB version (v2.0.1) and platform (windows_amd64)" |

So a user-supplied DLL works only when it's exactly the right version, in the right place, on every Windows machine, including for users who never use MotherDuck. It's not a workable way to support MotherDuck; anfra has to ship the matching DLL itself.

## Licenses

The DLL build means anfra ships DuckDB's own file, `duckdb.dll`, so DuckDB's license (MIT) notice must go with it. **But the static build ships DuckDB too, inside the binary, and so does every Linux and macOS release today.** Checked on 2026-11-24:

- anfra: `LICENSE` (Apache-2.0) and a two-line `NOTICE` (anfra's own copyright). No third-party notices, and nothing that generates them. The release (`build_release.yml`) ships only the gzipped binary.
- canal: no `LICENSE` or `NOTICE` file. anfra-node (holistics-core): no `LICENSE` file, no `license` field in `package.json`.
- The binaries contain no DuckDB copyright or license text.

What anfra redistributes inside its binaries, on every OS: DuckDB (MIT), Apache Arrow (Apache-2.0, which asks for its `NOTICE` to be passed on), godror and ODPI-C (Apache-2.0 / UPL), Bun's runtime with WebKit's JavaScriptCore (its own terms), and hundreds of Go and npm modules. The usual fix is a `THIRD_PARTY_NOTICES` file generated at build time (`go-licenses` for Go, a license tool for npm, hand-added entries for C libraries like DuckDB), shipped in the release archive and often printed by a command or a docs page.

**Decision rule (the user, 2026-11-24):** if anfra ships DuckDB's DLL, the license notices must be handled; otherwise the Windows build stays static and MotherDuck is marked as not supported on native Windows yet. Since the static build also ships DuckDB, the notices are needed either way; this is a gap for all OSes, to confirm with whoever owns licensing.

## Time log

| Task | Time spent |
|---|---|
| T1: toolchain, this doc | ~0.1 ED |
| T2, attempt 1 (Ubuntu MinGW) and finding the cause | ~0.1 ED |
| T2, attempt 2 (native build with DuckDB's toolchain), DLL check | ~0.2 ED |
| T3: DuckDB tests on Linux and Windows (finding they need canal-lake) | ~0.2 ED |
| T4: canal-lake's tests, extensions, the MotherDuck finding | ~0.15 ED |
| T5: the end-to-end check program, Linux and Windows | ~0.25 ED |
| **Total, T1–T5** | **~1 ED** of the 5 ED budget |
| T9: the DLL build | ~0.15 ED |
| T10: the DLL build's checks, the MotherDuck load check, a live MotherDuck query | ~0.3 ED |
| **Total, T1–T10** | **~1.45 ED** |
| T8: cross-compile check of the DLL build, conclusion, plan | ~0.2 ED |
| **Total, `win-canal`** | **~1.65 ED** |

## Conclusion

- **`anfra-canal.exe` builds and works on Windows x64 with no change to canal's code**, in about **1.5 engineer-days** of the 5 ED go/no-go budget (T1–T10, the cross-compile check included).
- **The Windows build ships as `anfra-canal.exe` + DuckDB's official `duckdb.dll`** (the DLL build), because only that DuckDB can load MotherDuck's extension. It passes everything the static build passes, and a live MotherDuck query answered with it. It needs **only Windows' own DLLs** (no Visual C++ Redistributable). Condition: DuckDB's license notice ships with it ([backlog](../todos/backlog.md#third-party-license-notices-in-releases)); otherwise the static build ships and MotherDuck isn't supported on native Windows yet.

What it hands on:

| To | What |
|---|---|
| The later CI step (canal's release row, its PR check) | Build natively on a **Windows runner** with **MinGW-Builds GCC 14.2.0, posix, SEH, UCRT** and Go for Windows (`dev/wincheck/build-windows-dll.ps1`): `CGO_ENABLED=1`, `CGO_LDFLAGS=-L<duckdb-dll>`, `-tags=duckdb_arrow,duckdb_use_lib`. Not Ubuntu's MinGW: it can't link the static build, and its DLL build crashes. Run canal's DuckDB tests (with canal-lake running) and canal-lake's tests there. |
| `win-release` | Embed `duckdb.dll` from DuckDB's `libduckdb-windows-amd64.zip` for the DuckDB version the Go bindings pin (v1.5.6 for `duckdb-go-bindings` v0.10506.0), checksum checked; check the version through canal (`select version()`), since a newer DLL starts silently on the wrong DuckDB. Check both files import only Windows' own DLLs. |
| `win-process-lifecycle` | Unpack `anfra-canal.exe` **and** `duckdb.dll` into one folder per version (Windows finds the DLL beside the program). Start canal with `CREATE_NEW_PROCESS_GROUP`; stop it with `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`: canal takes its normal shutdown and removes its socket. |
| A DuckDB upgrade | The DLL must match the new bindings exactly (an older DLL stops canal starting, a newer one runs the wrong DuckDB). The holistics `duckdb-go` fork must keep to DuckDB's C API. |
| Docs | Oracle on Windows needs Oracle's Instant Client (canal's README, on `feat/win-canal`). |

## Go / no-go

**Recommended: go.** Building and running canal on Windows took about **1 ED** of the 5 ED budget, with **no change to canal's code**: it builds with DuckDB's own toolchain, needs only Windows' own DLLs, passes canal's DuckDB and canal-lake tests, and works end to end the way anfra uses it. With `win-sidecar-transport` done, both halves of the go/no-go are in.

To decide alongside it: **MotherDuck on Windows** (no MinGW build of its extension). Options: document it as not supported on Windows yet; link DuckDB's MSVC `duckdb.dll` instead (MotherDuck works, but anfra ships a DLL next to canal); or ask DuckDB/MotherDuck for a MinGW build.

**Decided by the user (2026-11-24): go.** And for MotherDuck: the DLL build was tried (T9–T10), passed, and **is the Windows build**: anfra ships `duckdb.dll` beside `anfra-canal.exe`.
