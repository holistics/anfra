# Copying a DuckDB data source to MotherDuck

> Status: built (`internal/motherduck`, `internal/publish/motherduck.go`). The checks under [Verified](#verified) were run on 2026-10-21 against MotherDuck CLI v1.5.6, anfra-node 0.9.1 and canal-query 2.19.1.

A repo's DuckDB data source is a file on the machine that publishes it, which the org's server never reads: `anfra publish` says so and creates nothing (`localOnly`, `internal/publish/datasources.go`). Its models are published anyway, and their queries fail on the org until someone loads the data into a warehouse the server reaches.

For DuckDB, that warehouse can be MotherDuck: it takes a DuckDB file as it is, its SQL is DuckDB's, and the pieces to use it are already there. This design adds an offer, in `anfra publish`, to copy the file to MotherDuck once and give the org a data source on the copy.

## What is already there

- **anfra-cloud takes `motherduck` data sources** and refuses `duckdb` (`anfra-cloud/internal/bi/datasources/catalog.go`): connection `token` (required), `dbname`, `default_dbname`.
- **canal-query has a MotherDuck connector** (`canal/internal/query/connector/motherduck_connector.go`). It opens an in-memory DuckDB, attaches `md:<dbname>` with the token, and sets `motherduck_saas_mode`, which stops a query from reading the server's own disk.
- **anfra-node compiles `motherduck` as DuckDB.** The dialect registry maps it to DuckDB's dialect (`holistics-core/packages/sql-generation/src/sqlGenerators/dialectRegistry.ts`), and anfra-node passes a data source's `dbtype` to the compiler as it is.

So the org's data source is `{type: motherduck, connection: {token, dbname}}`, under the local one's name. The models do not change, and neither does the repo's `.anfra/data_sources.yml`: here, the data source is still the local file.

## Safety

The copy writes to a person's own MotherDuck account, so it never changes or removes anything there, and never writes the local file. Each rule is enforced in code, not only followed:

- **Every statement anfra sends is in one file** (`internal/motherduck/cli.go`), and none replaces, drops, alters or deletes; a test checks each against a list of such words. A database name anfra makes is lowercase letters, digits and underscores (`DatabaseName`), checked again before any statement uses it, so nothing in a name needs escaping; a path is a quoted literal.
- **A copy is a new database, under a free name.** Taken names are kept clear of: `northwind`, then `northwind_2`, `northwind_3`… The upload is `CREATE DATABASE`, never `OR REPLACE`, so a name taken between the check and the upload fails it, and the existing database is untouched. There is no "reuse" or "replace" choice.
- **The local file is only read.** It is attached `READ_ONLY`, and uploaded from that attachment: DuckDB opens it for no writing, which replaying its write-ahead log would otherwise be.
- **Nothing is cleaned up on MotherDuck's side, except anfra's own token.** An upload that fails or is interrupted may leave a database: it is said, by name, for the person to drop; anfra drops nothing. The one thing anfra removes is the token it made in the same publish, when no data source on the org came to use it (`mdCopy.finish`), so it is not left behind reading everything. A failure that says the name was taken is not called partial: that database is not this upload's.
- **The org gets only a read-only token**, one anfra made for it or one the person pasted, never the person's own (below). One MotherDuck answers otherwise than asked, as a token that may write, is revoked at once and not used; a pasted one that says it may write is refused.
- **Each step is agreed first,** at a terminal: the upload, with the account it goes to (default no), signing in (default no). Without a terminal there is no offer. The org's data source is created through `createDataSource`, which tests it first and asks before creating one the server cannot reach.
- **Secrets stay out of output.** The person's token is read from the CLI's output and kept in memory for the REST calls; no error carries the CLI's output for that call. API errors carry MotherDuck's message, never the request.

## Authentication

MotherDuck has no OAuth that gives a third party a token DuckDB accepts. Its remote MCP server's OAuth issues tokens for that server only. A token comes from MotherDuck's web UI, or from signing in with its own clients.

anfra uses MotherDuck's CLI, `motherduck`, for both the sign-in and the upload:

| Need | How |
|---|---|
| Find the CLI | `motherduck` on `PATH`, else `${MOTHERDUCK_HOME:-~/.motherduck}/bin/motherduck`: the installer puts it there and adds that folder to the shell's profile, so a shell started before the install does not see it |
| Is the user signed in, and as whom | `motherduck status -o json`: `authenticated`, `user` (the username the REST API takes). Signed out, it still exits 0, with `authenticated: false`. |
| Sign in or sign up | `motherduck login`, run on anfra's terminal: MotherDuck's own browser and device-code flow. MotherDuck's sign-in page also signs up. |
| Upload the file | `motherduck query "CREATE DATABASE <name> FROM '<absolute path>'"`. The CLI embeds DuckDB and reads local files. |
| A token for anfra's REST calls | `motherduck query "PRAGMA PRINT_MD_TOKEN"`, whose output is one line, `export motherduck_token=<token>`. Documented, unlike `credentials.json`. |
| The org's token | MotherDuck's REST API, `POST https://api.motherduck.com/v1/users/{username}/tokens` |

anfra does not install the CLI: it would be running MotherDuck's install script for the user. anfra also saves no MotherDuck credential. The user stays signed in to the MotherDuck CLI afterwards, as that tool does.

A `MOTHERDUCK_TOKEN` variable is not a way in. It adds a second path that does the same thing less clearly.

### The org's token

The signed-in user's token is a 24-hour session token that reads and writes all of their MotherDuck databases. The org must not get it. It would expire the next day, and it grants far more than the org needs.

anfra creates a token for the org instead, with the user's token, once the user agrees:

- `token_type: read_scaling`: it reads, and MotherDuck refuses its writes.
- No `ttl`: it lasts until revoked. An expiring one would break the org's queries later, with nothing to say why.
- Named `anfra <org>`, such as `anfra acme`: short, for MotherDuck's token list. The token's `description` says the rest: `Read-only access for the anfra org https://bi.example.com/o/acme, made by anfra publish. Revoke it to cut the org off.` One token per publish, whatever number of databases it copies.
- **Shown once, never kept.** MotherDuck answers a token's secret only when it is made, and the org's data sources do not give it back, so anfra prints it, marked as sensitive, for the person to keep (a password manager) and give again in a later publish. anfra stores no MotherDuck secret.

MotherDuck refuses a name one of the user's tokens has (409), as `anfra acme` is after a first publish. anfra then replaces and revokes nothing, and asks which token the org uses:

```
! A token named "anfra acme" already exists in your MotherDuck account. anfra doesn't store token secrets, so it
  can't reuse it.
? Which token should data source ecommerce_duckdb use? [p]aste a read-only token / [c]reate new token named
  "anfra acme 2" / [n]ame a new token / [s]kip uploading this data source (default: skip uploading this data source)
```

- **paste:** a token they kept. It must say it is read-only: a MotherDuck token's claims carry its type, and one other than `read_scaling` is refused (`motherduck.ReadOnly`). The claims are not verified, MotherDuck enforces them; the org's connection test then checks it reads the copy. A pasted token is never revoked.
- **create new token named `anfra acme 2`:** the next number, asked again if that is taken too.
- **name a new token:** one they type.
- **skip uploading this data source:** nothing is uploaded for it; the next file, if any, is asked about again.

What is typed shows as it is typed, as the printed secret does: hiding it would need the terminal to itself, while one reader takes every answer.

Creating a token for one's own account needs no admin role. Creating it is part of what the person agreed to with the copy, so it is not asked again. It is settled before the upload: when it fails, or is skipped, nothing is uploaded, and a token made for an upload that then fails is revoked (`mdCopy.finish`).

## The flow

In `offerDataSourcesToOrg`, a data source the org lacks whose type is `duckdb` gets the offer, where today it gets the warning. Not offered: an in-memory one (no `path`, `:memory:`, or only `?options`) or a path already `md:`.

```
Data sources
! Data sources northwind_copy, northwind_duckdb (duckdb) use data/northwind.duckdb, a local file that the org's
  server can't read.
? Upload data/northwind.duckdb (412.0 MiB) to your MotherDuck account scottbui (holistics), and give the anfra
  org read-only access to it?
    [y]es
    [a]ll: yes to this and all remaining duckdb data sources
    [q]uit: switch accounts with motherduck logout and motherduck login first, then publish again
    [n]o, skip uploading these data sources
  (default: no) y
  A database named northwind already exists in your MotherDuck account, so this one will be named northwind_2.
• Uploading data/northwind.duckdb to MotherDuck as database northwind_2… (a large file takes a while)
✓ Uploaded data/northwind.duckdb to MotherDuck as database northwind_2.
✓ Created read-only MotherDuck token "anfra acme" for the org.
✓ Created data source northwind_copy (motherduck) on the org.
✓ Created data source northwind_duckdb (motherduck) on the org.
  The org will query this snapshot. Later changes to the local file are not uploaded.
```

1. **Group by file.** Resolve each data source's `path` relative to the repo, without its `?options`, and compare files with `os.SameFile`, which also catches symlinks and hard links. One question and one upload per file, naming every data source that reads it.
2. **Ask.** Give the file's size. Without a terminal, there is no offer: the warning stays, adding that a publish from a terminal can copy it.
3. **Sign in.** The account is part of step 2's question, not a question of its own, since one per data source would be too many. Before asking, anfra looks at the CLI once (`motherduck status -o json`, which sends nothing of the repo's) and names the account the upload goes to: the CLI may have been signed in long ago, or as someone else. Its choices are yes; all, when other DuckDB files are still to come, which uploads them too without asking again; no (skip this data source; the default, so Enter never ends the publish); and quit: the publish ends, for the person to switch accounts with `motherduck logout` and `motherduck login`; anfra never signs the CLI out itself, which would sign them out wherever they use it. With no CLI, or none signed in, the question names no account and has no quit. Then anfra finds the CLI, helping install it, and signs in with `motherduck login`, the account chosen there not asked about again. One signed in to that the question did not name, as when the CLI was installed since, is asked about with the same question, naming it, though all was chosen: all covers only an account a question named, or the one chosen in signing in. No there turns all off. Declining to install or sign in skips this data source; a later one is asked again. A sign-in that fails is not tried again in this publish.
4. **Name the database.** The file's name without its extension (a symlink's target's), made a valid identifier; if the account has a database of that name, in any case (`md_information_schema.databases`, column `name`), the first free of `_2` to `_99`. Said, never asked: nothing existing is offered for reuse or replacement.
5. **Upload.** `ATTACH '<absolute path>' AS <alias> (READ_ONLY); CREATE DATABASE <name> FROM <alias>`. One statement, with no progress to show; the CLI's query timeout is raised to a day, since its default of 120 seconds would cut a large upload short, and interrupting the publish stops it. When another process holds the file, see [A file in use](#a-file-in-use). When it fails, say MotherDuck's error as it is, say a database it may have left behind, and go on with the publish without this data source.
6. **The org's token** is settled before step 5, as above, once per publish. There is no falling back to the person's own token: it can write, and it expires in a day.
7. **Create the data sources** through `createDataSource`, which tests each first: `{type: motherduck, connection: {token, dbname}}`, with no settings. The local ones are DuckDB's, and whether the motherduck type takes the same names is not checked yet.

