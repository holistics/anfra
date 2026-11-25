# Tasks: win-canal

Plan: [win-canal-plan.md](win-canal-plan.md). Project plan: [WINDOWS-PLAN.md](../../WINDOWS-PLAN.md). Spec: [SPEC-win-canal.md](../../SPEC-win-canal.md).

canal's `.exe` files are built **natively on Windows** (`C:\Users\nuita\wincheck\canal-src`, toolchain in `…\wincheck\toolchain`, script `dev/wincheck/build-windows.ps1`), since Ubuntu's MinGW can't link DuckDB's library (T2). The check programs that need no cgo can still be cross-compiled in WSL. Everything runs from `C:\Users\nuita\wincheck`. Log the time each task takes in the findings doc: it decides the go/no-go.

---

## Phase 1: Build (the spike)

### - [x] T1: MinGW toolchain in WSL, and the findings doc

**Description:** Get a MinGW-w64 cross-compiler into WSL, and create the doc the results go in.

**Acceptance criteria:**
- [x] With the user's OK (spec: ask first), `gcc-mingw-w64-x86-64` and `g++-mingw-w64-x86-64` are installed (13.2.0, by the user, since sudo needs a password); `x86_64-w64-mingw32-gcc --version` prints "13-win32", and the `-posix` variant "13-posix".
- [x] `docs/research/windows-canal-build.md` exists: the question, the machine and toolchain versions, empty sections for the build, the DuckDB tests, canal-lake's tests and the end-to-end check, a time log, and the go/no-go. `docs/README.md`'s Research table lists it.

**Verification:**
- [x] Manual: the compiler's version prints; the doc's links resolve.

**Dependencies:** None

**Files likely touched:** `docs/research/windows-canal-build.md`, `docs/README.md`

**Estimated scope:** XS

### - [x] T2: Cross-compile `anfra-canal.exe` (built natively on Windows instead, see below)

**Description:** The deciding step. Build canal's anfra binary for Windows with cgo and DuckDB, and make it a single file that needs only Windows' own DLLs.

**Acceptance criteria:**
- [x] ~~The WSL cross-compile~~ compiles but can't link (Ubuntu's MinGW is GCC 13 with MSVCRT; DuckDB's library needs GCC 14.2.0 with UCRT). Changed with the user: built natively on Windows with MinGW-Builds GCC 14.2.0 UCRT; `go build -buildvcs=false -tags=duckdb_arrow ./app/anfra-canal` succeeds. Every failed attempt, its error and its fix are in the findings doc.
- [x] `x86_64-w64-mingw32-objdump -p anfra-canal.exe | grep 'DLL Name'` lists only Windows system DLLs (no `libstdc++-6.dll`, `libgcc_s_seh-1.dll`, `libwinpthread-1.dll`). If needed, static-link flags are added and recorded.
- [x] The exact build command (env and flags) is saved as `dev/wincheck/build-windows.ps1` on the canal spike branch (PowerShell, since it runs on Windows).

**Verification:**
- [x] Build succeeds (`build-windows.ps1`, 5.1 minutes).
- [x] Manual: the import table check above: KERNEL32, WS2_32, RstrtMgr and UCRT's `api-ms-win-crt-*` only.

**Dependencies:** T1

**Files likely touched:** `dev/wincheck/build-windows.sh` (canal spike branch), findings doc

**Estimated scope:** S

### Checkpoint A: It builds
- [x] `anfra-canal.exe` built and needs only Windows' own DLLs → continue to Phase 2.
- [ ] It can't be built → record why in the findings doc, stop, and review the go/no-go with the user.

---

## Phase 2: Prove it on Windows

### - [x] T3: `--version` and canal's DuckDB connector tests on Windows

