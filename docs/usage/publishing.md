# Publishing and login

Sign in to an organization, publish a repo, and manage the credentials stored on this machine.

## Sign in

```sh
anfra auth login https://bi.example.com/o/acme
anfra publish --set-org https://bi.example.com/o/acme
anfra auth status
anfra auth logout https://bi.example.com/o/acme
```

Login opens a browser with a loopback callback. Consent chooses the org; it may differ
from the address's hint. Login saves the returned org's credentials but never changes
the repo's destination. Device/headless login is not supported yet.

Human-readable login also prints the authorization URL to stderr so you can open it
manually on the same machine. If the browser launcher fails, login keeps waiting for
approval until you cancel or the ten-minute login timeout expires. Treat this URL as
private; it contains login state. It is not included in JSON output or diagnostic logs.
With `--json`, a browser-launch failure still exits with an error instead of waiting.

Bare `anfra auth login` offers the build's optional default cloud URL alongside
`Enter your organization URL`. Runtime `ANFRA_CLOUD_URL` overrides that default;
an explicitly empty value disables it. With neither a build default nor a runtime value,
login goes straight to URL entry.
The cloud choice uses the configured origin without an organization hint. In the browser,
consent preselects your last used eligible organization; you can change it before approving.
Entering an organization URL sends its slug as a hint. Login always starts fresh browser
approval; it does not reuse credentials or switch the repo.
Use up/down and Enter, Esc to go back/cancel, and Ctrl-C to cancel anywhere.
Interaction requires terminal stdin and stderr, outside CI and `TERM=dumb`; stdout may be
redirected. Otherwise supply an explicit URL. `ANFRA_CLOUD_URL` must be an HTTPS origin,
or HTTP on literal `127.0.0.1` or `[::1]` for local testing; see
[DEVELOPMENT.md](../../DEVELOPMENT.md#cloud-login-url).

## Stored credentials and logout

Credentials are in `<ANFRA_HOME>/credentials.json`, defaulting to
`~/.anfra/credentials.json`, with mode `0600`. Keep this file private and out of git.
Multiple server/org logins coexist. Status lists cached names and canonical URLs without
network access or tokens, so a login revoked on its server still shows as valid. It marks
the current repo's org (its publish destination), not a global active org, and lists that
org even when no login for it is saved.
From nested directories it reads the nearest `.anfra/config.yml`, stopping at the nearest
Git root (including worktrees), home directory or filesystem root. A nearer config without
`org.url` stops lookup; malformed config is an error. A login is `valid`, `invalid`
(locally expired grants, an uncertain refresh, a damaged entry; access expiry alone isn't),
or `missing`, for the current repo's org with none saved. Cached names can be stale. Logout uses its address, else repo `org.url`, else an
interactive picker. It removes only the selected local login, even when remote
revocation fails. In that case, revoke it on the server's `/account/connected-apps` page.

## Scripted use

All auth commands accept `--yaml` or `--json` for one secret-free result on stdout, and
`--non-interactive` (or `--no-interactive`) to disable prompts and animation. A script passes both:
`--json` alone still asks on a terminal. Explicit
login still needs browser approval in either mode. Each result opens with `message`, the
outcome, and status's and logout's end with `hints`, what to do next. Login reports `org_url`;
it never changes the repo's destination. Status reports `orgs`, each with `name` (absent
without a saved login), `url`, `saved_login` (`valid`, `invalid` or `missing`) and
`current_repo`. Logout reports `org_url`, `local_removed` and `remote_revocation_confirmed`;
a failed revocation after local removal fails with `command_failed`, whose details are that
result. Progress goes to stderr; final results go to stdout. `NO_COLOR` disables colors, not
keyboard navigation. Auth remains machine-local, outside the HTTP/MCP command registry.

## Publish a repo

Publish uses `org.url` or `--set-org` and that destination's credentials. With no
destination, an interactive terminal always offers stored logins and new authentication.
After selection and successful credential loading, it saves `org.url` before publishing,
preserving other config. A later publish failure leaves that destination configured.
Without a terminal, provide a destination and matching stored credentials; there is no
automatic selection or browser launch. `--yes` only skips overwrite confirmation.
`anfra publish check` lists the files a publish would send, and where to, without authentication
or config writes: the preflight, which checks more as it grows. It was `--list`, and before it
`--dry-run`, which still work for now, with a warning.

## Refresh and recovery

Refresh happens on demand within five minutes of access expiry, under one cross-process
lock. If a process crashes, a response is lost or a credential save fails, the login may
be marked uncertain. Run `anfra auth login <org address>` again; do not clear pending
state or retry the old refresh token by hand. Revoke the old grant in connected apps.
The former development session-cookie environment hook is no longer used.