Steps 3 to 7 run once per file. Signing in and the org's token happen once per publish.

### Without the CLI, or signed out

The CLI missing, or not seen from this shell:

```
? Upload data/northwind.duckdb (412.0 MiB) to MotherDuck, and give the anfra org read-only access to it?
  [y]es / [n]o (default: no) y
! Uploading to MotherDuck needs the MotherDuck CLI, which is not installed.
  MotherDuck has a free plan, and signing up needs no credit card. Install the CLI in another terminal, then
  continue here:
      curl -s https://install.motherduck.com | SKIP_DUCKDB_CLI=1 sh
? Continue once it is installed? [y]es / [n]o (default: no) y
✓ Found the MotherDuck CLI v1.5.6-2026-10-484.
```

`SKIP_DUCKDB_CLI=1`, as MotherDuck's install page has it. The free plan (Lite: ongoing, no credit card, 10 GB of storage as of 2026-10) is said where installing or signing up comes up, without its limits, which change: the script otherwise installs DuckDB's own CLI too, which anfra does not use. The publish waits rather than ending, so the user does not start over. Yes looks for the CLI again, in `~/.motherduck/bin` too, since the shell's `PATH` does not have it until a new shell starts. Still not found: the same question again. On Windows the command is MotherDuck's PowerShell one; on musl Linux there is no CLI, which is said, and the copy skipped.