**Description:** Show the binary starts, and DuckDB (with the holistics fork's Arrow streaming) works on Windows, using canal's own tests.

**Acceptance criteria:**
- [x] On Windows, `anfra-canal.exe --version` prints the version (`1.1.1`, canal-query 2.28.0, canal-lake 2.10.0).
- [x] `connector.test.exe`, built with the T2 command but `go test -c -tags=duckdb_arrow ./internal/query/connector`, runs `TestDuckdbSuite` and `TestDuckdbConnectorGating` on Windows; each test's result is in the findings doc. All `TestDuckdb*` tests ran: 12 of 12 pass, with `anfra-canal.exe lake` serving the cache, as canal's CI does.
- [x] Any failing test is analysed (Windows-specific, or failing on Linux too) before moving on; no test is disabled. Without canal-lake they fail on Linux too: test setup, not Windows.

**Verification:**
- [x] Tests pass on Linux first: `go test -tags=duckdb_arrow -run 'TestDuckdb' ./internal/query/connector` (with canal-lake running).
- [x] Manual, on Windows: `.\connector.test.exe -test.run 'TestDuckdb' -test.v`, output in the findings doc.

**Dependencies:** T2

**Files likely touched:** findings doc

**Estimated scope:** XS

### - [x] T4: canal-lake's tests on Windows

**Description:** Show canal-lake (the query cache, also DuckDB) works on Windows, including downloading and loading the `json` and `icu` extensions.

**Acceptance criteria:**
- [x] `lake.test.exe` (`go test -c -tags=duckdb_arrow ./internal/lake`, built natively) runs on Windows; each test's result is in the findings doc. 46 pass, 1 skip, as on Linux.
- [x] The doc says where DuckDB put the extensions on Windows (`%USERPROFILE%\.duckdb\extensions\v1.5.6\windows_amd64_mingw`), and that they loaded. Also found: MotherDuck's extension has no `windows_amd64_mingw` build.

**Verification:**
- [x] Tests pass on Linux first: `go test -tags=duckdb_arrow ./internal/lake` (46 pass, 1 skip).
- [x] Manual, on Windows: `.\lake.test.exe -test.v`, output in the findings doc.

**Dependencies:** T2

**Files likely touched:** findings doc

**Estimated scope:** XS

### - [x] T5: End to end: canal-query over a Unix socket, `SELECT 1`, Ctrl-Break

**Description:** Run canal the way anfra will: `anfra-canal.exe query` listening on a Unix socket in `%TEMP%`, answer a DuckDB query over its HTTP API, then stop it with Ctrl-Break.

**Acceptance criteria:**
- [x] `dev/wincheck/canalquery` (canal spike branch) starts `anfra-canal.exe query` with `SOCKET=<%TEMP%\…\canal-query.sock>`, `SKIP_HOLISTICS_DB=1` and `ENABLE_DUCKDB_CONNECTOR=1` (as `internal/sidecar/canalquery/canalquery.go` does), in a new process group (`CREATE_NEW_PROCESS_GROUP`).
- [x] It waits for `GET /health`, then `POST /query` with `{"sql": "select 1 as one", "dbtype": "duckdb", "dbconfig": {"path": ":memory:"}}` (the request shape from anfra's `canalquery.Client`), and gets the row back. Also one query on `anfra-demo-data`'s `example-data/ecommerce.duckdb`.
- [x] It sends Ctrl-Break (`GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`); canal-query exits within 5 s (0.0 s, through its normal shutdown), and its socket file is gone.

**Verification:**
- [x] Build: `GOOS=windows CGO_ENABLED=0 go build ./dev/wincheck/canalquery` (the check program itself needs no cgo); `go vet` clean for Windows and Linux.
- [x] Manual, on Windows: the program prints the row, the demo query's result, "stopped after Ns", "socket removed", `PASS`.

**Dependencies:** T2

**Files likely touched:** `dev/wincheck/canalquery/main.go`, `dev/wincheck/README.md` (canal spike branch), findings doc

**Estimated scope:** S

### Checkpoint B: Go / no-go
- [x] T2–T5 results and the time spent are in the findings doc (~1 ED).
- [x] Under about 5 ED and working → **go** (the user's decision, 2026-11-24): recorded in `WINDOWS-PLAN.md` (Decisions made; Where it stands). Also decided: try a DLL build for MotherDuck on Windows (T9–T10).

---

## Phase 2b: MotherDuck on Windows (the DLL spike)

### - [x] T9: Build `anfra-canal.exe` against DuckDB's official `duckdb.dll`

**Description:** MotherDuck's DuckDB extension has no MinGW build (T4), so the static MinGW build can't use it. Build canal against DuckDB's official Microsoft-built DLL instead, which loads `windows_amd64` extensions, MotherDuck's included.

**Acceptance criteria:**
- [x] `duckdb-shared-libs-windows-amd64` (for v1.5.6 the asset is named `libduckdb-windows-amd64.zip`) from DuckDB's release matching the Go bindings (DuckDB v1.5.6, `duckdb-go-bindings` v0.10506.0) is downloaded into `C:\Users\nuita\wincheck\toolchain\duckdb-dll`, checksum recorded.
- [x] `anfra-canal.exe` builds natively with `-tags=duckdb_arrow,duckdb_use_lib` and `CGO_LDFLAGS=-L<duckdb-dll>`, saved as `build-windows-dll.ps1` on the canal spike branch.
- [x] With `duckdb.dll` beside it, `anfra-canal.exe --version` runs, and the import table lists `duckdb.dll` plus only Windows' own DLLs. `duckdb.dll` itself imports only `KERNEL32`, `WS2_32`, `RstrtMgr`: no Visual C++ Redistributable needed.

**Verification:**
- [x] Build succeeds (the script, 1.0 minute).
- [x] Manual, on Windows: `--version`; `x86_64-w64-mingw32-objdump -p anfra-canal.exe | grep 'DLL Name'`.

**Dependencies:** Checkpoint B

**Files likely touched:** `dev/wincheck/build-windows-dll.ps1` (canal spike branch), findings doc

**Estimated scope:** S

### - [x] T10: The DLL build passes the checks, and loads MotherDuck

**Description:** Show the DLL build is as good as the static one, and adds MotherDuck.

**Acceptance criteria:**
- [x] With the DLL build: canal's DuckDB tests (T3) and canal-lake's tests (T4) pass on Windows, and the end-to-end check (T5) passes, each with `duckdb.dll` beside the `.exe`. 12/12, 46 + 1 skip, pass.
- [x] DuckDB loads MotherDuck's extension: it downloads into `%USERPROFILE%\.duckdb\extensions\v1.5.6\windows_amd64\` (`dev/wincheck/mdext`; the static build gets HTTP 404).
- [x] If the user sets a MotherDuck token in the environment (`TEST_MOTHERDUCK_TOKEN`, never printed): canal's `TestMotherduck` tests pass on Windows, or a `motherduck` query through the end-to-end program answers. Without a token, this is recorded as not tested. First run: no token, recorded as not tested. **Then, with the user's token: a live MotherDuck query answered with the DLL build** (read-only, `my_db`), and failed clearly with the static build (extension 404). `TestMotherduck` not run: the token is read-write.

**Verification:**
- [x] Manual, on Windows: the test binaries and `canalquery.exe` with the DLL build, outputs in the findings doc.

**Dependencies:** T9

**Files likely touched:** `dev/wincheck/mdext/main.go` (canal spike branch), findings doc

**Estimated scope:** S

### Checkpoint B2: Static or DLL
- [x] T10 passed → the Windows build uses the DLL (the user, 2026-11-24); recorded in `WINDOWS-PLAN.md` (Decisions made: MotherDuck on Windows; `win-process-lifecycle` unpacks `duckdb.dll`; `win-release` embeds it).
- [x] ~~T10 failed~~ (not applicable: T10 passed).

---

## Phase 3: Close

### - [x] T6: Code fixes found in T2–T5 (only if any) — **skipped:** canal's code needed no change (T2–T5, T9–T10)

*Skip, and mark skipped, if T2–T5 needed no code change.*

**Description:** Turn any workaround from the spike into a real change in canal, Windows-only, so Linux and macOS are untouched.

**Acceptance criteria:**
- [ ] Each fix is on a new local branch `feat/win-canal` (from `origin/master`), in `_windows.go` / `_unix.go` build-tagged files, never a `runtime.GOOS` branch.
- [ ] The T2 build and the T3–T5 checks pass again on Windows with the fixes.
- [ ] Not pushed: pushing or opening a PR in holistics/canal is asked first (spec).

**Verification:**
- [ ] Tests pass on Linux: `go test -tags=duckdb_arrow ./<touched packages>`, one command at a time.
- [ ] Manual, on Windows: T3–T5 re-run.

**Dependencies:** Checkpoint B

**Files likely touched:** depends on the fix (canal `feat/win-canal`)

**Estimated scope:** S (per fix)

### - [x] T7: Oracle note in canal's docs

**Description:** Say that Oracle data sources on Windows need Oracle's client libraries (Instant Client), as on other OSes.

**Acceptance criteria:**
- [x] The note is where canal documents data-source requirements (its README, or the godror connector's docs), on `feat/win-canal`: README's "Developing oracle connector" section, on a local `feat/win-canal` branch (worktree `anfra/tmp/canal-feat`, not pushed, not committed).
- [x] If godror had to be compiled out of the Windows build (T2), the note says Oracle isn't supported on Windows yet instead. Not needed: godror built in both Windows builds.

**Verification:**
- [x] Manual: the note reads clearly, and its links resolve (ODPI-C's `#windowsinstallation` section, checked against its source: it lists the Visual Studio redistributable the Instant Client needs).

**Dependencies:** T2

**Files likely touched:** canal `README.md` (or the connector's doc)

**Estimated scope:** XS

### - [x] T8: Findings final, plan updated

**Description:** Close the module: the findings doc's conclusion, and the plan updated with what was learned.

**Acceptance criteria:**
- [x] The findings doc ends with the conclusion, the total time spent, and what it hands on (the build command and flags for the later CI step, any build tag, the DLL check). Also tested: the DLL build can't be cross-compiled with Ubuntu's MinGW (it builds, then crashes on the first query), so CI needs a UCRT MinGW.
- [x] `WINDOWS-PLAN.md`: Where it stands marks `win-canal`'s build part done (CI deferred); its work rows and Estimate reflect what was learned; the Summary's status is updated.

**Verification:**
- [x] Manual: the spec's Success Criteria 1–5 are each met. 1: the findings doc has the go/no-go evidence (~1.65 ED, every problem met). 2: `--version`, DuckDB tests, canal-lake tests, end to end and Ctrl-Break pass on Windows 11, for both builds. 3: no canal code changed, so no build-tagged files were needed. 4: the Oracle note is in canal's README (`feat/win-canal`). 5: MotherDuck decided on evidence: the DLL build (live query answered).

**Dependencies:** T3–T7, T9–T10

**Files likely touched:** `docs/research/windows-canal-build.md`, `WINDOWS-PLAN.md`

**Estimated scope:** XS

### Checkpoint C: Complete
- [x] Spec success criteria 1–5 met.
- [x] If canal code changed: its tests pass on Linux. Not applicable: only canal's README changed.
- [ ] Ready for review with the user.
