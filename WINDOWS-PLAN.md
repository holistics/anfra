# Plan: run anfra natively on Windows (PowerShell / cmd)

The [brief](#summary) is for reviewing, deciding and estimating: the summary, the decisions needed, the estimate, the risks, the alternatives and where the work stands. The [detail](#modules) below it is for implementers: every piece of work, by module.

## Summary

- **Today:** anfra runs only on macOS and Linux. `install.sh` refuses other OSes, the release builds only 4 targets (linux/darwin × x64/arm64), and `GOOS=windows go build ./cmd/anfra` failed until `win-process-lifecycle` (on this branch, not released). A Windows user needs WSL or Docker first, and some features don't work there ([Alternatives](#alternatives-wsl-and-docker)). The README "one prompt" quickstart runs `curl | bash`, so it breaks at step 1 in PowerShell.
- **Goal:** a Windows user runs one PowerShell line, and then every anfra feature works the same as on Mac/Linux: the CLI, `anfra serve`, `anfra console`, publish/pull, update, and agent setup.
- **Recommendation:** build native Windows support (windows-x64). Ship WSL docs now as the interim path. Keep Docker for hosting.
- **Cost:** **≈ 31–45 engineer-days** one-time (signing left for later), with Claude helping ([Estimate](#estimate)); about 4–6 calendar weeks with 2 engineers. Plus ongoing upkeep, mostly in canal's Windows build and the release ([Long-term maintenance](#long-term-maintenance)).
- **Decision:** **go** (2026-11-24). Both checks passed: the transport, and canal building and running on Windows (~1 ED, no canal code change). MotherDuck on Windows: supported, by linking DuckDB's official `duckdb.dll` ([Decisions made](#decisions-made)).
- **Status:** **M1 reached** (2026-11-25): anfra runs on Windows 11. `win-sidecar-transport`, `win-canal` (CI deferred) and `win-process-lifecycle` are done: `version`, `validate`, `show` and `query`, with and without the cache, work with the sidecars, and no sidecar outlives anfra. Next: `win-anfra-node` ([Where it stands](#where-it-stands)).



## Decisions needed


| Decision | Options | Recommended | Needed by | What it changes |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Third-party license notices** | Generate and ship a `THIRD_PARTY_NOTICES` file with every release, on every OS (DuckDB, Arrow, godror/ODPI-C, Bun and the Go and npm modules); or don't | Ship them, for all OSes. **Note:** the static build also contains DuckDB, on Linux and macOS today, so this is needed whichever Windows build is chosen; nothing ships these notices now. Confirm with whoever owns licensing (Bun's WebKit part has its own terms). | Before `win-release`; it decides the MotherDuck row above | Ships: the Windows DLL build (MotherDuck). Doesn't ship: the static Windows build, MotherDuck not on native Windows yet. ~1–2 ED, all OSes, outside this plan's total: tracked in the [backlog](docs/todos/backlog.md#third-party-license-notices-in-releases) ([findings](docs/research/windows-canal-build.md#licenses)) |
| Windows versions supported | "Windows 10 1803+" (what AF_UNIX needs) or "Windows 10 22H2 and Windows 11, x64" | Windows 10 22H2 and Windows 11, x64 | `win-skills-docs` (docs), the test matrix | Windows 10 support ended in October 2025 (paid extended updates run to October 2026). anfra technically needs Windows 10 1803+, but a stated policy such as "Windows 10 22H2 and Windows 11, x64" keeps old Windows bugs from becoming upkeep |
| windows-arm64 | x64 only (Windows on ARM runs it through emulation, slower); or native arm64 | x64 only for now; revisit once canal is on DuckDB 2.0 | `win-release` | Native: +4–7.5 ED, once canal is on DuckDB 2.0 ([windows-arm64](#windows-arm64-for-a-later-decision)) |




## Decisions made


| Topic | Decision | Why |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go / no-go | **Go** (2026-11-24) | Both checks passed with no code change in any repo: the transport (`win-sidecar-transport`) and canal on Windows (`win-canal`'s build spike, ~1 ED of the 5 ED budget: builds, needs only Windows' own DLLs, passes canal's DuckDB and canal-lake tests, works end to end) ([findings](docs/research/windows-canal-build.md)) |
| MotherDuck on Windows | **Supported, by linking DuckDB's official `duckdb.dll`** (2026-11-24), **on one condition:** anfra ships DuckDB's license notice with it ([Decisions needed](#decisions-needed): third-party license notices). If notices aren't handled, ship the static MinGW build and mark MotherDuck as **not supported on native Windows yet**. | MotherDuck's DuckDB extension has no MinGW build, so the static build can't load it (HTTP 404). The DLL build passed every check the static build passed and loads MotherDuck's extension (`win-canal` T9–T10). Cost: anfra ships `duckdb.dll` beside `anfra-canal.exe`, version-matched to the Go bindings. A live MotherDuck query answered on Windows with the DLL build (read-only) ([findings](docs/research/windows-canal-build.md)). A `duckdb.dll` the **user downloads themselves** is no alternative: the DLL build can't start without it (Windows `0xC0000135`, DLL not found), which breaks every query, not only MotherDuck; an older DLL stops it starting (`0xC0000139`, entry point not found); a newer one starts silently on the wrong DuckDB, and MotherDuck then fails |
| Direction | Native is the goal. **WSL** is the interim path (docs and a skills note now). **Docker** stays for hosting `anfra serve` for a team ([docs/usage/docker.md](docs/usage/docker.md)), not for a person's daily work. | Native is the only option where a Windows user runs one PowerShell line and every feature works the same ([Alternatives](#alternatives-wsl-and-docker)) |
| Scope | Everything (all commands, serve, console, publish/pull, update, skills) | The goal is "the same as on Mac/Linux" |
| CPU | windows-x64 now; arm64 [still open](#decisions-needed) | Native arm64 needs canal on DuckDB 2.0 first ([windows-arm64](#windows-arm64-for-a-later-decision)) |
| Repos | All 3: anfra, canal (anfra-canal), holistics-core (anfra-node) | anfra embeds the other two; no Windows sidecars, no Windows anfra |
| Install | `install.ps1` only (`irm https://anfra.ai/install.ps1 \| iex`) | The same pattern as `install.sh`, and what the one-prompt quickstart needs |
| Transport | Keep Unix sockets (AF_UNIX) on Windows; no fallback transport needed | All five `win-sidecar-transport` checks passed on Windows 11, with no code change in any repo: anfra → anfra-node, canal-query → canal-lake (gRPC), canal's restart on a leftover socket, socket path length, anfra-node's stdin watchdog ([findings](docs/research/windows-sidecar-transport.md)) |
| Signing | **Unsigned first, signing later** (2026-11-27) | `install.ps1` downloads `anfra.exe` through PowerShell, which doesn't mark it as from the internet, so SmartScreen doesn't warn; anfra's sidecars and `anfra update` don't come through a browser either. `install.ps1` verifies the release's SHA-256 instead. What unsigned costs: a browser download from GitHub warns ("More info → Run anyway"), and antivirus false alarms are more likely. Revisit when those show up, or manual downloads matter: Azure Trusted Signing (~$10/month, if Holistics is eligible) or an OV certificate (~$220–440/year, plus a cloud signing service for CI). |
| Windows download format | **`anfra-windows-x64.zip`** (2026-11-27), `anfra.exe` at its top | Windows opens a `.zip` with a double-click, for an install by hand; `anfra update` unzips it on Windows. Linux and macOS keep their `.gz` |
| Updates verify a checksum | **On every OS** (2026-11-27): `anfra update` installs only a download whose sha256 matches the release's `SHA256SUMS`, which `build_release.yml` publishes | Windows ships unsigned, with `install.ps1` checking SHA-256; `anfra update` makes the same promise everywhere, where it checked nothing before |
| Developing anfra | Windows users can *use* anfra natively; contributors still develop in WSL. CI gets a Windows job. | Users need it; contributors don't |
| Line endings | `anfra init` writes `.gitattributes` (LF), and publish/pull hash text files with CRLF→LF | No change to the hash format or the server |
| Tests on Windows | CI runs `GOOS=windows go vet ./...`, and `go test` on Windows only for packages with Windows logic | Catches Windows bugs in the PR that causes them, without fixing tests that only fail because of test setup |
| Risk scope | Only things we decide or build | Company IT policies, SmartScreen's reputation system, antivirus vendors and Docker's licensing are outside our control |




## Estimate

The only place effort is added up. Every figure assumes **Claude helping**. Each module's number is the sum of its work rows in the [detail](#work-by-module).


| Module | ED | Confidence | After it ships |
| ------------------------------------------------------------------ | ----------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------- |
| `win-sidecar-transport` | 1.5–2 | Done: all five checks passed | Done once |
| `win-canal` (build spike 1–2.5, DLL spike 1–2, then the rest 1.2–2.1) | 3.2–6.6 | High: built and tested on Windows in ~1.65 ED (both spikes: ~1 ED planned 1–2.5, ~0.45 ED planned 1–2); what's left is the release row, with CI | Keeps costing effort, often |
| `win-anfra-node` | 1.1–1.75 | Medium: Bun cross-builds and there are no native modules; the Windows watchdog is untested | Keeps costing effort, sometimes |
| `win-process-lifecycle` | 3.5–4.5 | Medium: Windows API work; anfra has never run on Windows | Done once; new code must follow its rule |
| `win-files-auth` | 2.45–3.2 | Medium: the list comes from reading code; more may appear | Done once; new code may repeat its mistakes |
| `win-line-endings` | 1.5–2 | Medium: touches publish hashing | Done once; new code must follow its rule |
| `win-terminal` | 1.5–2.25 | Medium: mostly manual testing | Mostly done once |
| `win-release` | 4–5 | Medium: the Windows CI build is new; signing (1–1.5) left for later | Keeps costing effort, often |
| `win-self-update` | 0.6–1.25 | High: the library already handles Windows | Mostly done once |
| `win-skills-docs` | 2.7–3.6 | High: text work | Keeps costing effort, sometimes |
| Guard: Windows smoke tests in canal's and holistics-core's CI | 0.5–1 | High | — |
| Guard: CI check of the release binaries' minimum OS versions | 0.25–0.5 | High | — |
| End-to-end testing on Windows ([Verification](#verification)) | 3–4 | Medium | — |
| **Subtotal** | **≈ 26–37.5** |  |  |
| Buffer for review, CI waits, release coordination, surprises (20%) | 5–7.5 |  |  |
| **Total** | **≈ 31–45 ED (6–9 engineer-weeks)** |  |  |


Not in the total:


| Add-on | ED |
| ----------------------------------------------------------------------------------------------- | ----------------------------------- |
| Native windows-arm64, if chosen ([detail](#windows-arm64-for-a-later-decision)) | +4–7.5, once canal is on DuckDB 2.0 |
| Interim: Windows via WSL, docs and a skills note ([Alternatives](#alternatives-wsl-and-docker)) | 0.5–1 |


**How to read it:**

- **ED** = engineer-day: one engineer who knows the code, with Claude helping, and a Windows 11 machine or VM.
- Each figure is a range: the low end is "works first time", the high end covers the Windows surprises that are likely.
- Each work row was estimated by what Claude speeds up. Code, tests, scripts and docs go about 2–3× faster. Manual testing on Windows, CI round trips (the canal MinGW/DuckDB build), the signing setup, steps a person does (the Cloudflare rule) and cross-team coordination barely speed up. Claude can write Windows code from WSL but can't fully test it; a person on Windows, or the Windows CI job, does that. The speed-ups are judgment, not measured.
- Code review, waiting on CI and release coordination aren't in the module rows; the 20% buffer covers them.
- **Calendar time:** with 2 engineers (one on anfra, one on canal + anfra-node, then both on anfra), about **4–6 weeks**.
- **Re-estimate at M1,** when anfra first runs on Windows.



## Risks

Only risks in things we decide or build: our code, our builds, our release process and our own teams. Things outside our control (company IT policies, Windows SmartScreen's reputation system, antivirus vendors, Docker's licensing) are out of scope.

No known blocker. What's proven so far: anfra → anfra-node over a Unix socket on Windows (`win-sidecar-transport`, check 1). Go and Bun cross-build for Windows, and DuckDB ships `windows-amd64` libraries.


| # | Risk | Severity | Mitigation |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------ |
| 1 | **canal may not build for Windows easily:** C code (the DuckDB fork, the Oracle driver) with MinGW. No Windows canal means no Windows anfra. | **High** | `win-canal`'s build spike, before anything else is committed, with a go/no-go |
| 2 | **The agent experience:** the skills are written for bash; Codex and Cursor use PowerShell on Windows, and AQL quoting differs | Medium | Run the README one-prompt flow with each agent at M1, not only at the end (Verification 9) |
| 3 | **First-run size:** ~330 MB download, then ~300 MB of sidecars written on first run | Low–Medium | Measure it in the build spike's run and at M1; say it in the docs |
| 4 | **Release drift:** canal already silently needs macOS 26 and glibc 2.38 ([the drift we've already seen](#the-drift-weve-already-seen)) | Medium | The minimum-OS check, added early in `win-release` |




## Alternatives: WSL and Docker

Judged only on what we control:


|  | Native (this plan) | Docker | WSL |
| ----------------------------------- | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Install | One PowerShell line | Docker, then `docker run` with a mounted folder | WSL, then the Linux install inside it |
| `anfra auth login` / publish / pull | Works (after `win-files-auth`) | **Broken today:** the login callback listens on `127.0.0.1` inside the container (`internal/cliauth/oauth_login.go:73`), which the Windows browser can't reach. Fix: ~1–2 ED plus server work. | Should work: WSL forwards Windows' `localhost` into Linux. Verify once. |
| Data App live reload | Works | Often broken for repos on `C:\`, and file access is slow | Works for repos inside WSL |
| Agent commands | `anfra …`, as on Mac/Linux | `docker exec anfra-serve anfra …` everywhere; skills need a Docker version | `anfra …`, with the agent run inside WSL |
| Line endings (`win-line-endings`) | Needed | Also needed (files edited on Windows are CRLF) | Needed if files are edited from Windows |
| Our one-time cost | ~31–45 ED | ~2–4 ED (login fix, skills, docs) | ~0.5–1 ED (docs, skills note) |
| Our ongoing cost | ~3–6 ED a quarter, across 3 repos | ~0 | ~0 |


Tested by hand (Docker on Windows, with Claude installing Docker from one prompt):

- The **WSL** backend works. The **Docker VMM** backend (Docker 4.86+) works. The **Hyper-V** backend should work, but wasn't tested: it needs Windows Pro.
- `docker exec -it anfra-serve anfra pull` works.
- The sign-in is lost when the container is removed.
- `anfra auth login` can't open the Windows browser from the container. The open question was whether the approval gets back into the container: it doesn't, because anfra waits for the browser to call back to `127.0.0.1:<random port>` inside the container (`internal/cliauth/oauth_login.go:73`), and a random port can't be published in advance. That's the "Broken today" row above.
- There's no native install for cmd or PowerShell: the reason for this plan.

**Decision:** native stays the goal: it's the only option where a Windows user runs one PowerShell line and every feature works the same. **WSL is the interim path**, shipped now as docs and a skills note (~0.5–1 ED), so Windows users have a working setup while native is built. **Docker stays for hosting** `anfra serve` for a team ([docs/usage/docker.md](docs/usage/docker.md)), not for a person's daily work.

## Where it stands


| Module | State | Docs |
| ---------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| `win-sidecar-transport` | **Done.** All 5 checks passed (T2–T6); no fixes needed in any repo (T7–T9 skipped); decision: Unix sockets. | [Spec](SPEC-win-sidecar-transport.md), [tasks](tasks/archive/win-sidecar-transport-todo.md), [findings](docs/research/windows-sidecar-transport.md) |
| `win-canal` | **Done** (CI deferred). canal builds and works on Windows with no code change; the Windows build links DuckDB's official `duckdb.dll`, for MotherDuck (live query tested). Left for the CI step: canal's Windows release row and PR check. | [Spec](SPEC-win-canal.md), [tasks](tasks/archive/win-canal-todo.md), [findings](docs/research/windows-canal-build.md) |
| `win-process-lifecycle` | **Done** (M1, 2026-11-25). anfra compiles and runs on Windows 11 with its sidecars: stops them cleanly with or without a console, ends them with anfra (Job Object), unpacks a folder per version with `duckdb.dll`, keeps the socket's folder private. The `GOOS=windows go vet` CI step waits for the CI work. | [Spec](SPEC-win-process-lifecycle.md), [tasks](tasks/archive/win-process-lifecycle-todo.md), [findings](docs/research/windows-sidecar-transport.md#later-stopping-canal-with-ctrl-break-with-and-without-a-console) |
| `win-anfra-node` | **Code done** (2026-11-25); its release row and Windows smoke test wait for the CI work. anfra-node shuts down on `SIGBREAK` (holistics-core branch `feat/anfra-node-windows-stop`, not pushed), and anfra stops it with Ctrl-Break, so no `watchdog.parent_death` after each command. Checked on Windows with Bun 1.4.4, as the release uses. anfra's change waits for anfra's pin to an anfra-node release with the handler. | [Spec](SPEC-win-anfra-node.md), [tasks](tasks/archive/win-anfra-node-todo.md), [findings](docs/research/windows-sidecar-transport.md#later-anfra-node-with-bun-144) |
| `win-files-auth` | **Done** (2026-11-27). Login, pull and publish checked on Windows against `acme`. Logins save and stay private (they all failed before), saving survives readers, exports get `file:///C:/` links, Data Apps can't reach hidden files (they could) and get Windows' time zone, pull refuses names Windows can't hold, private files are private on Windows too. Login checked on Windows by the user. | [Spec](SPEC-win-files-auth.md), [tasks](tasks/archive/win-files-auth-todo.md) |
| `win-line-endings` | **Done** (2026-11-27). publish and pull read text files as LF, so a file saved on Windows publishes only for real changes and merges cleanly; pull keeps each file's own line ending; `anfra init` writes `.gitattributes`; `config.yml` and `AGENTS.md` keep theirs. Round trip on Windows with `acme`. | [Spec](SPEC-win-line-endings.md), [tasks](tasks/archive/win-line-endings-todo.md) |
| `win-terminal` | **Done** (2026-11-27), checked by hand in Windows Terminal only (the user decided): colors, Unicode trees, the console, typed answers to questions (fixed: `cancelreader`'s raw mode), the publish prompt. Also fixed on the way: the home folder taken for a repo (every OS), and the test copies' update-check cascade. | [Spec](SPEC-win-terminal.md), [tasks](tasks/archive/win-terminal-todo.md) |
| `win-self-update` | **Done** (2026-11-27). `anfra update` verifies the release's `SHA256SUMS` on every OS (published by `build_release.yml` from now on), unzips `anfra-windows-x64.zip` on Windows, gets past an `.anfra.exe.old` still in use, and advises `install.ps1` on failure. Checked on Windows with a local fake release: from PowerShell, a wrong checksum refused, and inside `anfra console`, which restarts on the new anfra (fixed: it didn't restart on Windows). | [Spec](SPEC-win-self-update.md), [tasks](tasks/archive/win-self-update-todo.md) |
| `win-release`, `win-skills-docs` | Not started | — |
| Interim WSL docs | Not started | — |
| Code signing | Not needed for the first release: unsigned first, signing later (2026-11-27) | [Decisions made](#decisions-made) |




## Milestones

1. `win-sidecar-transport` **and** `win-canal`**'s build spike**, in parallel. Start the interim WSL docs the same day. **Go/no-go** after both ([Decisions needed](#decisions-needed)).
2. **M1: it compiles** (reached 2026-11-25, the CI step deferred with the rest of CI) — `win-process-lifecycle` + the `GOOS=windows go vet` CI step. `anfra version`, `validate`, `show` and `query` work on Windows, with a locally built `anfra-node.exe` and the build spike's `anfra-canal.exe`. Re-estimate the rest here. (~2 weeks)
3. **M2: sidecars ship** — `win-canal` and `win-anfra-node`, then `build_release.yml` produces `anfra-windows-x64` (unsigned at first). Can run in parallel with M1. (~2 weeks)
4. **M3: everything works** — `win-files-auth`, `win-line-endings`, `win-terminal`, `win-self-update`, and the Windows CI test job (`win-release`). (~2–3 weeks)
5. **M4: users can get it** — `install.ps1`, skills, README and docs (`win-release`, `win-skills-docs`), end-to-end testing, announce. (~1–2 weeks)

---



## Modules

The work below, split into modules that each ship and are checked on their own. Module ids are fixed; specs and tasks refer to them.


| Module id | What it covers | Repo | Depends on | Spec |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- | ------------------------------------------------------- | --------------------------------------------------------------------------- |
| `win-sidecar-transport` | Prove that Unix sockets work on Windows between anfra (Go), anfra-node (Bun) and canal (Go and gRPC). Fix socket path length, the stale-socket check, and the gRPC `unix:` address with `C:\`. | all 3 | — | [SPEC-win-sidecar-transport.md](SPEC-win-sidecar-transport.md), in progress |
| `win-process-lifecycle` | anfra compiles on Windows. Job Object, clean stop, `.exe` sidecar unpack, `detach` and console restart, the console's background server stop, serve's busy-port fallback. | anfra | `win-sidecar-transport` | [SPEC-win-process-lifecycle.md](SPEC-win-process-lifecycle.md) |
| `win-canal` | canal builds for windows-x64 (MinGW, DuckDB fork) and stops cleanly on Ctrl-Break (already handled). Its Windows CI (release row, PR check) and signing come later. | canal | — (the build spike); `win-sidecar-transport` (the rest) | [SPEC-win-canal.md](SPEC-win-canal.md) |
| `win-anfra-node` | anfra-node builds for windows-x64 (Bun cross-compile; signing later), `SIGBREAK`, path fallback (the watchdog already works on Windows). A Windows smoke test in holistics-core's CI. Input: `win-sidecar-transport`'s check 5 (stdin watchdog) result. | holistics-core | `win-sidecar-transport` | — |
| `win-files-auth` | Saved logins, safe file replace, export `file:` links, `RemoveLegacy` guard, path checks (security), time zone, DuckDB lock message. | anfra | `win-process-lifecycle` | — |
| `win-line-endings` | `.gitattributes` from `init`, reading files as LF on publish and pull (hashes, uploads, pull's merge), keeping each file's own line ending when anfra edits it. | anfra | `win-process-lifecycle` | — |
| `win-terminal` | Colors in old cmd and PowerShell, UTF-8 trees, console keys, prompts. | anfra | `win-process-lifecycle` | — |
| `win-release` | The Windows target in `build_release.yml`, embedding the Windows sidecars, `install.ps1` (signing later), the Windows CI job and its tests, the binaries' minimum-OS check. | anfra | `win-process-lifecycle`, `win-canal`, `win-anfra-node` | — |
| `win-self-update` | The `anfra update` asset name, the `.old` exe left behind, the reinstall hint. | anfra | `win-release` | — |
| `win-skills-docs` | Skill hooks (bash JSON parse, `.ps1` versions), skill text for PowerShell. Install docs: README, the anfra.ai Installation page, the Cloudflare redirect for `anfra.ai/install.ps1`, release notes. Design and dev docs. | anfra | `win-release` | — |


Build order:

```
win-sidecar-transport, win-canal's build spike              (in parallel; go/no-go after both)
  → win-process-lifecycle, win-canal, win-anfra-node        (in parallel)
  → win-files-auth, win-line-endings, win-terminal          (in parallel)
  → win-release
  → win-self-update, win-skills-docs                        (in parallel)
```



## Work by module

Each module's work, with per-row effort in engineer-days (ED), with Claude helping. Totals are only in the [Estimate](#estimate).

### `win-sidecar-transport`

Checks, on a real Windows machine, that anfra, anfra-node and canal can keep talking over Unix sockets ([research](docs/research/windows-sidecar-transport.md)). Spec: [SPEC-win-sidecar-transport.md](SPEC-win-sidecar-transport.md). Tasks: [tasks/todo.md](tasks/archive/win-sidecar-transport-todo.md).


| Check | ED |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| Build anfra-node with `BUN_COMPILE_TARGET=bun-windows-x64` (`holistics-core/apps/anfra-node/scripts/build-sidecar.sh`). Start it with `--socket=C:\...\x.sock`. Connect from a tiny Go program with `net.Dial("unix", ...)` and send one JSON-RPC call. | 0.5 |
| Socket path length: a Unix socket path must stay under about 108 bytes. Try a long user name under `%TEMP%`. If it's too long, use a shorter folder (e.g. `%LOCALAPPDATA%\a\<random>`). | 0.25 |
| canal: does grpc-go accept `unix:C:\...` for canal-query → canal-lake? (The `C:` may confuse grpc's address parsing; `unix:///C:/...` or a custom dialer may be needed.) Does `listen.go`'s stale-socket check (`fi.Mode()&os.ModeSocket`) see the socket file on Windows? | 0.5–1 |
| Bun's stdin-EOF watchdog: does `fdIsPipe(0)` (`src/rpc/watchdog.ts`) return true for a Windows anonymous pipe? | 0.25 |


**Result: passed.** All five checks passed on Windows 11 with no code change in any repo, so the transport stays Unix sockets and no fallback is needed ([findings](docs/research/windows-sidecar-transport.md)). Check 5's result feeds `win-anfra-node` (no watchdog change) and `win-process-lifecycle` (closing stdin stops anfra-node).

### `win-canal`



#### Build spike (first, in parallel with `win-sidecar-transport`)

The biggest risk in the plan: canal is C code as well as Go (holistics' DuckDB fork, the Oracle driver), built with MinGW. If it won't build or run on Windows, there's no Windows anfra, since every query needs canal. The rest of `win-canal` would find out weeks in; this spike finds out first. It runs in parallel with `win-sidecar-transport` and needs no other module.


| Work | ED |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| Cross-compile in WSL with MinGW-w64 (`x86_64-w64-mingw32-gcc`/`-g++`): `GOOS=windows CGO_ENABLED=1 go build -tags=duckdb_arrow ./app/anfra-canal`, plus canal's DuckDB tests built with `go test -c` (`internal/query/connector`, `internal/lake`). Check the holistics DuckDB fork keeps upstream's `windows-amd64` bindings, and that godror builds. Spec: [SPEC-win-canal.md](SPEC-win-canal.md). | 0.5–1.5 |
| Run on Windows: `anfra-canal.exe --version`; canal's DuckDB tests (connector and canal-lake, which load the `json` and `icu` extensions); end to end, `anfra-canal.exe query` on a Unix socket answers `SELECT 1` on DuckDB; Ctrl-Break stops it cleanly. | 0.5–1 |


**Result: done, go** (2026-11-24): canal built and passed its tests on Windows in ~1 ED, with no code change; then the DLL build added MotherDuck ([findings](docs/research/windows-canal-build.md#conclusion)). The rule it was checked against: **go/no-go after** `win-sidecar-transport` **and this spike:** if canal needs more than about **5 ED** to build and run on Windows, or a transport check fails with no workable fallback, stop and re-plan before the anfra modules start. Otherwise go ahead. The spike's `anfra-canal.exe` is also what M1 uses to run `anfra query` on Windows before `win-canal` ships a release build.

#### Graceful stop, extensions, Oracle, the DLL spike; release build later

Note: the local `/home/nui/works/canal` checkout is old. Work from `origin/master`.


| Work | ED |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| **Later, with CI (deferred):** `.github/workflows/build_and_release_binaries.yml`: a `windows-x64` row on a **Windows runner** with **MinGW-Builds GCC 14.2.0 (posix, SEH, UCRT)** and Go for Windows, building the DLL build as `dev/wincheck/build-windows-dll.ps1` does (`CGO_LDFLAGS=-L<duckdb-dll>`, `-tags=duckdb_arrow,duckdb_use_lib`), output `anfra-canal-windows-x64.exe` plus the matching `duckdb.dll`, and sign them. Not Ubuntu's MinGW: it can't link the static build, and its DLL build crashes on the first query ([findings](docs/research/windows-canal-build.md#conclusion)). Also run canal's DuckDB tests (with canal-lake running) and canal-lake's tests there. | 0.75–1.5 |
| DuckDB extensions (`json`, `icu`, motherduck) download for `windows_amd64` at runtime. Check `DUCKDB_EXTENSION_DIRECTORY` (`internal/query/connector/duckdb_shared.go:126`) with Windows paths. | 0.25 |
| Graceful stop: check only. canal already stops on `os.Interrupt` (`internal/query/start.go:56`, `internal/lake/start.go:38`), which is how Go delivers Windows' Ctrl-Break, so it needs no change. No stdin watchdog: anfra's Job Object (`win-process-lifecycle`) stops canal when anfra dies. | 0.1–0.25 |
| Oracle (godror) builds with gcc; at runtime it needs the Oracle Instant Client DLLs, as on other OSes. Document this. | 0.1 |
| **DLL spike, for MotherDuck on Windows:** build `anfra-canal.exe` against DuckDB's official Microsoft-built `duckdb.dll` (`duckdb_use_lib`, same DuckDB version as the Go bindings); re-run canal's DuckDB tests, canal-lake's tests and the end-to-end check; load MotherDuck's `windows_amd64` extension, and connect to MotherDuck with a real token if one is available. Pass → the Windows build uses the DLL; fail → keep the static build, and document MotherDuck as not on native Windows yet. | 1–2 |




### `win-anfra-node`


| Work | ED |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `.github/workflows/anfra_node_build_release.yml`: add `windows-x64` (Bun cross-compiles `bun-windows-x64` from Linux; there are no native modules, and `@holistics/aml` already ships `win32-x64-msvc` plus a WASM fallback). Output `anfra-node-windows-x64.exe` (signing later, as anfra's). Check that Bun 1.2.10 is fine (the research says yes for sockets), or bump it. | 0.5–1 |
| `src/rpc/server.ts:51-113`: keep `listen(path)` with a normal file path; never pass a `\\.\pipe\` name (see the research). Make sure the stale-file `fs.unlinkSync` works on Windows. | 0.1–0.25 |
| `src/main.ts:89-90`: also handle `SIGBREAK`, for anfra's Ctrl-Break stop. The watchdog needs no change: `win-sidecar-transport`'s check 5 showed stdin EOF stops anfra-node on Windows, cleanly, after a crash, and after `taskkill /F` of anfra; the parent-PID check never fires there, but never misfires either. | 0.25 |
| `src/amql-shell/source/selectors.ts:10-14`: the `'/repo'` fallback should be a real Windows path. | 0.25 |

**Result: code done** (2026-11-25), tested on Windows 11 ([tasks](tasks/archive/win-anfra-node-todo.md)). What changed from the rows above:

- **`SIGBREAK`:** Bun 1.4.4 delivers Ctrl-Break to `process.on('SIGBREAK')` on Windows; anfra-node now shuts down on it, and anfra stops every sidecar with Ctrl-Break, closing anfra-node's stdin only after it exits. That ends the `watchdog.parent_death` warning each command logged.
- **Bun 1.4.4** (the release's, not the research's 1.2.10): the socket, stdin-EOF and stale-socket checks pass again ([findings](docs/research/windows-sidecar-transport.md#later-anfra-node-with-bun-144)). No bump needed.
- **`server.ts`:** no change; the stale socket file is removed on Windows too.
- **`selectors.ts`:** no change. Without a repo folder it uses POSIX paths throughout, so `/repo` never meets a Windows path; on Windows, `validate "models/**"` works and `"..\x"` is refused.
- **After `taskkill /F` on anfra,** the Job Object ends anfra-node before its watchdog can warn: nothing is left, but nothing is logged either. The watchdog stays as the backup when the job can't be joined.
- **Left for the CI work:** the `windows-x64` row in `anfra_node_build_release.yml` (cross-compiling with Bun works: every check here used such a build), signing, and the Windows smoke test.
- **Order of merging:** the holistics-core change ships in an anfra-node release; anfra's pin moves to it; then anfra's stop change can merge.

### `win-process-lifecycle`


| Work | ED |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| New `internal/sidecar/proc_windows.go`; change `proc_other.go` to `//go:build !linux && !windows`. Use `CREATE_NEW_PROCESS_GROUP \| CREATE_NO_WINDOW`, so Ctrl-C in the terminal doesn't kill the sidecars and no console window pops up. Put each child in a **Job Object** with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (`golang.org/x/sys/windows`): if anfra dies, Windows kills the sidecars too. This replaces Linux's `Pdeathsig` and the process group. | 1–1.5 |
| `internal/sidecar/process.go` `Close`: move the kill into a per-OS function. On Windows, close stdin first (anfra-node's graceful stop), then send `CTRL_BREAK_EVENT` (`GenerateConsoleCtrlEvent`) for canal, wait 3 s, then `TerminateJobObject`. Unix keeps SIGTERM → `syscall.Kill(-pid)`. | 0.5 |
| `internal/sidecar/binary.go`, **Windows only** (Linux/macOS keep `sidecars/<name>-<hash>`, decided with the user): unpack each version into its own folder, `sidecars\<name>-<hash>\<name>.exe`, so the `.exe` runs and a second version sits beside the first; write through a temp name of its own, so two anfra processes don't collide; prune skips a folder still in use (a running program and its DLL are locked). | 0.5 |
| `internal/sidecar/anfracanal` (and the unpack above): Windows builds also embed DuckDB's `duckdb.dll` and unpack it into canal's version folder, beside `anfra-canal.exe`, where Windows finds it. A test that canal starts with it. | 0.5 |
| `cmd/anfra/detach_windows.go` (`DETACHED_PROCESS \| CREATE_NEW_PROCESS_GROUP \| CREATE_NO_WINDOW`), used by `update.go:332` and `console_server.go:99`. `cmd/anfra/console_restart_windows.go`: `syscall.Exec` doesn't work on Windows, so start the new binary with the same args and stdio, wait, and exit with its code. | 0.5–0.75 |
| `cmd/anfra/console_server.go:163` stop: stop the background server without a signal: call a shutdown op on loopback, or send CTRL_BREAK to its process group. | 0.25–0.5 |
| `cmd/anfra/serve.go:292`: Windows returns `WSAEADDRINUSE`, not `syscall.EADDRINUSE`. Add a build-tagged `isAddrInUse(err)` so serve moves to a free port. | 0.25 |

**Result: done** (2026-11-25), tested on Windows 11 ([tasks](tasks/archive/win-process-lifecycle-todo.md)). What changed from the rows above:

- **Console flags:** a sidecar gets `CREATE_NO_WINDOW` only when anfra has no console. With it, Ctrl-Break sent directly doesn't reach the sidecar's hidden console; so anfra without a console attaches to the sidecar's console for a moment to send it. With a console, the sidecar shares it, and Ctrl-Break goes directly ([findings](docs/research/windows-sidecar-transport.md#later-stopping-canal-with-ctrl-break-with-and-without-a-console)).
- **Stop:** one signal per sidecar, never both: closing stdin for anfra-node, Ctrl-Break for canal, since Ctrl-Break's default handler would cut anfra-node's shutdown short. After the wait, the sidecar alone is killed (`Process.Kill`); ending the shared Job Object would end every sidecar.
- **The console's server** starts on the console's own console (`CREATE_NEW_PROCESS_GROUP`, not `detachProcess`), so Ctrl-Break can reach it; `detachProcess` stays for the update check.
- **Added at review:** the socket's folder is now private on Windows too (an access list for this user only), as mode `700` makes it on Linux and macOS. Before, a shared `TMP` such as `C:\Temp` gave everyone access.
- **The cache needs anfra-node 0.16.0** on Windows, built for M1 from its tag (Bun 1.2.10 cross-compiles it from Linux); the spike's 0.13.1 gives no cache key.
- **For `win-anfra-node`:** each command logs a `watchdog.parent_death` warning, since closing stdin is how anfra stops anfra-node on Windows. Handling `SIGBREAK` (its row) lets anfra stop it with Ctrl-Break instead, and ends the noise.

### `win-files-auth`


| Work | ED |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `internal/cliauth/credential_store.go:69`: skip the `0600` check on Windows (the user profile's permissions protect the file). Close the open `credentials.json` before renaming over it (`:60-62` vs `:117`). `internal/cliauth/refresh_recovery.go:61`: directory `Sync()` is a no-op on Windows. **Without these, login and publish fail.** | 0.5 |
| Atomic writes by temp file + rename fail on Windows when another process has the file open. Add one helper in `shared/` that retries on "access denied / sharing violation". Use it in `cmd/anfra/runtime.go:58`, `internal/publish/syncstate/syncstate.go:212`, `internal/repoconfig/repoconfig.go:303`, `cmd/anfra/console_history.go:99`, `internal/publish/workdir/apply.go:231`. | 0.5–0.75 |
| Exports: `cmd/anfra/commands.go:590` closes `src` before removing the file. `internal/exports/exports.go:150` and `commands.go:620` build and read `file:` URLs the Windows way (`file:///C:/...`). | 0.5 |
| `internal/home/home.go:37` `RemoveLegacy`: **don't run it on Windows**. There, `os.UserCacheDir()/anfra` is `%LOCALAPPDATA%\anfra`, which must never be deleted. | 0.1 |
| Security and path checks: `internal/appserve/files.go:73` also rejects `\` and `:`. `internal/publish/workdir/apply.go:171` `checkPath` refuses Windows-reserved names (`CON`, `NUL`, `AUX`…), `:` `<>\|?*`, and a trailing dot or space, with a clear error. | 0.5–0.75 |
| `internal/appserve/reader.go:43`: use the time zone from `time.Local` (Windows has no `/etc/localtime`), mapping Windows zone names to IANA names. | 0.25–0.5 |
| `internal/motherduck/cli.go:269`: also match DuckDB's Windows lock message ("being used by another process"). | 0.1 |

**Result: code done** (2026-11-26), tested on Windows 11 ([tasks](tasks/archive/win-files-auth-todo.md)). What changed from the rows above:

- **Logins failed outright on Windows,** before any permission check: syncing anfra's folder is "Access is denied" there. Skipped on Windows (NTFS journals the rename).
- **Private files:** not skipped, as the first row said, but kept private the Windows way (decided with the user): `internal/fileperm` gives an owner-only access list, for the login file (refused when others can read it, with an `icacls` fix) and also `data_sources.yml`, the console history and `serve.json`.
- **The rename helper** is `internal/robustio` (`shared/` is public API); 5 save sites. `workdir/apply.go:231` no longer renamed anything.
- **Data Apps could serve a hidden file** on Windows (`apps/sub\.secret`): now refused, on every OS.
- **Time zone:** from the ICU Windows ships, no table to keep up.
- **DuckDB's Windows lock message** is different ("being used by another process", "File is already open in"), and a read-only open is refused too when the holder has the file read-write.
- **`RemoveLegacy`** had a different reason to skip Windows: anfra never kept files there.
- **Moved elsewhere:** file names that differ only in case, and names Windows can't hold on publish ([backlog](docs/todos/backlog.md)); `anfra logs`, the live-reload watcher and symlink tests failing on Windows (`win-terminal`).
- **Added at the end, decided with the user:** export files and MotherDuck's upload copy, which hold the user's data, are owner-only on Windows too.

### `win-line-endings`


| Work | ED |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| `cmd/anfra/init.go`: write `.gitattributes` with `*.aml`, `*.aql`, `*.yml` and `apps/**` as `text eol=lf` (skip if a file already sets them). | 0.25 |
| Read every working-folder file "as published": one function in `internal/publish/workdir` that turns CRLF into LF in text files (git's rule: no zero byte in the first 8 KB, so binaries are untouched; a lone CR is kept). Use it at all six places publish and pull read the folder: `workdir/tree.go:33` (`Read`, the hashes) and `:81` (`ReadBlob`, the uploaded content and the merge's local side), `workdir/apply.go:102` (pull's "was it edited?" check), `publish.go:345`, `pull.go:208` and `pullhints.go:35`, `settings.go:174`. Uploads become LF; the hash format and the server stay unchanged (`shared/repoformat/repoformat.go` untouched). The three-way merge (`shared/treemerge`, `shared/textmerge`) gets the local side through `ReadBlob`, so it compares LF with LF, and a CRLF file no longer becomes one whole-file conflict. | 0.75–1.25 |
| `internal/repoconfig/repoconfig.go` `appendBlock` and `internal/agents/agentsmd.go`: write new lines with the file's own line ending. | 0.25 |
| Tests: a repo with CRLF files publishes with the LF hashes and uploads only the really edited file; a pull over CRLF files updates untouched files without an "edited" refusal; a pull with a real change on each side of one CRLF file merges cleanly; a binary file with CR bytes is untouched; appending to a CRLF config file keeps CRLF. | 0.25 |

Compiling needs no change: the AML parser already treats a CR as whitespace and rebuilds multi-line values (`'''…'''`, `@sql … ;;`, `@aql … ;;`) with LF (`aml` repo: `parser/lexer.ts`, `modules/expression/string/utils.ts`), so `validate` and `query` give the same results for CRLF and LF files. Only publish and pull, which compare raw bytes, need this module.

**Result: done** (2026-11-27), tested on Windows 11 with `acme` ([tasks](tasks/archive/win-line-endings-todo.md)). What changed from the rows above:

- **Five places, not six:** `workdir/apply.go:102` is `backup`, which must keep exact bytes for `--abort`. And of the five, three need nothing (`pull.go:208`, `pullhints.go:35`, `settings.go:174` already read CRLF right): only `Read`, `ReadBlob` and the upload changed.
- **Pull keeps each file's own line ending** (decided with the user, as editors do): a CRLF file stays CRLF, conflict markers included.
- **`.gitattributes`:** `text=auto eol=lf` for anfra's folders (decided with the user): the row's `apps/** text` would have damaged images and fonts in git.
- **`AGENTS.md`:** the block was found in CRLF files too; the problem was `CheckBlock` comparing exactly, so a CRLF block read as outdated each time.
- **On Windows with `acme`:** a model saved CRLF shows nothing to publish; a change made on LF pulls into a CRLF folder as CRLF; removed there and published, `acme` holds the original bytes again.

### `win-terminal`


| Work | ED |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| At startup on Windows, turn on `ENABLE_VIRTUAL_TERMINAL_PROCESSING` for stdout/stderr (`internal/cliui`), so colors work in old cmd and PowerShell 5 windows. `internal/pretty/pretty.go:741` `utf8Terminal` → true on Windows. `internal/cliui/entry.go:180`: say "Ctrl-Z then Enter" on Windows. | 0.25–0.5 |
| Manually test `anfra console` (bubbletea) and the publish prompt (`muesli/cancelreader`) in Windows Terminal, PowerShell 5 and cmd, and fix what breaks. | 1.25–1.75 |
| Found in `win-files-auth` (tests failing on Windows, causes not yet looked into): `anfra logs` and following the log (`cmd/anfra`: `TestLogsPretty`, `TestLogsNarrowed`, `TestLogsHintsWhyNoAppReports`, `TestLogsFollow`, `TestReadLinesEndWithTheirContext`); the Data Apps live-reload watcher gets folder events on Windows, so it reports apps that didn't change (`internal/appserve`: `TestWatch`, `TestLiveReloadWatchesOnlyWhileAStreamIsOpen`). Tests that need symlinks (`TestFiles`, `TestCheckPath`, `TestInitRefusesASymlinkedGitignore`) need administrator rights or developer mode on Windows: run them so in the Windows CI job, or skip them there. | not estimated |

**Result: done** (2026-11-27), tested on Windows 11 ([tasks](tasks/archive/win-terminal-todo.md)). What changed from the rows above:

- **Colors:** anfra turns on escape-code processing at startup on Windows (`colorprofile` only detects it). Windows Terminal shows colors; the old console host (`cmd`, PowerShell 5 windows outside Windows Terminal) wasn't checked by hand (the user decided).
- **Unicode trees:** `utf8Terminal` is true on Windows. No "Ctrl-Z then Enter": the console reads keys through Bubble Tea, and Ctrl-D leaves it on Windows too.
- **Typed answers to questions** (found by hand): `cancelreader` puts a Windows console in raw mode, so answers weren't shown and Enter didn't end them. The console's own mode is put back on Windows; one way of asking on every OS (option A, chosen over Bubble Tea on Windows only).
- **The `anfra logs` tests** failed on Windows because of the update-check cascade from `cmd/anfra`'s test program (`d3d2150`, which also crashed the Windows machine once). The live-reload watcher now ignores folder events; tests needing symlinks skip on Windows, saying why.
- **Found on the way, every OS:** the home folder was taken for a repo, as anfra's own folder is `~/.anfra` (`8667b4c`).
- **Not done here:** Ctrl-C during a query forwarded to `anfra serve` prints "unavailable … interrupt signal received" rather than "Interrupted." (the same on Linux; the user decided against a backlog item).

### `win-self-update`


| Work | ED |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `internal/update/update.go:79` `assetName`: add `windows/amd64` → the Windows asset (`anfra-windows-x64.zip`, [decided](#decisions-made)). | 0.1–0.5 |
| `minio/selfupdate` already renames the running exe to `.anfra.exe.old`. Before updating, try to delete an old `.old`; if it's in use (a running `serve`), say so clearly instead of failing. `update.go:249`: point the reinstall advice to `install.ps1`. | 0.5–0.75 |

Also check, on Windows, `anfra update` from inside `anfra console`: the console restarts on the new anfra (`restartConsole`, from `win-process-lifecycle`, tested there by `TestRestartWith` only, since it needs a released Windows build).

- **Result:** done as above, plus `SHA256SUMS`: `build_release.yml` publishes it (uploaded before the assets), and `anfra update` refuses a download that doesn't match it, on every OS. A leftover `.anfra.exe.old` still in use doesn't fail the update: it moves the running file aside under a name of its own, and the next update removes them all.
- **Found on the way:** inside `anfra console` on Windows, `update` didn't restart the console: Go's `os.SameFile` reads a file's ID on first use, by its path, so after the update both sides were the new file (`44c339c`).




### `win-release`



#### Release, install, CI


| Work | ED |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| `.github/workflows/build_release.yml`: add `windows-x64` (`GOOS=windows GOARCH=amd64`), download `anfra-node-windows-x64.exe` and `anfra-canal-windows-x64.exe`, output the Windows asset as `anfra-windows-x64.zip`, `anfra.exe` at its top ([decided](#decisions-made); `SHA256SUMS` already covers every `anfra-*` asset). `manifest.yml`: bump the sidecar tags to versions with Windows builds. | 0.5 |
| **Later, not in the first release (decided 2026-11-27):** code signing. Set up Azure Trusted Signing or buy an OV certificate, then add a signing step (signtool / `azure/trusted-signing-action`) to this repo's release; the other two repos reuse it. Identity checks take days to weeks. | (1–1.5, later) |
| New `install.ps1` in the repo root, next to `install.sh`: detect the arch, download into `%USERPROFILE%\.anfra\bin\anfra.exe`, **verify its SHA-256** against the release's checksums and refuse a mismatch (what stands in for signing at first), add the folder to the user PATH, say to open a new terminal, refuse arm64 for now with a clear message. The same overrides as `install.sh` (`ANFRA_HOME`, `ANFRA_INSTALL_DIR`, `ANFRA_VERSION`, `ANFRA_NO_MODIFY_PATH`), plus `ANFRA_DOWNLOAD_URL`, where to download the release from, for testing. No `exit` in the script: under `irm … \| iex` it runs in the user's session and would close their window; `return` or `throw` instead. Plain ASCII, no byte-order mark. `install.sh`: on Git Bash/MSYS/Cygwin, point to `install.ps1`. **Merge order:** the script goes live the moment it reaches `main` (users fetch it from `main`, not from a release), so merge `install.ps1` only **after** the first release that has the Windows asset. Otherwise it downloads a file that doesn't exist yet. | 1–1.25 |
| **Test `install.ps1` before it goes live** (decided 2026-11-27), each layer on its own: (1) from a local file, `Get-Content install.ps1 -Raw \| iex`; (2) the real pipe from a local web server, `python3 -m http.server` in WSL and `irm http://localhost:8000/install.ps1 \| iex` on Windows, which catches an `exit` closing the window, encoding, and PowerShell 5.1 vs 7 (syntax, TLS); (3) downloads from a local build through `ANFRA_DOWNLOAD_URL`, with `ANFRA_HOME` in a temporary folder and `ANFRA_NO_MODIFY_PATH=1`, including a changed `SHA256SUMS` that must be refused; (4) before the Cloudflare redirect, a pushed branch's raw URL (the repo is public), `irm https://raw.githubusercontent.com/holistics/anfra/<branch>/install.ps1 \| iex`; (5) a clean machine: Windows Sandbox (Windows 10/11 Pro), with PowerShell 5.1 and 7, for the first run, PATH and antivirus; (6) `Invoke-ScriptAnalyzer install.ps1` (PSScriptAnalyzer), also in CI. | 0.5 |
| **PowerShell's execution policy** (found 2026-11-27, in `win-terminal`): Windows' default policy refuses to run a `.ps1` file, so a downloaded `install.ps1` run as `.\install.ps1` fails. `irm … \| iex` runs the script's text, not a file, so the policy doesn't apply. Every doc, hint and error message says `irm https://anfra.ai/install.ps1 \| iex`, never `.\install.ps1`. | — |
| `.github/workflows/continuous_integration.yml`: a `windows-latest` job running `go build ./...`, and `go test` on the packages with Windows logic (see the tests below), plus `GOOS=windows go vet ./...` in the Linux lint job, so Windows compile breaks show early. | 0.75–1 |
| `internal/sidecar/{anfranode,anfracanal}/embed_release.go`: keep the same embed path; the build copies the Windows sidecar into it. | 0.25 |
| Windows also embeds DuckDB's official `duckdb.dll` (from DuckDB's release `libduckdb-windows-amd64.zip`, the version the Go bindings pin, checksum checked), built with canal by `build-windows-dll.ps1`'s flags (`duckdb_use_lib`). A release check that the DLL's DuckDB version equals the bindings' (`select version()` through canal, or the file's version). | 0.5 |




#### Go tests on Windows, for the code with Windows logic


| Work | ED |
| --------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| Run `go test` on Windows for the packages with Windows logic, and fix or skip their tests that fail only because of test setup (listed below) | 0.5–1 |


Nobody develops anfra on Windows, so the goal is not "every test passes on Windows". The goal is **CI catching Windows bugs automatically**, since the team works on Mac/Linux and would not notice a Windows break. So:

- `GOOS=windows go vet ./...` in the Linux CI job catches code that doesn't compile on Windows.
- The `windows-latest` CI job runs `go test` only for the packages with Windows logic: `internal/sidecar`, `internal/cliauth`, `cmd/anfra`, `internal/publish/workdir`, `internal/exports`, `internal/home`, `internal/update` (the `.old` file still in use, from `win-self-update`), and the new `shared/` file-replace helper.
- Only the tests in those packages are fixed. Most of the ~16 failing tests fail because of test setup, not user bugs.

The fixes, where they fall in those packages:

- Permission-mode tests (`internal/sidecar/socket_test.go:31`, `cmd/anfra/init_test.go:68`, `console_test.go:245`, `internal/cliauth/*_test.go`, `syncstate_test.go:132`): skip the mode check on Windows.
- Tests that set `HOME` (`internal/home/home_test.go`, `internal/repo/repo_test.go`, `cmd/anfra/serve_test.go`, `publish_test.go`, `internal/app/*_test.go`): set `ANFRA_HOME` instead (or also `USERPROFILE`). `home_test.go` must never touch the real `%LOCALAPPDATA%`. (`home_test.go` done in `win-files-auth`; `repo_test.go`'s `TestCatalogPathUsesPerRepoDataDir` still fails on Windows.)
- Found 2026-11-27 (`win-terminal`), test setup only: `internal/publish`'s MotherDuck tests make a symlink (`link.duckdb`), which Windows allows only to administrators or developer mode: skip just what needs it, as `internal/appserve`'s `symlink` helper does. `TestPublishWarnsOfOverriddenChangesAndLocalSettings` reads `Etc/UTC` as no time zone on Windows: the `publish` test program lacks Go's zone database, which anfra itself carries (`internal/command/query/input.go` imports `time/tzdata`); import it in that package's tests.
- Fake `#!/bin/sh` binaries (`canallake_test.go:22`) and `/bin/echo` (`agents_test.go:316`): use a small Go helper, or skip on Windows.
- Symlink tests: skip when `os.Symlink` returns a privilege error.
- Hard-coded `/tmp` and `/home/a` paths: use `t.TempDir()` and `filepath.Join`.



### `win-skills-docs`

**Skills** (what coding agents read):


| Work | ED |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- |
| `skills/plugins/anfra/hooks/hooks.json` runs `sh .../check-update.sh` and `validate-aml.sh`. Claude Code on Windows has Git Bash, but the hook's `sed` JSON parse breaks on `C:\\` paths. Fix the parse, and add `.ps1` versions for agents without bash. The execution policy blocks a `.ps1` hook run as a file too (see `win-release`): run it as `powershell -NoProfile -ExecutionPolicy Bypass -File …`, which changes the policy for that one run only. A company's Group Policy can still block it, so check that a hook blocked this way fails visibly, not silently. | 0.75–1 |
| Skill text: `anfra-setup-repo/SKILL.md:22` gets the PowerShell install line, picked by OS. `anfra-build-data-app/SKILL.md:226` gets a PowerShell way to compute the SRI hash (`Get-FileHash` + base64) instead of openssl. `anfra-run-aql` / `anfra-validate-aql` show PowerShell quoting. | 0.5–0.75 |


**Install docs** (what people read). Every place that says how to install anfra today, found by searching for `install.sh` and "macOS and Linux":


| Where | Change | ED |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `README.md:62`, the "one prompt" quickstart | Its step 1 tells the agent: on Windows, run `irm https://anfra.ai/install.ps1 \| iex`, and the binary is `%USERPROFILE%\.anfra\bin\anfra.exe`; on macOS/Linux, `curl … \| bash` as today. | 0.1 |
| `README.md:75-80`, "Install anfra" | Add a **Windows (PowerShell)** block next to the `curl` one. Replace "Works on macOS and Linux" with "Works on macOS, Linux and Windows (x64)". | 0.1 |
| **anfra.ai/docs, [Installation](https://anfra.ai/docs/local-dev/installation)** (linked from `README.md:80` and `:294`; its source isn't in this repo, so find which repo builds anfra.ai first) | A **Windows** section with: • requirements (Windows 10 1803+ or 11, x64) • the PowerShell line • manual install from the release page (unzip, put `anfra.exe` in a folder, add it to PATH) • where anfra keeps its files (`%USERPROFILE%\.anfra`) • what a SmartScreen warning means • how to uninstall (delete the folder, remove it from PATH) • that WSL still works, using the Linux install | 0.5–0.75 |
| **anfra.ai: a Cloudflare redirect for** `/install.ps1` | anfra.ai doesn't host the scripts. `https://anfra.ai/install.sh` is a Cloudflare 301 redirect to `https://raw.githubusercontent.com/holistics/anfra/main/install.sh`. Add the same rule: `/install.ps1` → `…/main/install.ps1`. Whoever manages anfra.ai's Cloudflare adds it. PowerShell's `irm` follows the redirect, and GitHub serves the file as plain text. **The PowerShell install line in the docs only works once this rule exists.** | 0.25 |
| Release notes | The release that first ships Windows says so at the top of its `CHANGELOG.md` section (written by `pnpm bump` from a `feat:` commit), with the install line. | 0.1 |


**Design and dev docs** (what contributors read):


| Where | Change | ED |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | -------- |
| `docs/designs/architecture.md:94`, `internal/home/home.go:5` (comment) | `bin/anfra` → `bin/anfra` (`bin\anfra.exe` on Windows), "the installer's binary (`install.sh`, `install.ps1`)" | 0.1 |
| `docs/designs/sidecars.md` | A Windows section: Job Object, graceful stop, Unix sockets on Windows (from `win-sidecar-transport`'s findings doc) | 0.1–0.25 |
| `docs/designs/release.md` | The Windows target, the `.zip` asset, `install.ps1` and its checksum check, unsigned at first | 0.1 |
| `DEVELOPMENT.md` | One line: on Windows, develop in WSL; the Windows CI job tests the Windows code | 0.1 |
| `docs/README.md` | List the `win-sidecar-transport` findings doc under Research (done in its T1) | — |


Not changed: `internal/update/update.go:249`'s reinstall message is in `win-self-update`. `install.sh`'s own "next steps" text stays Linux/macOS; on Git Bash it points to `install.ps1` (`win-release`). `docs/usage/docker.md` already covers Docker on Windows.

## Long-term maintenance



### By module

What brings work back after each module ships, what happens if that work is missed, and how to prevent it. Shared drift, guards and policies follow below.


| Module id | Will it need work later? | What brings work back later | If it's missed | Mitigation | Risk: can't release a new version |
| ----------------------- | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| `win-sidecar-transport` | One-off | A Bun upgrade changes how it listens on Windows | anfra can't reach anfra-node on Windows after a Bun bump | Keep check 1 (`dialnode`) as a Windows smoke test in holistics-core's CI. Pin Bun. Loopback TCP + token stays the fallback. | Medium |
| `win-process-lifecycle` | One-off | Any new place that starts a process (a new sidecar, a background job) | Orphaned sidecars holding DuckDB locks, Ctrl-C killing sidecars, console windows popping up | Start every process through one helper (`sidecar.StartProcess`). A lint rule (`forbidigo`) refusing `exec.Command` outside it, and `syscall.Kill` and `Setpgid` everywhere. A Windows CI test: kill the parent, check the children die. | Medium |
| `win-canal` | Keeps costing effort, often: every canal release and every database engine upgrade can need Windows work. | Every canal release; every DuckDB upgrade; the holistics fork of duckdb-go; any new C-based database driver | **anfra's release breaks or stalls**: no Windows canal means no Windows anfra. Or anfra ships an old canal. | The Windows build in canal's CI on every PR, so a break shows in the PR that caused it. A Windows smoke test (`--version`, `SELECT 1` through DuckDB). `build_release.yml` fails loudly when a Windows asset is missing. A named owner on the canal team. | High |
| `win-anfra-node` | Keeps costing effort, sometimes: every Bun upgrade and some new packages can need Windows work. | Bun upgrades; new npm packages with native code but no Windows build; new code that assumes `/` in paths | anfra-node's Windows build fails, or path bugs appear only on Windows | The Windows smoke test in holistics-core's CI. Tests with Windows-style paths in the path-handling code. | Medium |
| `win-files-auth` | One-off | New code that saves files by rename, checks Unix permissions, builds `file:` links, or splits paths on `/` | Random "Access is denied" errors, failed logins, broken export links, on Windows only | Use the shared helpers the fixes add (safe file replace, `file:` links), not copies. The Windows CI job tests those packages. A review checklist item. | Low |
| `win-line-endings` | One-off | New code that hashes, compares or writes repo files; new file types in a repo. | False "changed" files and conflicts for Windows users in mixed teams | One function for reading repo text, normalizing line endings. CRLF test files in the publish tests. `.gitattributes` covers new file types. | Low |
| `win-terminal` | One-off | New terminal UI | Garbled colors in old cmd/PowerShell windows. Low impact: Windows 11 uses Windows Terminal by default. | Route all output through `internal/cliui`. A quick manual look at each minor release. | Low |
| `win-release` | Keeps costing effort, on every release, and whenever the signing certificate or the build machines change. | Every release; GitHub changing the Windows runner image; once signing is added, **the signing certificate expiring** | **Users can't install anfra:** the release fails, or its checksums don't match | `install.ps1` refuses a checksum mismatch. Pin the runner image (`windows-2022`). The minimum-OS check. Once signing is added: a CI step that checks the signature (`signtool verify`), and Azure Trusted Signing (renews itself) or a calendar reminder and owner for a bought certificate. | High |
| `win-self-update` | One-off | The release file name lives in **three places**: `build_release.yml`, `assetName`, `install.ps1` | `anfra update` says "no release build", or `install.ps1` downloads nothing | One source of truth for target names (`manifest.yml`), and a test that the three agree. A Windows CI test that updates from the previous release. | Low–Medium |
| `win-skills-docs` | Keeps costing a little effort: every new skill, hook or doc page needs a Windows version. | Every new skill, doc page or hook with shell commands | Agents on Windows fail, or skip hooks silently; docs drift back to Mac/Linux only | Move the hook logic into the `anfra` binary (e.g. `anfra hook check-update`), so there is one version, not `.sh` and `.ps1` copies. A "Windows?" item in the PR template. | Low |




### The drift we've already seen

The same drift has already happened on **macOS**. canal builds on GitHub's `macos-latest` and `macos-15-intel` runners without setting a minimum macOS (`MACOSX_DEPLOYMENT_TARGET`), so each build only accepts the macOS of the runner it was built on. Read from the release binaries' headers (`LC_BUILD_VERSION`):


| Release binary | Oldest macOS it accepts |
| ------------------------------- | ----------------------- |
| anfra v0.23.1 (arm64, x64) | 13 Ventura |
| anfra-node v0.16.0 (arm64, x64) | 13 Ventura |
| **anfra-canal v1.1.1, arm64** | **26 Tahoe** |
| **anfra-canal v1.1.1, x64** | **15 Sequoia** |


So on an Apple Silicon Mac with macOS 15 or older, anfra starts, but anything that runs SQL fails when canal-query starts. On Linux, canal needs **glibc 2.38** (it's built on Ubuntu 24.04), so Ubuntu 22.04, Debian 12, RHEL 9 and Amazon Linux 2023 can't run it either. Nobody chose these limits; the runners moved. Windows runners will move the same way (new Windows Server images, new MinGW versions).

**Fix for the existing drift (canal's release workflow):** set `MACOSX_DEPLOYMENT_TARGET=13.0`, and build Linux on an older base (an Ubuntu 22.04 runner or a manylinux container). This is canal work outside the Windows effort above.

### Guards for all modules


| Guard | What it catches | Effort |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| **The Windows CI job** and `GOOS=windows go vet` (`win-release`) | Compile breaks and regressions in the Windows code, in the PR that caused them | In `win-release` |
| **Windows smoke tests in canal's and holistics-core's CI** | A sidecar that no longer builds or runs on Windows, before anfra's release | **0.5–1 ED**, in the [Estimate](#estimate) |
| **Minimum-OS check in anfra's release CI**: read each release binary's oldest supported OS (macOS `LC_BUILD_VERSION`, Linux's highest `GLIBC_` symbol version, Windows' PE subsystem version) and fail when it goes up | Drift like canal's macOS 26 requirement, on every OS | **0.25–0.5 ED**, in the [Estimate](#estimate) |
| **A release smoke test on real Windows 11** (install, query, serve) | What Windows Server CI misses: SmartScreen, Windows Terminal, Defender | ~0.5 ED **per release**, manual; or scripted on a Windows 11 VM. Ongoing, not in the one-time total. |
| **A Windows owner** | Someone responsible when the Windows build breaks | A decision |
| **"Windows?" in the PR template** | New code that forgets Windows | Minutes |
| **The OS in logs and error reports** | Seeing that failures cluster on Windows | Small, if the logs don't already record it |




## windows-arm64 (for a later decision)

Not in the module specs or `tasks/` yet: they cover windows-x64 only, until arm64 is decided. If native arm64 is chosen, `win-sidecar-transport` also needs its checks re-run on an arm64 machine with a newer Bun (included in the anfra-node row below).


| Part | Work | ED | Risk |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ | ----------------------------- |
| anfra (Go) | Add `windows-arm64` to the build, `assetName`, and `install.ps1`. Pure Go, so it cross-builds. | 0.25–0.5 | Low |
| anfra-node (Bun) | Bun needs a version with `bun-windows-arm64` (newer than 1.2.10), with a retest. `@holistics/aml` has no `win32-arm64` native build, so it falls back to WASM (slower compiles) unless that package adds one (+1–2 ED in the aml repo). | 1.5–2.5 | Medium |
| anfra-canal | DuckDB ships official Windows ARM64 builds (static and shared libraries, from its releases). `duckdb-go-bindings` bundles no `windows-arm64` library, but since **v0.20001.0** (DuckDB 2.0.1, released 2026-11-21) it can link DuckDB's official ones: static (llvm-mingw plus Microsoft's ARM64 build tools) or dynamic (`duckdb.dll` shipped beside the program; anfra then unpacks it beside `anfra-canal.exe`, in `internal/sidecar/binary.go`). **Needs canal on DuckDB 2.0 first:** canal pins `duckdb-go-bindings` v0.10506.0 and holistics' fork `duckdb-go` v2.10506.0-streaming.1, so the bindings and the fork's streaming patch move to the 2.0 line. That upgrade is canal-wide and not estimated here. **Unverified:** the Oracle driver (godror) on Windows ARM64, which needs Oracle's client libraries for that platform. | 1.5–3, after the DuckDB 2.0 upgrade | Medium: the Go support is new |
| CI | An arm64 Windows runner (GitHub `windows-11-arm`) for canal's native build and the tests. | 0.75–1.5 | Medium |
| **Total native arm64** |  | **≈ 4–7.5 ED**, after canal's DuckDB 2.0 upgrade |  |
| **Cheapest option** | Ship **x64 only**. Windows 11 on ARM runs x64 programs through emulation, so arm64 users can still use the x64 build (slower). `install.ps1` installs x64 on arm64 with a note instead of refusing. | 0.25 | Low |




## Verification

On a clean Windows 11 x64 machine (and a Windows Server CI runner):

1. In PowerShell: `irm https://anfra.ai/install.ps1 | iex`, then open a new terminal. Check that `anfra version` runs with no SmartScreen block.
2. `anfra init --example`, then `anfra validate`, `anfra show`, `anfra query` (with and without the cache). Check that the sidecars start and stop, and that no `anfra-node` or `anfra-canal` stays in Task Manager afterwards.
3. Kill anfra with `taskkill /F` during a long query. Check that the sidecars are gone too (Job Object).
4. `anfra serve`, then open a Data App in the browser and check live reload. Start a second `anfra serve` while the port is taken; it should move to a free one. Press Ctrl-C and check everything stops cleanly.
5. `anfra console`: run AQL, press Ctrl-C during a query (the sidecars should survive), then leave and check the background server stops.
6. `anfra auth login`, `anfra publish`, `anfra pull` in a repo cloned with `core.autocrlf=true`. Check there are no false changes or conflicts.
7. `anfra update` while a `serve` is running, then again after it stops.
8. Run the same steps in cmd.exe and in Windows PowerShell 5 (colors, prompts).
9. In Claude Code and Cursor on Windows, paste the README quickstart prompt; it should finish end to end.
10. CI: the `windows-latest` job runs `go test` green on the packages with Windows logic, and `make check` / `make test` stay green on Linux.
11. Docs: a person who has never used anfra installs it on Windows using only the anfra.ai Installation page, once with `install.ps1` and once by hand from the release `.zip`. `https://anfra.ai/install.ps1` opens as plain text in a browser. A search of the repo for "macOS and Linux" finds no install text left that leaves Windows out.



## Sources

- [Bun source: Listener.rs (main)](https://github.com/oven-sh/bun/blob/main/src/runtime/socket/Listener.rs)
- [Bun source: bsd.cpp (main)](https://github.com/oven-sh/bun/blob/main/packages/bun-usockets/src/bsd.cpp)
- [Bun source: socket.zig at bun-v1.2.10](https://github.com/oven-sh/bun/blob/bun-v1.2.10/src/bun.js/api/bun/socket.zig)
- [Bun source: node:net (net.ts)](https://github.com/oven-sh/bun/blob/main/src/js/node/net.ts)
- [Bun issue #8851: Unix sockets on Windows](https://github.com/oven-sh/bun/issues/8851)
- [Node.js net docs: Windows uses named pipes](https://nodejs.org/api/net.md)
- [Claude Code issue: Bun named-pipe crash on Windows](https://claudeissues.com/issue/22416-chrome-native-host-crashes-on-windows-bun-1-3-5-panic-on-named-pipe-creation)
- [DuckDB releases: official](https://github.com/duckdb/duckdb/releases) `windows-arm64` [builds](https://github.com/duckdb/duckdb/releases)
- [duckdb-go-bindings: Windows ARM64 linking (PR #124)](https://github.com/duckdb/duckdb-go-bindings/pull/124)