Signed out:

```
✓ Found the MotherDuck CLI v1.5.6.
? Sign in to MotherDuck? It opens your browser, where you can also sign up for free, with no credit card. [y]es
  / [n]o (default: no) y
• Running motherduck login…
  …the CLI's own output: its code and its URL…
✓ Signed in to MotherDuck as scottbui (holistics).
```

anfra runs `motherduck login` with the terminal handed to it: the CLI opens the browser, shows its code, and waits for it. anfra checks `motherduck status -o json` afterwards, not the CLI's exit code alone. Not signed in: this data source is not uploaded, and the publish goes on.

Two alternatives, not taken:

- **anfra drives the device flow itself:** `motherduck login --headless` prints the code, the URL (`https://auth.motherduck.com/activate?user_code=…`) and when it expires, and `motherduck login --device-code=<code>` finishes it. anfra could open the browser and show the code in its own style. It would parse the CLI's text, which has no `-o json`, for a nicer look than the CLI's own.
- **`motherduck new`:** creates an organization with no sign-up, and saves its token. It is unclaimed until claimed in a browser (`motherduck new claim`). What happens to an unclaimed organization and its data is not documented. Making an account for the user is not anfra's to do unasked.

### A file in use

DuckDB locks a file it opens for writing, and no other process can then open it at all, even `READ_ONLY` (`Could not set lock on file … Conflicting lock is held in <program> (PID <pid>)`). canal-query opens DuckDB files for writing and pools them, so a running `anfra serve` holds the file of every DuckDB data source it has queried, for up to its pool's idle eviction (3 hours, `SoftMinEvictableIdleTime` in canal's `connpool.go`).

A plain copy of the file does not take DuckDB's lock, and uploads: `CREATE DATABASE … FROM '<copy>'` works while the original is held. What the copy has is what is in the file on disk. A writer can hold committed changes it has not written to the file yet: in a test, rows a holder inserted were in the file only once it exited. So a copy is right when the holder only reads, and may be out of date or torn when it writes.

So, on that error:

- **The holder is anfra's canal-query** (the error names the program): it only reads. Copy the file, and its `.wal` if there is one, to a temporary folder, check the original's size and modification time are unchanged across the copy, upload the copy, delete it.
- **Anything else:** say which program holds it, from the error, and ask to close it and continue, or skip.

Releasing the file instead, so the server lets go, needs canal-query to close a data source's pooled connections on request. It has no such endpoint today (`/health`, `/query`, `/do_lake_action`). The copy needs nothing from canal-query, at the cost of disk space for the copy while it uploads.

### Storage

MotherDuck says what an account stores (`md_information_schema.storage_info`: `active_bytes` and more, per database) but not the plan's limit. And a file's size here is not what it takes there: a test file of 524 KiB took 268 KiB in MotherDuck. So anfra cannot tell beforehand whether a file fits. It says the file's size when asking, and handles a refused upload as any failed one (step 5).

### Afterwards

The org's data source is `motherduck` and the repo's is `duckdb`. The check that says when a data source's type differs on the org (`offerDataSourcesToOrg`) treats this pair as the same, since both compile to DuckDB's SQL.

The copy does not follow the file. Once the org has the data source, a publish no longer offers anything for it. Refreshing the copy is a later command, which needs to know which file went where. anfra could record that, next to the publish state (`.anfra/`, `internal/publish/state.go`): the file's path and content hash, mapped to the org, database name and time.

## Verified

Run with the MotherDuck CLI v1.5.6 against a real account, on a throwaway database and short-lived tokens, all removed afterwards:

- `motherduck status -o json` answers `{"authenticated":true,"token_source":…,"user":"<username>",…}`.
- `motherduck query` attaches and writes local DuckDB files, so it runs DuckDB on this machine.
- `CREATE DATABASE <name> FROM '<local file>'` uploads the file (about 9 s for 0.5 MiB, mostly connecting). Running it again fails with `database with name "<name>" already exists`, and `CREATE OR REPLACE` replaces it, which anfra never sends. `ATTACH '<file>' AS src (READ_ONLY); CREATE DATABASE <name> FROM src` uploads the same, and leaves the file byte for byte as it was.
- `motherduck query` prints a failed statement as `{"success":false,"error":"…"}` on stdout; its `--timeout` must be a positive number of seconds (120 by default).
- `motherduck query` exits 1 on an error and prints `{"success":false,"error":"…"}` with `-o json`. A statement with no rows prints nothing.
- `md_information_schema.databases` lists databases in a column `name`, not `database_name`.
- The login token in `~/.motherduck/credentials.json` is `{"version":1,"motherduck_token":"<jwt>"}`: a `read_write` session token valid for 24 hours. `PRAGMA PRINT_MD_TOKEN` through the CLI returns another session token, also 24 hours, as `export motherduck_token=<jwt>` even with `-o json`.
- The REST API takes the login token to create a token for the same user: `read_scaling`, a `name`, an optional `ttl`. The response carries the secret once, with `id` and `expire_at`. `DELETE /v1/users/{username}/tokens/{id}` revokes it. The spec is at `https://api.motherduck.com/docs/specs`.
- A `read_scaling` token reads the uploaded database through `ATTACH 'md:<name>'` and `motherduck_saas_mode`, as canal's connector does, and a write is refused (`attached in read-only mode`).
- A file another process holds cannot be attached, even `READ_ONLY`, but a copy of it uploads.
- `motherduck status -o json`, signed out, exits 0 with `authenticated: false`. `motherduck login --headless` prints a device code, the activation URL and its expiry (900 seconds), and returns.
- End to end: `anfra query` on a repo whose data source is `type: motherduck` with such a token compiled AQL (anfra-node) and ran it (canal-query).

## Open

- **The CLI's output is not promised stable.** `-o json` has no stated contract across `motherduck upgrade`. Parse little (`status`, errors) and fail with the CLI's own message when it changes.
- **Linux needs glibc** for the MotherDuck CLI. On musl (Alpine) there is no CLI, so no copy.
